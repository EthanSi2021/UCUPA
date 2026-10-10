package acceptance_test

import (
	"context"
	"net"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	"ucupa/composition"
	netpb "ucupa/shared/bindings/go/protobuf"
	"ucupa/shared/values"
	pb "ucupa/ucupa-core/bindings/go/protobuf"
	"ucupa/ucupa-core/contract"
	coresdk "ucupa/ucupa-core/sdk/go"
	coregrpc "ucupa/ucupa-core/transport/grpc"
	realm "ucupa/ucupa-realm/contract"
)

func transitionApprovalSetup(t *testing.T) (target, source *networkFixture, binding *pb.TransitionApprovalBinding) {
	t.Helper()
	source, target = newNetworkFixtureRealm(t, true, "source-realm"), newNetworkFixture(t, false)
	target.grant(t, contract.PermissionPlaceElement, values.CoreElementScope(target.c.AuthorityRef(), networkRoot), false)
	ctx := context.Background()
	out, err := source.client.Core().CreateAnchor(ctx, &pb.CreateAnchorRequest{Handle: source.enter(t, pb.Intent_CREATE_ANCHOR), Element: proto.String(networkEntity), OperationId: []byte("source-element"), Final: networkTuple(4)})
	if err != nil || out.GetResult() == nil {
		t.Fatalf("source create: %v %v", out, err)
	}
	read, err := source.client.Core().ReadElement(ctx, &pb.ReadElementRequest{Handle: source.enter(t, pb.Intent_READ), Address: &netpb.Address{Down: &netpb.ElementPath{Items: []string{networkEntity}}}})
	if err != nil || read.GetResult().GetRecord() == nil {
		t.Fatalf("source read: %v %v", read, err)
	}
	command := &pb.BeginTransitionRequest{OperationId: []byte{255, 0, 128}, Kind: pb.TransitionKind_TRANSITION_SLICE.Enum(), Target: proto.String(string(target.c.AuthorityRef())), Boundary: proto.String(networkEntity), Expected: read.GetResult().GetRecord().GetRevision(), Final: networkTuple(4), SourceAddress: proto.String(string(source.config.NextHop)), TargetAddress: proto.String(string(target.config.NextHop))}
	binding = &pb.TransitionApprovalBinding{Source: proto.String(string(source.c.AuthorityRef())), Command: command}
	target.config.ReadDelegation = &coregrpc.ReadDelegationConfig{ClientRoots: source.clientConfig.Roots, Peers: map[values.FactAuthorityRef][32]byte{source.c.AuthorityRef(): source.clientConfig.SPKISHA256}, MaxTTL: 10 * time.Second, MaxGrants: 4, MaxPerCaller: 3}
	target.config.TransitionApprovals = &coregrpc.TransitionApprovalConfig{MaxTTL: 5 * time.Second, MaxApprovals: 3, MaxPerCaller: 2}
	target.config.TransitionImport = target.c.Transfers()
	return
}

func startTransitionApprovalTarget(t *testing.T, target, source *networkFixture) *coresdk.Client {
	t.Helper()
	l, err := net.Listen("tcp", strings.TrimPrefix(string(target.config.NextHop), "https://"))
	if err != nil {
		t.Fatal(err)
	}
	target.server, err = coregrpc.New(target.c.Access(), composition.RealmNetworkAuthentication{Connections: target.realmLinks, Owner: target.c.Access()}, target.config)
	if err != nil {
		_ = l.Close()
		t.Fatal(err)
	}
	go func() { _ = target.server.Serve(l) }()
	target.connect(t)
	return transitionApprovalService(t, target, source)
}

func transitionApprovalService(t *testing.T, target, source *networkFixture) *coresdk.Client {
	t.Helper()
	cfg := target.clientConfig
	cfg.Actor, cfg.ActorCertificate = source.c.AuthorityRef(), &source.config.Certificate
	service, err := coresdk.New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = service.Close() })
	return service
}

func createTransitionApproval(t *testing.T, target *networkFixture, b *pb.TransitionApprovalBinding, seconds uint64) *pb.TransitionApproval {
	t.Helper()
	out, err := target.client.TransitionApprovals().CreateTransitionApproval(context.Background(), &pb.CreateTransitionApprovalRequest{Binding: b, TtlSeconds: proto.Uint64(seconds)})
	if err != nil || out.GetResult() == nil {
		t.Fatalf("create transition approval: %v %v", out, err)
	}
	return out.GetResult()
}

