package facet

import (
	"context"

	"ucupa/shared/values"
	corecontract "ucupa/ucupa-core/contract"
	accessregistry "ucupa/ucupa-core/internal/access/registry"
	accessvalue "ucupa/ucupa-core/internal/access/value"
)

func (core *Operations) ReadAttachmentControl(ctx context.Context, caller values.CallerContext, key values.FacetAcceptanceKey, prepare bool) (corecontract.AttachmentControlRead, error) {
	if core == nil || core.registry == nil {
		return corecontract.AttachmentControlRead{}, values.PublicUnavailable
	}
	if ctx == nil || !key.Valid() || key.Core != core.owner.AuthorityRef() {
		return corecontract.AttachmentControlRead{}, values.PublicInvalid
	}
	defer core.registry.DrainRetired()
	if err := core.admission.Acquire(); err != nil {
		return corecontract.AttachmentControlRead{}, err
	}
	defer core.admission.Release()
	requirements := []values.Requirement{core.requirement(corecontract.PermissionFacetAttachmentRead, key.Element)}
	if prepare {
		requirements = append(requirements, core.requirement(corecontract.PermissionObserveAttachment, key.Element))
	}
	view, permit, err := core.admission.Authorize(ctx, caller, requirements)
	retainedView := false
	defer func() {
		if !retainedView {
			core.admission.ReleasePermit(ctx, permit)
		}
	}()
	if err != nil {
		return corecontract.AttachmentControlRead{}, err
	}
	core.owner.Lock()
	defer core.owner.Unlock()
	if core.owner.ClosedLocked() || !core.owner.AvailableLocked() {
		return corecontract.AttachmentControlRead{}, values.PublicUnavailable
	}
	if err := core.admission.Validate(ctx, caller, view, requirements, false, permit); err != nil {
		return corecontract.AttachmentControlRead{}, err
	}
	r := corecontract.AttachmentControlRead{Read: corecontract.FacetAttachmentRead{Authority: core.owner.AuthorityRef(), Element: key.Element, State: corecontract.ExistenceNotFound, Cut: core.owner.CurrentCutLocked()}}
	if core.owner.FactsLocked().Elements.Kind(key.Element) != 0 {
		record, ok := core.owner.FactsLocked().Attachments.Record(key.Element)
		if !ok {
			return corecontract.AttachmentControlRead{}, values.PublicUnavailable
		}
		r.Read.State, r.Read.Attachment = corecontract.ExistencePresent, record.Attachment
		if record.Attachment.Key == key {
			r.Proposal = core.owner.SigningLocked().AttachmentRevision(key.Proposal)
		}
	}
	if prepare {
		parent, err := core.registry.IssueLocalReadLocked(caller, key.Element, accessregistry.Ref{}, core.owner.CurrentCutLocked(), values.AuthorizationView{}, nil, "", "")
		if err != nil {
			return corecontract.AttachmentControlRead{}, err
		}
		retainedView = core.registry.OwnAuthorizationReleaseLocked(parent.Ref, core.admission.PermitRelease(permit))
		prep, err := core.registry.PrepareControlLocked(parent.Ref, requirements, view, core.owner.CurrentCutLocked(), key)
		if err != nil {
			core.registry.RemoveLocked(parent.Ref)
			return corecontract.AttachmentControlRead{}, err
		}
		r.Preparation = prep.Token
	}
	return r, nil
}

