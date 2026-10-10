package query_test

import (
	"errors"
	"fmt"
	"math"
	"reflect"
	"testing"

	"ucupa/shared/values"
	corecontract "ucupa/ucupa-core/contract"
	core "ucupa/ucupa-core/instance"
)

func TestQueryRegionScopeAndBasisFindCrossBranchMatches(t *testing.T) {
	f := newQueryTestTree(t)
	for _, tc := range []struct {
		name  string
		scope values.ElementID
		want  []values.ElementID
	}{
		{"root", f.root.Anchor.ID, []values.ElementID{f.siblingEntity.Entity.ID, f.higherEntity.Entity.ID, f.outsideEntity.Entity.ID, f.ownEntity.Entity.ID}},
		{"bounded-scope", f.scope.Anchor.ID, []values.ElementID{f.siblingEntity.Entity.ID, f.higherEntity.Entity.ID, f.ownEntity.Entity.ID}},
		{"branch-scope", f.branch.Anchor.ID, []values.ElementID{f.siblingEntity.Entity.ID, f.ownEntity.Entity.ID}},
		{"basis-is-scope", f.basis.Anchor.ID, []values.ElementID{f.ownEntity.Entity.ID}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			query := corecontract.RegionQuery{
				StartAnchorID: tc.scope,
				BasisAnchorID: f.basis.Anchor.ID,
				Sphere:        corecontract.QuerySphere{Radius: 5},
				Selector:      corecontract.QueryEntities,
			}
			page, err := f.c.Queries().QueryRegion(regionActor, query, nil, 256, nil)
			assertQueryTestComplete(t, page, err)
			assertQueryTestIDs(t, page, tc.want)
			assertQueryTestMatchesAtCut(t, f.c, tc.scope, page)
		})
	}
}

func TestQueryRegionSelectorsIncludeScopeAndEmptyAnchors(t *testing.T) {
	f := newQueryTestTree(t)
	for _, tc := range []struct {
		name     string
		selector corecontract.QuerySelector
		want     []values.ElementID
	}{
		{"anchors", corecontract.QueryAnchors, []values.ElementID{f.scope.Anchor.ID, f.branch.Anchor.ID, f.basis.Anchor.ID, f.sibling.Anchor.ID, f.higher.Anchor.ID, f.empty.Anchor.ID}},
		{"entities", corecontract.QueryEntities, []values.ElementID{f.siblingEntity.Entity.ID, f.higherEntity.Entity.ID, f.ownEntity.Entity.ID}},
		{"both", corecontract.QueryBoth, []values.ElementID{f.scope.Anchor.ID, f.branch.Anchor.ID, f.basis.Anchor.ID, f.sibling.Anchor.ID, f.siblingEntity.Entity.ID, f.higher.Anchor.ID, f.higherEntity.Entity.ID, f.ownEntity.Entity.ID, f.empty.Anchor.ID}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			query := corecontract.RegionQuery{StartAnchorID: f.scope.Anchor.ID, BasisAnchorID: f.basis.Anchor.ID, Sphere: corecontract.QuerySphere{Radius: 5}, Selector: tc.selector}
			page, err := f.c.Queries().QueryRegion(regionActor, query, nil, 256, nil)
			assertQueryTestComplete(t, page, err)
			assertQueryTestIDs(t, page, tc.want)
			assertQueryTestMatchesAtCut(t, f.c, f.scope.Anchor.ID, page)
		})
	}
}

func TestQueryRegionClosedSphereTangencyAndEmptyResults(t *testing.T) {
	c := newRegionCore(t)
	root := regionCreateRoot(t, c, regionID(6100), "root")
	tangent := regionCreateEntity(t, c, regionID(6101), regionTuple(t, root.Anchor.ID, regionPose(t, 3, 0, 0), 2), "tangent")
	regionCreateEntity(t, c, regionID(6102), regionTuple(t, root.Anchor.ID, regionPose(t, math.Nextafter(3, math.Inf(1)), 0, 0), 2), "outside")
	query := corecontract.RegionQuery{StartAnchorID: root.Anchor.ID, BasisAnchorID: root.Anchor.ID, Sphere: corecontract.QuerySphere{Radius: 1}, Selector: corecontract.QueryEntities}
	page, err := c.Queries().QueryRegion(regionActor, query, nil, 256, nil)
	assertQueryTestComplete(t, page, err)
	assertQueryTestIDs(t, page, []values.ElementID{tangent.Entity.ID})
	assertQueryTestMatchesAtCut(t, c, root.Anchor.ID, page)

	query.Sphere = corecontract.QuerySphere{Center: corecontract.Vec3{X: 1000}, Radius: 0}
	empty, err := c.Queries().QueryRegion(regionActor, query, nil, 256, nil)
	assertQueryTestComplete(t, empty, err)
	assertQueryTestIDs(t, empty, nil)
	if !empty.Cut.Equal(page.Cut) {
		t.Fatal("read-only empty query changed cut")
	}
	query.Selector = corecontract.QueryAnchors
	anchors, err := c.Queries().QueryRegion(regionActor, query, nil, 256, nil)
	assertQueryTestComplete(t, anchors, err)
	assertQueryTestIDs(t, anchors, []values.ElementID{root.Anchor.ID})
	assertQueryTestMatchesAtCut(t, c, root.Anchor.ID, anchors)
}

