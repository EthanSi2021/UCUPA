package instance

import (
	"context"

	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	commitpkg "ucupa/ucupa-core/internal/commit"
)

// contactAuthorizer adapts the owner's current durable Realm contact to the
// Core's trusted authorizer port. Each call reads the lock-free contact
// snapshot, uses only that persisted address and Realm, and rechecks that the
// same contact generation is still current before the decision can be used.
// It never retries against another address, caches a decision, or turns an
// unavailable Realm into a proven denial.
type contactAuthorizer struct {
	owner *commitpkg.Owner
	port  contract.RealmConnections
}

var _ contract.Authorizer = (*contactAuthorizer)(nil)

// The captured contact belongs to the evidence being retired, not a new
// authorization source. No address discovery or current-contact substitution
// is performed for cleanup after an address update.
func (adapter *contactAuthorizer) ReleaseRealmEvidence(ctx context.Context, contact contract.RealmAttachment, evidence [32]byte) {
	if releaser, ok := adapter.port.(contract.RealmEvidenceReleaser); ok {
		releaser.ReleaseRealmEvidence(ctx, contact.NextHop, contact.Realm, evidence)
	}
}

func (adapter *contactAuthorizer) snapshot() (contract.RealmAttachment, bool) {
	if adapter == nil || adapter.owner == nil || adapter.port == nil {
		return contract.RealmAttachment{}, false
	}
	return adapter.owner.CurrentContact()
}

// settle keeps one authorization bound to the contact generation it started
// with: a decision from a superseded contact is unavailable evidence.
func (adapter *contactAuthorizer) settle(contact contract.RealmAttachment, decision values.AuthorizationDecision) values.AuthorizationDecision {
	current, ok := adapter.snapshot()
	if !ok || current != contact {
		return values.AuthorizationUnavailable
	}
	return decision
}

func (adapter *contactAuthorizer) AuthorizePrincipal(actor values.PrincipalRef, requirements ...values.Requirement) values.AuthorizationDecision {
	contact, ok := adapter.snapshot()
	if !ok {
		return values.AuthorizationUnavailable
	}
	return adapter.settle(contact, adapter.port.AuthorizePrincipal(contact.NextHop, contact.Realm, actor, requirements...))
}

func (adapter *contactAuthorizer) AuthorizeView(ctx context.Context, caller values.CallerContext, requirements ...values.Requirement) (values.AuthorizationDecision, values.AuthorizationView) {
	contact, ok := adapter.snapshot()
	if !ok {
		return values.AuthorizationUnavailable, values.AuthorizationView{}
	}
	decision, view := adapter.port.AuthorizeView(ctx, contact.NextHop, contact.Realm, caller, requirements...)
	if adapter.settle(contact, decision) != decision {
		if ctx != nil {
			adapter.ReleaseRealmEvidence(context.WithoutCancel(ctx), contact, [32]byte(view))
		}
		return values.AuthorizationUnavailable, values.AuthorizationView{}
	}
	return decision, view
}

func (adapter *contactAuthorizer) ValidateView(ctx context.Context, caller values.CallerContext, view values.AuthorizationView, requirements ...values.Requirement) values.AuthorizationDecision {
	contact, ok := adapter.snapshot()
	if !ok {
		return values.AuthorizationUnavailable
	}
	return adapter.settle(contact, adapter.port.ValidateView(ctx, contact.NextHop, contact.Realm, caller, view, requirements...))
}

func (adapter *contactAuthorizer) CaptureDenialView(ctx context.Context, caller values.CallerContext, requirement values.Requirement) (values.AuthorizationDecision, values.AuthorizationDenialView) {
	contact, ok := adapter.snapshot()
	if !ok {
		return values.AuthorizationUnavailable, values.AuthorizationDenialView{}
	}
	decision, view := adapter.port.CaptureDenialView(ctx, contact.NextHop, contact.Realm, caller, requirement)
	if adapter.settle(contact, decision) != decision {
		if ctx != nil {
			adapter.ReleaseRealmEvidence(context.WithoutCancel(ctx), contact, [32]byte(view))
		}
		return values.AuthorizationUnavailable, values.AuthorizationDenialView{}
	}
	return decision, view
}

func (adapter *contactAuthorizer) ValidateDenialView(ctx context.Context, caller values.CallerContext, view values.AuthorizationDenialView, requirement values.Requirement) values.AuthorizationDecision {
	contact, ok := adapter.snapshot()
	if !ok {
		return values.AuthorizationUnavailable
	}
	return adapter.settle(contact, adapter.port.ValidateDenialView(ctx, contact.NextHop, contact.Realm, caller, view, requirement))
}
