package composition

import (
	"context"
	"errors"
	"math"
	"sync"
	"time"

	"ucupa/shared/values"
	corecontract "ucupa/ucupa-core/contract"
	facetcontract "ucupa/ucupa-facet/contract"
	realm "ucupa/ucupa-realm/contract"
	realminstance "ucupa/ucupa-realm/instance"
)

var (
	_ corecontract.RealmConnections  = (*LocalRealmConnections)(nil)
	_ facetcontract.RealmConnections = (*LocalRealmConnections)(nil)
)

var (
	ErrInvalidRealmEndpoint     = errors.New("invalid local Realm endpoint")
	ErrRealmEndpointUnavailable = errors.New("local Realm endpoint unavailable")
	ErrRealmTargetMismatch      = errors.New("local Realm target identity mismatch")
)

type realmEndpoint struct {
	peer   *realminstance.Realm
	serial uint64
}

// LocalRealmConnections is a trusted local address binding, not a RealmID
// directory. It owns neither authorization facts nor the owner's durable
// contact. Every use names one address and the expected Realm identity.
type LocalRealmConnections struct {
	mu        sync.RWMutex
	endpoints map[values.NextHop]realmEndpoint
	serial    uint64
}

func (connections *LocalRealmConnections) BindEndpoint(address values.NextHop, peer *realminstance.Realm) error {
	if connections == nil || !address.IsLocal() || peer == nil {
		return ErrInvalidRealmEndpoint
	}
	id, active := peer.ActiveID()
	if !active || id == "" {
		return ErrInvalidRealmEndpoint
	}
	connections.mu.Lock()
	defer connections.mu.Unlock()
	if connections.endpoints == nil {
		connections.endpoints = make(map[values.NextHop]realmEndpoint)
	}
	if connections.serial == math.MaxUint64 {
		return ErrInvalidRealmEndpoint
	}
	connections.serial++
	connections.endpoints[address] = realmEndpoint{peer: peer, serial: connections.serial}
	return nil
}

func (connections *LocalRealmConnections) UnbindEndpoint(address values.NextHop) error {
	if connections == nil || !address.IsLocal() {
		return ErrInvalidRealmEndpoint
	}
	connections.mu.Lock()
	defer connections.mu.Unlock()
	if _, exists := connections.endpoints[address]; !exists {
		return ErrRealmEndpointUnavailable
	}
	delete(connections.endpoints, address)
	return nil
}

func (connections *LocalRealmConnections) selectEndpoint(address values.NextHop, expected values.RealmID) (realmEndpoint, error) {
	if connections == nil || !address.IsLocal() || expected == "" {
		return realmEndpoint{}, ErrInvalidRealmEndpoint
	}
	connections.mu.RLock()
	endpoint, exists := connections.endpoints[address]
	connections.mu.RUnlock()
	if !exists {
		return realmEndpoint{}, ErrRealmEndpointUnavailable
	}
	id, active := endpoint.peer.ActiveID()
	if !active {
		return realmEndpoint{}, ErrRealmEndpointUnavailable
	}
	if id != expected {
		return realmEndpoint{}, ErrRealmTargetMismatch
	}
	return endpoint, nil
}

// stillBound prevents a result from an endpoint removed or replaced during
// the call from being used as current authorization evidence.
func (connections *LocalRealmConnections) stillBound(address values.NextHop, expected values.RealmID, endpoint realmEndpoint) bool {
	connections.mu.RLock()
	current, exists := connections.endpoints[address]
	connections.mu.RUnlock()
	id, active := endpoint.peer.ActiveID()
	return exists && current.serial == endpoint.serial && current.peer == endpoint.peer && active && id == expected
}

// Check is for trusted owner initialization and open. A matching ID is an
// assembly check; it is not proof that an arbitrary untrusted object is safe.
// The local profile performs bounded in-process work only.
func (connections *LocalRealmConnections) Check(ctx context.Context, address values.NextHop, expected values.RealmID) error {
	_ = ctx
	endpoint, err := connections.selectEndpoint(address, expected)
	if err != nil {
		return err
	}
	if !connections.stillBound(address, expected, endpoint) {
		return ErrRealmEndpointUnavailable
	}
	return nil
}

func (connections *LocalRealmConnections) AuthorizePrincipal(address values.NextHop, expected values.RealmID, actor values.PrincipalRef, requirements ...values.Requirement) values.AuthorizationDecision {
	endpoint, err := connections.selectEndpoint(address, expected)
	if err != nil {
		return values.AuthorizationUnavailable
	}
	decision := endpoint.peer.AuthorizePrincipal(actor, requirements...)
	if !connections.stillBound(address, expected, endpoint) {
		return values.AuthorizationUnavailable
	}
	return decision
}

