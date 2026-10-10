package entry

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	networkpb "ucupa/shared/bindings/go/protobuf"
	"ucupa/shared/values"
	corepb "ucupa/ucupa-core/bindings/go/protobuf"
	federationcontract "ucupa/ucupa-core/contract/federation"
	coresdk "ucupa/ucupa-core/sdk/go"
	"ucupa/ucupa-core/transport/grpc/federationconn"
	"ucupa/ucupa-core/transport/grpc/internal/admission"
	grpcsession "ucupa/ucupa-core/transport/grpc/internal/session"
)

func TestCallPreservesCallerReferenceAndCanonicalRequestShape(t *testing.T) {
	owner := &entryTestOwner{err: values.PublicDenied}
	service, _ := newEntryTestService(owner, 1<<20)
	reference := entryTestReference()
	reference.TrustHint = proto.String("pinned-entry")
	ctx := grpcsession.WithCaller(context.Background(), entryTestCaller)

	out, err := service.Enter(ctx, &corepb.EnterRequest{Reference: reference, Intent: corepb.Intent_QUERY.Enum()})
	if err != nil || out == nil || out.GetFailure() != networkpb.Failure_DENIED {
		t.Fatalf("Enter response = %v, error = %v", out, err)
	}
	calls := owner.snapshotCalls()
	if len(calls) != 1 || calls[0].caller != entryTestCaller {
		t.Fatalf("owner calls = %+v, want one exact-caller call", calls)
	}
	value, err := values.ParseCanonicalJSON(calls[0].request, values.PublicJSONLimits())
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]any{
		"op":     "enter",
		"intent": "query",
		"reference": map[string]any{
			"boundary":  map[string]any{"core": string(entryTestAuthority), "anchor": string(entryTestAnchor)},
			"nextHop":   string(entryTestNextHop),
			"trustHint": "pinned-entry",
		},
	}
	if !reflect.DeepEqual(value, want) {
		t.Fatalf("owner request = %#v, want %#v", value, want)
	}
}

func TestCallRejectsShapeNextHopAndMissingCallerBeforeOwner(t *testing.T) {
	owner := &entryTestOwner{err: values.PublicDenied}
	service, _ := newEntryTestService(owner, 1<<20)
	callerContext := grpcsession.WithCaller(context.Background(), entryTestCaller)

	t.Run("required request shape", func(t *testing.T) {
		out, err := service.Enter(callerContext, &corepb.EnterRequest{})
		if out != nil || status.Code(err) != codes.InvalidArgument {
			t.Fatalf("invalid request = %v, %v", out, err)
		}
	})
	t.Run("entry next hop before caller", func(t *testing.T) {
		reference := entryTestReference()
		reference.NextHop = proto.String("https://127.0.0.1:2")
		out, err := service.Enter(context.Background(), &corepb.EnterRequest{Reference: reference, Intent: corepb.Intent_READ.Enum()})
		if out != nil || status.Code(err) != codes.InvalidArgument || status.Convert(err).Message() != "reference does not name this entry" {
			t.Fatalf("wrong entry reference = %v, %v", out, err)
		}
	})
	t.Run("caller after valid entry", func(t *testing.T) {
		out, err := service.Enter(context.Background(), &corepb.EnterRequest{Reference: entryTestReference(), Intent: corepb.Intent_READ.Enum()})
		if out != nil || status.Code(err) != codes.Unauthenticated {
			t.Fatalf("missing caller = %v, %v", out, err)
		}
	})
	t.Run("request capacity", func(t *testing.T) {
		reference := entryTestReference()
		reference.TrustHint = proto.String(strings.Repeat("x", 4<<20))
		out, err := service.Enter(callerContext, &corepb.EnterRequest{Reference: reference, Intent: corepb.Intent_READ.Enum()})
		if out != nil || status.Code(err) != codes.InvalidArgument {
			t.Fatalf("oversized request = %v, %v", out, err)
		}
	})
	if calls := owner.snapshotCalls(); len(calls) != 0 {
		t.Fatalf("owner saw rejected requests: %+v", calls)
	}
}

