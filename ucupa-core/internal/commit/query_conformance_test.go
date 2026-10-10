package commit_test

import (
	"errors"
	"math"
	"testing"

	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	"ucupa/ucupa-core/internal/commit"
)

func TestQueryRegionCompleteMatchesSameCutOracle(t *testing.T) {
	c := newQueryFixture(t)
	root := queryCreateRoot(t, c, linearOracleID(10000), "query-root")
	scope := queryCreateAnchor(t, c, linearOracleID(10001), queryFixtureTuple(t, root, 0, 0, 0, 100), "query-scope")
	left := queryCreateAnchor(t, c, linearOracleID(10002), queryFixtureTuple(t, scope, -20, 0, 0, 20), "query-left")
	basisParent := queryCreateAnchor(t, c, linearOracleID(10003), queryFixtureTuple(t, left, 0, 0, 0, 10), "query-basis-parent")
	basis := queryCreateAnchor(t, c, linearOracleID(10004), queryFixtureTuple(t, basisParent, 0, 0, 0, 5), "query-basis")
	right := queryCreateAnchor(t, c, linearOracleID(10005), queryFixtureTuple(t, scope, 20, 0, 0, 30), "query-right")
	empty := queryCreateAnchor(t, c, linearOracleID(10006), queryFixtureTuple(t, right, 1, 0, 0, 1), "query-empty")
	large := queryCreateEntity(t, c, linearOracleID(10007), queryFixtureTuple(t, right, 15, 0, 0, 15), "query-large")
	queryCreateEntity(t, c, linearOracleID(10008), queryFixtureTuple(t, root, -20, 0, 0, 1), "query-outside-scope")
	before := commit.CaptureCompleteStateForTest(c.owner)

	for _, test := range []struct {
		name  string
		query contract.RegionQuery
		want  []values.ElementID
	}{
		{"entity tangent across higher sibling", contract.RegionQuery{StartAnchorID: scope, UpDepth: 0, BasisAnchorID: basis, Sphere: contract.QuerySphere{Radius: 40}, Selector: contract.QueryEntities}, []values.ElementID{large}},
		{"anchors include scope and empty tangent", contract.RegionQuery{StartAnchorID: scope, UpDepth: 0, BasisAnchorID: basis, Sphere: contract.QuerySphere{Radius: 40}, Selector: contract.QueryAnchors}, []values.ElementID{scope, left, basisParent, basis, right, empty}},
		{"both selectors", contract.RegionQuery{StartAnchorID: scope, UpDepth: 0, BasisAnchorID: basis, Sphere: contract.QuerySphere{Radius: 40}, Selector: contract.QueryBoth}, []values.ElementID{scope, left, basisParent, basis, right, empty, large}},
		{"complete empty", contract.RegionQuery{StartAnchorID: scope, UpDepth: 0, BasisAnchorID: basis, Sphere: contract.QuerySphere{Center: contract.Vec3{X: 1000}}, Selector: contract.QueryEntities}, nil},
		{"unbounded root", contract.RegionQuery{StartAnchorID: root, UpDepth: 0, BasisAnchorID: root, Sphere: contract.QuerySphere{Center: contract.Vec3{X: 1000}}, Selector: contract.QueryAnchors}, []values.ElementID{root}},
	} {
		t.Run(test.name, func(t *testing.T) {
			assertQueryMatchesOracle(t, c, test.query, test.want)
		})
	}
	if !commit.MatchesCompleteStateForTest(c.owner, before) {
		t.Fatal("complete queries changed facts, revisions, cut, or retained operations")
	}
}

