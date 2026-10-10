package federationconn

import (
	"context"
	"time"

	"sync"
	federationcontract "ucupa/ucupa-core/contract/federation"

	"ucupa/shared/values"

	coresdk "ucupa/ucupa-core/sdk/go"
)

// FederationConnectionConfig is explicit transport assembly, not a route map.
// Peers are selected by the relationship's address and then checked against its
// expected owner. Neither this table nor Source identifies a spatial root.
type FederationConnectionConfig struct {
	Source   values.FactAuthorityRef
	Peers    []coresdk.Config
	MaxParts int
	TTL      time.Duration
}

// FederationConnections limits outbound source leases across all requesters.
// An individual lease owns a fresh target-bound actor client and cannot replace
// another consumer's grant or transparently renew an old read basis.
type FederationConnections struct {
	source values.FactAuthorityRef
	peers  map[values.NextHop]coresdk.Config
	limit  int
	ttl    time.Duration
	mu     sync.Mutex
	closed bool
	parts  map[*networkFederationPart]struct{}
}

func NewFederationConnections(c FederationConnectionConfig) (*FederationConnections, error) {
	if !c.Source.Valid() || len(c.Peers) == 0 || len(c.Peers) > 256 || c.MaxParts <= 0 || c.TTL <= 0 || c.TTL%time.Second != 0 {
		return nil, values.PublicInvalid
	}
	f := &FederationConnections{source: c.Source, limit: c.MaxParts, ttl: c.TTL, peers: make(map[values.NextHop]coresdk.Config), parts: make(map[*networkFederationPart]struct{})}
	for _, p := range c.Peers {
		if p.Expected == c.Source || p.Actor != c.Source || p.ActorCertificate == nil || f.peers[p.NextHop].Expected != "" {
			return nil, values.PublicInvalid
		}
		// New is lazy: this validates the explicit configuration without dialing
		// or inventing a session. Each real part gets its own fresh client.
		client, err := coresdk.New(p)
		if err != nil {
			return nil, values.PublicInvalid
		}
		_ = client.Close()
		p.Roots = p.Roots.Clone()
		certificate := *p.ActorCertificate
		certificate.Certificate = make([][]byte, len(p.ActorCertificate.Certificate))
		for i, der := range p.ActorCertificate.Certificate {
			certificate.Certificate[i] = append([]byte(nil), der...)
		}
		p.ActorCertificate = &certificate
		f.peers[p.NextHop] = p
	}
	return f, nil
}

type federationCallerConnections struct {
	connections *FederationConnections
	caller      values.CallerContext
	issuer      identityProofIssuer
	expires     time.Time
}

type identityProofIssuer interface {
	Issue(context.Context, values.CallerContext, values.FactAuthorityRef, [32]byte, time.Duration) ([]byte, []byte, error)
	Check(context.Context, []byte, []byte) error
}

// WithCaller is a trusted per-invocation adapter. expires is the original
// authenticated session deadline, not a duration supplied by a remote body.
// Issuance still validates the original local caller in its Realm.
func (f *FederationConnections) WithCaller(caller values.CallerContext, issuer identityProofIssuer, expires time.Time) (federationcontract.FederationConnections, error) {
	if f == nil || !caller.Subject.Valid() || issuer == nil || !time.Now().Before(expires) {
		return nil, values.PublicInvalid
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.closed {
		return nil, values.PublicUnavailable
	}
	return &federationCallerConnections{connections: f, caller: caller, issuer: issuer, expires: expires}, nil
}

// MatchesSource is the narrow assembly check used to bind this connector to
// the Core that owns its source identity. It exposes no peer or lease state.
func (f *FederationConnections) MatchesSource(expected values.FactAuthorityRef) bool {
	return f != nil && f.source == expected
}

func (f *FederationConnections) reserve() (*networkFederationPart, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.closed {
		return nil, values.PublicUnavailable
	}
	if len(f.parts) >= f.limit {
		return nil, values.PublicResourceExhausted
	}
	p := &networkFederationPart{connections: f}
	f.parts[p] = struct{}{}
	return p, nil
}

// Close ends local connections only; remote grants/parts have their own finite
// deadlines. Shutdown never retries reads or extends a remote lease.
func (f *FederationConnections) Close() {
	if f == nil {
		return
	}
	f.mu.Lock()
	f.closed = true
	parts := make([]*networkFederationPart, 0, len(f.parts))
	for p := range f.parts {
		parts = append(parts, p)
	}
	f.mu.Unlock()
	for _, p := range parts {
		p.close()
	}
}

type networkFederationPart struct {
	connections *FederationConnections
	mu          sync.Mutex
	closed      bool
	client      *coresdk.Client
	info        federationcontract.FederationReadPart
	expires     time.Time
	timer       *time.Timer
	requester   *federationCallerConnections
}

func (p *networkFederationPart) install(client *coresdk.Client, info federationcontract.FederationReadPart, expires time.Time) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed || !time.Now().Before(expires) {
		_ = client.Close()
		return values.PublicInvalidContext
	}
	p.client, p.info, p.expires = client, cloneFederationPart(info), expires
	p.timer = time.AfterFunc(time.Until(expires), p.close)
	return nil
}

func (p *networkFederationPart) close() {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return
	}
	p.closed = true
	if p.timer != nil {
		p.timer.Stop()
	}
	client := p.client
	p.mu.Unlock()
	if client != nil {
		_ = client.Close()
	}
	p.connections.mu.Lock()
	delete(p.connections.parts, p)
	p.connections.mu.Unlock()
}

func (p *networkFederationPart) Part() federationcontract.FederationReadPart {
	p.mu.Lock()
	defer p.mu.Unlock()
	return cloneFederationPart(p.info)
}

func cloneFederationPart(in federationcontract.FederationReadPart) federationcontract.FederationReadPart {
	if in.Incoming != nil {
		v := *in.Incoming
		in.Incoming = &v
	}
	if in.Preparation != nil {
		v := *in.Preparation
		in.Preparation = &v
	}
	return in
}
