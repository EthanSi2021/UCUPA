package source

import (
	"context"
	federationcontract "ucupa/ucupa-core/contract/federation"
	accesspkg "ucupa/ucupa-core/internal/access"

	"ucupa/shared/values"
	mountfact "ucupa/ucupa-core/internal/mount/fact"
	federationdenial "ucupa/ucupa-core/internal/query/federation/denial"
)

// CaptureFederationDenial proves only absence of the discovery permission at
// an already established incoming mount. Missing Observe permission alone is
// not a reason to hide otherwise discoverable facts. Exact Read denial is not
// a successful NOT_FOUND result, so this port accepts Query and List only.
func (core *Operations) CaptureFederationDenial(ctx context.Context, caller values.CallerContext, source values.FactAuthorityRef, request federationcontract.FederationReadRequest) (federationcontract.FederationDenial, error) {
	finish, err := core.enterFederationCall(ctx)
	if err != nil {
		return federationcontract.FederationDenial{}, err
	}
	defer finish()
	evidence, _, err := core.captureFederationDenial(ctx, caller, source, request)
	return evidence, err
}

// captureFederationDenial captures one denial online, outside any fact lock.
// The minted DenialPermit carries exactly this operation to the in-lock
// disclosure guard.
func (core *Operations) captureFederationDenial(ctx context.Context, caller values.CallerContext, source values.FactAuthorityRef, request federationcontract.FederationReadRequest) (federationdenial.Evidence, accesspkg.DenialPermit, error) {
	if !source.Valid() || !request.Incoming.Valid() || (request.Intent != "query" && request.Intent != "list") {
		return federationdenial.Evidence{}, nil, values.PublicInvalid
	}
	base := request
	base.PrepareObserve = false
	requirements, err := core.federationRequirements(base)
	if err != nil || len(requirements) != 1 {
		return federationdenial.Evidence{}, nil, values.PublicInvalid
	}
	decision, denial, permit, err := core.admission.CaptureDenial(ctx, caller, requirements[0])
	keep := false
	defer func() {
		if !keep {
			accesspkg.ReleaseDenialPermit(ctx, permit)
		}
	}()
	if err == nil && decision != values.AuthorizationDenied {
		// A still-valid authorization is not a denial.
		return federationdenial.Evidence{}, nil, values.PublicInvalidContext
	}
	if err != nil {
		return federationdenial.Evidence{}, nil, err
	}
	core.owner.Lock()
	defer core.owner.Unlock()
	if ctx.Err() != nil || core.owner.ClosedLocked() || !core.owner.AvailableLocked() {
		return federationdenial.Evidence{}, nil, values.PublicUnavailable
	}
	in := core.owner.FactsLocked().Mount.Incoming()
	if in.Key != request.Incoming || in.Detached || in.Gate != mountfact.GateOpen || !mountfact.ValidMountAdmissionKey(in.Admission.Key) {
		return federationdenial.Evidence{}, nil, values.PublicUnavailable
	}
	if err := core.admission.ValidateDenial(ctx, caller, denial, requirements[0], permit); err != nil {
		return federationdenial.Evidence{}, nil, err
	}
	keep = true
	return federationdenial.New(core.owner.AuthorityRef(), source, caller, request, requirements[0], denial, in, core.registry.DenialDeadlineLocked()), permit, nil
}

func (core *Operations) CheckFederationDenial(ctx context.Context, caller values.CallerContext, source values.FactAuthorityRef, request federationcontract.FederationReadRequest, evidence federationcontract.FederationDenial) error {
	finish, err := core.enterFederationCall(ctx)
	if err != nil {
		return err
	}
	defer finish()
	// The binding is checked before any online work; the retained denial view
	// is then rechecked outside the lock and only guarded under it.
	core.owner.Lock()
	if !federationdenial.MatchesBinding(evidence, core.owner.AuthorityRef(), source, caller, request) {
		core.owner.Unlock()
		return values.PublicDenied
	}
	core.owner.Unlock()
	view, requirement := federationdenial.Authorization(evidence)
	permit, err := core.admission.RecheckDenial(ctx, caller, view, requirement)
	if err != nil {
		return err
	}
	core.owner.Lock()
	defer core.owner.Unlock()
	return core.checkFederationDenialLocked(ctx, caller, source, request, evidence, permit)
}

// checkFederationDenialLocked re-verifies the same evidence under the fact
// lock and guards with the DenialPermit minted for exactly this operation.
func (core *Operations) checkFederationDenialLocked(ctx context.Context, caller values.CallerContext, source values.FactAuthorityRef, request federationcontract.FederationReadRequest, evidence federationdenial.Evidence, permit accesspkg.DenialPermit) error {
	if !federationdenial.MatchesBinding(evidence, core.owner.AuthorityRef(), source, caller, request) {
		return values.PublicDenied
	}
	if core.owner.ClosedLocked() || !core.owner.AvailableLocked() || ctx.Err() != nil {
		return values.PublicUnavailable
	}
	if core.registry.DenialExpiredLocked(evidence) || !federationdenial.MatchesIncoming(evidence, core.owner.FactsLocked().Mount.Incoming()) {
		return values.PublicInvalidContext
	}
	view, requirement := federationdenial.Authorization(evidence)
	return core.admission.ValidateDenial(ctx, caller, view, requirement, permit)
}
