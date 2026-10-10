package acceptance_test

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"

	"ucupa/shared/values"
	corecontract "ucupa/ucupa-core/contract"
	core "ucupa/ucupa-core/instance"
)

func TestP3ConnectedWritesRespectSelectedReadMode(t *testing.T) {
	for _, a := range []bool{false, true} {
		for _, b := range []bool{false, true} {
			t.Run(fmt.Sprintf("A=%t/B=%t", a, b), func(t *testing.T) {
				f := newConnectedFixture(t, a, b)
				first := f.queryB(1)
				before := f.queryB(256)
				f.moveBEntity("connected-move-during-pagination")
				if !a || !b {
					page, err := f.b.Connected().QueryRegionPath(context.Background(), f.callers(), f.queryFromB(), nil, 1, first.Continuation)
					assertConnectedQueryError(t, page, err, corecontract.ErrReadCutExpired)
				} else {
					matches := collectConnected(t, f.b, f.callers(), f.queryFromB(), first, 1)
					if !reflect.DeepEqual(matches, before.Matches) {
						t.Fatalf("retained pages mixed new state: %#v vs %#v", matches, before.Matches)
					}
					for _, match := range matches {
						if match.ElementID != mountEntity {
							continue
						}
						read, err := f.b.Connected().ResolvePath(context.Background(), f.callers(), connectedReadPath(first.ResolvedScopePath, match), &first.Cut)
						if err != nil || read.Record == nil || read.Record.Entity == nil || read.Record.Entity.Revision != match.Revision {
							t.Fatalf("old match exact read switched to latest: %#v, %v", read, err)
						}
					}
				}
				fresh := f.queryB(256)
				if reflect.DeepEqual(fresh.Matches, before.Matches) {
					t.Fatal("fresh query did not read committed entity revision")
				}
			})
		}
	}
}

