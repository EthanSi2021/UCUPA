package acceptance_test

import (
	"context"
	"crypto/sha256"
	"net"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	"ucupa/composition"
	networkpb "ucupa/shared/bindings/go/protobuf"
	"ucupa/shared/values"
	corepb "ucupa/ucupa-core/bindings/go/protobuf"
	corecontract "ucupa/ucupa-core/contract"
	core "ucupa/ucupa-core/instance"
	coresdk "ucupa/ucupa-core/sdk/go"
	coregrpc "ucupa/ucupa-core/transport/grpc"
	realm "ucupa/ucupa-realm/contract"
)

func mountApprovalFixture(t *testing.T, empty ...bool) (target, actor *networkFixture, service *coresdk.Client, binding *corepb.MountApprovalBinding) {
	t.Helper()
	actor, target = newNetworkFixtureRealm(t, false, "mount-parent-realm"), newNetworkFixture(t, false)
	if len(empty) != 0 && empty[0] {
		if err := target.c.Close(); err != nil {
			t.Fatal(err)
		}
		var err error
		target.c, err = core.CreateDurable(target.realmLinks, corecontract.RealmInitialization{Realm: target.actor.Realm, NextHop: target.realmAddress}, filepath.Join(target.dir, "empty-carrier.db"), corecontract.Config{PublicAccessPolicy: &values.PublicAccessPolicy{ContextTTL: time.Minute, ObservationTTL: time.Minute}})
		if err != nil {
			t.Fatal(err)
		}
		target.config.Expected, target.clientConfig.Expected = target.c.AuthorityRef(), target.c.AuthorityRef()
	}
	target.config.ReadDelegation = &coregrpc.ReadDelegationConfig{ClientRoots: actor.clientConfig.Roots, Peers: map[values.FactAuthorityRef][32]byte{actor.c.AuthorityRef(): actor.clientConfig.SPKISHA256}, MaxTTL: 10 * time.Second, MaxGrants: 4, MaxPerCaller: 3}
	target.config.MountApprovals = &coregrpc.MountApprovalConfig{MaxTTL: 5 * time.Second, MaxApprovals: 3, MaxPerCaller: 2}
	l, err := net.Listen("tcp", strings.TrimPrefix(string(target.config.NextHop), "https://"))
	if err != nil {
		t.Fatal(err)
	}
	target.config.FacetControl = target.c.Facets()
	target.config.MountControl = target.c.Mounts()
	target.config.Federation = target.c.Federation()
	target.server, err = coregrpc.New(target.c.Access(), composition.RealmNetworkAuthentication{Connections: target.realmLinks, Owner: target.c.Access()}, target.config)
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = target.server.Serve(l) }()
	target.connect(t)
	cfg := target.clientConfig
	cfg.Actor, cfg.ActorCertificate = actor.c.AuthorityRef(), &actor.config.Certificate
	service, err = coresdk.New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = service.Close() })
	digest := sha256.Sum256([]byte("one exact test mount command commitment"))
	binding = &corepb.MountApprovalBinding{Actor: proto.String(string(actor.c.AuthorityRef())), Element: proto.String(networkEntity), OperationId: []byte("mount-command"), CommandDigest: digest[:]}
	return
}

func createMountApproval(t *testing.T, target *networkFixture, binding *corepb.MountApprovalBinding, seconds uint64) *corepb.MountApproval {
	t.Helper()
	out, err := target.client.MountApprovals().CreateMountApproval(context.Background(), &corepb.CreateMountApprovalRequest{Binding: binding, TtlSeconds: proto.Uint64(seconds)})
	if err != nil || out.GetResult() == nil {
		t.Fatalf("approval: %v %v", out, err)
	}
	return out.GetResult()
}

