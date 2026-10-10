package facet

import (
	"context"

	"ucupa/shared/values"
	corecontract "ucupa/ucupa-core/contract"
	"ucupa/ucupa-core/internal/access"
	commitpkg "ucupa/ucupa-core/internal/commit"
	commitbinding "ucupa/ucupa-core/internal/commit/binding"
	facetfact "ucupa/ucupa-core/internal/facet/fact"
)

func (core *Operations) InspectFacetControl(ctx context.Context, caller values.CallerContext, element values.ElementID) (corecontract.PublicFacetInspection, error) {
	if core == nil || !core.admission.Enabled() {
		return corecontract.PublicFacetInspection{}, values.PublicUnavailable
	}
	if ctx == nil || !element.Valid() {
		return corecontract.PublicFacetInspection{}, values.PublicInvalid
	}
	cut, err := core.captureReadableCut()
	if err != nil {
		return corecontract.PublicFacetInspection{}, err
	}
	reqs := []values.Requirement{core.requirement(corecontract.PermissionFacetProposalRead, element)}
	// The authorization runs outside the fact lock; the in-lock steps guard only
	// with the permit this one call minted.
	view, permit, err := core.admission.Authorize(ctx, caller, reqs)
	defer core.admission.ReleasePermit(ctx, permit)
	if err != nil {
		return corecontract.PublicFacetInspection{}, err
	}
	core.owner.RLock()
	defer core.owner.RUnlock()
	if core.owner.ClosedLocked() || !core.owner.AvailableLocked() || core.owner.CurrentCutLocked() != cut {
		return corecontract.PublicFacetInspection{}, values.PublicUnavailable
	}
	if err = core.admission.Validate(ctx, caller, view, reqs, false, permit); err != nil {
		return corecontract.PublicFacetInspection{}, err
	}
	r, ok := core.owner.FactsLocked().Attachments.Record(element)
	if !ok {
		return corecontract.PublicFacetInspection{}, values.PublicNotFound
	}
	return facetfact.PublicInspection(core.owner.SigningLocked(), element, r.Result(core.owner.CurrentCutLocked(), corecontract.MutationUnchanged)), nil
}

// ApplyFacetControl uses the existing durable attachment transition, with the
// original caller view held across the fixed service's acceptance round trip.
// Exact retries resolve their original comparison from the operation journal;
// an unrelated caller or changed command cannot adopt that operation.
func (core *Operations) ApplyFacetControl(ctx context.Context, caller values.CallerContext, id corecontract.OperationID, input corecontract.PublicFacetCommand) (corecontract.MutationStatus, corecontract.PublicFacetInspection, error) {
	if core == nil || !core.admission.Enabled() {
		return 0, corecontract.PublicFacetInspection{}, values.PublicUnavailable
	}
	if ctx == nil || !commitbinding.ValidOperationID(id) || !validPublicFacetCommand(input, core.owner.AuthorityRef()) {
		return 0, corecontract.PublicFacetInspection{}, values.PublicInvalid
	}
	if err := core.admission.Acquire(); err != nil {
		return 0, corecontract.PublicFacetInspection{}, err
	}
	defer core.admission.Release()
	b := facetfact.Binding{Actor: caller.Subject, Kind: commitpkg.FacetCommandKind(input.Kind), Element: input.Element, Target: input.Target, Address: input.NextHop}
	reqs := []values.Requirement{core.requirement(b.Permission(), b.Element)}
	view, permit, err := core.admission.Authorize(ctx, caller, reqs)
	defer core.admission.ReleasePermit(ctx, permit)
	if err != nil {
		return 0, corecontract.PublicFacetInspection{}, err
	}
	guard := &controlAuthorization{ctx: ctx, caller: caller, view: view, requirements: reqs, permit: permit}
	core.owner.Lock()
	if core.owner.ClosedLocked() || !core.owner.AvailableLocked() {
		err = values.PublicUnavailable
	} else if err = guard.validate(core.admission); err == nil {
		b, err = core.owner.ResolvePublicFacetLocked(id, input, b)
	}
	core.owner.Unlock()
	if err != nil {
		return 0, corecontract.PublicFacetInspection{}, err
	}
	r, err := core.executeAttachmentGuarded(ctx, id, b, guard)
	if err != nil {
		return r.Status, corecontract.PublicFacetInspection{}, err
	}
	core.owner.RLock()
	defer core.owner.RUnlock()
	if core.owner.ClosedLocked() || !core.owner.AvailableLocked() {
		return 0, corecontract.PublicFacetInspection{}, values.PublicUnavailable
	}
	if err = guard.validate(core.admission); err != nil {
		return 0, corecontract.PublicFacetInspection{}, err
	}
	return r.Status, facetfact.PublicInspection(core.owner.SigningLocked(), input.Element, r), nil
}

// captureReadableCut holds the read lock only long enough to record that the
// owner is open and to capture its current cut, releasing it before any
// authorization wait. A later comparison against the same cut means a decision
// made while facts moved is never consumed as current evidence.
func (core *Operations) captureReadableCut() (corecontract.CoreReadCut, error) {
	core.owner.RLock()
	defer core.owner.RUnlock()
	if core.owner.ClosedLocked() || !core.owner.AvailableLocked() {
		return corecontract.CoreReadCut{}, values.PublicUnavailable
	}
	return core.owner.CurrentCutLocked(), nil
}

func validPublicFacetCommand(c corecontract.PublicFacetCommand, owner values.FactAuthorityRef) bool {
	k := commitpkg.FacetCommandKind(c.Kind)
	if k == 0 || !c.Element.Valid() || !c.Expected.Valid() || c.Expected.Owner != owner {
		return false
	}
	if k == facetfact.Address {
		return c.Expected.Kind == "contact-revision" && c.Target == "" && c.NextHop.Valid()
	}
	if c.Expected.Kind != "attachment-revision" {
		return false
	}
	if k == facetfact.Propose || k == facetfact.Release {
		return c.Target.Valid() && c.NextHop.Valid()
	}
	return c.Target == "" && c.NextHop == ""
}

// controlAuthorization is private to this operation, immutable after capture,
// and validates the exact original request. The permit is the one-operation
// evidence minted by the lock-free authorization; every in-lock boundary of
// this same operation consumes only it. The guard carries the original
// request's context, so a request that died during a peer wait never passes
// a commit boundary on a previously minted permit.
type controlAuthorization struct {
	ctx          context.Context
	caller       values.CallerContext
	view         values.AuthorizationView
	requirements []values.Requirement
	permit       access.Permit
}

func (g *controlAuthorization) validate(admission *access.Admission) error {
	return admission.Validate(g.ctx, g.caller, g.view, g.requirements, false, g.permit)
}
