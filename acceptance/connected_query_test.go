package acceptance_test

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"testing"

	"ucupa/shared/values"
	corecontract "ucupa/ucupa-core/contract"
	peercontract "ucupa/ucupa-core/contract/connected"
	core "ucupa/ucupa-core/instance"
)

func TestP3ConnectedFourRetentionConfigurations(t *testing.T) {
	for _, a := range []bool{false, true} {
		for _, b := range []bool{false, true} {
			t.Run(fmt.Sprintf("A=%t/B=%t", a, b), func(t *testing.T) {
				f := newConnectedFixture(t, a, b)
				first := f.queryB(1)
				mode := corecontract.QueryCurrentVerified
				if a && b {
					mode = corecontract.QueryRetained
				}
				if first.ReadMode != mode || first.ActualUpDepth != 2 ||
					!connectedPathsEqual(first.ResolvedScopePath, corecontract.Path{OriginAnchorID: mountBranch, Up: 2}) {
					t.Fatalf("wrong mode, actual depth or scope: %#v", first)
				}
				matches := collectConnected(t, f.b, f.callers(), f.queryFromB(), first, 1)
				want := []values.ElementID{mountRoot, mountAnchor, mountOther, mountBranch, mountEntity,
					connectedRootEntity, connectedSiblingEntity, connectedDirectEntity}
				sort.Slice(want, func(i, j int) bool { return want[i] < want[j] })
				if got := connectedIDs(matches); !reflect.DeepEqual(got, want) {
					t.Fatalf("brother/ancestor/descendant closure: %v, want %v", got, want)
				}
				fromA, err := f.a.Connected().QueryRegionPath(context.Background(), f.callers(), f.queryFromA(), nil, 256, nil)
				if err != nil || fromA.ReadMode != mode || !reflect.DeepEqual(fromA.Matches, matches) {
					t.Fatalf("entry deployment changed discovery: %#v, %v", fromA, err)
				}
				for _, match := range matches {
					read, err := f.b.Connected().ResolvePath(context.Background(), f.callers(), connectedReadPath(first.ResolvedScopePath, match), &first.Cut)
					if err != nil || read.State != corecontract.ExistencePresent || read.Record == nil || read.ReadMode != mode || read.Source != match.Source {
						t.Fatalf("scope-relative exact read %s: %#v, %v", match.ElementID, read, err)
					}
					id, revision := connectedRecordIdentity(t, *read.Record)
					if id != match.ElementID || revision != match.Revision || read.Record.Kind != match.Kind {
						t.Fatalf("exact record differs from discovery: %#v vs %#v", read, match)
					}
					owner := f.a.AuthorityRef()
					if id == mountBranch || id == mountEntity || id == connectedDirectEntity {
						owner = f.b.AuthorityRef()
					}
					if read.Source.Authority != owner {
						t.Fatalf("wrong source for %s: %v", id, read.Source.Authority)
					}
				}
				if err := f.b.Connected().ReleaseConnectedCut(context.Background(), f.callers(), first.Cut); err != nil {
					t.Fatal(err)
				}
			})
		}
	}
}