func TestTransitionApprovalExactInputOriginalTargetIdentityAndPermission(t *testing.T) {
	target, source, b := transitionApprovalSetup(t)
	service := startTransitionApprovalTarget(t, target, source)
	ctx := context.Background()
	request := &pb.CreateTransitionApprovalRequest{Binding: b, TtlSeconds: proto.Uint64(5)}
	if out, err := target.client.TransitionApprovals().CreateTransitionApproval(ctx, request); err != nil || out.GetFailure() != netpb.Failure_DENIED {
		t.Fatalf("CRUD became import: %v %v", out, err)
	}
	target.grant(t, contract.PermissionTransitionImport, values.CoreElementScope(target.c.AuthorityRef(), values.ElementID(networkEntity)), false)
	approval := createTransitionApproval(t, target, b, 5)
	check := &pb.CheckTransitionApprovalRequest{Binding: b, Token: approval.Token}
	out, err := service.TransitionApprovals().CheckTransitionApproval(ctx, check)
	if err != nil || out.GetResult().GetRealm() != string(target.actor.Realm) || out.GetResult().GetPrincipal() != string(target.actor.Principal) {
		t.Fatalf("target did not retain original identity: %v %v", out, err)
	}
	for name, change := range map[string]func(*pb.BeginTransitionRequest){
		"operation":      func(c *pb.BeginTransitionRequest) { c.OperationId = []byte{255, 0, 129} },
		"boundary":       func(c *pb.BeginTransitionRequest) { c.Boundary = proto.String(string(networkRoot)) },
		"expected":       func(c *pb.BeginTransitionRequest) { c.Expected.Value[0] ^= 1 },
		"placement":      func(c *pb.BeginTransitionRequest) { c.Final = networkTuple(5) },
		"source address": func(c *pb.BeginTransitionRequest) { c.SourceAddress = proto.String("https://other.example:443") },
		"target address": func(c *pb.BeginTransitionRequest) { c.TargetAddress = proto.String("https://other.example:443") },
	} {
		t.Run(name, func(t *testing.T) {
			changed := proto.Clone(b).(*pb.TransitionApprovalBinding)
			change(changed.Command)
			if _, err := service.TransitionApprovals().CheckTransitionApproval(ctx, &pb.CheckTransitionApprovalRequest{Binding: changed, Token: approval.Token}); status.Code(err) != codes.Unauthenticated {
				t.Fatalf("changed binding accepted: %v", err)
			}
		})
	}
	if _, err := target.client.TransitionApprovals().CheckTransitionApproval(ctx, check); status.Code(err) != codes.Unauthenticated {
		t.Fatalf("user replaced fixed source identity: %v", err)
	}
	read := issueReadDelegation(t, target, source, false, 5, pb.Intent_READ)
	delegated := delegatedClient(t, target, source, read)
	if _, err := delegated.TransitionApprovals().CreateTransitionApproval(ctx, request); status.Code(err) != codes.Unauthenticated {
		t.Fatalf("read grant upgraded: %v", err)
	}
	if _, err := target.client.TransitionApprovals().RevokeTransitionApproval(ctx, &pb.RevokeTransitionApprovalRequest{Token: approval.Token}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.TransitionApprovals().CheckTransitionApproval(ctx, check); status.Code(err) != codes.Unauthenticated {
		t.Fatalf("revoked approval remained usable: %v", err)
	}
}

func TestTransitionApprovalABAExpiryCapacityAndCredentialRevocation(t *testing.T) {
	target, source, b := transitionApprovalSetup(t)
	service := startTransitionApprovalTarget(t, target, source)
	ctx := context.Background()
	scope := values.CoreElementScope(target.c.AuthorityRef(), values.ElementID(networkEntity))
	target.grant(t, contract.PermissionTransitionImport, scope, false)
	a := createTransitionApproval(t, target, b, 5)
	target.grant(t, contract.PermissionTransitionImport, scope, true)
	target.grant(t, contract.PermissionTransitionImport, scope, false)
	if out, err := service.TransitionApprovals().CheckTransitionApproval(ctx, &pb.CheckTransitionApprovalRequest{Binding: b, Token: a.Token}); err != nil || out.GetFailure() != netpb.Failure_INVALID_CONTEXT {
		t.Fatalf("regrant revived original view: %v %v", out, err)
	}
	short := createTransitionApproval(t, target, b, 1)
	if _, err := target.client.TransitionApprovals().CreateTransitionApproval(ctx, &pb.CreateTransitionApprovalRequest{Binding: b, TtlSeconds: proto.Uint64(5)}); status.Code(err) != codes.ResourceExhausted {
		t.Fatalf("capacity did not bound retained approvals: %v", err)
	}
	time.Sleep(1100 * time.Millisecond)
	if _, err := service.TransitionApprovals().CheckTransitionApproval(ctx, &pb.CheckTransitionApprovalRequest{Binding: b, Token: short.Token}); status.Code(err) != codes.Unauthenticated {
		t.Fatalf("expired approval: %v", err)
	}
	fresh := createTransitionApproval(t, target, b, 5)
	if _, err := target.r.RevokeCredential(target.admin, realm.CredentialID(target.id), target.r.CredentialSetRevision(), "revoke-import-credential"); err != nil {
		t.Fatal(err)
	}
	if out, err := service.TransitionApprovals().CheckTransitionApproval(ctx, &pb.CheckTransitionApprovalRequest{Binding: b, Token: fresh.Token}); err != nil || out.GetFailure() != netpb.Failure_DENIED {
		t.Fatalf("revoked credential: %v %v", out, err)
	}
}
