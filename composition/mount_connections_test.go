package composition_test

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"ucupa/composition"
	"ucupa/shared/values"
	corecontract "ucupa/ucupa-core/contract"
	core "ucupa/ucupa-core/instance"
)

type mountConnectionPort struct {
	ref      values.FactAuthorityRef
	identity func() values.FactAuthorityRef
	prepare  func(context.Context, values.FactAuthorityRef, corecontract.MountPrepareRequest) (corecontract.MountPrepared, error)
	finish   func(context.Context, values.FactAuthorityRef, corecontract.MountFinishRequest) (corecontract.MountClosed, error)
	read     func(context.Context, values.FactAuthorityRef, corecontract.MountAttemptKey) (corecontract.MountDecisionEvidence, error)
}

func (port *mountConnectionPort) AuthorityRef() values.FactAuthorityRef {
	if port.identity != nil {
		return port.identity()
	}
	return port.ref
}
func (port *mountConnectionPort) Prepare(ctx context.Context, source values.FactAuthorityRef, request corecontract.MountPrepareRequest) (corecontract.MountPrepared, error) {
	return port.prepare(ctx, source, request)
}
func (port *mountConnectionPort) Finish(ctx context.Context, source values.FactAuthorityRef, request corecontract.MountFinishRequest) (corecontract.MountClosed, error) {
	return port.finish(ctx, source, request)
}
func (port *mountConnectionPort) ReadDecision(ctx context.Context, source values.FactAuthorityRef, request corecontract.MountAttemptKey) (corecontract.MountDecisionEvidence, error) {
	return port.read(ctx, source, request)
}

func TestMountConnectionsValidateAndNeverResolveByIdentity(t *testing.T) {
	var connections composition.LocalMountConnections
	var typedNil *mountConnectionPort
	for _, test := range []struct {
		address values.NextHop
		peer    corecontract.MountPeerPort
	}{
		{"invalid", &mountConnectionPort{ref: connectionRefA}},
		{"local:peer", nil},
		{"local:peer", typedNil},
		{"local:peer", &mountConnectionPort{}},
	} {
		if err := connections.BindEndpoint(test.address, test.peer); !errors.Is(err, composition.ErrInvalidCoreEndpoint) {
			t.Fatalf("invalid endpoint accepted: %v", err)
		}
	}
	if _, err := connections.TransportFor(nil); !errors.Is(err, composition.ErrInvalidCoreEndpoint) {
		t.Fatal(err)
	}
	if err := connections.UnbindEndpoint("invalid"); !errors.Is(err, composition.ErrInvalidCoreEndpoint) {
		t.Fatal(err)
	}
	if err := connections.UnbindEndpoint("local:missing"); !errors.Is(err, corecontract.ErrUnavailable) {
		t.Fatal(err)
	}

	source, transport := mountTestTransport(t, &connections)
	_ = source
	identityCalls := 0
	alternate := &mountConnectionPort{identity: func() values.FactAuthorityRef { identityCalls++; return connectionRefA }}
	if err := connections.BindEndpoint("local:alternate", alternate); err != nil {
		t.Fatal(err)
	}
	if err := connections.BindEndpoint("local:wrong", &mountConnectionPort{ref: connectionRefB}); err != nil {
		t.Fatal(err)
	}
	identityCalls = 0
	for _, test := range []struct {
		address values.NextHop
		want    error
	}{
		{"local:missing", corecontract.ErrUnavailable}, {"local:wrong", corecontract.ErrOwnerMismatch},
	} {
		prepared, err := transport.Prepare(context.Background(), test.address, connectionRefA, corecontract.MountPrepareRequest{})
		if !errors.Is(err, test.want) || prepared != (corecontract.MountPrepared{}) {
			t.Fatalf("Prepare: %v, %v", prepared, err)
		}
		closed, err := transport.Finish(context.Background(), test.address, connectionRefA, corecontract.MountFinishRequest{})
		if !errors.Is(err, test.want) || closed != (corecontract.MountClosed{}) {
			t.Fatalf("Finish: %v, %v", closed, err)
		}
		decision, err := transport.ReadDecision(context.Background(), test.address, connectionRefA, corecontract.MountAttemptKey{})
		if !errors.Is(err, test.want) || decision != (corecontract.MountDecisionEvidence{}) {
			t.Fatalf("ReadDecision: %v, %v", decision, err)
		}
	}
	if identityCalls != 0 {
		t.Fatal("transport scanned a different address")
	}
}

