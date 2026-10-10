package connected_test

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"ucupa/shared/values"
	corecontract "ucupa/ucupa-core/contract"
)

func TestConnectedCoreTwoModesAndReadback(t *testing.T) {
	for _, retained := range []bool{false, true} {
		t.Run(map[bool]string{false: "current", true: "retained"}[retained], func(t *testing.T) {
			f, branch, callers, _, _ := newPairFixture(t, retained)
			path := corecontract.Path{OriginAnchorID: branch}
			query := corecontract.PathRegionQuery{StartPath: path, UpDepth: -1, BasisPath: path, Sphere: corecontract.QuerySphere{Radius: 100}, Selector: corecontract.QueryBoth}
			page, err := f.carrier.Connected().QueryRegionPath(context.Background(), callers, query, nil, 256, nil)
			if err != nil {
				t.Fatal(err)
			}
			if len(page.Matches) != 4 || page.ActualUpDepth != 2 || page.ResolvedScopePath.Up != 2 || page.Status != corecontract.QueryComplete {
				t.Fatalf("unexpected page: %#v", page)
			}
			for _, match := range page.Matches {
				readPath := pairClonePath(page.ResolvedScopePath)
				readPath.Down = append(readPath.Down, match.Path...)
				read, err := f.carrier.Connected().ResolvePath(context.Background(), callers, readPath, &page.Cut)
				if err != nil || read.Record == nil || pairRecordID(*read.Record) != match.ElementID || read.Source != match.Source {
					t.Fatalf("readback %s: %#v %v", match.ElementID, read, err)
				}
			}
		})
	}
}

func TestConnectedMountOriginReadUsesOnlyItsOwner(t *testing.T) {
	for _, retained := range []bool{false, true} {
		t.Run(map[bool]string{false: "current", true: "retained"}[retained], func(t *testing.T) {
			f, _, callers, _, _ := newPairFixture(t, retained)
			path := corecontract.Path{OriginAnchorID: f.mount}
			query := corecontract.PathRegionQuery{StartPath: path, BasisPath: path, Sphere: corecontract.QuerySphere{Radius: 100}, Selector: corecontract.QueryBoth}
			page, err := f.parent.Connected().QueryRegionPath(context.Background(), callers, query, nil, 256, nil)
			if err != nil {
				t.Fatal(err)
			}
			f.carrier.policy.set(func(values.PrincipalRef, ...values.Requirement) bool { return false })
			read, err := f.parent.Connected().ResolvePath(context.Background(), callers, path, &page.Cut)
			if err != nil || read.Record == nil || read.Record.Anchor == nil || pairRecordID(*read.Record) != f.mount || read.Source.Authority != f.parent.AuthorityRef() {
				t.Fatalf("M-origin exact read requested carrier content: %#v %v", read, err)
			}
		})
	}
}

