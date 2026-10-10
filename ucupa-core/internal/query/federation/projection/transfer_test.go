package projection

import (
	"context"
	"testing"

	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	mountfact "ucupa/ucupa-core/internal/mount/fact"
	federationvalue "ucupa/ucupa-core/internal/query/federation/value"
	transferfact "ucupa/ucupa-core/internal/transfer/fact"
)

func TestTransferProjectionDoesNotDiscloseOutsideClosureOrLoseListGap(t *testing.T) {
	f := newQueryFixture(t)
	f.anchor(t, f.root, "", 0, 0)
	left, departed := queryID(2), queryID(3)
	f.anchor(t, left, f.root, 0, 10)
	s := f.projection()
	s.transfers = transferfact.NewVisibility(map[values.ElementID]values.ElementID{departed: departed}, []transferfact.SourceFootprint{{
		Boundary: departed, Origin: contract.SpatialTuple{Parent: f.root, LocalPose: queryPose(t, 20), Range: querySphere(t, 1)},
	}}, nil, nil, nil)
	ctx := context.Background()
	for _, request := range []federationvalue.FederationProjectionRequest{
		{Kind: "read", Element: departed},
		{Kind: "read-at", Element: departed},
		{Kind: "path", Path: &federationvalue.FederationPathRequest{Origin: departed}},
		queryRequest(departed, 1),
	} {
		if _, _, _, err := s.Project(ctx, left, request, "", 10); err != values.PublicInvalid {
			t.Fatalf("outside closure revealed departure for %s: %v", request.Kind, err)
		}
	}
	request := federationvalue.FederationProjectionRequest{Kind: "path", Path: &federationvalue.FederationPathRequest{Origin: f.root, Down: []values.ElementID{departed}}}
	if _, _, _, err := s.Project(ctx, f.root, request, "", 10); err != values.PublicUnavailable {
		t.Fatalf("verified departed edge became absence: %v", err)
	}
	page, last, _, err := s.Project(ctx, f.root, federationvalue.FederationProjectionRequest{Kind: "list", Element: f.root}, "", 10)
	if err != nil || page.Complete || page.Gap != contract.QueryUnresolvedCoverage || last != "" || len(page.Records) != 1 {
		t.Fatalf("direct gap without fake continuation: %#v %q %v", page, last, err)
	}
	page, _, _, err = s.Project(ctx, left, queryRequest(left, 100), "", 10)
	if err != nil || !page.Complete || page.Gap != contract.QueryNoGap {
		t.Fatalf("unrelated subtree affected: %#v %v", page, err)
	}
}

func TestHeldParentFrontierUsesExactRelationForPathListAndQuery(t *testing.T) {
	f := newQueryFixture(t)
	f.anchor(t, f.root, "", 0, 0)
	id := queryID(2)
	f.anchor(t, id, f.root, 0, 10)
	r := f.mountRecord(t, id, contract.MountActive, "transfer-parent:held")
	f.mounts.ReplaceOutgoing(id, r)
	s := f.projection()
	s.transfers = transferfact.NewVisibility(nil, nil, map[values.ElementID]mountfact.Record{id: r}, nil, nil)
	for _, request := range []federationvalue.FederationProjectionRequest{
		{Kind: "path", Path: &federationvalue.FederationPathRequest{Origin: f.root, Down: []values.ElementID{id, queryID(3)}}},
		{Kind: "list", Element: id}, queryRequest(f.root, 100),
	} {
		page, _, deps, err := s.Project(context.Background(), f.root, request, "", 10)
		if err != nil || deps[id] != r {
			t.Fatal("held relation lost", request.Kind, page, err)
		}
		if request.Kind == "path" {
			if page.Path.Frontier == nil || !page.Path.Frontier.Relation.WriteGateOpen {
				t.Fatal("path closed", page)
			}
		} else if len(page.Frontiers) != 1 || !page.Frontiers[0].Relation.WriteGateOpen {
			t.Fatal("frontier closed", page)
		}
	}
	// An ordinary slot, or a changed contact at the same mount, must not inherit
	// the held relation's read eligibility. This does not open a mutation gate.
	changed := r
	changed.Contact.NextHop = "local:new"
	f.mounts.ReplaceOutgoing(id, changed)
	s.mount = f.mounts.View
	if _, _, _, err := s.Project(context.Background(), f.root, federationvalue.FederationProjectionRequest{Kind: "list", Element: id}, "", 10); err != values.PublicUnavailable {
		t.Fatal("stale held relation followed", err)
	}
}

func TestInstalledPendingBranchIsNotAResultOrRemoteFrontier(t *testing.T) {
	f := newQueryFixture(t)
	f.anchor(t, f.root, "", 0, 0)
	pending, visible := queryID(2), queryID(3)
	f.anchor(t, pending, f.root, 0, 10)
	f.anchor(t, visible, f.root, 30, 1)
	s := f.projection()
	mounts := mountfact.New()
	mounts.ReplaceOutgoing(pending, mountfact.Record{State: contract.MountActive})
	s.mount = mounts.View
	s.transfers = transferfact.NewVisibility(map[values.ElementID]values.ElementID{pending: pending}, []transferfact.SourceFootprint{{
		Boundary: pending, Origin: contract.SpatialTuple{Parent: f.root, LocalPose: queryPose(t, 0), Range: querySphere(t, 10)},
	}}, nil, nil, nil)
	ctx := context.Background()
	for _, request := range []federationvalue.FederationProjectionRequest{
		{Kind: "read", Element: pending}, {Kind: "read-at", Element: pending},
		{Kind: "path", Path: &federationvalue.FederationPathRequest{Origin: f.root, Down: []values.ElementID{pending}}},
	} {
		if _, _, _, err := s.Project(ctx, f.root, request, "", 10); err != values.PublicUnavailable {
			t.Fatal("pending target disclosed", request.Kind, err)
		}
	}
	query := queryRequest(f.root, 100)
	query.Query.Selector = contract.QueryBoth
	page, last, deps, err := s.Project(ctx, f.root, query, "", 10)
	if err != nil || page.Complete || page.Gap != contract.QueryUnresolvedCoverage || len(page.Frontiers) != 0 || len(deps) != 0 || last != "" {
		t.Fatal("pending query followed target or lost gap", page, err)
	}
	for _, match := range page.Matches {
		if match.ID == pending {
			t.Fatal("pending target in matches")
		}
	}
	page, last, _, err = s.Project(ctx, f.root, federationvalue.FederationProjectionRequest{Kind: "list", Element: f.root}, "", 1)
	if err != nil || len(page.Records) != 1 || page.Complete || page.Gap != contract.QueryUnresolvedCoverage || last != "" {
		t.Fatal("pending target consumed list position", page, err)
	}
	page, _, _, err = s.Project(ctx, visible, queryRequest(visible, 0), "", 10)
	if err != nil || !page.Complete || page.Gap != contract.QueryNoGap {
		t.Fatal("unrelated branch blocked", page, err)
	}
}
