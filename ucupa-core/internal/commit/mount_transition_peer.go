package commit

import (
	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	commitbinding "ucupa/ucupa-core/internal/commit/binding"
	elementfact "ucupa/ucupa-core/internal/element/fact"
	mountfact "ucupa/ucupa-core/internal/mount/fact"
	"ucupa/ucupa-core/internal/token"
)

func preparedReplay(attempt PeerAttempt, request contract.MountPrepareRequest) (contract.MountPrepared, error) {
	if attempt.Request != request {
		return contract.MountPrepared{}, contract.ErrOperationConflict
	}
	if token.Valid(attempt.Prepared.Cut) {
		return attempt.Prepared, nil
	}
	if !attempt.IsClosed {
		return contract.MountPrepared{}, contract.ErrStore
	}
	return contract.MountPrepared{Request: request, Rejection: contract.MountAbortConflict, Cut: attempt.Closed.Cut}, nil
}

func ValidMountPrepared(prepared contract.MountPrepared) bool {
	if !ValidMountRequest(prepared.Request) || !token.Valid(prepared.Cut) {
		return false
	}
	if prepared.Prepared {
		return prepared.Rejection == contract.MountAbortNone
	}
	return prepared.Rejection >= contract.MountAbortDenied && prepared.Rejection <= contract.MountAbortRevisionConflict
}

func (state *coreState) lookupMountPrepare(key contract.MountAttemptKey, request contract.MountPrepareRequest) (contract.MountPrepared, error, bool) {
	old, exists := state.mount.attempts[key]
	if !exists {
		return contract.MountPrepared{}, nil, false
	}
	prepared, err := preparedReplay(old, request)
	return prepared, err, true
}

// prepareMountPeer constructs no candidate for an exact retained attempt. Both
// local and authenticated callers use it after their original policy checks.
func (state *coreState) prepareMountPeer(source values.FactAuthorityRef, request contract.MountPrepareRequest, authorized bool) (*coreState, contract.MountPrepared, error) {
	key := contract.MountAttemptKey{ParentAuthority: source, OperationID: request.OperationID}
	if prepared, err, found := state.lookupMountPrepare(key, request); found {
		return nil, prepared, err
	}
	next, err := state.prepareMountEvent(MountEvent{Kind: MountEventPrepare, Request: request, Source: source, Authorized: authorized})
	if err != nil {
		return nil, contract.MountPrepared{}, err
	}
	return next, next.mount.attempts[key].Prepared, nil
}

func (state *coreState) prepareMountFinish(source values.FactAuthorityRef, request contract.MountFinishRequest) (*coreState, contract.MountClosed, error) {
	key := contract.MountAttemptKey{ParentAuthority: source, OperationID: request.Evidence.Request.OperationID}
	if old, exists := state.mount.attempts[key]; exists && old.IsClosed {
		if old.Closed.Evidence != request.Evidence {
			return nil, contract.MountClosed{}, contract.ErrOperationConflict
		}
		return nil, old.Closed, nil
	}
	next, err := state.prepareMountEvent(MountEvent{Kind: MountEventFinish, Finish: request, Source: source})
	if err != nil {
		return nil, contract.MountClosed{}, err
	}
	return next, next.mount.attempts[key].Closed, nil
}

