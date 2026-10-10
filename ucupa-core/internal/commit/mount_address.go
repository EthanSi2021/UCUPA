package commit

import (
	"ucupa/ucupa-core/contract"
	commitbinding "ucupa/ucupa-core/internal/commit/binding"
	elementfact "ucupa/ucupa-core/internal/element/fact"
	mountfact "ucupa/ucupa-core/internal/mount/fact"
	"ucupa/ucupa-core/internal/token"
)

// prepareMountAddress keeps retry receipts separate from the decision error:
// a newly retained rejection still has to be persisted before it is returned.
func (state *coreState) prepareMountAddress(id contract.OperationID, binding NextHopBinding) (*coreState, nextHopOperation, error) {
	if retained, exists := state.mount.addresses[id]; exists {
		if retained.binding != binding {
			return nil, nextHopOperation{}, contract.ErrOperationConflict
		}
		return nil, retained, nil
	}
	if _, exists := state.operations[id]; exists {
		return nil, nextHopOperation{}, contract.ErrOperationConflict
	}
	if _, exists := state.mount.operations[id]; exists {
		return nil, nextHopOperation{}, contract.ErrOperationConflict
	}
	next, err := state.prepareMountEvent(MountEvent{Kind: MountEventAddress, Operation: id, Address: binding, Authorized: true})
	if err != nil {
		return nil, nextHopOperation{}, err
	}
	return next, next.mount.addresses[id], nil
}

func updateMountAddress(state *coreState, event MountEvent) error {
	binding, id := event.Address, event.Operation
	if !event.Authorized || !commitbinding.ValidOperationID(id) || !binding.Actor.Valid() || !binding.Key.Valid() || binding.Element != binding.Key.MountAnchorUUID || !token.ValidNextHopRevision(binding.Expected) || !binding.NextHop.Valid() {
		return contract.ErrInvalid
	}
	if state.mountOperationClaimed(id) {
		return contract.ErrOperationConflict
	}
	if _, exists := state.operations[id]; exists {
		return contract.ErrOperationConflict
	}
	incoming := binding.Key.CarrierAuthority == state.authority
	if !incoming && binding.Key.ParentAuthority != state.authority {
		return contract.ErrInvalid
	}
	var contact mountfact.Contact
	var currentKey contract.MountKey
	if incoming {
		currentKey, contact = state.mount.facts.Incoming().Key, state.mount.facts.Incoming().Contact
	} else {
		record := state.mount.facts.Record(binding.Element)
		currentKey, contact = record.Key, record.Contact
	}
	outcome := nextHopOperation{binding: binding}
	switch {
	case !currentKey.Valid():
		outcome.errorKind = elementfact.NotFound
	case currentKey != binding.Key:
		outcome.errorKind = elementfact.Conflict
	case contact.Revision != binding.Expected:
		outcome.errorKind = elementfact.RevisionConflict
	default:
		changed := contact.NextHop != binding.NextHop
		if changed {
			if err := state.issueControlChange(); err != nil {
				return err
			}
			contact = state.newContact(binding.Key, binding.NextHop, incoming)
			if incoming {
				relation := state.mount.facts.Incoming()
				relation.Contact = contact
				state.mount.facts.ReplaceIncoming(relation)
			} else {
				record := state.mount.facts.Record(binding.Element)
				record.Contact = contact
				state.mount.facts.ReplaceOutgoing(binding.Element, record)
			}
		}
		outcome.result = contract.NextHopUpdateResult{OperationID: id, MountKey: binding.Key, NextHop: contact.NextHop, NextHopRevision: contact.Revision, Changed: changed, Cut: state.currentCut()}
	}
	state.mount.addresses[id] = outcome
	return nil
}

func (owner *Owner) ApplyMountAddressLocked(id contract.OperationID, binding NextHopBinding) (contract.NextHopUpdateResult, error) {
	next, outcome, err := owner.state.prepareMountAddress(id, binding)
	if err != nil {
		return contract.NextHopUpdateResult{}, err
	}
	if next != nil {
		if err := owner.publishStateLocked(next); err != nil {
			return contract.NextHopUpdateResult{}, err
		}
	}
	return outcome.output()
}
func (operation nextHopOperation) output() (contract.NextHopUpdateResult, error) {
	switch operation.errorKind {
	case elementfact.NoError:
		return operation.result, nil
	case elementfact.NotFound:
		return contract.NextHopUpdateResult{}, contract.ErrNotFound
	case elementfact.Conflict:
		return contract.NextHopUpdateResult{}, contract.ErrConflict
	case elementfact.RevisionConflict:
		return contract.NextHopUpdateResult{}, contract.ErrRevisionConflict
	default:
		return contract.NextHopUpdateResult{}, contract.ErrStore
	}
}
