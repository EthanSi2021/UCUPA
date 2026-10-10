package commit

import (
	"errors"
	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	"ucupa/ucupa-core/internal/token"
	transferencoding "ucupa/ucupa-core/internal/transfer/encoding"
	transferfact "ucupa/ucupa-core/internal/transfer/fact"
)

// applyTransferEvent is shared by live submission and the original Core
// replay. Intent is not a retained spatial preflight: control relations may
// change later at the same data cut. Fence will recapture them in its own cut.
func (s *coreState) applyTransferEvent(e transferencoding.TransferEvent) error {
	if e.Kind != transferencoding.TransferTargetAccepted && e.TargetParent != "" {
		return contract.ErrStore
	}
	if e.Kind != transferencoding.TransferParentReserved && e.Parent != (transferfact.ParentIntent{}) || e.Kind != transferencoding.TransferParentReleased && e.ParentRelease != (transferfact.ParentRelease{}) {
		return contract.ErrStore
	}
	if e.Kind != transferencoding.TransferTargetCompleted && e.EdgeResults != "" {
		return contract.ErrStore
	}
	if e.Kind != transferencoding.TransferSourceSettled && e.Kind != transferencoding.TransferCarrierParentSettled && e.Kind != transferencoding.TransferCarrierSourceSettled && e.Completion != (transferfact.CoreCompletion{}) {
		return contract.ErrStore
	}
	if e.Kind != transferencoding.TransferEdgeInstalled && (e.EdgeParent != (transferfact.CoreInstallation{}) || e.EdgeMount != "") {
		return contract.ErrStore
	}
	if e.Kind != transferencoding.TransferTargetAccepted && e.Kind != transferencoding.TransferCarrierAccepted && e.Closures != "" {
		return contract.ErrStore
	}
	if e.Kind != transferencoding.TransferEdgeClosed && e.EdgeFence != (transferfact.EdgeFence{}) {
		return contract.ErrStore
	}
	if e.Kind != transferencoding.TransferCarrierParentSettled && e.Kind != transferencoding.TransferCarrierSourceSettled && e.CarrierInstallation != (transferfact.CarrierInstallation{}) {
		return contract.ErrStore
	}
	if e.Kind != transferencoding.TransferCarrierInstalled && e.CarrierBinding != (transferfact.CarrierBinding{}) {
		return contract.ErrStore
	}
	if e.Kind != transferencoding.TransferCarrierBound && e.CarrierAcceptance != (transferfact.CarrierAcceptance{}) {
		return contract.ErrStore
	}
	if e.Kind != transferencoding.TransferCarrierAccepted && e.CarrierFence != (transferfact.CarrierFence{}) {
		return contract.ErrStore
	}
	if e.Kind != transferencoding.TransferCarrierRefused && e.Kind != transferencoding.TransferCarrierFenced && e.CarrierPermit != (transferfact.CarrierControl{}) ||
		e.Kind != transferencoding.TransferCarrierAborted && e.CarrierRefusal != (transferfact.CarrierRefusal{}) {
		return contract.ErrStore
	}
	if e.At != s.currentCut() || len(s.transfers.events) >= transferencoding.MaxTransferEvents || (e.Kind != transferencoding.TransferTargetAccepted && e.Fence != (transferfact.SourceFence{})) {
		return contract.ErrStore
	}
	if e.Kind != transferencoding.TransferSuccessorDecided && e.Acceptance != (transferfact.TargetAcceptance{}) {
		return contract.ErrStore
	}
	if e.Kind != transferencoding.TransferTargetInstalled && e.Decision != (transferfact.SourceDecision{}) {
		return contract.ErrStore
	}
	if e.Kind != transferencoding.TransferSourceSettled && e.Installation != (transferfact.TargetInstallation{}) {
		return contract.ErrStore
	}
	if e.Kind != transferencoding.TransferCarrierReserved && e.Carrier != (transferfact.CarrierIntent{}) ||
		e.Kind != transferencoding.TransferCarrierPermitted && e.Kind != transferencoding.TransferCarrierCancelled && e.CarrierExpected != (contract.CoreReadCut{}) {
		return contract.ErrStore
	}
	if e.Kind != transferencoding.TransferFacetReserved && e.Facet != (transferfact.FacetReplacementIntent{}) ||
		e.Kind != transferencoding.TransferFacetFenced && e.FacetFence != (transferfact.FacetReplacementFence{}) ||
		e.Kind != transferencoding.TransferFacetRefused && e.FacetRefusal != (transferfact.FacetReplacementRefusal{}) {
		return contract.ErrStore
	}
	switch e.Kind {
	case transferencoding.TransferParentReserved, transferencoding.TransferParentReleased:
		if err := s.applyTransferParent(e); err != nil {
			return err
		}
	case transferencoding.TransferTargetCompleted:
		if err := s.applyTargetCompletion(e); err != nil {
			return err
		}
	case transferencoding.TransferEdgeInstalled:
		if err := s.applyEdgeInstallation(e); err != nil {
			return err
		}
	case transferencoding.TransferEdgeClosed:
		if err := s.applyTransferEdgeClosure(e); err != nil {
			return err
		}
	case transferencoding.TransferCarrierParentSettled, transferencoding.TransferCarrierSourceSettled:
		if err := s.applyCarrierSettlement(e); err != nil {
			return err
		}
	case transferencoding.TransferCarrierInstalled:
		if err := s.applyCarrierInstallation(e); err != nil {
			return err
		}
	case transferencoding.TransferCarrierBound:
		if err := s.applyCarrierBinding(e); err != nil {
			return err
		}
	case transferencoding.TransferCarrierAccepted:
		if err := s.applyCarrierAcceptance(e); err != nil {
			return err
		}
	case transferencoding.TransferCarrierFenced:
		if err := s.applyCarrierFence(e); err != nil {
			return err
		}
	case transferencoding.TransferCarrierRefused, transferencoding.TransferCarrierAborted:
		if err := s.applyCarrierRefusal(e); err != nil {
			return err
		}
	case transferencoding.TransferCarrierReserved, transferencoding.TransferCarrierPermitted, transferencoding.TransferCarrierCancelled:
		if err := s.applyCarrierControl(e); err != nil {
			return err
		}
	case transferencoding.TransferPending:
		i := e.Input
		if !i.Valid() || i.Source != s.authority || e.Operation != "" || e.Actor != (values.PrincipalRef{}) || e.Expected != 0 || e.Manifest != "" {
			return contract.ErrInvalid
		}
		if _, err := transferencoding.EncodeTransferIntent(i); err != nil {
			return err
		}
		if s.transferIntentCount() >= transferencoding.MaxTransferIntents {
			return values.PublicResourceExhausted
		}
		if _, ok := s.operations[i.Operation]; ok || s.mountOperationClaimed(i.Operation) {
			return contract.ErrOperationConflict
		}
		if err := s.guardTransferScope(i.Boundary); err != nil {
			return err
		}
		var revision contract.Revision
		if a, ok := s.elements.Anchor(i.Boundary); ok {
			if a.Root() != (i.Profile == transferfact.RootSuccessor) {
				return contract.ErrConflict
			}
			revision = a.Revision
		} else if v, ok := s.elements.Entity(i.Boundary); ok {
			if i.Profile != transferfact.SliceTransfer {
				return contract.ErrConflict
			}
			revision = v.Revision
		} else {
			return contract.ErrNotFound
		}
		if revision != i.Expected {
			return contract.ErrRevisionConflict
		}
		s.transfers.records[i.Operation] = transferfact.IntentRecord{Input: i, Progress: transferfact.IntentPending, Revision: uint64(len(s.transfers.events)) + 1, Admission: e.At}
	case transferencoding.TransferCancelled:
		if e.Input != (transferfact.Intent{}) || !e.Actor.Valid() || e.Expected == 0 || e.Manifest != "" {
			return contract.ErrInvalid
		}
		r, ok := s.transfers.records[e.Operation]
		if !ok || r.Progress != transferfact.IntentPending || r.Revision != e.Expected || r.Input.Actor != e.Actor {
			return contract.ErrConflict
		}
		r.Progress, r.Revision = transferfact.IntentCancelled, uint64(len(s.transfers.events))+1
		s.transfers.records[e.Operation] = r
	case transferencoding.TransferFenced:
		if err := s.applyTransferFence(e); err != nil {
			return err
		}
	case transferencoding.TransferTargetAccepted:
		if err := s.applyTransferAcceptance(e); err != nil {
			return err
		}
	case transferencoding.TransferSuccessorDecided:
		if err := s.applyTransferSuccessor(e); err != nil {
			return err
		}
	case transferencoding.TransferTargetInstalled:
		if err := s.applyTransferInstallation(e); err != nil {
			return err
		}
	case transferencoding.TransferSourceSettled:
		if err := s.applyTransferSettlement(e); err != nil {
			return err
		}
	case transferencoding.TransferFacetReserved, transferencoding.TransferFacetPermitted, transferencoding.TransferFacetFenced,
		transferencoding.TransferFacetAccepted, transferencoding.TransferFacetCommitted, transferencoding.TransferFacetInstalled,
		transferencoding.TransferFacetSettled, transferencoding.TransferFacetCancelled, transferencoding.TransferFacetRefused:
		if err := s.applyFacetReplacement(e); err != nil {
			return err
		}
	default:
		return contract.ErrInvalid
	}
	s.transfers.events = append(s.transfers.events, e)
	return nil
}