func prepareMount(state *coreState, event MountEvent) error {
	request := event.Request
	if !ValidMountRequest(request) || request.Command.Kind == contract.MountRevoke ||
		request.Key.CarrierAuthority != state.authority || event.Source != request.Key.ParentAuthority {
		return contract.ErrInvalid
	}
	key := contract.MountAttemptKey{ParentAuthority: event.Source, OperationID: request.OperationID}
	if _, exists := state.mount.attempts[key]; exists {
		return contract.ErrOperationConflict
	}
	cause := contract.MountAbortNone
	if !event.Authorized {
		cause = contract.MountAbortDenied
	}
	if cause == contract.MountAbortNone && state.transferHoldsIncoming() {
		cause = contract.MountAbortConflict
	}
	incoming := state.mount.facts.Incoming()
	if cause == contract.MountAbortNone {
		if request.Command.Kind == contract.MountEstablish {
			if err := state.guardTransferRootActivation(); err != nil {
				cause = contract.MountAbortConflict
			}
			if state.elements.RootExists() || state.elements.Counts().Claims != 0 || incoming.Key.Valid() {
				cause = contract.MountAbortConflict
			}
			for _, outgoing := range state.mount.facts.Outgoings() {
				if outgoing.State != contract.MountNone {
					cause = contract.MountAbortConflict
				}
			}
		} else if incoming.Key != request.Key || incoming.Admission.Key != request.PreviousAdmission ||
			incoming.Gate != mountfact.GateOpen || incoming.Detached {
			cause = contract.MountAbortConflict
		} else {
			inside, err := state.elements.RangeContainsDirectContent(request.Key.MountAnchorUUID, request.Tuple.Range)
			if err != nil {
				return err
			}
			if !inside {
				cause = contract.MountAbortConflict
			}
		}
	}
	if err := state.issueControlChange(); err != nil {
		return err
	}
	prepared := contract.MountPrepared{Request: request, Prepared: cause == contract.MountAbortNone, Rejection: cause, Cut: state.currentCut()}
	state.mount.attempts[key] = PeerAttempt{Request: request, Prepared: prepared}
	if cause != contract.MountAbortNone {
		return nil
	}
	if request.Command.Kind == contract.MountEstablish {
		incoming = mountfact.Incoming{Key: request.Key, Acceptance: request, AcceptedAt: state.currentCut(), Contact: state.newContact(request.Key, request.Command.ParentNextHop, true)}
	}
	incoming.Gate, incoming.Preparing = mountfact.GatePreparing, key
	state.mount.facts.ReplaceIncoming(incoming)
	return nil
}

func decideMount(state *coreState, event MountEvent) error {
	operation, exists := state.mount.operations[event.Operation]
	if !exists || operation.transferControl() || operation.Decision != contract.MountPending || !ValidMountRequest(operation.Request) {
		return contract.ErrConflict
	}
	request := operation.Request
	record := state.mount.facts.Record(request.Key.MountAnchorUUID)
	if record.Slot != event.Operation || record.Key != request.Key || record.Revision != request.ReservedRevision {
		return contract.ErrConflict
	}
	cause := contract.MountAbortNone
	if !event.Authorized {
		if event.Prepared != (contract.MountPrepared{}) && (event.Prepared.Request != request || !ValidMountPrepared(event.Prepared)) {
			return contract.ErrInvalid
		}
		cause = contract.MountAbortDenied
	} else if event.Prepared.Request != request || !token.Valid(event.Prepared.Cut) {
		return contract.ErrInvalid
	} else if !event.Prepared.Prepared {
		cause = event.Prepared.Rejection
		if cause == contract.MountAbortNone || cause > contract.MountAbortRevisionConflict {
			return contract.ErrInvalid
		}
	} else if event.Prepared.Rejection != contract.MountAbortNone {
		return contract.ErrInvalid
	}
	if cause == contract.MountAbortNone && state.mount.facts.Incoming().Key.Valid() {
		// An ancestor may have started changing its envelope after this child
		// operation reserved its slot or obtained a peer preparation. Keep the
		// pending operation intact while that ancestor is preparing; after it
		// finishes, the normal mutation decision below checks the new envelope.
		// A proved detach cannot permit a new spatial commit, but may settle
		// this exact operation as an abort and release the child peer's gate.
		if state.mount.facts.Incoming().Detached {
			cause = contract.MountAbortConflict
		} else if state.mount.facts.Incoming().Gate != mountfact.GateOpen {
			return contract.ErrMountBusy
		}
	}
	anchor, found := state.elements.Anchor(request.Key.MountAnchorUUID)
	if cause == contract.MountAbortNone && (!found || anchor.Revision != request.Command.ExpectedElementRevision) {
		cause = contract.MountAbortRevisionConflict
	}
	var outcome retainedOutcome
	if cause == contract.MountAbortNone {
		if request.Command.Kind == contract.MountEstablish {
			if record.State != contract.MountEstablishing || !elementfact.AnchorTupleEqual(anchor, request.Tuple) || state.elements.HasDirectContent(anchor.ID) {
				cause = contract.MountAbortConflict
			}
		} else if request.Command.Kind == contract.MountUpdate {
			if record.State != contract.MountActive || record.Admission != request.PreviousAdmission {
				cause = contract.MountAbortConflict
			} else {
				probe := state.clone()
				var err error
				outcome, err = decideMutation(probe, commitbinding.Binding{Actor: operation.Binding.Actor, Kind: elementfact.OperationUpdateAnchor, Element: anchor.ID, Final: request.Tuple, Expected: request.Command.ExpectedElementRevision})
				if err != nil {
					return err
				}
				if outcome.Status == contract.MutationRejected {
					cause = mountAbort(outcome.Rejection)
				}
			}
		} else {
			return contract.ErrInvalid
		}
	}
	if cause == contract.MountAbortNone && request.Command.Kind == contract.MountUpdate {
		var err error
		outcome, err = decideMutation(state, commitbinding.Binding{Actor: operation.Binding.Actor, Kind: elementfact.OperationUpdateAnchor, Element: anchor.ID, Final: request.Tuple, Expected: request.Command.ExpectedElementRevision})
		if err != nil {
			return err
		}
		if outcome.Status != contract.MutationCommitted {
			return contract.ErrStore
		}
		operation.Record, operation.HasRecord = outcome.Anchor, true
	} else if err := state.issueControlChange(); err != nil {
		return err
	}
	operation.DecisionCut = state.currentCut()
	operation.Prepared = event.Prepared
	operation.AbortCause = cause
	operation.Decision = contract.MountAborted
	if cause == contract.MountAbortNone {
		operation.Decision, operation.Changed = contract.MountCommitted, true
		record.State = contract.MountActive
		record.Admission = contract.MountAdmissionKey{MountKey: request.Key, OperationID: event.Operation}
	}
	record.Revision = state.mountRevision(request.Key.MountAnchorUUID)
	state.mount.facts.ReplaceOutgoing(request.Key.MountAnchorUUID, record)
	state.mount.operations[event.Operation] = operation
	return nil
}