func TestP3ConnectedDepthDoesNotLimitDescendantsOrUseCentersOnly(t *testing.T) {
	f := newConnectedFixture(t, false, false)
	for _, test := range []struct {
		depth int64
		want  []values.ElementID
	}{
		{0, []values.ElementID{mountEntity, mountBranch}},
		{1, []values.ElementID{mountAnchor, mountEntity, mountBranch, connectedDirectEntity}},
	} {
		q := f.queryFromB()
		q.UpDepth = test.depth
		page, err := f.b.Connected().QueryRegionPath(context.Background(), f.callers(), q, nil, 256, nil)
		if err != nil || page.Status != corecontract.QueryComplete || page.Gap != corecontract.QueryNoGap || !reflect.DeepEqual(connectedIDs(page.Matches), test.want) {
			t.Fatalf("depth %d: %#v, %v", test.depth, page, err)
		}
	}
	q := f.queryFromB()
	q.Selector, q.Sphere = corecontract.QueryEntities, corecontract.QuerySphere{}
	page, err := f.b.Connected().QueryRegionPath(context.Background(), f.callers(), q, nil, 256, nil)
	if err != nil || page.Status != corecontract.QueryComplete || !reflect.DeepEqual(connectedIDs(page.Matches), []values.ElementID{mountEntity}) {
		t.Fatalf("entity centered outside point probe was omitted: %#v, %v", page, err)
	}
	q = f.queryFromB()
	q.UpDepth = 100
	page, err = f.b.Connected().QueryRegionPath(context.Background(), f.callers(), q, nil, 256, nil)
	if err != nil || page.ActualUpDepth != 2 || page.ResolvedScopePath.Up != 2 {
		t.Fatalf("root saturation: %#v, %v", page, err)
	}
	q.UpDepth = -2
	page, err = f.b.Connected().QueryRegionPath(context.Background(), f.callers(), q, nil, 256, nil)
	assertConnectedQueryError(t, page, err, corecontract.ErrInvalid)
	q = f.queryFromB()
	q.UpDepth = 0
	q.BasisPath.Up = 2
	page, err = f.b.Connected().QueryRegionPath(context.Background(), f.callers(), q, nil, 256, nil)
	assertConnectedQueryError(t, page, err, corecontract.ErrInvalid)
}

func TestP3ConnectedListMountContentAndProveMissingAdjacency(t *testing.T) {
	f := newConnectedFixture(t, true, true)
	path := corecontract.Path{OriginAnchorID: mountBranch, Up: 1}
	first, err := f.b.Connected().ListDirectPath(context.Background(), f.callers(), path, nil, 1, nil)
	if err != nil || first.Continuation == nil || len(first.Records) != 1 {
		t.Fatalf("first direct page: %#v, %v", first, err)
	}
	last, err := f.b.Connected().ListDirectPath(context.Background(), f.callers(), path, nil, 1, first.Continuation)
	if err != nil || last.Continuation != nil || last.Status != corecontract.QueryComplete || !last.Cut.Equal(first.Cut) {
		t.Fatalf("last direct page: %#v, %v", last, err)
	}
	var ids []values.ElementID
	for _, item := range append(first.Records, last.Records...) {
		id, _ := connectedRecordIdentity(t, item.Record)
		ids = append(ids, id)
		if item.Source.Authority != f.b.AuthorityRef() || id == mountAnchor {
			t.Fatal("direct contents fabricated or duplicated parent-owned M")
		}
	}
	if !reflect.DeepEqual(ids, []values.ElementID{mountBranch, connectedDirectEntity}) {
		t.Fatalf("wrong direct adjacency: %v", ids)
	}
	read, err := f.b.Connected().ResolvePath(context.Background(), f.callers(), path, &first.Cut)
	if err != nil || read.Source.Authority != f.a.AuthorityRef() || read.Record.Anchor.ID != mountAnchor {
		t.Fatalf("M record owner: %#v, %v", read, err)
	}
	missing := corecontract.Path{OriginAnchorID: mountRoot, Down: []values.ElementID{mountAnchor, connectedRootEntity}}
	read, err = f.a.Connected().ResolvePath(context.Background(), f.callers(), missing, nil)
	if err != nil || read.State != corecontract.ExistenceNotFound || read.AbsenceSource.Authority != f.b.AuthorityRef() {
		t.Fatalf("missing adjacency searched another branch: %#v, %v", read, err)
	}
}

func TestP3ConnectedBasisIsIndependentOfStartWithinSelectedScope(t *testing.T) {
	f := newConnectedFixture(t, true, true)
	expected := f.queryB(256)
	q := f.queryFromB()
	q.BasisPath = corecontract.Path{OriginAnchorID: mountBranch, Up: 2, Down: []values.ElementID{mountOther}}
	q.Sphere.Center.X = 27 // A's sibling basis is at -20; B's start is at +7.
	page, err := f.b.Connected().QueryRegionPath(context.Background(), f.callers(), q, nil, 256, nil)
	if err != nil || page.Status != corecontract.QueryComplete || !reflect.DeepEqual(page.Matches, expected.Matches) {
		t.Fatalf("changing to equivalent sibling basis changed matches: %#v, %v", page, err)
	}
}

