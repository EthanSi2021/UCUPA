package composition

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"math"
	"sync"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"ucupa/shared/values"
	corecontract "ucupa/ucupa-core/contract"
	facetcontract "ucupa/ucupa-facet/contract"
	realmsdk "ucupa/ucupa-realm/sdk/go"
)

var (
	_ corecontract.RealmConnections  = (*NetworkRealmConnections)(nil)
	_ facetcontract.RealmConnections = (*NetworkRealmConnections)(nil)
)

var (
	ErrInvalidNetworkRealmEndpoint = errors.New("invalid network Realm endpoint")
	ErrNetworkRealmUnavailable     = errors.New("network Realm endpoint unavailable")
	ErrNetworkRealmTargetMismatch  = errors.New("network Realm target not verified")
)

// NetworkRealmEndpointConfig is the pinned material for one network Realm
// address: TLS 1.3 roots, the SPKI pin, the expected Realm identity and the
// service certificate the Realm service binds to this Core's owner. Nothing
// here is discovered, downgraded or replaced per request.
type NetworkRealmEndpointConfig struct {
	Roots            *x509.CertPool
	SPKISHA256       [32]byte
	Service          *tls.Certificate
	RPCDeadline      time.Duration
	ConnectTimeout   time.Duration
	MaxResponseBytes int
	MaxMetadataBytes uint32
}

type networkRealmEndpoint struct {
	config NetworkRealmEndpointConfig
	client *realmsdk.Client
	serial uint64
}

// NetworkRealmConnections is a trusted network address binding, not a RealmID
// directory. It owns neither authorization facts nor the owner's durable
// contact. Every use names the owner's persisted address and expected Realm
// identity; a result from an address replaced during the call is never used
// as current evidence.
type NetworkRealmConnections struct {
	mu        sync.RWMutex
	endpoints map[values.NextHop]*networkRealmEndpoint
	serial    uint64
}

// BindNetworkEndpoint validates and retains the pinned client material for one
// address. The client is lazy: binding proves configuration shape, not that
// the Realm is currently reachable. Rebinding the same address retires every
// in-flight result from the previous binding, including an A→B→A return.
func (connections *NetworkRealmConnections) BindNetworkEndpoint(address values.NextHop, expected values.RealmID, config NetworkRealmEndpointConfig) error {
	if connections == nil {
		return ErrInvalidNetworkRealmEndpoint
	}
	if _, ok := address.NetworkTarget(); !ok || expected == "" || config.Roots == nil || config.SPKISHA256 == ([32]byte{}) ||
		config.RPCDeadline <= 0 || config.ConnectTimeout <= 0 || config.MaxResponseBytes <= 0 || config.MaxMetadataBytes < 256 {
		return ErrInvalidNetworkRealmEndpoint
	}
	client, err := realmsdk.New(realmsdk.Config{
		NextHop: address, Expected: expected, Roots: config.Roots, SPKISHA256: config.SPKISHA256,
		Certificate: config.Service, RPCDeadline: config.RPCDeadline, ConnectTimeout: config.ConnectTimeout,
		MaxResponseBytes: config.MaxResponseBytes, MaxMetadataBytes: config.MaxMetadataBytes,
	})
	if err != nil {
		return ErrInvalidNetworkRealmEndpoint
	}
	connections.mu.Lock()
	defer connections.mu.Unlock()
	if connections.serial == math.MaxUint64 {
		_ = client.Close()
		return ErrInvalidNetworkRealmEndpoint
	}
	if old, exists := connections.endpoints[address]; exists && old.client != nil {
		_ = old.client.Close()
	}
	connections.serial++
	if connections.endpoints == nil {
		connections.endpoints = make(map[values.NextHop]*networkRealmEndpoint)
	}
	connections.endpoints[address] = &networkRealmEndpoint{config: config, client: client, serial: connections.serial}
	return nil
}

