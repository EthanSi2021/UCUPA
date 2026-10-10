package policy

import (
	"errors"
	"fmt"
	"math"

	"ucupa/shared/values"
	contract "ucupa/ucupa-realm/contract"
	token "ucupa/ucupa-realm/internal/token"
)

type credentialHistoryOperation struct {
	id      contract.CredentialOperationID
	binding credentialOperationBinding
	outcome retainedCredentialOutcome
}

func validateCredentialOperationHistory(value normalizedRealm) error {
	changed := make(map[uint64]credentialHistoryOperation)
	observations := make(map[uint64][]credentialHistoryOperation)
	for operationID, operation := range value.credentialOperations {
		binding := operation.binding
		outcome := operation.outcome
		entry := credentialHistoryOperation{id: operationID, binding: binding, outcome: outcome}
		if !credentialManagerBindingExists(value, binding.actor, binding.kind) {
			return fmt.Errorf(
				"Realm credential operation %q actor lacks immutable lifecycle authority",
				operationID,
			)
		}
		if !validStoredCredentialOperationBinding(binding) {
			return fmt.Errorf("Realm credential operation %q binding is invalid", operationID)
		}
		if binding.kind == credentialOperationRotate {
			old, claimed := value.credentials[binding.target]
			if !claimed || old.principal != binding.actor.Principal {
				return fmt.Errorf(
					"Realm credential operation %q rotation source is not permanently associated with its actor",
					operationID,
				)
			}
		}

		switch outcome.kind {
		case credentialOutcomeSuccess:
			if !token.Valid(outcome.revision) ||
				token.Scope(outcome.revision) != token.Scope(value.credentialRevision) ||
				token.Scope(binding.expected) != token.Scope(value.credentialRevision) ||
				token.Generation(outcome.revision) > token.Generation(value.credentialRevision) {
				return fmt.Errorf(
					"Realm credential operation %q success revision is foreign or impossible",
					operationID,
				)
			}
			switch token.Generation(outcome.revision) {
			case token.Generation(binding.expected):
				if binding.kind != credentialOperationRevoke {
					return fmt.Errorf(
						"Realm credential operation %q Rotate cannot retain a no-op",
						operationID,
					)
				}
				observations[token.Generation(binding.expected)] = append(
					observations[token.Generation(binding.expected)],
					entry,
				)
			case token.Generation(binding.expected) + 1:
				if token.Generation(binding.expected) == math.MaxUint64 ||
					token.Generation(outcome.revision) < 2 ||
					(binding.kind == credentialOperationRotate && binding.target == binding.replacement) {
					return fmt.Errorf(
						"Realm credential operation %q changed revision is impossible",
						operationID,
					)
				}
				if _, duplicate := changed[token.Generation(outcome.revision)]; duplicate {
					return fmt.Errorf(
						"duplicate changed credential generation %d",
						token.Generation(outcome.revision),
					)
				}
				changed[token.Generation(outcome.revision)] = entry
			default:
				return fmt.Errorf(
					"Realm credential operation %q success revision is not exact",
					operationID,
				)
			}
		case credentialOutcomeNotFound:
			if outcome.revision != (contract.CredentialSetRevision{}) ||
				binding.kind != credentialOperationRevoke ||
				token.Scope(binding.expected) != token.Scope(value.credentialRevision) ||
				token.Generation(binding.expected) > token.Generation(value.credentialRevision) {
				return fmt.Errorf(
					"Realm credential operation %q not-found outcome is impossible",
					operationID,
				)
			}
			observations[token.Generation(binding.expected)] = append(
				observations[token.Generation(binding.expected)],
				entry,
			)
		case credentialOutcomeConflict:
			if outcome.revision != (contract.CredentialSetRevision{}) ||
				binding.kind != credentialOperationRotate ||
				token.Scope(binding.expected) != token.Scope(value.credentialRevision) ||
				token.Generation(binding.expected) > token.Generation(value.credentialRevision) {
				return fmt.Errorf(
					"Realm credential operation %q conflict outcome is impossible",
					operationID,
				)
			}
			observations[token.Generation(binding.expected)] = append(
				observations[token.Generation(binding.expected)],
				entry,
			)
		case credentialOutcomeRevisionConflict:
			if outcome.revision != (contract.CredentialSetRevision{}) ||
				(token.Scope(binding.expected) == token.Scope(value.credentialRevision) &&
					token.Generation(binding.expected) >= token.Generation(value.credentialRevision)) {
				return fmt.Errorf(
					"Realm credential operation %q revision-conflict outcome is impossible",
					operationID,
				)
			}
		default:
			return fmt.Errorf("Realm credential operation %q outcome is unrecognized", operationID)
		}
	}

	if token.Generation(value.credentialRevision)-1 != uint64(len(changed)) {
		return fmt.Errorf(
			"changed credential operation count %d does not match current generation %d",
			len(changed),
			token.Generation(value.credentialRevision),
		)
	}
	return validateCredentialHistoryGraph(value, changed, observations)
}

