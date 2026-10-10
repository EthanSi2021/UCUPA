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

// These private transactions reserve the parent's existing mount control slot.
// No RPC/authorization runs under this lock. The caller must authenticate the
// source intent and authorize its original actor at this parent first.
func (o *Owner) ReserveTransferParentLocked(i transferfact.ParentIntent) (transferfact.ParentHold, error) {
	if _, err := transferencoding.EncodeTransferParentIntent(i); err != nil {
		return transferfact.ParentHold{}, err
	}
	if err := o.transferPreflightReadyLocked(); err != nil {
		return transferfact.ParentHold{}, err
	}
	if o.store == nil {
		return transferfact.ParentHold{}, values.PublicUnavailable
	}
	if i.Parent != o.authority {
		return transferfact.ParentHold{}, contract.ErrConflict
	}
	if old, ok := o.state.mount.operations[i.Slot()]; ok {
		if old.Parent.Hold.Input != i {
			return transferfact.ParentHold{}, contract.ErrOperationConflict
		}
		return old.Parent.Hold, nil
	}
	next := o.state.clone()
	if err := next.applyTransferEvent(transferencoding.TransferEvent{Kind: transferencoding.TransferParentReserved, At: next.currentCut(), Parent: i}); err != nil {
		return transferfact.ParentHold{}, err
	}
	if err := o.publishStateLocked(next); err != nil {
		return transferfact.ParentHold{}, err
	}
	return o.state.mount.operations[i.Slot()].Parent.Hold, nil
}

func (o *Owner) TransferParentLocked(actor values.PrincipalRef, slot contract.OperationID) (transferfact.ParentControl, error) {
	if !actor.Valid() || len(slot) == 0 || len(slot) > 128 {
		return transferfact.ParentControl{}, contract.ErrInvalid
	}
	if err := o.transferPreflightReadyLocked(); err != nil {
		return transferfact.ParentControl{}, err
	}
	op, ok := o.state.mount.operations[slot]
	if !ok || op.Parent == (transferfact.ParentControl{}) {
		return transferfact.ParentControl{}, contract.ErrNotFound
	}
	if actor != op.Parent.Hold.Input.Initial.Input.Actor {
		return transferfact.ParentControl{}, values.PublicDenied
	}
	return op.Parent, nil
}

// The operation obtains terminal evidence from the designated source. A
// timeout, lease expiry or caller-requested abort cannot release this promise.
func (o *Owner) ReleaseTransferParentLocked(h transferfact.ParentHold, terminal transferfact.ParentRelease) (transferfact.ParentControl, error) {
	if !terminal.ValidFor(h) {
		return transferfact.ParentControl{}, contract.ErrInvalid
	}
	if err := o.transferPreflightReadyLocked(); err != nil {
		return transferfact.ParentControl{}, err
	}
	if o.store == nil {
		return transferfact.ParentControl{}, values.PublicUnavailable
	}
	slot := h.Input.Slot()
	op, ok := o.state.mount.operations[slot]
	if !ok || op.Parent.Hold != h {
		return transferfact.ParentControl{}, contract.ErrConflict
	}
	if op.Parent.ReleasedAt != (contract.CoreReadCut{}) {
		if op.Parent.Release != terminal {
			return transferfact.ParentControl{}, contract.ErrOperationConflict
		}
		return op.Parent, nil
	}
	next := o.state.clone()
	e := transferencoding.TransferEvent{Kind: transferencoding.TransferParentReleased, At: next.currentCut(), Operation: slot, Expected: h.Revision, ParentRelease: terminal}
	if err := next.applyTransferEvent(e); err != nil {
		return transferfact.ParentControl{}, err
	}
	if err := o.publishStateLocked(next); err != nil {
		return transferfact.ParentControl{}, err
	}
	return o.state.mount.operations[slot].Parent, nil
}

