package commit

import (
	"fmt"
	"math"

	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	commitbinding "ucupa/ucupa-core/internal/commit/binding"
	elementfact "ucupa/ucupa-core/internal/element/fact"
	"ucupa/ucupa-core/internal/geometry"
	mountfact "ucupa/ucupa-core/internal/mount/fact"
	"ucupa/ucupa-core/internal/token"
)

// prepareMountIntent retains request binding and peer-identity precedence before
// constructing a reservation. An exact retry requires no new publication.
func (state *coreState) prepareMountIntent(callers contract.LocalCallers, id contract.OperationID, binding MountBinding) (*coreState, error) {
	if _, exists := state.operations[id]; exists {
		return nil, contract.ErrOperationConflict
	}
	if _, exists := state.mount.addresses[id]; exists {
		return nil, contract.ErrOperationConflict
	}
	command := binding.Command
	record := state.mount.facts.Record(command.MountAnchorUUID)
	carrier := record.Key.CarrierAuthority
	if command.Kind == contract.MountEstablish {
		carrier = command.CarrierAuthority
	}
	if retained, exists := state.mount.operations[id]; exists {
		if retained.transferControl() {
			return nil, contract.ErrOperationConflict
		}
		if retained.Request.Key.Valid() {
			carrier = retained.Request.Key.CarrierAuthority
		}
		if command.Kind != contract.MountRevoke {
			binding.PeerActor = callers[carrier]
		}
		if retained.Binding.PeerActor.Valid() && !binding.PeerActor.Valid() {
			return nil, contract.ErrDenied
		}
		if retained.Binding != binding {
			return nil, contract.ErrOperationConflict
		}
		return nil, nil
	}
	if command.Kind != contract.MountRevoke {
		binding.PeerActor = callers[carrier]
	}
	if command.Kind == contract.MountEstablish && !binding.PeerActor.Valid() {
		return nil, contract.ErrDenied
	}
	if command.Kind == contract.MountUpdate {
		if anchor, exists := state.elements.Anchor(command.MountAnchorUUID); exists && !anchor.Root() {
			rangeValue, ok := anchor.Range.BoundedSphere()
			if ok && !geometry.EqualSphere(rangeValue, command.Final.Range) && !binding.PeerActor.Valid() {
				return nil, contract.ErrDenied
			}
		}
	}
	return state.prepareMountEvent(MountEvent{Kind: MountEventReserve, Operation: id, Binding: binding, Authorized: true})
}

// prepareMountEvent captures the admission cut and applies the one shared live/
// replay transition to a candidate; the publishing owner retains the lock.
func (state *coreState) prepareMountEvent(event MountEvent) (*coreState, error) {
	event.BeforeCut = state.currentCut()
	next := state.clone()
	if err := applyMountEvent(next, event); err != nil {
		return nil, err
	}
	return next, nil
}

// applyMountEvent is the only control transition function, shared by live
// commits and Store replay. No owner, peer, policy or Store calls occur here.
func applyMountEvent(state *coreState, event MountEvent) error {
	if state == nil || !validMountEventShape(event) || event.BeforeCut != state.currentCut() {
		return contract.ErrStore
	}
	var err error
	switch event.Kind {
	case MountEventReserve:
		err = reserveMount(state, event)
	case MountEventPrepare:
		err = prepareMount(state, event)
	case MountEventDecide:
		err = decideMount(state, event)
	case MountEventFinish:
		err = finishMount(state, event)
	case MountEventSettle:
		err = settleMount(state, event)
	case MountEventAddress:
		err = updateMountAddress(state, event)
	default:
		return contract.ErrStore
	}
	if err != nil {
		return err
	}
	if event.AfterCut != (contract.CoreReadCut{}) && event.AfterCut != state.currentCut() {
		return contract.ErrStore
	}
	event.AfterCut = state.currentCut()
	state.mount.events = append(state.mount.events, event)
	if event.AfterCut != event.BeforeCut {
		if token.Serial(event.AfterCut) != token.Serial(event.BeforeCut)+1 {
			return contract.ErrStore
		}
		state.commits = append(state.commits, coreCommit{kind: mountCommitted, controlIndex: uint64(len(state.mount.events))})
	}
	return nil
}

func (state *coreState) issueControlChange() error {
	if state.cutSerial == math.MaxUint64 {
		return fmt.Errorf("%w: Core cut exhausted", contract.ErrStore)
	}
	state.cutSerial++
	return nil
}

func (state *coreState) mountRevision(id values.ElementID) contract.MountRevision {
	return token.MountRevisionFromParts(state.scope, id, state.cutSerial)
}

func (state *coreState) newContact(key contract.MountKey, address values.NextHop, incoming bool) mountfact.Contact {
	return mountfact.Contact{NextHop: address, Revision: token.NextHopRevisionFromParts(state.scope, key, incoming, state.cutSerial)}
}

