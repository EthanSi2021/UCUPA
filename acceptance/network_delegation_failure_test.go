package acceptance_test

import (
	"context"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"ucupa/composition"
	networkpb "ucupa/shared/bindings/go/protobuf"
	"ucupa/shared/values"
	corepb "ucupa/ucupa-core/bindings/go/protobuf"
	coresdk "ucupa/ucupa-core/sdk/go"
	coregrpc "ucupa/ucupa-core/transport/grpc"
)

func TestNetworkReadDelegationExpiresAndReopensWithoutRevival(t *testing.T) {
	target, actor := newDelegationFixture(t)
	grant := issueReadDelegation(t, target, actor, false, 1, corepb.Intent_READ)
	client := delegatedClient(t, target, actor, grant)
	ctx := context.Background()
	request := &corepb.ReadElementRequest{Handle: grant.Handle, Address: &networkpb.Address{}}
	if out, err := client.Core().ReadElement(ctx, request); err != nil || out.GetResult() == nil {
		t.Fatalf("initial delegation: %v %v", out, err)
	}
	time.Sleep(1100 * time.Millisecond)
	if _, err := client.Core().ReadElement(ctx, request); status.Code(err) != codes.Unauthenticated {
		t.Fatalf("expired: %v", err)
	}
	old := issueReadDelegation(t, target, actor, false, 10, corepb.Intent_READ)
	target.server.Stop()
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
	// New connection does not revive the old run-local grant.
	freshConnection := delegatedClient(t, target, actor, old)
	if _, err := freshConnection.Core().ReadElement(ctx, &corepb.ReadElementRequest{Handle: old.Handle, Address: &networkpb.Address{}}); status.Code(err) != codes.Unauthenticated {
		t.Fatalf("restart revival: %v", err)
	}
	_ = target.client.Close()
	target.connect(t)
	newGrant := issueReadDelegation(t, target, actor, false, 10, corepb.Intent_READ)
	if proto.Equal(old.Handle, newGrant.Handle) {
		t.Fatal("grant reused old caller context")
	}
	newClient := delegatedClient(t, target, actor, newGrant)
	if _, err := newClient.Core().ReadElement(ctx, &corepb.ReadElementRequest{Handle: old.Handle, Address: &networkpb.Address{}}); status.Code(err) != codes.Unauthenticated {
		t.Fatalf("new grant borrowed old handle: %v", err)
	}
}

func TestNetworkReadDelegationConcurrentCapacityAndNewSessionRevocation(t *testing.T) {
	target, actor := newDelegationFixture(t)
	var wg sync.WaitGroup
	results := make(chan *corepb.ReadDelegation, 12)
	errors := make(chan error, 12)
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			out, err := target.client.Delegations().CreateReadDelegation(context.Background(), &corepb.CreateReadDelegationRequest{Reference: target.reference(), Actor: proto.String(string(actor.c.AuthorityRef())), Intents: []corepb.Intent{corepb.Intent_READ}, Observe: proto.Bool(false), TtlSeconds: proto.Uint64(10)})
			if err != nil {
				errors <- err
				return
			}
			results <- out.GetResult()
		}()
	}
	wg.Wait()
	close(results)
	close(errors)
	var granted []*corepb.ReadDelegation
	for grant := range results {
		if grant == nil {
			t.Fatal("unexpected domain refusal")
		}
		granted = append(granted, grant)
	}
	for err := range errors {
		if status.Code(err) != codes.ResourceExhausted {
			t.Fatalf("capacity: %v", err)
		}
	}
	if len(granted) != 3 {
		t.Fatalf("per-caller admission %d", len(granted))
	}
	client := delegatedClient(t, target, actor, granted[0])
	if err := target.client.Release(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := target.client.Establish(context.Background(), target.actor.Realm, target.id, target.secret); err != nil {
		t.Fatal(err)
	}
	if _, err := target.client.Delegations().RevokeReadDelegation(context.Background(), &corepb.RevokeReadDelegationRequest{Token: granted[0].Token}); err != nil {
		t.Fatal(err)
	}
	if _, err := client.Core().ReadElement(context.Background(), &corepb.ReadElementRequest{Handle: granted[0].Handle, Address: &networkpb.Address{}}); status.Code(err) != codes.Unauthenticated {
		t.Fatalf("revoke from new session: %v", err)
	}
	_ = issueReadDelegation(t, target, actor, false, 10, corepb.Intent_READ)
}

