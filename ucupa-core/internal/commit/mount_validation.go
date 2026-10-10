package commit

import (
	"fmt"

	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	commitbinding "ucupa/ucupa-core/internal/commit/binding"
	elementfact "ucupa/ucupa-core/internal/element/fact"
	"ucupa/ucupa-core/internal/geometry"
	mountfact "ucupa/ucupa-core/internal/mount/fact"
	"ucupa/ucupa-core/internal/token"
	transferfact "ucupa/ucupa-core/internal/transfer/fact"
)

func ValidMountAttemptKey(key contract.MountAttemptKey) bool {
	return key.ParentAuthority.Valid() && commitbinding.ValidOperationID(key.OperationID)
}

func validMountEventShape(event MountEvent) bool {
	if !token.Valid(event.BeforeCut) || (event.AfterCut != (contract.CoreReadCut{}) && (!token.Valid(event.AfterCut) || token.Scope(event.AfterCut) != token.Scope(event.BeforeCut))) {
		return false
	}
	canonical := MountEvent{Kind: event.Kind, BeforeCut: event.BeforeCut, AfterCut: event.AfterCut}
	switch event.Kind {
	case MountEventReserve:
		canonical.Operation, canonical.Binding, canonical.Authorized = event.Operation, event.Binding, event.Authorized
	case MountEventPrepare:
		canonical.Request, canonical.Source, canonical.Authorized = event.Request, event.Source, event.Authorized
	case MountEventDecide:
		canonical.Operation, canonical.Prepared, canonical.Authorized = event.Operation, event.Prepared, event.Authorized
	case MountEventFinish:
		canonical.Finish, canonical.Source = event.Finish, event.Source
	case MountEventSettle:
		canonical.Operation, canonical.Closed = event.Operation, event.Closed
	case MountEventAddress:
		canonical.Operation, canonical.Address, canonical.Authorized = event.Operation, event.Address, event.Authorized
	default:
		return false
	}
	return event == canonical
}

type mountSnapshotFacts struct {
	edgeInstallation                                                                   transferfact.EdgeInstallation
	edgeClosure                                                                        transferfact.EdgeClosure
	carrierInstallation                                                                transferfact.CarrierInstallation
	carrierFence                                                                       transferfact.CarrierFence
	authority                                                                          values.FactAuthorityRef
	scope                                                                              [32]byte
	cutSerial                                                                          uint64
	revisionSerial                                                                     uint64
	elements                                                                           elementfact.View
	mount                                                                              mountfact.View
	operations                                                                         map[contract.OperationID]MountOperation
	attempts                                                                           map[contract.MountAttemptKey]PeerAttempt
	mountInitialized, operationsInitialized, addressesInitialized, attemptsInitialized bool
}

func mountSnapshotFactsFromState(state *coreState) mountSnapshotFacts {
	if state == nil {
		return mountSnapshotFacts{}
	}
	return mountSnapshotFacts{
		edgeClosure:         state.currentEdgeClosure(),
		edgeInstallation:    state.currentEdgeInstallation(),
		carrierInstallation: state.currentCarrierInstallation(),
		carrierFence:        state.currentCarrierFence(),
		authority:           state.authority, scope: state.scope, cutSerial: state.cutSerial,
		revisionSerial: state.revisionSerial, elements: state.elements.View,
		mount: state.mount.facts.View, operations: state.mount.operations, attempts: state.mount.attempts,
		mountInitialized: state.mount.facts.Initialized(), operationsInitialized: state.mount.operations != nil,
		addressesInitialized: state.mount.addresses != nil, attemptsInitialized: state.mount.attempts != nil,
	}
}

func mountSnapshotFactsFromProjection(projection ReadView) mountSnapshotFacts {
	return mountSnapshotFacts{
		edgeClosure:         projection.edgeClosure,
		edgeInstallation:    projection.edgeInstallation,
		carrierInstallation: projection.carrierInstallation,
		carrierFence:        projection.carrierFence,
		authority:           projection.Authority, scope: projection.Scope, cutSerial: projection.CutSerial,
		revisionSerial: projection.RevisionSerial, elements: projection.Elements, mount: projection.Mount,
		operations: projection.mountOperations, attempts: projection.mountAttempts,
		mountInitialized: projection.mountInitialized, operationsInitialized: projection.mountOperationsInitialized,
		addressesInitialized: projection.mountAddressesInitialized, attemptsInitialized: projection.mountAttemptsInitialized,
	}
}

