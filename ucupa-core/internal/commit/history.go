package commit

import (
	"errors"
	"fmt"

	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	commitbinding "ucupa/ucupa-core/internal/commit/binding"
	"ucupa/ucupa-core/internal/token"
)

func validateOperationHistory(final *coreState) error {
	replay, err := replayCoreHistory(final)
	if err != nil {
		return err
	}
	if !sameCompleteCoreState(replay, final) {
		return errors.New("operation history does not reproduce complete Core state")
	}
	return nil
}

func replayCoreHistory(final *coreState) (*coreState, error) {
	if final == nil || !final.authority.Valid() || final.scope == ([32]byte{}) || final.cutSerial == 0 || uint64(len(final.commits)) != final.cutSerial-1 {
		return nil, errors.New("Core history cut sequence is invalid")
	}
	committed := make(map[contract.OperationID]retainedOperation)
	decisions := make(map[uint64]map[contract.OperationID]retainedOperation)
	issued := make(map[contract.Revision]uint64)
	for operationID, operation := range final.operations {
		if !commitbinding.ValidOperationID(operationID) || !commitbinding.CanonicalBindingForStore(operation.binding) {
			return nil, fmt.Errorf("Operation %q binding is invalid", operationID)
		}
		if operation.binding.Boundary != "" {
			input, err := values.ParseCanonicalJSON([]byte(operation.binding.PublicRequest), values.PublicJSONLimits())
			if err != nil {
				return nil, err
			}
			object, ok := input.(map[string]any)
			if !ok {
				return nil, errors.New("public operation request is not an object")
			}
			_, originalID, _, err := commitbinding.ParsePublicMutation(object)
			if err != nil || originalID != operationID {
				return nil, errors.New("public operation id does not match retained key")
			}
		}
		cut := operation.outcome.cut
		admitted := token.Serial(cut)
		if operation.outcome.Status == contract.MutationCommitted {
			admitted--
		}
		if operation.admission != token.FromParts(final.scope, admitted) {
			return nil, fmt.Errorf("Operation %q has inconsistent admission cut", operationID)
		}
		if !token.Valid(cut) || token.Scope(cut) != final.scope || token.Serial(cut) > final.cutSerial {
			return nil, fmt.Errorf("Operation %q cut is invalid", operationID)
		}
		switch operation.outcome.Status {
		case contract.MutationCommitted:
			if token.Serial(cut) <= 1 {
				return nil, fmt.Errorf("Operation %q committed at the initial cut", operationID)
			}
			committed[operationID] = operation
		case contract.MutationUnchanged, contract.MutationRejected:
			if decisions[token.Serial(cut)] == nil {
				decisions[token.Serial(cut)] = make(map[contract.OperationID]retainedOperation)
			}
			decisions[token.Serial(cut)][operationID] = operation
		default:
			return nil, fmt.Errorf("Operation %q status is invalid", operationID)
		}
	}
	replay := newCoreState(final.authority, final.scope)
	replay.publicKey = final.publicKey
	if final.realmContact.initial != (contract.RealmInitialization{}) {
		contact, err := replayRealmContact(final.realmContact, final.authority, final.scope)
		if err != nil {
			return nil, err
		}
		replay.realmContact = contact
	}
	controls := newMountHistory(final.mount.events)
	attachments := newAttachmentHistory(final.attachments.events)
	transferIndex := 0
	for cutSerial := uint64(1); ; cutSerial++ {
		if cutSerial > 1 {
			event := final.commits[cutSerial-2]
			if event.kind == mountCommitted {
				if err := controls.commit(replay, event, issued); err != nil {
					return nil, err
				}
			} else if event.kind == attachmentCommitted {
				if err := attachments.commit(replay, event); err != nil {
					return nil, err
				}
			} else if event.kind == transferCommitted {
				if err := replayTransferCommit(replay, final.transfers.events, &transferIndex, event, issued); err != nil {
					return nil, err
				}
				controls.capture(replay)
			} else {
				operation, exists := committed[event.operation]
				if event.kind != elementCommitted || event.controlIndex != 0 || !exists || token.Serial(operation.outcome.cut) != cutSerial || replay.mountOperationClaimed(event.operation) {
					return nil, fmt.Errorf("commit at cut %d does not bind its original outcome", cutSerial)
				}
				delete(committed, event.operation)
				if err := replay.validatePublicAdmission(operation.binding); err != nil {
					return nil, err
				}
				if err := validateExpectedIssued(operation.binding, issued, final.scope, cutSerial-1); err != nil {
					return nil, fmt.Errorf("committed operation at cut %d: %w", cutSerial, err)
				}
				if err := replay.guardOrdinaryMutation(operation.binding); err != nil {
					return nil, fmt.Errorf("Element commit at cut %d bypassed mount guard: %w", cutSerial, err)
				}
				actual, err := decideMutation(replay, operation.binding)
				if err != nil {
					return nil, fmt.Errorf("replay commit at cut %d: %w", cutSerial, err)
				}
				if actual != operation.outcome || actual.Status != contract.MutationCommitted {
					return nil, fmt.Errorf("committed outcome at cut %d is impossible", cutSerial)
				}
				replay.commits = append(replay.commits, event)
				replay.operations[event.operation] = operation
				issued[committedRevision(actual)] = cutSerial
			}
		}
		if err := controls.decisions(replay, issued); err != nil {
			return nil, err
		}
		attachments.capture(replay)
		if err := attachments.decisions(replay); err != nil {
			return nil, err
		}
		for operationID, operation := range decisions[cutSerial] {
			if err := replay.validatePublicAdmission(operation.binding); err != nil {
				return nil, err
			}
			if replay.mountOperationClaimed(operationID) {
				return nil, errors.New("OperationID is claimed by Element and mount decisions")
			}
			if err := validateExpectedIssued(operation.binding, issued, final.scope, cutSerial); err != nil {
				return nil, fmt.Errorf("retained decision at cut %d: %w", cutSerial, err)
			}
			candidate := replay.clone()
			if err := candidate.guardOrdinaryMutation(operation.binding); err != nil {
				return nil, fmt.Errorf("Element decision at cut %d bypassed mount guard: %w", cutSerial, err)
			}
			actual, err := decideMutation(candidate, operation.binding)
			if err != nil {
				return nil, fmt.Errorf("replay decision at cut %d: %w", cutSerial, err)
			}
			if actual != operation.outcome || actual.Status == contract.MutationCommitted {
				return nil, fmt.Errorf("retained decision at cut %d is impossible", cutSerial)
			}
			if !sameCoreFacts(candidate, replay) {
				return nil, fmt.Errorf("retained decision at cut %d changed facts", cutSerial)
			}
			replay.operations[operationID] = operation
		}
		if err := replayTransferDecisions(replay, final.transfers.events, &transferIndex); err != nil {
			return nil, err
		}
		if cutSerial == final.cutSerial {
			break
		}
	}
	if len(committed) != 0 || controls.index != len(controls.events) || attachments.index != len(attachments.events) || transferIndex != len(final.transfers.events) {
		return nil, errors.New("operation history contains unconsumed decisions")
	}
	for id := range replay.operations {
		if replay.mountOperationClaimed(id) {
			return nil, errors.New("OperationID is claimed by Element and mount decisions")
		}
	}
	return replay, nil
}

