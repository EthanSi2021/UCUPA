package commit

import (
	"crypto/sha256"
	"math"
	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	elementfact "ucupa/ucupa-core/internal/element/fact"
	mountfact "ucupa/ucupa-core/internal/mount/fact"
	transferencoding "ucupa/ucupa-core/internal/transfer/encoding"
	transferfact "ucupa/ucupa-core/internal/transfer/fact"
)

// The authenticated operation must verify the parent's exact permit and the
// original caller's source approval before entering this private transaction.
// Refusal and fence share the original lock, qualified record and Store commit.
func (o *Owner) FenceCarrierSourceLocked(actor values.PrincipalRef, c transferfact.CarrierControl) (transferfact.CarrierFence, string, error) {
	if _, err := transferencoding.EncodeCarrierPermit(c); err != nil {
		return transferfact.CarrierFence{}, "", err
	}
	i := c.Reservation.Input
	if !actor.Valid() || actor != i.Actor {
		return transferfact.CarrierFence{}, "", values.PublicDenied
	}
	if err := o.transferPreflightReadyLocked(); err != nil {
		return transferfact.CarrierFence{}, "", err
	}
	if o.store == nil {
		return transferfact.CarrierFence{}, "", values.PublicUnavailable
	}
	if i.Source != o.authority {
		return transferfact.CarrierFence{}, "", contract.ErrConflict
	}
	if old, exists := o.state.transfers.carrierSources[i.Key()]; exists {
		if old.Permit != c {
			return transferfact.CarrierFence{}, "", contract.ErrOperationConflict
		}
		return o.state.carrierSourceFence(old)
	}
	_, raw, err := o.state.captureCarrierManifest(c)
	if err != nil {
		return transferfact.CarrierFence{}, "", err
	}
	next := o.state.clone()
	if err := next.applyTransferEvent(transferencoding.TransferEvent{Kind: transferencoding.TransferCarrierFenced, At: next.currentCut(), CarrierPermit: c, Manifest: raw}); err != nil {
		return transferfact.CarrierFence{}, "", err
	}
	if err := o.publishStateLocked(next); err != nil {
		return transferfact.CarrierFence{}, "", err
	}
	return o.state.carrierSourceFence(o.state.transfers.carrierSources[i.Key()])
}

func (o *Owner) CarrierSourceFenceLocked(actor values.PrincipalRef, key transferfact.TransferKey) (transferfact.CarrierFence, string, error) {
	if !actor.Valid() || !key.Valid() {
		return transferfact.CarrierFence{}, "", contract.ErrInvalid
	}
	if err := o.transferPreflightReadyLocked(); err != nil {
		return transferfact.CarrierFence{}, "", err
	}
	r, exists := o.state.transfers.carrierSources[key]
	if !exists {
		return transferfact.CarrierFence{}, "", contract.ErrNotFound
	}
	if r.Permit.Reservation.Input.Actor != actor {
		return transferfact.CarrierFence{}, "", values.PublicDenied
	}
	return o.state.carrierSourceFence(r)
}

func (s *coreState) carrierSourceFence(r carrierSourceRecord) (transferfact.CarrierFence, string, error) {
	f := r.Fence
	if !f.Valid() {
		return transferfact.CarrierFence{}, "", values.PublicUnavailable
	}
	if r.Refusal != (transferfact.CarrierRefusal{}) || f.Permit != r.Permit || f.Revision > uint64(len(s.transfers.events)) {
		return transferfact.CarrierFence{}, "", contract.ErrStore
	}
	e := s.transfers.events[f.Revision-1]
	if e.Kind != transferencoding.TransferCarrierFenced || e.CarrierPermit != f.Permit || e.At != f.FinalCut || sha256.Sum256([]byte(e.Manifest)) != f.ManifestDigest {
		return transferfact.CarrierFence{}, "", contract.ErrStore
	}
	return f, e.Manifest, nil
}