// validateMountSnapshot checks only published current state and retained
// evidence. Historical reachability is separately proved by event replay.
func validateMountSnapshot(state *coreState) error {
	if state == nil {
		return fmt.Errorf("mount maps are not initialized")
	}
	return validateMountSnapshotFacts(mountSnapshotFactsFromState(state))
}

func ValidateMountSnapshotProjection(projection ReadView) error {
	return validateMountSnapshotFacts(mountSnapshotFactsFromProjection(projection))
}

func validateMountSnapshotFacts(facts mountSnapshotFacts) error {
	if !facts.mountInitialized || !facts.operationsInitialized || !facts.addressesInitialized || !facts.attemptsInitialized {
		return fmt.Errorf("mount maps are not initialized")
	}
	for element, record := range facts.mount.Outgoings() {
		if !token.ValidMountRevision(record.Revision) || token.MountRevisionScope(record.Revision) != facts.scope || token.MountRevisionElement(record.Revision) != element || token.MountRevisionSerial(record.Revision) > facts.cutSerial {
			return fmt.Errorf("invalid outgoing mount revision")
		}
		if record.State == contract.MountNone {
			if record != (mountfact.Record{Revision: record.Revision}) {
				return fmt.Errorf("empty mount retains live relation")
			}
			continue
		}
		anchor, exists := facts.elements.Anchor(element)
		if !exists || anchor.Root() || !record.Key.Valid() || record.Key.ParentAuthority != facts.authority || record.Key.MountAnchorUUID != element || token.MountRevisionScope(record.Key.ProposalRevision) != facts.scope || token.MountRevisionSerial(record.Key.ProposalRevision) > token.MountRevisionSerial(record.Revision) || !ValidMountContactValues(facts.scope, facts.cutSerial, record.Key, false, record.Contact) {
			return fmt.Errorf("invalid outgoing mount")
		}
		if facts.elements.HasDirectContent(element) {
			return fmt.Errorf("outgoing mount has local children")
		}
		if record.State < contract.MountEstablishing || record.State > contract.MountDetached {
			return fmt.Errorf("unknown outgoing mount state")
		}
		if record.State == contract.MountEstablishing {
			if record.Admission != (contract.MountAdmissionKey{}) || record.Slot == "" {
				return fmt.Errorf("invalid establishing mount")
			}
		} else if !mountfact.ValidMountAdmissionKey(record.Admission) || record.Admission.MountKey != record.Key {
			return fmt.Errorf("invalid outgoing admission")
		}
		if record.State == contract.MountRevoking && record.Slot == "" {
			return fmt.Errorf("revoking mount has no operation")
		}
		if record.State == contract.MountDetached && record.Slot != "" {
			return fmt.Errorf("detached mount has a slot")
		}
		if record.Slot != "" {
			operation, exists := facts.operations[record.Slot]
			if !exists || operation.Settled || !operation.transferControl() && operation.Request.Key != record.Key {
				return fmt.Errorf("mount slot does not bind its operation")
			}
			if c := operation.Carrier; c != (transferfact.CarrierControl{}) && (c.CancelledAt != (contract.CoreReadCut{}) || c.Reservation.Input.Mount != element) {
				return fmt.Errorf("mount slot carries a cancelled or different replacement")
			}
			if err := validateMountOperationFacts(facts, record.Slot, operation); err != nil {
				return err
			}
		}
	}
	incoming := facts.mount.Incoming()
	if err := validateCarrierIncomingOrigin(facts); err != nil {
		return err
	}
	if facts.carrierFence != (transferfact.CarrierFence{}) && incoming.Gate != mountfact.GateFenced {
		return fmt.Errorf("carrier fence lost its closed gate")
	}
	if facts.edgeClosure != (transferfact.EdgeClosure{}) && incoming.Gate != mountfact.GateReparenting {
		return fmt.Errorf("departed parent lost its closed gate")
	}
	if incoming.Key.Valid() {
		if incoming.Key.CarrierAuthority != facts.authority || !validLocalMountCutValues(facts.scope, facts.cutSerial, incoming.AcceptedAt) || !ValidMountContactValues(facts.scope, facts.cutSerial, incoming.Key, true, incoming.Contact) {
			return fmt.Errorf("invalid incoming acceptance")
		}
		if incoming.Acceptance != (contract.MountPrepareRequest{}) && (!ValidMountRequest(incoming.Acceptance) || incoming.Acceptance.Command.Kind != contract.MountEstablish || incoming.Acceptance.Key != incoming.Key) {
			return fmt.Errorf("invalid ordinary incoming acceptance")
		}
		if incoming.Admission.Key != (contract.MountAdmissionKey{}) && (!mountfact.ValidMountAdmissionKey(incoming.Admission.Key) || incoming.Admission.Key.MountKey != incoming.Key || !geometry.ValidSphere(incoming.Admission.RangeValue) || !token.Valid(incoming.Admission.DecisionCut) || token.Scope(incoming.Admission.DecisionCut) != token.MountRevisionScope(incoming.Key.ProposalRevision)) {
			return fmt.Errorf("invalid incoming admission")
		}
		if incoming.Admission.Key == (contract.MountAdmissionKey{}) && incoming.Admission != (mountfact.Admission{}) {
			return fmt.Errorf("partial incoming admission")
		}
		switch incoming.Gate {
		case mountfact.GateReparenting:
			c := facts.edgeClosure
			if !c.Valid(facts.authority) || c.Previous != incoming.Admission ||
				token.Scope(c.At) != facts.scope || token.Serial(c.At) > facts.cutSerial ||
				incoming.Detached || incoming.Preparing != (contract.MountAttemptKey{}) {
				return fmt.Errorf("departed parent lacks exact carrier closure")
			}
			inside, err := facts.elements.RangeContainsDirectContent(c.Fence.Mount, c.Range)
			if err != nil || !inside {
				return fmt.Errorf("held carrier content violates its promised range")
			}
		case mountfact.GateFenced:
			f := facts.carrierFence
			if !f.Valid() || f.Permit.Reservation.Input.Source != facts.authority ||
				f.Permit.Reservation.Previous != incoming.Admission.Key || incoming.Key != f.Permit.Reservation.Previous.MountKey ||
				f.Permit.Reservation.Tuple.Range != incoming.Admission.RangeValue || token.Scope(f.FinalCut) != facts.scope || token.Serial(f.FinalCut) >= facts.cutSerial ||
				incoming.Preparing != (contract.MountAttemptKey{}) || incoming.Detached || facts.elements.Counts().Anchors != 0 || facts.elements.Counts().Entities != 0 {
				return fmt.Errorf("carrier fence lacks exact source withdrawal")
			}
		case mountfact.GatePreparing:
			attempt, exists := facts.attempts[incoming.Preparing]
			if !exists || !attempt.Prepared.Prepared || attempt.IsClosed || incoming.Detached || attempt.Request.Key != incoming.Key || attempt.Request.PreviousAdmission != incoming.Admission.Key || !ValidMountPrepared(attempt.Prepared) || attempt.Prepared.Request != attempt.Request || !validLocalMountCutValues(facts.scope, facts.cutSerial, attempt.Prepared.Cut) {
				return fmt.Errorf("preparing gate lacks exact prepared attempt")
			}
		case mountfact.GateOpen:
			if incoming.Preparing != (contract.MountAttemptKey{}) || incoming.Detached || !mountfact.ValidMountAdmissionKey(incoming.Admission.Key) {
				return fmt.Errorf("invalid open mount gate")
			}
		case mountfact.GateClosed:
			if incoming.Preparing != (contract.MountAttemptKey{}) || !incoming.Detached || !mountfact.ValidMountAdmissionKey(incoming.Admission.Key) {
				return fmt.Errorf("invalid closed mount gate")
			}
		default:
			return fmt.Errorf("unknown mount gate")
		}
	} else if incoming != (mountfact.Incoming{}) {
		return fmt.Errorf("partial incoming relation")
	}
	return nil
}

