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

// CloseTransferredEdgeLocked is the external carrier's original-owner
// transaction. The calling operation must authenticate the departing parent
// and authorize the original actor before supplying its exact final manifest.
// It preserves all carrier content and closes the old incoming relation.
func (o *Owner) CloseTransferredEdgeLocked(actor values.PrincipalRef, f transferfact.EdgeFence, raw string) (transferfact.EdgeClosure, error) {
	if !f.Valid() {
		return transferfact.EdgeClosure{}, contract.ErrInvalid
	}
	_, _, original := f.Owners()
	if !actor.Valid() || actor != original {
		return transferfact.EdgeClosure{}, values.PublicDenied
	}
	if err := o.transferPreflightReadyLocked(); err != nil {
		return transferfact.EdgeClosure{}, err
	}
	if o.store == nil {
		return transferfact.EdgeClosure{}, values.PublicUnavailable
	}
	if old, exists := o.state.transfers.edges[f.Key()]; exists {
		if old.closure.Fence != f || o.state.transfers.events[old.closure.Revision-1].Manifest != raw {
			return transferfact.EdgeClosure{}, contract.ErrOperationConflict
		}
		return old.closure, nil
	}
	next := o.state.clone()
	e := transferencoding.TransferEvent{Kind: transferencoding.TransferEdgeClosed, At: next.currentCut(), EdgeFence: f, Manifest: raw}
	if err := next.applyTransferEvent(e); err != nil {
		return transferfact.EdgeClosure{}, err
	}
	if err := o.publishStateLocked(next); err != nil {
		return transferfact.EdgeClosure{}, err
	}
	return o.state.transfers.edges[f.Key()].closure, nil
}

func decodeEdgeManifest(f transferfact.EdgeFence, raw string) (transferfact.Slice, error) {
	if !f.Valid() {
		return transferfact.Slice{}, contract.ErrInvalid
	}
	if f.Source != (transferfact.SourceFence{}) {
		return transferencoding.DecodeTransferManifest(f.Source, raw)
	}
	return transferencoding.DecodeCarrierManifest(f.Carrier, raw)
}

func (s *coreState) applyTransferEdgeClosure(e transferencoding.TransferEvent) error {
	if e.Kind != transferencoding.TransferEdgeClosed || e.Input != (transferfact.Intent{}) || e.Actor != (values.PrincipalRef{}) || e.Operation != "" || e.Expected != 0 {
		return contract.ErrInvalid
	}
	f := e.EdgeFence
	if _, exists := s.transfers.edges[f.Key()]; exists {
		return contract.ErrOperationConflict
	}
	if s.transferIntentCount() >= transferencoding.MaxTransferIntents || s.cutSerial == math.MaxUint64 {
		return values.PublicResourceExhausted
	}
	if err := s.checkTransferRetainedCapacity(e.Manifest, ""); err != nil {
		return err
	}
	slice, err := decodeEdgeManifest(f, e.Manifest)
	if err != nil {
		return err
	}
	var member transferfact.Member
	for _, candidate := range slice.Members() {
		if candidate.ID() == f.Mount {
			member = candidate
			break
		}
	}
	rangeValue, err := f.EdgeRange(member)
	if err != nil {
		return err
	}
	in := s.mount.facts.Incoming()
	if in.Gate != mountfact.GateOpen || in.Detached || in.Preparing != (contract.MountAttemptKey{}) ||
		in.Key != member.Mount.Key || in.Admission.Key != member.Mount.Admission {
		return contract.ErrConflict
	}
	oldRange, _ := member.Anchor.Range.BoundedSphere()
	if in.Admission.RangeValue != oldRange || token.Scope(in.Admission.DecisionCut) != token.Scope(slice.Cut()) ||
		token.Serial(in.Admission.DecisionCut) > token.MountRevisionSerial(member.Mount.Revision) {
		return contract.ErrConflict
	}
	// An already preparing outgoing relationship or local transfer may own a
	// dependent promise. Do not freeze it halfway through its own protocol.
	for _, outgoing := range s.mount.facts.Outgoings() {
		if outgoing.Slot != "" {
			return contract.ErrMountBusy
		}
	}
	for _, facet := range s.attachments.facts.Records() {
		if facet.Pending != (values.FacetAcceptanceKey{}) {
			return contract.ErrAttachmentStateConflict
		}
	}
	for _, accepted := range s.transfers.targets {
		if !accepted.complete() {
			return contract.ErrConflict
		}
	}
	for _, r := range s.transfers.records {
		if r.Progress == transferfact.IntentPending || r.SourcePending() {
			return contract.ErrConflict
		}
	}
	inside, err := s.elements.RangeContainsDirectContent(f.Mount, rangeValue)
	if err != nil {
		return err
	}
	if !inside {
		return contract.ErrConflict
	}
	s.cutSerial++
	c := transferfact.EdgeClosure{Fence: f, Previous: in.Admission, Range: rangeValue, At: s.currentCut(), Revision: uint64(len(s.transfers.events)) + 1}
	if !c.Valid(s.authority) {
		return contract.ErrConflict
	}
	in.Gate = mountfact.GateReparenting
	s.mount.facts.ReplaceIncoming(in)
	s.transfers.edges[f.Key()] = edgeRecord{closure: c}
	s.commits = append(s.commits, coreCommit{kind: transferCommitted, controlIndex: c.Revision})
	return validateCurrentSnapshot(s)
}

func (s *coreState) currentEdgeClosure() transferfact.EdgeClosure {
	in := s.mount.facts.Incoming()
	for _, r := range s.transfers.edges {
		c := r.closure
		if c.Previous.Key.MountKey == in.Key {
			return c
		}
	}
	return transferfact.EdgeClosure{}
}
