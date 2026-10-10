package namespace

import (
	"context"

	"ucupa/shared/values"
	"ucupa/ucupa-facet/contract"
)

// The local Realm contact control surface lives on the original namespace
// owner: it has no independent contact state, grant cache or Store. The
// connection port is trusted local assembly, not an address discovery service.

func (f *Owner) currentRealmContact() (contract.RealmAttachment, error) {
	if f == nil || f.ownerState == nil || f.realmPort == nil {
		return contract.RealmAttachment{}, contract.ErrStore
	}
	f.mu.Lock()
	contact, err := f.realmContactLocked()
	f.mu.Unlock()
	return contact, err
}

// authorizeContactMaintenance first attempts the persisted address. An
// explicitly denied maintenance request cannot try the candidate as another
// authorization source. A candidate is used only when the current endpoint is
// unavailable, and it must prove the same Realm as the persisted contact.
// The contact check is a real remote wait under a network profile and carries
// the original request's context.
func (f *Owner) authorizeContactMaintenance(ctx context.Context, contact contract.RealmAttachment, candidate values.NextHop, caller values.CallerContext, permission values.PermissionID) error {
	if ctx == nil || ctx.Err() != nil {
		return contract.ErrUnavailable
	}
	if !caller.Subject.Valid() || !contact.Owner.Valid() || !contact.NextHop.Valid() || contact.Realm == "" {
		return contract.ErrInvalid
	}
	address := contact.NextHop
	if f.realmPort.Check(ctx, address, contact.Realm) != nil {
		if !candidate.Valid() || f.realmPort.Check(ctx, candidate, contact.Realm) != nil {
			return contract.ErrUnavailable
		}
		address = candidate
	}
	requirement := values.Requirement{Permission: permission, Scope: values.AuthorityScope(contact.Owner)}
	decision, view := f.realmPort.AuthorizeView(ctx, address, contact.Realm, caller, requirement)
	if releaser, ok := f.realmPort.(interface {
		ReleaseRealmEvidence(context.Context, values.NextHop, values.RealmID, [32]byte)
	}); ok && view != (values.AuthorizationView{}) {
		defer releaser.ReleaseRealmEvidence(context.WithoutCancel(ctx), address, contact.Realm, [32]byte(view))
	}
	switch decision {
	case values.AuthorizationValid:
		return nil
	case values.AuthorizationDenied:
		return contract.ErrDenied
	default:
		return contract.ErrUnavailable
	}
}

// InspectRealmContact discloses the current contact only after the independent
// read permission. candidate is optional and is useful when the persisted
// address is offline; it is never selected while the current address can
// authorize.
func (f *Owner) InspectRealmContact(ctx context.Context, caller values.CallerContext, candidate values.NextHop) (contract.RealmAttachment, error) {
	contact, err := f.currentRealmContact()
	if err != nil {
		return contract.RealmAttachment{}, err
	}
	if err := f.authorizeContactMaintenance(ctx, contact, candidate, caller, contract.PermissionRealmContactRead); err != nil {
		return contract.RealmAttachment{}, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if ctx.Err() != nil {
		return contract.RealmAttachment{}, contract.ErrUnavailable
	}
	latest, err := f.realmContactLocked()
	if err != nil || latest.Revision != contact.Revision {
		return contract.RealmAttachment{}, contract.ErrUnavailable
	}
	return latest, nil
}

// UpdateRealmContact authorizes through the current contact, then consumes the
// original owner transaction. A contact changed during authorization is not
// judged using a stale approval; the caller must retry against the new current
// address. The original owner retains committed, unchanged and rejected CAS
// results.
func (f *Owner) UpdateRealmContact(ctx context.Context, caller values.CallerContext, request contract.RealmContactUpdate) (contract.RealmContactResult, error) {
	contact, err := f.currentRealmContact()
	if err != nil {
		return contract.RealmContactResult{}, err
	}
	if request.Owner != contact.Owner || request.Realm != contact.Realm || !request.NextHop.Valid() ||
		request.Expected == (contract.RealmContactRevision{}) || !validOperation(request.OperationID) {
		return contract.RealmContactResult{}, contract.ErrInvalid
	}
	if err := f.authorizeContactMaintenance(ctx, contact, request.NextHop, caller, contract.PermissionRealmContactUpdate); err != nil {
		return contract.RealmContactResult{}, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if ctx.Err() != nil {
		return contract.RealmContactResult{}, contract.ErrUnavailable
	}
	latest, err := f.realmContactLocked()
	if err != nil {
		return contract.RealmContactResult{}, err
	}
	if latest.Revision != contact.Revision {
		return contract.RealmContactResult{}, contract.ErrUnavailable
	}
	return f.commitRealmContactLocked(caller.Subject, request)
}

// RecoverRealmContact reads only a retained decision; it never re-executes the
// update. The candidate need not be the old operation's address because later
// maintenance may already have moved the current contact again.
func (f *Owner) RecoverRealmContact(ctx context.Context, caller values.CallerContext, operation contract.OperationID, candidate values.NextHop) (contract.RealmContactResult, error) {
	if !validOperation(operation) {
		return contract.RealmContactResult{}, contract.ErrInvalid
	}
	contact, err := f.currentRealmContact()
	if err != nil {
		return contract.RealmContactResult{}, err
	}
	if err := f.authorizeContactMaintenance(ctx, contact, candidate, caller, contract.PermissionRealmContactUpdate); err != nil {
		return contract.RealmContactResult{}, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if ctx.Err() != nil {
		return contract.RealmContactResult{}, contract.ErrUnavailable
	}
	latest, err := f.realmContactLocked()
	if err != nil || latest.Revision != contact.Revision {
		return contract.RealmContactResult{}, contract.ErrUnavailable
	}
	result, found := f.realmContactOutcomeLocked(operation)
	if !found {
		return contract.RealmContactResult{}, contract.ErrOperationNotFound
	}
	return result, nil
}
