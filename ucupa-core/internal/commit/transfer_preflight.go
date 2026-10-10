package commit

import (
	"slices"

	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	"ucupa/ucupa-core/internal/token"
	transferfact "ucupa/ucupa-core/internal/transfer/fact"
)

// A non-durable target cannot promise to participate in an ownership transfer.
// This exposes readiness only, never a Store or permission decision.
func (owner *Owner) CheckTransferTargetAdmissionLocked() error {
	if err := owner.transferPreflightReadyLocked(); err != nil {
		return err
	}
	if owner.store == nil {
		return values.PublicUnavailable
	}
	return nil
}

// PreflightTransferSourceLocked captures a bounded observation from this exact
// owner under its existing lock. It neither authorizes nor fences the slice.
// A later fence must recapture/recheck it in its actual durable transaction.
func (owner *Owner) PreflightTransferSourceLocked(boundary values.ElementID, expected contract.Revision, maxRecords int) (transferfact.Slice, error) {
	if !boundary.Valid() || !token.ValidRevision(expected) || token.RevisionElement(expected) != boundary || maxRecords <= 0 {
		return transferfact.Slice{}, contract.ErrInvalid
	}
	if err := owner.transferPreflightReadyLocked(); err != nil {
		return transferfact.Slice{}, err
	}
	s := owner.state
	if err := s.guardTransferScope(boundary); err != nil {
		return transferfact.Slice{}, err
	}
	var actual contract.Revision
	if a, ok := s.elements.Anchor(boundary); ok {
		actual = a.Revision
	} else if e, ok := s.elements.Entity(boundary); ok {
		actual = e.Revision
	} else {
		return transferfact.Slice{}, contract.ErrNotFound
	}
	if !actual.Equal(expected) {
		return transferfact.Slice{}, contract.ErrRevisionConflict
	}
	slice, err := transferfact.Select(s.authority, s.currentCut(), s.elements.View,
		s.attachments.facts.View, s.mount.facts.View, boundary, maxRecords)
	if err != nil {
		return transferfact.Slice{}, err
	}
	return s.captureTransferEvidence(slice)
}

// PreflightTransferTargetLocked preserves the caller's exact final tuple.
// This checks facts only: it cannot stand in for target authorization, source
// authentication, complete transfer validation, or durable placement retention.
func (owner *Owner) PreflightTransferTargetLocked(source transferfact.Slice, final contract.SpatialTuple, maxRecords int) (transferfact.Placement, error) {
	if err := owner.transferPreflightReadyLocked(); err != nil {
		return transferfact.Placement{}, err
	}
	s := owner.state
	return s.planTransferTarget(source, final, maxRecords)
}

// Incoming UUID promises are current control facts, even before installation.
// Adding disjoint children to a held ancestor does not change its recipe and
// must not be confused with deleting, moving or fencing that ancestor itself.
func (s *coreState) planTransferTarget(source transferfact.Slice, final contract.SpatialTuple, maxRecords int) (transferfact.Placement, error) {
	returns, err := s.transferReturns(source)
	if err != nil {
		return transferfact.Placement{}, err
	}
	for _, member := range source.Members() {
		if _, returning := returns[member.ID()]; returning {
			for _, accepted := range s.transfers.targets {
				if !accepted.complete() && slices.Contains(accepted.members, member.ID()) {
					return transferfact.Placement{}, contract.ErrConflict
				}
			}
			continue
		}
		if err := s.guardTransferElement(member.ID()); err != nil {
			return transferfact.Placement{}, err
		}
	}
	if err := s.guardTransferElement(final.Parent); err != nil {
		return transferfact.Placement{}, err
	}
	return transferfact.Place(source, s.authority, s.currentCut(), s.elements, s.mount.facts.View, final, maxRecords, returns)
}

func (owner *Owner) transferPreflightReadyLocked() error {
	if owner == nil || owner.ClosedLocked() || !owner.AvailableLocked() {
		return values.PublicUnavailable
	}
	if owner.state.authority != owner.authority {
		return contract.ErrStore
	}
	if err := owner.ValidateCurrentLocked(); err != nil {
		return contract.ErrStore
	}
	return nil
}
