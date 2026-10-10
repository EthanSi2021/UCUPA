package commit

import (
	"ucupa/ucupa-core/contract"
	mountfact "ucupa/ucupa-core/internal/mount/fact"
	"ucupa/ucupa-core/internal/token"
	transferfact "ucupa/ucupa-core/internal/transfer/fact"
)

// The runtime obtains this hold from the authenticated, designated parent.
// The transaction checks the precise incoming facts; neither the proof's shape
// nor the source manifest grants permission or appoints a parent for the target.
func (s *coreState) validateTargetParent(f transferfact.SourceFence, h transferfact.ParentHold) error {
	in := s.mount.facts.Incoming()
	if !in.Key.Valid() {
		if h != (transferfact.ParentHold{}) {
			return contract.ErrConflict
		}
		return nil
	}
	if !h.Valid() || h.Input.Role != transferfact.TargetParent || h.Input.Initial.Input != f.Input ||
		h.Input.Carrier() != s.authority || in.Key != h.Admission.MountKey || in.Admission.Key != h.Admission ||
		in.Admission.RangeValue != h.Tuple.Range || in.Detached || in.Gate != mountfact.GateOpen ||
		in.Preparing != (contract.MountAttemptKey{}) || token.Scope(h.Input.Initial.Admission) != token.Scope(f.FinalCut) ||
		token.Serial(h.Input.Initial.Admission) > token.Serial(f.FinalCut) {
		return contract.ErrMountCoordinationRequired
	}
	return nil
}

// These are obligations in the existing acceptance index, not a second gate.
// Local unrelated writes remain admitted while upstream range/revoke must wait.
func (s *coreState) transferHoldsIncoming() bool {
	if s.sourceHoldsIncoming() {
		return true
	}
	for _, r := range s.transfers.targets {
		if !r.complete() && r.parent != (transferfact.ParentHold{}) {
			return true
		}
	}
	return false
}
