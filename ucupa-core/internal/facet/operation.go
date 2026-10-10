// Package facet implements the Core-side Facet relationship, not Facet content.
package facet

import (
	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	"ucupa/ucupa-core/internal/access"
	"ucupa/ucupa-core/internal/access/registry"
	"ucupa/ucupa-core/internal/commit"
)

// Operations owns relationship request execution and the fixed acceptance source.
// It shares the Core's sole fact/commit lock and sole public admission instance.
type Operations struct {
	owner                  *commit.Owner
	authorizer             contract.Authorizer
	admission              *access.Admission
	registry               *registry.Registry
	facetSource            contract.FacetAcceptanceSource
	facetDispositionSource contract.FacetDispositionSource
}

func New(owner *commit.Owner, authorizer contract.Authorizer, admission *access.Admission, resources *registry.Registry) *Operations {
	return &Operations{owner: owner, authorizer: authorizer, admission: admission, registry: resources}
}

// AuthorityRef supplies the actual local reverse-contact identity port. It is
// immutable across Close and is not proof of availability or authentication.
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
