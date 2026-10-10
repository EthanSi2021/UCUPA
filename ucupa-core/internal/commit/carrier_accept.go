package commit

import (
	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	transferencoding "ucupa/ucupa-core/internal/transfer/encoding"
	transferfact "ucupa/ucupa-core/internal/transfer/fact"
)

// AcceptCarrierTargetLocked retains source bytes and a receiver reservation,
// never an active incoming relation. The operation layer must authenticate the
// parent/source and each carrier hold, and authorize the actor before this private
// transaction. It performs no I/O while holding the sole owner lock.
func (o *Owner) AcceptCarrierTargetLocked(actor values.PrincipalRef, f transferfact.CarrierFence, manifest string, closures []transferfact.EdgeClosure) (transferfact.CarrierAcceptance, error) {
	if !actor.Valid() || !f.Valid() {
		return transferfact.CarrierAcceptance{}, contract.ErrInvalid
	}
	i := f.Permit.Reservation.Input
	if actor != i.Actor {
		return transferfact.CarrierAcceptance{}, values.PublicDenied
	}
	if err := o.transferPreflightReadyLocked(); err != nil {
		return transferfact.CarrierAcceptance{}, err
	}
	if o.store == nil {
		return transferfact.CarrierAcceptance{}, values.PublicUnavailable
	}
	if i.Target != o.authority {
		return transferfact.CarrierAcceptance{}, contract.ErrConflict
	}
	closed, err := transferencoding.EncodeEdgeClosures(transferfact.EdgeFence{Carrier: f, Mount: i.Mount}, closures)
	if err != nil {
		return transferfact.CarrierAcceptance{}, err
	}
	if old, ok := o.state.transfers.targets[i.Key()]; ok {
		a := old.carrier
		if a.Revision == 0 || a.Fence != f || a.Revision > uint64(len(o.state.transfers.events)) || o.state.transfers.events[a.Revision-1].Manifest != manifest || o.state.transfers.events[a.Revision-1].Closures != closed {
			return transferfact.CarrierAcceptance{}, contract.ErrOperationConflict
		}
		return a, nil
	}
	next := o.state.clone()
	e := transferencoding.TransferEvent{Kind: transferencoding.TransferCarrierAccepted, At: next.currentCut(), CarrierFence: f, Manifest: manifest, Closures: closed}
	if err := next.applyTransferEvent(e); err != nil {
		return transferfact.CarrierAcceptance{}, err
	}
	if err := o.publishStateLocked(next); err != nil {
		return transferfact.CarrierAcceptance{}, err
	}
	return o.state.transfers.targets[i.Key()].carrier, nil
}

func (s *coreState) applyCarrierAcceptance(e transferencoding.TransferEvent) error {
	f := e.CarrierFence
	i := f.Permit.Reservation.Input
	if !f.Valid() || e.Input != (transferfact.Intent{}) || e.Operation != "" || e.Actor != (values.PrincipalRef{}) || e.Expected != 0 || i.Target != s.authority {
		return contract.ErrInvalid
	}
	if _, exists := s.transfers.targets[i.Key()]; exists {
		return contract.ErrOperationConflict
	}
	slice, err := transferencoding.DecodeCarrierManifest(f, e.Manifest)
	if err != nil {
		return err
	}
	slice, err = transferencoding.AcceptedEdges(slice, e)
	if err != nil {
		return err
	}
	if err := s.checkTransferTargetCapacity(e.Manifest, e.Closures, "", len(slice.ClosedEdges())); err != nil {
		return err
	}
	if err := s.validateReturningFacetProposals(slice); err != nil {
		return err
	}
	returns, err := s.transferReturns(slice)
	if err != nil {
		return err
	}
	if err := s.guardTransferRootActivation(); err != nil {
		return err
	}
	// Even an empty incoming space reserves its one receiver position. A
	// different pending slice/space cannot share or replace that obligation.
	for _, r := range s.transfers.targets {
		if !r.complete() {
			return contract.ErrConflict
		}
	}
	if err := transferfact.PlaceCarrier(slice, s.authority, s.currentCut(), s.elements, s.mount.facts.View, transferencoding.MaxTransferManifestRecords, returns, s.currentCarrierSettlement(), f.Permit); err != nil {
		return err
	}
	a := transferfact.CarrierAcceptance{Fence: f, At: e.At, Revision: uint64(len(s.transfers.events)) + 1}
	if !a.Valid() {
		return contract.ErrInvalid
	}
	members := slice.Members()
	ids := make([]values.ElementID, len(members))
	for index, member := range members {
		ids[index] = member.ID()
	}
	s.transfers.targets[i.Key()] = acceptedTransfer{carrier: a, members: ids, closures: slice.ClosedEdges()}
	return nil
}

func (s *coreState) currentCarrierSettlement() transferfact.CarrierSettlement {
	in := s.mount.facts.Incoming()
	for _, r := range s.transfers.carrierSources {
		if r.Fence.Valid() && r.Permit.Reservation.Previous.MountKey == in.Key {
			return r.Settlement
		}
	}
	return transferfact.CarrierSettlement{}
}