func TestNetworkMountApprovalExactBindingAndOriginalIdentity(t *testing.T) {
	target, actor, service, binding := mountApprovalFixture(t)
	ctx := context.Background()
	out, err := target.client.MountApprovals().CreateMountApproval(ctx, &corepb.CreateMountApprovalRequest{Binding: binding, TtlSeconds: proto.Uint64(5)})
	if err != nil || out.GetFailure() != networkpb.Failure_DENIED {
		t.Fatalf("missing grant: %v %v", out, err)
	}
	target.grant(t, corecontract.PermissionMountAccept, values.CoreElementScope(target.c.AuthorityRef(), values.ElementID(networkEntity)), false)
	approval := createMountApproval(t, target, binding, 5)
	request := &corepb.CheckMountApprovalRequest{Binding: binding, Token: approval.Token}
	got, err := service.MountApprovals().CheckMountApproval(ctx, request)
	if err != nil || got.GetResult().GetRealm() != string(target.actor.Realm) || got.GetResult().GetPrincipal() != string(target.actor.Principal) {
		t.Fatalf("original caller: %v %v", got, err)
	}
	// All fields are bound independently; a correct token is not a general grant.
	for _, field := range []string{"element", "operation", "command", "actor"} {
		changed := proto.Clone(binding).(*corepb.MountApprovalBinding)
		switch field {
		case "element":
			changed.Element = proto.String(string(networkRoot))
		case "operation":
			changed.OperationId = []byte("another")
		case "command":
			changed.CommandDigest[0] ^= 1
		case "actor":
			changed.Actor = proto.String(string(target.c.AuthorityRef()))
		}
		if _, err := service.MountApprovals().CheckMountApproval(ctx, &corepb.CheckMountApprovalRequest{Binding: changed, Token: approval.Token}); status.Code(err) != codes.Unauthenticated {
			t.Fatalf("changed %s: %v", field, err)
		}
	}
	if _, err := target.client.MountApprovals().CheckMountApproval(ctx, request); status.Code(err) != codes.Unauthenticated {
		t.Fatalf("user session substituted for actor key: %v", err)
	}
	read := issueReadDelegation(t, target, actor, false, 5, corepb.Intent_READ)
	delegated := delegatedClient(t, target, actor, read)
	if _, err := delegated.MountApprovals().CreateMountApproval(ctx, &corepb.CreateMountApprovalRequest{Binding: binding, TtlSeconds: proto.Uint64(5)}); status.Code(err) != codes.Unauthenticated {
		t.Fatalf("read grant upgraded: %v", err)
	}
	if _, err := target.client.MountApprovals().RevokeMountApproval(ctx, &corepb.RevokeMountApprovalRequest{Token: approval.Token}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.MountApprovals().CheckMountApproval(ctx, request); status.Code(err) != codes.Unauthenticated {
		t.Fatalf("revoked: %v", err)
	}
}

func TestNetworkMountApprovalABAExpiryCapacityAndCredentialRevocation(t *testing.T) {
	target, _, service, binding := mountApprovalFixture(t)
	ctx := context.Background()
	scope := values.CoreElementScope(target.c.AuthorityRef(), values.ElementID(networkEntity))
	target.grant(t, corecontract.PermissionMountAccept, scope, false)
	a := createMountApproval(t, target, binding, 5)
	target.grant(t, corecontract.PermissionMountAccept, scope, true)
	target.grant(t, corecontract.PermissionMountAccept, scope, false)
	out, err := service.MountApprovals().CheckMountApproval(ctx, &corepb.CheckMountApprovalRequest{Binding: binding, Token: a.Token})
	if err != nil || out.GetFailure() != networkpb.Failure_INVALID_CONTEXT {
		t.Fatalf("ABA refreshed approval: %v %v", out, err)
	}
	b := createMountApproval(t, target, binding, 1)
	if _, err := target.client.MountApprovals().CreateMountApproval(ctx, &corepb.CreateMountApprovalRequest{Binding: binding, TtlSeconds: proto.Uint64(5)}); status.Code(err) != codes.ResourceExhausted {
		t.Fatalf("capacity: %v", err)
	}
	time.Sleep(1100 * time.Millisecond)
	if _, err := service.MountApprovals().CheckMountApproval(ctx, &corepb.CheckMountApprovalRequest{Binding: binding, Token: b.Token}); status.Code(err) != codes.Unauthenticated {
		t.Fatalf("expired: %v", err)
	}
	c := createMountApproval(t, target, binding, 5)
	if _, err := target.r.RevokeCredential(target.admin, realm.CredentialID(target.id), target.r.CredentialSetRevision(), "revoke-approval-caller"); err != nil {
		t.Fatal(err)
	}
	out, err = service.MountApprovals().CheckMountApproval(ctx, &corepb.CheckMountApprovalRequest{Binding: binding, Token: c.Token})
	if err != nil || out.GetFailure() != networkpb.Failure_DENIED {
		t.Fatalf("revoked credential: %v %v", out, err)
	}
}