func TestP3ConnectedQueryAndExactReadHaveIndependentPermissions(t *testing.T) {
	f := newConnectedFixture(t, true, true)
	f.revokePermission(false, mountEntity, corecontract.PermissionReadElement)
	page := f.queryB(256)
	var target corecontract.ConnectedMatch
	for _, match := range page.Matches {
		if match.ElementID == mountEntity {
			target = match
		}
	}
	if target.ElementID != mountEntity {
		t.Fatal("query incorrectly required unknown entity exact-read grant")
	}
	read, err := f.b.Connected().ResolvePath(context.Background(), f.callers(), connectedReadPath(page.ResolvedScopePath, target), &page.Cut)
	if !errors.Is(err, corecontract.ErrDenied) || !reflect.ValueOf(read).IsZero() {
		t.Fatalf("query grant authorized exact read: %#v, %v", read, err)
	}
	callers := f.callers()
	callers[f.a.AuthorityRef()] = f.actorB
	denied, err := f.b.Connected().QueryRegionPath(context.Background(), callers, f.queryFromB(), nil, 256, nil)
	assertConnectedQueryError(t, denied, err, corecontract.ErrDenied)
	denied, err = f.b.Connected().QueryRegionPath(context.Background(), corecontract.LocalCallers{f.b.AuthorityRef(): f.actorB}, f.queryFromB(), nil, 256, nil)
	assertConnectedQueryError(t, denied, err, corecontract.ErrDenied)
	f.revokePermission(true, mountRoot, corecontract.PermissionQueryRegion)
	denied, err = f.b.Connected().QueryRegionPath(context.Background(), f.callers(), f.queryFromB(), nil, 256, nil)
	assertConnectedQueryError(t, denied, err, corecontract.ErrDenied)
	localScope := f.queryFromB()
	localScope.UpDepth = 0
	narrow, err := f.b.Connected().QueryRegionPath(context.Background(), f.callers(), localScope, nil, 256, nil)
	if err != nil || narrow.Status != corecontract.QueryComplete || len(narrow.Matches) != 2 {
		t.Fatalf("B-only scope unnecessarily required parent-wide query grant: %#v, %v", narrow, err)
	}
}

func TestP3ConnectedDuplicateElementIdentityRejectsWholeQuery(t *testing.T) {
	for _, retained := range []bool{false, true} {
		t.Run(fmt.Sprintf("retained=%t", retained), func(t *testing.T) {
			f := newConnectedFixture(t, retained, retained)
			// Both stores accept their own local UUID; composition must not turn
			// those two facts into one result or choose an owner by arrival order.
			if _, err := f.a.Elements().CreateEntity(f.actorA, mountEntity, f.tuple(mountOther, 0, 1), "connected-duplicate-identity"); err != nil {
				t.Fatal(err)
			}
			for _, owner := range []*core.Core{f.a, f.b} {
				query := f.queryFromA()
				if owner == f.b {
					query = f.queryFromB()
				}
				page, err := owner.Connected().QueryRegionPath(context.Background(), f.callers(), query, nil, 1, nil)
				assertConnectedQueryError(t, page, err, peercontract.ErrIdentityConflict)
			}
		})
	}
}

