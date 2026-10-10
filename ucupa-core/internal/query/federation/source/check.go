package source

import (
	"context"
	accessresolve "ucupa/ucupa-core/internal/access/resolve"

	"ucupa/shared/values"
	corecontract "ucupa/ucupa-core/contract"
	accesspkg "ucupa/ucupa-core/internal/access"
	accessregistry "ucupa/ucupa-core/internal/access/registry"
	accessvalue "ucupa/ucupa-core/internal/access/value"
	federationdenial "ucupa/ucupa-core/internal/query/federation/denial"
	federationprojection "ucupa/ucupa-core/internal/query/federation/projection"
)

// checkFederationBasisLocked guards one basis entry under the fact lock with
// the permit minted for exactly this operation: the normal branch consumes
// permit, the hidden branch consumes denialPermit. Which branch applies is
// fixed when the entry is issued and cannot change across a recheck.
func (core *Operations) checkFederationBasisLocked(ctx context.Context, caller values.CallerContext, source values.FactAuthorityRef, basis accessregistry.ReadUse, permit accesspkg.Permit, denialPermit accesspkg.DenialPermit) error {
	if !basis.Source.Present {
		return values.PublicInvalidContext
	}
	if !source.Valid() || basis.Source.Source != source {
		return values.PublicDenied
	}
	if denial := basis.Source.Denial; denial != nil {
		return core.checkFederationDenialLocked(ctx, caller, source, basis.Source.Request, *denial, denialPermit)
	}
	if err := core.admission.Validate(ctx, caller, basis.View, basis.Requirements, false, permit); err != nil {
		return err
	}
	if err := core.registry.CheckSourceRelationsLocked(basis.Ref, core.owner.FactsLocked().Mount, core.owner.ParentReadVisibilityLocked()); err != nil {
		return err
	}
	if basis.Source.Mode == corecontract.QueryCurrentVerified {
		if core.owner.CurrentCutLocked() != basis.Cut {
			return values.PublicInvalidContext
		}
	} else {
		if !core.queries.SnapshotPresent(basis.Snapshot) {
			return values.PublicInvalidContext
		}
	}
	if basis.HasPreparation {
		prep, present := core.registry.PreparationLocked(basis.Preparation)
		if !present || !prep.HasQueue || prep.Gap {
			return values.PublicGap
		}
	}
	return nil
}

// requireCurrent is used only to seal a newly collected composite: even a
// retained part must still be its owner's current cut at that initial check.
// The retained view is rechecked online outside the fact lock, then guarded
// together with all relation, cut and snapshot checks under one lock.
func (core *Operations) CheckFederationReadPart(ctx context.Context, caller values.CallerContext, source values.FactAuthorityRef, token values.PublicToken, requireCurrent bool) error {
	finish, err := core.enterFederationCall(ctx)
	if err != nil {
		return err
	}
	defer finish()
	core.owner.Lock()
	basis, err := accessresolve.ReadLocked(core.owner, core.registry, caller, accessvalue.TokenJSON(token))
	core.owner.Unlock()
	if err != nil {
		return err
	}
	var permit accesspkg.Permit
	var denialPermit accesspkg.DenialPermit
	if basis.Source.Denial != nil {
		view, requirement := federationdenial.Authorization(*basis.Source.Denial)
		denialPermit, err = core.admission.RecheckDenial(ctx, caller, view, requirement)
	} else {
		permit, err = core.admission.Recheck(ctx, caller, basis.View, basis.Requirements, false)
	}
	if err != nil {
		return err
	}
	core.owner.Lock()
	defer core.owner.Unlock()
	basis, err = accessresolve.ReadLocked(core.owner, core.registry, caller, accessvalue.TokenJSON(token))
	if err != nil {
		return err
	}
	if err := core.checkFederationBasisLocked(ctx, caller, source, basis, permit, denialPermit); err != nil {
		return err
	}
	if requireCurrent && basis.Source.Denial == nil && core.owner.CurrentCutLocked() != basis.Cut {
		return values.PublicInvalidContext
	}
	if basis.Source.Denial == nil && basis.Snapshot != nil {
		if err := core.queries.CheckSnapshotLocked(caller.Subject, *basis.Snapshot); err != nil {
			return federationprojection.ReadError(err)
		}
	}
	return nil
}

// Release does not need a new authorization grant and is idempotent after
// expiry. A still-live basis cannot be released by another caller/coordinator.
func (core *Operations) ReleaseFederationReadPart(ctx context.Context, caller values.CallerContext, source values.FactAuthorityRef, token values.PublicToken) error {
	finish, err := core.enterFederationCall(ctx)
	if err != nil {
		return err
	}
	defer finish()
	return core.release(caller, source, token)
}

// release is shared by admitted service calls and the private local retirement.
func (core *Operations) release(caller values.CallerContext, source values.FactAuthorityRef, token values.PublicToken) error {
	if !token.Valid() || token.Kind != "read-receipt" || token.Owner != core.owner.AuthorityRef() || !source.Valid() {
		return values.PublicInvalid
	}
	core.owner.Lock()
	defer core.owner.Unlock()
	basis, err := accessresolve.ReadLocked(core.owner, core.registry, caller, accessvalue.TokenJSON(token))
	if err == values.PublicInvalidContext {
		return nil
	}
	if err != nil {
		return err
	}
	if !basis.Source.Present {
		return values.PublicInvalidContext
	}
	if basis.Source.Source != source {
		return values.PublicDenied
	}
	core.registry.RemoveLocked(basis.Ref)
	return nil
}
