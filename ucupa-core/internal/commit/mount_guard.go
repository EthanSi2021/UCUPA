package commit

import (
	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	commitbinding "ucupa/ucupa-core/internal/commit/binding"
	elementfact "ucupa/ucupa-core/internal/element/fact"
	"ucupa/ucupa-core/internal/geometry"
	mountfact "ucupa/ucupa-core/internal/mount/fact"
)

// Receipt replay precedes this gate. A refusal here never claims OperationID.
func (state *coreState) guardOrdinaryMutation(binding commitbinding.Binding) error {
	if binding.Kind == elementfact.OperationCreateRoot {
		if err := state.guardTransferRootActivation(); err != nil {
			return err
		}
	}
	if err := state.guardTransferElement(binding.Element); err != nil {
		return err
	}
	if err := state.guardTransferElement(binding.Final.Parent); err != nil {
		return err
	}
	if binding.Kind == elementfact.OperationUpdateAnchor || binding.Kind == elementfact.OperationDeleteAnchor {
		if err := state.guardTransferScope(binding.Element); err != nil {
			return err
		}
	}
	// An open re-acceptance or recovery proposal is a durable obligation to
	// the receiving Facet: the Element cannot be deleted while the new
	// acceptance may already exist without its activation response. A DETACHED
	// relationship without an open proposal still holds the retired-content
	// lineage: release needs the stored record, so deletion stays blocked until
	// the final disposition retires it.
	if binding.Kind == elementfact.OperationDeleteAnchor || binding.Kind == elementfact.OperationDeleteEntity {
		if r, ok := state.attachments.facts.Record(binding.Element); ok && (r.ReacceptancePending() || r.RecoveryPending() || r.Attachment.State == contract.FacetDetached) {
			return contract.ErrConflict
		}
	}
	if state.mount.facts.Incoming().Key.Valid() && state.mount.facts.Incoming().Gate != mountfact.GateOpen {
		return contract.ErrMountBusy
	}
	if binding.Kind != elementfact.OperationUpdateAnchor {
		return nil
	}
	mount, exists := state.mount.facts.Outgoing(binding.Element)
	if !exists || mount.State == contract.MountNone {
		return nil
	}
	if mount.Slot != "" {
		return contract.ErrMountBusy
	}
	anchor, exists := state.elements.Anchor(binding.Element)
	if !exists {
		return contract.ErrStore
	}
	rangeValue, bounded := anchor.Range.BoundedSphere()
	if !bounded {
		return contract.ErrStore
	}
	if !geometry.EqualSphere(rangeValue, binding.Final.Range) {
		return contract.ErrMountCoordinationRequired
	}
	return nil
}

func ExternalMountParentForView(mounts mountfact.View) (values.ElementID, contract.BoundedSphere, bool) {
	incoming := mounts.Incoming()
	if !incoming.Key.Valid() {
		return "", contract.BoundedSphere{}, false
	}
	rangeValue := incoming.Admission.RangeValue
	if !mountfact.ValidMountAdmissionKey(incoming.Admission.Key) {
		rangeValue = incoming.Acceptance.Tuple.Range
	}
	return incoming.Key.MountAnchorUUID, rangeValue, true
}

// The relationship owner supplies only the external-parent evidence required
// by element rules; no mount state or callback crosses into fact.
func (state *coreState) elementExternalParent() elementfact.ExternalParent {
	return ElementExternalParentForView(state.mount.facts.View)
}

func ElementExternalParentForView(mounts mountfact.View) elementfact.ExternalParent {
	id, rangeValue, present := ExternalMountParentForView(mounts)
	incoming := mounts.Incoming()
	return elementfact.ExternalParent{ID: id, Range: rangeValue, Present: present,
		Admitted: mountfact.ValidMountAdmissionKey(incoming.Admission.Key)}
}