func TestMountConnectionsBindSourceAndForwardExactMessages(t *testing.T) {
	var connections composition.LocalMountConnections
	source, transport := mountTestTransport(t, &connections)
	// Even a payload naming a different source cannot change the authenticated
	// local port context. The real peer, not this transport, rejects bad content.
	request := corecontract.MountPrepareRequest{Key: corecontract.MountKey{ParentAuthority: connectionRefB}, OperationID: "prepare"}
	finishRequest := corecontract.MountFinishRequest{Evidence: corecontract.MountDecisionEvidence{Request: request, Decision: corecontract.MountCommitted}}
	attempt := corecontract.MountAttemptKey{ParentAuthority: connectionRefB, OperationID: "read"}
	ctx := context.Background()
	var peerErr error
	peer := &mountConnectionPort{ref: connectionRefA}
	peer.prepare = func(gotContext context.Context, gotSource values.FactAuthorityRef, got corecontract.MountPrepareRequest) (corecontract.MountPrepared, error) {
		if gotContext != ctx || gotSource != source.AuthorityRef() || got != request {
			t.Fatal("Prepare changed source or request")
		}
		return corecontract.MountPrepared{Request: request, Prepared: true}, peerErr
	}
	peer.finish = func(gotContext context.Context, gotSource values.FactAuthorityRef, got corecontract.MountFinishRequest) (corecontract.MountClosed, error) {
		if gotContext != ctx || gotSource != source.AuthorityRef() || got != finishRequest {
			t.Fatal("Finish changed source or request")
		}
		return corecontract.MountClosed{Evidence: finishRequest.Evidence}, peerErr
	}
	peer.read = func(gotContext context.Context, gotSource values.FactAuthorityRef, got corecontract.MountAttemptKey) (corecontract.MountDecisionEvidence, error) {
		if gotContext != ctx || gotSource != source.AuthorityRef() || got != attempt {
			t.Fatal("ReadDecision changed source or request")
		}
		return finishRequest.Evidence, peerErr
	}
	if err := connections.BindEndpoint("local:peer", peer); err != nil {
		t.Fatal(err)
	}
	for _, peerErr = range []error{nil, corecontract.ErrDenied, corecontract.ErrStore} {
		prepared, err := transport.Prepare(ctx, "local:peer", connectionRefA, request)
		if !errors.Is(err, peerErr) {
			t.Fatal(err)
		}
		closed, err := transport.Finish(ctx, "local:peer", connectionRefA, finishRequest)
		if !errors.Is(err, peerErr) {
			t.Fatal(err)
		}
		decision, err := transport.ReadDecision(ctx, "local:peer", connectionRefA, attempt)
		if !errors.Is(err, peerErr) {
			t.Fatal(err)
		}
		if peerErr == nil {
			if !prepared.Prepared || prepared.Request != request || closed.Evidence != finishRequest.Evidence || decision != finishRequest.Evidence {
				t.Fatal("successful response changed")
			}
		} else if prepared != (corecontract.MountPrepared{}) || closed != (corecontract.MountClosed{}) || decision != (corecontract.MountDecisionEvidence{}) {
			t.Fatal("failed response disclosed payload")
		}
	}
}

func TestMountConnectionsCheckIdentityAfterEveryCall(t *testing.T) {
	for _, method := range []string{"prepare", "finish", "read"} {
		t.Run(method, func(t *testing.T) {
			var connections composition.LocalMountConnections
			_, transport := mountTestTransport(t, &connections)
			peer := &mountConnectionPort{ref: connectionRefA}
			peer.prepare = func(context.Context, values.FactAuthorityRef, corecontract.MountPrepareRequest) (corecontract.MountPrepared, error) {
				peer.ref = connectionRefB
				return corecontract.MountPrepared{Prepared: true}, corecontract.ErrStore
			}
			peer.finish = func(context.Context, values.FactAuthorityRef, corecontract.MountFinishRequest) (corecontract.MountClosed, error) {
				peer.ref = connectionRefB
				return corecontract.MountClosed{Evidence: corecontract.MountDecisionEvidence{Decision: corecontract.MountCommitted}}, corecontract.ErrStore
			}
			peer.read = func(context.Context, values.FactAuthorityRef, corecontract.MountAttemptKey) (corecontract.MountDecisionEvidence, error) {
				peer.ref = connectionRefB
				return corecontract.MountDecisionEvidence{Decision: corecontract.MountCommitted}, corecontract.ErrStore
			}
			if err := connections.BindEndpoint("local:peer", peer); err != nil {
				t.Fatal(err)
			}
			var result any
			var err error
			switch method {
			case "prepare":
				result, err = transport.Prepare(context.Background(), "local:peer", connectionRefA, corecontract.MountPrepareRequest{})
			case "finish":
				result, err = transport.Finish(context.Background(), "local:peer", connectionRefA, corecontract.MountFinishRequest{})
			case "read":
				result, err = transport.ReadDecision(context.Background(), "local:peer", connectionRefA, corecontract.MountAttemptKey{})
			}
			if !errors.Is(err, corecontract.ErrOwnerMismatch) || !errors.Is(err, corecontract.ErrStore) || !reflect.ValueOf(result).IsZero() {
				t.Fatalf("identity changed: %v, %v", result, err)
			}
		})
	}
}

func TestMountConnectionsKeepCapturedPortWithoutHoldingConnectionLock(t *testing.T) {
	var connections composition.LocalMountConnections
	_, transport := mountTestTransport(t, &connections)
	replacement := &mountConnectionPort{ref: connectionRefB}
	peer := &mountConnectionPort{ref: connectionRefA}
	peer.prepare = func(context.Context, values.FactAuthorityRef, corecontract.MountPrepareRequest) (corecontract.MountPrepared, error) {
		return corecontract.MountPrepared{Prepared: true}, connections.BindEndpoint("local:peer", replacement)
	}
	if err := connections.BindEndpoint("local:peer", peer); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		got, err := transport.Prepare(context.Background(), "local:peer", connectionRefA, corecontract.MountPrepareRequest{})
		if err == nil && !got.Prepared {
			err = errors.New("captured peer response lost")
		}
		done <- err
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("connection lock held across peer call")
	}
	if _, err := transport.Prepare(context.Background(), "local:peer", connectionRefA, corecontract.MountPrepareRequest{}); !errors.Is(err, corecontract.ErrOwnerMismatch) {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := transport.Prepare(ctx, "local:peer", connectionRefA, corecontract.MountPrepareRequest{}); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}

func mountTestTransport(t *testing.T, connections *composition.LocalMountConnections) (*core.Core, corecontract.MountTransport) {
	t.Helper()
	source, err := core.New(connectionContacts{}, connectionContact, corecontract.Config{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = source.Close() })
	transport, err := connections.TransportFor(source)
	if err != nil {
		t.Fatal(err)
	}
	return source, transport
}