func (owner *Owner) LookupMountPrepareLocked(key contract.MountAttemptKey, request contract.MountPrepareRequest) (contract.MountPrepared, error, bool) {
	return owner.state.lookupMountPrepare(key, request)
}
func (owner *Owner) ApplyMountPrepareLocked(source values.FactAuthorityRef, request contract.MountPrepareRequest, authorized bool) (contract.MountPrepared, error) {
	next, prepared, err := owner.state.prepareMountPeer(source, request, authorized)
	if err != nil {
		return contract.MountPrepared{}, err
	}
	if next != nil {
		if err := owner.publishStateLocked(next); err != nil {
			return contract.MountPrepared{}, err
		}
	}
	return prepared, nil
}
func (owner *Owner) ApplyMountFinishLocked(source values.FactAuthorityRef, request contract.MountFinishRequest) (contract.MountClosed, error) {
	next, closed, err := owner.state.prepareMountFinish(source, request)
	if err != nil {
		return contract.MountClosed{}, err
	}
	if next != nil {
		if err := owner.publishStateLocked(next); err != nil {
			return contract.MountClosed{}, err
		}
	}
	return closed, nil
}

func ValidMountDecision(evidence contract.MountDecisionEvidence) bool {
	request := evidence.Request
	if !ValidMountRequest(request) || !token.Valid(evidence.DecisionCut) ||
		token.Scope(evidence.DecisionCut) != token.MountRevisionScope(request.Key.ProposalRevision) ||
		token.Serial(evidence.DecisionCut) < token.MountRevisionSerial(request.ReservedRevision) {
		return false
	}
	if evidence.Decision == contract.MountAborted {
		return request.Command.Kind != contract.MountRevoke && evidence.AbortCause >= contract.MountAbortDenied &&
			evidence.AbortCause <= contract.MountAbortRevisionConflict && evidence.AdmissionKey == request.PreviousAdmission
	}
	if evidence.Decision != contract.MountCommitted || evidence.AbortCause != contract.MountAbortNone {
		return false
	}
	if request.Command.Kind == contract.MountRevoke {
		return evidence.AdmissionKey == request.PreviousAdmission
	}
	return evidence.AdmissionKey == (contract.MountAdmissionKey{MountKey: request.Key, OperationID: request.OperationID}) &&
		token.Serial(evidence.DecisionCut) > token.MountRevisionSerial(request.ReservedRevision)
}

