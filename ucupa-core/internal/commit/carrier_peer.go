package commit

import (
	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	"ucupa/ucupa-core/internal/transfer/fact"
)

// Existence only selects fresh authorization versus fulfilment. The complete
// original permit and peer binding are still checked before returning a proof.
func (o *Owner) HasCarrierSourceFenceLocked(parent values.FactAuthorityRef, id contract.OperationID) bool {
	r, ok := o.state.transfers.carrierSources[fact.TransferKey{Controller: parent, Operation: id}]
	return ok && r.Fence.Valid()
}

// The public exact binding is checked against the parent's original signer
// and original record. Caller/peer admission belongs to the operation layer.
func (o *Owner) CarrierPlanLocked(b contract.CarrierApprovalBinding) (fact.CarrierControl, fact.CarrierBinding, error) {
	if b.Parent != o.authority || !PublicCarrierInputValid(b.Parent, b.Command) || len(b.Operation) == 0 || len(b.Operation) > 128 {
		return fact.CarrierControl{}, fact.CarrierBinding{}, contract.ErrInvalid
	}
	if err := o.transferPreflightReadyLocked(); err != nil {
		return fact.CarrierControl{}, fact.CarrierBinding{}, err
	}
	op, ok := o.state.mount.operations[b.Operation]
	if !ok || !op.Carrier.Valid() {
		return fact.CarrierControl{}, fact.CarrierBinding{}, contract.ErrNotFound
	}
	if o.publicCarrierCommand(op.Carrier.Reservation.Input) != b.Command {
		return fact.CarrierControl{}, fact.CarrierBinding{}, contract.ErrOperationConflict
	}
	return op.Carrier, op.CarrierBinding, nil
}

// External holds are a remaining P7.2 workflow, not optional evidence. Until
// acquired by that workflow, reject before withdrawing any source members.
func (o *Owner) CheckUnconnectedCarrierSourceLocked(c fact.CarrierControl) error {
	s, _, err := o.state.captureCarrierManifest(c)
	if err != nil {
		return err
	}
	for _, m := range s.Members() {
		if m.Mount.Key.Valid() {
			return contract.ErrMountCoordinationRequired
		}
	}
	return nil
}

// Reading completion cannot cause an installation or fulfil an external edge.
func (o *Owner) CarrierCompletionLocked(b fact.CarrierBinding) (fact.CoreCompletion, error) {
	if !b.Valid() || b.Acceptance.Fence.Permit.Reservation.Input.Target != o.authority {
		return fact.CoreCompletion{}, contract.ErrInvalid
	}
	if err := o.transferPreflightReadyLocked(); err != nil {
		return fact.CoreCompletion{}, err
	}
	r, ok := o.state.transfers.targets[b.Acceptance.Fence.Permit.Reservation.Input.Key()]
	if !ok || r.carrierInstallation.Binding != b || !r.carrierInstallation.Valid() {
		return fact.CoreCompletion{}, values.PublicUnavailable
	}
	if len(r.closures) != 0 {
		if !r.completion.Valid() {
			return fact.CoreCompletion{}, values.PublicUnavailable
		}
		return r.completion, nil
	}
	i := fact.CoreInstallation{Carrier: r.carrierInstallation}
	return fact.CoreCompletion{Installed: i, At: i.Cut(), Revision: i.Revision()}, nil
}