// Retained receipts grow with history and are checked only at full validation
// boundaries, never by a current-cut read.
func validateMountRetained(state *coreState) error {
	incoming := state.mount.facts.Incoming()
	for id, operation := range state.mount.operations {
		if !commitbinding.ValidOperationID(id) || !operation.transferControl() && !validMountBinding(operation.Binding) {
			return fmt.Errorf("invalid mount operation binding")
		}
		if _, exists := state.operations[id]; exists {
			return fmt.Errorf("Element and mount operations reuse one ID")
		}
		if _, exists := state.mount.addresses[id]; exists {
			return fmt.Errorf("mount and address operations reuse one ID")
		}
		if err := validateMountOperation(state, id, operation); err != nil {
			return err
		}
	}
	for key, attempt := range state.mount.attempts {
		if !ValidMountAttemptKey(key) || !ValidMountRequest(attempt.Request) || key.ParentAuthority != attempt.Request.Key.ParentAuthority || key.OperationID != attempt.Request.OperationID || attempt.Request.Key.CarrierAuthority != state.authority {
			return fmt.Errorf("invalid peer attempt")
		}
		if attempt.Prepared != (contract.MountPrepared{}) && (attempt.Prepared.Request != attempt.Request || !validLocalMountCut(state, attempt.Prepared.Cut) || (attempt.Prepared.Prepared && attempt.Prepared.Rejection != contract.MountAbortNone) || (!attempt.Prepared.Prepared && (attempt.Prepared.Rejection < contract.MountAbortDenied || attempt.Prepared.Rejection > contract.MountAbortRevisionConflict))) {
			return fmt.Errorf("invalid retained prepare")
		}
		if attempt.IsClosed {
			if attempt.Closed.Evidence.Request != attempt.Request || !ValidMountDecision(attempt.Closed.Evidence) || !validLocalMountCut(state, attempt.Closed.Cut) {
				return fmt.Errorf("invalid peer closure")
			}
		} else if attempt.Closed != (contract.MountClosed{}) || attempt.Prepared == (contract.MountPrepared{}) {
			return fmt.Errorf("peer attempt has no outcome")
		}
		if attempt.Prepared.Prepared && !attempt.IsClosed && (!incoming.Key.Valid() || incoming.Preparing != key || incoming.Gate != mountfact.GatePreparing) {
			return fmt.Errorf("prepared obligation lost its gate")
		}
	}
	for id, operation := range state.mount.addresses {
		binding := operation.binding
		if !commitbinding.ValidOperationID(id) || !binding.Actor.Valid() || !binding.Key.Valid() || binding.Element != binding.Key.MountAnchorUUID || !token.ValidNextHopRevision(binding.Expected) || !binding.NextHop.Valid() {
			return fmt.Errorf("invalid address operation")
		}
		if _, exists := state.operations[id]; exists {
			return fmt.Errorf("Element and address operations reuse one ID")
		}
		if operation.errorKind == elementfact.NoError {
			result := operation.result
			if result.OperationID != id || result.MountKey != binding.Key || result.NextHop != binding.NextHop || !token.ValidNextHopRevision(result.NextHopRevision) || !validLocalMountCut(state, result.Cut) {
				return fmt.Errorf("invalid address receipt")
			}
		} else if operation.errorKind < elementfact.NotFound || operation.errorKind > elementfact.RevisionConflict || operation.result != (contract.NextHopUpdateResult{}) {
			return fmt.Errorf("invalid address rejection")
		}
	}
	return nil
}

