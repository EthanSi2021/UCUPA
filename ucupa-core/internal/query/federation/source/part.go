package source

import (
	federationcontract "ucupa/ucupa-core/contract/federation"
	accesspkg "ucupa/ucupa-core/internal/access"
	federationprojection "ucupa/ucupa-core/internal/query/federation/projection"

	"context"
	"errors"

	"ucupa/shared/values"
	corecontract "ucupa/ucupa-core/contract"
	accessregistry "ucupa/ucupa-core/internal/access/registry"
	mountfact "ucupa/ucupa-core/internal/mount/fact"
)

// A source checks the original operation, not the coordinator's implementation
// steps. Internal path traversal never upgrades Query into Read or List.
func (core *Operations) federationRequirements(request federationcontract.FederationReadRequest) ([]values.Requirement, error) {
	if !request.Boundary.Valid() || request.Purpose == ([32]byte{}) ||
		(request.Mode != 0 && request.Mode != corecontract.QueryCurrentVerified) ||
		(request.Incoming != (corecontract.MountKey{}) && (!request.Incoming.Valid() || request.Incoming.MountAnchorUUID != request.Boundary || request.Incoming.CarrierAuthority != core.owner.AuthorityRef())) {
		return nil, values.PublicInvalid
	}
	var permissions []values.PermissionID
	switch request.Intent {
	case "read":
		permissions = []values.PermissionID{corecontract.PermissionReadElement}
	case "list":
		permissions = []values.PermissionID{corecontract.PermissionListDirect}
	case "query":
		permissions = []values.PermissionID{corecontract.PermissionQueryRegion}
	default:
		return nil, values.PublicInvalid
	}
	if request.PrepareObserve {
		permissions = append(permissions, corecontract.PermissionObserveElement)
	}
	return accesspkg.ClosureRequirements(core.owner.AuthorityRef(), request.Boundary, permissions...), nil
}

