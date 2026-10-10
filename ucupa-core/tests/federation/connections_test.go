package federation_test

import (
	"context"
	"errors"
	"sync"
	"testing"

	"ucupa/shared/values"
	federationcontract "ucupa/ucupa-core/contract/federation"
)

// A consumer-owned port for the actual source methods used by this adapter.
// No instance, registry, query state or local-snapshot internals are exposed.
type sourcePort interface {
	AuthorityRef() values.FactAuthorityRef
	OpenFederationReadPart(context.Context, values.CallerContext, values.FactAuthorityRef, federationcontract.FederationReadRequest) (federationcontract.FederationReadPart, error)
	ProjectFederationReadPart(context.Context, values.CallerContext, values.FactAuthorityRef, values.PublicToken, federationcontract.FederationProjectionRequest, *values.PublicToken, uint16) (federationcontract.FederationProjectionPage, error)
	CheckFederationReadPart(context.Context, values.CallerContext, values.FactAuthorityRef, values.PublicToken, bool) error
	ObserveFederationReadPart(context.Context, values.CallerContext, values.FactAuthorityRef, values.PublicToken, federationcontract.FederationObserveRequest) (federationcontract.FederationObservationPage, error)
	ReleaseFederationReadPart(context.Context, values.CallerContext, values.FactAuthorityRef, values.PublicToken) error
}
type vectorTestConnections struct {
	source values.FactAuthorityRef
	owners map[values.NextHop]sourcePort
	mu     sync.Mutex
	opened int
	parts  []*trackedSource
}

func (f *vectorTestConnections) OpenFederationRead(ctx context.Context, caller values.CallerContext, hop values.NextHop, target values.FactAuthorityRef, request federationcontract.FederationReadRequest) (federationcontract.FederationReadConnection, error) {
	owner := f.owners[hop]
	if owner == nil || owner.AuthorityRef() != target {
		return nil, values.PublicUnavailable
	}
	info, err := owner.OpenFederationReadPart(ctx, caller, f.source, request)
	if err != nil {
		return nil, err
	}
	part := &trackedSource{owner: owner, caller: caller, source: f.source, part: info}
	f.mu.Lock()
	f.opened++
	f.parts = append(f.parts, part)
	f.mu.Unlock()
	return part, nil
}

type trackedSource struct {
	owner    sourcePort
	caller   values.CallerContext
	source   values.FactAuthorityRef
	part     federationcontract.FederationReadPart
	mu       sync.Mutex
	releases int
}

func (p *trackedSource) Part() federationcontract.FederationReadPart {
	v := p.part
	if v.Incoming != nil {
		copy := *v.Incoming
		v.Incoming = &copy
	}
	if v.Preparation != nil {
		copy := *v.Preparation
		v.Preparation = &copy
	}
	return v
}
func (p *trackedSource) Project(ctx context.Context, r federationcontract.FederationProjectionRequest, c *values.PublicToken, l uint16) (federationcontract.FederationProjectionPage, error) {
	return p.owner.ProjectFederationReadPart(ctx, p.caller, p.source, p.part.Basis, r, c, l)
}
func (p *trackedSource) Check(ctx context.Context, current bool) error {
	return p.owner.CheckFederationReadPart(ctx, p.caller, p.source, p.part.Basis, current)
}
func (p *trackedSource) Observe(ctx context.Context, r federationcontract.FederationObserveRequest) (federationcontract.FederationObservationPage, error) {
	return p.owner.ObserveFederationReadPart(ctx, p.caller, p.source, p.part.Basis, r)
}
func (p *trackedSource) Release(ctx context.Context) error {
	p.mu.Lock()
	p.releases++
	p.mu.Unlock()
	return p.owner.ReleaseFederationReadPart(ctx, p.caller, p.source, p.part.Basis)
}

// These are actual adapter release calls plus source-side invalidation, not
// a substitute for Registry owner's separate exact entries/bytes assertions.
func (f *vectorTestConnections) requireReleased(t *testing.T) {
	t.Helper()
	f.mu.Lock()
	parts := append([]*trackedSource(nil), f.parts...)
	f.mu.Unlock()
	for i, p := range parts {
		p.mu.Lock()
		releases := p.releases
		p.mu.Unlock()
		if releases != 1 {
			t.Fatalf("source %d Release calls=%d, want 1", i, releases)
		}
		if err := p.Check(context.Background(), false); !errors.Is(err, values.PublicInvalidContext) {
			t.Fatalf("source %d remained usable: %v", i, err)
		}
	}
}