func validLocalMountCut(state *coreState, cut contract.CoreReadCut) bool {
	return state != nil && validLocalMountCutValues(state.scope, state.cutSerial, cut)
}

func validMountContact(state *coreState, key contract.MountKey, incoming bool, contact mountfact.Contact) bool {
	return state != nil && ValidMountContactValues(state.scope, state.cutSerial, key, incoming, contact)
}

func validLocalMountCutValues(scope [32]byte, cutSerial uint64, cut contract.CoreReadCut) bool {
	return token.Valid(cut) && token.Scope(cut) == scope && token.Serial(cut) <= cutSerial
}

func ValidMountContactValues(scope [32]byte, cutSerial uint64, key contract.MountKey, incoming bool, contact mountfact.Contact) bool {
	return contact.NextHop.Valid() && token.ValidNextHopRevision(contact.Revision) && token.NextHopScope(contact.Revision) == scope && token.NextHopKey(contact.Revision) == key && token.NextHopIncoming(contact.Revision) == incoming && token.NextHopSerial(contact.Revision) <= cutSerial
}

func validateMountOperation(state *coreState, id contract.OperationID, operation MountOperation) error {
	if state == nil {
		return fmt.Errorf("invalid mount operation")
	}
	return validateMountOperationFacts(mountSnapshotFactsFromState(state), id, operation)
}