func (s *coreState) captureCarrierManifest(c transferfact.CarrierControl) (transferfact.Slice, string, error) {
	if s.transferHoldsIncoming() {
		return transferfact.Slice{}, "", contract.ErrConflict
	}
	r, in := c.Reservation, s.mount.facts.Incoming()
	if r.Input.Source != s.authority || in.Key != r.Previous.MountKey || in.Admission.Key != r.Previous || in.Admission.RangeValue != r.Tuple.Range ||
		in.Contact.NextHop != r.Input.ParentAddress || in.Detached || in.Gate != mountfact.GateOpen || in.Preparing != (contract.MountAttemptKey{}) {
		return transferfact.Slice{}, "", contract.ErrConflict
	}
	external := elementfact.ExternalParent{ID: r.Input.Mount, Range: r.Tuple.Range, Present: true, Admitted: true}
	slice, err := transferfact.SelectCarrier(s.authority, s.currentCut(), s.revisionSerial, s.elements.View, s.attachments.facts.View,
		s.mount.facts.View, external, transferencoding.MaxTransferManifestRecords)
	if err != nil {
		return transferfact.Slice{}, "", err
	}
	slice, err = s.captureTransferEvidence(slice)
	if err != nil {
		return transferfact.Slice{}, "", err
	}
	raw, err := transferencoding.EncodeCarrierManifest(c, slice)
	return slice, raw, err
}

func (s *coreState) applyCarrierFence(e transferencoding.TransferEvent) error {
	c := e.CarrierPermit
	if e.Kind != transferencoding.TransferCarrierFenced || e.Input != (transferfact.Intent{}) || e.Operation != "" || e.Actor != (values.PrincipalRef{}) || e.Expected != 0 {
		return contract.ErrInvalid
	}
	if _, err := transferencoding.EncodeCarrierPermit(c); err != nil {
		return err
	}
	i := c.Reservation.Input
	if _, exists := s.transfers.carrierSources[i.Key()]; exists {
		return contract.ErrOperationConflict
	}
	if s.transferIntentCount() >= transferencoding.MaxTransferIntents || s.cutSerial == math.MaxUint64 {
		return values.PublicResourceExhausted
	}
	if err := s.checkTransferRetainedCapacity(e.Manifest, ""); err != nil {
		return err
	}
	slice, canonical, err := s.captureCarrierManifest(c)
	if err != nil {
		return err
	}
	if e.Manifest != canonical {
		return contract.ErrConflict
	}
	ids := make([]values.ElementID, 0, len(slice.Members()))
	for _, member := range slice.Members() {
		ids = append(ids, member.ID())
	}
	// Empty is legitimate; no phantom parent or departure is minted for it.
	if len(ids) != 0 {
		if err := s.elements.Depart(ids, i.Parent, i.Operation); err != nil {
			return err
		}
	}
	for _, id := range ids {
		s.attachments.facts.ElementDeleted(id)
	}
	f := transferfact.CarrierFence{Permit: c, FinalCut: e.At, ManifestDigest: sha256.Sum256([]byte(canonical)), Revision: uint64(len(s.transfers.events)) + 1}
	if !f.Valid() {
		return contract.ErrConflict
	}
	s.transfers.carrierSources[i.Key()] = carrierSourceRecord{Permit: c, Fence: f}
	in := s.mount.facts.Incoming()
	in.Gate = mountfact.GateFenced
	s.mount.facts.ReplaceIncoming(in)
	s.cutSerial++
	s.withdrawTransferredMounts(slice)
	s.commits = append(s.commits, coreCommit{kind: transferCommitted, controlIndex: f.Revision})
	return validateCurrentSnapshot(s)
}

// At most the current incoming relation's finite proof enters a read view.
// No manifest or history journal is copied or decoded by ordinary reads.
func (s *coreState) currentCarrierFence() transferfact.CarrierFence {
	in := s.mount.facts.Incoming()
	for _, r := range s.transfers.carrierSources {
		if r.Fence.Valid() && r.Permit.Reservation.Previous.MountKey == in.Key {
			return r.Fence
		}
	}
	return transferfact.CarrierFence{}
}
