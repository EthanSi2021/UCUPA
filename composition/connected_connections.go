package composition

import (
	"context"
	"errors"
	"reflect"
	"sync"

	"ucupa/shared/values"
	corecontract "ucupa/ucupa-core/contract"
	peercontract "ucupa/ucupa-core/contract/connected"
	core "ucupa/ucupa-core/instance"
)

// LocalConnectedConnections connects restricted, opaque Core read ports at
// explicitly configured addresses. It owns neither spatial state nor contact
// facts, and provides no Ref lookup, fallback, or network authentication.
// The zero value is ready to use.
type LocalConnectedConnections struct {
	mu        sync.RWMutex
	endpoints map[values.NextHop]peercontract.PeerPort
}

// BindEndpoint replaces one address. In-flight calls retain their captured
// peer; rebinding does not redirect those calls or change their expected owner.
func (connections *LocalConnectedConnections) BindEndpoint(address values.NextHop, peer peercontract.PeerPort) error {
	if connections == nil || !address.IsLocal() || nilConnectedPeer(peer) || !peer.AuthorityRef().Valid() {
		return ErrInvalidCoreEndpoint
	}
	connections.mu.Lock()
	defer connections.mu.Unlock()
	if connections.endpoints == nil {
		connections.endpoints = make(map[values.NextHop]peercontract.PeerPort)
	}
	connections.endpoints[address] = peer
	return nil
}

func (connections *LocalConnectedConnections) UnbindEndpoint(address values.NextHop) error {
	if connections == nil || !address.IsLocal() {
		return ErrInvalidCoreEndpoint
	}
	connections.mu.Lock()
	defer connections.mu.Unlock()
	if _, exists := connections.endpoints[address]; !exists {
		return corecontract.ErrUnavailable
	}
	delete(connections.endpoints, address)
	return nil
}

// TransportFor establishes the trusted sending Core independently of request
// fields. Only process-local assembly may install these transports; callers'
// permissions remain independently checked by each receiving Core.
func (connections *LocalConnectedConnections) TransportFor(source *core.Core) (peercontract.Transport, error) {
	if connections == nil || source == nil || !source.AuthorityRef().Valid() {
		return nil, ErrInvalidCoreEndpoint
	}
	return localConnectedTransport{connections: connections, source: source, sourceRef: source.AuthorityRef()}, nil
}

type localConnectedTransport struct {
	connections *LocalConnectedConnections
	source      *core.Core
	sourceRef   values.FactAuthorityRef
}

func (transport localConnectedTransport) ReadConnected(
	ctx context.Context,
	address values.NextHop,
	expected values.FactAuthorityRef,
	request peercontract.Request,
) (peercontract.Reply, error) {
	if ctx == nil || !address.IsLocal() || !expected.Valid() {
		return peercontract.Reply{}, corecontract.ErrInvalid
	}
	if err := ctx.Err(); err != nil {
		return peercontract.Reply{}, err
	}
	if transport.source.AuthorityRef() != transport.sourceRef {
		return peercontract.Reply{}, corecontract.ErrOwnerMismatch
	}
	transport.connections.mu.RLock()
	peer, exists := transport.connections.endpoints[address]
	transport.connections.mu.RUnlock()
	if !exists {
		return peercontract.Reply{}, corecontract.ErrUnavailable
	}
	if peer.AuthorityRef() != expected {
		return peercontract.Reply{}, corecontract.ErrOwnerMismatch
	}
	result, err := peer.ReadConnected(ctx, transport.sourceRef, request)
	if peer.AuthorityRef() != expected || transport.source.AuthorityRef() != transport.sourceRef {
		return peercontract.Reply{}, errors.Join(corecontract.ErrOwnerMismatch, err)
	}
	if err != nil {
		return peercontract.Reply{}, err
	}
	return result, nil
}

func nilConnectedPeer(peer peercontract.PeerPort) bool {
	if peer == nil {
		return true
	}
	value := reflect.ValueOf(peer)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return value.IsNil()
	default:
		return false
	}
}
