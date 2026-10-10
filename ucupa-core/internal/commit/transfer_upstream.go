package commit

import (
	"ucupa/ucupa-core/contract"
	mountfact "ucupa/ucupa-core/internal/mount/fact"
	transferencoding "ucupa/ucupa-core/internal/transfer/encoding"
	transferfact "ucupa/ucupa-core/internal/transfer/fact"
)

// A live fence consumes the exact durable source intent and an authenticated
// parent hold. Its relationship cannot be replaced by a look-alike token or a
// caller-supplied range. The runtime must obtain that hold from the parent.
func (s *coreState) selectTransferSource(i transferfact.Intent, h transferfact.ParentHold) (transferfact.Slice, error) {
	in := s.mount.facts.Incoming()
	if !in.Key.Valid() {
		if h != (transferfact.ParentHold{}) {
			return transferfact.Slice{}, contract.ErrConflict
		}
		return transferfact.Select(s.authority, s.currentCut(), s.elements.View, s.attachments.facts.View, s.mount.facts.View, i.Boundary, transferencoding.MaxTransferManifestRecords)
	}
	r, exists := s.transfers.records[i.Operation]
	if !exists || h.Input.Initial != r || r.Input != i || !h.Valid() || h.Input.Role != transferfact.SourceParent ||
		h.Input.Carrier() != s.authority || in.Key != h.Admission.MountKey || in.Admission.Key != h.Admission ||
		in.Admission.RangeValue != h.Tuple.Range || in.Contact.NextHop != h.Input.ParentAddress ||
		in.Detached || in.Gate != mountfact.GateOpen || in.Preparing != (contract.MountAttemptKey{}) {
		return transferfact.Slice{}, contract.ErrMountCoordinationRequired
	}
	return transferfact.SelectHeld(h, s.currentCut(), s.elements.View, s.attachments.facts.View, s.mount.facts.View, transferencoding.MaxTransferManifestRecords)
}

// An upstream range/revoke cannot overtake a still-unsettled source closure.
// Ordinary unrelated content writes remain legal under the unchanged admission.
func (s *coreState) sourceHoldsIncoming() bool {
	if !s.mount.facts.Incoming().Key.Valid() {
		return false
	}
	for _, r := range s.transfers.records {
		if r.SourcePending() {
			return true
		}
	}
	return false
}
