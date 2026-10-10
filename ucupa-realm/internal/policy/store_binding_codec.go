package policy

import (
	"bytes"
	"encoding/binary"
	"errors"

	"go.etcd.io/bbolt"

	"ucupa/shared/values"
	contract "ucupa/ucupa-realm/contract"
	token "ucupa/ucupa-realm/internal/token"
)

func encodeOperationID(operationID contract.OperationID) ([]byte, error) {
	encoded := []byte(operationID)
	if len(encoded) == 0 {
		return nil, errors.New("OperationID is empty")
	}
	if len(encoded) > bbolt.MaxKeySize {
		return nil, errors.New("OperationID exceeds Store key capacity")
	}
	return bytes.Clone(encoded), nil
}

func encodePermissionBinding(binding contract.PermissionBinding) ([]byte, error) {
	requirement := values.Requirement{Permission: binding.Permission, Scope: binding.Scope}
	if !binding.Subject.Valid() || !requirement.Valid() {
		return nil, errors.New("PermissionBinding is invalid")
	}

	encoded := make([]byte, 0, 96)
	var err error
	encoded, err = appendRealmField(encoded, string(binding.Subject.Realm))
	if err != nil {
		return nil, err
	}
	encoded, err = appendRealmField(encoded, string(binding.Subject.Principal))
	if err != nil {
		return nil, err
	}
	encoded, err = appendRealmField(encoded, string(binding.Permission))
	if err != nil {
		return nil, err
	}

	if owner, ok := binding.Scope.AuthorityTarget(); ok {
		encoded = append(encoded, authorityScopeTag)
		return appendRealmField(encoded, string(owner))
	}
	if targetCore, element, ok := binding.Scope.CoreElementTarget(); ok {
		encoded = append(encoded, coreElementScopeTag)
		encoded, err = appendRealmField(encoded, string(targetCore))
		if err != nil {
			return nil, err
		}
		return appendRealmField(encoded, string(element))
	}
	if owner, anchor, ok := binding.Scope.AnchorClosureTarget(); ok {
		encoded = append(encoded, anchorClosureScopeTag)
		encoded, err = appendRealmField(encoded, string(owner))
		if err != nil {
			return nil, err
		}
		return appendRealmField(encoded, string(anchor))
	}
	if owner, element, ok := binding.Scope.FacetElementTarget(); ok {
		encoded = append(encoded, facetElementScopeTag)
		encoded, err = appendRealmField(encoded, string(owner))
		if err != nil {
			return nil, err
		}
		return appendRealmField(encoded, string(element))
	}
	if owner, key, ok := binding.Scope.FacetKeyTarget(); ok {
		encoded = append(encoded, facetKeyScopeTag)
		encoded, err = appendRealmField(encoded, string(owner))
		if err != nil {
			return nil, err
		}
		encoded, err = appendRealmField(encoded, string(key.Element))
		if err != nil {
			return nil, err
		}
		return appendRealmBytes(encoded, key.Type.Bytes())
	}
	if realmID, ok := binding.Scope.RealmTarget(); ok {
		encoded = append(encoded, realmScopeTag)
		return appendRealmField(encoded, string(realmID))
	}
	return nil, errors.New("PermissionBinding scope is invalid")
}

func decodePermissionBinding(encoded []byte) (contract.PermissionBinding, error) {
	decoder := realmDecoder{encoded: encoded}
	subjectRealm, err := decoder.readField()
	if err != nil {
		return contract.PermissionBinding{}, err
	}
	subjectPrincipal, err := decoder.readField()
	if err != nil {
		return contract.PermissionBinding{}, err
	}
	permission, err := decoder.readField()
	if err != nil {
		return contract.PermissionBinding{}, err
	}
	scopeTag, err := decoder.readByte()
	if err != nil {
		return contract.PermissionBinding{}, err
	}
	first, err := decoder.readField()
	if err != nil {
		return contract.PermissionBinding{}, err
	}

	binding := contract.PermissionBinding{
		Subject: values.PrincipalRef{
			Realm:     values.RealmID(copyString(subjectRealm)),
			Principal: values.PrincipalID(copyString(subjectPrincipal)),
		},
		Permission: values.PermissionID(copyString(permission)),
	}
	switch scopeTag {
	case authorityScopeTag:
		binding.Scope = values.AuthorityScope(values.FactAuthorityRef(copyString(first)))
	case anchorClosureScopeTag:
		anchor, err := decoder.readField()
		if err != nil {
			return contract.PermissionBinding{}, err
		}
		binding.Scope = values.AnchorClosureScope(values.FactAuthorityRef(copyString(first)), values.ElementID(copyString(anchor)))
	case coreElementScopeTag:
		element, err := decoder.readField()
		if err != nil {
			return contract.PermissionBinding{}, err
		}
		binding.Scope = values.CoreElementScope(
			values.FactAuthorityRef(copyString(first)),
			values.ElementID(copyString(element)),
		)
	case facetElementScopeTag:
		element, err := decoder.readField()
		if err != nil {
			return contract.PermissionBinding{}, err
		}
		binding.Scope = values.FacetElementScope(values.FactAuthorityRef(copyString(first)), values.ElementID(copyString(element)))
	case facetKeyScopeTag:
		element, err := decoder.readField()
		if err != nil {
			return contract.PermissionBinding{}, err
		}
		facetType, err := decoder.readField()
		if err != nil {
			return contract.PermissionBinding{}, err
		}
		typeRef, err := values.NewFacetTypeRef(facetType)
		if err != nil {
			return contract.PermissionBinding{}, err
		}
		binding.Scope = values.FacetKeyScope(
			values.FactAuthorityRef(copyString(first)),
			values.ElementID(copyString(element)),
			typeRef,
		)
	case realmScopeTag:
		binding.Scope = values.RealmScope(values.RealmID(copyString(first)))
	default:
		return contract.PermissionBinding{}, errors.New("PermissionBinding scope tag is unrecognized")
	}
	if !decoder.done() {
		return contract.PermissionBinding{}, errors.New("PermissionBinding has trailing bytes")
	}
	canonical, err := encodePermissionBinding(binding)
	if err != nil {
		return contract.PermissionBinding{}, err
	}
	if !bytes.Equal(canonical, encoded) {
		return contract.PermissionBinding{}, errors.New("PermissionBinding encoding is noncanonical")
	}
	return binding, nil
}

