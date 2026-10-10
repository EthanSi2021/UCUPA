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
	peercontract "ucupa/ucupa-core/contract/connected"
	core "ucupa/ucupa-core/instance"
)

type connectedConnectionPort struct {
	ref      values.FactAuthorityRef
	identity func() values.FactAuthorityRef
	read     func(context.Context, values.FactAuthorityRef, peercontract.Request) (peercontract.Reply, error)
}

func (port *connectedConnectionPort) AuthorityRef() values.FactAuthorityRef {
	if port.identity != nil {
		return port.identity()
	}
	return port.ref
}

func (port *connectedConnectionPort) ReadConnected(ctx context.Context, source values.FactAuthorityRef, request peercontract.Request) (peercontract.Reply, error) {
	return port.read(ctx, source, request)
}

func TestConnectedConnectionsValidateAndUseOnlySpecifiedAddress(t *testing.T) {
	var connections composition.LocalConnectedConnections
	var typedNil *connectedConnectionPort
	for _, test := range []struct {
		address values.NextHop
		peer    peercontract.PeerPort
	}{
		{"invalid", &connectedConnectionPort{ref: connectionRefA}},
		{"local:peer", nil}, {"local:peer", typedNil},
		{"local:peer", &connectedConnectionPort{}},
	} {
		if err := connections.BindEndpoint(test.address, test.peer); !errors.Is(err, composition.ErrInvalidCoreEndpoint) {
			t.Fatalf("invalid endpoint accepted: %v", err)
		}
	}
	for _, source := range []*core.Core{nil, {}} {
		if _, err := connections.TransportFor(source); !errors.Is(err, composition.ErrInvalidCoreEndpoint) {
			t.Fatalf("invalid source accepted: %v", err)
		}
	}
	if err := connections.UnbindEndpoint("invalid"); !errors.Is(err, composition.ErrInvalidCoreEndpoint) {
		t.Fatal(err)
	}
	if err := connections.UnbindEndpoint("local:missing"); !errors.Is(err, corecontract.ErrUnavailable) {
		t.Fatal(err)
	}
	_, transport := connectedTestTransport(t, &connections)
	identityCalls := 0
	alternate := &connectedConnectionPort{identity: func() values.FactAuthorityRef { identityCalls++; return connectionRefA }}
	if err := connections.BindEndpoint("local:alternate", alternate); err != nil {
		t.Fatal(err)
	}
	if err := connections.BindEndpoint("local:wrong", &connectedConnectionPort{ref: connectionRefB}); err != nil {
		t.Fatal(err)
	}
	identityCalls = 0
	for _, test := range []struct {
		address values.NextHop
		want    error
	}{{"local:missing", corecontract.ErrUnavailable}, {"local:wrong", corecontract.ErrOwnerMismatch}, {"invalid", corecontract.ErrInvalid}} {
		got, err := transport.ReadConnected(context.Background(), test.address, connectionRefA, peercontract.Request{})
		if !errors.Is(err, test.want) || !reflect.ValueOf(got).IsZero() {
			t.Fatalf("ReadConnected(%s): %v, %v", test.address, got, err)
		}
	}
	if identityCalls != 0 {
		t.Fatal("transport scanned another address to find expected identity")
	}
	if _, err := transport.ReadConnected(nil, "local:wrong", connectionRefA, peercontract.Request{}); !errors.Is(err, corecontract.ErrInvalid) {
		t.Fatal(err)
	}
}

func TestConnectedConnectionsFixSourceAndPreservePeerErrors(t *testing.T) {
	var connections composition.LocalConnectedConnections
	source, transport := connectedTestTransport(t, &connections)
	ctx := context.Background()
	request := peercontract.Request{}
	var peerError error
	peer := &connectedConnectionPort{ref: connectionRefA}
	peer.read = func(gotContext context.Context, gotSource values.FactAuthorityRef, got peercontract.Request) (peercontract.Reply, error) {
		if gotContext != ctx || gotSource != source.AuthorityRef() || !reflect.DeepEqual(got, request) {
			t.Fatal("changed trusted source, context, or opaque request")
		}
		return peercontract.Reply{}, peerError
	}
	if err := connections.BindEndpoint("local:peer", peer); err != nil {
		t.Fatal(err)
	}
	for _, peerError = range []error{nil, corecontract.ErrDenied, corecontract.ErrStore, corecontract.ErrReadCutExpired} {
		got, err := transport.ReadConnected(ctx, "local:peer", connectionRefA, request)
		if !errors.Is(err, peerError) || !reflect.ValueOf(got).IsZero() {
			t.Fatalf("response or error changed: %v, %v", got, err)
		}
	}
}

func TestConnectedConnectionsRecheckCapturedPeerIdentity(t *testing.T) {
	var connections composition.LocalConnectedConnections
	_, transport := connectedTestTransport(t, &connections)
	peer := &connectedConnectionPort{ref: connectionRefA}
	peer.read = func(context.Context, values.FactAuthorityRef, peercontract.Request) (peercontract.Reply, error) {
		peer.ref = connectionRefB
		return peercontract.Reply{}, corecontract.ErrStore
	}
	if err := connections.BindEndpoint("local:peer", peer); err != nil {
		t.Fatal(err)
	}
	got, err := transport.ReadConnected(context.Background(), "local:peer", connectionRefA, peercontract.Request{})
	if !errors.Is(err, corecontract.ErrOwnerMismatch) || !errors.Is(err, corecontract.ErrStore) || !reflect.ValueOf(got).IsZero() {
		t.Fatalf("post-call identity mismatch lost: %v, %v", got, err)
	}
}

func TestConnectedConnectionsRebindDoesNotRedirectInFlightCall(t *testing.T) {
	var connections composition.LocalConnectedConnections
	_, transport := connectedTestTransport(t, &connections)
	peer := &connectedConnectionPort{ref: connectionRefA}
	peer.read = func(context.Context, values.FactAuthorityRef, peercontract.Request) (peercontract.Reply, error) {
		return peercontract.Reply{}, connections.BindEndpoint("local:peer", &connectedConnectionPort{ref: connectionRefB})
	}
	if err := connections.BindEndpoint("local:peer", peer); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		_, err := transport.ReadConnected(context.Background(), "local:peer", connectionRefA, peercontract.Request{})
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
	if _, err := transport.ReadConnected(context.Background(), "local:peer", connectionRefA, peercontract.Request{}); !errors.Is(err, corecontract.ErrOwnerMismatch) {
		t.Fatalf("later call followed captured old peer: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := transport.ReadConnected(ctx, "local:peer", connectionRefA, peercontract.Request{}); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if err := connections.UnbindEndpoint("local:peer"); err != nil {
		t.Fatal(err)
	}
	if _, err := transport.ReadConnected(context.Background(), "local:peer", connectionRefB, peercontract.Request{}); !errors.Is(err, corecontract.ErrUnavailable) {
		t.Fatal(err)
	}
}

func connectedTestTransport(t *testing.T, connections *composition.LocalConnectedConnections) (*core.Core, peercontract.Transport) {
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