func credentialManagerBindingExists(
	value normalizedRealm,
	actor values.PrincipalRef,
	kind credentialOperationKind,
) bool {
	permission := contract.PermissionRotateOwnCredential
	if kind == credentialOperationRevoke {
		permission = contract.PermissionRevokeCredential
	} else if kind != credentialOperationRotate {
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

func validStoredCredentialOperationBinding(binding credentialOperationBinding) bool {
	if !binding.actor.Valid() || !validCredentialID(binding.target) || !token.Valid(binding.expected) {
		return false
	}
	switch binding.kind {
	case credentialOperationRotate:
		return validCredentialID(binding.replacement)
	case credentialOperationRevoke:
		return binding.replacement == (contract.CredentialID{}) &&
			binding.replacementVerifier == ([32]byte{})
	default:
		return false
	}
}

func validateCredentialHistoryGraph(
	value normalizedRealm,
	changed map[uint64]credentialHistoryOperation,
	observations map[uint64][]credentialHistoryOperation,
) error {
	replacements := make(map[contract.CredentialID]struct{})
	for _, operation := range changed {
		if operation.binding.kind != credentialOperationRotate {
			continue
		}
		if _, duplicate := replacements[operation.binding.replacement]; duplicate {
			return fmt.Errorf(
				"credential %x has more than one replacement activation",
				operation.binding.replacement,
			)
		}
		replacements[operation.binding.replacement] = struct{}{}
	}

	graph := make(map[contract.CredentialID]credentialRecord, len(value.credentials))
	for id, current := range value.credentials {
		if _, replacement := replacements[id]; replacement {
			continue
		}
		bootstrap := current
		bootstrap.active = true
		if !current.active {
			bootstrap.verifier = [32]byte{}
		}
		graph[id] = bootstrap
	}

	for generation := uint64(1); generation <= token.Generation(value.credentialRevision); generation++ {
		if err := validateCredentialObservations(generation, graph, observations[generation]); err != nil {
			return err
		}
		if generation == token.Generation(value.credentialRevision) {
			break
		}
		operation, exists := changed[generation+1]
		if !exists {
			return fmt.Errorf("missing changed credential generation %d", generation+1)
		}
		if err := applyCredentialHistoryChange(generation+1, graph, operation); err != nil {
			return err
		}
	}

	if len(graph) != len(value.credentials) {
		return errors.New("credential history claim set contradicts current credential set")
	}
	for id, current := range value.credentials {
		reconstructed, exists := graph[id]
		if !exists || reconstructed != current {
			return fmt.Errorf("credential %x history contradicts current state", id)
		}
	}
	return nil
}

func validateCredentialObservations(
	generation uint64,
	graph map[contract.CredentialID]credentialRecord,
	operations []credentialHistoryOperation,
) error {
	for _, operation := range operations {
		binding := operation.binding
		switch operation.outcome.kind {
		case credentialOutcomeSuccess:
			record, claimed := graph[binding.target]
			if !claimed || record.active {
				return fmt.Errorf(
					"Realm credential operation %q no-op contradicts generation %d",
					operation.id,
					generation,
				)
			}
		case credentialOutcomeNotFound:
			if _, claimed := graph[binding.target]; claimed {
				return fmt.Errorf(
					"Realm credential operation %q not-found contradicts generation %d",
					operation.id,
					generation,
				)
			}
		case credentialOutcomeConflict:
			old, oldClaimed := graph[binding.target]
			_, replacementClaimed := graph[binding.replacement]
			if !oldClaimed || !old.active || !replacementClaimed {
				return fmt.Errorf(
					"Realm credential operation %q conflict contradicts generation %d",
					operation.id,
					generation,
				)
			}
		default:
			return fmt.Errorf(
				"Realm credential operation %q has an invalid state observation",
				operation.id,
			)
		}
	}
	return nil
}

func applyCredentialHistoryChange(
	generation uint64,
	graph map[contract.CredentialID]credentialRecord,
	operation credentialHistoryOperation,
) error {
	binding := operation.binding
	switch binding.kind {
	case credentialOperationRotate:
		old, claimed := graph[binding.target]
		if !claimed || !old.active || old.principal != binding.actor.Principal {
			return fmt.Errorf(
				"Realm credential operation %q rotation source is not active at generation %d",
				operation.id,
				generation-1,
			)
		}
		if binding.target == binding.replacement {
			return fmt.Errorf("Realm credential operation %q rotates to the same ID", operation.id)
		}
		if _, claimed := graph[binding.replacement]; claimed {
			return fmt.Errorf(
				"Realm credential operation %q replacement was already claimed at generation %d",
				operation.id,
				generation-1,
			)
		}
		graph[binding.target] = credentialRecord{principal: old.principal}
		graph[binding.replacement] = credentialRecord{
			principal: old.principal,
			verifier:  binding.replacementVerifier,
			active:    true,
		}
	case credentialOperationRevoke:
		target, claimed := graph[binding.target]
		if !claimed || !target.active {
			return fmt.Errorf(
				"Realm credential operation %q revoke target is not active at generation %d",
				operation.id,
				generation-1,
			)
		}
		graph[binding.target] = credentialRecord{principal: target.principal}
	default:
		return fmt.Errorf("changed credential generation %d has invalid kind", generation)
	}
	return nil
}