func TestQueryRegionUsesRotatedBasisForSiblingEntity(t *testing.T) {
	c := newRegionCore(t)
	root := regionCreateRoot(t, c, regionID(6120), "root")
	quarterTurn, err := corecontract.NewRotation(1, 0, 0, 1)
	if err != nil {
		t.Fatal(err)
	}
	basisPose, err := corecontract.NewPose(corecontract.Vec3{Y: 2}, quarterTurn)
	if err != nil {
		t.Fatal(err)
	}
	basis := regionCreateAnchor(t, c, regionID(6121), root.Anchor.ID, regionTuple(t, root.Anchor.ID, basisPose, 10), "rotated-basis")
	target := regionCreateEntity(t, c, regionID(6122), regionTuple(t, root.Anchor.ID, regionPose(t, 0, 3, 0), 0), "target")
	regionCreateEntity(t, c, regionID(6123), regionTuple(t, root.Anchor.ID, regionPose(t, 1, 2, 0), 0), "translation-only-decoy")
	query := corecontract.RegionQuery{
		StartAnchorID: root.Anchor.ID,
		BasisAnchorID: basis.Anchor.ID,
		Sphere:        corecontract.QuerySphere{Center: corecontract.Vec3{X: 1}, Radius: 0},
		Selector:      corecontract.QueryEntities,
	}
	page, err := c.Queries().QueryRegion(regionActor, query, nil, 256, nil)
	assertQueryTestComplete(t, page, err)
	assertQueryTestIDs(t, page, []values.ElementID{target.Entity.ID})
	assertQueryTestMatchesAtCut(t, c, root.Anchor.ID, page)
}

func TestQueryRegionEntityOriginProbeUsesSameReadCut(t *testing.T) {
	c := newRegionCore(t)
	root := regionCreateRoot(t, c, regionID(6110), "root")
	parent := regionCreateAnchor(t, c, regionID(6111), root.Anchor.ID, regionTuple(t, root.Anchor.ID, regionPose(t, 10, 0, 0), 10), "parent")
	probe := regionCreateEntity(t, c, regionID(6112), regionTuple(t, parent.Anchor.ID, regionPose(t, 2, 0, 0), 0), "probe")
	target := regionCreateEntity(t, c, regionID(6113), regionTuple(t, parent.Anchor.ID, regionPose(t, 4, 0, 0), 1), "target")
	read, err := c.Elements().ReadElement(regionActor, probe.Entity.ID, nil)
	if err != nil || read.Record == nil || read.Record.Entity == nil {
		t.Fatalf("read probe: %#v err=%v", read, err)
	}
	query := corecontract.RegionQuery{
		StartAnchorID: root.Anchor.ID,
		BasisAnchorID: read.Record.Entity.ContainingAnchorID,
		Sphere:        corecontract.QuerySphere{Center: read.Record.Entity.LocalPose.Translation, Radius: 1},
		Selector:      corecontract.QueryEntities,
	}
	page, err := c.Queries().QueryRegion(regionActor, query, &read.Cut, 256, nil)
	assertQueryTestComplete(t, page, err)
	assertQueryTestIDs(t, page, []values.ElementID{probe.Entity.ID, target.Entity.ID})
	if !page.Cut.Equal(read.Cut) {
		t.Fatal("probe query ignored explicit cut")
	}

	moved, err := c.Elements().UpdateEntity(regionActor, probe.Entity.ID, regionTuple(t, parent.Anchor.ID, regionPose(t, 8, 0, 0), 0), probe.Entity.Revision, "move-probe")
	if err != nil || moved.Status != corecontract.MutationCommitted {
		t.Fatalf("move probe: %#v err=%v", moved, err)
	}
	stale, err := c.Queries().QueryRegion(regionActor, query, &read.Cut, 256, nil)
	assertQueryTestError(t, stale, err, corecontract.ErrReadCutExpired)
	read, err = c.Elements().ReadElement(regionActor, probe.Entity.ID, nil)
	if err != nil || read.Record == nil || read.Record.Entity == nil {
		t.Fatalf("refresh probe: %#v err=%v", read, err)
	}
	query.BasisAnchorID = read.Record.Entity.ContainingAnchorID
	query.Sphere.Center = read.Record.Entity.LocalPose.Translation
	refreshed, err := c.Queries().QueryRegion(regionActor, query, &read.Cut, 256, nil)
	assertQueryTestComplete(t, refreshed, err)
	assertQueryTestIDs(t, refreshed, []values.ElementID{probe.Entity.ID})
	assertQueryTestMatchesAtCut(t, c, root.Anchor.ID, refreshed)
}

