package transfer

import (
	"context"

	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	"ucupa/ucupa-core/contract/transition"
	"ucupa/ucupa-core/internal/access"
	"ucupa/ucupa-core/internal/transfer/exchange"
	"ucupa/ucupa-core/internal/transfer/fact"
)

// AcceptTransitionImport fetches the actual frozen source through its explicit
// contact. The target user and the source actor have distinct roles: only the
// former supplies target permissions; the latter is retained source provenance.
func (o *Operations) AcceptTransitionImport(ctx context.Context, peer values.FactAuthorityRef, caller values.CallerContext, b contract.TransitionApprovalBinding, view values.AuthorizationView, transport transition.SourceTransport) (transition.Acceptance, error) {
	r, err := o.importRequirements(b)
	if err != nil || ctx == nil || transport == nil || peer != b.Source {
		return transition.Acceptance{}, values.PublicInvalid
	}
	if err := o.admission.Acquire(); err != nil {
		return transition.Acceptance{}, err
	}
	defer o.admission.Release()
	// The online recheck of the retained view runs before any lock is held; the
	// in-lock steps only guard with the evidence this recheck minted.
	o.owner.RLock()
	var check bool
	if !o.owner.HasTransferAcceptanceLocked(b.Source, b.Operation) {
		check = true
	} else {
		err = o.owner.CheckTransferTargetAdmissionLocked()
	}
	o.owner.RUnlock()
	if err != nil {
		return transition.Acceptance{}, err
	}
	var permit access.Permit
	if check {
		permit, err = o.admission.Recheck(ctx, caller, view, r, false)
		if err != nil {
			return transition.Acceptance{}, err
		}
	}
	if ctx.Err() != nil {
		return transition.Acceptance{}, values.PublicUnavailable
	}
	frozen, err := transport.ReadTransitionFence(ctx, b.Command.SourceAddress, b.Source, b)
	if err != nil {
		return transition.Acceptance{}, err
	}
	actual, f, raw := exchange.OpenFrozen(frozen)
	if actual != b || !f.Input.Valid() || f.Input.Source != b.Source || f.Input.Target != o.owner.AuthorityRef() || f.Input.Operation != b.Operation {
		return transition.Acceptance{}, contract.ErrConflict
	}
	o.owner.Lock()
	defer o.owner.Unlock()
	if ctx.Err() != nil {
		return transition.Acceptance{}, values.PublicUnavailable
	}
	old, accepted, err := o.owner.AcceptedTransferLocked(f)
	if err != nil {
		return transition.Acceptance{}, err
	}
	if accepted {
		return exchange.Accept(old), nil
	}
	if err := o.checkImportLocked(ctx, caller, view, r, permit); err != nil {
		return transition.Acceptance{}, err
	}
	if err := o.owner.CheckUnconnectedTransferTargetLocked(); err != nil {
		return transition.Acceptance{}, err
	}
	a, err := o.owner.AcceptTransferTargetLocked(f.Input.Actor, f, raw, nil, fact.ParentHold{})
	if err != nil {
		return transition.Acceptance{}, err
	}
	return exchange.Accept(a), nil
}

// Installation consumes a durable promise plus a freshly authenticated source
// decision. Approval expiry or grant revocation cannot erase that promise.
func (o *Operations) InstallTransition(ctx context.Context, peer values.FactAuthorityRef, b contract.TransitionApprovalBinding, transport transition.SourceTransport) (transition.Completion, error) {
	if _, err := o.importRequirements(b); err != nil || ctx == nil || transport == nil || peer != b.Source {
		return transition.Completion{}, values.PublicInvalid
	}
	if err := o.admission.Acquire(); err != nil {
		return transition.Completion{}, err
	}
	defer o.admission.Release()
	o.owner.RLock()
	err := o.owner.CheckTransferTargetAdmissionLocked()
	accepted := o.owner.HasTransferAcceptanceLocked(b.Source, b.Operation)
	o.owner.RUnlock()
	if err != nil {
		return transition.Completion{}, err
	}
	if !accepted {
		return transition.Completion{}, contract.ErrConflict
	}
	if ctx.Err() != nil {
		return transition.Completion{}, values.PublicUnavailable
	}
	decision, err := transport.ReadTransitionDecision(ctx, b.Command.SourceAddress, b.Source, b)
	if err != nil {
		return transition.Completion{}, err
	}
	actual, d := exchange.OpenDecision(decision)
	if actual != b || !d.Acceptance.Fence.Input.Valid() || d.Acceptance.Fence.Input.Source != b.Source || d.Acceptance.Fence.Input.Target != o.owner.AuthorityRef() || d.Acceptance.Fence.Input.Operation != b.Operation {
		return transition.Completion{}, contract.ErrConflict
	}
	o.owner.Lock()
	defer o.owner.Unlock()
	if ctx.Err() != nil {
		return transition.Completion{}, values.PublicUnavailable
	}
	i, err := o.owner.InstallTransferTargetLocked(d)
	if err != nil {
		return transition.Completion{}, err
	}
	c, err := o.owner.CompleteTransferTargetLocked(fact.CoreInstallation{Source: i}, nil)
	if err != nil {
		return transition.Completion{}, err
	}
	return exchange.Complete(c), nil
}