func validateExpectedIssued(
	binding commitbinding.Binding,
	issued map[contract.Revision]uint64,
	storeScope [32]byte,
	latestCut uint64,
) error {
	expected := binding.Expected
	if expected == (contract.Revision{}) || token.RevisionScope(expected) != storeScope {
		return nil
	}
	// A foreign-scope token is a legitimate opaque caller input and is replayed
	// through the ordinary decision order. A token claiming this Store's scope
	// must, however, have actually been issued before it could be supplied.
	issuedAt, exists := issued[expected]
	if !exists {
		return errors.New("expected Revision was not issued by this Store")
	}
	if issuedAt > latestCut {
		return errors.New("expected Revision was issued after the operation")
	}
	return nil
}

func committedRevision(outcome retainedOutcome) contract.Revision {
	if token.ValidRevision(outcome.Tombstone) {
		return outcome.Tombstone
	}
	if outcome.Kind == contract.ElementAnchor {
		return outcome.Anchor.Revision
	}
	if outcome.Kind == contract.ElementEntity {
		return outcome.Entity.Revision
	}
	return contract.Revision{}
}

func sameCoreFacts(left, right *coreState) bool {
	if left == nil || right == nil || left.authority != right.authority || left.scope != right.scope || left.cutSerial != right.cutSerial || left.revisionSerial != right.revisionSerial || left.elements.Counts() != right.elements.Counts() {
		return false
	}
	if !sameMountFacts(left.mount, right.mount) {
		return false
	}
	if !sameAttachmentState(left.attachments, right.attachments) {
		return false
	}
	return left.elements.Equal(right.elements)
}

func sameCompleteCoreState(left, right *coreState) bool {
	if !sameCoreFacts(left, right) || left.publicKey != right.publicKey || len(left.operations) != len(right.operations) || len(left.commits) != len(right.commits) || len(left.mount.events) != len(right.mount.events) || !left.transfers.equal(right.transfers) {
		return false
	}
	if !sameRealmContact(left.realmContact, right.realmContact) {
		return false
	}
	for index, event := range left.mount.events {
		if event != right.mount.events[index] {
			return false
		}
	}
	for index, event := range left.commits {
		if event != right.commits[index] {
			return false
		}
	}
	for operationID, operation := range left.operations {
		other, exists := right.operations[operationID]
		if !exists || other != operation {
			return false
		}
	}
	return true
}

func validateCoreState(state *coreState) error {
	if err := validateCurrentSnapshot(state); err != nil {
		return err
	}
	if state.operations == nil {
		return errors.New("Core state metadata is invalid")
	}
	if err := validateMountRetained(state); err != nil {
		return err
	}
	return validateOperationHistory(state)
}

// validateCurrentSnapshot proves only the currently published owner-local fact
// cut. It is linear in current records and deliberately does not replay retained
// OperationID history; full Store-boundary validation remains validateCoreState.
func validateCurrentSnapshot(state *coreState) error {
	if state == nil || !state.authority.Valid() || state.scope == ([32]byte{}) || state.cutSerial == 0 || !state.elements.Initialized() {
		return errors.New("Core state metadata is invalid")
	}
	if !validRevisionSequence(state.revisionSerial, state.cutSerial, state.transferRevisionSurplus()) || uint64(len(state.commits)) != state.cutSerial-1 {
		return errors.New("Core cut and Revision serials are inconsistent")
	}
	if !state.realmContact.initial.Valid() || !state.realmContact.current.Valid() || state.realmContact.serial == 0 {
		return errors.New("Core Realm contact is missing or invalid")
	}
	if err := validateMountSnapshot(state); err != nil {
		return err
	}
	return validateCurrentFacts(state)
}

func validateCurrentFacts(state *coreState) error {
	return state.elements.Validate(state.scope, state.revisionSerial, state.elementExternalParent())
}
