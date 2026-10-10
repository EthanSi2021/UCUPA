package commit

import (
	"slices"
	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	mountfact "ucupa/ucupa-core/internal/mount/fact"
	transferfact "ucupa/ucupa-core/internal/transfer/fact"
)

// A departed fact is unavailable at this owner, not proven nonexistent. Original
// operation lookup must precede this admission guard; refusal claims no new ID.
func (s *coreState) guardTransferElement(id values.ElementID) error {
	if s.mount.facts.Incoming().Gate == mountfact.GateReparenting {
		return contract.ErrMountBusy
	}
	if _, departed := s.elements.Departure(id); departed {
		return values.PublicUnavailable
	}
	for _, accepted := range s.transfers.targets {
		if accepted.complete() {
			continue
		}
		if accepted.reservesWholeSpace() {
			return contract.ErrConflict
		}
		if slices.Contains(accepted.members, id) {
			return contract.ErrConflict
		}
	}
	// An open facet namespace replacement holds the element's current control:
	// no attachment change, Element lifecycle change or competing transfer may
	// interleave before one of its permanent outcomes.
	if s.facetReplacementOpen(id) {
		return contract.ErrConflict
	}
	return nil
}

func (s *coreState) facetReplacementOpen(id values.ElementID) bool {
	for _, c := range s.transfers.replacements {
		if !c.Terminal() && c.Input.Element == id {
			return true
		}
	}
	return false
}

// guardTransferScope protects the exact source ancestor recipe until source
// settlement proves the continuation. Fenced records are bounded control values;
// ordinary writes do not decode manifests or replay the journal. The ancestor
// chain cannot change while held, so its live parent links remain authoritative.
// Adding or modifying unrelated children is not a mutation of this recipe.
func (s *coreState) guardTransferScope(id values.ElementID) error {
	if err := s.guardTransferElement(id); err != nil {
		return err
	}
	for _, relation := range s.mount.facts.Outgoings() {
		if relation.Slot == "" {
			continue
		}
		operation := s.mount.operations[relation.Slot]
		if p := operation.Parent; p != (transferfact.ParentControl{}) && p.ReleasedAt == (contract.CoreReadCut{}) && s.elements.DescendsFrom(p.Hold.Input.Mount, id) {
			return contract.ErrMountBusy
		}
		c := operation.Carrier
		if c != (transferfact.CarrierControl{}) && c.CancelledAt == (contract.CoreReadCut{}) && s.elements.DescendsFrom(c.Reservation.Input.Mount, id) {
			return contract.ErrMountBusy
		}
	}
	for _, accepted := range s.transfers.targets {
		if accepted.complete() {
			continue
		}
		parent := accepted.receipt.Fence.Input.Final.Parent
		if accepted.reservesWholeSpace() {
			return contract.ErrConflict
		}
		if _, exists := s.elements.Anchor(parent); !exists {
			if accepted.parent != (transferfact.ParentHold{}) && parent == accepted.parent.Input.Mount {
				continue // the upstream owner holds the external parent recipe
			}
			return contract.ErrStore
		}
		if s.elements.DescendsFrom(parent, id) {
			return contract.ErrConflict
		}
	}
	return s.guardSourceTransferScope(id)
}

func (s *coreState) guardSourceTransferScope(id values.ElementID) error {
	for _, record := range s.transfers.records {
		if !record.SourcePending() {
			continue
		}
		if record.Input.Profile == transferfact.RootSuccessor {
			return values.PublicUnavailable
		}
		if _, exists := s.elements.Anchor(record.Origin.Parent); !exists {
			if in := s.mount.facts.Incoming(); in.Key.Valid() && record.Origin.Parent == in.Key.MountAnchorUUID {
				continue // parent-side hold protects this external ancestor
			}
			return contract.ErrStore
		}
		if s.elements.DescendsFrom(record.Origin.Parent, id) {
			return values.PublicUnavailable
		}
	}
	return nil
}

// Root activation is one reserved slot, not a fabricated parent Element.
// Both ordinary CreateRoot and incoming mount adoption must consult it.
func (s *coreState) guardTransferRootActivation() error {
	if _, retired := s.elements.RetiredRoot(); retired {
		return values.PublicUnavailable
	}
	for _, accepted := range s.transfers.targets {
		if accepted.reservesWholeSpace() {
			return contract.ErrConflict
		}
	}
	return nil
}
