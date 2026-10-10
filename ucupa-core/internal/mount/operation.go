// Package mount owns local and authenticated mount control and peer coordination.
package mount

import (
	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	"ucupa/ucupa-core/internal/access"
	"ucupa/ucupa-core/internal/commit"
)

// Operations uses the Core's one commit owner and shared public admission.
// Transport configuration is guarded by the same fact lock; it is not a
// second lifecycle, fact set, control slot or permission authority.
type Operations struct {
	owner          *commit.Owner
	authorizer     contract.Authorizer
	admission      *access.Admission
	mountTransport contract.MountTransport
}

func New(owner *commit.Owner, authorizer contract.Authorizer, admission *access.Admission) *Operations {
	return &Operations{owner: owner, authorizer: authorizer, admission: admission}
}

// AuthorityRef is the immutable identity used by trusted peer assembly.
func (core *Operations) AuthorityRef() values.FactAuthorityRef {
	if core == nil {
		return ""
	}
	return core.owner.AuthorityRef()
}

// authorize resolves one typed requirement set through the owner's current
// durable Realm contact. An unreachable or unverifiable Realm is never
// compressed into a denial. The returned approval is the contact generation
// captured before the authorization: consuming it at a later final read or
// commit boundary requires the same generation to still be current.
func (core *Operations) authorize(actor values.PrincipalRef, requirements ...values.Requirement) (contract.RealmAttachment, error) {
	if core == nil || core.authorizer == nil {
		return contract.RealmAttachment{}, contract.ErrStore
	}
	if !actor.Valid() {
		return contract.RealmAttachment{}, contract.ErrDenied
	}
	approval, ok := core.owner.CurrentContact()
	if !ok {
		return contract.RealmAttachment{}, contract.ErrUnavailable
	}
	switch core.authorizer.AuthorizePrincipal(actor, requirements...) {
	case values.AuthorizationValid:
		return approval, nil
	case values.AuthorizationDenied:
		return contract.RealmAttachment{}, contract.ErrDenied
	default:
		return contract.RealmAttachment{}, contract.ErrUnavailable
	}
}

// settled reports whether an authorization approval's contact generation is
// still the current one at a final read or commit boundary.
func (core *Operations) settled(approval contract.RealmAttachment) bool {
	current, ok := core.owner.CurrentContact()
	return ok && current == approval
}
func (core *Operations) requirement(permission values.PermissionID, element values.ElementID) values.Requirement {
	return values.Requirement{Permission: permission, Scope: values.CoreElementScope(core.owner.AuthorityRef(), element)}
}