// OpenFederationReadPart binds an original caller to an immutable owner-local
// read part. source is trusted transport's authenticated coordinator identity,
// not a user body field and not necessarily the immediate spatial parent.
func (core *Operations) OpenFederationReadPart(ctx context.Context, caller values.CallerContext, source values.FactAuthorityRef, request federationcontract.FederationReadRequest) (federationcontract.FederationReadPart, error) {
	finish, err := core.enterFederationCall(ctx)
	if err != nil {
		return federationcontract.FederationReadPart{}, err
	}
	defer finish()
	if !source.Valid() {
		return federationcontract.FederationReadPart{}, values.PublicInvalid
	}
	requirements, err := core.federationRequirements(request)
	if err != nil {
		return federationcontract.FederationReadPart{}, err
	}
	view, permit, err := core.admission.Authorize(ctx, caller, requirements)
	retainedView := false
	defer func() {
		if !retainedView {
			core.admission.ReleasePermit(ctx, permit)
		}
	}()
	if err != nil {
		if errors.Is(err, values.PublicDenied) && request.Incoming.Valid() && (request.Intent == "query" || request.Intent == "list") {
			return core.openFederationHiddenPart(ctx, caller, source, request)
		}
		return federationcontract.FederationReadPart{}, err
	}
	core.owner.Lock()
	if core.owner.ClosedLocked() || !core.owner.AvailableLocked() {
		core.owner.Unlock()
		return federationcontract.FederationReadPart{}, values.PublicUnavailable
	}
	if err := core.admission.Validate(ctx, caller, view, requirements, false, permit); err != nil {
		core.owner.Unlock()
		return federationcontract.FederationReadPart{}, err
	}
	state := core.owner.FactsLocked()
	if core.owner.SourceVisibilityLocked().Inactive(request.Boundary) {
		core.owner.Unlock()
		return federationcontract.FederationReadPart{}, values.PublicUnavailable
	}
	if request.Incoming.Valid() {
		if state.Mount.Incoming().Key != request.Incoming || state.Mount.Incoming().Detached || state.Mount.Incoming().Gate != mountfact.GateOpen {
			core.owner.Unlock()
			return federationcontract.FederationReadPart{}, values.PublicUnavailable
		}
	} else if _, ok := state.Elements.Anchor(request.Boundary); !ok {
		core.owner.Unlock()
		return federationcontract.FederationReadPart{}, values.PublicNotFound
	}
	reservation, err := core.registry.ReserveLocked(4096)
	if err != nil {
		core.owner.Unlock()
		return federationcontract.FederationReadPart{}, err
	}
	part := accessregistry.SourceConfig{Source: source, Request: request, Mode: core.queries.Mode(), Incoming: state.Mount.Incoming()}
	if request.Mode == corecontract.QueryCurrentVerified {
		part.Mode = corecontract.QueryCurrentVerified
	}
	cut := state.CurrentCut()
	core.owner.Unlock()
	defer core.registry.Release(reservation)
	core.owner.Lock()
	if core.owner.ClosedLocked() || !core.owner.AvailableLocked() || ctx.Err() != nil {
		core.owner.Unlock()
		return federationcontract.FederationReadPart{}, values.PublicUnavailable
	}
	if core.owner.CurrentCutLocked() != cut {
		core.owner.Unlock()
		return federationcontract.FederationReadPart{}, values.PublicInvalidContext
	}
	if err := core.admission.Validate(ctx, caller, view, requirements, false, permit); err != nil {
		core.owner.Unlock()
		return federationcontract.FederationReadPart{}, err
	}
	if err := accessregistry.CheckSourceConfig(part, core.owner.FactsLocked().Mount); err != nil {
		core.owner.Unlock()
		return federationcontract.FederationReadPart{}, err
	}
	basis, err := core.registry.IssueSourceLocked(reservation, caller, request.Boundary, cut, view, requirements, part)
	if err != nil {
		core.owner.Unlock()
		return federationcontract.FederationReadPart{}, err
	}
	retainedView = core.registry.OwnAuthorizationReleaseLocked(basis.Ref, core.admission.PermitRelease(permit))
	var preparation accessregistry.PreparationUse
	if request.PrepareObserve {
		preparation, err = core.registry.PrepareSourceLocked(basis.Ref, requirements, view, cut, request.Intent == "read")
	}
	core.owner.Unlock()
	keep := false
	defer func() {
		if !keep {
			core.owner.Lock()
			core.registry.RemoveLocked(basis.Ref)
			core.owner.Unlock()
		}
	}()
	if err != nil {
		return federationcontract.FederationReadPart{}, err
	}
	borrow, err := core.queries.Capture(caller.Subject, request.Boundary, &cut, part.Mode)
	if err != nil {
		return federationcontract.FederationReadPart{}, federationprojection.ReadError(err)
	}
	defer borrow.Finish()
	core.owner.Lock()
	defer core.owner.Unlock()
	if current, present := core.registry.ReadLocked(basis.Ref); !present || current.Ref != basis.Ref {
		if borrow.Snapshot() != nil {
			// The part expired before taking ownership of this snapshot.
			core.queries.DiscardOwnedSnapshot(*borrow.Snapshot())
		}
		return federationcontract.FederationReadPart{}, values.PublicInvalidContext
	}
	if !core.registry.InstallSnapshotLocked(basis.Ref, borrow.Snapshot()) {
		if borrow.Snapshot() != nil {
			core.queries.DiscardOwnedSnapshot(*borrow.Snapshot())
		}
		return federationcontract.FederationReadPart{}, values.PublicInvalidContext
	}
	basis, _ = core.registry.ReadLocked(basis.Ref)
	if err := core.queries.CheckLocked(borrow, ""); err != nil {
		return federationcontract.FederationReadPart{}, federationprojection.ReadError(err)
	}
	if err := core.checkFederationBasisLocked(ctx, caller, source, basis, permit, nil); err != nil {
		return federationcontract.FederationReadPart{}, err
	}
	if ctx.Err() != nil {
		return federationcontract.FederationReadPart{}, values.PublicUnavailable
	}
	signer := core.owner.SigningLocked()
	result := federationcontract.FederationReadPart{Basis: basis.Token, Boundary: request.Boundary, Purpose: request.Purpose, ReadMode: part.Mode}
	result.Subject = caller.Subject
	result.CutStamp, result.AuthorizationStamp = federationReadStamps(basis)
	if part.Incoming.Key.Valid() {
		relation := federationprojection.Incoming(borrow.Projection().Mount, signer)
		result.Incoming = &relation
	}
	if preparation.Ref != (accessregistry.Ref{}) {
		token := preparation.Token
		result.Preparation = &token
	}
	keep = true
	return result, nil
}
