package commit

import (
	"crypto/sha256"
	"math"
	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	mountfact "ucupa/ucupa-core/internal/mount/fact"
	transferencoding "ucupa/ucupa-core/internal/transfer/encoding"
	transferfact "ucupa/ucupa-core/internal/transfer/fact"
)

// FenceTransferSourceLocked is a private construction-stage transaction, not
// a public/peer entry point. The eventual operation must establish the original
// actor's exact source-limited approval. No network call is made under this lock.
func (o *Owner) FenceTransferSourceLocked(actor values.PrincipalRef, id contract.OperationID, expected uint64, parent transferfact.ParentHold) (transferfact.IntentRecord, error) {
	if !actor.Valid() || len(id) == 0 || len(id) > 128 || expected == 0 {
		return transferfact.IntentRecord{}, contract.ErrInvalid
	}
	if err := o.transferPreflightReadyLocked(); err != nil {
		return transferfact.IntentRecord{}, err
	}
	if o.store == nil {
		return transferfact.IntentRecord{}, values.PublicUnavailable
	}
	r, ok := o.state.transfers.records[id]
	if !ok {
		return transferfact.IntentRecord{}, contract.ErrNotFound
	}
	if r.Input.Actor != actor {
		return transferfact.IntentRecord{}, values.PublicDenied
	}
	if r.SourceClosed() {
		if o.state.transfers.events[r.FenceRevision-1].Expected != expected {
			return transferfact.IntentRecord{}, contract.ErrRevisionConflict
		}
		f, raw, err := o.state.sourceFence(r)
		if err != nil {
			return transferfact.IntentRecord{}, err
		}
		slice, err := transferencoding.DecodeTransferManifest(f, raw)
		if err != nil || slice.SourceParent() != parent {
			return transferfact.IntentRecord{}, contract.ErrOperationConflict
		}
		// Recover the original fence result, not the latest control progress.
		r.Progress, r.Revision = transferfact.IntentFenced, r.FenceRevision
		r.DecisionRevision = 0
		return r, nil
	}
	if r.Progress != transferfact.IntentPending || r.Revision != expected {
		return transferfact.IntentRecord{}, contract.ErrRevisionConflict
	}
	_, manifest, err := o.state.captureTransferManifest(r.Input, parent)
	if err != nil {
		return transferfact.IntentRecord{}, err
	}
	e := transferencoding.TransferEvent{Kind: transferencoding.TransferFenced, At: o.state.currentCut(), Operation: id, Actor: actor, Expected: expected, Manifest: manifest}
	next := o.state.clone()
	if err := next.applyTransferEvent(e); err != nil {
		return transferfact.IntentRecord{}, err
	}
	if err := o.publishStateLocked(next); err != nil {
		return transferfact.IntentRecord{}, err
	}
	return o.state.transfers.records[id], nil
}

// Shared live/replay rule: the evidence must exactly match the full selected
// closure at this cut, then the same commit removes its active records. A fence
// does not issue replacement Element Revisions or erase original outcomes.
func (s *coreState) applyTransferFence(e transferencoding.TransferEvent) error {
	if e.Input != (transferfact.Intent{}) || len(e.Manifest) == 0 || len(e.Manifest) > transferencoding.MaxTransferManifestBytes || !e.Actor.Valid() || e.Expected == 0 {
		return contract.ErrInvalid
	}
	r, ok := s.transfers.records[e.Operation]
	if !ok || r.Progress != transferfact.IntentPending || r.Revision != e.Expected || r.Input.Actor != e.Actor {
		return contract.ErrConflict
	}
	if err := s.checkTransferRetainedCapacity(e.Manifest, ""); err != nil {
		return err
	}
	if s.cutSerial == math.MaxUint64 {
		return values.PublicResourceExhausted
	}
	encoded, err := transferencoding.DecodeTransferManifest(transferfact.SourceFence{Input: r.Input, FinalCut: e.At, ManifestDigest: sha256.Sum256([]byte(e.Manifest))}, e.Manifest)
	if err != nil {
		return err
	}
	slice, canonical, err := s.captureTransferManifest(r.Input, encoded.SourceParent())
	if err != nil {
		return err
	}
	if e.Manifest != canonical {
		return contract.ErrConflict
	}
	ancestors := slice.Ancestors()
	root := r.Input.Profile == transferfact.RootSuccessor
	if root && len(ancestors) != 0 || !root && len(ancestors) == 0 && !slice.ExternalParent().Present {
		return contract.ErrStore
	}
	members := slice.Members()
	ids := make([]values.ElementID, len(members))
	var origin contract.SpatialTuple
	for index, member := range members {
		ids[index] = member.ID()
		if member.ID() == r.Input.Boundary && !root {
			if member.Kind == contract.ElementAnchor {
				rangeValue, bounded := member.Anchor.Range.BoundedSphere()
				if !bounded {
					return contract.ErrStore
				}
				origin = contract.SpatialTuple{Parent: member.Anchor.Parent, LocalPose: member.Anchor.LocalPose, Range: rangeValue}
			} else {
				origin = contract.SpatialTuple{Parent: member.Entity.Parent, LocalPose: member.Entity.LocalPose, Range: member.Entity.Range}
			}
		}
	}
	if !root {
		parentID := slice.ExternalParent().ID
		if len(ancestors) != 0 {
			parentID = ancestors[0].ID
		}
		if origin.Parent != parentID {
			return contract.ErrStore
		}
	}
	if root {
		err = s.elements.DepartRoot(ids, r.Input.Key().Controller, e.Operation, r.Input.Boundary)
	} else {
		err = s.elements.Depart(ids, r.Input.Key().Controller, e.Operation)
	}
	if err != nil {
		return err
	}
	for _, id := range ids {
		s.attachments.facts.ElementDeleted(id)
	}
	r.Progress, r.Revision, r.FinalCut = transferfact.IntentFenced, uint64(len(s.transfers.events))+1, e.At
	r.FenceRevision = r.Revision
	r.Origin = origin
	s.transfers.records[e.Operation] = r
	s.cutSerial++
	s.withdrawTransferredMounts(slice)
	s.commits = append(s.commits, coreCommit{kind: transferCommitted, controlIndex: r.Revision})
	return nil
}

// The source withdraws only its own delegation. Its exact former relation
// remains in the original manifest for the external carrier's continuation;
// this does not claim that the remote gate has closed or a new parent is valid.
func (s *coreState) withdrawTransferredMounts(slice transferfact.Slice) {
	for _, member := range slice.Members() {
		if member.Mount.State != contract.MountNone {
			s.mount.facts.ReplaceOutgoing(member.ID(), mountfact.Record{Revision: s.mountRevision(member.ID())})
		}
	}
}
