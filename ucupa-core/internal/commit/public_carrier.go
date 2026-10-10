package commit

import (
	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	transferfact "ucupa/ucupa-core/internal/transfer/fact"
)

// PublicCarrierInputValid checks only bounded shape, not authentication or CAS.
func PublicCarrierInputValid(parent values.FactAuthorityRef, c contract.CarrierCommand) bool {
	return parent.Valid() && c.Source.Valid() && c.Target.Valid() && parent != c.Source && parent != c.Target && c.Source != c.Target && c.Mount.Valid() &&
		c.ExpectedElement.Valid() && c.ExpectedElement.Owner == parent && c.ExpectedElement.Kind == "record-revision" &&
		c.ExpectedMount.Valid() && c.ExpectedMount.Owner == parent && c.ExpectedMount.Kind == "mount-revision" &&
		c.ParentAddress.Valid() && c.SourceAddress.Valid() && c.TargetAddress.Valid()
}

func (o *Owner) publicCarrierCommand(i transferfact.CarrierIntent) contract.CarrierCommand {
	signer := o.SigningLocked()
	return contract.CarrierCommand{Source: i.Source, Target: i.Target, Mount: i.Mount,
		ExpectedElement: signer.ElementRevision(i.ExpectedElement), ExpectedMount: signer.MountRevision(i.ExpectedMount),
		ParentAddress: i.ParentAddress, SourceAddress: i.SourceAddress, TargetAddress: i.TargetAddress}
}

// BeginPublicCarrierLocked only reserves the existing parent mount slot. The
// original command is compared before looking at newer mount revisions. A
// retry returns the original reservation even after cancellation or settlement.
func (o *Owner) BeginPublicCarrierLocked(actor values.PrincipalRef, id contract.OperationID, command contract.CarrierCommand) (contract.CarrierStatus, error) {
	if !actor.Valid() || len(id) == 0 || len(id) > 128 || !PublicCarrierInputValid(o.authority, command) {
		return contract.CarrierStatus{}, contract.ErrInvalid
	}
	if err := o.transferPreflightReadyLocked(); err != nil {
		return contract.CarrierStatus{}, err
	}
	if old, exists := o.state.mount.operations[id]; exists {
		c := old.Carrier
		if !c.Valid() || c.Reservation.Input.Actor != actor || o.publicCarrierCommand(c.Reservation.Input) != command {
			return contract.CarrierStatus{}, contract.ErrOperationConflict
		}
		return o.publicCarrierStatus(MountOperation{Carrier: transferfact.CarrierControl{Reservation: c.Reservation}})
	}
	a, exists := o.state.elements.Anchor(command.Mount)
	if !exists {
		return contract.CarrierStatus{}, contract.ErrNotFound
	}
	i := transferfact.CarrierIntent{Operation: id, Actor: actor, Parent: o.authority, Source: command.Source, Target: command.Target,
		Mount: command.Mount, ExpectedElement: a.Revision, ExpectedMount: o.state.mount.facts.Record(command.Mount).Revision,
		ParentAddress: command.ParentAddress, SourceAddress: command.SourceAddress, TargetAddress: command.TargetAddress}
	if o.publicCarrierCommand(i) != command {
		return contract.CarrierStatus{}, contract.ErrRevisionConflict
	}
	c, err := o.ReserveCarrierReplacementLocked(i)
	if err != nil {
		return contract.CarrierStatus{}, err
	}
	return o.publicCarrierStatus(MountOperation{Carrier: c})
}

func (o *Owner) InspectPublicCarrierLocked(actor values.PrincipalRef, id contract.OperationID, mount values.ElementID) (contract.CarrierStatus, error) {
	op, err := o.publicCarrierRecord(actor, id, mount)
	if err != nil {
		return contract.CarrierStatus{}, err
	}
	return o.publicCarrierStatus(op)
}

// Local cancellation is safe only before dispatch permission. A permitted
// operation needs the source's durable refusal; this entry never infers one
// from a timeout. A previously cancelled reservation returns its exact result.
func (o *Owner) CancelPublicCarrierLocked(actor values.PrincipalRef, id contract.OperationID, mount values.ElementID, expected values.PublicToken) (contract.CarrierStatus, error) {
	if !expected.Valid() || expected.Owner != o.authority || expected.Kind != "carrier-revision" {
		return contract.CarrierStatus{}, contract.ErrInvalid
	}
	op, err := o.publicCarrierRecord(actor, id, mount)
	if err != nil {
		return contract.CarrierStatus{}, err
	}
	c := op.Carrier
	if o.SigningLocked().CarrierRevision(c.Reservation.At, c.Reservation.At, string(id), mount) != expected {
		return contract.CarrierStatus{}, contract.ErrRevisionConflict
	}
	c, err = o.CancelCarrierReservationLocked(actor, id, c.Reservation.At)
	if err != nil {
		return contract.CarrierStatus{}, err
	}
	return o.publicCarrierStatus(MountOperation{Carrier: c})
}

func (o *Owner) publicCarrierRecord(actor values.PrincipalRef, id contract.OperationID, mount values.ElementID) (MountOperation, error) {
	if !mount.Valid() {
		return MountOperation{}, contract.ErrInvalid
	}
	c, err := o.CarrierReplacementLocked(actor, id)
	if err != nil {
		return MountOperation{}, err
	}
	if c.Reservation.Input.Mount != mount {
		return MountOperation{}, values.PublicDenied
	}
	return o.state.mount.operations[id], nil
}

func (o *Owner) publicCarrierStatus(op MountOperation) (contract.CarrierStatus, error) {
	c := op.Carrier
	if !c.Valid() {
		return contract.CarrierStatus{}, contract.ErrStore
	}
	i := c.Reservation.Input
	progress, at := contract.CarrierReserved, c.Reservation.At
	if c.PermitAt != (contract.CoreReadCut{}) {
		progress, at = contract.CarrierPermitted, c.PermitAt
	}
	if c.CancelledAt != (contract.CoreReadCut{}) {
		progress, at = contract.CarrierCancelled, c.CancelledAt
	}
	if op.CarrierBinding.Valid() {
		progress, at = contract.CarrierCommitted, op.CarrierBinding.At
	}
	if op.CarrierSettlement.Valid(o.authority) {
		progress, at = contract.CarrierSettled, op.CarrierSettlement.At
	}
	revision := o.SigningLocked().CarrierRevision(c.Reservation.At, at, string(i.Operation), i.Mount)
	if !revision.Valid() {
		return contract.CarrierStatus{}, contract.ErrStore
	}
	return contract.CarrierStatus{Parent: o.authority, Source: i.Source, Target: i.Target, Operation: i.Operation,
		Mount: i.Mount, Progress: progress, Revision: revision}, nil
}