func CanonicalMountCommand(command contract.MountCommand) (contract.MountCommand, bool) {
	if !command.MountAnchorUUID.Valid() {
		return contract.MountCommand{}, false
	}
	switch command.Kind {
	case contract.MountEstablish:
		if !token.ValidRevision(command.ExpectedElementRevision) || command.ExpectedMountRevision != (contract.MountRevision{}) ||
			!command.CarrierAuthority.Valid() || !command.CarrierNextHop.Valid() || !command.ParentNextHop.Valid() || command.Final != (contract.SpatialTuple{}) {
			return contract.MountCommand{}, false
		}
	case contract.MountUpdate:
		if !token.ValidRevision(command.ExpectedElementRevision) || !token.ValidMountRevision(command.ExpectedMountRevision) ||
			command.CarrierAuthority != "" || command.CarrierNextHop != "" || command.ParentNextHop != "" {
			return contract.MountCommand{}, false
		}
		final, ok := commitbinding.CanonicalFinalTuple(command.Final)
		if !ok {
			return contract.MountCommand{}, false
		}
		command.Final = final
	case contract.MountRevoke:
		if command.ExpectedElementRevision != (contract.Revision{}) || !token.ValidMountRevision(command.ExpectedMountRevision) ||
			command.CarrierAuthority != "" || command.CarrierNextHop != "" || command.ParentNextHop != "" || command.Final != (contract.SpatialTuple{}) {
			return contract.MountCommand{}, false
		}
	default:
		return contract.MountCommand{}, false
	}
	return command, true
}

func validMountBinding(binding MountBinding) bool {
	canonical, ok := CanonicalMountCommand(binding.Command)
	return ok && canonical == binding.Command && binding.Actor.Valid() &&
		(binding.PeerActor == (values.PrincipalRef{}) || binding.PeerActor.Valid())
}

func mountAbort(kind elementfact.Rejection) contract.MountAbortCause {
	switch kind {
	case elementfact.NotFound:
		return contract.MountAbortNotFound
	case elementfact.RevisionConflict:
		return contract.MountAbortRevisionConflict
	default:
		return contract.MountAbortConflict
	}
}

func rejectMount(state *coreState, id contract.OperationID, binding MountBinding, cause contract.MountAbortCause) {
	state.mount.operations[id] = MountOperation{Binding: binding, Decision: contract.MountAborted, Settled: true, DecisionCut: state.currentCut(), AbortCause: cause}
}

func reserveMount(state *coreState, event MountEvent) error {
	id, binding := event.Operation, event.Binding
	if !commitbinding.ValidOperationID(id) || !validMountBinding(binding) || !event.Authorized {
		return contract.ErrStore
	}
	if state.mountOperationClaimed(id) {
		return contract.ErrOperationConflict
	}
	if _, exists := state.operations[id]; exists {
		return contract.ErrOperationConflict
	}
	// An incoming parent coordinates its envelope by closing this owner's
	// spatial write gate. Child mount control can change an owned Anchor's
	// tuple, so it shares that gate with ordinary writes. Already retained
	// operations are replayed before this transition, and contact repair and
	// settlement do not enter it.
	if state.mount.facts.Incoming().Key.Valid() && state.mount.facts.Incoming().Gate != mountfact.GateOpen {
		return contract.ErrMountBusy
	}
	command := binding.Command
	m := command.MountAnchorUUID
	if err := state.guardTransferScope(m); err != nil {
		return err
	}
	record := state.mount.facts.Record(m)
	if record.Slot != "" {
		return contract.ErrMountBusy
	}
	anchor, exists := state.elements.Anchor(m)
	if !exists {
		cause := contract.MountAbortNotFound
		if state.elements.Kind(m) != 0 {
			cause = contract.MountAbortConflict
		}
		rejectMount(state, id, binding, cause)
		return nil
	}
	if anchor.Root() {
		rejectMount(state, id, binding, contract.MountAbortConflict)
		return nil
	}
	if command.Kind != contract.MountRevoke && anchor.Revision != command.ExpectedElementRevision {
		rejectMount(state, id, binding, contract.MountAbortRevisionConflict)
		return nil
	}
	if command.Kind == contract.MountEstablish {
		if command.CarrierAuthority == state.authority || record.State != contract.MountNone || state.elements.HasDirectContent(m) {
			rejectMount(state, id, binding, contract.MountAbortConflict)
			return nil
		}
		if !binding.PeerActor.Valid() {
			return contract.ErrDenied
		}
	} else {
		if record.State != contract.MountActive {
			rejectMount(state, id, binding, contract.MountAbortConflict)
			return nil
		}
		if command.ExpectedMountRevision != record.Revision {
			rejectMount(state, id, binding, contract.MountAbortRevisionConflict)
			return nil
		}
	}
	if command.Kind == contract.MountUpdate {
		probe := state.clone()
		outcome, err := decideMutation(probe, commitbinding.Binding{Actor: binding.Actor, Kind: elementfact.OperationUpdateAnchor, Element: m, Final: command.Final, Expected: command.ExpectedElementRevision})
		if err != nil {
			return err
		}
		if outcome.Status == contract.MutationRejected {
			rejectMount(state, id, binding, mountAbort(outcome.Rejection))
			return nil
		}
		oldRange, ok := anchor.Range.BoundedSphere()
		if !ok {
			return contract.ErrStore
		}
		if geometry.EqualSphere(oldRange, command.Final.Range) {
			// Reuse the existing local mutation decision without a second
			// authorization surface, journal, or intermediate spatial tuple.
			outcome, err = decideMutation(state, commitbinding.Binding{Actor: binding.Actor, Kind: elementfact.OperationUpdateAnchor, Element: m, Final: command.Final, Expected: command.ExpectedElementRevision})
			if err != nil {
				return err
			}
			state.mount.operations[id] = MountOperation{Binding: binding, Request: contract.MountPrepareRequest{Key: record.Key}, Decision: contract.MountCommitted, Settled: true, DecisionCut: state.currentCut(), Changed: outcome.Status == contract.MutationCommitted, Record: outcome.Anchor, HasRecord: true}
			return nil
		}
		if !binding.PeerActor.Valid() {
			return contract.ErrDenied
		}
	}
	if err := state.issueControlChange(); err != nil {
		return err
	}
	record.Revision = state.mountRevision(m)
	record.Slot = id
	if command.Kind == contract.MountEstablish {
		record.State = contract.MountEstablishing
		record.Key = contract.MountKey{ParentAuthority: state.authority, MountAnchorUUID: m, ProposalRevision: record.Revision, CarrierAuthority: command.CarrierAuthority}
		record.Contact = state.newContact(record.Key, command.CarrierNextHop, false)
	}
	sphere, ok := anchor.Range.BoundedSphere()
	if !ok {
		return contract.ErrStore
	}
	tuple := contract.SpatialTuple{Parent: anchor.Parent, LocalPose: anchor.LocalPose, Range: sphere}
	if command.Kind == contract.MountUpdate {
		tuple = command.Final
	}
	request := contract.MountPrepareRequest{Key: record.Key, OperationID: id, Command: command, ParentActor: binding.Actor, CarrierActor: binding.PeerActor, PreviousAdmission: record.Admission, ReservedRevision: record.Revision, Tuple: tuple}
	operation := MountOperation{Binding: binding, Request: request, Decision: contract.MountPending}
	if command.Kind == contract.MountRevoke {
		record.State = contract.MountRevoking
		operation.Decision, operation.DecisionCut, operation.Changed = contract.MountCommitted, state.currentCut(), true
	}
	state.mount.facts.ReplaceOutgoing(m, record)
	state.mount.operations[id] = operation
	return nil
}