func TestNetworkReadDelegationRevokedWhileReadIsInFlight(t *testing.T) {
	target, actor := newDelegationFixture(t)
	target.server.Stop()
	owner := &pausedNetworkOwner{Core: target.c, entered: make(chan struct{}), release: make(chan struct{})}
	var release sync.Once
	defer release.Do(func() { close(owner.release) })
	l, err := net.Listen("tcp", strings.TrimPrefix(string(target.config.NextHop), "https://"))
	if err != nil {
		t.Fatal(err)
	}
	target.server, err = coregrpc.New(owner, composition.RealmNetworkAuthentication{Connections: target.realmLinks, Owner: target.c.Access()}, target.config)
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = target.server.Serve(l) }()
	_ = target.client.Close()
	target.connect(t)
	grant := issueReadDelegation(t, target, actor, false, 10, corepb.Intent_READ)
	client := delegatedClient(t, target, actor, grant)
	owner.armed.Store(true)
	done := make(chan error, 1)
	go func() {
		_, err := client.Core().ReadElement(context.Background(), &corepb.ReadElementRequest{Handle: grant.Handle, Address: &networkpb.Address{}})
		done <- err
	}()
	select {
	case <-owner.entered:
	case <-time.After(3 * time.Second):
		t.Fatal("read not dispatched")
	}
	if _, err := target.client.Delegations().RevokeReadDelegation(context.Background(), &corepb.RevokeReadDelegationRequest{Token: grant.Token}); err != nil {
		t.Fatal(err)
	}
	release.Do(func() { close(owner.release) })
	select {
	case err := <-done:
		if status.Code(err) != codes.Unauthenticated {
			t.Fatalf("in-flight revoked read: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("read did not finish")
	}
}

func TestNetworkReadDelegationRejectsTrustedCAWrongActorKey(t *testing.T) {
	target, actor := newDelegationFixture(t)
	// The key is cryptographically authenticated, but not bound to this actor.
	// Trusting a CA alone must not permit another Core to use a stolen grant.
	target.server.Stop()
	config := *target.config.ReadDelegation
	config.Peers = map[values.FactAuthorityRef][32]byte{actor.c.AuthorityRef(): target.clientConfig.SPKISHA256}
	target.config.ReadDelegation = &config
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
	_ = target.client.Close()
	target.connect(t)
	grant := issueReadDelegation(t, target, actor, false, 10, corepb.Intent_READ)
	cfg := target.clientConfig
	cert := actor.config.Certificate
	cfg.Actor, cfg.ActorCertificate = actor.c.AuthorityRef(), &cert
	client, err := coresdk.New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	if err := client.UseReadDelegation(grant); err != nil {
		t.Fatal(err)
	}
	if _, err := client.Core().ReadElement(context.Background(), &corepb.ReadElementRequest{Handle: grant.Handle, Address: &networkpb.Address{}}); status.Code(err) != codes.Unauthenticated {
		t.Fatalf("CA without actor pin: %v", err)
	}
	// The ordinary user profile remains usable without any client certificate.
	if _, err := target.client.Core().Enter(context.Background(), &corepb.EnterRequest{Reference: target.reference(), Intent: corepb.Intent_READ.Enum()}); err != nil {
		t.Fatal(err)
	}
}

func TestNetworkReadDelegationRevokePreservesAuthenticationUnavailable(t *testing.T) {
	target, actor := newDelegationFixture(t)
	grant := issueReadDelegation(t, target, actor, false, 10, corepb.Intent_READ)
	if err := target.r.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := target.client.Delegations().RevokeReadDelegation(context.Background(), &corepb.RevokeReadDelegationRequest{Token: grant.Token}); status.Code(err) != codes.Unavailable {
		t.Fatalf("closed Realm presented as invalid identity or successful revoke: %v", err)
	}
}