func finishMount(state *coreState, event MountEvent) error {
	evidence := event.Finish.Evidence
	request := evidence.Request
	if !ValidMountDecision(evidence) || request.Key.CarrierAuthority != state.authority || event.Source != request.Key.ParentAuthority {
		return contract.ErrInvalid
	}
	key := contract.MountAttemptKey{ParentAuthority: event.Source, OperationID: request.OperationID}
	attempt, exists := state.mount.attempts[key]
	if exists && (attempt.Request != request || attempt.IsClosed) {
		return contract.ErrOperationConflict
	}
	incoming := state.mount.facts.Incoming()
	prepared := exists && attempt.Prepared.Prepared
	if request.Command.Kind == contract.MountRevoke {
		if state.transferHoldsIncoming() || evidence.Decision != contract.MountCommitted || incoming.Key != request.Key || incoming.Admission.Key != request.PreviousAdmission || incoming.Gate != mountfact.GateOpen {
			return contract.ErrConflict
		}
	} else if evidence.Decision == contract.MountCommitted && (!prepared || incoming.Key != request.Key || incoming.Preparing != key || incoming.Gate != mountfact.GatePreparing) {
		return contract.ErrConflict
	}
	if prepared && (incoming.Key != request.Key || incoming.Preparing != key || incoming.Gate != mountfact.GatePreparing) {
		return contract.ErrConflict
	}
	if err := state.issueControlChange(); err != nil {
		return err
	}
	if request.Command.Kind == contract.MountRevoke {
		incoming.Gate, incoming.Detached = mountfact.GateClosed, true
		incoming.Preparing = contract.MountAttemptKey{}
		state.mount.facts.ReplaceIncoming(incoming)
	} else if prepared {
		if evidence.Decision == contract.MountAborted && request.Command.Kind == contract.MountEstablish {
			state.mount.facts.ReplaceIncoming(mountfact.Incoming{})
		} else {
			if evidence.Decision == contract.MountCommitted {
				incoming.Admission = mountfact.Admission{Key: evidence.AdmissionKey, RangeValue: request.Tuple.Range, DecisionCut: evidence.DecisionCut}
			}
			incoming.Gate, incoming.Preparing = mountfact.GateOpen, contract.MountAttemptKey{}
			state.mount.facts.ReplaceIncoming(incoming)
		}
	}
	// An Abort that overtakes Prepare only closes that exact attempt. It never
	// clears another attempt's incoming state or gate.
	attempt.Request, attempt.IsClosed = request, true
	attempt.Closed = contract.MountClosed{Evidence: evidence, Cut: state.currentCut()}
	state.mount.attempts[key] = attempt
	return nil
}

func settleMount(state *coreState, event MountEvent) error {
	operation, exists := state.mount.operations[event.Operation]
	if !exists || operation.transferControl() || operation.Decision == contract.MountPending || operation.Settled ||
		event.Closed.Evidence != operation.Evidence() || !token.Valid(event.Closed.Cut) {
		return contract.ErrConflict
	}
	request := operation.Request
	record := state.mount.facts.Record(request.Key.MountAnchorUUID)
	if record.Key != request.Key || record.Slot != event.Operation {
		return contract.ErrConflict
	}
	if err := state.issueControlChange(); err != nil {
		return err
	}
	record.Slot, record.Revision = "", state.mountRevision(request.Key.MountAnchorUUID)
	if operation.Decision == contract.MountAborted && request.Command.Kind == contract.MountEstablish {
		record = mountfact.Record{State: contract.MountNone, Revision: record.Revision}
	} else if request.Command.Kind == contract.MountRevoke {
		record.State = contract.MountDetached
	}
	operation.Settled, operation.Closed = true, event.Closed
	state.mount.facts.ReplaceOutgoing(request.Key.MountAnchorUUID, record)
	state.mount.operations[event.Operation] = operation
	return nil
}