func TestQueryRegionRejectsInvalidInputAndContext(t *testing.T) {
	f := newQueryTestTree(t)
	base := corecontract.RegionQuery{StartAnchorID: f.scope.Anchor.ID, BasisAnchorID: f.basis.Anchor.ID, Sphere: corecontract.QuerySphere{Radius: 5}, Selector: corecontract.QueryEntities}
	for _, tc := range []struct {
		name string
		edit func(*corecontract.RegionQuery)
		want error
	}{
		{"bad-scope-id", func(q *corecontract.RegionQuery) { q.StartAnchorID = "bad" }, corecontract.ErrInvalid},
		{"bad-basis-id", func(q *corecontract.RegionQuery) { q.BasisAnchorID = "bad" }, corecontract.ErrInvalid},
		{"missing-scope", func(q *corecontract.RegionQuery) { q.StartAnchorID = regionID(6199) }, corecontract.ErrInvalid},
		{"entity-scope", func(q *corecontract.RegionQuery) { q.StartAnchorID = f.ownEntity.Entity.ID }, corecontract.ErrInvalid},
		{"missing-basis", func(q *corecontract.RegionQuery) { q.BasisAnchorID = regionID(6199) }, corecontract.ErrInvalid},
		{"entity-basis", func(q *corecontract.RegionQuery) { q.BasisAnchorID = f.ownEntity.Entity.ID }, corecontract.ErrInvalid},
		{"outside-basis", func(q *corecontract.RegionQuery) { q.BasisAnchorID = f.outside.Anchor.ID }, corecontract.ErrInvalid},
		{"zero-selector", func(q *corecontract.RegionQuery) { q.Selector = 0 }, corecontract.ErrInvalid},
		{"unknown-selector", func(q *corecontract.RegionQuery) { q.Selector = corecontract.QuerySelector(255) }, corecontract.ErrInvalid},
		{"nan-center", func(q *corecontract.RegionQuery) { q.Sphere.Center.X = math.NaN() }, corecontract.ErrInvalid},
		{"infinite-center", func(q *corecontract.RegionQuery) { q.Sphere.Center.Y = math.Inf(1) }, corecontract.ErrInvalid},
		{"negative-radius", func(q *corecontract.RegionQuery) { q.Sphere.Radius = -1 }, corecontract.ErrInvalid},
		{"nan-radius", func(q *corecontract.RegionQuery) { q.Sphere.Radius = math.NaN() }, corecontract.ErrInvalid},
		{"infinite-radius", func(q *corecontract.RegionQuery) { q.Sphere.Radius = math.Inf(1) }, corecontract.ErrInvalid},
	} {
		t.Run(tc.name, func(t *testing.T) {
			query := base
			tc.edit(&query)
			page, err := f.c.Queries().QueryRegion(regionActor, query, nil, 10, nil)
			assertQueryTestError(t, page, err, tc.want)
		})
	}
	for _, limit := range []uint16{0, 257, math.MaxUint16} {
		t.Run(fmt.Sprintf("limit-%d", limit), func(t *testing.T) {
			page, err := f.c.Queries().QueryRegion(regionActor, base, nil, limit, nil)
			assertQueryTestError(t, page, err, corecontract.ErrInvalid)
		})
	}
	zeroCut := corecontract.CoreReadCut{}
	page, err := f.c.Queries().QueryRegion(regionActor, base, &zeroCut, 10, nil)
	assertQueryTestError(t, page, err, corecontract.ErrInvalid)
	zeroContinuation := corecontract.QueryContinuation{}
	page, err = f.c.Queries().QueryRegion(regionActor, base, nil, 10, &zeroContinuation)
	assertQueryTestError(t, page, err, corecontract.ErrInvalid)
}