// Replay uses the already reconstructed Element cut, preserving exact input
// bytes and the original owner's result namespace. It does not authorize anew.
func replayTransferDecisions(replay *coreState, events []transferencoding.TransferEvent, index *int) error {
	for *index < len(events) {
		e := events[*index]
		if token.Scope(e.At) != replay.scope || !token.Valid(e.At) || token.Serial(e.At) < replay.cutSerial {
			return errors.New("transfer journal cut is invalid or out of order")
		}
		if token.Serial(e.At) > replay.cutSerial {
			return nil
		}
		if transferCommitsCut(e.Kind) {
			return nil
		} // consumed only by its ordered cut commit
		if err := replay.applyTransferEvent(e); err != nil {
			return err
		}
		*index++
	}
	return nil
}

func replayTransferCommit(replay *coreState, events []transferencoding.TransferEvent, index *int, commit coreCommit, issued map[contract.Revision]uint64) error {
	if commit.kind != transferCommitted || commit.operation != "" || commit.controlIndex != uint64(*index)+1 || *index >= len(events) || !transferCommitsCut(events[*index].Kind) {
		return errors.New("transfer commit does not bind its exact fence")
	}
	if err := replay.applyTransferEvent(events[*index]); err != nil {
		return err
	}
	if e := events[*index]; e.Kind == transferencoding.TransferTargetInstalled || e.Kind == transferencoding.TransferCarrierInstalled {
		key := e.Decision.Acceptance.Fence.Key()
		if e.Kind == transferencoding.TransferCarrierInstalled {
			key = e.CarrierBinding.Acceptance.Fence.Permit.Reservation.Input.Key()
		}
		accepted := replay.transfers.targets[key]
		for _, id := range accepted.members {
			if a, ok := replay.elements.Anchor(id); ok {
				issued[a.Revision] = replay.cutSerial
			} else if v, ok := replay.elements.Entity(id); ok {
				issued[v.Revision] = replay.cutSerial
			} else {
				return contract.ErrStore
			}
		}
	}
	*index++
	return nil
}

