// Package access owns the Core's shared public request admission and authorization
// continuity. It does not own facts, observation contexts or commit lifecycle.
package access

import (
	"context"
	"sync/atomic"

	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
)

// authorizer is the trusted consumer port. Authorization and online rechecks
// run outside any fact lock; under a network Realm contact they are actual
// Realm calls, so every method carries the original request's context and
// must honor its cancellation and deadline. The in-lock guard consumes only
// the minted Permit and never calls back into this port.
type authorizer interface {
	AuthorizeView(context.Context, values.CallerContext, ...values.Requirement) (values.AuthorizationDecision, values.AuthorizationView)
	ValidateView(context.Context, values.CallerContext, values.AuthorizationView, ...values.Requirement) values.AuthorizationDecision
	CaptureDenialView(context.Context, values.CallerContext, values.Requirement) (values.AuthorizationDecision, values.AuthorizationDenialView)
	ValidateDenialView(context.Context, values.CallerContext, values.AuthorizationDenialView, values.Requirement) values.AuthorizationDecision
}

// ContactSource reports the owner's current durable Realm contact through the
// same lock-free snapshot the trusted authorizer adapter captures.
type ContactSource interface {
	CurrentContact() (contract.RealmAttachment, bool)
}

// Admission is assembled once per Core run and shared by every public operation.
// The active counter is not a second fact lock and does not change shutdown into
// a drain operation. Only successful acquisitions must be released.
type Admission struct {
	authorizer contract.Authorizer
	contacts   ContactSource
	limit      uint64
	active     atomic.Uint64
}

func New(authorizer contract.Authorizer, contacts ContactSource, limits values.PublicLimits) *Admission {
	if limits.ContextTTL == 0 {
		return nil
	}
	return &Admission{authorizer: authorizer, contacts: contacts, limit: limits.ConcurrentReads}
}

func (a *Admission) Enabled() bool { return a != nil }

func (a *Admission) Acquire() error {
	if a == nil {
		return values.PublicUnavailable
	}
	if a.active.Add(1) > a.limit {
		a.active.Add(^uint64(0))
		return values.PublicResourceExhausted
	}
	return nil
}

func (a *Admission) Release() { a.active.Add(^uint64(0)) }

// Authorize resolves one typed requirement set outside any fact lock. Under a
// network Realm contact this is the online authorization call, and it honors
// the original request's context. The returned Permit is this one
// operation's evidence for the local guard; it fails closed when the durable
// contact moved while the decision was in flight.
func (a *Admission) Authorize(ctx context.Context, caller values.CallerContext, requirements []values.Requirement) (values.AuthorizationView, Permit, error) {
	if a == nil {
		return values.AuthorizationView{}, nil, values.PublicUnavailable
	}
	authorizer, ok := a.authorizer.(authorizer)
	if !ok {
		return values.AuthorizationView{}, nil, values.PublicUnavailable
	}
	before, ok := a.contacts.CurrentContact()
	if !ok {
		return values.AuthorizationView{}, nil, values.PublicUnavailable
	}
	decision, view := authorizer.AuthorizeView(ctx, caller, requirements...)
	if decision != values.AuthorizationValid {
		return values.AuthorizationView{}, nil, ViewError(decision, false)
	}
	permit := a.mintPermit(caller, view, requirements, before)
	if permit == nil {
		if release := a.evidenceRelease(before, [32]byte(view)); release != nil && ctx != nil {
			release(context.WithoutCancel(ctx))
		}
		return values.AuthorizationView{}, nil, values.PublicUnavailable
	}
	return view, permit, nil
}

