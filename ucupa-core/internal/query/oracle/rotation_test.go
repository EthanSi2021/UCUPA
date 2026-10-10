package oracle_test

import (
	"testing"
	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	queryoracle "ucupa/ucupa-core/internal/query/oracle"
)

func TestLinearOracleRotatedBasisCrossBranchGolden(t *testing.T) {
	c, err := newOracleFixture()
	if err != nil {
		t.Fatal(err)
	}
	root := oracleCreateRoot(t, c, oracleID(6000), "rotation-root")
	quarterTurn, err := contract.NewRotation(1, 0, 0, 1)
	if err != nil {
		t.Fatal(err)
	}
	leftPose, err := contract.NewPose(contract.Vec3{X: 10}, quarterTurn)
	if err != nil {
		t.Fatal(err)
	}
	leftRange, err := contract.NewBoundedSphere(100)
	if err != nil {
		t.Fatal(err)
	}
	left := oracleCreateAnchor(
		t, c, oracleID(6001),
		contract.SpatialTuple{Parent: root, LocalPose: leftPose, Range: leftRange},
		"rotation-left",
	)
	basis := oracleCreateAnchor(
		t, c, oracleID(6002), oracleTuple(t, left, 2, 0, 0, 50),
		"rotation-basis",
	)
	right := oracleCreateAnchor(
		t, c, oracleID(6003), oracleTuple(t, root, 10, 12, 0, 100),
		"rotation-right",
	)
	positiveX := oracleCreateEntity(
		t, c, oracleID(6004), oracleTuple(t, right, 0, 0, 0, 0),
		"rotation-positive-x",
	)
	negativeX := oracleCreateEntity(
		t, c, oracleID(6005), oracleTuple(t, right, 0, -20, 0, 0),
		"rotation-negative-x",
	)

	// The basis origin is (10,2) in root coordinates. Its +90-degree Z
	// rotation maps the sibling centers (10,12) and (10,-8) to respectively
	// (+10,0) and (-10,0) in basis coordinates.
	positive, err := queryoracle.Scan(c.owner,
		root, basis, nil, contract.QuerySphere{Center: contract.Vec3{X: 10}}, queryoracle.Entities,
	)
	if err != nil || len(positive.Records) != 1 || positive.Records[0].Entity == nil ||
		positive.Records[0].Entity.ID != positiveX {
		t.Fatalf("+90Z positive-basis golden: %#v err=%v", positive, err)
	}
	negative, err := queryoracle.Scan(c.owner,
		root, basis, nil, contract.QuerySphere{Center: contract.Vec3{X: -10}}, queryoracle.Entities,
	)
	if err != nil || len(negative.Records) != 1 || negative.Records[0].Entity == nil ||
		negative.Records[0].Entity.ID != negativeX {
		t.Fatalf("+90Z negative-basis golden: %#v err=%v", negative, err)
	}
	if !positive.Cut.Equal(negative.Cut) {
		t.Fatal("unchanged rotated-basis scans selected different cuts")
	}
}

func TestLinearOracleNonCommutingRotationChainGolden(t *testing.T) {
	c, err := newOracleFixture()
	if err != nil {
		t.Fatal(err)
	}
	root := oracleCreateRoot(t, c, oracleID(6100), "non-commuting-root")
	quarterX, err := contract.NewRotation(1, 1, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	quarterY, err := contract.NewRotation(1, 0, 1, 0)
	if err != nil {
		t.Fatal(err)
	}
	quarterZ, err := contract.NewRotation(1, 0, 0, 1)
	if err != nil {
		t.Fatal(err)
	}
	tuple := func(parent values.ElementID, translation contract.Vec3, rotation contract.Rotation, radius float64) contract.SpatialTuple {
		t.Helper()
		pose, poseErr := contract.NewPose(translation, rotation)
		if poseErr != nil {
			t.Fatal(poseErr)
		}
		rangeValue, rangeErr := contract.NewBoundedSphere(radius)
		if rangeErr != nil {
			t.Fatal(rangeErr)
		}
		return contract.SpatialTuple{Parent: parent, LocalPose: pose, Range: rangeValue}
	}

	// The basis world transform is Rz*Rx with origin (0,1,0).
	basisOuter := oracleCreateAnchor(
		t, c, oracleID(6101), tuple(root, contract.Vec3{}, quarterZ, 100),
		"non-commuting-basis-outer",
	)
	basis := oracleCreateAnchor(
		t, c, oracleID(6102), tuple(basisOuter, contract.Vec3{X: 1}, quarterX, 50),
		"non-commuting-basis",
	)

	// The target world transform is Rx*Ry. Its deep local +X translation gives
	// world center (10,1,1), hence basis coordinates (0,1,10). Reversing either
	// non-commuting product does not produce that point; reversing both produces
	// (0,-10,1), which the second query explicitly rejects.
	targetOuter := oracleCreateAnchor(
		t, c, oracleID(6103), tuple(root, contract.Vec3{X: 10}, quarterX, 100),
		"non-commuting-target-outer",
	)
	targetParent := oracleCreateAnchor(
		t, c, oracleID(6104), tuple(targetOuter, contract.Vec3{Y: 1}, quarterY, 50),
		"non-commuting-target-parent",
	)
	target := oracleCreateEntity(
		t, c, oracleID(6105), oracleTuple(t, targetParent, 1, 0, 0, 0),
		"non-commuting-target",
	)

	hit, err := queryoracle.Scan(c.owner,
		root, basis, nil, contract.QuerySphere{Center: contract.Vec3{Y: 1, Z: 10}}, queryoracle.Entities,
	)
	if err != nil || len(hit.Records) != 1 || hit.Records[0].Entity == nil ||
		hit.Records[0].Entity.ID != target {
		t.Fatalf("non-commuting hand-computed hit: %#v err=%v", hit, err)
	}
	miss, err := queryoracle.Scan(c.owner,
		root, basis, nil, contract.QuerySphere{Center: contract.Vec3{Y: -10, Z: 1}}, queryoracle.Entities,
	)
	if err != nil || len(miss.Records) != 0 {
		t.Fatalf("reversed-product coordinate must miss: %#v err=%v", miss, err)
	}
	if !hit.Cut.Equal(miss.Cut) {
		t.Fatal("unchanged non-commuting scans selected different cuts")
	}
}