func transferCommitsCut(kind transferencoding.TransferEventKind) bool {
	switch kind {
	case transferencoding.TransferParentReserved, transferencoding.TransferParentReleased:
		return true
	case transferencoding.TransferEdgeClosed, transferencoding.TransferEdgeInstalled, transferencoding.TransferTargetCompleted:
		return true
	case transferencoding.TransferCarrierParentSettled, transferencoding.TransferCarrierSourceSettled:
		return true
	case transferencoding.TransferFenced, transferencoding.TransferTargetInstalled, transferencoding.TransferSourceSettled, transferencoding.TransferCarrierReserved, transferencoding.TransferCarrierPermitted, transferencoding.TransferCarrierCancelled, transferencoding.TransferCarrierAborted, transferencoding.TransferCarrierFenced, transferencoding.TransferCarrierBound, transferencoding.TransferCarrierInstalled:
		return true
	case transferencoding.TransferFacetReserved, transferencoding.TransferFacetPermitted, transferencoding.TransferFacetFenced, transferencoding.TransferFacetAccepted, transferencoding.TransferFacetCommitted, transferencoding.TransferFacetInstalled, transferencoding.TransferFacetSettled, transferencoding.TransferFacetCancelled, transferencoding.TransferFacetRefused:
		return true
	}
	return false
}
