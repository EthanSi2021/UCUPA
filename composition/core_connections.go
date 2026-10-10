package composition

import (
	"context"
	"errors"
	"reflect"
	"sync"

	"ucupa/shared/values"
	corecontract "ucupa/ucupa-core/contract"
)

var (
	ErrInvalidCoreEndpoint     = errors.New("invalid local Core endpoint")
	ErrCoreEndpointUnavailable = errors.New("local Core endpoint unavailable")
	ErrCoreTargetMismatch      = errors.New("local Core target identity mismatch")
)

// CoreReadPort is the existing authorized local read boundary. It exposes no
// Store, mutation entry point, or authorization-free Element lookup.
type CoreReadPort interface {
	AuthorityRef() values.FactAuthorityRef
	ReadElement(values.PrincipalRef, values.ElementID, *corecontract.CoreReadCut) (corecontract.ElementRead, error)
}

// CorePublicPort is the optional public-envelope entry on the same fact owner.
// It is assembled explicitly, not discovered through a large Core receiver.
type CorePublicPort interface {
	AuthorityRef() values.FactAuthorityRef
	PublicCall(context.Context, values.CallerContext, []byte) ([]byte, error)
}

// CoreEndpoint groups the independently located read and public entries. They
// are captured and replaced together, and must identify the same owner.
type CoreEndpoint struct {
	Read   CoreReadPort
	Public CorePublicPort
}

// LocalCoreConnections binds specified addresses to trusted local read ports.
// Its zero value is ready to use. It owns no identities or persistent contact
// records and performs no discovery, fallback, or remote authentication.
type LocalCoreConnections struct {
	mu        sync.RWMutex
	endpoints map[values.NextHop]CoreEndpoint
}

// BindEndpoint explicitly binds or replaces one address. An in-flight read
// retains the port it already selected, even if this address is rebound.
func (connections *LocalCoreConnections) BindEndpoint(address values.NextHop, endpoint CoreEndpoint) error {
	if connections == nil || !address.IsLocal() || nilCorePort(endpoint.Read) {
		return ErrInvalidCoreEndpoint
	}
	identity := endpoint.Read.AuthorityRef()
	if !identity.Valid() || endpoint.Public != nil && (nilCorePort(endpoint.Public) || endpoint.Public.AuthorityRef() != identity) {
		return ErrInvalidCoreEndpoint
	}
	connections.mu.Lock()
	defer connections.mu.Unlock()
	if connections.endpoints == nil {
		connections.endpoints = make(map[values.NextHop]CoreEndpoint)
	}
	connections.endpoints[address] = endpoint
	return nil
}

// UnbindEndpoint removes only the named address, without closing its peer or
// cancelling reads that have already captured that peer.
func (connections *LocalCoreConnections) UnbindEndpoint(address values.NextHop) error {
	if connections == nil || !address.IsLocal() {
		return ErrInvalidCoreEndpoint
	}
	connections.mu.Lock()
	defer connections.mu.Unlock()
	if _, exists := connections.endpoints[address]; !exists {
		return ErrCoreEndpointUnavailable
	}
	delete(connections.endpoints, address)
	return nil
}

// ReadElement calls only the specified address and forwards the exact caller,
// Element, and optional read cut to the existing authorized Core operation.
// Identity is checked on the same captured port before and after the call;
// identity equality is not proof of remote authentication or writer fencing.
func (connections *LocalCoreConnections) ReadElement(
	address values.NextHop,
	expectedRef values.FactAuthorityRef,
	actor values.PrincipalRef,
	id values.ElementID,
	at *corecontract.CoreReadCut,
) (corecontract.ElementRead, error) {
	if connections == nil || !address.IsLocal() || !expectedRef.Valid() {
		return corecontract.ElementRead{}, ErrInvalidCoreEndpoint
	}
	connections.mu.RLock()
	endpoint, exists := connections.endpoints[address]
	connections.mu.RUnlock()
	if !exists {
		return corecontract.ElementRead{}, ErrCoreEndpointUnavailable
	}
	peer := endpoint.Read
	if peer.AuthorityRef() != expectedRef {
		return corecontract.ElementRead{}, ErrCoreTargetMismatch
	}
	result, err := peer.ReadElement(actor, id, at)
	if peer.AuthorityRef() != expectedRef {
		return corecontract.ElementRead{}, errors.Join(ErrCoreTargetMismatch, err)
	}
	if err != nil {
		return corecontract.ElementRead{}, err
	}
	return result, nil
}

func nilCorePort(peer interface {
	AuthorityRef() values.FactAuthorityRef
}) bool {
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
