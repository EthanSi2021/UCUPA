package oracle_test

import (
	"errors"
	"fmt"
	"math"
	"testing"
	"ucupa/ucupa-core/contract"
	queryoracle "ucupa/ucupa-core/internal/query/oracle"
	queryscan "ucupa/ucupa-core/internal/query/scan"
)

func TestLinearOracleMemoizesRepeatedAncestorProjection(t *testing.T) {
	c, err := newOracleFixture()
	if err != nil {
		t.Fatal(err)
	}
	root := oracleCreateRoot(t, c, oracleID(4000), "memo-root")
	parent := root
	const depth = 64
	for index := 1; index <= depth; index++ {
		parent = oracleCreateAnchor(
			t, c, oracleID(uint64(4000+index)),
			oracleTuple(t, parent, 0, 0, 0, 1000),
			contract.OperationID(fmt.Sprintf("memo-anchor-%d", index)),
		)
	}
	const entities = 64
	for index := 1; index <= entities; index++ {
		oracleCreateEntity(
			t, c, oracleID(uint64(5000+index)),
			oracleTuple(t, parent, 0, 0, 0, 0),
			contract.OperationID(fmt.Sprintf("memo-entity-%d", index)),
		)
	}

	stats := queryscan.Stats{}
	result, err := queryoracle.ScanMeasured(c.owner,
		root, root, nil, contract.QuerySphere{Radius: 1}, queryoracle.Entities, &stats,
	)
	if err != nil || len(result.Records) != entities {
		t.Fatalf("memoized scan: records=%d err=%v", len(result.Records), err)
	}
	if stats.AnchorRecordsScanned != depth+1 || stats.ClosureAnchorsVisited != depth+1 ||
		stats.EntityRecordsScanned != entities {
		t.Fatalf("non-linear closure accounting: %#v", stats)
	}
	if stats.AnchorProjectionEdges != depth || stats.EntityProjectionEdges != entities {
		t.Fatalf("ancestor projection was repeated: %#v", stats)
	}
}

func TestLinearOracleExactBitLimitReturnsNoPartialMatches(t *testing.T) {
	c, err := newOracleFixture()
	if err != nil {
		t.Fatal(err)
	}
	root := oracleCreateRoot(t, c, oracleID(100), "bit-root")
	rotation, err := contract.NewRotation(1, math.SmallestNonzeroFloat64, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	localPose, err := contract.NewPose(contract.Vec3{Y: 1}, rotation)
	if err != nil {
		t.Fatal(err)
	}
	parent := root
	for depth := 1; depth <= 40; depth++ {
		child := oracleID(uint64(100 + depth))
		radius, sphereErr := contract.NewBoundedSphere(float64(101 - depth))
		if sphereErr != nil {
			t.Fatal(sphereErr)
		}
		parent = oracleCreateAnchor(
			t, c, child,
			contract.SpatialTuple{Parent: parent, LocalPose: localPose, Range: radius},
			contract.OperationID(fmt.Sprintf("bit-%d", depth)),
		)
	}
	result, err := queryoracle.Scan(c.owner, root, root, nil, contract.QuerySphere{Radius: 1}, queryoracle.Both)
	if !errors.Is(err, contract.ErrUnrepresentable) || len(result.Records) != 0 ||
		!result.Cut.Equal(contract.CoreReadCut{}) {
		t.Fatalf("bit-limit scan returned partial facts: %#v err=%v", result, err)
	}
	deepest, readErr := c.Elements().ReadElement(oracleActor, parent, nil)
	if readErr != nil || deepest.State != contract.ExistencePresent || deepest.Record == nil {
		t.Fatalf("bit-limit scan truncated deepest truth: %#v err=%v", deepest, readErr)
	}
}
