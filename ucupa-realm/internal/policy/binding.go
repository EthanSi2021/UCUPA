package policy

import (
	"fmt"
	"math"

	"ucupa/shared/values"
	contract "ucupa/ucupa-realm/contract"
	token "ucupa/ucupa-realm/internal/token"
)

type bindingKey struct {
	subject     values.PrincipalRef
	requirement values.Requirement
}

type operationKind uint8

const (
	operationPutBinding operationKind = iota + 1
	operationDeleteBinding
)

type operationBinding struct {
	actor    values.PrincipalRef
	kind     operationKind
	target   contract.PermissionBinding
	expected contract.BindingSetRevision
}

type retainedOutcomeKind uint8

const (
	outcomeSuccess retainedOutcomeKind = iota + 1
	outcomeNotFound
	outcomeRevisionConflict
)

type retainedOutcome struct {
	kind     retainedOutcomeKind
	revision contract.BindingSetRevision
}

func (outcome retainedOutcome) result() (contract.BindingSetRevision, error) {
	switch outcome.kind {
	case outcomeSuccess:
		return outcome.revision, nil
	case outcomeNotFound:
		return contract.BindingSetRevision{}, contract.ErrNotFound
	case outcomeRevisionConflict:
		return contract.BindingSetRevision{}, contract.ErrRevisionConflict
	default:
		return contract.BindingSetRevision{}, contract.ErrStore
	}
}

type retainedOperation struct {
	binding operationBinding
	outcome retainedOutcome
}

type bindingChange uint8

const (
	bindingUnchanged bindingChange = iota
	bindingPut
	bindingDelete
)

type mutationPlan struct {
	change  bindingChange
	key     bindingKey
	outcome retainedOutcome
}

type mutationState struct {
	id              values.RealmID
	localPrincipals map[values.PrincipalID]struct{}
	bindings        map[bindingKey]struct{}
	revision        contract.BindingSetRevision
}

func (realm *Realm) BindingSetRevision() contract.BindingSetRevision {
	if realm == nil || realm.state == nil {
		return contract.BindingSetRevision{}
	}
	state := realm.state
	state.mu.RLock()
	defer state.mu.RUnlock()
	if state.retired {
		return contract.BindingSetRevision{}
	}
	return state.revision
}

func (realm *Realm) Authorize(actor values.PrincipalRef, requirements ...values.Requirement) bool {
	return realm.AuthorizePrincipal(actor, requirements...) == values.AuthorizationValid
}

// AuthorizePrincipal is the trusted local decision for callers that already
// have an established principal. Unlike the legacy bool form, it does not
// confuse an unavailable Realm with a proven permission denial.
func (realm *Realm) AuthorizePrincipal(actor values.PrincipalRef, requirements ...values.Requirement) values.AuthorizationDecision {
	if realm == nil || realm.state == nil || !actor.Valid() || len(requirements) == 0 {
		if realm == nil || realm.state == nil {
			return values.AuthorizationUnavailable
		}
		return values.AuthorizationDenied
	}
	state := realm.state
	state.mu.RLock()
	defer state.mu.RUnlock()
	if state.retired {
		return values.AuthorizationUnavailable
	}
	if actor.Realm == state.id {
		if _, exists := state.localPrincipals[actor.Principal]; !exists {
			return values.AuthorizationDenied
		}
	}

	for _, requirement := range requirements {
		if !requirement.Valid() {
			return values.AuthorizationDenied
		}
		if _, bound := state.bindings[bindingKey{subject: actor, requirement: requirement}]; !bound {
			return values.AuthorizationDenied
		}
	}
	return values.AuthorizationValid
}

func (realm *Realm) PutBinding(
	actor values.PrincipalRef,
	binding contract.PermissionBinding,
	expectedRevision contract.BindingSetRevision,
	operationID contract.OperationID,
) (contract.BindingSetRevision, error) {
	return realm.mutateBinding(actor, operationPutBinding, binding, expectedRevision, operationID)
}

func (realm *Realm) DeleteBinding(
	actor values.PrincipalRef,
	binding contract.PermissionBinding,
	expectedRevision contract.BindingSetRevision,
	operationID contract.OperationID,
) (contract.BindingSetRevision, error) {
	return realm.mutateBinding(actor, operationDeleteBinding, binding, expectedRevision, operationID)
}

