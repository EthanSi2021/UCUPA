package connected_test

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"ucupa/shared/values"
	corecontract "ucupa/ucupa-core/contract"
)

func TestConnectedReviewResolveCutCannotProveAbsentSibling(t *testing.T) {
	c, query, ids := localFixture(t, corecontract.Config{})
	callers := corecontract.LocalCallers{c.AuthorityRef(): localActor}
	firstPath := corecontract.Path{OriginAnchorID: query.StartAnchorID, Down: []values.ElementID{ids[0]}}
	first, err := c.Connected().ResolvePath(context.Background(), callers, firstPath, nil)
	if err != nil || first.State != corecontract.ExistencePresent {
		t.Fatalf("initial Resolve: %#v %v", first, err)
	}
	t.Cleanup(func() { _ = c.Connected().ReleaseConnectedCut(context.Background(), callers, first.Cut) })
	siblingPath := corecontract.Path{OriginAnchorID: query.StartAnchorID, Down: []values.ElementID{ids[1]}}
	read, err := c.Connected().ResolvePath(context.Background(), callers, siblingPath, &first.Cut)
	if !errors.Is(err, corecontract.ErrInvalid) || !reflect.DeepEqual(read, corecontract.PathRead{}) {
		t.Fatalf("outside original Resolve range reported facts: %#v %v", read, err)
	}
}

func TestConnectedReviewListCutCannotProveAbsentGrandchild(t *testing.T) {
	c, query, _ := localFixture(t, corecontract.Config{})
	anchor := localCreateAnchor(t, c, localID(96000), localTuple(t, query.StartAnchorID, 0, 0, 0, 5), "review-list-anchor")
	grandchild := localCreateEntity(t, c, localID(96001), localTuple(t, anchor, 0, 0, 0, 0), "review-list-grandchild")
	callers := corecontract.LocalCallers{c.AuthorityRef(): localActor}
	first, err := c.Connected().ListDirectPath(context.Background(), callers, corecontract.Path{OriginAnchorID: query.StartAnchorID}, nil, 256, nil)
	if err != nil || first.Status != corecontract.QueryComplete {
		t.Fatalf("initial List: %#v %v", first, err)
	}
	t.Cleanup(func() { _ = c.Connected().ReleaseConnectedCut(context.Background(), callers, first.Cut) })
	path := corecontract.Path{OriginAnchorID: query.StartAnchorID, Down: []values.ElementID{anchor, grandchild}}
	read, err := c.Connected().ResolvePath(context.Background(), callers, path, &first.Cut)
	if !errors.Is(err, corecontract.ErrInvalid) || !reflect.DeepEqual(read, corecontract.PathRead{}) {
		t.Fatalf("outside original List range reported facts: %#v %v", read, err)
	}
}

func TestConnectedReviewQueryScopeNeedsNoExactReadButFollowupDoes(t *testing.T) {
	c, query, ids := localFixture(t, corecontract.Config{})
	callers := corecontract.LocalCallers{c.AuthorityRef(): localActor}
	c.policy.set(authorizeFunc(func(actor values.PrincipalRef, requirements ...values.Requirement) bool {
		for _, r := range requirements {
			if r.Permission != corecontract.PermissionQueryRegion {
				return false
			}
		}
		return true
	}))
	page, err := c.Connected().QueryRegionPath(context.Background(), callers, connectedReviewQuery(query), nil, 256, nil)
	if err != nil || page.Status != corecontract.QueryComplete || len(page.Matches) != len(ids) {
		t.Fatalf("query demanded full-record read: %#v %v", page, err)
	}
	t.Cleanup(func() { _ = c.Connected().ReleaseConnectedCut(context.Background(), callers, page.Cut) })
	read, err := c.Connected().ResolvePath(context.Background(), callers, corecontract.Path{OriginAnchorID: query.StartAnchorID, Down: []values.ElementID{ids[0]}}, &page.Cut)
	if !errors.Is(err, corecontract.ErrDenied) || !reflect.DeepEqual(read, corecontract.PathRead{}) {
		t.Fatalf("query grant supplied full record: %#v %v", read, err)
	}
}

func TestConnectedReviewQueryFailureUsesQueryDiagnosticGrant(t *testing.T) {
	c, query, _ := localFixture(t, corecontract.Config{})
	callers := corecontract.LocalCallers{c.AuthorityRef(): localActor}
	query.StartAnchorID, query.BasisAnchorID = localID(96888), localID(96888)
	exactChecks := 0
	c.policy.set(authorizeFunc(func(actor values.PrincipalRef, requirements ...values.Requirement) bool {
		for _, r := range requirements {
			if r.Permission == corecontract.PermissionReadElement {
				exactChecks++
				return false
			}
			if r.Permission != corecontract.PermissionQueryRegion {
				return false
			}
		}
		return true
	}))
	page, err := c.Connected().QueryRegionPath(context.Background(), callers, connectedReviewQuery(query), nil, 256, nil)
	if !errors.Is(err, corecontract.ErrNotFound) || exactChecks != 0 || !reflect.DeepEqual(page, corecontract.ConnectedQueryPage{}) {
		t.Fatalf("query diagnosis required full read: exact=%d result=%#v err=%v", exactChecks, page, err)
	}
}

func TestConnectedReviewMissingStartDiagnosisCannotOutliveItsCut(t *testing.T) {
	c, query, _ := localFixture(t, corecontract.Config{})
	root, missing := query.StartAnchorID, localID(96889)
	query.StartAnchorID, query.BasisAnchorID = missing, missing
	checks := 0
	c.policy.set(authorizeFunc(func(actor values.PrincipalRef, requirements ...values.Requirement) bool {
		for _, r := range requirements {
			if r.Permission == corecontract.PermissionQueryRegion {
				checks++
				if checks == 2 {
					localCreateAnchor(t, c, missing, localTuple(t, root, 0, 0, 0, 5), "review-create-missing-during-diagnosis")
				}
				break
			}
		}
		return true
	}))
	page, err := c.Connected().QueryRegionPath(context.Background(), corecontract.LocalCallers{c.AuthorityRef(): localActor}, connectedReviewQuery(query), nil, 256, nil)
	if !errors.Is(err, corecontract.ErrReadCutExpired) || !reflect.DeepEqual(page, corecontract.ConnectedQueryPage{}) {
		t.Fatalf("state diagnosis outlived its inspected cut: checks=%d result=%#v err=%v", checks, page, err)
	}
}

func connectedReviewQuery(query corecontract.RegionQuery) corecontract.PathRegionQuery {
	return corecontract.PathRegionQuery{StartPath: corecontract.Path{OriginAnchorID: query.StartAnchorID}, BasisPath: corecontract.Path{OriginAnchorID: query.BasisAnchorID}, Sphere: query.Sphere, Selector: query.Selector}
}
