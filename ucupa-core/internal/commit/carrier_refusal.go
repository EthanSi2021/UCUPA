package commit

import (
	"math"
	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	transferencoding "ucupa/ucupa-core/internal/transfer/encoding"
	transferfact "ucupa/ucupa-core/internal/transfer/fact"
)

// Foreign source obligations belong to the same transfer journal and owner.
// They do not occupy the source's local operation-ID namespace: the parent is
// the controller. Source fence admission must use this same qualified record.
type carrierSourceRecord struct {
	Permit     transferfact.CarrierControl
	Refusal    transferfact.CarrierRefusal
	Fence      transferfact.CarrierFence
	Settlement transferfact.CarrierSettlement
}

// These private transactions require the operation layer to authenticate the
// designated parent/source and authorize the original actor. A serialized
// control or refusal by itself is not a credential and has no public entry.
func (o *Owner) RefuseCarrierFenceLocked(actor values.PrincipalRef, permit transferfact.CarrierControl) (transferfact.CarrierRefusal, error) {
	if !permit.Valid() || permit.PermitAt == (contract.CoreReadCut{}) || permit.CancelledAt != (contract.CoreReadCut{}) {
		return transferfact.CarrierRefusal{}, contract.ErrInvalid
	}
	i := permit.Reservation.Input
	if !actor.Valid() || actor != i.Actor {
		return transferfact.CarrierRefusal{}, values.PublicDenied
	}
	if err := o.transferPreflightReadyLocked(); err != nil {
		return transferfact.CarrierRefusal{}, err
	}
	if o.store == nil {
		return transferfact.CarrierRefusal{}, values.PublicUnavailable
	}
	if i.Source != o.authority {
		return transferfact.CarrierRefusal{}, contract.ErrConflict
	}
	if old, ok := o.state.transfers.carrierSources[i.Key()]; ok {
		if old.Permit != permit {
			return transferfact.CarrierRefusal{}, contract.ErrOperationConflict
		}
		if !old.Refusal.Valid() {
			return transferfact.CarrierRefusal{}, values.PublicUnavailable
		}
		return old.Refusal, nil
	}
	next := o.state.clone()
	if err := next.applyTransferEvent(transferencoding.TransferEvent{Kind: transferencoding.TransferCarrierRefused, At: next.currentCut(), CarrierPermit: permit}); err != nil {
		return transferfact.CarrierRefusal{}, err
	}
	if err := o.publishStateLocked(next); err != nil {
		return transferfact.CarrierRefusal{}, err
	}
	return o.state.transfers.carrierSources[i.Key()].Refusal, nil
}

func (o *Owner) CarrierSourceRefusalLocked(actor values.PrincipalRef, key transferfact.TransferKey) (transferfact.CarrierRefusal, error) {
	if !actor.Valid() || !key.Valid() {
		return transferfact.CarrierRefusal{}, contract.ErrInvalid
	}
	if err := o.transferPreflightReadyLocked(); err != nil {
		return transferfact.CarrierRefusal{}, err
	}
	r, ok := o.state.transfers.carrierSources[key]
	if !ok {
		return transferfact.CarrierRefusal{}, contract.ErrNotFound
	}
	if r.Permit.Reservation.Input.Actor != actor {
		return transferfact.CarrierRefusal{}, values.PublicDenied
	}
	if !r.Refusal.Valid() {
		return transferfact.CarrierRefusal{}, values.PublicUnavailable
	}
	return r.Refusal, nil
}

