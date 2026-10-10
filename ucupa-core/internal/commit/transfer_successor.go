package commit

import (
	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	"ucupa/ucupa-core/internal/token"
	transferencoding "ucupa/ucupa-core/internal/transfer/encoding"
	transferfact "ucupa/ucupa-core/internal/transfer/fact"
)

// CommitTransferSourceLocked is the owner-local slice's irreversible control
// decision. The operation layer must revalidate the original actor's approval
// and obtain acceptance from the designated, authenticated target before this
// private transaction. No external call occurs under the owner lock.
func (o *Owner) CommitTransferSourceLocked(actor values.PrincipalRef, acceptance transferfact.TargetAcceptance, expected uint64) (transferfact.SourceDecision, error) {
	if !actor.Valid() || !acceptance.Fence.Input.Valid() || expected == 0 {
		return transferfact.SourceDecision{}, contract.ErrInvalid
	}
	if err := o.transferPreflightReadyLocked(); err != nil {
		return transferfact.SourceDecision{}, err
	}
	if o.store == nil {
		return transferfact.SourceDecision{}, values.PublicUnavailable
	}
	id := acceptance.Fence.Input.Operation
	r, ok := o.state.transfers.records[id]
	if !ok {
		return transferfact.SourceDecision{}, contract.ErrNotFound
	}
	if actor != r.Input.Actor {
		return transferfact.SourceDecision{}, values.PublicDenied
	}
	if r.Progress == transferfact.IntentCommitted || r.Progress == transferfact.IntentSettled {
		d, err := o.state.sourceDecision(r)
		if err != nil {
			return transferfact.SourceDecision{}, err
		}
		e := o.state.transfers.events[r.DecisionRevision-1]
		if e.Acceptance != acceptance {
			return transferfact.SourceDecision{}, contract.ErrOperationConflict
		}
		if e.Expected != expected {
			return transferfact.SourceDecision{}, contract.ErrRevisionConflict
		}
		return d, nil
	}
	if r.Progress != transferfact.IntentFenced || r.Revision != expected {
		return transferfact.SourceDecision{}, contract.ErrRevisionConflict
	}
	next := o.state.clone()
	e := transferencoding.TransferEvent{Kind: transferencoding.TransferSuccessorDecided, At: next.currentCut(), Operation: id, Actor: actor, Expected: expected, Acceptance: acceptance}
	if err := next.applyTransferEvent(e); err != nil {
		return transferfact.SourceDecision{}, err
	}
	if err := o.publishStateLocked(next); err != nil {
		return transferfact.SourceDecision{}, err
	}
	return transferDecision(e, o.state.transfers.records[id].Revision), nil
}

func transferDecision(e transferencoding.TransferEvent, revision uint64) transferfact.SourceDecision {
	return transferfact.SourceDecision{Acceptance: e.Acceptance, At: e.At, Revision: revision}
}

// Source successor decisions change only control history. Source departure,
// read gaps, parent holds and original CRUD outcomes remain exactly as fenced.
// The shared replay rule binds the acceptance to the original immutable fence.
func (s *coreState) applyTransferSuccessor(e transferencoding.TransferEvent) error {
	if e.Input != (transferfact.Intent{}) || e.Manifest != "" || !e.Actor.Valid() || e.Expected == 0 {
		return contract.ErrInvalid
	}
	r, ok := s.transfers.records[e.Operation]
	if !ok || r.Progress != transferfact.IntentFenced || r.Revision != e.Expected || r.Input.Actor != e.Actor {
		return contract.ErrConflict
	}
	f, _, err := s.sourceFence(r)
	if err != nil {
		return err
	}
	a := e.Acceptance
	if a.Fence != f || a.Revision == 0 || !token.Valid(a.At) || token.Scope(a.At) == s.scope {
		return contract.ErrConflict
	}
	r.Progress, r.Revision = transferfact.IntentCommitted, uint64(len(s.transfers.events))+1
	r.DecisionRevision = r.Revision
	s.transfers.records[e.Operation] = r
	return nil
}

// TransferSourceDecisionLocked is an exact, authorized-disclosure building
// block, not an enumerable task index or an installation permit for callers.
func (o *Owner) TransferSourceDecisionLocked(actor values.PrincipalRef, id contract.OperationID) (transferfact.SourceDecision, error) {
	r, err := o.TransferIntentLocked(id)
	if err != nil {
		return transferfact.SourceDecision{}, err
	}
	if !actor.Valid() || actor != r.Input.Actor {
		return transferfact.SourceDecision{}, values.PublicDenied
	}
	return o.state.sourceDecision(r)
}

func (s *coreState) sourceDecision(r transferfact.IntentRecord) (transferfact.SourceDecision, error) {
	if (r.Progress != transferfact.IntentCommitted && r.Progress != transferfact.IntentSettled) || r.DecisionRevision == 0 || r.DecisionRevision > uint64(len(s.transfers.events)) {
		return transferfact.SourceDecision{}, contract.ErrConflict
	}
	e := s.transfers.events[r.DecisionRevision-1]
	if e.Kind != transferencoding.TransferSuccessorDecided || e.Operation != r.Input.Operation {
		return transferfact.SourceDecision{}, contract.ErrStore
	}
	return transferDecision(e, r.DecisionRevision), nil
}
