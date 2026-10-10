package policy

import (
	"errors"
	"fmt"
	"math"

	"ucupa/shared/values"
	contract "ucupa/ucupa-realm/contract"
	token "ucupa/ucupa-realm/internal/token"
)

func validateOperationHistory(value normalizedRealm) error {
	changed := make(map[uint64]operationBinding)
	noops := make(map[uint64][]operationBinding)
	for operationID, operation := range value.operations {
		binding := operation.binding
		outcome := operation.outcome
		if !managerBindingExists(value, binding.actor, binding.kind) {
			return fmt.Errorf("Realm operation %q actor lacks immutable manager authority", operationID)
		}
		if !validOnlineTarget(binding.target) || !token.Valid(binding.expected) {
			return fmt.Errorf("Realm operation %q binding is invalid", operationID)
		}
		switch outcome.kind {
		case outcomeSuccess:
			if !token.Valid(outcome.revision) || token.Scope(outcome.revision) != token.Scope(value.revision) ||
				token.Scope(binding.expected) != token.Scope(value.revision) ||
				token.Generation(outcome.revision) > token.Generation(value.revision) {
				return fmt.Errorf("Realm operation %q success revision is foreign or impossible", operationID)
			}
			if binding.target.Subject.Realm == value.id {
				if _, registered := value.localPrincipals[binding.target.Subject.Principal]; !registered {
					return fmt.Errorf("Realm operation %q succeeded for an unregistered local target", operationID)
				}
			}
			switch token.Generation(outcome.revision) {
			case token.Generation(binding.expected):
				noops[token.Generation(outcome.revision)] = append(noops[token.Generation(outcome.revision)], binding)
			case token.Generation(binding.expected) + 1:
				if token.Generation(binding.expected) == math.MaxUint64 || token.Generation(outcome.revision) < 2 {
					return fmt.Errorf("Realm operation %q changed revision is impossible", operationID)
				}
				if _, duplicate := changed[token.Generation(outcome.revision)]; duplicate {
					return fmt.Errorf("duplicate changed generation %d", token.Generation(outcome.revision))
				}
				changed[token.Generation(outcome.revision)] = binding
			default:
				return fmt.Errorf("Realm operation %q success revision is not exact", operationID)
			}
		case outcomeNotFound:
			if outcome.revision != (contract.BindingSetRevision{}) ||
				token.Scope(binding.expected) != token.Scope(value.revision) ||
				token.Generation(binding.expected) > token.Generation(value.revision) ||
				binding.target.Subject.Realm != value.id {
				return fmt.Errorf("Realm operation %q not-found outcome is impossible", operationID)
			}
			if _, registered := value.localPrincipals[binding.target.Subject.Principal]; registered {
				return fmt.Errorf("Realm operation %q retained not-found for a registered Principal", operationID)
			}
		case outcomeRevisionConflict:
			if outcome.revision != (contract.BindingSetRevision{}) ||
				(token.Scope(binding.expected) == token.Scope(value.revision) &&
					token.Generation(binding.expected) >= token.Generation(value.revision)) {
				return fmt.Errorf("Realm operation %q revision-conflict outcome is impossible", operationID)
			}
		default:
			return fmt.Errorf("Realm operation %q outcome is unrecognized", operationID)
		}
	}
	if token.Generation(value.revision)-1 != uint64(len(changed)) {
		return fmt.Errorf(
			"changed operation count %d does not match current generation %d",
			len(changed),
			token.Generation(value.revision),
		)
	}
	return validateBindingHistory(value, changed, noops)
}

func managerBindingExists(
	value normalizedRealm,
	actor values.PrincipalRef,
	kind operationKind,
) bool {
	permission := contract.PermissionPutBinding
	if kind == operationDeleteBinding {
		permission = contract.PermissionDeleteBinding
	} else if kind != operationPutBinding {
		return false
	}
	if actor.Realm != value.id {
		return false
	}
	if _, registered := value.localPrincipals[actor.Principal]; !registered {
		return false
	}
	_, exists := value.bindings[bindingKey{
		subject: actor,
		requirement: values.Requirement{
			Permission: permission,
			Scope:      values.RealmScope(value.id),
		},
	}]
	return exists
}

func validateBindingHistory(
	value normalizedRealm,
	changed map[uint64]operationBinding,
	noops map[uint64][]operationBinding,
) error {
	touched := make(map[bindingKey]struct{})
	first := make(map[bindingKey]operationKind)
	for generation := uint64(2); generation <= token.Generation(value.revision); generation++ {
		binding := changed[generation]
		key := bindingKeyFor(binding.target)
		touched[key] = struct{}{}
		if _, exists := first[key]; !exists {
			first[key] = binding.kind
		}
		if generation == math.MaxUint64 {
			break
		}
	}
	for _, bindings := range noops {
		for _, binding := range bindings {
			touched[bindingKeyFor(binding.target)] = struct{}{}
		}
	}
	membership := make(map[bindingKey]bool, len(touched))
	for key := range touched {
		if kind, changedLater := first[key]; changedLater {
			membership[key] = kind == operationDeleteBinding
			continue
		}
		_, membership[key] = value.bindings[key]
	}
	for generation := uint64(1); generation <= token.Generation(value.revision); generation++ {
		if generation >= 2 {
			binding := changed[generation]
			key := bindingKeyFor(binding.target)
			switch binding.kind {
			case operationPutBinding:
				if membership[key] {
					return fmt.Errorf("changed Put at generation %d was already present", generation)
				}
				membership[key] = true
			case operationDeleteBinding:
				if !membership[key] {
					return fmt.Errorf("changed Delete at generation %d was already absent", generation)
				}
				membership[key] = false
			default:
				return fmt.Errorf("changed generation %d has invalid kind", generation)
			}
		}
		for _, binding := range noops[generation] {
			present := membership[bindingKeyFor(binding.target)]
			if (binding.kind == operationPutBinding && !present) ||
				(binding.kind == operationDeleteBinding && present) {
				return fmt.Errorf("no-op at generation %d contradicts membership", generation)
			}
		}
		if generation == math.MaxUint64 {
			break
		}
	}
	for key := range touched {
		_, present := value.bindings[key]
		if membership[key] != present {
			return errors.New("operation history contradicts current binding set")
		}
	}
	return nil
}

func bindingKeyFor(binding contract.PermissionBinding) bindingKey {
	return bindingKey{
		subject: binding.Subject,
		requirement: values.Requirement{
			Permission: binding.Permission,
			Scope:      binding.Scope,
		},
	}
}