// UnbindNetworkEndpoint drops one address binding. Results already taken from
// it stay retired; a later rebinding of the same address gets a fresh serial.
func (connections *NetworkRealmConnections) UnbindNetworkEndpoint(address values.NextHop) error {
	if connections == nil {
		return ErrInvalidNetworkRealmEndpoint
	}
	if _, ok := address.NetworkTarget(); !ok {
		return ErrInvalidNetworkRealmEndpoint
	}
	connections.mu.Lock()
	defer connections.mu.Unlock()
	endpoint, exists := connections.endpoints[address]
	if !exists {
		return ErrNetworkRealmUnavailable
	}
	delete(connections.endpoints, address)
	if endpoint.client != nil {
		_ = endpoint.client.Close()
	}
	return nil
}

func (connections *NetworkRealmConnections) selectEndpoint(address values.NextHop, expected values.RealmID) (*networkRealmEndpoint, error) {
	if connections == nil || expected == "" {
		return nil, ErrInvalidNetworkRealmEndpoint
	}
	if _, ok := address.NetworkTarget(); !ok {
		return nil, ErrInvalidNetworkRealmEndpoint
	}
	connections.mu.RLock()
	endpoint, exists := connections.endpoints[address]
	connections.mu.RUnlock()
	if !exists || endpoint.client == nil {
		return nil, ErrNetworkRealmUnavailable
	}
	if nextHop, realmID := endpoint.client.Target(); nextHop != address || realmID != expected {
		return nil, ErrNetworkRealmTargetMismatch
	}
	return endpoint, nil
}

// stillBound prevents a result from an endpoint removed or replaced during the
// call from being used as current authorization evidence.
func (connections *NetworkRealmConnections) stillBound(address values.NextHop, expected values.RealmID, endpoint *networkRealmEndpoint) bool {
	if connections == nil || endpoint == nil {
		return false
	}
	connections.mu.RLock()
	current, exists := connections.endpoints[address]
	connections.mu.RUnlock()
	if !exists || current != endpoint || current.serial != endpoint.serial {
		return false
	}
	nextHop, realmID := endpoint.client.Target()
	return nextHop == address && realmID == expected
}

// Check is for trusted owner initialization and open. It performs one online
// target check against the pinned address; an unreachable or unverified target
// is a failed check, never a silent pass or a fallback to another source.
func (connections *NetworkRealmConnections) Check(ctx context.Context, address values.NextHop, expected values.RealmID) error {
	endpoint, err := connections.selectEndpoint(address, expected)
	if err != nil {
		return err
	}
	if err := connections.verifyTarget(ctx, endpoint); err != nil {
		return err
	}
	if !connections.stillBound(address, expected, endpoint) {
		return ErrNetworkRealmUnavailable
	}
	return nil
}

// verifyTarget runs the pinned target check once, derived from the caller's
// context. A target reply is accepted only from the TLS transport that
// actually answered it, per the SDK contract.
func (connections *NetworkRealmConnections) verifyTarget(ctx context.Context, endpoint *networkRealmEndpoint) error {
	ctx, cancel := context.WithTimeout(ctx, endpoint.config.RPCDeadline)
	defer cancel()
	if _, err := endpoint.client.CheckTarget(ctx); err != nil {
		return ErrNetworkRealmTargetMismatch
	}
	return nil
}

// invoke runs one bounded protected call on the original request's context:
// the configured RPC deadline is an additional upper bound derived from that
// context, never a replacement for it, so the request's cancellation and
// deadline always propagate into the remote wait. A rebuilt connection needs
// one fresh target verification before credentials may be sent; exactly one
// check-and-retry follows a FailedPrecondition, and a second failure stands.
func (connections *NetworkRealmConnections) invoke(ctx context.Context, endpoint *networkRealmEndpoint, call func(context.Context, *realmsdk.Client) error) error {
	ctx, cancel := context.WithTimeout(ctx, endpoint.config.RPCDeadline)
	defer cancel()
	err := call(ctx, endpoint.client)
	if status.Code(err) == codes.FailedPrecondition {
		if _, checkErr := endpoint.client.CheckTarget(ctx); checkErr == nil {
			err = call(ctx, endpoint.client)
		}
	}
	return err
}
