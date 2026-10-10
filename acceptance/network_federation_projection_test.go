package acceptance_test

import (
	federationcontract "ucupa/ucupa-core/contract/federation"

	"context"
	"errors"
	"testing"

	"google.golang.org/protobuf/proto"

	netpb "ucupa/shared/bindings/go/protobuf"
	"ucupa/shared/values"

	pb "ucupa/ucupa-core/bindings/go/protobuf"
	corecontract "ucupa/ucupa-core/contract"
)

func TestNetworkFederationProjectionQueryOnlyAndPath(t *testing.T) {
	f := newFederationNetworkFixture(t, false)
	f.request.Intent = "query"
	f.grant(t, corecontract.PermissionQueryRegion, false, "only-query")
	part, err := f.open()
	if err != nil {
		t.Fatal(err)
	}
	defer part.Release(context.Background())
	q := federationcontract.FederationProjectionRequest{Kind: "query", Query: &federationcontract.FederationQueryRequest{Scope: networkRoot, Sphere: corecontract.QuerySphere{Radius: 1}, Selector: corecontract.QueryEntities}}
	first, err := part.Project(context.Background(), q, nil, 1)
	if err != nil || first.Complete || first.Continuation == nil || len(first.Matches) != 1 || len(first.Records) != 0 || first.Matches[0].ID != networkEntity {
		t.Fatalf("query-only projection: %+v %v", first, err)
	}
	second, err := part.Project(context.Background(), q, first.Continuation, 1)
	if err != nil || !second.Complete || len(second.Matches) != 1 || second.Matches[0].ID == first.Matches[0].ID {
		t.Fatalf("query continuation: %+v %v", second, err)
	}
	changed := *q.Query
	if first.Coverage == ([32]byte{}) || second.Coverage != first.Coverage {
		t.Fatal("network scan evidence missing or changed across pages")
	}
	changed.Sphere.Radius = 2
	if _, err := part.Project(context.Background(), federationcontract.FederationProjectionRequest{Kind: "query", Query: &changed}, first.Continuation, 1); !errors.Is(err, values.PublicInvalidContext) {
		t.Fatalf("changed query reused cursor: %v", err)
	}
	path, err := part.Project(context.Background(), federationcontract.FederationProjectionRequest{Kind: "path", Path: &federationcontract.FederationPathRequest{Origin: networkRoot, Down: []values.ElementID{networkEntity}}}, nil, 1)
	if err != nil || path.Path == nil || path.Path.Element != networkEntity || len(path.Path.Spine) != 2 || len(path.Records) != 0 {
		t.Fatalf("query path recipe: %+v %v", path, err)
	}
	for _, kind := range []string{"read", "read-at", "list"} {
		if _, err := part.Project(context.Background(), federationcontract.FederationProjectionRequest{Kind: kind, Element: networkRoot}, nil, 1); !errors.Is(err, values.PublicDenied) {
			t.Fatalf("query used %s permission: %v", kind, err)
		}
	}
}

func TestNetworkFederationProjectionRetainedNeedsInitialSeal(t *testing.T) {
	f := newFederationNetworkFixture(t, true)
	f.grant(t, corecontract.PermissionReadElement, false, "read")
	part, err := f.open()
	if err != nil {
		t.Fatal(err)
	}
	defer part.Release(context.Background())
	if err := part.Check(context.Background(), true); err != nil {
		t.Fatal(err)
	}
	response, err := f.target.client.Core().CreateEntity(context.Background(), &pb.CreateEntityRequest{Handle: f.target.enter(t, pb.Intent_CREATE_ENTITY), Element: proto.String("00000000-0000-4000-8000-000000000004"), Final: networkTuple(1), OperationId: []byte("after-capture")})
	if err != nil || response.GetResult() == nil {
		t.Fatalf("write: %v %v", response, err)
	}
	if err := part.Check(context.Background(), false); err != nil {
		t.Fatalf("retained read unnecessarily invalidated: %v", err)
	}
	if err := part.Check(context.Background(), true); !errors.Is(err, values.PublicInvalidContext) {
		t.Fatalf("noncurrent source accepted for initial composite: %v", err)
	}
	old, err := part.Project(context.Background(), federationcontract.FederationProjectionRequest{Kind: "read", Element: networkEntity}, nil, 1)
	if err != nil || len(old.Records) != 1 || old.Records[0].Attachment == nil {
		t.Fatalf("retained exact projection: %+v %v", old, err)
	}
}

func TestNetworkFederationReadAtObtainsIndependentProof(t *testing.T) {
	f := newFederationNetworkFixture(t, true)
	f.request.Intent = "query"
	f.grant(t, corecontract.PermissionQueryRegion, false, "query")
	f.grant(t, corecontract.PermissionReadElement, false, "read-is-not-delegated")
	part, err := f.open()
	if err != nil {
		t.Fatal(err)
	}
	defer part.Release(context.Background())
	if page, err := part.Project(context.Background(), federationcontract.FederationProjectionRequest{Kind: "read-at", Element: networkEntity}, nil, 1); err != nil || len(page.Records) != 1 || page.Records[0].Attachment == nil {
		t.Fatal("independent Read proof failed", page, err)
	}
	f.grant(t, corecontract.PermissionReadElement, true, "revoke-read")
	f.grant(t, corecontract.PermissionReadElement, false, "regrant-read")
	if _, err := part.Project(context.Background(), federationcontract.FederationProjectionRequest{Kind: "read-at", Element: networkEntity}, nil, 1); !errors.Is(err, values.PublicInvalidContext) {
		t.Fatal("fresh proof renewed old Read authorization", err)
	}
}

func TestNetworkFederationReadAtKeepsHistoricalRecord(t *testing.T) {
	f := newFederationNetworkFixture(t, true)
	f.request.Intent = "query"
	f.grant(t, corecontract.PermissionQueryRegion, false, "query-history")
	f.grant(t, corecontract.PermissionReadElement, false, "read-history")
	part, err := f.open()
	if err != nil {
		t.Fatal(err)
	}
	defer part.Release(context.Background())
	read := federationcontract.FederationProjectionRequest{Kind: "read-at", Element: networkEntity}
	old, err := part.Project(context.Background(), read, nil, 1)
	if err != nil {
		t.Fatal(err)
	}
	final := networkTuple(2)
	final.Pose.Translation.Items[0] = 3
	raw, err := values.DecodePublicBytes(old.Records[0].Revision.Value)
	if err != nil {
		t.Fatal(err)
	}
	expected := &netpb.Token{Owner: proto.String(string(old.Records[0].Revision.Owner)), Kind: netpb.TokenKind_RECORD_REVISION.Enum(), Value: raw}
	updated, err := f.target.client.Core().UpdateEntity(context.Background(), &pb.UpdateEntityRequest{Element: proto.String(networkEntity), Expected: expected, Final: final, Handle: f.target.enter(t, pb.Intent_UPDATE_ENTITY), OperationId: []byte("move-after-read-basis")})
	if err != nil || updated.GetResult() == nil {
		t.Fatal("update", updated, err)
	}
	again, err := part.Project(context.Background(), read, nil, 1)
	if err != nil || len(again.Records) != 1 || again.Records[0].Pose.Translation.X != old.Records[0].Pose.Translation.X || again.Records[0].Revision != old.Records[0].Revision {
		t.Fatal("read proof recaptured latest facts", again, err)
	}
}