func TestQueryRegionPaginationChangesLimitAndReplays(t *testing.T) {
	c, root, query, want := newQueryTestPages(t)
	read, err := c.Elements().ReadElement(regionActor, root.Anchor.ID, nil)
	if err != nil {
		t.Fatal(err)
	}
	first, err := c.Queries().QueryRegion(regionActor, query, &read.Cut, 2, nil)
	assertQueryTestPartial(t, first, err)
	assertQueryTestIDs(t, first, want[:2])
	assertQueryTestMatchesAtCut(t, c, root.Anchor.ID, first)
	if !first.Cut.Equal(read.Cut) {
		t.Fatal("first page ignored explicit cut")
	}
	bad, err := c.Queries().QueryRegion(regionActor, query, &first.Cut, 1, first.Continuation)
	assertQueryTestError(t, bad, err, corecontract.ErrInvalid)

	second, err := c.Queries().QueryRegion(regionActor, query, nil, 1, first.Continuation)
	assertQueryTestPartial(t, second, err)
	assertQueryTestIDs(t, second, want[2:3])
	replayed, err := c.Queries().QueryRegion(regionActor, query, nil, 1, first.Continuation)
	if err != nil || !reflect.DeepEqual(replayed, second) {
		t.Fatalf("same cursor replay changed page: want=%#v got=%#v err=%v", second, replayed, err)
	}
	last, err := c.Queries().QueryRegion(regionActor, query, nil, 256, second.Continuation)
	assertQueryTestComplete(t, last, err)
	assertQueryTestIDs(t, last, want[3:])
	if !second.Cut.Equal(first.Cut) || !last.Cut.Equal(first.Cut) {
		t.Fatal("pagination mixed fact cuts")
	}
	assertQueryTestMatchesAtCut(t, c, root.Anchor.ID, second)
	assertQueryTestMatchesAtCut(t, c, root.Anchor.ID, last)
	var joined []values.ElementID
	for _, page := range []corecontract.QueryPage{first, second, last} {
		for _, match := range page.Matches {
			joined = append(joined, match.ElementID)
		}
	}
	if !reflect.DeepEqual(joined, want) {
		t.Fatalf("incomplete or duplicated pages: got=%v want=%v", joined, want)
	}
}

func TestQueryRegionContinuationBindings(t *testing.T) {
	c, root, query, _ := newQueryTestPages(t)
	first, err := c.Queries().QueryRegion(regionActor, query, nil, 1, nil)
	assertQueryTestPartial(t, first, err)
	for _, tc := range []struct {
		name string
		edit func(*corecontract.RegionQuery)
	}{
		{"scope", func(q *corecontract.RegionQuery) { q.StartAnchorID = regionID(6201) }},
		{"basis", func(q *corecontract.RegionQuery) { q.BasisAnchorID = regionID(6201) }},
		{"center-x", func(q *corecontract.RegionQuery) { q.Sphere.Center.X = 1 }},
		{"center-y", func(q *corecontract.RegionQuery) { q.Sphere.Center.Y = 1 }},
		{"center-z", func(q *corecontract.RegionQuery) { q.Sphere.Center.Z = 1 }},
		{"radius", func(q *corecontract.RegionQuery) { q.Sphere.Radius++ }},
		{"selector", func(q *corecontract.RegionQuery) { q.Selector = corecontract.QueryBoth }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			changed := query
			tc.edit(&changed)
			page, err := c.Queries().QueryRegion(regionActor, changed, nil, 1, first.Continuation)
			assertQueryTestError(t, page, err, corecontract.ErrInvalid)
		})
	}
	otherActor := values.PrincipalRef{Realm: regionActor.Realm, Principal: "other"}
	page, err := c.Queries().QueryRegion(otherActor, query, nil, 1, first.Continuation)
	assertQueryTestError(t, page, err, corecontract.ErrInvalid)
	foreign, _, _, _ := newQueryTestPages(t)
	page, err = foreign.Queries().QueryRegion(regionActor, query, nil, 1, first.Continuation)
	assertQueryTestError(t, page, err, corecontract.ErrInvalid)
	page, err = foreign.Queries().QueryRegion(regionActor, query, &first.Cut, 1, nil)
	assertQueryTestError(t, page, err, corecontract.ErrInvalid)

	// Binding failure must win over expiration, even when the original Core
	// advances after issuing the cursor.
	regionCreateEntity(t, c, regionID(6299), regionTuple(t, root.Anchor.ID, regionPose(t, 1000, 0, 0), 0), "advance")
	changed := query
	changed.Sphere.Radius++
	page, err = c.Queries().QueryRegion(regionActor, changed, nil, 1, first.Continuation)
	assertQueryTestError(t, page, err, corecontract.ErrInvalid)
	page, err = c.Queries().QueryRegion(otherActor, query, nil, 1, first.Continuation)
	assertQueryTestError(t, page, err, corecontract.ErrInvalid)
}