func TestConnectedSmallerQueryUsesOnlyItsContentGrants(t *testing.T) {
	for _, retained := range []bool{false, true} {
		f, branch, callers, _, _ := newPairFixture(t, retained)
		sibling := pairID(9130)
		if _, err := f.parent.Elements().CreateAnchor(pairActor, sibling, pairTuple(t, f.root, 20, 5), "connected-sibling"); err != nil {
			t.Fatal(err)
		}
		root := corecontract.Path{OriginAnchorID: f.root}
		query := corecontract.PathRegionQuery{StartPath: root, BasisPath: root, Sphere: corecontract.QuerySphere{Radius: 100}, Selector: corecontract.QueryBoth}
		first, err := f.parent.Connected().QueryRegionPath(context.Background(), callers, query, nil, 256, nil)
		if err != nil {
			t.Fatal(err)
		}
		f.carrier.policy.set(func(values.PrincipalRef, ...values.Requirement) bool { return false })
		path := corecontract.Path{OriginAnchorID: f.root, Down: []values.ElementID{sibling}}
		small := corecontract.PathRegionQuery{StartPath: path, BasisPath: path, Sphere: corecontract.QuerySphere{Radius: 1}, Selector: corecontract.QueryBoth}
		page, err := f.parent.Connected().QueryRegionPath(context.Background(), callers, small, &first.Cut, 256, nil)
		if err != nil || page.Status != corecontract.QueryComplete || page.Gap != corecontract.QueryNoGap || len(page.Matches) != 1 {
			t.Fatalf("out-of-Scope source became a gap: %#v %v", page, err)
		}
		f.carrier.policy.set(nil)
		start := corecontract.Path{OriginAnchorID: branch}
		broad := corecontract.PathRegionQuery{StartPath: start, UpDepth: -1, BasisPath: start, Sphere: corecontract.QuerySphere{Radius: 100}, Selector: corecontract.QueryBoth}
		fromB, err := f.carrier.Connected().QueryRegionPath(context.Background(), callers, broad, nil, 256, nil)
		if err != nil {
			t.Fatal(err)
		}
		f.parent.policy.set(func(_ values.PrincipalRef, requirements ...values.Requirement) bool {
			for _, requirement := range requirements {
				if requirement.Permission == corecontract.PermissionQueryRegion {
					return false
				}
			}
			return true
		})
		small = corecontract.PathRegionQuery{StartPath: start, BasisPath: start, Sphere: corecontract.QuerySphere{Radius: 100}, Selector: corecontract.QueryBoth}
		page, err = f.carrier.Connected().QueryRegionPath(context.Background(), callers, small, &fromB.Cut, 256, nil)
		if err != nil || page.Status != corecontract.QueryComplete || len(page.Matches) != 2 {
			t.Fatalf("proof owner demanded old broad query: %#v %v", page, err)
		}
	}
}

func TestConnectedReviewRemoteKindDiagnosisCannotOutliveItsCut(t *testing.T) {
	f, branch, callers, _, _ := newPairFixture(t, true)
	id := pairID(9121)
	before, err := f.carrier.Elements().ReadElement(pairActor, id, nil)
	if err != nil {
		t.Fatal(err)
	}
	checks := 0
	f.carrier.policy.set(func(_ values.PrincipalRef, requirements ...values.Requirement) bool {
		for _, r := range requirements {
			if r.Permission == corecontract.PermissionQueryRegion {
				checks++
				if checks == 2 {
					if _, err := f.carrier.Elements().DeleteEntity(pairActor, id, before.Record.Entity.Revision, "review-delete-during-remote-diagnosis"); err != nil {
						t.Fatal(err)
					}
				}
				break
			}
		}
		return true
	})
	path := corecontract.Path{OriginAnchorID: f.root, Down: []values.ElementID{f.mount, branch, id}}
	query := corecontract.PathRegionQuery{StartPath: path, BasisPath: path, Sphere: corecontract.QuerySphere{Radius: 1}, Selector: corecontract.QueryBoth}
	page, err := f.parent.Connected().QueryRegionPath(context.Background(), callers, query, nil, 256, nil)
	if !errors.Is(err, corecontract.ErrReadCutExpired) || !reflect.DeepEqual(page, corecontract.ConnectedQueryPage{}) {
		t.Fatalf("remote kind diagnosis outlived its cut: checks=%d result=%#v err=%v", checks, page, err)
	}
}

func TestConnectedReviewOriginalQueryRevocationDoesNotBlockAuthorizedExactRead(t *testing.T) {
	for _, retained := range []bool{false, true} {
		t.Run(map[bool]string{false: "current", true: "retained"}[retained], func(t *testing.T) {
			f, branch, callers, _, _ := newPairFixture(t, retained)
			query := pairRootQuery(f.root)
			page, err := f.parent.Connected().QueryRegionPath(context.Background(), callers, query, nil, 256, nil)
			if err != nil {
				t.Fatal(err)
			}
			withoutQuery := authorizeFunc(func(_ values.PrincipalRef, requirements ...values.Requirement) bool {
				for _, r := range requirements {
					if r.Permission == corecontract.PermissionQueryRegion {
						return false
					}
				}
				return true
			})
			f.parent.policy.set(withoutQuery)
			f.carrier.policy.set(withoutQuery)
			path := corecontract.Path{OriginAnchorID: f.root, Down: []values.ElementID{f.mount, branch, pairID(9121)}}
			read, err := f.parent.Connected().ResolvePath(context.Background(), callers, path, &page.Cut)
			if err != nil || read.State != corecontract.ExistencePresent || read.Record == nil || pairRecordID(*read.Record) != pairID(9121) {
				t.Fatalf("old query grant remained a full-read prerequisite: %#v %v", read, err)
			}
		})
	}
}