func TestP3ConnectedNarrowReadDoesNotReauthorizeOriginalQuery(t *testing.T) {
	for _, retained := range []bool{false, true} {
		t.Run(fmt.Sprintf("retained=%t", retained), func(t *testing.T) {
			f := newConnectedFixture(t, retained, retained)
			first := f.queryB(256)
			var target corecontract.ConnectedMatch
			for _, match := range first.Matches {
				if match.ElementID == mountEntity {
					target = match
				}
			}
			if target.ElementID != mountEntity {
				t.Fatal("query did not capture the intended exact-read target")
			}
			f.revokePermission(true, mountRoot, corecontract.PermissionQueryRegion)
			f.revokePermission(false, mountAnchor, corecontract.PermissionQueryRegion)
			path := connectedReadPath(first.ResolvedScopePath, target)
			read, err := f.b.Connected().ResolvePath(context.Background(), f.callers(), path, &first.Cut)
			if err != nil || read.State != corecontract.ExistencePresent || read.Record == nil ||
				read.Record.Entity == nil || read.Record.Entity.ID != mountEntity || read.Source != target.Source {
				t.Fatalf("exact read unnecessarily reauthorized original query: %#v, %v", read, err)
			}
			f.revokePermission(false, mountEntity, corecontract.PermissionReadElement)
			read, err = f.b.Connected().ResolvePath(context.Background(), f.callers(), path, &first.Cut)
			if !errors.Is(err, corecontract.ErrDenied) || !reflect.ValueOf(read).IsZero() {
				t.Fatalf("retained contents bypassed current exact-read permission: %#v, %v", read, err)
			}
		})
	}
}

func collectConnected(t *testing.T, owner *core.Core, callers corecontract.LocalCallers, query corecontract.PathRegionQuery, first corecontract.ConnectedQueryPage, limit uint16) []corecontract.ConnectedMatch {
	t.Helper()
	page := first
	var matches []corecontract.ConnectedMatch
	for pages := 0; pages < 64; pages++ {
		if page.ReadMode != first.ReadMode || !page.Cut.Equal(first.Cut) || page.Gap != corecontract.QueryNoGap ||
			page.ActualUpDepth != first.ActualUpDepth || !connectedPathsEqual(page.ResolvedScopePath, first.ResolvedScopePath) {
			t.Fatalf("pagination changed fixed context: %#v", page)
		}
		matches = append(matches, page.Matches...)
		if page.Continuation == nil {
			if page.Status != corecontract.QueryComplete {
				t.Fatalf("static complete source ended incomplete: %#v", page)
			}
			return matches
		}
		if page.Status != corecontract.QueryPartial || len(page.Matches) != int(limit) {
			t.Fatalf("invalid progressing page: %#v", page)
		}
		next, err := owner.Connected().QueryRegionPath(context.Background(), callers, query, nil, limit, page.Continuation)
		if err != nil {
			t.Fatal(err)
		}
		replay, err := owner.Connected().QueryRegionPath(context.Background(), callers, query, nil, limit, page.Continuation)
		if err != nil || !reflect.DeepEqual(replay, next) {
			t.Fatalf("replaying cursor changed page: %#v vs %#v, %v", replay, next, err)
		}
		page = next
	}
	t.Fatal("pagination failed to make bounded progress")
	return nil
}

func connectedIDs(matches []corecontract.ConnectedMatch) []values.ElementID {
	ids := make([]values.ElementID, len(matches))
	for i, match := range matches {
		ids[i] = match.ElementID
	}
	return ids
}

func connectedPathsEqual(a, b corecontract.Path) bool {
	if a.OriginAnchorID != b.OriginAnchorID || a.Up != b.Up || len(a.Down) != len(b.Down) {
		return false
	}
	for i := range a.Down {
		if a.Down[i] != b.Down[i] {
			return false
		}
	}
	return true
}

func connectedRecordIdentity(t *testing.T, record corecontract.ElementRecord) (values.ElementID, corecontract.Revision) {
	t.Helper()
	if record.Kind == corecontract.ElementAnchor && record.Anchor != nil {
		return record.Anchor.ID, record.Anchor.Revision
	}
	if record.Kind == corecontract.ElementEntity && record.Entity != nil {
		return record.Entity.ID, record.Entity.Revision
	}
	t.Fatalf("malformed record: %#v", record)
	return "", corecontract.Revision{}
}

func assertConnectedQueryError(t *testing.T, page corecontract.ConnectedQueryPage, err, want error) {
	t.Helper()
	if !errors.Is(err, want) || !reflect.ValueOf(page).IsZero() {
		t.Fatalf("expected zero page / %v, got %#v / %v", want, page, err)
	}
}
