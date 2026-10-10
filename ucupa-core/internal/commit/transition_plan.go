package commit

import (
	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	transferfact "ucupa/ucupa-core/internal/transfer/fact"
)

// PublicTransitionPlanLocked captures only this operation, never the live
// owner state. Its public comparison is reconstructed with the original signer.
func (o *Owner) PublicTransitionPlanLocked(actor values.PrincipalRef, id contract.OperationID, boundary values.ElementID) (transferfact.IntentRecord, contract.TransitionApprovalBinding, error) {
	r, err := o.publicTransitionRecord(actor, id, boundary)
	if err != nil {
		return transferfact.IntentRecord{}, contract.TransitionApprovalBinding{}, err
	}
	b, err := o.transitionBinding(r)
	return r, b, err
}

func (o *Owner) transitionBinding(r transferfact.IntentRecord) (contract.TransitionApprovalBinding, error) {
	kind := contract.TransitionSlice
	if r.Input.Profile == transferfact.RootSuccessor {
		kind = contract.TransitionRoot
	} else if r.Input.Profile != transferfact.SliceTransfer {
		return contract.TransitionApprovalBinding{}, contract.ErrConflict
	}
	i := r.Input
	return contract.TransitionApprovalBinding{Source: i.Source, Operation: i.Operation,
		Command: contract.TransitionCommand{Kind: kind, Target: i.Target, Boundary: i.Boundary,
			Expected: o.SigningLocked().ElementRevision(i.Expected), Final: i.Final,
			SourceAddress: i.SourceAddress, TargetAddress: i.TargetAddress}}, nil
}

// TransferPeerRecordLocked discloses only to the immutable target of this exact
// transfer. The trusted adapter must establish peer identity independently.
func (o *Owner) TransferPeerRecordLocked(peer values.FactAuthorityRef, b contract.TransitionApprovalBinding) (transferfact.IntentRecord, error) {
	if !peer.Valid() || b.Source != o.authority || b.Command.Target != peer || !PublicTransitionInputValid(b.Source, b.Command) {
		return transferfact.IntentRecord{}, values.PublicDenied
	}
	r, err := o.TransferIntentLocked(b.Operation)
	if err != nil {
		return transferfact.IntentRecord{}, err
	}
	actual, err := o.transitionBinding(r)
	if err != nil || actual != b {
		return transferfact.IntentRecord{}, values.PublicDenied
	}
	return r, nil
}

// Until the runtime has acquired each external hold, refuse before fencing.
// This is a safe partial implementation boundary, not an exemption from P7.2.
func (o *Owner) CheckUnconnectedTransferSourceLocked(r transferfact.IntentRecord) error {
	s, err := o.state.selectTransferSource(r.Input, transferfact.ParentHold{})
	if err != nil {
		return err
	}
	for _, member := range s.Members() {
		if member.Mount.Key.Valid() {
			return contract.ErrMountCoordinationRequired
		}
	}
	return nil
}

func (o *Owner) CheckUnconnectedTransferTargetLocked() error {
	if err := o.CheckTransferTargetAdmissionLocked(); err != nil {
		return err
	}
	if o.state.mount.facts.Incoming().Key.Valid() {
		return contract.ErrMountCoordinationRequired
	}
	return nil
}

// An already accepted obligation survives grant revocation. It is returned
// only after the operation reacquires and matches its actual source fence.
func (o *Owner) AcceptedTransferLocked(f transferfact.SourceFence) (transferfact.TargetAcceptance, bool, error) {
	if err := o.CheckTransferTargetAdmissionLocked(); err != nil {
		return transferfact.TargetAcceptance{}, false, err
	}
	r, ok := o.state.transfers.targets[f.Key()]
	if !ok {
		return transferfact.TargetAcceptance{}, false, nil
	}
	if r.receipt.Fence != f {
		return transferfact.TargetAcceptance{}, false, contract.ErrOperationConflict
	}
	return r.receipt, true, nil
}

func (o *Owner) HasTransferAcceptanceLocked(source values.FactAuthorityRef, id contract.OperationID) bool {
	_, exists := o.state.transfers.targets[transferfact.TransferKey{Controller: source, Operation: id}]
	return exists
}