func TestQueryRegionNonCommutingBasisMatchesSameCutOracle(t *testing.T) {
	c := newQueryFixture(t)
	root := queryCreateRoot(t, c, linearOracleID(11000), "query-rotation-root")
	rz, err := contract.NewRotation(1, 0, 0, 1)
	if err != nil {
		t.Fatal(err)
	}
	rx, err := contract.NewRotation(1, 1, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	ry, err := contract.NewRotation(1, 0, 1, 0)
	if err != nil {
		t.Fatal(err)
	}
	rotated := func(parent values.ElementID, position contract.Vec3, rotation contract.Rotation, radius float64) contract.SpatialTuple {
		pose, poseErr := contract.NewPose(position, rotation)
		if poseErr != nil {
			t.Fatal(poseErr)
		}
		rangeValue, rangeErr := contract.NewBoundedSphere(radius)
		if rangeErr != nil {
			t.Fatal(rangeErr)
		}
		return contract.SpatialTuple{Parent: parent, LocalPose: pose, Range: rangeValue}
	}
	basisOuter := queryCreateAnchor(t, c, linearOracleID(11001), rotated(root, contract.Vec3{}, rz, 100), "query-basis-outer")
	basis := queryCreateAnchor(t, c, linearOracleID(11002), rotated(basisOuter, contract.Vec3{X: 1}, rx, 50), "query-rotated-basis")
	targetOuter := queryCreateAnchor(t, c, linearOracleID(11003), rotated(root, contract.Vec3{X: 10}, rx, 100), "query-target-outer")
	targetParent := queryCreateAnchor(t, c, linearOracleID(11004), rotated(targetOuter, contract.Vec3{Y: 1}, ry, 50), "query-target-parent")
	entity := queryCreateEntity(t, c, linearOracleID(11005), queryFixtureTuple(t, targetParent, 1, 0, 0, 0), "query-rotated-entity")
	// Independent golden: world origin=(10,1,1), basis origin=(0,1,0),
	// basis rotation=Rz*Rx, so the Entity is at (0,1,10) in Basis.
	assertQueryMatchesOracle(t, c, contract.RegionQuery{StartAnchorID: root, UpDepth: 0, BasisAnchorID: basis, Sphere: contract.QuerySphere{Center: contract.Vec3{Y: 1, Z: 10}}, Selector: contract.QueryEntities}, []values.ElementID{entity})
	assertQueryMatchesOracle(t, c, contract.RegionQuery{StartAnchorID: root, UpDepth: 0, BasisAnchorID: basis, Sphere: contract.QuerySphere{Center: contract.Vec3{Y: -10, Z: 1}}, Selector: contract.QueryEntities}, nil)
}

func TestQueryRegionExactTwiceMaxFloatTangentNeedsNoFiniteProjection(t *testing.T) {
	c := newQueryFixture(t)
	root := queryCreateRoot(t, c, linearOracleID(50000), "query-max-root")
	m := math.MaxFloat64
	anchor := queryCreateAnchor(t, c, linearOracleID(50001), queryFixtureTuple(t, root, m, 0, 0, m), "query-max-anchor")
	entity := queryCreateEntity(t, c, linearOracleID(50002), queryFixtureTuple(t, anchor, m, 0, 0, 0), "query-max-entity")
	before := commit.CaptureCompleteStateForTest(c.owner)
	projection, err := c.elements.ProjectToAncestor(linearOracleActor, entity, root, nil)
	if !errors.Is(err, contract.ErrUnrepresentable) || projection != (contract.Projection{}) {
		t.Fatalf("2M unexpectedly had a finite projection DTO: %#v err=%v", projection, err)
	}
	// Exact distance |2M-M|=M. Binary64 overflow is irrelevant because the
	// QueryMatch has no projected Pose and tangency is an exact predicate.
	assertQueryMatchesOracle(t, c, contract.RegionQuery{StartAnchorID: root, UpDepth: 0, BasisAnchorID: root, Sphere: contract.QuerySphere{Center: contract.Vec3{X: m}, Radius: m}, Selector: contract.QueryEntities}, []values.ElementID{entity})
	if !commit.MatchesCompleteStateForTest(c.owner, before) {
		t.Fatal("exact intersection changed state")
	}
}
