package facet

import (
	"context"

	"ucupa/shared/values"
	corecontract "ucupa/ucupa-core/contract"
	commitbinding "ucupa/ucupa-core/internal/commit/binding"
	facetfact "ucupa/ucupa-core/internal/facet/fact"
)

// SetFacetAcceptanceSource installs trusted local assembly, not a request's
// caller-supplied proof. Attachment writes work without it except Activate.
func (core *Operations) SetFacetAcceptanceSource(source corecontract.FacetAcceptanceSource) error {
	if core == nil || source == nil {
		return corecontract.ErrInvalid
	}
	core.owner.Lock()
	defer core.owner.Unlock()
	if core.owner.ClosedLocked() || !core.owner.AvailableLocked() {
		return corecontract.ErrStore
	}
	if core.facetSource != nil {
		return corecontract.ErrConflict
	}
	core.facetSource = source
	return nil
}

// SetFacetDispositionSource installs the trusted final-disposition reader used
// only by Release. It is consumer-owned assembly, never caller-supplied proof.
func (core *Operations) SetFacetDispositionSource(source corecontract.FacetDispositionSource) error {
	if core == nil || source == nil {
		return corecontract.ErrInvalid
	}
	core.owner.Lock()
	defer core.owner.Unlock()
	if core.owner.ClosedLocked() || !core.owner.AvailableLocked() {
		return corecontract.ErrStore
	}
	if core.facetDispositionSource != nil {
		return corecontract.ErrConflict
	}
	core.facetDispositionSource = source
	return nil
}

func (core *Operations) ProposeFacet(actor values.PrincipalRef, id values.ElementID, target values.FactAuthorityRef, address values.NextHop, expected values.AttachmentRevision, operation corecontract.OperationID) (corecontract.FacetAttachmentResult, error) {
	return core.executeAttachment(context.Background(), operation, facetfact.Binding{Actor: actor, Kind: facetfact.Propose, Element: id, Target: target, Address: address, Expected: expected})
}

func (core *Operations) CancelFacetProposal(actor values.PrincipalRef, id values.ElementID, expected values.AttachmentRevision, operation corecontract.OperationID) (corecontract.FacetAttachmentResult, error) {
	return core.executeAttachment(context.Background(), operation, facetfact.Binding{Actor: actor, Kind: facetfact.Cancel, Element: id, Expected: expected})
}

func (core *Operations) ActivateFacet(ctx context.Context, actor values.PrincipalRef, id values.ElementID, expected values.AttachmentRevision, operation corecontract.OperationID) (corecontract.FacetAttachmentResult, error) {
	return core.executeAttachment(ctx, operation, facetfact.Binding{Actor: actor, Kind: facetfact.Activate, Element: id, Expected: expected})
}

func (core *Operations) DetachFacet(actor values.PrincipalRef, id values.ElementID, expected values.AttachmentRevision, operation corecontract.OperationID) (corecontract.FacetAttachmentResult, error) {
	return core.executeAttachment(context.Background(), operation, facetfact.Binding{Actor: actor, Kind: facetfact.Detach, Element: id, Expected: expected})
}

// ReleaseFacet releases control occupancy for the retired Facet content. The
// trusted disposition source must confirm the final disposition at the
// recorded next hop before the release decision is made.
func (core *Operations) ReleaseFacet(ctx context.Context, actor values.PrincipalRef, id values.ElementID, target values.FactAuthorityRef, address values.NextHop, expected values.AttachmentRevision, operation corecontract.OperationID) (corecontract.FacetAttachmentResult, error) {
	return core.executeAttachment(ctx, operation, facetfact.Binding{Actor: actor, Kind: facetfact.Release, Element: id, Target: target, Address: address, Expected: expected})
}

func (core *Operations) UpdateFacetNextHop(actor values.PrincipalRef, operation corecontract.OperationID, key values.FacetAcceptanceKey, expected corecontract.FacetContactRevision, address values.NextHop) (corecontract.FacetAttachmentResult, error) {
	return core.executeAttachment(context.Background(), operation, facetfact.Binding{Actor: actor, Kind: facetfact.Address, Element: key.Element, Key: key, Contact: expected, Address: address})
}

func (core *Operations) executeAttachment(ctx context.Context, id corecontract.OperationID, binding facetfact.Binding) (corecontract.FacetAttachmentResult, error) {
	return core.executeAttachmentGuarded(ctx, id, binding, nil)
}

