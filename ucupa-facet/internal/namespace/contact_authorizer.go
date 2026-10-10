package namespace

import (
	"context"

	"ucupa/shared/values"
	"ucupa/ucupa-facet/contract"
)

// contactAuthorizer adapts the owner's current durable Realm contact to the
// Facet's trusted authorizer port. Each call reads the lock-free contact
// snapshot, uses only that persisted address and Realm, and rechecks that the
// same contact generation is still current before the decision can be used.
// It never retries against another address, caches a decision, or turns an
// unavailable Realm into a proven denial. The adapter stays lock-free and
// non-reentrant because the snapshot is atomic; the context-bearing view
// methods may wait on the network under the pinned contact, so callers must
// not hold the Facet lock across them.
type contactAuthorizer struct {
	owner *ownerState
}

var _ contract.Authorizer = (*contactAuthorizer)(nil)

func (adapter *contactAuthorizer) snapshot() (contract.RealmAttachment, bool) {
	if adapter == nil || adapter.owner == nil || adapter.owner.realmPort == nil {
		return contract.RealmAttachment{}, false
	}
	if attachment := adapter.owner.contact.Load(); attachment != nil && attachment.Revision != (contract.RealmContactRevision{}) {
		return *attachment, true
	}
	return contract.RealmAttachment{}, false
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
	return adapter.settle(contact, adapter.owner.realmPort.AuthorizePrincipal(contact.NextHop, contact.Realm, actor, requirements...))
}

func (adapter *contactAuthorizer) AuthorizeView(ctx context.Context, caller values.CallerContext, requirements ...values.Requirement) (values.AuthorizationDecision, values.AuthorizationView) {
	contact, ok := adapter.snapshot()
	if !ok {
		return values.AuthorizationUnavailable, values.AuthorizationView{}
	}
	decision, view := adapter.owner.realmPort.AuthorizeView(ctx, contact.NextHop, contact.Realm, caller, requirements...)
	if adapter.settle(contact, decision) != decision {
		if port, ok := adapter.owner.realmPort.(interface {
			ReleaseRealmEvidence(context.Context, values.NextHop, values.RealmID, [32]byte)
		}); ok && ctx != nil && view != (values.AuthorizationView{}) {
			port.ReleaseRealmEvidence(context.WithoutCancel(ctx), contact.NextHop, contact.Realm, [32]byte(view))
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
	return adapter.settle(contact, adapter.owner.realmPort.ValidateView(ctx, contact.NextHop, contact.Realm, caller, view, requirements...))
}

func (adapter *contactAuthorizer) CaptureDenialView(ctx context.Context, caller values.CallerContext, requirement values.Requirement) (values.AuthorizationDecision, values.AuthorizationDenialView) {
	contact, ok := adapter.snapshot()
	if !ok {
		return values.AuthorizationUnavailable, values.AuthorizationDenialView{}
	}
	decision, view := adapter.owner.realmPort.CaptureDenialView(ctx, contact.NextHop, contact.Realm, caller, requirement)
	if adapter.settle(contact, decision) != decision {
		return values.AuthorizationUnavailable, values.AuthorizationDenialView{}
	}
	return decision, view
}

func (adapter *contactAuthorizer) ValidateDenialView(ctx context.Context, caller values.CallerContext, view values.AuthorizationDenialView, requirement values.Requirement) values.AuthorizationDecision {
	contact, ok := adapter.snapshot()
	if !ok {
		return values.AuthorizationUnavailable
	}
	return adapter.settle(contact, adapter.owner.realmPort.ValidateDenialView(ctx, contact.NextHop, contact.Realm, caller, view, requirement))
}
