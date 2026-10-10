package commit

import (
	"math"
	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	mountfact "ucupa/ucupa-core/internal/mount/fact"
	"ucupa/ucupa-core/internal/token"
	transferencoding "ucupa/ucupa-core/internal/transfer/encoding"
	transferfact "ucupa/ucupa-core/internal/transfer/fact"
)

// Parent transactions do not authorize callers. The operation layer must
// authorize the original caller and authenticate every peer dependency.
// A reservation alone cannot be supplied to a source as permission to fence.
func (o *Owner) ReserveCarrierReplacementLocked(input transferfact.CarrierIntent) (transferfact.CarrierControl, error) {
	if _, err := transferencoding.EncodeCarrierIntent(input); err != nil {
		return transferfact.CarrierControl{}, err
	}
	if err := o.transferPreflightReadyLocked(); err != nil {
		return transferfact.CarrierControl{}, err
	}
	if o.store == nil {
		return transferfact.CarrierControl{}, values.PublicUnavailable
	}
	if input.Parent != o.authority {
		return transferfact.CarrierControl{}, contract.ErrConflict
	}
	if old, ok := o.state.mount.operations[input.Operation]; ok {
		if old.Carrier.Reservation.Input != input {
			return transferfact.CarrierControl{}, contract.ErrOperationConflict
		}
		return transferfact.CarrierControl{Reservation: old.Carrier.Reservation}, nil
	}
	next := o.state.clone()
	e := transferencoding.TransferEvent{Kind: transferencoding.TransferCarrierReserved, At: next.currentCut(), Carrier: input}
	if err := next.applyTransferEvent(e); err != nil {
		return transferfact.CarrierControl{}, err
	}
	if err := o.publishStateLocked(next); err != nil {
		return transferfact.CarrierControl{}, err
	}
	return o.state.mount.operations[input.Operation].Carrier, nil
}

func (o *Owner) CarrierReplacementLocked(actor values.PrincipalRef, id contract.OperationID) (transferfact.CarrierControl, error) {
	if !actor.Valid() || len(id) == 0 || len(id) > 128 {
		return transferfact.CarrierControl{}, contract.ErrInvalid
	}
	if err := o.transferPreflightReadyLocked(); err != nil {
		return transferfact.CarrierControl{}, err
	}
	operation, ok := o.state.mount.operations[id]
	if !ok || operation.Carrier == (transferfact.CarrierControl{}) {
		return transferfact.CarrierControl{}, contract.ErrNotFound
	}
	if operation.Carrier.Reservation.Input.Actor != actor {
		return transferfact.CarrierControl{}, values.PublicDenied
	}
	return operation.Carrier, nil
}

func (o *Owner) PermitCarrierFenceLocked(actor values.PrincipalRef, id contract.OperationID, expected contract.CoreReadCut) (transferfact.CarrierControl, error) {
	return o.advanceCarrierLocked(transferencoding.TransferCarrierPermitted, actor, id, expected)
}

func (o *Owner) CancelCarrierReservationLocked(actor values.PrincipalRef, id contract.OperationID, expected contract.CoreReadCut) (transferfact.CarrierControl, error) {
	return o.advanceCarrierLocked(transferencoding.TransferCarrierCancelled, actor, id, expected)
}

func (o *Owner) advanceCarrierLocked(kind transferencoding.TransferEventKind, actor values.PrincipalRef, id contract.OperationID, expected contract.CoreReadCut) (transferfact.CarrierControl, error) {
	c, err := o.CarrierReplacementLocked(actor, id)
	if err != nil {
		return transferfact.CarrierControl{}, err
	}
	if o.store == nil {
		return transferfact.CarrierControl{}, values.PublicUnavailable
	}
	if expected != c.Reservation.At {
		return transferfact.CarrierControl{}, contract.ErrRevisionConflict
	}
	if kind == transferencoding.TransferCarrierPermitted && c.PermitAt != (contract.CoreReadCut{}) {
		return transferfact.CarrierControl{Reservation: c.Reservation, PermitAt: c.PermitAt}, nil
	}
	// Once a permit can have reached the old carrier, only its durable refusal
	// can reopen cancellation. Network silence is never that proof.
	if kind == transferencoding.TransferCarrierCancelled && c.PermitAt != (contract.CoreReadCut{}) {
		return transferfact.CarrierControl{}, values.PublicUnavailable
	}
	if kind == transferencoding.TransferCarrierCancelled && c.CancelledAt != (contract.CoreReadCut{}) {
		return c, nil
	}
	next := o.state.clone()
	e := transferencoding.TransferEvent{Kind: kind, At: next.currentCut(), Actor: actor, Operation: id, CarrierExpected: expected}
	if err := next.applyTransferEvent(e); err != nil {
		return transferfact.CarrierControl{}, err
	}
	if err := o.publishStateLocked(next); err != nil {
		return transferfact.CarrierControl{}, err
	}
	return o.state.mount.operations[id].Carrier, nil
}

func (s *coreState) transferIntentCount() int {
	n := len(s.transfers.records) + len(s.transfers.carrierSources) + len(s.transfers.edges) + len(s.transfers.replacements)
	for _, operation := range s.mount.operations {
		if operation.transferControl() {
			n++
		}
	}
	return n
}