func TestQueryRegionCanonicalZerosPreserveContinuationIdentity(t *testing.T) {
	c := newRegionCore(t)
	root := regionCreateRoot(t, c, regionID(6300), "root")
	for offset := uint64(1); offset <= 2; offset++ {
		regionCreateEntity(t, c, regionID(6300+offset), regionTuple(t, root.Anchor.ID, regionPose(t, 0, 0, 0), 0), corecontract.OperationID(fmt.Sprintf("point-%d", offset)))
	}
	negativeZero := math.Copysign(0, -1)
	query := corecontract.RegionQuery{StartAnchorID: root.Anchor.ID, BasisAnchorID: root.Anchor.ID, Sphere: corecontract.QuerySphere{Center: corecontract.Vec3{X: negativeZero, Y: negativeZero, Z: negativeZero}, Radius: negativeZero}, Selector: corecontract.QueryEntities}
	first, err := c.Queries().QueryRegion(regionActor, query, nil, 1, nil)
	assertQueryTestPartial(t, first, err)
	query.Sphere = corecontract.QuerySphere{}
	last, err := c.Queries().QueryRegion(regionActor, query, nil, 1, first.Continuation)
	assertQueryTestComplete(t, last, err)
	assertQueryTestIDs(t, first, []values.ElementID{regionID(6301)})
	assertQueryTestIDs(t, last, []values.ElementID{regionID(6302)})
	if !last.Cut.Equal(first.Cut) {
		t.Fatal("canonical-equivalent request changed cut")
	}
}

func TestQueryRegionContinuationSurvivesNonFactOutcomesButExpiresOnCommit(t *testing.T) {
	c, root, query, want := newQueryTestPages(t)
	first, err := c.Queries().QueryRegion(regionActor, query, nil, 1, nil)
	assertQueryTestPartial(t, first, err)
	read, err := c.Elements().ReadElement(regionActor, want[0], &first.Cut)
	if err != nil || read.Record == nil || read.Record.Entity == nil {
		t.Fatalf("read existing entity: %#v err=%v", read, err)
	}
	entity := read.Record.Entity
	final := regionTuple(t, entity.ContainingAnchorID, entity.LocalPose, entity.Range.Radius())
	unchanged, err := c.Elements().UpdateEntity(regionActor, entity.ID, final, entity.Revision, "unchanged")
	if err != nil || unchanged.Status != corecontract.MutationUnchanged || !unchanged.OutcomeCut.Equal(first.Cut) {
		t.Fatalf("unchanged: %#v err=%v", unchanged, err)
	}
	rejected, err := c.Elements().CreateEntity(regionActor, entity.ID, final, "occupied")
	assertRegionRejected(t, rejected, err, corecontract.ErrConflict)
	if !rejected.OutcomeCut.Equal(first.Cut) {
		t.Fatal("negative retained outcome advanced cut")
	}
	second, err := c.Queries().QueryRegion(regionActor, query, nil, 1, first.Continuation)
	assertQueryTestPartial(t, second, err)
	assertQueryTestIDs(t, second, want[1:2])
	if !second.Cut.Equal(first.Cut) {
		t.Fatal("non-fact outcome expired or changed query cut")
	}

	regionCreateEntity(t, c, regionID(6298), regionTuple(t, root.Anchor.ID, regionPose(t, 1000, 0, 0), 0), "unrelated-commit")
	stale, err := c.Queries().QueryRegion(regionActor, query, nil, 1, first.Continuation)
	assertQueryTestError(t, stale, err, corecontract.ErrReadCutExpired)
	stale, err = c.Queries().QueryRegion(regionActor, query, &first.Cut, 1, nil)
	assertQueryTestError(t, stale, err, corecontract.ErrReadCutExpired)
	restarted, err := c.Queries().QueryRegion(regionActor, query, nil, 256, nil)
	assertQueryTestComplete(t, restarted, err)
	assertQueryTestIDs(t, restarted, want)
	if restarted.Cut.Equal(first.Cut) {
		t.Fatal("restart retained stale cut")
	}
}

