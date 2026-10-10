package projection

import (
	"context"
	"fmt"
	"reflect"
	"testing"

	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	elementfact "ucupa/ucupa-core/internal/element/fact"
	facetfact "ucupa/ucupa-core/internal/facet/fact"
	"ucupa/ucupa-core/internal/geometry"
	mountfact "ucupa/ucupa-core/internal/mount/fact"
	federationvalue "ucupa/ucupa-core/internal/query/federation/value"
	"ucupa/ucupa-core/internal/token"
	transferfact "ucupa/ucupa-core/internal/transfer/fact"
)

func projectionID(n uint64) values.ElementID {
	return values.ElementID(fmt.Sprintf("10000000-0000-4000-8000-%012x", n))
}

func projectionFixture(t *testing.T) (*Projection, values.ElementID, values.ElementID, values.ElementID) {
	t.Helper()
	root, anchor, entity := projectionID(1), projectionID(2), projectionID(3)
	scope := [32]byte{1}
	sphere, err := contract.NewBoundedSphere(10)
	if err != nil {
		t.Fatal(err)
	}
	build := elementfact.NewProjection()
	build.AddAnchor(root, elementfact.Anchor{ID: root, Range: geometry.UnboundedRootRange(), Revision: token.RevisionFromParts(scope, root, 1)})
	build.AddAnchor(anchor, elementfact.Anchor{ID: anchor, Parent: root, LocalPose: geometry.IdentityPose(), Range: geometry.BoundedRange(sphere), Revision: token.RevisionFromParts(scope, anchor, 2)})
	build.AddEntity(entity, elementfact.Entity{ID: entity, Parent: anchor, LocalPose: geometry.IdentityPose(), Range: sphere, Revision: token.RevisionFromParts(scope, entity, 3)})
	attachments := facetfact.New()
	for _, id := range []values.ElementID{root, anchor, entity} {
		attachments.ElementCreated(id, token.FromParts(scope, 3))
	}
	return New(build.Build().View, mountfact.New().View, attachments.View, transferfact.Visibility{}, token.NewPublicSigner([32]byte{9}, "owner")), root, anchor, entity
}