func TestCallMapsEveryPublicFailureAndHidesPrivateErrors(t *testing.T) {
	owner := &entryTestOwner{}
	service, _ := newEntryTestService(owner, 1<<20)
	ctx := grpcsession.WithCaller(context.Background(), entryTestCaller)
	request := &corepb.EnterRequest{Reference: entryTestReference(), Intent: corepb.Intent_READ.Enum()}

	tests := []struct {
		failure values.PublicError
		want    networkpb.Failure
	}{
		{values.PublicInvalid, networkpb.Failure_INVALID},
		{values.PublicDenied, networkpb.Failure_DENIED},
		{values.PublicUnavailable, networkpb.Failure_UNAVAILABLE},
		{values.PublicNotFound, networkpb.Failure_NOT_FOUND},
		{values.PublicGap, networkpb.Failure_GAP},
		{values.PublicInvalidContext, networkpb.Failure_INVALID_CONTEXT},
		{values.PublicResourceExhausted, networkpb.Failure_RESOURCE_EXHAUSTED},
		{values.PublicConflict, networkpb.Failure_CONFLICT},
		{values.PublicIndeterminate, networkpb.Failure_INDETERMINATE},
		{values.PublicOutcomeUnknown, networkpb.Failure_OUTCOME_UNKNOWN},
		{values.PublicUnsupportedType, networkpb.Failure_UNSUPPORTED_TYPE},
	}
	for _, test := range tests {
		t.Run(string(test.failure), func(t *testing.T) {
			owner.setOutcome(nil, test.failure)
			out, err := service.Enter(ctx, request)
			if err != nil || out == nil || out.GetFailure() != test.want || out.GetResult() != nil {
				t.Fatalf("mapped response = %v, error = %v, want %v", out, err, test.want)
			}
		})
	}

	owner.setOutcome(nil, errors.New("private owner failure"))
	out, err := service.Enter(ctx, request)
	if out != nil || status.Code(err) != codes.Internal || status.Convert(err).Message() != "owner result unavailable" {
		t.Fatalf("private owner failure escaped: response = %v, error = %v", out, err)
	}
	owner.setOutcome(nil, values.PublicError("PRIVATE"))
	out, err = service.Enter(ctx, request)
	if out != nil || status.Code(err) != codes.Internal {
		t.Fatalf("unknown public failure escaped: response = %v, error = %v", out, err)
	}
}

func TestDelegatedResultRecheckPrecedesOwnerPublicFailure(t *testing.T) {
	owner := &entryTestOwner{err: values.PublicConflict}
	gate := admission.New(owner, entryTestAuthenticator{caller: entryTestCaller}, admission.Config{
		SessionTTL: time.Minute, MaxSessions: 2, MaxSessionsPerCaller: 2, MaxConcurrentRPCs: 1,
	})
	service := New(owner, gate, Config{
		NextHop: entryTestNextHop, MaxResponseBytes: 128,
		FederationConnections: entryTestFederationConnections(t), IdentityProofs: entryTestIdentityIssuer{},
	})
	ctx := grpcsession.WithGrant(grpcsession.WithCaller(context.Background(), entryTestCaller), [32]byte{9})
	out, err := service.Close(ctx, &corepb.CloseRequest{Handle: entryTestToken(networkpb.TokenKind_SPACE_HANDLE, 1)})
	if out != nil || status.Code(err) != codes.Unauthenticated {
		t.Fatalf("missing delegated grant = %v, %v", out, err)
	}
	calls := owner.snapshotCalls()
	if len(calls) != 1 || calls[0].federation || calls[0].caller != entryTestCaller {
		t.Fatalf("delegated owner call = %+v, want one ordinary call before grant recheck", calls)
	}
}

func TestCallDecodesValidResultsAndEnforcesBothResponseBounds(t *testing.T) {
	const validResult = `{"closed":true}`
	ctx := grpcsession.WithCaller(context.Background(), entryTestCaller)
	request := &corepb.CloseRequest{Handle: entryTestToken(networkpb.TokenKind_SPACE_HANDLE, 1)}

	t.Run("valid", func(t *testing.T) {
		owner := &entryTestOwner{result: []byte(validResult)}
		service, _ := newEntryTestService(owner, 128)
		out, err := service.Close(ctx, request)
		if err != nil || out == nil || out.GetResult() == nil || !out.GetResult().GetClosed() || out.GetFailure() != networkpb.Failure_FAILURE_UNSPECIFIED {
			t.Fatalf("valid result = %v, error = %v", out, err)
		}
	})
	t.Run("canonical bytes", func(t *testing.T) {
		owner := &entryTestOwner{result: []byte(validResult)}
		service, _ := newEntryTestService(owner, len(validResult)-1)
		out, err := service.Close(ctx, request)
		if out != nil || status.Code(err) != codes.ResourceExhausted {
			t.Fatalf("oversized canonical result = %v, %v", out, err)
		}
	})
	t.Run("materialized protobuf", func(t *testing.T) {
		owner := &entryTestOwner{result: []byte(validResult)}
		service, _ := newEntryTestService(owner, 32)
		response := new(corepb.CloseResponse)
		response.ProtoReflect().SetUnknown(bytes.Repeat([]byte{0x98, 0x06, 0x00}, 16))
		err := service.call(ctx, "close", request, response)
		if status.Code(err) != codes.ResourceExhausted {
			t.Fatalf("oversized protobuf error = %v", err)
		}
	})
	t.Run("invalid owner result", func(t *testing.T) {
		owner := &entryTestOwner{result: []byte(`{"closed":"yes"}`)}
		service, _ := newEntryTestService(owner, 128)
		out, err := service.Close(ctx, request)
		if out != nil || status.Code(err) != codes.Internal {
			t.Fatalf("invalid owner result = %v, %v", out, err)
		}
	})
}

