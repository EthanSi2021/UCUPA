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
	networkpb "ucupa/shared/bindings/go/protobuf"
	"ucupa/shared/values"
	corepb "ucupa/ucupa-core/bindings/go/protobuf"
	corecontract "ucupa/ucupa-core/contract"
	coresdk "ucupa/ucupa-core/sdk/go"
	coregrpc "ucupa/ucupa-core/transport/grpc"
	realm "ucupa/ucupa-realm/contract"
)

func newDelegationFixture(t *testing.T) (target, actor *networkFixture) {
	t.Helper()
	actor, target = newNetworkFixture(t, false), newNetworkFixture(t, false)
	target.config.ReadDelegation = &coregrpc.ReadDelegationConfig{ClientRoots: actor.clientConfig.Roots, Peers: map[values.FactAuthorityRef][32]byte{actor.c.AuthorityRef(): actor.clientConfig.SPKISHA256}, MaxTTL: 10 * time.Second, MaxGrants: 4, MaxPerCaller: 3}
	l, err := net.Listen("tcp", strings.TrimPrefix(string(target.config.NextHop), "https://"))
	if err != nil {
		t.Fatal(err)
	}
	target.config.FacetControl = target.c.Facets()
	target.config.MountControl = target.c.Mounts()
	target.server, err = coregrpc.New(target.c.Access(), composition.RealmNetworkAuthentication{Connections: target.realmLinks, Owner: target.c.Access()}, target.config)
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = target.server.Serve(l) }()
	target.connect(t)
	return target, actor
}

func issueReadDelegation(t *testing.T, target, actor *networkFixture, observe bool, ttl uint64, intents ...corepb.Intent) *corepb.ReadDelegation {
	t.Helper()
	out, err := target.client.Delegations().CreateReadDelegation(context.Background(), &corepb.CreateReadDelegationRequest{Reference: target.reference(), Actor: proto.String(string(actor.c.AuthorityRef())), Intents: intents, Observe: proto.Bool(observe), TtlSeconds: proto.Uint64(ttl)})
	if err != nil || out.GetResult() == nil {
		t.Fatalf("issue delegation: %v %v", out, err)
	}
	return out.GetResult()
}

func delegatedClient(t *testing.T, target, actor *networkFixture, grant *corepb.ReadDelegation) *coresdk.Client {
	t.Helper()
	cfg := target.clientConfig
	cfg.Actor, cfg.ActorCertificate = actor.c.AuthorityRef(), &actor.config.Certificate
	client, err := coresdk.New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	if err := client.UseReadDelegation(grant); err != nil {
		t.Fatal(err)
	}
	return client
}

func TestNetworkReadDelegationPublicUseAndBoundaries(t *testing.T) {
	target, actor := newDelegationFixture(t)
	grant := issueReadDelegation(t, target, actor, true, 10, corepb.Intent_READ, corepb.Intent_LIST, corepb.Intent_QUERY)
	client := delegatedClient(t, target, actor, grant)
	ctx := context.Background()
	read, err := client.Core().ReadElement(ctx, &corepb.ReadElementRequest{Handle: grant.Handle, Address: &networkpb.Address{}, PrepareObserve: proto.Bool(true)})
	if err != nil || read.GetResult().GetRecord() == nil {
		t.Fatalf("delegated read: %v %v", read, err)
	}
	begin, err := client.Core().BeginObserve(ctx, &corepb.BeginObserveRequest{Preparation: read.GetResult().Preparation})
	if err != nil || begin.GetResult() == nil {
		t.Fatalf("delegated observe: %v %v", begin, err)
	}
	if out, err := client.Core().PollChanges(ctx, &corepb.PollChangesRequest{Cursor: begin.GetResult().Cursor}); err != nil || out.GetResult() == nil {
		t.Fatalf("delegated poll: %v %v", out, err)
	}
	query, err := client.Core().QueryRegion(ctx, &corepb.QueryRegionRequest{Handle: grant.Handle, Start: &networkpb.Address{}, Basis: &networkpb.Address{}, Sphere: &corepb.Sphere{Center: &networkpb.Vector3{Items: []float64{0, 0, 0}}, Radius: proto.Float64(1)}, UpDepth: proto.Int64(-1)})
	if err != nil || query.GetResult() == nil || len(query.GetResult().Matches) != 1 {
		t.Fatalf("delegated query: %v %v", query, err)
	}
	if out, err := client.Core().ListDirect(ctx, &corepb.ListDirectRequest{Handle: grant.Handle, Address: &networkpb.Address{}}); err != nil || out.GetResult() == nil {
		t.Fatalf("delegated list: %v %v", out, err)
	}
	// A never receives the user's credential, session, local caller or a write capability.
	if _, err := client.Core().Enter(ctx, &corepb.EnterRequest{Reference: target.reference(), Intent: corepb.Intent_READ.Enum()}); status.Code(err) != codes.Unauthenticated {
		t.Fatalf("delegated Enter: %v", err)
	}
	if _, err := client.Core().CreateEntity(ctx, &corepb.CreateEntityRequest{Handle: grant.Handle, Element: proto.String(networkEntity), OperationId: []byte("not-authorized-by-grant"), Final: networkTuple(1)}); status.Code(err) != codes.Unauthenticated {
		t.Fatalf("delegated write: %v", err)
	}
	if _, err := client.Delegations().CreateReadDelegation(ctx, &corepb.CreateReadDelegationRequest{Reference: target.reference(), Actor: grant.Actor, Intents: []corepb.Intent{corepb.Intent_READ}, Observe: proto.Bool(false), TtlSeconds: proto.Uint64(5)}); status.Code(err) != codes.Unauthenticated {
		t.Fatalf("redelegation: %v", err)
	}
	foreign := target.enter(t, corepb.Intent_READ)
	if _, err := client.Core().ReadElement(ctx, &corepb.ReadElementRequest{Handle: foreign, Address: &networkpb.Address{}}); status.Code(err) != codes.Unauthenticated {
		t.Fatalf("borrow user handle: %v", err)
	}
	if _, err := target.client.Delegations().RevokeReadDelegation(ctx, &corepb.RevokeReadDelegationRequest{Token: grant.Token}); err != nil {
		t.Fatal(err)
	}
	if _, err := client.Core().ReadElement(ctx, &corepb.ReadElementRequest{Handle: grant.Handle, Address: &networkpb.Address{}}); status.Code(err) != codes.Unauthenticated {
		t.Fatalf("revoked delegation: %v", err)
	}
	if _, err := target.client.Delegations().RevokeReadDelegation(ctx, &corepb.RevokeReadDelegationRequest{Token: grant.Token}); err != nil {
		t.Fatal(err)
	}
}