func (s *coreState) applyCarrierControl(e transferencoding.TransferEvent) error {
	if e.Input != (transferfact.Intent{}) || e.Expected != 0 || e.Manifest != "" {
		return contract.ErrInvalid
	}
	if s.cutSerial == math.MaxUint64 {
		return values.PublicResourceExhausted
	}
	if e.Kind == transferencoding.TransferCarrierReserved {
		return s.reserveCarrier(e)
	}
	if e.Carrier != (transferfact.CarrierIntent{}) || !e.Actor.Valid() || !token.Valid(e.CarrierExpected) {
		return contract.ErrInvalid
	}
	op, ok := s.mount.operations[e.Operation]
	c := op.Carrier
	if !ok || !c.Valid() || c.Reservation.Input.Actor != e.Actor || c.Reservation.At != e.CarrierExpected || c.PermitAt != (contract.CoreReadCut{}) || c.CancelledAt != (contract.CoreReadCut{}) {
		return contract.ErrConflict
	}
	r := s.mount.facts.Record(c.Reservation.Input.Mount)
	if r.Slot != e.Operation || r.Key != c.Reservation.Previous.MountKey {
		return contract.ErrStore
	}
	if e.Kind != transferencoding.TransferCarrierPermitted && e.Kind != transferencoding.TransferCarrierCancelled {
		return contract.ErrInvalid
	}
	if e.Kind == transferencoding.TransferCarrierPermitted {
		if r.Contact.Revision != c.Reservation.PreviousContact {
			return contract.ErrRevisionConflict
		}
		if incoming := s.mount.facts.Incoming(); incoming.Key.Valid() && incoming.Gate != mountfact.GateOpen {
			return contract.ErrMountBusy
		}
	}
	s.cutSerial++
	if e.Kind == transferencoding.TransferCarrierPermitted {
		c.PermitAt = s.currentCut()
	} else {
		c.CancelledAt = s.currentCut()
		r.Slot = ""
		r.Revision = s.mountRevision(c.Reservation.Input.Mount)
		s.mount.facts.ReplaceOutgoing(c.Reservation.Input.Mount, r)
	}
	op.Carrier = c
	s.mount.operations[e.Operation] = op
	s.commits = append(s.commits, coreCommit{kind: transferCommitted, controlIndex: uint64(len(s.transfers.events)) + 1})
	return validateCurrentSnapshot(s)
}

func (s *coreState) reserveCarrier(e transferencoding.TransferEvent) error {
	i := e.Carrier
	if !i.Valid() || i.Parent != s.authority || e.Operation != "" || e.Actor != (values.PrincipalRef{}) || e.CarrierExpected != (contract.CoreReadCut{}) {
		return contract.ErrInvalid
	}
	if _, err := transferencoding.EncodeCarrierIntent(i); err != nil {
		return err
	}
	if s.transferIntentCount() >= transferencoding.MaxTransferIntents {
		return values.PublicResourceExhausted
	}
	if _, ok := s.operations[i.Operation]; ok || s.transferClaimed(i.Operation) || s.mountOperationClaimed(i.Operation) {
		return contract.ErrOperationConflict
	}
	if err := s.guardTransferScope(i.Mount); err != nil {
		return err
	}
	if incoming := s.mount.facts.Incoming(); incoming.Key.Valid() && incoming.Gate != mountfact.GateOpen {
		return contract.ErrMountBusy
	}
	r := s.mount.facts.Record(i.Mount)
	a, ok := s.elements.Anchor(i.Mount)
	if !ok || a.Root() || r.State != contract.MountActive || r.Key.CarrierAuthority != i.Source || s.elements.HasDirectContent(i.Mount) {
		return contract.ErrConflict
	}
	if r.Slot != "" {
		return contract.ErrMountBusy
	}
	if a.Revision != i.ExpectedElement || r.Revision != i.ExpectedMount {
		return contract.ErrRevisionConflict
	}
	if r.Contact.NextHop != i.SourceAddress {
		return contract.ErrConflict
	}
	sphere, ok := a.Range.BoundedSphere()
	if !ok {
		return contract.ErrStore
	}
	s.cutSerial++
	r.Revision = s.mountRevision(i.Mount)
	r.Slot = i.Operation
	c := transferfact.CarrierControl{Reservation: transferfact.CarrierReservation{Input: i, Previous: r.Admission, PreviousContact: r.Contact.Revision,
		Tuple:   contract.SpatialTuple{Parent: a.Parent, LocalPose: a.LocalPose, Range: sphere},
		NextKey: contract.MountKey{ParentAuthority: i.Parent, CarrierAuthority: i.Target, MountAnchorUUID: i.Mount, ProposalRevision: r.Revision}, At: s.currentCut()}}
	if !c.Valid() {
		return contract.ErrStore
	}
	s.mount.facts.ReplaceOutgoing(i.Mount, r)
	s.mount.operations[i.Operation] = MountOperation{Carrier: c}
	s.commits = append(s.commits, coreCommit{kind: transferCommitted, controlIndex: uint64(len(s.transfers.events)) + 1})
	return validateCurrentSnapshot(s)
}
