package composition

import (
	"context"
	"errors"
	"reflect"
	"sync"

	"ucupa/shared/values"
	corecontract "ucupa/ucupa-core/contract"
	core "ucupa/ucupa-core/instance"
)

// LocalMountConnections is trusted, process-local assembly for mount control.
// It binds specified addresses to restricted peer ports, not raw Core stores.
// It owns no contact facts, discovery, authentication, or Ref-to-address lookup.
// The zero value is ready to use.
type LocalMountConnections struct {
	mu        sync.RWMutex
	endpoints map[values.NextHop]corecontract.MountPeerPort
}

// BindEndpoint installs or replaces precisely one local address. Calls already
// in flight keep their captured port; a replacement only affects later calls.
func (connections *LocalMountConnections) BindEndpoint(address values.NextHop, peer corecontract.MountPeerPort) error {
	if connections == nil || !address.IsLocal() || nilMountPeer(peer) || !peer.AuthorityRef().Valid() {
		return ErrInvalidCoreEndpoint
	}
	connections.mu.Lock()
	defer connections.mu.Unlock()
	if connections.endpoints == nil {
		connections.endpoints = make(map[values.NextHop]corecontract.MountPeerPort)
	}
	connections.endpoints[address] = peer
	return nil
}

func (connections *LocalMountConnections) UnbindEndpoint(address values.NextHop) error {
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

// TransportFor binds the caller identity to a concrete local Core, independently
// of outgoing message fields. Only trusted assembly configures these objects;
// this is not a network authentication boundary or an application capability.
func (connections *LocalMountConnections) TransportFor(source *core.Core) (corecontract.MountTransport, error) {
	if connections == nil || source == nil || !source.AuthorityRef().Valid() {
		return nil, ErrInvalidCoreEndpoint
	}
	return localMountTransport{connections: connections, source: source, sourceRef: source.AuthorityRef()}, nil
}

type localMountTransport struct {
	connections *LocalMountConnections
	source      *core.Core
	sourceRef   values.FactAuthorityRef
}

func (transport localMountTransport) peer(ctx context.Context, address values.NextHop, expected values.FactAuthorityRef) (corecontract.MountPeerPort, error) {
	if ctx == nil || !address.IsLocal() || !expected.Valid() {
		return nil, corecontract.ErrInvalid
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if transport.source.AuthorityRef() != transport.sourceRef {
		return nil, corecontract.ErrOwnerMismatch
	}
	transport.connections.mu.RLock()
	peer, exists := transport.connections.endpoints[address]
	transport.connections.mu.RUnlock()
	if !exists {
		return nil, corecontract.ErrUnavailable
	}
	if peer.AuthorityRef() != expected {
		return nil, corecontract.ErrOwnerMismatch
	}
	return peer, nil
}

func (transport localMountTransport) Prepare(ctx context.Context, address values.NextHop, expected values.FactAuthorityRef, request corecontract.MountPrepareRequest) (corecontract.MountPrepared, error) {
	peer, err := transport.peer(ctx, address, expected)
	if err != nil {
		return corecontract.MountPrepared{}, err
	}
	result, err := peer.Prepare(ctx, transport.sourceRef, request)
	if peer.AuthorityRef() != expected {
		return corecontract.MountPrepared{}, errors.Join(corecontract.ErrOwnerMismatch, err)
	}
	if err != nil {
		return corecontract.MountPrepared{}, err
	}
	return result, nil
}

func (transport localMountTransport) Finish(ctx context.Context, address values.NextHop, expected values.FactAuthorityRef, request corecontract.MountFinishRequest) (corecontract.MountClosed, error) {
	peer, err := transport.peer(ctx, address, expected)
	if err != nil {
		return corecontract.MountClosed{}, err
	}
	result, err := peer.Finish(ctx, transport.sourceRef, request)
	if peer.AuthorityRef() != expected {
		return corecontract.MountClosed{}, errors.Join(corecontract.ErrOwnerMismatch, err)
	}
	if err != nil {
		return corecontract.MountClosed{}, err
	}
	return result, nil
}

func (transport localMountTransport) ReadDecision(ctx context.Context, address values.NextHop, expected values.FactAuthorityRef, attempt corecontract.MountAttemptKey) (corecontract.MountDecisionEvidence, error) {
	peer, err := transport.peer(ctx, address, expected)
	if err != nil {
		return corecontract.MountDecisionEvidence{}, err
	}
	result, err := peer.ReadDecision(ctx, transport.sourceRef, attempt)
	if peer.AuthorityRef() != expected {
		return corecontract.MountDecisionEvidence{}, errors.Join(corecontract.ErrOwnerMismatch, err)
	}
	if err != nil {
		return corecontract.MountDecisionEvidence{}, err
	}
	return result, nil
}

func nilMountPeer(peer corecontract.MountPeerPort) bool {
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
