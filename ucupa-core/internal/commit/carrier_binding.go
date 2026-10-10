package commit

import (
	"math"
	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	mountfact "ucupa/ucupa-core/internal/mount/fact"
	transferencoding "ucupa/ucupa-core/internal/transfer/encoding"
	transferfact "ucupa/ucupa-core/internal/transfer/fact"
)

// CommitCarrierBindingLocked is parent-owned and irreversible. The operation
// must revalidate the original actor's approval and authenticate both peers'
// exact evidence before this private transaction, without network I/O under
// the owner lock. Neither a source nor a target can make this decision.
func (o *Owner) CommitCarrierBindingLocked(actor values.PrincipalRef, a transferfact.CarrierAcceptance) (transferfact.CarrierBinding, error) {
	if !a.Valid() {
		return transferfact.CarrierBinding{}, contract.ErrInvalid
	}
	i := a.Fence.Permit.Reservation.Input
	c, err := o.CarrierReplacementLocked(actor, i.Operation)
	if err != nil {
		return transferfact.CarrierBinding{}, err
	}
	if o.store == nil {
		return transferfact.CarrierBinding{}, values.PublicUnavailable
	}
	old := o.state.mount.operations[i.Operation].CarrierBinding
	if old != (transferfact.CarrierBinding{}) {
		if old.Acceptance != a {
			return transferfact.CarrierBinding{}, contract.ErrOperationConflict
		}
		return old, nil
	}
	if c != a.Fence.Permit || i.Parent != o.authority {
		return transferfact.CarrierBinding{}, contract.ErrConflict
	}
	next := o.state.clone()
	e := transferencoding.TransferEvent{Kind: transferencoding.TransferCarrierBound, At: next.currentCut(), CarrierAcceptance: a}
	if err := next.applyTransferEvent(e); err != nil {
		return transferfact.CarrierBinding{}, err
	}
	if err := o.publishStateLocked(next); err != nil {
		return transferfact.CarrierBinding{}, err
	}
	return o.state.mount.operations[i.Operation].CarrierBinding, nil
}

func (o *Owner) CarrierBindingLocked(actor values.PrincipalRef, id contract.OperationID) (transferfact.CarrierBinding, error) {
	if _, err := o.CarrierReplacementLocked(actor, id); err != nil {
		return transferfact.CarrierBinding{}, err
	}
	b := o.state.mount.operations[id].CarrierBinding
	if !b.Valid() {
		return transferfact.CarrierBinding{}, values.PublicUnavailable
	}
	return b, nil
}

func (s *coreState) applyCarrierBinding(e transferencoding.TransferEvent) error {
	a := e.CarrierAcceptance
	if !a.Valid() || e.Actor != (values.PrincipalRef{}) || e.Operation != "" || e.Input != (transferfact.Intent{}) || e.Manifest != "" || e.Expected != 0 {
		return contract.ErrInvalid
	}
	i := a.Fence.Permit.Reservation.Input
	op, ok := s.mount.operations[i.Operation]
	if !ok || i.Parent != s.authority || op.Carrier != a.Fence.Permit || op.CarrierBinding != (transferfact.CarrierBinding{}) {
		return contract.ErrConflict
	}
	if err := validateCarrierMountOperationFacts(mountSnapshotFactsFromState(s), i.Operation, op); err != nil {
		return contract.ErrStore
	}
	if in := s.mount.facts.Incoming(); in.Key.Valid() && in.Gate != mountfact.GateOpen {
		return contract.ErrMountBusy
	}
	if s.cutSerial == math.MaxUint64 {
		return values.PublicResourceExhausted
	}
	s.cutSerial++
	b := transferfact.CarrierBinding{Acceptance: a, At: s.currentCut(), Revision: uint64(len(s.transfers.events)) + 1}
	if !b.Valid() {
		return contract.ErrInvalid
	}
	r := s.mount.facts.Record(i.Mount)
	r.Key = op.Carrier.Reservation.NextKey
	r.Admission = contract.MountAdmissionKey{MountKey: r.Key, OperationID: i.Operation}
	r.Revision = s.mountRevision(i.Mount)
	r.Contact = s.newContact(r.Key, i.TargetAddress, false)
	// Keep the same occupied slot until matching installation is settled;
	// ordinary range update/revoke cannot race an unfinished replacement.
	op.CarrierBinding = b
	s.mount.facts.ReplaceOutgoing(i.Mount, r)
	s.mount.operations[i.Operation] = op
	s.commits = append(s.commits, coreCommit{kind: transferCommitted, controlIndex: b.Revision})
	return validateCurrentSnapshot(s)
}