func TestQueryRegionReturnedMatchesPathsAndCursorDoNotAlias(t *testing.T) {
	c, root, query, want := newQueryTestPages(t)
	first, err := c.Queries().QueryRegion(regionActor, query, nil, 2, nil)
	assertQueryTestPartial(t, first, err)
	assertQueryTestMatchesAtCut(t, c, root.Anchor.ID, first)
	if len(first.Matches) != 2 || len(first.Matches[0].Path) != 2 || len(first.Matches[1].Path) != 2 {
		t.Fatalf("alias fixture: %#v", first)
	}
	savedCursor := *first.Continuation
	secondPath := append([]values.ElementID(nil), first.Matches[1].Path...)
	first.Matches[0].Path[0] = regionID(9998)
	if !reflect.DeepEqual(first.Matches[1].Path, secondPath) {
		t.Fatal("one returned Path aliases another match Path")
	}
	first.Matches[0].Path[1] = regionID(9999)
	first.Matches[0].ElementID = regionID(9999)
	first.Matches[0].Kind = corecontract.ElementKind(255)
	first.Matches[0].Revision = corecontract.Revision{}
	*first.Continuation = corecontract.QueryContinuation{}

	again, err := c.Queries().QueryRegion(regionActor, query, &first.Cut, 2, nil)
	assertQueryTestPartial(t, again, err)
	assertQueryTestIDs(t, again, want[:2])
	assertQueryTestMatchesAtCut(t, c, root.Anchor.ID, again)
	remaining, err := c.Queries().QueryRegion(regionActor, query, nil, 256, &savedCursor)
	assertQueryTestComplete(t, remaining, err)
	assertQueryTestIDs(t, remaining, want[2:])
	assertQueryTestMatchesAtCut(t, c, root.Anchor.ID, remaining)
}

type queryTestTree struct {
	c             *core.Core
	root          corecontract.ElementRecord
	scope         corecontract.ElementRecord
	branch        corecontract.ElementRecord
	basis         corecontract.ElementRecord
	sibling       corecontract.ElementRecord
	siblingEntity corecontract.ElementRecord
	higher        corecontract.ElementRecord
	higherEntity  corecontract.ElementRecord
	outside       corecontract.ElementRecord
	outsideEntity corecontract.ElementRecord
	ownEntity     corecontract.ElementRecord
	empty         corecontract.ElementRecord
}

func newQueryTestTree(t *testing.T) queryTestTree {
	t.Helper()
	f := queryTestTree{c: newRegionCore(t)}
	f.root = regionCreateRoot(t, f.c, regionID(6000), "root")
	f.scope = regionCreateAnchor(t, f.c, regionID(6001), f.root.Anchor.ID, regionTuple(t, f.root.Anchor.ID, regionPose(t, 0, 0, 0), 100), "scope")
	f.branch = regionCreateAnchor(t, f.c, regionID(6002), f.scope.Anchor.ID, regionTuple(t, f.scope.Anchor.ID, regionPose(t, 10, 0, 0), 50), "branch")
	f.basis = regionCreateAnchor(t, f.c, regionID(6003), f.branch.Anchor.ID, regionTuple(t, f.branch.Anchor.ID, regionPose(t, 5, 0, 0), 10), "basis")
	f.sibling = regionCreateAnchor(t, f.c, regionID(6004), f.branch.Anchor.ID, regionTuple(t, f.branch.Anchor.ID, regionPose(t, 20, 0, 0), 15), "sibling")
	f.siblingEntity = regionCreateEntity(t, f.c, regionID(6005), regionTuple(t, f.sibling.Anchor.ID, regionPose(t, -5, 0, 0), 5), "sibling-entity")
	f.higher = regionCreateAnchor(t, f.c, regionID(6006), f.scope.Anchor.ID, regionTuple(t, f.scope.Anchor.ID, regionPose(t, 50, 0, 0), 45), "higher-sibling")
	f.higherEntity = regionCreateEntity(t, f.c, regionID(6007), regionTuple(t, f.higher.Anchor.ID, regionPose(t, -15, 0, 0), 25), "higher-entity")
	f.outside = regionCreateAnchor(t, f.c, regionID(6008), f.root.Anchor.ID, regionTuple(t, f.root.Anchor.ID, regionPose(t, 40, 0, 0), 30), "root-sibling")
	f.outsideEntity = regionCreateEntity(t, f.c, regionID(6009), regionTuple(t, f.outside.Anchor.ID, regionPose(t, 0, 0, 0), 30), "root-sibling-entity")
	f.ownEntity = regionCreateEntity(t, f.c, regionID(6010), regionTuple(t, f.basis.Anchor.ID, regionPose(t, 0, 0, 0), 1), "own-entity")
	f.empty = regionCreateAnchor(t, f.c, regionID(6011), f.basis.Anchor.ID, regionTuple(t, f.basis.Anchor.ID, regionPose(t, 5, 0, 0), 0), "empty-point-anchor")
	regionCreateEntity(t, f.c, regionID(6012), regionTuple(t, f.scope.Anchor.ID, regionPose(t, -80, 0, 0), 1), "away")
	return f
}