func TestConnectedReviewVerifyOnlyCannotBypassReceiverHandleBinding(t *testing.T) {
	for _, retained := range []bool{false, true} {
		t.Run(map[bool]string{false: "current", true: "retained"}[retained], func(t *testing.T) {
			f, _, callers, base, _ := newPairFixture(t, retained)
			page, err := f.parent.Connected().QueryRegionPath(context.Background(), callers, pairRootQuery(f.root), nil, 256, nil)
			if err != nil {
				t.Fatal(err)
			}
			requests := 0
			base.after = func() { requests++ }
			transport := base
			if err := f.parent.Connected().ConfigureConnectedTransport(transport); err != nil {
				t.Fatal(err)
			}
			wrong := corecontract.LocalCallers{f.parent.AuthorityRef(): pairActor, f.carrier.AuthorityRef(): {Realm: "review", Principal: "different"}}
			read, err := f.parent.Connected().ResolvePath(context.Background(), wrong, corecontract.Path{OriginAnchorID: f.root}, &page.Cut)
			if !errors.Is(err, corecontract.ErrInvalid) || requests != 0 || !reflect.DeepEqual(read, corecontract.PathRead{}) {
				t.Fatalf("wrong bound caller reached no-content peer check: calls=%d read=%#v err=%v", requests, read, err)
			}
			if err := f.parent.Connected().ReleaseConnectedCut(context.Background(), callers, page.Cut); err != nil {
				t.Fatal(err)
			}
			requests = 0
			read, err = f.parent.Connected().ResolvePath(context.Background(), callers, corecontract.Path{OriginAnchorID: f.root}, &page.Cut)
			if !errors.Is(err, corecontract.ErrReadCutExpired) || requests != 0 || !reflect.DeepEqual(read, corecontract.PathRead{}) {
				t.Fatalf("released handle reached no-content peer check: calls=%d read=%#v err=%v", requests, read, err)
			}
		})
	}
}

func TestConnectedReviewExternalMountReadDoesNotInventCarrierPermission(t *testing.T) {
	for _, retained := range []bool{false, true} {
		t.Run(map[bool]string{false: "current", true: "retained"}[retained], func(t *testing.T) {
			f, _, callers, _, _ := newPairFixture(t, retained)
			path := corecontract.Path{OriginAnchorID: f.mount}
			query := pairRootQuery(f.mount)
			initial, err := f.carrier.Connected().QueryRegionPath(context.Background(), callers, query, nil, 256, nil)
			if err != nil {
				t.Fatal(err)
			}
			carrierChecks := 0
			f.carrier.policy.set(func(_ values.PrincipalRef, requirements ...values.Requirement) bool {
				carrierChecks += len(requirements)
				return false
			})
			for _, at := range []*corecontract.ConnectedCut{nil, &initial.Cut} {
				read, err := f.carrier.Connected().ResolvePath(context.Background(), callers, path, at)
				if err != nil || read.State != corecontract.ExistencePresent || read.Source.Authority != f.parent.AuthorityRef() ||
					read.Record == nil || pairRecordID(*read.Record) != f.mount {
					t.Fatalf("B entry invented a content permission for A-owned M: old-cut=%t checks=%d result=%#v err=%v", at != nil, carrierChecks, read, err)
				}
			}
			if carrierChecks != 0 {
				t.Fatalf("no-content carrier received %d permission requirements", carrierChecks)
			}
		})
	}
}