// The optional private guard retains the original public request view; it is
// not a caller-supplied callback and runs at the unchanged commit boundaries.
// A guarded call already carries the permit minted by the lock-free admission
// authorize, so the typed principal authorize and its contact-generation
// settle run only on the unguarded local channel; every in-lock boundary of
// the guarded path consumes only that one permit.
func (core *Operations) executeAttachmentGuarded(ctx context.Context, id corecontract.OperationID, binding facetfact.Binding, guard *controlAuthorization) (corecontract.FacetAttachmentResult, error) {
	if core == nil || ctx == nil || !commitbinding.ValidOperationID(id) || !binding.Valid() {
		return corecontract.FacetAttachmentResult{}, corecontract.ErrInvalid
	}
	if binding.Kind == facetfact.Address && binding.Key.Core != core.owner.AuthorityRef() {
		return corecontract.FacetAttachmentResult{}, corecontract.ErrInvalid
	}
	var approval corecontract.RealmAttachment
	var err error
	if guard == nil {
		approval, err = core.authorize(binding.Actor, core.requirement(binding.Permission(), binding.Element))
		if err != nil {
			return corecontract.FacetAttachmentResult{}, err
		}
	}
	if err := ctx.Err(); err != nil {
		return corecontract.FacetAttachmentResult{}, err
	}
	core.owner.Lock()
	if core.owner.ClosedLocked() || !core.owner.AvailableLocked() {
		core.owner.Unlock()
		return corecontract.FacetAttachmentResult{}, corecontract.ErrStore
	}
	if guard == nil && !core.settled(approval) {
		core.owner.Unlock()
		return corecontract.FacetAttachmentResult{}, corecontract.ErrUnavailable
	}
	if guard != nil {
		if err := guard.validate(core.admission); err != nil {
			core.owner.Unlock()
			return corecontract.FacetAttachmentResult{}, err
		}
	}
	if result, err, found := core.owner.LookupFacetLocked(id, binding); found {
		core.owner.Unlock()
		return result, err
	}
	var evidence corecontract.FacetAcceptanceEvidence
	var disposition corecontract.FacetDispositionRead
	// Preserve the same error order before making any external call. A second
	// decision after the call protects against concurrent cancel/delete/change.
	preliminary := core.owner.PreviewFacetLocked(binding, evidence, disposition)
	if binding.Kind == facetfact.Activate && preliminary == facetfact.AcceptanceMissing {
		record, _ := core.owner.FactsLocked().Attachments.Record(binding.Element)
		source := core.facetSource
		core.owner.Unlock()
		if source == nil {
			return corecontract.FacetAttachmentResult{}, corecontract.ErrUnavailable
		}
		evidence, err = source.ReadFacetAcceptance(ctx, record.NextHop, record.Pending)
		if err != nil {
			return corecontract.FacetAttachmentResult{}, corecontract.ErrUnavailable
		}
		if evidence != (corecontract.FacetAcceptanceEvidence{}) && (evidence.Key != record.Pending || !evidence.AcceptedAt.Valid()) {
			return corecontract.FacetAttachmentResult{}, corecontract.ErrUnavailable
		}
		if err := ctx.Err(); err != nil {
			return corecontract.FacetAttachmentResult{}, err
		}
		if guard == nil {
			approval, err = core.authorize(binding.Actor, core.requirement(binding.Permission(), binding.Element))
			if err != nil {
				return corecontract.FacetAttachmentResult{}, err
			}
		}
		core.owner.Lock()
		if core.owner.ClosedLocked() || !core.owner.AvailableLocked() {
			core.owner.Unlock()
			return corecontract.FacetAttachmentResult{}, corecontract.ErrStore
		}
		if guard == nil && !core.settled(approval) {
			core.owner.Unlock()
			return corecontract.FacetAttachmentResult{}, corecontract.ErrUnavailable
		}
		if guard != nil {
			if err := guard.validate(core.admission); err != nil {
				core.owner.Unlock()
				return corecontract.FacetAttachmentResult{}, err
			}
		}
		if result, err, found := core.owner.LookupFacetLocked(id, binding); found {
			core.owner.Unlock()
			return result, err
		}
	}
	if binding.Kind == facetfact.Release && preliminary == facetfact.DispositionMissing {
		source := core.facetDispositionSource
		core.owner.Unlock()
		if source == nil {
			return corecontract.FacetAttachmentResult{}, corecontract.ErrUnavailable
		}
		disposition, err = source.ReadFacetDisposition(ctx, binding.Address, binding.Target, binding.Element)
		if err != nil {
			return corecontract.FacetAttachmentResult{}, corecontract.ErrUnavailable
		}
		if disposition != (corecontract.FacetDispositionRead{}) && (!disposition.Found || !disposition.Key.Valid() || !disposition.Cut.Valid() ||
			disposition.Facet != binding.Target || disposition.Element != binding.Element) {
			return corecontract.FacetAttachmentResult{}, corecontract.ErrUnavailable
		}
		if err := ctx.Err(); err != nil {
			return corecontract.FacetAttachmentResult{}, err
		}
		if guard == nil {
			approval, err = core.authorize(binding.Actor, core.requirement(binding.Permission(), binding.Element))
			if err != nil {
				return corecontract.FacetAttachmentResult{}, err
			}
		}
		core.owner.Lock()
		if core.owner.ClosedLocked() || !core.owner.AvailableLocked() {
			core.owner.Unlock()
			return corecontract.FacetAttachmentResult{}, corecontract.ErrStore
		}
		if guard == nil && !core.settled(approval) {
			core.owner.Unlock()
			return corecontract.FacetAttachmentResult{}, corecontract.ErrUnavailable
		}
		if guard != nil {
			if err := guard.validate(core.admission); err != nil {
				core.owner.Unlock()
				return corecontract.FacetAttachmentResult{}, err
			}
		}
		if result, err, found := core.owner.LookupFacetLocked(id, binding); found {
			core.owner.Unlock()
			return result, err
		}
	}
	defer core.owner.Unlock()
	result, code, err := core.owner.ApplyFacetLocked(id, binding, evidence, disposition)
	if err != nil {
		return corecontract.FacetAttachmentResult{}, err
	}
	if guard != nil {
		if err := guard.validate(core.admission); err != nil {
			return corecontract.FacetAttachmentResult{}, err
		}
	}
	return result, code.Err()
}
