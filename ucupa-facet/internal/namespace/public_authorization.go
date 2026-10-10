package namespace

import (
	"context"

	"ucupa/shared/values"
	"ucupa/ucupa-facet/contract"
)

// facetPermit belongs to one in-flight operation, not a registry or durable
// capability. Network decisions are obtained without mu; their final guard
// only compares the captured contact and request lifetime under mu. Local
// contacts retain the original bounded kernel validation at that boundary.
type facetPermit struct {
	owner        *Owner
	contact      contract.RealmAttachment
	caller       values.CallerContext
	view         values.AuthorizationView
	requirements []values.Requirement
}

func (f *Owner) authorizePublic(ctx context.Context, caller values.CallerContext, requirements []values.Requirement) (*facetPermit, error) {
	contact, ok := f.contactSnapshot()
	if !ok || ctx == nil || ctx.Err() != nil {
		return nil, values.PublicUnavailable
	}
	decision, view := f.authorizer.AuthorizeView(ctx, caller, requirements...)
	p := &facetPermit{f, contact, caller, view, append([]values.Requirement(nil), requirements...)}
	if decision != values.AuthorizationValid || !f.settledContact(contact) || ctx.Err() != nil {
		p.release(ctx)
		if decision != values.AuthorizationValid {
			return nil, facetViewError(decision, false)
		}
		return nil, values.PublicUnavailable
	}
	return p, nil
}

// recheckPublic checks the original view, never replaces it with a fresh Allow.
// The returned permit owns no new handle; the receipt that owns the view stays
// responsible for releasing it after its last use.
func (f *Owner) recheckPublic(ctx context.Context, caller values.CallerContext, view values.AuthorizationView, requirements []values.Requirement, observe bool) (*facetPermit, error) {
	contact, ok := f.contactSnapshot()
	if !ok || ctx == nil || ctx.Err() != nil {
		return nil, values.PublicUnavailable
	}
	p := &facetPermit{f, contact, caller, view, append([]values.Requirement(nil), requirements...)}
	if err := p.recheck(ctx, observe); err != nil {
		return nil, err
	}
	return p, nil
}

// recheck must be called outside the fact lock, after external Core reads.
func (p *facetPermit) recheck(ctx context.Context, observe bool) error {
	if p == nil || ctx == nil || ctx.Err() != nil || !p.owner.settledContact(p.contact) {
		return values.PublicUnavailable
	}
	if p.contact.NextHop.IsLocal() {
		// Local continuity is checked at the original locked guard positions;
		// do not insert an additional kernel decision before those positions.
		return nil
	}
	decision := p.owner.authorizer.ValidateView(ctx, p.caller, p.view, p.requirements...)
	if decision != values.AuthorizationValid {
		return facetViewError(decision, observe)
	}
	if ctx.Err() != nil || !p.owner.settledContact(p.contact) {
		return values.PublicUnavailable
	}
	return nil
}

func (p *facetPermit) guardLocked(ctx context.Context, observe bool) error {
	if p == nil || ctx == nil || ctx.Err() != nil || !p.owner.settledContact(p.contact) {
		return values.PublicUnavailable
	}
	if p.contact.NextHop.IsLocal() {
		decision := p.owner.authorizer.ValidateView(ctx, p.caller, p.view, p.requirements...)
		if decision != values.AuthorizationValid {
			return facetViewError(decision, observe)
		}
		if ctx.Err() != nil || !p.owner.settledContact(p.contact) {
			return values.PublicUnavailable
		}
	}
	return nil
}

func (p *facetPermit) release(ctx context.Context) {
	if p == nil || ctx == nil || p.view == (values.AuthorizationView{}) {
		return
	}
	if port, ok := p.owner.realmPort.(interface {
		ReleaseRealmEvidence(context.Context, values.NextHop, values.RealmID, [32]byte)
	}); ok {
		port.ReleaseRealmEvidence(context.WithoutCancel(ctx), p.contact.NextHop, p.contact.Realm, [32]byte(p.view))
	}
}

// Replacement peers consume the authenticated fixed-service caller, not its
// subject name. They share the public view mechanism and original owner lock.
func (f *Owner) authorizeNamespacePeer(ctx context.Context, caller values.CallerContext, key values.FacetAcceptanceKey, permission values.PermissionID) (*facetPermit, error) {
	if f == nil || f.ownerState == nil || f.authorizer == nil {
		return nil, contract.ErrStore
	}
	permit, err := f.authorizePublic(ctx, caller, []values.Requirement{{Permission: permission, Scope: values.FacetElementScope(f.authority, key.Element)}})
	if err == values.PublicDenied {
		return nil, contract.ErrDenied
	}
	if err != nil {
		return nil, contract.ErrUnavailable
	}
	return permit, nil
}

func (f *Owner) namespacePeerGuardLocked(ctx context.Context, permit *facetPermit) error {
	if err := f.available(); err != nil {
		return err
	}
	if permit.guardLocked(ctx, false) != nil {
		return contract.ErrUnavailable
	}
	return nil
}
