package acceptance_test

import (
	"context"
	"encoding/binary"
	"net"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	"ucupa/composition"
	networkpb "ucupa/shared/bindings/go/protobuf"
	"ucupa/shared/values"
	corepb "ucupa/ucupa-core/bindings/go/protobuf"
	corecontract "ucupa/ucupa-core/contract"
	coresdk "ucupa/ucupa-core/sdk/go"
	coregrpc "ucupa/ucupa-core/transport/grpc"
)

const controlFacet values.FactAuthorityRef = "00000000-0000-4000-8000-000000004fac"

func startControlFixture(t *testing.T, enabled bool) (*networkFixture, *corepb.ServiceAttachmentKey) {
	t.Helper()
	f := newNetworkFixture(t, false)
	f.grant(t, corecontract.PermissionFacetAttachmentRead, values.CoreElementScope(f.c.AuthorityRef(), networkRoot), false)
	f.grant(t, corecontract.PermissionObserveAttachment, values.CoreElementScope(f.c.AuthorityRef(), networkRoot), false)
	r, err := f.c.Facets().ReadFacetAttachment(f.actor, networkRoot)
	if err != nil {
		t.Fatal(err)
	}
	b := r.Attachment.Revision.Bytes()
	k := &corepb.ServiceAttachmentKey{Core: proto.String(string(f.c.AuthorityRef())), Facet: proto.String(string(controlFacet)), Element: proto.String(string(networkRoot)), Proposal: &corepb.ServiceRevision{Incarnation: b[:32], Sequence: proto.Uint64(binary.BigEndian.Uint64(b[68:]))}}
	if enabled {
		f.config.FacetServices = map[values.PrincipalRef]values.FactAuthorityRef{f.actor: controlFacet}
	}
	address, _ := f.config.NextHop.NetworkTarget()
	l, err := net.Listen("tcp", address)
	if err != nil {
		t.Fatal(err)
	}
	f.config.FacetControl = f.c.Facets()
	f.config.AttachmentControl = f.c.Facets()
	f.config.MountControl = f.c.Mounts()
	f.server, err = coregrpc.New(f.c.Access(), composition.RealmNetworkAuthentication{Connections: f.realmLinks, Owner: f.c.Access()}, f.config)
	if err != nil {
		l.Close()
		t.Fatal(err)
	}
	go f.server.Serve(l)
	f.connect(t)
	return f, k
}

func TestNetworkAttachmentControlServiceIsolation(t *testing.T) {
	f, key := startControlFixture(t, true)
	ctx := context.Background()
	api := f.client.AttachmentControl()
	read, err := api.ReadControl(ctx, &corepb.ReadControlRequest{Key: key, Prepare: proto.Bool(true)})
	if err != nil || read.GetResult() == nil || read.GetResult().GetState() != corepb.AttachmentState_NONE || read.GetResult().Preparation == nil {
		t.Fatalf("control read: %v %v", read, err)
	}
	check := &corepb.CheckControlRequest{Key: key, Preparation: read.GetResult().Preparation}
	out, err := api.CheckControl(ctx, check)
	if err != nil || !out.GetResult().GetValid() {
		t.Fatalf("check: %v %v", out, err)
	}
	// A fresh authentication creates a distinct service context. It cannot
	// borrow preparation state from the original connection's session.
	other, err := coresdk.New(f.clientConfig)
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	if err := other.Establish(ctx, f.actor.Realm, f.id, f.secret); err != nil {
		t.Fatal(err)
	}
	borrowed, err := other.AttachmentControl().CheckControl(ctx, check)
	if err != nil || borrowed.GetFailure() != networkpb.Failure_DENIED {
		t.Fatalf("foreign session borrowed preparation: %v %v", borrowed, err)
	}
	wrong := proto.Clone(key).(*corepb.ServiceAttachmentKey)
	wrong.Facet = proto.String("00000000-0000-4000-8000-000000004fad")
	if _, err := api.ReadControl(ctx, &corepb.ReadControlRequest{Key: wrong, Prepare: proto.Bool(false)}); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("wrong relationship: %v", err)
	}
	f.grant(t, corecontract.PermissionFacetAttachmentRead, values.CoreElementScope(f.c.AuthorityRef(), networkRoot), true)
	denied, err := api.ReadControl(ctx, &corepb.ReadControlRequest{Key: key, Prepare: proto.Bool(false)})
	if err != nil || denied.GetFailure() != networkpb.Failure_DENIED {
		t.Fatalf("service configuration replaced owner authorization: %v %v", denied, err)
	}
}

func TestNetworkAttachmentControlDisabled(t *testing.T) {
	f, key := startControlFixture(t, false)
	_, err := f.client.AttachmentControl().ReadControl(context.Background(), &corepb.ReadControlRequest{Key: key, Prepare: proto.Bool(false)})
	if status.Code(err) != codes.PermissionDenied {
		t.Fatalf("ordinary authorized user gained service port: %v", err)
	}
}
