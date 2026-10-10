package acceptance_test

import (
	federationcontract "ucupa/ucupa-core/contract/federation"

	"context"
	"errors"
	"reflect"
	"testing"

	"google.golang.org/protobuf/proto"

	"ucupa/shared/values"
	pb "ucupa/ucupa-core/bindings/go/protobuf"
	corecontract "ucupa/ucupa-core/contract"
)

func TestNetworkFederationObserveSourceOriginalCut(t *testing.T) {
	f := newFederationNetworkFixture(t, false)
	f.request.Intent, f.request.PrepareObserve = "query", true
	f.grant(t, corecontract.PermissionQueryRegion, false, "query-observe")
	f.grant(t, corecontract.PermissionObserveElement, false, "observe-source")
	part, err := f.open()
	if err != nil {
		t.Fatal(err)
	}
	defer part.Release(context.Background())
	initial, err := part.Project(context.Background(), federationcontract.FederationProjectionRequest{Kind: "query", Query: &federationcontract.FederationQueryRequest{Scope: networkRoot, Sphere: corecontract.QuerySphere{Center: corecontract.Vec3{X: 100}, Radius: 1}, Selector: corecontract.QueryEntities}}, nil, 1)
	if err != nil || len(initial.Matches) != 0 {
		t.Fatal("expected initially empty projection", initial, err)
	}
	created, err := f.target.client.Core().CreateEntity(context.Background(), &pb.CreateEntityRequest{Handle: f.target.enter(t, pb.Intent_CREATE_ENTITY), Element: proto.String("00000000-0000-4000-8000-000000000019"), Final: networkTuple(1), OperationId: []byte("observe-between-cut-and-begin")})
	if err != nil || created.GetResult() == nil {
		t.Fatal(created, err)
	}
	begin, err := part.Observe(context.Background(), federationcontract.FederationObserveRequest{Operation: "begin"})
	if err != nil {
		t.Fatal("ordinary write broke prepared stream", err)
	}
	request := federationcontract.FederationObserveRequest{Operation: "poll", Cursor: &begin.Cursor, Limit: 1}
	page, err := part.Observe(context.Background(), request)
	if err != nil || len(page.Groups) != 1 || page.Groups[0].ID.Owner != f.target.c.AuthorityRef() {
		t.Fatal("missed source change", page, err)
	}
	repeat, err := part.Observe(context.Background(), request)
	if err != nil || !reflect.DeepEqual(page, repeat) {
		t.Fatal("unstable wire retry", repeat, err)
	}
	request.Cursor = &page.Cursor
	empty, err := part.Observe(context.Background(), request)
	if err != nil || len(empty.Groups) != 0 {
		t.Fatal(empty, err)
	}
	f.grant(t, corecontract.PermissionObserveElement, true, "revoke-observe-source")
	f.grant(t, corecontract.PermissionObserveElement, false, "regrant-observe-source")
	if _, err := part.Observe(context.Background(), request); !errors.Is(err, values.PublicGap) {
		t.Fatal("regrant renewed source observation", err)
	}
}

func TestNetworkFederationObserveRequiresPreparedRead(t *testing.T) {
	f := newFederationNetworkFixture(t, false)
	f.request.Intent = "query"
	f.grant(t, corecontract.PermissionQueryRegion, false, "query")
	f.grant(t, corecontract.PermissionObserveElement, false, "observe-but-not-requested")
	part, err := f.open()
	if err != nil {
		t.Fatal(err)
	}
	defer part.Release(context.Background())
	if _, err := part.Observe(context.Background(), federationcontract.FederationObserveRequest{Operation: "begin"}); !errors.Is(err, values.PublicInvalidContext) {
		t.Fatal("unprepared observation added later", err)
	}
}
