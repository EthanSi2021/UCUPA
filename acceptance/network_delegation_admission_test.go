package acceptance_test

import (
	"context"
	"crypto/tls"
	"strings"
	"testing"
	"time"
	corecontract "ucupa/ucupa-core/contract"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	networkpb "ucupa/shared/bindings/go/protobuf"
	"ucupa/shared/values"
	corepb "ucupa/ucupa-core/bindings/go/protobuf"
)

func TestNetworkReadDelegationInvalidOrDeniedDoesNotOccupyCapacity(t *testing.T) {
	target, actor := newDelegationFixture(t)
	base := &corepb.CreateReadDelegationRequest{Reference: target.reference(), Actor: proto.String(string(actor.config.Expected)), Intents: []corepb.Intent{corepb.Intent_READ}, Observe: proto.Bool(false), TtlSeconds: proto.Uint64(10)}
	for name, change := range map[string]func(*corepb.CreateReadDelegationRequest){
		"zero ttl":          func(r *corepb.CreateReadDelegationRequest) { r.TtlSeconds = proto.Uint64(0) },
		"large ttl":         func(r *corepb.CreateReadDelegationRequest) { r.TtlSeconds = proto.Uint64(^uint64(0)) },
		"write":             func(r *corepb.CreateReadDelegationRequest) { r.Intents = []corepb.Intent{corepb.Intent_CREATE_ENTITY} },
		"empty intents":     func(r *corepb.CreateReadDelegationRequest) { r.Intents = nil },
		"duplicate intents": func(r *corepb.CreateReadDelegationRequest) { r.Intents = append(r.Intents, corepb.Intent_READ) },
		"malformed actor":   func(r *corepb.CreateReadDelegationRequest) { r.Actor = proto.String("not-a-core") },
		"different address": func(r *corepb.CreateReadDelegationRequest) {
			r.Reference.NextHop = proto.String(string(actor.config.NextHop))
		},
	} {
		t.Run(name, func(t *testing.T) {
			request := proto.Clone(base).(*corepb.CreateReadDelegationRequest)
			change(request)
			if _, err := target.client.Delegations().CreateReadDelegation(context.Background(), request); status.Code(err) != codes.InvalidArgument {
				t.Fatalf("invalid issue: %v", err)
			}
		})
	}
	// A valid shape still requires the user's target-side permission, not the
	// actor's service privileges. Repeated refusals must release reservations.
	scope := values.AnchorClosureScope(target.config.Expected, networkRoot)
	target.grant(t, corecontract.PermissionReadElement, scope, true)
	for i := 0; i < 6; i++ {
		out, err := target.client.Delegations().CreateReadDelegation(context.Background(), base)
		if err != nil || out.GetFailure() != networkpb.Failure_DENIED {
			t.Fatalf("unauthorized issue %d: %v %v", i, out, err)
		}
	}
	target.grant(t, corecontract.PermissionReadElement, scope, false)
	for i := 0; i < 3; i++ {
		_ = issueReadDelegation(t, target, actor, false, 10, corepb.Intent_READ)
	}
	if _, err := target.client.Delegations().CreateReadDelegation(context.Background(), base); status.Code(err) != codes.ResourceExhausted {
		t.Fatalf("capacity not enforced: %v", err)
	}
}

func TestNetworkReadDelegationTokenAloneCannotAuthenticateActor(t *testing.T) {
	target, actor := newDelegationFixture(t)
	grant := issueReadDelegation(t, target, actor, false, 10, corepb.Intent_READ)
	// Bypass the SDK's local check: the server must reject a stolen token on a
	// valid TLS connection lacking the actor's authenticated client certificate.
	conn, err := grpc.NewClient("passthrough:///"+strings.TrimPrefix(string(target.config.NextHop), "https://"), grpc.WithNoProxy(), grpc.WithDisableRetry(), grpc.WithTransportCredentials(credentials.NewTLS(&tls.Config{RootCAs: target.clientConfig.Roots, ServerName: "127.0.0.1", MinVersion: tls.VersionTLS13})))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	ctx = metadata.NewOutgoingContext(ctx, metadata.Pairs("ucupa-target", string(target.config.Expected), "authorization", "Delegation "+values.EncodePublicBytes(grant.Token)))
	if _, err := corepb.NewCoreServiceClient(conn).ReadElement(ctx, &corepb.ReadElementRequest{Handle: grant.Handle, Address: &networkpb.Address{}}); status.Code(err) != codes.Unauthenticated {
		t.Fatalf("token without actor key: %v", err)
	}
}

func TestNetworkReadDelegationDoesNotBroadenAnchorBoundary(t *testing.T) {
	target, actor := newDelegationFixture(t)
	ctx := context.Background()
	created, err := target.client.Core().CreateAnchor(ctx, &corepb.CreateAnchorRequest{Handle: target.enter(t, corepb.Intent_CREATE_ANCHOR), Element: proto.String(networkEntity), OperationId: []byte("delegation-anchor"), Final: networkTuple(10)})
	if err != nil || created.GetResult() == nil {
		t.Fatalf("create scope: %v %v", created, err)
	}
	anchor := values.ElementID(networkEntity)
	scope := values.AnchorClosureScope(target.config.Expected, anchor)
	target.grant(t, corecontract.PermissionReadElement, scope, false)
	reference := target.reference()
	reference.Boundary.Anchor = proto.String(networkEntity)
	issued, err := target.client.Delegations().CreateReadDelegation(ctx, &corepb.CreateReadDelegationRequest{Reference: reference, Actor: proto.String(string(actor.config.Expected)), Intents: []corepb.Intent{corepb.Intent_READ}, Observe: proto.Bool(false), TtlSeconds: proto.Uint64(10)})
	if err != nil || issued.GetResult() == nil {
		t.Fatalf("scoped issue: %v %v", issued, err)
	}
	grant := issued.GetResult()
	client := delegatedClient(t, target, actor, grant)
	inside, err := client.Core().ReadElement(ctx, &corepb.ReadElementRequest{Handle: grant.Handle, Address: &networkpb.Address{}})
	if err != nil || inside.GetResult().GetRecord().GetElement() != networkEntity {
		t.Fatalf("scope root: %v %v", inside, err)
	}
	for _, address := range []*networkpb.Address{{Up: proto.Uint64(1)}, {Origin: proto.String(string(networkRoot))}} {
		out, err := client.Core().ReadElement(ctx, &corepb.ReadElementRequest{Handle: grant.Handle, Address: address})
		// P5 rejects an address outside its declared boundary as INVALID before
		// target lookup. Delegation retains that rule, rather than adding a new
		// existence or permission oracle for the enclosing tree.
		if err != nil || out.GetFailure() != networkpb.Failure_INVALID {
			t.Fatalf("escaped delegated anchor: %v %v", out, err)
		}
	}
}

func TestNetworkReadDelegationDisabledIsNotAnImplicitAllow(t *testing.T) {
	target := newNetworkFixture(t, true)
	_, err := target.client.Delegations().CreateReadDelegation(context.Background(), &corepb.CreateReadDelegationRequest{Reference: target.reference(), Actor: proto.String(string(target.config.Expected)), Intents: []corepb.Intent{corepb.Intent_READ}, Observe: proto.Bool(false), TtlSeconds: proto.Uint64(1)})
	if status.Code(err) != codes.Unimplemented {
		t.Fatalf("disabled delegation: %v", err)
	}
	_ = target.enter(t, corepb.Intent_READ)
}
