package source

import (
	federationcontract "ucupa/ucupa-core/contract/federation"
	accessresolve "ucupa/ucupa-core/internal/access/resolve"
	federationprojection "ucupa/ucupa-core/internal/query/federation/projection"

	"context"

	"ucupa/shared/values"
	corecontract "ucupa/ucupa-core/contract"
	accesspkg "ucupa/ucupa-core/internal/access"
	accessregistry "ucupa/ucupa-core/internal/access/registry"
	accessvalue "ucupa/ucupa-core/internal/access/value"
	federationdenial "ucupa/ucupa-core/internal/query/federation/denial"
)

// A hidden part retains only an authenticated denial and the incoming relation.
// It never captures facts, a historical snapshot, or an observation queue.
func (core *Operations) openFederationHiddenPart(ctx context.Context, caller values.CallerContext, source values.FactAuthorityRef, request federationcontract.FederationReadRequest) (federationcontract.FederationReadPart, error) {
	denial, permit, err := core.captureFederationDenial(ctx, caller, source, request)
	retainedView := false
	defer func() {
		if !retainedView {
			accesspkg.ReleaseDenialPermit(ctx, permit)
		}
	}()
	if err != nil {
		return federationcontract.FederationReadPart{}, err
	}
	core.owner.Lock()
	defer core.owner.Unlock()
	if err := core.checkFederationDenialLocked(ctx, caller, source, request, denial, permit); err != nil {
		return federationcontract.FederationReadPart{}, err
	}
	part := accessregistry.SourceConfig{Source: source, Request: request, Mode: core.queries.Mode(), Incoming: federationdenial.Incoming(denial), Denial: &denial}
	if request.Mode == corecontract.QueryCurrentVerified {
		part.Mode = corecontract.QueryCurrentVerified
	}
	basis, err := core.registry.IssueHiddenSourceLocked(caller, request.Boundary, core.owner.CurrentCutLocked(), part)
	if err != nil {
		return federationcontract.FederationReadPart{}, err
	}
	retainedView = core.registry.OwnAuthorizationReleaseLocked(basis.Ref, accesspkg.DenialPermitRelease(permit))
	keep := false
	defer func() {
		if !keep {
			core.registry.RemoveLocked(basis.Ref)
		}
	}()
	result := federationcontract.FederationReadPart{Hidden: true, Basis: basis.Token, Boundary: request.Boundary, Purpose: request.Purpose, ReadMode: part.Mode, Subject: caller.Subject}
	relation := federationprojection.Incoming(core.owner.FactsLocked().Mount, core.owner.SigningLocked())
	result.Incoming = &relation
	result.CutStamp, result.AuthorizationStamp = federationReadStamps(basis)
	if request.PrepareObserve {
		prep, _, err := core.registry.PrepareHiddenLocked(basis.Ref)
		if err != nil {
			return federationcontract.FederationReadPart{}, err
		}
		result.Preparation = &prep.Token
	}
	keep = true
	return result, nil
}

// Repeated Begin/Poll validates the same denial, but emits no evidence about
// hidden writes, their number or timing. It cannot renew the finite lease.
func (core *Operations) observeFederationHiddenLocked(ctx context.Context, caller values.CallerContext, source values.FactAuthorityRef, basis accessregistry.ReadUse, request federationcontract.FederationObserveRequest, permit accesspkg.DenialPermit) (federationcontract.FederationObservationPage, error) {
	part := basis.Source
	if err := core.checkFederationDenialLocked(ctx, caller, source, part.Request, *part.Denial, permit); err != nil {
		return federationcontract.FederationObservationPage{}, err
	}
	prep, present := core.registry.PreparationLocked(basis.Preparation)
	if !present || !prep.HasSource || prep.Source != basis.Ref {
		return federationcontract.FederationObservationPage{}, values.PublicGap
	}
	cursor, err := accessresolve.CursorLocked(core.owner, core.registry, caller, accessvalue.TokenJSON(prep.Initial))
	if err != nil {
		return federationcontract.FederationObservationPage{}, err
	}
	if cursor.Preparation != prep.Ref {
		return federationcontract.FederationObservationPage{}, values.PublicInvalidContext
	}
	if request.Operation == "poll" && *request.Cursor != cursor.Token {
		return federationcontract.FederationObservationPage{}, values.PublicInvalidContext
	}
	return federationcontract.FederationObservationPage{Cursor: cursor.Token}, nil
}
