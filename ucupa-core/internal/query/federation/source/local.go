package source

import (
	"context"

	"ucupa/shared/values"
	federationcontract "ucupa/ucupa-core/contract/federation"
)

// The collector calls the actual source operations through its existing local
// connection. This object holds one source receipt, not a second source owner.
type localFederationPart struct {
	owner  *Operations
	caller values.CallerContext
	source values.FactAuthorityRef
	part   federationcontract.FederationReadPart
}

func OpenLocal(core *Operations, ctx context.Context, caller values.CallerContext, request federationcontract.FederationReadRequest) (federationcontract.FederationReadConnection, error) {
	part, err := core.OpenFederationReadPart(ctx, caller, core.owner.AuthorityRef(), request)
	if err != nil {
		return nil, err
	}
	return &localFederationPart{owner: core, caller: caller, source: core.owner.AuthorityRef(), part: part}, nil
}

func (p *localFederationPart) Part() federationcontract.FederationReadPart {
	result := p.part
	if result.Incoming != nil {
		copy := *result.Incoming
		result.Incoming = &copy
	}
	if result.Preparation != nil {
		copy := *result.Preparation
		result.Preparation = &copy
	}
	return result
}

func (p *localFederationPart) Project(ctx context.Context, request federationcontract.FederationProjectionRequest, continuation *values.PublicToken, limit uint16) (federationcontract.FederationProjectionPage, error) {
	return p.owner.ProjectFederationReadPart(ctx, p.caller, p.source, p.part.Basis, request, continuation, limit)
}

func (p *localFederationPart) Check(ctx context.Context, current bool) error {
	return p.owner.CheckFederationReadPart(ctx, p.caller, p.source, p.part.Basis, current)
}

func (p *localFederationPart) Observe(ctx context.Context, request federationcontract.FederationObserveRequest) (federationcontract.FederationObservationPage, error) {
	return p.owner.ObserveFederationReadPart(ctx, p.caller, p.source, p.part.Basis, request)
}

func (p *localFederationPart) Release(ctx context.Context) error {
	// Retirement already belongs to the original local envelope. It must not
	// compete for another admission slot; the ordinary service Release does.
	// Preserve the original local adapter's non-nil context precondition.
	if ctx == nil {
		panic("cannot create context from nil parent")
	}
	if p.owner == nil || p.owner.registry == nil || ctx.Err() != nil {
		return values.PublicUnavailable
	}
	return p.owner.release(p.caller, p.source, p.part.Basis)
}