func TestProjectReadListPathAndErrors(t *testing.T) {
	s, root, anchor, entity := projectionFixture(t)
	for _, kind := range []string{"read", "read-at"} {
		page, last, deps, err := s.Project(context.Background(), root, federationvalue.FederationProjectionRequest{Kind: kind, Element: entity}, "", 1)
		if err != nil || !page.Complete || last != "" || len(deps) != 0 || len(page.Records) != 1 {
			t.Fatalf("%s: %#v %q %v %v", kind, page, last, deps, err)
		}
		row := page.Records[0]
		if row.ID != entity || row.Kind != contract.ElementEntity || row.Parent != anchor || row.Pose == nil || row.Attachment == nil || !reflect.DeepEqual(row.Path, []values.ElementID{anchor, entity}) {
			t.Fatalf("exact record mismatch: %#v", row)
		}
	}
	page, _, _, err := s.Project(context.Background(), root, federationvalue.FederationProjectionRequest{Kind: "list", Element: root}, "", 1)
	if err != nil || !page.Complete || len(page.Records) != 1 || page.Records[0].ID != anchor || page.Records[0].Attachment != nil {
		t.Fatal("list disclosed wrong record or attachment", page, err)
	}
	path := federationvalue.FederationPathRequest{Origin: anchor, Up: 1, Down: []values.ElementID{anchor, entity}}
	page, _, _, err = s.Project(context.Background(), root, federationvalue.FederationProjectionRequest{Kind: "path", Path: &path}, "", 1)
	if err != nil || page.Path == nil || page.Path.Element != entity || page.Path.ActualUp != 1 || page.Path.RemainingUp != 0 || len(page.Path.RemainingDown) != 0 || len(page.Path.Spine) != 3 {
		t.Fatal("up/down path mismatch", page, err)
	}
	for _, tc := range []struct {
		request federationvalue.FederationProjectionRequest
		after   values.ElementID
		want    error
	}{
		{federationvalue.FederationProjectionRequest{Kind: "unknown"}, "", values.PublicInvalid},
		{federationvalue.FederationProjectionRequest{Kind: "path", Path: &path}, entity, values.PublicInvalidContext},
		{federationvalue.FederationProjectionRequest{Kind: "read", Element: entity}, entity, values.PublicInvalid},
		{federationvalue.FederationProjectionRequest{Kind: "read", Element: projectionID(999)}, "", values.PublicInvalid},
		{federationvalue.FederationProjectionRequest{Kind: "list", Element: entity}, "", values.PublicInvalid},
	} {
		if _, _, _, err := s.Project(context.Background(), root, tc.request, tc.after, 1); err != tc.want {
			t.Fatalf("%s: %v != %v", tc.request.Kind, err, tc.want)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, _, err := s.Project(ctx, root, federationvalue.FederationProjectionRequest{}, "", 1); err != values.PublicUnavailable {
		t.Fatal("cancellation priority changed", err)
	}
}

func TestRecordShapeBudgetAndMissingAttachment(t *testing.T) {
	s, root, anchor, entity := projectionFixture(t)
	for _, id := range []values.ElementID{root, anchor, entity} {
		row, cost, err := s.record(id, root, true, 128)
		if err != nil || row.ID != id || row.Attachment == nil || (row.Pose == nil) != (id == root) || cost != uint64(len(row.Path))*64 {
			t.Fatalf("record: %#v cost=%d err=%v", row, cost, err)
		}
	}
	if _, _, err := s.record(entity, root, false, 127); err != values.PublicResourceExhausted {
		t.Fatal("record depth budget widened", err)
	}
	without := New(s.elements, s.mount, facetfact.New().View, s.transfers, s.signer)
	if _, _, err := without.record(entity, root, true, 128); err != values.PublicUnavailable {
		t.Fatal("missing attachment accepted", err)
	}
	if row, _, err := without.record(entity, root, false, 128); err != nil || row.Attachment != nil {
		t.Fatal("list started depending on attachment", err)
	}
	bad := facetfact.NewProjectionBuilder()
	bad.Add(entity, facetfact.Record{})
	without = New(s.elements, s.mount, bad.Build().View, s.transfers, s.signer)
	if _, _, err := without.record(entity, root, true, 128); err != values.PublicUnavailable {
		t.Fatal("zero attachment revision accepted", err)
	}
	if _, _, err := s.record(projectionID(999), projectionID(999), false, 128); err != values.PublicNotFound {
		t.Fatal("absent record boundary changed", err)
	}
}

func TestPathIDsPreserves4096EdgeLimitAndMissingPath(t *testing.T) {
	root := projectionID(10000)
	build := elementfact.NewProjection()
	build.AddAnchor(root, elementfact.Anchor{ID: root})
	for n := uint64(1); n <= geometry.MaxExactProjectionEdges+1; n++ {
		id := projectionID(10000 + n)
		build.AddAnchor(id, elementfact.Anchor{ID: id, Parent: projectionID(9999 + n)})
	}
	s := &Projection{elements: build.Build().View}
	if path, err := s.PathIDs(root, root, 0); err != nil || len(path) != 0 {
		t.Fatal("boundary path", path, err)
	}
	if path, err := s.PathIDs(projectionID(10000+geometry.MaxExactProjectionEdges), root, geometry.MaxExactProjectionEdges); err != nil || len(path) != geometry.MaxExactProjectionEdges {
		t.Fatal("4096 boundary rejected", len(path), err)
	}
	if _, err := s.PathIDs(projectionID(10001+geometry.MaxExactProjectionEdges), root, geometry.MaxExactProjectionEdges); err != values.PublicResourceExhausted {
		t.Fatal("4097 edges accepted", err)
	}
	if _, err := s.PathIDs(projectionID(99999), root, geometry.MaxExactProjectionEdges); err != values.PublicUnavailable {
		t.Fatal("missing path accepted", err)
	}
	cycle := elementfact.NewProjection()
	id := projectionID(42)
	cycle.AddAnchor(id, elementfact.Anchor{ID: id, Parent: id})
	s.elements = cycle.Build().View
	if _, err := s.PathIDs(id, root, geometry.MaxExactProjectionEdges); err != values.PublicResourceExhausted {
		t.Fatal("cycle not bounded", err)
	}
}
