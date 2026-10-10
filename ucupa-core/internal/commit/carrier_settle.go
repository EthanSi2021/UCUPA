package commit

import (
	"math"
	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	mountfact "ucupa/ucupa-core/internal/mount/fact"
	transferencoding "ucupa/ucupa-core/internal/transfer/encoding"
	transferfact "ucupa/ucupa-core/internal/transfer/fact"
)

// The operation layer must obtain exact installation evidence from the
// authenticated designated target. These local acknowledgements never make a
// second binding decision and perform no network I/O under the owner lock.
func (o *Owner) SettleCarrierParentLocked(c transferfact.CoreCompletion) (transferfact.CarrierSettlement, error) {
	return o.settleCarrierLocked(c, transferencoding.TransferCarrierParentSettled)
}

func (o *Owner) SettleCarrierSourceLocked(c transferfact.CoreCompletion) (transferfact.CarrierSettlement, error) {
	return o.settleCarrierLocked(c, transferencoding.TransferCarrierSourceSettled)
}

func (s *coreState) carrierSettlement(i transferfact.CarrierInstallation, kind transferencoding.TransferEventKind) (transferfact.CarrierSettlement, error) {
	input := i.Binding.Acceptance.Fence.Permit.Reservation.Input
	if kind == transferencoding.TransferCarrierParentSettled {
		op, ok := s.mount.operations[input.Operation]
		if input.Parent != s.authority || !ok || op.CarrierBinding != i.Binding {
			return transferfact.CarrierSettlement{}, contract.ErrOperationConflict
		}
		return op.CarrierSettlement, nil
	}
	r, ok := s.transfers.carrierSources[input.Key()]
	if kind != transferencoding.TransferCarrierSourceSettled || input.Source != s.authority || !ok || r.Fence != i.Binding.Acceptance.Fence {
		return transferfact.CarrierSettlement{}, contract.ErrOperationConflict
	}
	return r.Settlement, nil
}

func (o *Owner) settleCarrierLocked(c transferfact.CoreCompletion, kind transferencoding.TransferEventKind) (transferfact.CarrierSettlement, error) {
	i := c.Installed.Carrier
	if !c.Valid() || c.Installed.Source != (transferfact.TargetInstallation{}) || !i.Valid() {
		return transferfact.CarrierSettlement{}, contract.ErrInvalid
	}
	if err := o.transferPreflightReadyLocked(); err != nil {
		return transferfact.CarrierSettlement{}, err
	}
	if o.store == nil {
		return transferfact.CarrierSettlement{}, values.PublicUnavailable
	}
	old, err := o.state.carrierSettlement(i, kind)
	if err != nil {
		return transferfact.CarrierSettlement{}, err
	}
	if old.Revision != 0 {
		if old.Installation != i || o.state.transfers.events[old.Revision-1].Completion != c {
			return transferfact.CarrierSettlement{}, contract.ErrOperationConflict
		}
		return old, nil
	}
	next := o.state.clone()
	if err := next.applyTransferEvent(transferencoding.TransferEvent{Kind: kind, At: next.currentCut(), CarrierInstallation: i, Completion: c}); err != nil {
		return transferfact.CarrierSettlement{}, err
	}
	if err := o.publishStateLocked(next); err != nil {
		return transferfact.CarrierSettlement{}, err
	}
	return o.state.carrierSettlement(i, kind)
}

func (s *coreState) applyCarrierSettlement(e transferencoding.TransferEvent) error {
	if !e.Completion.Valid() || e.Completion.Installed != transferencoding.SettlementParent(e) {
		return contract.ErrInvalid
	}
	i := e.CarrierInstallation
	if !i.Valid() || e.Input != (transferfact.Intent{}) || e.Operation != "" || e.Actor != (values.PrincipalRef{}) || e.Expected != 0 || e.Manifest != "" {
		return contract.ErrInvalid
	}
	old, err := s.carrierSettlement(i, e.Kind)
	if err != nil {
		return err
	}
	if old.Revision != 0 {
		return contract.ErrConflict
	}
	input := i.Binding.Acceptance.Fence.Permit.Reservation.Input
	if e.Kind == transferencoding.TransferCarrierParentSettled {
		if err := validateCarrierMountOperationFacts(mountSnapshotFactsFromState(s), input.Operation, s.mount.operations[input.Operation]); err != nil {
			return contract.ErrStore
		}
	} else if err := s.validateCarrierDeparture(i, e.Completion); err != nil {
		return err
	}
	if s.cutSerial == math.MaxUint64 {
		return values.PublicResourceExhausted
	}
	s.cutSerial++
	settled := transferfact.CarrierSettlement{Installation: i, At: s.currentCut(), Revision: uint64(len(s.transfers.events)) + 1}
	if !settled.Valid(s.authority) {
		return contract.ErrStore
	}
	if e.Kind == transferencoding.TransferCarrierParentSettled {
		r := s.mount.facts.Record(input.Mount)
		r.Slot, r.Revision = "", s.mountRevision(input.Mount)
		s.mount.facts.ReplaceOutgoing(input.Mount, r)
		op := s.mount.operations[input.Operation]
		op.CarrierSettlement = settled
		s.mount.operations[input.Operation] = op
	} else {
		r := s.transfers.carrierSources[input.Key()]
		r.Settlement = settled
		s.transfers.carrierSources[input.Key()] = r
		// Keep the fenced incoming relation and departure claims. Finishing this
		// old obligation is not permission to accept arbitrary new content.
	}
	s.commits = append(s.commits, coreCommit{kind: transferCommitted, controlIndex: settled.Revision})
	return validateCurrentSnapshot(s)
}

func (s *coreState) validateCarrierDeparture(i transferfact.CarrierInstallation, completion transferfact.CoreCompletion) error {
	f := i.Binding.Acceptance.Fence
	input := f.Permit.Reservation.Input
	r := s.transfers.carrierSources[input.Key()]
	_, raw, err := s.carrierSourceFence(r)
	if err != nil {
		return err
	}
	in := s.mount.facts.Incoming()
	if in.Key != f.Permit.Reservation.Previous.MountKey || in.Gate != mountfact.GateFenced || in.Preparing != (contract.MountAttemptKey{}) || in.Detached {
		return contract.ErrConflict
	}
	slice, err := transferencoding.DecodeCarrierManifest(f, raw)
	if err != nil {
		return err
	}
	if slice.HasMounts() && completion.At == i.At {
		return contract.ErrMountCoordinationRequired
	}
	for _, member := range slice.Members() {
		d, ok := s.elements.Departure(member.ID())
		revision := member.Entity.Revision
		if member.Kind == contract.ElementAnchor {
			revision = member.Anchor.Revision
		}
		if !ok || d.Controller != input.Parent || d.Operation != input.Operation || d.Revision != revision || d.Root || s.elements.Kind(member.ID()) != 0 {
			return contract.ErrStore
		}
	}
	return nil
}
