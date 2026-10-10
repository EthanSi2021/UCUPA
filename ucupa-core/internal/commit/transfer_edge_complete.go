package commit

import (
	"math"
	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	transferencoding "ucupa/ucupa-core/internal/transfer/encoding"
	transferfact "ucupa/ucupa-core/internal/transfer/fact"
)

// CompleteTransferTargetLocked consumes exact results authenticated from every
// retained carrier. Until this transaction, ordinary writes cannot disturb the
// installed placement and queries cannot report the transferred closure ready.
func (o *Owner) CompleteTransferTargetLocked(i transferfact.CoreInstallation, results []transferfact.EdgeInstallation) (transferfact.CoreCompletion, error) {
	if !i.Valid() {
		return transferfact.CoreCompletion{}, contract.ErrInvalid
	}
	if err := o.transferPreflightReadyLocked(); err != nil {
		return transferfact.CoreCompletion{}, err
	}
	if o.store == nil {
		return transferfact.CoreCompletion{}, values.PublicUnavailable
	}
	r, ok := o.state.transfers.targets[i.Key()]
	if !ok || r.installedProof() != i {
		return transferfact.CoreCompletion{}, contract.ErrConflict
	}
	raw, err := transferencoding.EncodeEdgeResults(r.closures, r.installedProof(), results)
	if err != nil {
		return transferfact.CoreCompletion{}, err
	}
	if len(r.closures) == 0 {
		return transferfact.CoreCompletion{Installed: i, At: i.Cut(), Revision: i.Revision()}, nil
	}
	if r.completion.Revision != 0 {
		if o.state.transfers.events[r.completion.Revision-1].EdgeResults != raw {
			return transferfact.CoreCompletion{}, contract.ErrOperationConflict
		}
		return r.completion, nil
	}
	next := o.state.clone()
	e := transferencoding.TransferEvent{Kind: transferencoding.TransferTargetCompleted, At: next.currentCut(), Expected: i.Revision(), EdgeResults: raw}
	if err := next.applyTransferEvent(e); err != nil {
		return transferfact.CoreCompletion{}, err
	}
	if err := o.publishStateLocked(next); err != nil {
		return transferfact.CoreCompletion{}, err
	}
	return o.state.transfers.targets[i.Key()].completion, nil
}

func (s *coreState) applyTargetCompletion(e transferencoding.TransferEvent) error {
	if e.Expected == 0 || e.Input != (transferfact.Intent{}) || e.Operation != "" || e.Actor != (values.PrincipalRef{}) || e.Manifest != "" {
		return contract.ErrInvalid
	}
	for key, r := range s.transfers.targets {
		if !r.installed() || r.installedProof().Revision() != e.Expected {
			continue
		}
		if len(r.closures) == 0 || r.completion.Revision != 0 {
			return contract.ErrConflict
		}
		if err := transferencoding.ValidateEdgeResults(r.closures, r.installedProof(), e.EdgeResults); err != nil {
			return err
		}
		if s.cutSerial == math.MaxUint64 {
			return values.PublicResourceExhausted
		}
		s.cutSerial++
		r.completion = transferfact.CoreCompletion{Installed: r.installedProof(), At: s.currentCut(), Revision: uint64(len(s.transfers.events)) + 1}
		if !r.completion.Valid() {
			return contract.ErrConflict
		}
		s.transfers.targets[key] = r
		if err := s.checkTransferRetainedBytes(len(e.EdgeResults)); err != nil {
			return err
		}
		s.commits = append(s.commits, coreCommit{kind: transferCommitted, controlIndex: r.completion.Revision})
		return validateCurrentSnapshot(s)
	}
	return contract.ErrNotFound
}
