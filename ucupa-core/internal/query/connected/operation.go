// Package connected owns the existing trusted-local, two-Core query workflow.
// It borrows the instance's local query snapshots and unique commit owner.
package connected

import (
	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	peercontract "ucupa/ucupa-core/contract/connected"
	commitpkg "ucupa/ucupa-core/internal/commit"
	querypkg "ucupa/ucupa-core/internal/query"
)

type Operations struct {
	owner              *commitpkg.Owner
	authority          values.FactAuthorityRef
	authorizer         contract.Authorizer
	queries            *querypkg.Operations
	connectedTransport peercontract.Transport
	registry           *connectedRegistry
}

func New(owner *commitpkg.Owner, authorizer contract.Authorizer, queries *querypkg.Operations) *Operations {
	return &Operations{owner: owner, authority: owner.AuthorityRef(), authorizer: authorizer, queries: queries}
}

// AuthorityRef identifies the same immutable owner, including after Close.
func (core *Operations) AuthorityRef() values.FactAuthorityRef {
	if core == nil {
		return ""
	}
	return core.authority
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
	return values.Requirement{Permission: permission, Scope: values.CoreElementScope(core.authority, element)}
}
