// Package element implements authenticated local Element operations against
// the Core's single commit owner. It owns neither a second state nor a Store.
package element

import (
	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	"ucupa/ucupa-core/internal/commit"
	commitbinding "ucupa/ucupa-core/internal/commit/binding"
	elementfact "ucupa/ucupa-core/internal/element/fact"
	coretoken "ucupa/ucupa-core/internal/token"
)

// Operations is the authenticated local Element read/write entry. The owning instance assembles
// it with the same fact owner and authorizer used by its other operations.
type Operations struct {
	owner      *commit.Owner
	authorizer contract.Authorizer
}

func New(owner *commit.Owner, authorizer contract.Authorizer) *Operations {
	return &Operations{owner: owner, authorizer: authorizer}
}

func (core *Operations) CreateRoot(
	actor values.PrincipalRef,
	id values.ElementID,
	operationID contract.OperationID,
) (contract.MutationResult, error) {
	if core == nil || !id.Valid() || !commitbinding.ValidOperationID(operationID) {
		return contract.MutationResult{}, contract.ErrInvalid
	}
	approval, err := core.authorize(actor, core.requirement(contract.PermissionCreateRoot, id))
	if err != nil {
		return contract.MutationResult{}, err
	}
	binding := commitbinding.Binding{Actor: actor, Kind: elementfact.OperationCreateRoot, Element: id}
	core.owner.Lock()
	defer core.owner.Unlock()
	if !core.settled(approval) {
		return contract.MutationResult{}, contract.ErrUnavailable
	}
	return core.owner.ApplyElementLocked(operationID, binding)
}

func (core *Operations) CreateAnchor(
	actor values.PrincipalRef,
	id values.ElementID,
	final contract.SpatialTuple,
	operationID contract.OperationID,
) (contract.MutationResult, error) {
	return core.createChild(actor, id, final, operationID, elementfact.OperationCreateAnchor, contract.PermissionCreateAnchor)
}

func (core *Operations) CreateEntity(
	actor values.PrincipalRef,
	id values.ElementID,
	final contract.SpatialTuple,
	operationID contract.OperationID,
) (contract.MutationResult, error) {
	return core.createChild(actor, id, final, operationID, elementfact.OperationCreateEntity, contract.PermissionCreateEntity)
}

func (core *Operations) createChild(
	actor values.PrincipalRef,
	id values.ElementID,
	final contract.SpatialTuple,
	operationID contract.OperationID,
	kind elementfact.OperationKind,
	permission values.PermissionID,
) (contract.MutationResult, error) {
	if core == nil || !id.Valid() || !commitbinding.ValidOperationID(operationID) {
		return contract.MutationResult{}, contract.ErrInvalid
	}
	canonical, ok := commitbinding.CanonicalFinalTuple(final)
	if !ok {
		return contract.MutationResult{}, contract.ErrInvalid
	}
	approval, err := core.authorize(actor, core.requirement(permission, canonical.Parent))
	if err != nil {
		return contract.MutationResult{}, err
	}
	binding := commitbinding.Binding{Actor: actor, Kind: kind, Element: id, Final: canonical}
	core.owner.Lock()
	defer core.owner.Unlock()
	if !core.settled(approval) {
		return contract.MutationResult{}, contract.ErrUnavailable
	}
	return core.owner.ApplyElementLocked(operationID, binding)
}

func (core *Operations) UpdateAnchor(
	actor values.PrincipalRef,
	id values.ElementID,
	final contract.SpatialTuple,
	expected contract.Revision,
	operationID contract.OperationID,
) (contract.MutationResult, error) {
	return core.update(actor, id, final, expected, operationID, elementfact.OperationUpdateAnchor)
}

func (core *Operations) UpdateEntity(
	actor values.PrincipalRef,
	id values.ElementID,
	final contract.SpatialTuple,
	expected contract.Revision,
	operationID contract.OperationID,
) (contract.MutationResult, error) {
	return core.update(actor, id, final, expected, operationID, elementfact.OperationUpdateEntity)
}

func (core *Operations) update(
	actor values.PrincipalRef,
	id values.ElementID,
	final contract.SpatialTuple,
	expected contract.Revision,
	operationID contract.OperationID,
	kind elementfact.OperationKind,
) (contract.MutationResult, error) {
	if core == nil || !id.Valid() || !coretoken.ValidRevision(expected) || !commitbinding.ValidOperationID(operationID) {
		return contract.MutationResult{}, contract.ErrInvalid
	}
	canonical, ok := commitbinding.CanonicalFinalTuple(final)
	if !ok {
		return contract.MutationResult{}, contract.ErrInvalid
	}
	approval, err := core.authorize(actor,
		core.requirement(contract.PermissionUpdateElement, id),
		core.requirement(contract.PermissionPlaceElement, canonical.Parent),
	)
	if err != nil {
		return contract.MutationResult{}, err
	}
	binding := commitbinding.Binding{
		Actor: actor, Kind: kind, Element: id, Final: canonical, Expected: expected,
	}
	core.owner.Lock()
	defer core.owner.Unlock()
	if !core.settled(approval) {
		return contract.MutationResult{}, contract.ErrUnavailable
	}
	return core.owner.ApplyElementLocked(operationID, binding)
}

func (core *Operations) DeleteAnchor(
	actor values.PrincipalRef,
	id values.ElementID,
	expected contract.Revision,
	operationID contract.OperationID,
) (contract.MutationResult, error) {
	return core.deleteElement(actor, id, expected, operationID, elementfact.OperationDeleteAnchor, contract.PermissionDeleteAnchor)
}

func (core *Operations) DeleteEntity(
	actor values.PrincipalRef,
	id values.ElementID,
	expected contract.Revision,
	operationID contract.OperationID,
) (contract.MutationResult, error) {
	return core.deleteElement(actor, id, expected, operationID, elementfact.OperationDeleteEntity, contract.PermissionDeleteEntity)
}

func (core *Operations) deleteElement(
	actor values.PrincipalRef,
	id values.ElementID,
	expected contract.Revision,
	operationID contract.OperationID,
	kind elementfact.OperationKind,
	permission values.PermissionID,
) (contract.MutationResult, error) {
	if core == nil || !id.Valid() || !coretoken.ValidRevision(expected) || !commitbinding.ValidOperationID(operationID) {
		return contract.MutationResult{}, contract.ErrInvalid
	}
	approval, err := core.authorize(actor, core.requirement(permission, id))
	if err != nil {
		return contract.MutationResult{}, err
	}
	binding := commitbinding.Binding{
		Actor: actor, Kind: kind, Element: id, Expected: expected,
	}
	core.owner.Lock()
	defer core.owner.Unlock()
	if !core.settled(approval) {
		return contract.MutationResult{}, contract.ErrUnavailable
	}
	return core.owner.ApplyElementLocked(operationID, binding)
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

// AuthorityRef lets the existing local connection port verify this captured
// operation owner before and after a read. It remains stable after Close.
func (core *Operations) AuthorityRef() values.FactAuthorityRef {
	if core == nil {
		return ""
	}
	return core.owner.AuthorityRef()
}