// ReadSuccessorControl is the fixed Facet service's trusted read of one
// element's durable successor statement at the Core the element left. The
// caller is independently established as the fixed Facet service; a NOT_FOUND
// result is a proven exact absence, never a transfer completion proof.
func (core *Operations) ReadSuccessorControl(ctx context.Context, caller values.CallerContext, element values.ElementID) (corecontract.FacetSuccessorRead, error) {
	if core == nil || core.registry == nil {
		return corecontract.FacetSuccessorRead{}, values.PublicUnavailable
	}
	if ctx == nil || !element.Valid() {
		return corecontract.FacetSuccessorRead{}, values.PublicInvalid
	}
	if err := core.admission.Acquire(); err != nil {
		return corecontract.FacetSuccessorRead{}, err
	}
	defer core.admission.Release()
	requirements := []values.Requirement{core.requirement(corecontract.PermissionFacetAttachmentRead, element)}
	view, permit, err := core.admission.Authorize(ctx, caller, requirements)
	defer core.admission.ReleasePermit(ctx, permit)
	if err != nil {
		return corecontract.FacetSuccessorRead{}, err
	}
	core.owner.Lock()
	defer core.owner.Unlock()
	if core.owner.ClosedLocked() || !core.owner.AvailableLocked() {
		return corecontract.FacetSuccessorRead{}, values.PublicUnavailable
	}
	if err := core.admission.Validate(ctx, caller, view, requirements, false, permit); err != nil {
		return corecontract.FacetSuccessorRead{}, err
	}
	return core.successorReadLocked(element)
}

// CheckAttachmentControl checks a pending service preparation without
// activating it or moving a cursor. It is not an arbitrary client read port.
// The retained preparation view is rechecked online outside the lock; the
// in-lock steps re-resolve the same token and guard with the minted permit.
func (core *Operations) CheckAttachmentControl(ctx context.Context, caller values.CallerContext, key values.FacetAcceptanceKey, token values.PublicToken) error {
	if core == nil || core.registry == nil {
		return values.PublicUnavailable
	}
	if ctx == nil || !key.Valid() || key.Core != core.owner.AuthorityRef() {
		return values.PublicInvalid
	}
	core.owner.Lock()
	captured, err := core.controlPreparationLocked(caller, accessvalue.TokenJSON(token))
	core.owner.Unlock()
	if err != nil {
		return err
	}
	// A permanent gap and an unusable shape short-circuit before the online
	// recheck: no authorization call decides a preparation that cannot run.
	if captured.Gap {
		return values.PublicGap
	}
	if !captured.HasQueue || !captured.Selection.Control || captured.ControlKey != key {
		return values.PublicInvalid
	}
	permit, err := core.admission.Recheck(ctx, caller, captured.View, captured.Requirements, true)
	if err != nil {
		// A denial or changed basis permanently gaps the same preparation,
		// exactly as the original in-lock failure did; unavailability keeps it
		// retryable.
		if err != values.PublicUnavailable {
			core.owner.Lock()
			if current, e := core.controlPreparationLocked(caller, accessvalue.TokenJSON(token)); e == nil && current.Ref == captured.Ref {
				core.registry.GapLocked(captured.Ref)
			}
			core.owner.Unlock()
		}
		return err
	}
	core.owner.Lock()
	defer core.owner.Unlock()
	prep, err := core.controlPreparationLocked(caller, accessvalue.TokenJSON(token))
	if err != nil {
		return err
	}
	if prep.Ref != captured.Ref {
		return values.PublicInvalidContext
	}
	if !prep.HasQueue || !prep.Selection.Control || prep.ControlKey != key {
		return values.PublicInvalid
	}
	if prep.Gap {
		return values.PublicGap
	}
	if err := core.admission.Validate(ctx, caller, prep.View, prep.Requirements, true, permit); err != nil {
		if err != values.PublicUnavailable {
			core.registry.GapLocked(prep.Ref)
		}
		return err
	}
	return nil
}

func (core *Operations) controlPreparationLocked(caller values.CallerContext, raw any) (accessregistry.PreparationUse, error) {
	if core.owner.ClosedLocked() || !core.owner.AvailableLocked() {
		return accessregistry.PreparationUse{}, values.PublicUnavailable
	}
	token, err := accessvalue.Token(raw, "observation-preparation")
	if err != nil || token.Owner != core.owner.AuthorityRef() {
		return accessregistry.PreparationUse{}, values.PublicInvalid
	}
	return core.registry.ResolvePreparationLocked(caller, token)
}