func TestP3ConnectedModeIncludesNonOutputProofOwner(t *testing.T) {
	f := newConnectedFixture(t, false, true)
	query := f.queryFromB()
	query.UpDepth = 0
	first, err := f.b.Connected().QueryRegionPath(context.Background(), f.callers(), query, nil, 1, nil)
	if err != nil || first.ReadMode != corecontract.QueryCurrentVerified || first.Continuation == nil {
		t.Fatalf("non-output parent capability ignored: %#v, %v", first, err)
	}
	read, err := f.a.Elements().ReadElement(f.actorA, connectedRootEntity, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.a.Elements().UpdateEntity(f.actorA, connectedRootEntity, f.tuple(mountRoot, -3, 5), read.Record.Entity.Revision, "connected-change-proof-owner"); err != nil {
		t.Fatal(err)
	}
	page, err := f.b.Connected().QueryRegionPath(context.Background(), f.callers(), query, nil, 1, first.Continuation)
	assertConnectedQueryError(t, page, err, corecontract.ErrReadCutExpired)
}

func TestP3ConnectedCoordinatedRangeChangeKeepsMatchingHistoricalViews(t *testing.T) {
	for _, retained := range []bool{false, true} {
		t.Run(fmt.Sprintf("retained=%t", retained), func(t *testing.T) {
			f := newConnectedFixture(t, retained, retained)
			query := f.queryFromA()
			query.Sphere = corecontract.QuerySphere{Center: corecontract.Vec3{X: 90}}
			query.Selector = corecontract.QueryAnchors
			first, err := f.a.Connected().QueryRegionPath(context.Background(), f.callers(), query, nil, 1, nil)
			if err != nil || first.Continuation == nil || len(first.Matches) != 1 || first.Matches[0].ElementID != mountRoot {
				t.Fatalf("initial large M boundary: %#v, %v", first, err)
			}
			before := f.readB()
			changed, err := f.a.Mounts().ApplyMount(context.Background(), f.callers(), "connected-coordinated-shrink", f.rangeCommand(60))
			if err != nil || changed.Decision != corecontract.MountCommitted || !changed.Settled || !changed.Changed {
				t.Fatalf("legal P2 range update: %#v, %v", changed, err)
			}
			after := f.readB()
			if after.Range.Radius() != 60 || after.AdmissionKey == before.AdmissionKey || !after.WriteGateOpen {
				t.Fatalf("P2 did not replace carrier admission: %#v", after)
			}
			old, err := f.a.Connected().QueryRegionPath(context.Background(), f.callers(), query, nil, 1, first.Continuation)
			if !retained {
				assertConnectedQueryError(t, old, err, corecontract.ErrReadCutExpired)
			} else {
				if err != nil || old.Status != corecontract.QueryComplete || old.Continuation != nil ||
					len(old.Matches) != 1 || old.Matches[0].ElementID != mountAnchor || !old.Cut.Equal(first.Cut) {
					t.Fatalf("new admission pruned historical M hit: %#v, %v", old, err)
				}
				read, err := f.a.Connected().ResolvePath(context.Background(), f.callers(), connectedReadPath(first.ResolvedScopePath, old.Matches[0]), &first.Cut)
				if err != nil || read.Record == nil || read.Record.Anchor == nil || read.Record.Anchor.Revision != old.Matches[0].Revision {
					t.Fatalf("historical range read switched to new admission: %#v, %v", read, err)
				}
				if sphere, bounded := read.Record.Anchor.Range.BoundedSphere(); !bounded || sphere.Radius() != 100 {
					t.Fatalf("historical M did not retain its original range: %#v", read.Record.Anchor)
				}
			}
			fresh, err := f.a.Connected().QueryRegionPath(context.Background(), f.callers(), query, nil, 256, nil)
			if err != nil || fresh.Status != corecontract.QueryComplete || !reflect.DeepEqual(connectedIDs(fresh.Matches), []values.ElementID{mountRoot}) {
				t.Fatalf("new query still used superseded M range: %#v, %v", fresh, err)
			}
		})
	}
}

func TestP3ConnectedUnavailableParentCannotBecomeDepthBoundary(t *testing.T) {
	f := newConnectedFixture(t, true, true)
	if err := f.reads.UnbindEndpoint(mountAddressA); err != nil {
		t.Fatal(err)
	}
	page, err := f.b.Connected().QueryRegionPath(context.Background(), f.callers(), f.queryFromB(), nil, 256, nil)
	assertConnectedQueryError(t, page, err, corecontract.ErrUnavailable)
	f.reconnectReads()
	if page := f.queryB(256); page.Status != corecontract.QueryComplete {
		t.Fatalf("explicit reconnection did not restore new query: %#v", page)
	}
	if err := f.reads.BindEndpoint(mountAddressA, f.b.Connected().ConnectedPeer()); err != nil {
		t.Fatal(err)
	}
	page, err = f.b.Connected().QueryRegionPath(context.Background(), f.callers(), f.queryFromB(), nil, 256, nil)
	assertConnectedQueryError(t, page, err, corecontract.ErrUnavailable)
}

func TestP3ConnectedExcludedAndLostSourcesCannotReenterOldCut(t *testing.T) {
	for _, retained := range []bool{false, true} {
		t.Run(fmt.Sprintf("retained=%t", retained), func(t *testing.T) {
			f := newConnectedFixture(t, retained, retained)
			closed, err := f.a.Connected().QueryRegionPath(context.Background(), f.callers(), f.queryFromA(), nil, 1, nil)
			if err != nil || closed.Continuation == nil {
				t.Fatalf("first closed page: %#v, %v", closed, err)
			}
			if err := f.reads.UnbindEndpoint(mountAddressB); err != nil {
				t.Fatal(err)
			}
			terminal, err := f.a.Connected().QueryRegionPath(context.Background(), f.callers(), f.queryFromA(), nil, 256, closed.Continuation)
			if err != nil || terminal.Status != corecontract.QueryPartial || terminal.Gap&corecontract.QueryUnresolvedCoverage == 0 || terminal.Continuation != nil {
				t.Fatalf("lost source did not terminate original chain honestly: %#v, %v", terminal, err)
			}
			excluded, err := f.a.Connected().QueryRegionPath(context.Background(), f.callers(), f.queryFromA(), nil, 256, nil)
			if err != nil || excluded.Status != corecontract.QueryPartial || excluded.Gap&corecontract.QueryUnresolvedCoverage == 0 {
				t.Fatalf("offline source became empty: %#v, %v", excluded, err)
			}
			for _, match := range excluded.Matches {
				if match.Source.Authority != f.a.AuthorityRef() {
					t.Fatal("excluded source disclosed a match")
				}
			}
			f.reconnectReads()
			path := corecontract.Path{OriginAnchorID: mountRoot, Down: []values.ElementID{mountAnchor, mountBranch, mountEntity}}
			for _, cut := range []corecontract.ConnectedCut{closed.Cut, excluded.Cut} {
				read, err := f.a.Connected().ResolvePath(context.Background(), f.callers(), path, &cut)
				if !errors.Is(err, corecontract.ErrUnavailable) || !reflect.ValueOf(read).IsZero() {
					t.Fatalf("at added recovered source to old cut: %#v, %v", read, err)
				}
			}
			fresh, err := f.a.Connected().QueryRegionPath(context.Background(), f.callers(), f.queryFromA(), nil, 256, nil)
			if err != nil || fresh.Status != corecontract.QueryComplete || len(fresh.Matches) != 8 {
				t.Fatalf("new query did not recover complete sources: %#v, %v", fresh, err)
			}
		})
	}
}

func TestP3ConnectedRevocationAndContactChangeCloseOldAccess(t *testing.T) {
	t.Run("revocation before carrier notification", func(t *testing.T) {
		f := newConnectedFixture(t, true, true)
		first := f.queryB(1)
		if err := f.connections.UnbindEndpoint(mountAddressB); err != nil {
			t.Fatal(err)
		}
		result, err := f.a.Mounts().ApplyMount(context.Background(), f.callers(), "connected-revoke",
			corecontract.MountCommand{Kind: corecontract.MountRevoke, MountAnchorUUID: mountAnchor, ExpectedMountRevision: f.readA().MountRevision})
		if err != nil || result.Decision != corecontract.MountCommitted || result.Settled || f.readB().State != corecontract.MountActive {
			t.Fatalf("failed to establish delayed-notification case: %#v, %v", result, err)
		}
		page, err := f.b.Connected().QueryRegionPath(context.Background(), f.callers(), f.queryFromB(), nil, 1, first.Continuation)
		assertConnectedQueryError(t, page, err, corecontract.ErrUnavailable)
	})
	t.Run("specified address revision", func(t *testing.T) {
		f := newConnectedFixture(t, true, true)
		first := f.queryB(1)
		before := f.readA()
		const next values.NextHop = "local:connected-new-carrier"
		if _, err := f.a.Mounts().UpdateMountNextHop(f.actorA, "connected-address-change", mountAnchor, before.MountKey, before.NextHopRevision, next); err != nil {
			t.Fatal(err)
		}
		if err := f.reads.BindEndpoint(next, f.b.Connected().ConnectedPeer()); err != nil {
			t.Fatal(err)
		}
		page, err := f.b.Connected().QueryRegionPath(context.Background(), f.callers(), f.queryFromB(), nil, 1, first.Continuation)
		assertConnectedQueryError(t, page, err, corecontract.ErrReadCutExpired)
		if fresh := f.queryB(256); fresh.Status != corecontract.QueryComplete {
			t.Fatalf("new query did not use specified new address: %#v", fresh)
		}
	})
}

func TestP3ConnectedContextLifetimeBindingsAndReadRange(t *testing.T) {
	for _, retained := range []bool{false, true} {
		t.Run(fmt.Sprintf("retained=%t", retained), func(t *testing.T) {
			f := newConnectedFixture(t, retained, retained)
			first := f.queryB(1)
			changed := f.queryFromB()
			changed.UpDepth = 2 // Same effective root, different requested query.
			page, err := f.b.Connected().QueryRegionPath(context.Background(), f.callers(), changed, nil, 1, first.Continuation)
			assertConnectedQueryError(t, page, err, corecontract.ErrInvalid)
			if err := f.b.Connected().ReleaseConnectedCut(context.Background(), f.callers(), first.Cut); err != nil {
				t.Fatal(err)
			}
			if err := f.b.Connected().ReleaseConnectedCut(context.Background(), f.callers(), first.Cut); err != nil {
				t.Fatalf("release not idempotent: %v", err)
			}
			page, err = f.b.Connected().QueryRegionPath(context.Background(), f.callers(), f.queryFromB(), nil, 1, first.Continuation)
			assertConnectedQueryError(t, page, err, corecontract.ErrReadCutExpired)
			first = f.queryB(1)
			f.reopenConnected()
			page, err = f.b.Connected().QueryRegionPath(context.Background(), f.callers(), f.queryFromB(), nil, 1, first.Continuation)
			assertConnectedQueryError(t, page, err, corecontract.ErrReadCutExpired)
			q := f.queryFromB()
			q.UpDepth = 0
			narrow, err := f.b.Connected().QueryRegionPath(context.Background(), f.callers(), q, nil, 256, nil)
			if err != nil {
				t.Fatal(err)
			}
			read, err := f.b.Connected().ResolvePath(context.Background(), f.callers(), corecontract.Path{OriginAnchorID: mountBranch, Up: 1}, &narrow.Cut)
			if !errors.Is(err, corecontract.ErrInvalid) || !reflect.ValueOf(read).IsZero() {
				t.Fatalf("proof prefix expanded content range: %#v, %v", read, err)
			}
		})
	}
}

func TestP3ConnectedRemoteRestartExpiresCutWithoutRestartingEntry(t *testing.T) {
	for _, retained := range []bool{false, true} {
		t.Run(fmt.Sprintf("retained=%t", retained), func(t *testing.T) {
			f := newConnectedFixture(t, retained, retained)
			first := f.queryB(1)
			parent, entry := f.a.AuthorityRef(), f.b
			if err := f.a.Close(); err != nil {
				t.Fatal(err)
			}
			var err error
			f.a, err = core.OpenDurable(f.realmLinks, f.pathA, f.configA)
			if err != nil {
				t.Fatal(err)
			}
			if f.a.AuthorityRef() != parent || f.b != entry {
				t.Fatal("fixture changed persistent owner or restarted query entry")
			}
			f.connect()
			f.reconnectReads()
			page, err := f.b.Connected().QueryRegionPath(context.Background(), f.callers(), f.queryFromB(), nil, 1, first.Continuation)
			assertConnectedQueryError(t, page, err, corecontract.ErrReadCutExpired)
			if fresh := f.queryB(256); fresh.Status != corecontract.QueryComplete {
				t.Fatal("fresh query failed after explicit peer reconnection")
			}
		})
	}
}

func TestP3ConnectedPermissionsRemainCurrentDuringRetainedPagination(t *testing.T) {
	t.Run("necessary scope", func(t *testing.T) {
		f := newConnectedFixture(t, true, true)
		first := f.queryB(1)
		f.revokePermission(true, mountRoot, corecontract.PermissionQueryRegion)
		page, err := f.b.Connected().QueryRegionPath(context.Background(), f.callers(), f.queryFromB(), nil, 1, first.Continuation)
		assertConnectedQueryError(t, page, err, corecontract.ErrDenied)
	})
	t.Run("ordinary descendant", func(t *testing.T) {
		f := newConnectedFixture(t, true, true)
		first, err := f.a.Connected().QueryRegionPath(context.Background(), f.callers(), f.queryFromA(), nil, 1, nil)
		if err != nil || first.Continuation == nil {
			t.Fatalf("first page: %#v, %v", first, err)
		}
		f.revokePermission(false, mountAnchor, corecontract.PermissionQueryRegion)
		page, err := f.a.Connected().QueryRegionPath(context.Background(), f.callers(), f.queryFromA(), nil, 256, first.Continuation)
		if err != nil || page.Status != corecontract.QueryPartial || page.Gap&corecontract.QueryUnresolvedCoverage == 0 || page.Continuation != nil {
			t.Fatalf("revoked descendant grant yielded full old snapshot: %#v, %v", page, err)
		}
		for _, match := range page.Matches {
			if match.Source.Authority != f.a.AuthorityRef() {
				t.Fatal("revoked descendant disclosed a match")
			}
		}
	})
}
