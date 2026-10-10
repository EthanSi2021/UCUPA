// Package realmcontact owns the Core's local Realm contact maintenance flow.
// The original commit owner alone stores contact facts and retained results.
package realmcontact

import (
	"context"

	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	"ucupa/ucupa-core/internal/commit"
)

type contactOwner interface {
	AuthorityRef() values.FactAuthorityRef
	RLock()
	RUnlock()
	Lock()
	Unlock()
	RealmContactLocked() (contract.RealmAttachment, error)
	RealmContactOutcomeLocked(contract.OperationID) (contract.RealmContactResult, bool)
	ApplyRealmContactLocked(values.PrincipalRef, contract.RealmContactUpdate) (contract.RealmContactResult, error)
}

var _ contactOwner = (*commit.Owner)(nil)

// Operations has no independent contact state, grant cache or Store. The
// connection port is trusted local assembly, not an address discovery service.
type Operations struct {
	owner       contactOwner
	connections contract.RealmConnections
}

func New(owner *commit.Owner, connections contract.RealmConnections) *Operations {
	return &Operations{owner: owner, connections: connections}
}

func (o *Operations) current() (contract.RealmAttachment, error) {
	if o == nil || o.owner == nil || o.connections == nil {
		return contract.RealmAttachment{}, contract.ErrStore
	}
	o.owner.RLock()
	contact, err := o.owner.RealmContactLocked()
	o.owner.RUnlock()
	return contact, err
}

// authorize first attempts the persisted address. An explicitly denied
// maintenance request cannot try the candidate as another authorization
// source. A candidate is used only when the current endpoint is unavailable.
// The caller is an established Realm session; authorization runs online
// through the address's serving port while no fact lock is held, so it
// carries the original request's context.
func (o *Operations) authorize(ctx context.Context, contact contract.RealmAttachment, candidate values.NextHop, caller values.CallerContext, permission values.PermissionID) error {
	if ctx == nil || ctx.Err() != nil {
		return contract.ErrUnavailable
	}
	if !caller.Subject.Valid() || !contact.Owner.Valid() || !contact.NextHop.Valid() || contact.Realm == "" {
		return contract.ErrConflict
	}
	address := contact.NextHop
	if o.connections.Check(ctx, address, contact.Realm) != nil {
		if !candidate.Valid() || o.connections.Check(ctx, candidate, contact.Realm) != nil {
			return contract.ErrUnavailable
		}
		address = candidate
	}
	requirement := values.Requirement{Permission: permission, Scope: values.AuthorityScope(contact.Owner)}
	decision, view := o.connections.AuthorizeView(ctx, address, contact.Realm, caller, requirement)
	if releaser, ok := o.connections.(contract.RealmEvidenceReleaser); ok && view != (values.AuthorizationView{}) {
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

// Inspect discloses the current contact only after the independent read
// permission. candidate is optional and is useful when the persisted address
// is offline; it is never selected while the current address can authorize.
func (o *Operations) Inspect(ctx context.Context, caller values.CallerContext, candidate values.NextHop) (contract.RealmAttachment, error) {
	contact, err := o.current()
	if err != nil {
		return contract.RealmAttachment{}, err
	}
	if err := o.authorize(ctx, contact, candidate, caller, contract.PermissionRealmContactRead); err != nil {
		return contract.RealmAttachment{}, err
	}
	o.owner.RLock()
	defer o.owner.RUnlock()
	if ctx.Err() != nil {
		return contract.RealmAttachment{}, contract.ErrUnavailable
	}
	latest, err := o.owner.RealmContactLocked()
	if err != nil || latest.Revision != contact.Revision {
		return contract.RealmAttachment{}, contract.ErrUnavailable
	}
	return latest, nil
}

// Update uses the original owner transaction after authorization. A contact
// changed during authorization is not judged using a stale approval; the
// caller must retry against the new current address. The original owner
// retains committed, unchanged and rejected CAS results. The new address may
// be offline at commit time; its first use re-verifies the same Realm.
func (o *Operations) Update(ctx context.Context, caller values.CallerContext, request contract.RealmContactUpdate) (contract.RealmContactResult, error) {
	contact, err := o.current()
	if err != nil {
		return contract.RealmContactResult{}, err
	}
	if request.Owner != contact.Owner || request.Realm != contact.Realm || !request.NextHop.Valid() ||
		request.Expected == (contract.RealmContactRevision{}) || len(request.OperationID) == 0 || len(request.OperationID) > 128 {
		return contract.RealmContactResult{}, contract.ErrConflict
	}
	if err := o.authorize(ctx, contact, request.NextHop, caller, contract.PermissionRealmContactUpdate); err != nil {
		return contract.RealmContactResult{}, err
	}
	o.owner.Lock()
	defer o.owner.Unlock()
	if ctx.Err() != nil {
		return contract.RealmContactResult{}, contract.ErrUnavailable
	}
	latest, err := o.owner.RealmContactLocked()
	if err != nil {
		return contract.RealmContactResult{}, err
	}
	if latest.Revision != contact.Revision {
		return contract.RealmContactResult{}, contract.ErrUnavailable
	}
	return o.owner.ApplyRealmContactLocked(caller.Subject, request)
}

// Recover reads only a retained decision; it never re-executes the update.
// The candidate need not be the old operation's address because later
// maintenance may already have moved the current contact again.
func (o *Operations) Recover(ctx context.Context, caller values.CallerContext, operation contract.OperationID, candidate values.NextHop) (contract.RealmContactResult, error) {
	if len(operation) == 0 || len(operation) > 128 {
		return contract.RealmContactResult{}, contract.ErrConflict
	}
	contact, err := o.current()
	if err != nil {
		return contract.RealmContactResult{}, err
	}
	if err := o.authorize(ctx, contact, candidate, caller, contract.PermissionRealmContactUpdate); err != nil {
		return contract.RealmContactResult{}, err
	}
	o.owner.RLock()
	defer o.owner.RUnlock()
	if ctx.Err() != nil {
		return contract.RealmContactResult{}, contract.ErrUnavailable
	}
	latest, err := o.owner.RealmContactLocked()
	if err != nil || latest.Revision != contact.Revision {
		return contract.RealmContactResult{}, contract.ErrUnavailable
	}
	result, found := o.owner.RealmContactOutcomeLocked(operation)
	if !found {
		return contract.RealmContactResult{}, contract.ErrNotFound
	}
	return result, nil
}
