package access

import (
	"context"

	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
)

// Permit is the single-operation evidence minted by one online authorization
// or recheck. It is consumed only by the local guard at the final read,
// disclosure or commit boundary, only for the operation that minted it, and
// only while the fact lock is held. It is never persisted, registered or
// reused for another operation, page, retry or caller.
type Permit interface {
	// Guard is bounded local work under the fact lock: it confirms the online
	// decision still covers exactly this caller, view and requirement set at
	// the current durable Realm contact generation. It performs no network
	// I/O and never re-enters the authorizer port.
	Guard(context.Context, values.CallerContext, values.AuthorizationView, []values.Requirement, bool) error
}

// DenialPermit is the matching single-operation evidence for one captured
// denial branch. A valid denial proves the branch stayed denied; it never
// proves an Allow.
type DenialPermit interface {
	// DenialGuard is bounded local work under the fact lock for exactly the
	// recheck that minted this permit.
	DenialGuard(context.Context, values.CallerContext, values.AuthorizationDenialView, values.Requirement) error
}

// localPermit keeps the local profile's original behavior: the guard runs the
// same kernel ValidateView at the same guard positions this slice found.
type localPermit struct {
	authorizer authorizer
}

func (p localPermit) Guard(ctx context.Context, caller values.CallerContext, view values.AuthorizationView, requirements []values.Requirement, observe bool) error {
	decision := p.authorizer.ValidateView(ctx, caller, view, requirements...)
	if decision != values.AuthorizationValid {
		return ViewError(decision, observe)
	}
	return nil
}

type localDenialPermit struct {
	authorizer authorizer
}

func (p localDenialPermit) DenialGuard(ctx context.Context, caller values.CallerContext, denial values.AuthorizationDenialView, requirement values.Requirement) error {
	decision := p.authorizer.ValidateDenialView(ctx, caller, denial, requirement)
	if decision != values.AuthorizationValid {
		return ViewError(decision, false)
	}
	return nil
}

// onlinePermit carries the network profile's guard basis: the caller, view,
// requirement set and Realm contact generation captured immediately after the
// online decision settled. Its guard is a pure local comparison; the decision
// itself was made by the just-completed Realm call, and the next operation
// must recheck online again.
type onlinePermit struct {
	contacts     ContactSource
	caller       values.CallerContext
	view         values.AuthorizationView
	requirements []values.Requirement
	contact      contract.RealmAttachment
	release      func(context.Context)
}

// PermitRelease transfers cleanup ownership without exposing the remote
// evidence. The caller invokes it after unlocking, or binds it to the one
// retained receipt that continues to use the original view.
func (a *Admission) PermitRelease(permit Permit) func(context.Context) {
	if p, ok := permit.(*onlinePermit); ok {
		return p.release
	}
	return nil
}

func (a *Admission) ReleasePermit(ctx context.Context, permit Permit) {
	if release := a.PermitRelease(permit); release != nil && ctx != nil {
		release(context.WithoutCancel(ctx))
	}
}

func (p *onlinePermit) Guard(ctx context.Context, caller values.CallerContext, view values.AuthorizationView, requirements []values.Requirement, observe bool) error {
	_ = ctx
	if p == nil || p.caller != caller || p.view != view || !SameRequirements(p.requirements, requirements) {
		return values.PublicInvalidContext
	}
	current, ok := p.contacts.CurrentContact()
	if !ok || current != p.contact {
		return values.PublicUnavailable
	}
	return nil
}

type onlineDenialPermit struct {
	contacts    ContactSource
	caller      values.CallerContext
	denial      values.AuthorizationDenialView
	requirement values.Requirement
	contact     contract.RealmAttachment
	release     func(context.Context)
}

func DenialPermitRelease(permit DenialPermit) func(context.Context) {
	if p, ok := permit.(*onlineDenialPermit); ok {
		return p.release
	}
	return nil
}

func ReleaseDenialPermit(ctx context.Context, permit DenialPermit) {
	if release := DenialPermitRelease(permit); release != nil && ctx != nil {
		release(context.WithoutCancel(ctx))
	}
}

func (p *onlineDenialPermit) DenialGuard(ctx context.Context, caller values.CallerContext, denial values.AuthorizationDenialView, requirement values.Requirement) error {
	_ = ctx
	if p == nil || p.caller != caller || p.denial != denial || p.requirement != requirement {
		return values.PublicInvalidContext
	}
	current, ok := p.contacts.CurrentContact()
	if !ok || current != p.contact {
		return values.PublicUnavailable
	}
	return nil
}

// SameRequirements reports exact set equality without order sensitivity.
func SameRequirements(a, b []values.Requirement) bool {
	if len(a) != len(b) {
		return false
	}
	counts := make(map[values.Requirement]int, len(a))
	for _, requirement := range a {
		counts[requirement]++
	}
	for _, requirement := range b {
		if counts[requirement] == 0 {
			return false
		}
		counts[requirement]--
	}
	return true
}