func (s *coreState) applyTransferParent(e transferencoding.TransferEvent) error {
	if e.Input != (transferfact.Intent{}) || e.Actor != (values.PrincipalRef{}) || e.Manifest != "" {
		return contract.ErrInvalid
	}
	if s.cutSerial == math.MaxUint64 {
		return values.PublicResourceExhausted
	}
	if e.Kind == transferencoding.TransferParentReserved {
		return s.reserveTransferParent(e)
	}
	op, ok := s.mount.operations[e.Operation]
	c := op.Parent
	if !ok || e.Kind != transferencoding.TransferParentReleased || e.Parent != (transferfact.ParentIntent{}) || !c.Valid() ||
		c.ReleasedAt != (contract.CoreReadCut{}) || e.Expected != c.Hold.Revision || !e.ParentRelease.ValidFor(c.Hold) {
		return contract.ErrConflict
	}
	i := c.Hold.Input
	r := s.mount.facts.Record(i.Mount)
	if r.Slot != e.Operation || r.Admission != c.Hold.Admission {
		return contract.ErrStore
	}
	s.cutSerial++
	c.Release, c.ReleasedAt, c.Revision = e.ParentRelease, s.currentCut(), uint64(len(s.transfers.events))+1
	r.Slot, r.Revision = "", s.mountRevision(i.Mount)
	s.mount.facts.ReplaceOutgoing(i.Mount, r)
	op.Parent = c
	s.mount.operations[e.Operation] = op
	s.commits = append(s.commits, coreCommit{kind: transferCommitted, controlIndex: c.Revision})
	return validateCurrentSnapshot(s)
}

func (s *coreState) reserveTransferParent(e transferencoding.TransferEvent) error {
	i := e.Parent
	if !i.Valid() || i.Parent != s.authority || e.Operation != "" || e.Expected != 0 || e.ParentRelease != (transferfact.ParentRelease{}) {
		return contract.ErrInvalid
	}
	if _, err := transferencoding.EncodeTransferParentIntent(i); err != nil {
		return err
	}
	if s.transferIntentCount() >= transferencoding.MaxTransferIntents {
		return values.PublicResourceExhausted
	}
	slot := i.Slot()
	if _, ok := s.operations[slot]; ok || s.mountOperationClaimed(slot) {
		return contract.ErrOperationConflict
	}
	if err := s.guardTransferScope(i.Mount); err != nil {
		return err
	}
	if in := s.mount.facts.Incoming(); in.Key.Valid() && in.Gate != mountfact.GateOpen {
		return contract.ErrMountBusy
	}
	r := s.mount.facts.Record(i.Mount)
	a, ok := s.elements.Anchor(i.Mount)
	if !ok || a.Root() || r.State != contract.MountActive || r.Key.CarrierAuthority != i.Carrier() || s.elements.HasDirectContent(i.Mount) {
		return contract.ErrConflict
	}
	if r.Slot != "" {
		return contract.ErrMountBusy
	}
	if a.Revision != i.ExpectedElement || r.Revision != i.ExpectedMount {
		return contract.ErrRevisionConflict
	}
	if r.Contact.NextHop != i.CarrierAddress() {
		return contract.ErrConflict
	}
	sphere, ok := a.Range.BoundedSphere()
	if !ok {
		return contract.ErrStore
	}
	s.cutSerial++
	h := transferfact.ParentHold{Input: i, Admission: r.Admission,
		Tuple: contract.SpatialTuple{Parent: a.Parent, LocalPose: a.LocalPose, Range: sphere}, Contact: r.Contact.Revision,
		At: s.currentCut(), Revision: uint64(len(s.transfers.events)) + 1}
	if !h.Valid() {
		return contract.ErrStore
	}
	r.Slot, r.Revision = slot, token.MountRevisionFromParts(s.scope, i.Mount, s.cutSerial)
	s.mount.facts.ReplaceOutgoing(i.Mount, r)
	s.mount.operations[slot] = MountOperation{Parent: transferfact.ParentControl{Hold: h, Revision: h.Revision}}
	s.commits = append(s.commits, coreCommit{kind: transferCommitted, controlIndex: h.Revision})
	return validateCurrentSnapshot(s)
}