func ValidMountRequest(request contract.MountPrepareRequest) bool {
	canonical, ok := CanonicalMountCommand(request.Command)
	if !ok || canonical != request.Command || !request.Key.Valid() || !commitbinding.ValidOperationID(request.OperationID) ||
		request.Command.MountAnchorUUID != request.Key.MountAnchorUUID || !request.ParentActor.Valid() ||
		!token.ValidMountRevision(request.ReservedRevision) || token.MountRevisionScope(request.ReservedRevision) != token.MountRevisionScope(request.Key.ProposalRevision) ||
		token.MountRevisionElement(request.ReservedRevision) != request.Key.MountAnchorUUID || !commitbinding.ValidFinalTuple(request.Tuple) {
		return false
	}
	if request.Command.Kind == contract.MountEstablish {
		return request.CarrierActor.Valid() && request.PreviousAdmission == (contract.MountAdmissionKey{}) &&
			request.Command.CarrierAuthority == request.Key.CarrierAuthority && request.ReservedRevision == request.Key.ProposalRevision
	}
	return mountfact.ValidMountAdmissionKey(request.PreviousAdmission) && request.PreviousAdmission.MountKey == request.Key &&
		(request.Command.Kind == contract.MountRevoke || (request.CarrierActor.Valid() && request.Tuple == request.Command.Final))
}

func (operation MountOperation) Evidence() contract.MountDecisionEvidence {
	evidence := contract.MountDecisionEvidence{Request: operation.Request, Decision: operation.Decision, DecisionCut: operation.DecisionCut, AbortCause: operation.AbortCause}
	if operation.Decision == contract.MountCommitted && operation.Binding.Command.Kind != contract.MountRevoke {
		evidence.AdmissionKey = contract.MountAdmissionKey{MountKey: operation.Request.Key, OperationID: operation.Request.OperationID}
	} else {
		evidence.AdmissionKey = operation.Request.PreviousAdmission
	}
	return evidence
}

func (owner *Owner) ApplyMountIntentLocked(callers contract.LocalCallers, id contract.OperationID, binding MountBinding) error {
	next, err := owner.state.prepareMountIntent(callers, id, binding)
	if err != nil || next == nil {
		return err
	}
	return owner.publishStateLocked(next)
}
func (owner *Owner) ApplyMountEventLocked(event MountEvent) error {
	next, err := owner.state.prepareMountEvent(event)
	if err != nil {
		return err
	}
	return owner.publishStateLocked(next)
}