func (connections *LocalRealmConnections) AuthorizeView(ctx context.Context, address values.NextHop, expected values.RealmID, caller values.CallerContext, requirements ...values.Requirement) (values.AuthorizationDecision, values.AuthorizationView) {
	_ = ctx
	endpoint, err := connections.selectEndpoint(address, expected)
	if err != nil {
		return values.AuthorizationUnavailable, values.AuthorizationView{}
	}
	decision, view := endpoint.peer.AuthorizeView(caller, requirements...)
	if !connections.stillBound(address, expected, endpoint) {
		return values.AuthorizationUnavailable, values.AuthorizationView{}
	}
	return decision, view
}

func (connections *LocalRealmConnections) ValidateView(ctx context.Context, address values.NextHop, expected values.RealmID, caller values.CallerContext, view values.AuthorizationView, requirements ...values.Requirement) values.AuthorizationDecision {
	_ = ctx
	endpoint, err := connections.selectEndpoint(address, expected)
	if err != nil {
		return values.AuthorizationUnavailable
	}
	decision := endpoint.peer.ValidateView(caller, view, requirements...)
	if !connections.stillBound(address, expected, endpoint) {
		return values.AuthorizationUnavailable
	}
	return decision
}

func (connections *LocalRealmConnections) CaptureDenialView(ctx context.Context, address values.NextHop, expected values.RealmID, caller values.CallerContext, requirement values.Requirement) (values.AuthorizationDecision, values.AuthorizationDenialView) {
	_ = ctx
	endpoint, err := connections.selectEndpoint(address, expected)
	if err != nil {
		return values.AuthorizationUnavailable, values.AuthorizationDenialView{}
	}
	decision, view := endpoint.peer.CaptureDenialView(caller, requirement)
	if !connections.stillBound(address, expected, endpoint) {
		return values.AuthorizationUnavailable, values.AuthorizationDenialView{}
	}
	return decision, view
}

func (connections *LocalRealmConnections) ValidateDenialView(ctx context.Context, address values.NextHop, expected values.RealmID, caller values.CallerContext, view values.AuthorizationDenialView, requirement values.Requirement) values.AuthorizationDecision {
	_ = ctx
	endpoint, err := connections.selectEndpoint(address, expected)
	if err != nil {
		return values.AuthorizationUnavailable
	}
	decision := endpoint.peer.ValidateDenialView(caller, view, requirement)
	if !connections.stillBound(address, expected, endpoint) {
		return values.AuthorizationUnavailable
	}
	return decision
}

func (connections *LocalRealmConnections) EstablishCaller(ctx context.Context, address values.NextHop, expected values.RealmID, evidence realm.CredentialEvidence) (values.CallerContext, error) {
	_ = ctx
	endpoint, err := connections.selectEndpoint(address, expected)
	if err != nil {
		return values.CallerContext{}, err
	}
	caller, err := endpoint.peer.EstablishCaller(evidence)
	if !connections.stillBound(address, expected, endpoint) {
		return values.CallerContext{}, ErrRealmEndpointUnavailable
	}
	return caller, err
}

func (connections *LocalRealmConnections) ForkCaller(ctx context.Context, address values.NextHop, expected values.RealmID, caller values.CallerContext) (values.CallerContext, error) {
	endpoint, err := connections.selectEndpoint(address, expected)
	if err != nil {
		return values.CallerContext{}, err
	}
	fork, err := endpoint.peer.ForkCaller(ctx, caller)
	if !connections.stillBound(address, expected, endpoint) {
		return values.CallerContext{}, ErrRealmEndpointUnavailable
	}
	return fork, err
}

func (connections *LocalRealmConnections) TrustLocalCaller(address values.NextHop, expected values.RealmID, subject values.PrincipalRef) (values.CallerContext, error) {
	endpoint, err := connections.selectEndpoint(address, expected)
	if err != nil {
		return values.CallerContext{}, err
	}
	caller, err := endpoint.peer.TrustLocalCaller(subject)
	if !connections.stillBound(address, expected, endpoint) {
		return values.CallerContext{}, ErrRealmEndpointUnavailable
	}
	return caller, err
}

// EstablishForeignCaller has no local counterpart: the local profile's foreign
// identity chain is verified by trusted assembly and established through
// TrustLocalCaller, and no local Realm endpoint accepts a proof-shaped RPC.
// The local profile fails closed.
func (connections *LocalRealmConnections) EstablishForeignCaller(context.Context, values.NextHop, values.RealmID, values.FactAuthorityRef, []byte, []byte, [32]byte) (values.CallerContext, time.Time, error) {
	return values.CallerContext{}, time.Time{}, realm.ErrUnauthenticated
}
