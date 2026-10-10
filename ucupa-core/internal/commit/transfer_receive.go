package commit

import (
	"crypto/sha256"
	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	transferencoding "ucupa/ucupa-core/internal/transfer/encoding"
	transferfact "ucupa/ucupa-core/internal/transfer/fact"
)

// TransferSourceFenceLocked discloses immutable source evidence only. The
// operation layer must authorize disclosure to the original actor; these
// construction-stage methods are not exposed through public/peer interfaces.
func (o *Owner) TransferSourceFenceLocked(actor values.PrincipalRef, id contract.OperationID) (transferfact.SourceFence, string, error) {
	r, err := o.TransferIntentLocked(id)
	if err != nil {
		return transferfact.SourceFence{}, "", err
	}
	if !actor.Valid() || actor != r.Input.Actor {
		return transferfact.SourceFence{}, "", values.PublicDenied
	}
	if !r.SourceClosed() {
		return transferfact.SourceFence{}, "", contract.ErrConflict
	}
	return o.state.sourceFence(r)
}

func (s *coreState) sourceFence(r transferfact.IntentRecord) (transferfact.SourceFence, string, error) {
	if !r.SourceClosed() || r.FenceRevision == 0 || r.FenceRevision > uint64(len(s.transfers.events)) {
		return transferfact.SourceFence{}, "", contract.ErrConflict
	}
	e := s.transfers.events[r.FenceRevision-1]
	if e.Kind != transferencoding.TransferFenced || e.Operation != r.Input.Operation || e.At != r.FinalCut {
		return transferfact.SourceFence{}, "", contract.ErrStore
	}
	return transferfact.SourceFence{Input: r.Input, FinalCut: r.FinalCut, ManifestDigest: sha256.Sum256([]byte(e.Manifest))}, e.Manifest, nil
}
