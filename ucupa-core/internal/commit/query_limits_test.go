package commit_test

import (
	"errors"
	"reflect"
	"testing"

	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	"ucupa/ucupa-core/internal/commit"
	"ucupa/ucupa-core/internal/geometry"
	queryoracle "ucupa/ucupa-core/internal/query/oracle"
)

func TestQueryRegionExactEdgeBoundaryAndWholeQueryGap(t *testing.T) {
	if geometry.MaxExactProjectionEdges != 1024 || geometry.MaxExactRationalBits != 65536 {
		t.Fatal("QueryRegion exact-work limits differ from its accepted contract")
	}
	c := newQueryFixture(t)
	root := queryCreateRoot(t, c, linearOracleID(20000), "query-edge-root")
	anchors := commit.BuildQueryEdgeFixtureForTest(t, c.owner)
	atLimit, shallow := linearOracleID(30000), linearOracleID(30001)

	query := contract.RegionQuery{StartAnchorID: root, UpDepth: 0, BasisAnchorID: root, Sphere: contract.QuerySphere{}, Selector: contract.QueryEntities}
	// The selected Entity at exactly 1024 edges is calculable. The unselected
	// Anchor at 1025 edges is not by itself a numeric gap for QueryEntities.
	assertQueryMatchesOracle(t, c, query, []values.ElementID{atLimit, shallow})
	query.BasisAnchorID = anchors[1024]
	assertQueryMatchesOracle(t, c, query, []values.ElementID{atLimit, shallow})
	beforeFailure := commit.CaptureCompleteStateForTest(c.owner)
	query.BasisAnchorID = anchors[1025]
	page, err := c.queries.QueryRegion(linearOracleActor, query, nil, 1, nil)
	assertQueryInternalError(t, page, err, contract.ErrUnrepresentable)
	if !commit.MatchesCompleteStateForTest(c.owner, beforeFailure) {
		t.Fatal("unrepresentable Basis query changed state")
	}

	// This UUID sorts before every calculable hit. A suffix-only gap scan
	// would incorrectly report COMPLETE on the final page.
	overLimit := linearOracleID(1)
	commit.AddQueryOverflowForTest(t, c.owner)

	before := commit.CaptureCompleteStateForTest(c.owner)
	query.BasisAnchorID = root
	first, err := c.queries.QueryRegion(linearOracleActor, query, nil, 1, nil)
	assertQueryInternalGapPage(t, first, err, []values.ElementID{atLimit}, true)
	last, err := c.queries.QueryRegion(linearOracleActor, query, nil, 1, first.Continuation)
	assertQueryInternalGapPage(t, last, err, []values.ElementID{shallow}, false)
	if !last.Cut.Equal(first.Cut) {
		t.Fatal("gap pagination changed cut")
	}
	replay, err := c.queries.QueryRegion(linearOracleActor, query, nil, 1, first.Continuation)
	if err != nil || !reflect.DeepEqual(replay, last) {
		t.Fatalf("same gap cursor replay changed: got=%#v err=%v want=%#v", replay, err, last)
	}
	query.Sphere.Center.X = 1 // Both calculable points are definitively outside.
	gapOnly, err := c.queries.QueryRegion(linearOracleActor, query, nil, 1, nil)
	assertQueryInternalGapPage(t, gapOnly, err, nil, false)
	if !commit.MatchesCompleteStateForTest(c.owner, before) {
		t.Fatal("partial queries changed facts, cut, or retained operations")
	}
	read, err := c.elements.ReadElement(linearOracleActor, overLimit, nil)
	if err != nil || read.State != contract.ExistencePresent {
		t.Fatalf("numeric work limit damaged deep owner truth: %#v err=%v", read, err)
	}
	// Choosing a local Scope does not inherit the distance to the root as a
	// semantic depth limit: both the deep entity and its parent remain usable.
	assertQueryMatchesOracle(t, c, contract.RegionQuery{StartAnchorID: anchors[1024], UpDepth: 0, BasisAnchorID: anchors[1024], Sphere: contract.QuerySphere{}, Selector: contract.QueryEntities}, []values.ElementID{overLimit})
}

func TestQueryRegionRationalGapPreservesIndependentSiblingHits(t *testing.T) {
	c := newQueryFixture(t)
	root := queryCreateRoot(t, c, linearOracleID(40000), "query-bit-root")
	parent := commit.BuildQueryRationalFixtureForTest(t, c.owner)
	goodA, goodB := linearOracleID(45000), linearOracleID(45001)

	before := commit.CaptureCompleteStateForTest(c.owner)
	query := contract.RegionQuery{StartAnchorID: root, UpDepth: 0, BasisAnchorID: root, Sphere: contract.QuerySphere{}, Selector: contract.QueryEntities}
	// The oracle is all-or-nothing here, not the source of expected partial
	// results. The two root children independently have exact origin=(0,0,0).
	oracle, err := queryoracle.Scan(c.owner, root, root, nil, contract.QuerySphere{}, queryoracle.Entities)
	if !errors.Is(err, contract.ErrUnrepresentable) || len(oracle.Records) != 0 {
		t.Fatalf("fixture did not exceed rational work limit: %#v err=%v", oracle, err)
	}
	page, err := c.queries.QueryRegion(linearOracleActor, query, nil, 256, nil)
	assertQueryInternalGapPage(t, page, err, []values.ElementID{goodA, goodB}, false)
	query.BasisAnchorID = parent
	page, err = c.queries.QueryRegion(linearOracleActor, query, nil, 256, nil)
	assertQueryInternalError(t, page, err, contract.ErrUnrepresentable)
	if !commit.MatchesCompleteStateForTest(c.owner, before) {
		t.Fatal("rational limit queries changed facts or retained history")
	}
}