func validateMountOperationFacts(facts mountSnapshotFacts, id contract.OperationID, operation MountOperation) error {
	if operation.Parent != (transferfact.ParentControl{}) {
		return validateTransferParentFacts(facts, id, operation)
	}
	if operation.Carrier != (transferfact.CarrierControl{}) {
		return validateCarrierMountOperationFacts(facts, id, operation)
	}
	if operation.CarrierBinding != (transferfact.CarrierBinding{}) || operation.CarrierSettlement != (transferfact.CarrierSettlement{}) {
		return fmt.Errorf("ordinary mount carries replacement decision")
	}
	if operation.Decision < contract.MountPending || operation.Decision > contract.MountAborted {
		return fmt.Errorf("invalid mount decision")
	}
	fullRequest := operation.Request.OperationID != ""
	if fullRequest {
		request := operation.Request
		if !ValidMountRequest(request) || request.OperationID != id || request.Key.ParentAuthority != facts.authority || request.Command != operation.Binding.Command || request.ParentActor != operation.Binding.Actor || request.CarrierActor != operation.Binding.PeerActor {
			return fmt.Errorf("mount request does not bind original intent")
		}
	} else if operation.Request != (contract.MountPrepareRequest{Key: operation.Request.Key}) {
		return fmt.Errorf("partial mount request")
	}
	if operation.Decision == contract.MountPending {
		if !fullRequest || operation.Settled || operation.DecisionCut != (contract.CoreReadCut{}) || operation.Changed || operation.AbortCause != contract.MountAbortNone || operation.HasRecord || operation.Prepared != (contract.MountPrepared{}) || operation.Closed != (contract.MountClosed{}) {
			return fmt.Errorf("invalid pending result")
		}
	} else {
		if !validLocalMountCutValues(facts.scope, facts.cutSerial, operation.DecisionCut) {
			return fmt.Errorf("invalid mount decision cut")
		}
		if operation.Decision == contract.MountAborted {
			if operation.AbortCause < contract.MountAbortDenied || operation.AbortCause > contract.MountAbortRevisionConflict || operation.HasRecord || operation.Changed {
				return fmt.Errorf("invalid mount abort")
			}
		} else if operation.AbortCause != contract.MountAbortNone {
			return fmt.Errorf("committed mount has an abort cause")
		}
		if !fullRequest && !operation.Settled {
			return fmt.Errorf("local outcome retains remote obligation")
		}
		if operation.Settled && fullRequest && (operation.Closed.Evidence != operation.Evidence() || !token.Valid(operation.Closed.Cut)) {
			return fmt.Errorf("settled mount has no matching closure")
		}
	}
	if !operation.HasRecord && operation.Record != (elementfact.Anchor{}) {
		return fmt.Errorf("mount result has hidden record")
	}
	if operation.HasRecord && (!token.ValidRevision(operation.Record.Revision) || operation.Record.ID != operation.Binding.Command.MountAnchorUUID || operation.Record.Root() || token.RevisionScope(operation.Record.Revision) != facts.scope || token.RevisionSerial(operation.Record.Revision) > facts.revisionSerial) {
		return fmt.Errorf("invalid original mount record")
	}
	if !operation.Settled {
		record := facts.mount.Record(operation.Binding.Command.MountAnchorUUID)
		if record.Slot != id || record.Key != operation.Request.Key {
			return fmt.Errorf("unsettled operation lost its slot")
		}
	}
	return nil
}