func newQueryTestPages(t *testing.T) (*core.Core, corecontract.ElementRecord, corecontract.RegionQuery, []values.ElementID) {
	t.Helper()
	c := newRegionCore(t)
	root := regionCreateRoot(t, c, regionID(6200), "root")
	parent := regionCreateAnchor(t, c, regionID(6201), root.Anchor.ID, regionTuple(t, root.Anchor.ID, regionPose(t, 0, 0, 0), 20), "parent")
	// Creation order intentionally differs from UUID result order.
	for _, offset := range []uint64{6, 2, 5, 1, 4, 3} {
		regionCreateEntity(t, c, regionID(6201+offset), regionTuple(t, parent.Anchor.ID, regionPose(t, float64(offset), 0, 0), 1), corecontract.OperationID(fmt.Sprintf("entity-%d", offset)))
	}
	query := corecontract.RegionQuery{StartAnchorID: root.Anchor.ID, BasisAnchorID: root.Anchor.ID, Sphere: corecontract.QuerySphere{Radius: 100}, Selector: corecontract.QueryEntities}
	want := []values.ElementID{regionID(6202), regionID(6203), regionID(6204), regionID(6205), regionID(6206), regionID(6207)}
	return c, root, query, want
}

func assertQueryTestComplete(t *testing.T, page corecontract.QueryPage, err error) {
	t.Helper()
	if err != nil || page.Status != corecontract.QueryComplete || page.Gap != corecontract.QueryNoGap || page.Continuation != nil || page.Cut.Equal(corecontract.CoreReadCut{}) {
		t.Fatalf("complete query: %#v err=%v", page, err)
	}
}

func assertQueryTestPartial(t *testing.T, page corecontract.QueryPage, err error) {
	t.Helper()
	if err != nil || page.Status != corecontract.QueryPartial || page.Gap != corecontract.QueryNoGap || page.Continuation == nil || len(page.Matches) == 0 || page.Cut.Equal(corecontract.CoreReadCut{}) {
		t.Fatalf("paged query: %#v err=%v", page, err)
	}
}

func assertQueryTestError(t *testing.T, page corecontract.QueryPage, err, want error) {
	t.Helper()
	if !errors.Is(err, want) || !reflect.DeepEqual(page, corecontract.QueryPage{}) {
		t.Fatalf("query error: page=%#v err=%v want=%v", page, err, want)
	}
}

func assertQueryTestIDs(t *testing.T, page corecontract.QueryPage, want []values.ElementID) {
	t.Helper()
	got := make([]values.ElementID, len(page.Matches))
	for index, match := range page.Matches {
		got[index] = match.ElementID
	}
	if len(got) != len(want) {
		t.Fatalf("query IDs: got=%v want=%v", got, want)
	}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("query IDs: got=%v want=%v", got, want)
		}
	}
}