func TestNetworkReadDelegationNeedsActorKeyAndRequestedRights(t *testing.T) {
	target, actor := newDelegationFixture(t)
	grant := issueReadDelegation(t, target, actor, false, 10, corepb.Intent_READ)
	client := delegatedClient(t, target, actor, grant)
	ctx := context.Background()
	if _, err := client.Core().ListDirect(ctx, &corepb.ListDirectRequest{Handle: grant.Handle, Address: &networkpb.Address{}}); status.Code(err) != codes.Unauthenticated {
		t.Fatalf("extra list right: %v", err)
	}
	if _, err := client.Core().ReadElement(ctx, &corepb.ReadElementRequest{Handle: grant.Handle, Address: &networkpb.Address{}, PrepareObserve: proto.Bool(true)}); status.Code(err) != codes.Unauthenticated {
		t.Fatalf("extra observe right: %v", err)
	}
	// Correct claimed actor and stolen token, but the wrong certificate/key.
	cfg := target.clientConfig
	cfg.Actor, cfg.ActorCertificate = actor.c.AuthorityRef(), &target.config.Certificate
	wrong, err := coresdk.New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer wrong.Close()
	if err := wrong.UseReadDelegation(grant); err != nil {
		t.Fatal(err)
	}
	if _, err := wrong.Core().ReadElement(ctx, &corepb.ReadElementRequest{Handle: grant.Handle, Address: &networkpb.Address{}}); err == nil {
		t.Fatal("stolen token without actor key accepted")
	}
	// Current Realm grants still decide; the delegation is not a replacement Allow.
	target.grant(t, corecontract.PermissionReadElement, values.AnchorClosureScope(target.c.AuthorityRef(), networkRoot), true)
	read, err := client.Core().ReadElement(ctx, &corepb.ReadElementRequest{Handle: grant.Handle, Address: &networkpb.Address{}})
	if err != nil || read.GetFailure() != networkpb.Failure_DENIED {
		t.Fatalf("target revoke: %v %v", read, err)
	}
}

func TestNetworkReadDelegationObservationABAAndCredentialRevocation(t *testing.T) {
	target, actor := newDelegationFixture(t)
	grant := issueReadDelegation(t, target, actor, true, 10, corepb.Intent_READ)
	client := delegatedClient(t, target, actor, grant)
	ctx := context.Background()
	read, err := client.Core().ReadElement(ctx, &corepb.ReadElementRequest{Handle: grant.Handle, Address: &networkpb.Address{}, PrepareObserve: proto.Bool(true)})
	if err != nil || read.GetResult().Preparation == nil {
		t.Fatalf("prepare: %v %v", read, err)
	}
	scope := values.AnchorClosureScope(target.c.AuthorityRef(), networkRoot)
	target.grant(t, corecontract.PermissionObserveElement, scope, true)
	target.grant(t, corecontract.PermissionObserveElement, scope, false)
	begin, err := client.Core().BeginObserve(ctx, &corepb.BeginObserveRequest{Preparation: read.GetResult().Preparation})
	if err != nil || begin.GetFailure() != networkpb.Failure_GAP {
		t.Fatalf("delegation repaired old observation: %v %v", begin, err)
	}
	if _, err := target.r.RevokeCredential(target.admin, realm.CredentialID(target.id), target.r.CredentialSetRevision(), "revoke-delegator"); err != nil {
		t.Fatal(err)
	}
	after, err := client.Core().ReadElement(ctx, &corepb.ReadElementRequest{Handle: grant.Handle, Address: &networkpb.Address{}})
	if err != nil || after.GetFailure() != networkpb.Failure_DENIED {
		t.Fatalf("credential revoke ignored by fork: %v %v", after, err)
	}
}