type entryTestIdentityIssuer struct{}

func (entryTestIdentityIssuer) Issue(context.Context, values.CallerContext, values.FactAuthorityRef, [32]byte, time.Duration) ([]byte, []byte, error) {
	return []byte{1}, bytes.Repeat([]byte{2}, 64), nil
}

func (entryTestIdentityIssuer) Check(context.Context, []byte, []byte) error { return nil }

func entryTestFederationConnections(t *testing.T) *federationconn.FederationConnections {
	t.Helper()
	connections, err := federationconn.NewFederationConnections(federationconn.FederationConnectionConfig{
		Source: entryTestAuthority,
		Peers: []coresdk.Config{{
			NextHop: entryTestNextHop, Expected: entryTestPeer, Roots: x509.NewCertPool(), SPKISHA256: [32]byte{1},
			RPCDeadline: time.Second, ConnectTimeout: time.Second, MaxResponseBytes: 1 << 20, MaxMetadataBytes: 256,
			Actor: entryTestAuthority, ActorCertificate: &tls.Certificate{Certificate: [][]byte{{1}}, PrivateKey: struct{}{}},
		}},
		MaxParts: 1,
		TTL:      time.Minute,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(connections.Close)
	return connections
}

func entryTestEstablishedContext(t *testing.T, gate *admission.Service) context.Context {
	t.Helper()
	in := &networkpb.EstablishSessionRequest{
		Audience:     proto.String(string(entryTestCaller.Subject.Realm)),
		CredentialId: bytes.Repeat([]byte{1}, 16),
		Secret:       bytes.Repeat([]byte{2}, 32),
	}
	out, err := gate.EstablishSession(context.Background(), in)
	if err != nil || out == nil || len(out.Session) != 32 {
		t.Fatalf("establish session = %v, %v", out, err)
	}
	ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs("authorization", "Bearer "+values.EncodePublicBytes(out.Session)))
	return grpcsession.WithCaller(ctx, entryTestCaller)
}

func TestFederatedCallDoesNotDiscloseOrReviveAReleasedEntrySession(t *testing.T) {
	owner := &entryTestOwner{}
	gate := admission.New(owner, entryTestAuthenticator{caller: entryTestCaller}, admission.Config{
		SessionTTL: time.Minute, MaxSessions: 2, MaxSessionsPerCaller: 2, MaxConcurrentRPCs: 1,
	})
	service := New(owner, gate, Config{
		NextHop: entryTestNextHop, MaxResponseBytes: 128,
		FederationConnections: entryTestFederationConnections(t), IdentityProofs: entryTestIdentityIssuer{},
	})
	ctx := entryTestEstablishedContext(t, gate)
	entered := make(chan struct{}, 1)
	resume := make(chan struct{})
	resumed := false
	releaseOwner := func() {
		if !resumed {
			close(resume)
			resumed = true
		}
	}
	defer releaseOwner()
	owner.setFederatedCall(func(_ context.Context, caller values.CallerContext, _ []byte, connections federationcontract.FederationConnections) ([]byte, error) {
		if caller != entryTestCaller || connections == nil {
			return nil, errors.New("entry did not bind the original caller")
		}
		entered <- struct{}{}
		<-resume
		return []byte(`{"closed":true}`), nil
	})

	type result struct {
		out *corepb.CloseResponse
		err error
	}
	done := make(chan result, 1)
	go func() {
		out, err := service.Close(ctx, &corepb.CloseRequest{Handle: entryTestToken(networkpb.TokenKind_SPACE_HANDLE, 1)})
		done <- result{out: out, err: err}
	}()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("federated owner was not reached")
	}
	if out, err := gate.ReleaseSession(ctx, &networkpb.ReleaseSessionRequest{}); err != nil || out == nil {
		t.Fatalf("release session = %v, %v", out, err)
	}
	releaseOwner()
	var first result
	select {
	case first = <-done:
	case <-time.After(time.Second):
		t.Fatal("federated call did not finish after its owner was released")
	}
	if first.err != nil || first.out == nil || first.out.GetFailure() != networkpb.Failure_DENIED || first.out.GetResult() != nil {
		t.Fatalf("released in-flight disclosure = %v, %v", first.out, first.err)
	}

	second, err := service.Close(ctx, &corepb.CloseRequest{Handle: entryTestToken(networkpb.TokenKind_SPACE_HANDLE, 1)})
	if err != nil || second == nil || second.GetFailure() != networkpb.Failure_DENIED || second.GetResult() != nil {
		t.Fatalf("released session reused = %v, %v", second, err)
	}
	calls := owner.snapshotCalls()
	if len(calls) != 1 || !calls[0].federation {
		t.Fatalf("released session reached owner again: %+v", calls)
	}
}