func (realm *Realm) mutateBinding(
	actor values.PrincipalRef,
	kind operationKind,
	target contract.PermissionBinding,
	expected contract.BindingSetRevision,
	operationID contract.OperationID,
) (contract.BindingSetRevision, error) {
	if operationID == "" || !token.Valid(expected) || !validOnlineTarget(target) {
		return contract.BindingSetRevision{}, contract.ErrInvalid
	}
	if realm == nil || realm.state == nil {
		return contract.BindingSetRevision{}, contract.ErrStore
	}

	session := realm.state
	session.mu.Lock()
	defer session.mu.Unlock()
	if session.retired {
		return contract.BindingSetRevision{}, contract.ErrStore
	}
	permission := contract.PermissionPutBinding
	if kind == operationDeleteBinding {
		permission = contract.PermissionDeleteBinding
	}
	if !session.managerAuthorizedLocked(actor, permission) {
		return contract.BindingSetRevision{}, contract.ErrDenied
	}

	binding := operationBinding{actor: actor, kind: kind, target: target, expected: expected}
	if retained, exists := session.operations[operationID]; exists {
		if retained.binding != binding {
			return contract.BindingSetRevision{}, contract.ErrOperationConflict
		}
		return retained.outcome.result()
	}

	state := mutationState{
		id:              session.id,
		localPrincipals: session.localPrincipals,
		bindings:        session.bindings,
		revision:        session.revision,
	}
	var plan mutationPlan
	if session.store == nil {
		var err error
		plan, err = planMutation(binding, state)
		if err != nil {
			return contract.BindingSetRevision{}, err
		}
	} else {
		var failure storeFailure
		var err error
		plan, failure, err = session.store.commitOperation(operationID, binding, state)
		if err != nil {
			if failure != storeFailureDefinitePrecommit {
				session.retireLocked()
			}
			return contract.BindingSetRevision{}, err
		}
	}

	session.publishMutationLocked(operationID, binding, plan)
	return plan.outcome.result()
}

func validOnlineTarget(binding contract.PermissionBinding) bool {
	requirement := values.Requirement{Permission: binding.Permission, Scope: binding.Scope}
	if !binding.Subject.Valid() || !requirement.Valid() || isAdministrationPermission(binding.Permission) {
		return false
	}
	if _, realmScope := binding.Scope.RealmTarget(); realmScope {
		return false
	}
	_, _, element := binding.Scope.CoreElementTarget()
	_, _, namespace := binding.Scope.FacetElementTarget()
	_, _, key := binding.Scope.FacetKeyTarget()
	_, _, closure := binding.Scope.AnchorClosureTarget()
	_, authority := binding.Scope.AuthorityTarget()
	return element || namespace || key || closure || authority
}

func isAdministrationPermission(permission values.PermissionID) bool {
	return permission == contract.PermissionPutBinding ||
		permission == contract.PermissionDeleteBinding ||
		permission == contract.PermissionRotateOwnCredential ||
		permission == contract.PermissionRevokeCredential
}

func (state *realmState) managerAuthorizedLocked(
	actor values.PrincipalRef,
	permission values.PermissionID,
) bool {
	if !actor.Valid() || actor.Realm != state.id {
		return false
	}
	if _, exists := state.localPrincipals[actor.Principal]; !exists {
		return false
	}
	_, bound := state.bindings[bindingKey{
		subject: actor,
		requirement: values.Requirement{
			Permission: permission,
			Scope:      values.RealmScope(state.id),
		},
	}]
	return bound
}

func planMutation(binding operationBinding, state mutationState) (mutationPlan, error) {
	if binding.expected != state.revision {
		return mutationPlan{outcome: retainedOutcome{kind: outcomeRevisionConflict}}, nil
	}
	if binding.target.Subject.Realm == state.id {
		if _, registered := state.localPrincipals[binding.target.Subject.Principal]; !registered {
			return mutationPlan{outcome: retainedOutcome{kind: outcomeNotFound}}, nil
		}
	}

	key := bindingKey{
		subject: binding.target.Subject,
		requirement: values.Requirement{
			Permission: binding.target.Permission,
			Scope:      binding.target.Scope,
		},
	}
	_, present := state.bindings[key]
	change := bindingUnchanged
	switch binding.kind {
	case operationPutBinding:
		if !present {
			change = bindingPut
		}
	case operationDeleteBinding:
		if present {
			change = bindingDelete
		}
	default:
		return mutationPlan{}, contract.ErrInvalid
	}

	revision := state.revision
	if change != bindingUnchanged {
		if token.Generation(revision) == math.MaxUint64 {
			return mutationPlan{}, fmt.Errorf("%w: binding-set revision space exhausted", contract.ErrStore)
		}
		revision = token.Next(revision)
	}
	return mutationPlan{
		change:  change,
		key:     key,
		outcome: retainedOutcome{kind: outcomeSuccess, revision: revision},
	}, nil
}

func (state *realmState) publishMutationLocked(
	operationID contract.OperationID,
	binding operationBinding,
	plan mutationPlan,
) {
	if plan.change != bindingUnchanged {
		next := make(map[bindingKey]struct{}, len(state.bindings)+1)
		for key := range state.bindings {
			next[key] = struct{}{}
		}
		switch plan.change {
		case bindingPut:
			next[plan.key] = struct{}{}
		case bindingDelete:
			delete(next, plan.key)
		}
		state.bindings = next
		state.revision = plan.outcome.revision
		state.bindingEpochs[plan.key] = token.Generation(plan.outcome.revision)
	}
	state.operations[operationID] = retainedOperation{binding: binding, outcome: plan.outcome}
}
