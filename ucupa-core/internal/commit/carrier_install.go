package commit

import (
	"math"
	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	mountfact "ucupa/ucupa-core/internal/mount/fact"
	transferencoding "ucupa/ucupa-core/internal/transfer/encoding"
	transferfact "ucupa/ucupa-core/internal/transfer/fact"
)

// InstallCarrierTargetLocked fulfils the exact accepted obligation. Its caller
// must authenticate the designated parent's durable binding, not trust supplied
// bytes. Content and matching incoming relation publish in the same owner cut.
func (o *Owner) InstallCarrierTargetLocked(b transferfact.CarrierBinding) (transferfact.CarrierInstallation, error) {
	if !b.Valid() {
		return transferfact.CarrierInstallation{}, contract.ErrInvalid
	}
	if err := o.transferPreflightReadyLocked(); err != nil {
		return transferfact.CarrierInstallation{}, err
	}
	if o.store == nil {
		return transferfact.CarrierInstallation{}, values.PublicUnavailable
	}
	i := b.Acceptance.Fence.Permit.Reservation.Input
	r, ok := o.state.transfers.targets[i.Key()]
	if !ok {
		return transferfact.CarrierInstallation{}, contract.ErrNotFound
	}
	if r.carrierInstallation.Revision != 0 {
		if r.carrierInstallation.Binding != b {
			return transferfact.CarrierInstallation{}, contract.ErrOperationConflict
		}
		return r.carrierInstallation, nil
	}
	next := o.state.clone()
	if err := next.applyTransferEvent(transferencoding.TransferEvent{Kind: transferencoding.TransferCarrierInstalled, At: next.currentCut(), CarrierBinding: b}); err != nil {
		return transferfact.CarrierInstallation{}, err
	}
	if err := o.publishStateLocked(next); err != nil {
		return transferfact.CarrierInstallation{}, err
	}
	return o.state.transfers.targets[i.Key()].carrierInstallation, nil
}

func (s *coreState) applyCarrierInstallation(e transferencoding.TransferEvent) error {
	b, a := e.CarrierBinding, e.CarrierBinding.Acceptance
	reservation := a.Fence.Permit.Reservation
	i := reservation.Input
	if !b.Valid() || e.Input != (transferfact.Intent{}) || e.Operation != "" || e.Actor != (values.PrincipalRef{}) || e.Expected != 0 || e.Manifest != "" || i.Target != s.authority {
		return contract.ErrInvalid
	}
	r, ok := s.transfers.targets[i.Key()]
	if !ok || r.carrier != a || r.installed() {
		return contract.ErrConflict
	}
	if s.cutSerial == math.MaxUint64 {
		return values.PublicResourceExhausted
	}
	accepted := s.transfers.events[a.Revision-1]
	slice, err := transferencoding.DecodeCarrierManifest(a.Fence, accepted.Manifest)
	if err != nil {
		return err
	}
	slice, err = transferencoding.AcceptedEdges(slice, accepted)
	if err != nil {
		return err
	}
	returns, err := s.transferReturns(slice)
	if err != nil {
		return err
	}
	if err := transferfact.PlaceCarrier(slice, s.authority, s.currentCut(), s.elements, s.mount.facts.View, transferencoding.MaxTransferManifestRecords, returns, s.currentCarrierSettlement(), a.Fence.Permit); err != nil {
		return err
	}
	serial, err := transferfact.Install(slice, &s.elements, contract.SpatialTuple{}, s.scope, s.revisionSerial, returns)
	if err != nil {
		return err
	}
	s.revisionSerial, s.cutSerial = serial, s.cutSerial+1
	for _, member := range slice.Members() {
		if err := s.attachments.facts.ElementTransferred(member.ID(), s.currentCut(), member.Facet); err != nil {
			return err
		}
	}
	key := reservation.NextKey
	s.mount.facts.ReplaceIncoming(mountfact.Incoming{
		Key: key, AcceptedAt: a.At, Gate: mountfact.GateOpen,
		Admission: mountfact.Admission{Key: contract.MountAdmissionKey{MountKey: key, OperationID: i.Operation}, RangeValue: reservation.Tuple.Range, DecisionCut: b.At},
		Contact:   s.newContact(key, i.ParentAddress, true),
	})
	index := uint64(len(s.transfers.events)) + 1
	r.carrierInstallation = transferfact.CarrierInstallation{Binding: b, At: s.currentCut(), Revision: index}
	if err := s.installTransferredDelegations(r, slice); err != nil {
		return err
	}
	s.transfers.targets[i.Key()] = r
	s.commits = append(s.commits, coreCommit{kind: transferCommitted, controlIndex: index})
	return validateCurrentSnapshot(s)
}

// Current reads retain only the matching finite receipt, never the journal or
// manifest. Full replay proves the receipt's reachability when opening Store.
func (s *coreState) currentCarrierInstallation() transferfact.CarrierInstallation {
	in := s.mount.facts.Incoming()
	if !in.Key.Valid() || in.Acceptance != (contract.MountPrepareRequest{}) {
		return transferfact.CarrierInstallation{}
	}
	for _, r := range s.transfers.targets {
		if installed := r.carrierInstallation; installed.Revision != 0 && installed.Binding.Acceptance.Fence.Permit.Reservation.NextKey == in.Key {
			return installed
		}
	}
	return transferfact.CarrierInstallation{}
}