func encodeOperation(binding operationBinding, outcome retainedOutcome) ([]byte, error) {
	if !binding.actor.Valid() || !validOnlineTarget(binding.target) || !token.Valid(binding.expected) {
		return nil, errors.New("Realm operation binding is invalid")
	}
	encoded := make([]byte, 0, 256)
	switch binding.kind {
	case operationPutBinding:
		encoded = append(encoded, storedPutBinding)
	case operationDeleteBinding:
		encoded = append(encoded, storedDeleteBinding)
	default:
		return nil, errors.New("Realm operation kind is invalid")
	}
	var err error
	encoded, err = appendRealmField(encoded, string(binding.actor.Realm))
	if err != nil {
		return nil, err
	}
	encoded, err = appendRealmField(encoded, string(binding.actor.Principal))
	if err != nil {
		return nil, err
	}
	target, err := encodePermissionBinding(binding.target)
	if err != nil {
		return nil, err
	}
	encoded, err = appendRealmBytes(encoded, target)
	if err != nil {
		return nil, err
	}
	encoded = appendRevision(encoded, binding.expected)
	switch outcome.kind {
	case outcomeSuccess:
		if !token.Valid(outcome.revision) {
			return nil, errors.New("Realm success outcome revision is invalid")
		}
		encoded = append(encoded, storedSuccess)
		encoded = appendRevision(encoded, outcome.revision)
	case outcomeNotFound:
		if outcome.revision != (contract.BindingSetRevision{}) {
			return nil, errors.New("Realm not-found outcome contains a revision")
		}
		encoded = append(encoded, storedNotFound)
	case outcomeRevisionConflict:
		if outcome.revision != (contract.BindingSetRevision{}) {
			return nil, errors.New("Realm revision-conflict outcome contains a revision")
		}
		encoded = append(encoded, storedRevisionConflict)
	default:
		return nil, errors.New("Realm operation outcome is invalid")
	}
	if len(encoded) > bbolt.MaxValueSize {
		return nil, errors.New("Realm operation exceeds Store value capacity")
	}
	return encoded, nil
}

func decodeOperation(encoded []byte) (operationBinding, retainedOutcome, error) {
	decoder := realmDecoder{encoded: encoded}
	kindTag, err := decoder.readByte()
	if err != nil {
		return operationBinding{}, retainedOutcome{}, err
	}
	actorRealm, err := decoder.readField()
	if err != nil {
		return operationBinding{}, retainedOutcome{}, err
	}
	actorPrincipal, err := decoder.readField()
	if err != nil {
		return operationBinding{}, retainedOutcome{}, err
	}
	encodedTarget, err := decoder.readField()
	if err != nil {
		return operationBinding{}, retainedOutcome{}, err
	}
	target, err := decodePermissionBinding(encodedTarget)
	if err != nil {
		return operationBinding{}, retainedOutcome{}, err
	}
	expected, err := decoder.readRevision()
	if err != nil {
		return operationBinding{}, retainedOutcome{}, err
	}
	outcomeTag, err := decoder.readByte()
	if err != nil {
		return operationBinding{}, retainedOutcome{}, err
	}
	binding := operationBinding{
		actor: values.PrincipalRef{
			Realm:     values.RealmID(copyString(actorRealm)),
			Principal: values.PrincipalID(copyString(actorPrincipal)),
		},
		target:   target,
		expected: expected,
	}
	switch kindTag {
	case storedPutBinding:
		binding.kind = operationPutBinding
	case storedDeleteBinding:
		binding.kind = operationDeleteBinding
	default:
		return operationBinding{}, retainedOutcome{}, errors.New("Realm operation kind is unrecognized")
	}
	var outcome retainedOutcome
	switch outcomeTag {
	case storedSuccess:
		outcome.kind = outcomeSuccess
		outcome.revision, err = decoder.readRevision()
		if err != nil {
			return operationBinding{}, retainedOutcome{}, err
		}
	case storedNotFound:
		outcome.kind = outcomeNotFound
	case storedRevisionConflict:
		outcome.kind = outcomeRevisionConflict
	default:
		return operationBinding{}, retainedOutcome{}, errors.New("Realm operation outcome is unrecognized")
	}
	if !decoder.done() {
		return operationBinding{}, retainedOutcome{}, errors.New("Realm operation has trailing bytes")
	}
	canonical, err := encodeOperation(binding, outcome)
	if err != nil {
		return operationBinding{}, retainedOutcome{}, err
	}
	if !bytes.Equal(canonical, encoded) {
		return operationBinding{}, retainedOutcome{}, errors.New("Realm operation encoding is noncanonical")
	}
	return binding, outcome, nil
}

func appendRevision(encoded []byte, revision contract.BindingSetRevision) []byte {
	encoded = append(encoded, token.ScopeBytes(revision)...)
	return binary.BigEndian.AppendUint64(encoded, token.Generation(revision))
}