func assertQueryTestMatchesAtCut(t *testing.T, c *core.Core, scope values.ElementID, page corecontract.QueryPage) {
	t.Helper()
	for _, match := range page.Matches {
		read, err := c.Elements().ReadElement(regionActor, match.ElementID, &page.Cut)
		if err != nil || read.State != corecontract.ExistencePresent || read.Record == nil || !read.Cut.Equal(page.Cut) || read.Record.Kind != match.Kind {
			t.Fatalf("same-cut read for match %#v: %#v err=%v", match, read, err)
		}
		var revision corecontract.Revision
		if match.Kind == corecontract.ElementAnchor {
			if read.Record.Anchor == nil {
				t.Fatalf("Anchor match lacks Anchor record: %#v", read)
			}
			revision = read.Record.Anchor.Revision
		} else if match.Kind == corecontract.ElementEntity {
			if read.Record.Entity == nil {
				t.Fatalf("Entity match lacks Entity record: %#v", read)
			}
			revision = read.Record.Entity.Revision
		} else {
			t.Fatalf("unknown match kind: %#v", match)
		}
		if !revision.Equal(match.Revision) {
			t.Fatalf("match revision differs from same-cut record: %#v", match)
		}
		current := scope
		for _, step := range match.Path {
			edge, err := c.Elements().ReadElement(regionActor, step, &page.Cut)
			if err != nil || edge.State != corecontract.ExistencePresent || edge.Record == nil || !edge.Cut.Equal(page.Cut) {
				t.Fatalf("same-cut path edge %q: %#v err=%v", step, edge, err)
			}
			var parent values.ElementID
			if edge.Record.Anchor != nil && edge.Record.Anchor.ParentAnchorID != nil {
				parent = *edge.Record.Anchor.ParentAnchorID
			} else if edge.Record.Entity != nil {
				parent = edge.Record.Entity.ContainingAnchorID
			}
			if parent != current {
				t.Fatalf("path contains false edge %q -> %q for match %#v", current, step, match)
			}
			current = step
		}
		if current != match.ElementID {
			t.Fatalf("path does not end at match: scope=%q match=%#v", scope, match)
		}
	}
}

var regionActor = values.PrincipalRef{Realm: "test-realm", Principal: "builder"}

type regionAllowAuthorizer struct{}

func (regionAllowAuthorizer) Authorize(values.PrincipalRef, ...values.Requirement) bool { return true }

func newRegionCore(t *testing.T) *core.Core {
	t.Helper()
	c, err := core.New(queryPolicyContact{regionAllowAuthorizer{}}, queryContact, corecontract.Config{})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func regionID(n uint64) values.ElementID {
	return values.ElementID(fmt.Sprintf("00000000-0000-4000-8000-%012x", n))
}

func regionPose(t *testing.T, x, y, z float64) corecontract.Pose {
	t.Helper()
	r, err := corecontract.NewRotation(1, 0, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	p, err := corecontract.NewPose(corecontract.Vec3{X: x, Y: y, Z: z}, r)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func regionTuple(t *testing.T, parent values.ElementID, p corecontract.Pose, radius float64) corecontract.SpatialTuple {
	t.Helper()
	s, err := corecontract.NewBoundedSphere(radius)
	if err != nil {
		t.Fatal(err)
	}
	return corecontract.SpatialTuple{Parent: parent, LocalPose: p, Range: s}
}

func regionCreateRoot(t *testing.T, c *core.Core, v values.ElementID, op corecontract.OperationID) corecontract.ElementRecord {
	t.Helper()
	r, err := c.Elements().CreateRoot(regionActor, v, op)
	if err != nil || r.Status != corecontract.MutationCommitted || r.Record == nil {
		t.Fatalf("create root: %#v %v", r, err)
	}
	return *r.Record
}

func regionCreateAnchor(t *testing.T, c *core.Core, v, parent values.ElementID, final corecontract.SpatialTuple, op corecontract.OperationID) corecontract.ElementRecord {
	t.Helper()
	if final.Parent != parent {
		t.Fatal("bad helper parent")
	}
	r, err := c.Elements().CreateAnchor(regionActor, v, final, op)
	if err != nil || r.Status != corecontract.MutationCommitted || r.Record == nil {
		t.Fatalf("create anchor: %#v %v", r, err)
	}
	return *r.Record
}

func regionCreateEntity(t *testing.T, c *core.Core, v values.ElementID, final corecontract.SpatialTuple, op corecontract.OperationID) corecontract.ElementRecord {
	t.Helper()
	r, err := c.Elements().CreateEntity(regionActor, v, final, op)
	if err != nil || r.Status != corecontract.MutationCommitted || r.Record == nil {
		t.Fatalf("create entity: %#v %v", r, err)
	}
	return *r.Record
}

func assertRegionRejected(t *testing.T, r corecontract.MutationResult, err, want error) {
	t.Helper()
	if !errors.Is(err, want) || r.Status != corecontract.MutationRejected || r.Record != nil || r.OutcomeCut.Equal(corecontract.CoreReadCut{}) {
		t.Fatalf("rejected %#v err=%v want=%v", r, err, want)
	}
}