// Recheck revalidates one previously issued view online, outside any fact
// lock, before a continuation, observation or original-outcome disclosure
// consumes it. It never upgrades the view into a fresh authorization; the
// fresh Permit carries exactly this recheck to the local guard.
func (a *Admission) Recheck(ctx context.Context, caller values.CallerContext, view values.AuthorizationView, requirements []values.Requirement, observe bool) (Permit, error) {
	if a == nil {
		return nil, values.PublicUnavailable
	}
	authorizer, ok := a.authorizer.(authorizer)
	if !ok {
		return nil, values.PublicUnavailable
	}
	before, ok := a.contacts.CurrentContact()
	if !ok {
		return nil, values.PublicUnavailable
	}
	decision := authorizer.ValidateView(ctx, caller, view, requirements...)
	if decision != values.AuthorizationValid {
		return nil, ViewError(decision, observe)
	}
	permit := a.mintPermit(caller, view, requirements, before)
	if permit == nil {
		return nil, values.PublicUnavailable
	}
	return permit, nil
}

// Validate is the bounded local guard at the final read, disclosure or commit
// boundary, under the fact lock. It consumes exactly the Permit minted for
// this operation and fails closed without one. It performs no network I/O.
// A request whose context is already canceled or expired never reaches this
// boundary, even with a valid Permit already minted: authorization that
// settled after the request died must not start a new commit.
func (a *Admission) Validate(ctx context.Context, caller values.CallerContext, view values.AuthorizationView, requirements []values.Requirement, observe bool, permit Permit) error {
	if a == nil {
		return values.PublicUnavailable
	}
	if permit == nil {
		return values.PublicUnavailable
	}
	if ctx == nil || ctx.Err() != nil {
		return values.PublicUnavailable
	}
	return permit.Guard(ctx, caller, view, requirements, observe)
}

// CaptureDenial proves one closed branch online, outside any fact lock. Only
// a real denial with a kernel view yields evidence; a valid authorization is
// still reported as valid and grants nothing.
func (a *Admission) CaptureDenial(ctx context.Context, caller values.CallerContext, requirement values.Requirement) (values.AuthorizationDecision, values.AuthorizationDenialView, DenialPermit, error) {
	if a == nil {
		return 0, values.AuthorizationDenialView{}, nil, values.PublicUnavailable
	}
	authorizer, ok := a.authorizer.(authorizer)
	if !ok {
		return 0, values.AuthorizationDenialView{}, nil, values.PublicUnavailable
	}
	before, ok := a.contacts.CurrentContact()
	if !ok {
		return 0, values.AuthorizationDenialView{}, nil, values.PublicUnavailable
	}
	decision, denial := authorizer.CaptureDenialView(ctx, caller, requirement)
	if decision == values.AuthorizationValid {
		return decision, values.AuthorizationDenialView{}, nil, nil
	}
	if decision == values.AuthorizationUnavailable {
		return decision, values.AuthorizationDenialView{}, nil, values.PublicUnavailable
	}
	if decision != values.AuthorizationDenied || denial == (values.AuthorizationDenialView{}) {
		return decision, values.AuthorizationDenialView{}, nil, values.PublicDenied
	}
	permit := a.mintDenialPermit(caller, denial, requirement, before)
	if permit == nil {
		if release := a.evidenceRelease(before, [32]byte(denial)); release != nil && ctx != nil {
			release(context.WithoutCancel(ctx))
		}
		return decision, values.AuthorizationDenialView{}, nil, values.PublicUnavailable
	}
	return decision, denial, permit, nil
}

// RecheckDenial revalidates one captured denial online, outside any fact
// lock. A valid denial proves the branch stayed denied; it never proves an
// Allow, and an authorization that appeared meanwhile is reported as changed.
func (a *Admission) RecheckDenial(ctx context.Context, caller values.CallerContext, denial values.AuthorizationDenialView, requirement values.Requirement) (DenialPermit, error) {
	if a == nil {
		return nil, values.PublicUnavailable
	}
	authorizer, ok := a.authorizer.(authorizer)
	if !ok {
		return nil, values.PublicUnavailable
	}
	before, ok := a.contacts.CurrentContact()
	if !ok {
		return nil, values.PublicUnavailable
	}
	decision := authorizer.ValidateDenialView(ctx, caller, denial, requirement)
	if decision != values.AuthorizationValid {
		return nil, ViewError(decision, false)
	}
	permit := a.mintDenialPermit(caller, denial, requirement, before)
	if permit == nil {
		return nil, values.PublicUnavailable
	}
	return permit, nil
}