func (o *Owner) CancelPermittedCarrierLocked(actor values.PrincipalRef, id contract.OperationID, refusal transferfact.CarrierRefusal) (transferfact.CarrierControl, error) {
	c, err := o.CarrierReplacementLocked(actor, id)
	if err != nil {
		return transferfact.CarrierControl{}, err
	}
	if o.store == nil {
		return transferfact.CarrierControl{}, values.PublicUnavailable
	}
	if !refusal.Matches(c) {
		return transferfact.CarrierControl{}, contract.ErrConflict
	}
	if o.state.mount.operations[id].CarrierBinding != (transferfact.CarrierBinding{}) {
		return transferfact.CarrierControl{}, contract.ErrConflict
	}
	if c.CancelledAt != (contract.CoreReadCut{}) {
		if c.Refusal != refusal {
			return transferfact.CarrierControl{}, contract.ErrOperationConflict
		}
		return c, nil
	}
	next := o.state.clone()
	e := transferencoding.TransferEvent{Kind: transferencoding.TransferCarrierAborted, At: next.currentCut(), Operation: id, Actor: actor, CarrierRefusal: refusal}
	if err := next.applyTransferEvent(e); err != nil {
		return transferfact.CarrierControl{}, err
	}
	if err := o.publishStateLocked(next); err != nil {
		return transferfact.CarrierControl{}, err
	}
	return o.state.mount.operations[id].Carrier, nil
}

func (s *coreState) applyCarrierRefusal(e transferencoding.TransferEvent) error {
	if e.Input != (transferfact.Intent{}) || e.Carrier != (transferfact.CarrierIntent{}) || e.CarrierExpected != (contract.CoreReadCut{}) || e.Expected != 0 || e.Manifest != "" {
		return contract.ErrInvalid
	}
	if e.Kind == transferencoding.TransferCarrierRefused {
		c := e.CarrierPermit
		i := c.Reservation.Input
		if !c.Valid() || c.PermitAt == (contract.CoreReadCut{}) || c.CancelledAt != (contract.CoreReadCut{}) || i.Source != s.authority || e.Operation != "" || e.Actor != (values.PrincipalRef{}) {
			return contract.ErrInvalid
		}
		if _, err := transferencoding.EncodeCarrierIntent(i); err != nil {
			return err
		}
		if _, exists := s.transfers.carrierSources[i.Key()]; exists {
			return contract.ErrOperationConflict
		}
		if s.transferIntentCount() >= transferencoding.MaxTransferIntents {
			return values.PublicResourceExhausted
		}
		r := transferfact.CarrierRefusal{Key: i.Key(), Source: s.authority, ReservationAt: c.Reservation.At, PermitAt: c.PermitAt, At: e.At, Revision: uint64(len(s.transfers.events)) + 1}
		if !r.Valid() {
			return contract.ErrConflict
		}
		// The absence of a record is checked in the original source owner, not
		// inferred by the parent. Once retained, neither retries nor a different
		// parent connection can remove this permanent no-fence disposition.
		s.transfers.carrierSources[i.Key()] = carrierSourceRecord{Permit: c, Refusal: r}
		return nil
	}
	if e.Kind != transferencoding.TransferCarrierAborted || !e.Actor.Valid() {
		return contract.ErrInvalid
	}
	op, ok := s.mount.operations[e.Operation]
	c := op.Carrier
	if !ok || !c.Valid() || c.CancelledAt != (contract.CoreReadCut{}) || op.CarrierBinding != (transferfact.CarrierBinding{}) || c.Reservation.Input.Actor != e.Actor || !e.CarrierRefusal.Matches(c) {
		return contract.ErrConflict
	}
	r := s.mount.facts.Record(c.Reservation.Input.Mount)
	if r.Slot != e.Operation || r.Key != c.Reservation.Previous.MountKey || r.Admission != c.Reservation.Previous {
		return contract.ErrStore
	}
	if s.cutSerial == math.MaxUint64 {
		return values.PublicResourceExhausted
	}
	s.cutSerial++
	c.CancelledAt, c.Refusal = s.currentCut(), e.CarrierRefusal
	r.Slot, r.Revision = "", s.mountRevision(c.Reservation.Input.Mount)
	s.mount.facts.ReplaceOutgoing(c.Reservation.Input.Mount, r)
	op.Carrier = c
	s.mount.operations[e.Operation] = op
	s.commits = append(s.commits, coreCommit{kind: transferCommitted, controlIndex: uint64(len(s.transfers.events)) + 1})
	return validateCurrentSnapshot(s)
}