// ValidateDenial is the bounded local guard for one denial recheck, under the
// fact lock. It consumes exactly the DenialPermit minted for this operation.
// A request whose context is already canceled or expired never reaches this
// boundary.
func (a *Admission) ValidateDenial(ctx context.Context, caller values.CallerContext, denial values.AuthorizationDenialView, requirement values.Requirement, permit DenialPermit) error {
	if a == nil {
		return values.PublicUnavailable
	}
	if permit == nil {
		return values.PublicUnavailable
	}
	if ctx == nil || ctx.Err() != nil {
		return values.PublicUnavailable
	}
	return permit.DenialGuard(ctx, caller, denial, requirement)
}

// mintPermit binds one permit to the contact generation that was already
// current before the online call and is still current now: a decision from a
// superseded contact is unavailable evidence. The local profile keeps the
// kernel-backed guard; the network profile compares generations only.
func (a *Admission) mintPermit(caller values.CallerContext, view values.AuthorizationView, requirements []values.Requirement, before contract.RealmAttachment) Permit {
	current, ok := a.contacts.CurrentContact()
	if !ok || current != before {
		return nil
	}
	if current.NextHop.IsLocal() {
		authorizer, ok := a.authorizer.(authorizer)
		if !ok {
			return nil
		}
		return localPermit{authorizer: authorizer}
	}
	return &onlinePermit{contacts: a.contacts, caller: caller, view: view,
		requirements: append([]values.Requirement(nil), requirements...), contact: current,
		release: a.evidenceRelease(before, [32]byte(view))}
}

func (a *Admission) mintDenialPermit(caller values.CallerContext, denial values.AuthorizationDenialView, requirement values.Requirement, before contract.RealmAttachment) DenialPermit {
	current, ok := a.contacts.CurrentContact()
	if !ok || current != before {
		return nil
	}
	if current.NextHop.IsLocal() {
		authorizer, ok := a.authorizer.(authorizer)
		if !ok {
			return nil
		}
		return localDenialPermit{authorizer: authorizer}
	}
	return &onlineDenialPermit{contacts: a.contacts, caller: caller, denial: denial, requirement: requirement, contact: current,
		release: a.evidenceRelease(before, [32]byte(denial))}
}

func (a *Admission) evidenceRelease(contact contract.RealmAttachment, evidence [32]byte) func(context.Context) {
	if contact.NextHop.IsLocal() || evidence == ([32]byte{}) {
		return nil
	}
	releaser, ok := a.authorizer.(interface {
		ReleaseRealmEvidence(context.Context, contract.RealmAttachment, [32]byte)
	})
	if !ok {
		return nil
	}
	return func(ctx context.Context) { releaser.ReleaseRealmEvidence(ctx, contact, evidence) }
}

func ViewError(decision values.AuthorizationDecision, observe bool) error {
	switch decision {
	case values.AuthorizationDenied:
		return values.PublicDenied
	case values.AuthorizationChanged:
		if observe {
			return values.PublicGap
		}
		return values.PublicInvalidContext
	default:
		return values.PublicUnavailable
	}
}

// The context key is private and identifies only the one already-acquired
// local Admission. It is not caller evidence and never crosses the wire.
type admissionKey struct{}

// WithAdmission is used by the public envelope after a successful Acquire.
// Source calls made inside that envelope borrow its one existing slot.
func WithAdmission(ctx context.Context, a *Admission) context.Context {
	return context.WithValue(ctx, admissionKey{}, a)
}

func HasAdmission(ctx context.Context, a *Admission) bool {
	return a != nil && ctx.Value(admissionKey{}) == a
}

func ClosureRequirements(authority values.FactAuthorityRef, boundary values.ElementID, permissions ...values.PermissionID) []values.Requirement {
	result := make([]values.Requirement, len(permissions))
	for i := range permissions {
		result[i] = values.Requirement{Permission: permissions[i], Scope: values.AnchorClosureScope(authority, boundary)}
	}
	return result
}
