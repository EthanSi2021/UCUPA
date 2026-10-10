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

func encodeCredentialOperationID(operationID contract.CredentialOperationID) ([]byte, error) {
	encoded := []byte(operationID)
	if len(encoded) == 0 {
		return nil, errors.New("CredentialOperationID is empty")
	}
	if len(encoded) > bbolt.MaxKeySize {
		return nil, errors.New("CredentialOperationID exceeds Store key capacity")
	}
	return bytes.Clone(encoded), nil
}

func encodeCredentialID(id contract.CredentialID) ([]byte, error) {
	if !validCredentialID(id) {
		return nil, errors.New("CredentialID is invalid")
	}
	return bytes.Clone(id[:]), nil
}

func decodeCredentialID(encoded []byte) (contract.CredentialID, error) {
	if len(encoded) != len(contract.CredentialID{}) {
		return contract.CredentialID{}, errors.New("CredentialID length is invalid")
	}
	var id contract.CredentialID
	copy(id[:], encoded)
	if !validCredentialID(id) {
		return contract.CredentialID{}, errors.New("CredentialID is invalid")
	}
	return id, nil
}

func encodeCredentialRecord(record credentialRecord) ([]byte, error) {
	if record.principal == "" {
		return nil, errors.New("credential PrincipalID is invalid")
	}
	encoded := make([]byte, 0, 1+4+len(record.principal)+len(record.verifier))
	if record.active {
		encoded = append(encoded, storedCredentialActive)
	} else {
		if record.verifier != ([32]byte{}) {
			return nil, errors.New("revoked credential retains a verifier")
		}
		encoded = append(encoded, storedCredentialRevoked)
	}
	var err error
	encoded, err = appendRealmField(encoded, string(record.principal))
	if err != nil {
		return nil, err
	}
	if record.active {
		encoded = append(encoded, record.verifier[:]...)
	}
	if len(encoded) > bbolt.MaxValueSize {
		return nil, errors.New("credential record exceeds Store value capacity")
	}
	return encoded, nil
}

func decodeCredentialRecord(encoded []byte) (credentialRecord, error) {
	decoder := realmDecoder{encoded: encoded}
	stateTag, err := decoder.readByte()
	if err != nil {
		return credentialRecord{}, err
	}
	principal, err := decoder.readField()
	if err != nil {
		return credentialRecord{}, err
	}
	record := credentialRecord{principal: values.PrincipalID(copyString(principal))}
	switch stateTag {
	case storedCredentialActive:
		verifier, err := decoder.read(len(record.verifier))
		if err != nil {
			return credentialRecord{}, err
		}
		copy(record.verifier[:], verifier)
		record.active = true
	case storedCredentialRevoked:
	case 0:
		return credentialRecord{}, errors.New("credential state tag is invalid")
	default:
		return credentialRecord{}, errors.New("credential state tag is unrecognized")
	}
	if !decoder.done() {
		return credentialRecord{}, errors.New("credential record has trailing bytes")
	}
	canonical, err := encodeCredentialRecord(record)
	if err != nil {
		return credentialRecord{}, err
	}
	if !bytes.Equal(canonical, encoded) {
		return credentialRecord{}, errors.New("credential record encoding is noncanonical")
	}
	return record, nil
}

func encodeCredentialOperation(
	binding credentialOperationBinding,
	outcome retainedCredentialOutcome,
) ([]byte, error) {
	if !validStoredCredentialOperationBinding(binding) {
		return nil, errors.New("Realm credential operation binding is invalid")
	}
	if (binding.kind == credentialOperationRotate && outcome.kind == credentialOutcomeNotFound) ||
		(binding.kind == credentialOperationRevoke && outcome.kind == credentialOutcomeConflict) {
		return nil, errors.New("Realm credential operation outcome does not match its kind")
	}

	encoded := make([]byte, 0, 192)
	switch binding.kind {
	case credentialOperationRotate:
		encoded = append(encoded, storedRotateCredential)
	case credentialOperationRevoke:
		encoded = append(encoded, storedRevokeCredential)
	default:
		return nil, errors.New("Realm credential operation kind is invalid")
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
	encoded = append(encoded, binding.target[:]...)
	if binding.kind == credentialOperationRotate {
		encoded = append(encoded, binding.replacement[:]...)
		encoded = append(encoded, binding.replacementVerifier[:]...)
	}
	encoded = appendCredentialRevision(encoded, binding.expected)

	switch outcome.kind {
	case credentialOutcomeSuccess:
		if !token.Valid(outcome.revision) {
			return nil, errors.New("Realm credential success outcome revision is invalid")
		}
		encoded = append(encoded, storedSuccess)
		encoded = appendCredentialRevision(encoded, outcome.revision)
	case credentialOutcomeNotFound:
		if outcome.revision != (contract.CredentialSetRevision{}) {
			return nil, errors.New("Realm credential not-found outcome contains a revision")
		}
		encoded = append(encoded, storedNotFound)
	case credentialOutcomeRevisionConflict:
		if outcome.revision != (contract.CredentialSetRevision{}) {
			return nil, errors.New("Realm credential revision-conflict outcome contains a revision")
		}
		encoded = append(encoded, storedRevisionConflict)
	case credentialOutcomeConflict:
		if outcome.revision != (contract.CredentialSetRevision{}) {
			return nil, errors.New("Realm credential conflict outcome contains a revision")
		}
		encoded = append(encoded, storedCredentialConflict)
	default:
		return nil, errors.New("Realm credential operation outcome is invalid")
	}
	if len(encoded) > bbolt.MaxValueSize {
		return nil, errors.New("Realm credential operation exceeds Store value capacity")
	}
	return encoded, nil
}

func decodeCredentialOperation(
	encoded []byte,
) (credentialOperationBinding, retainedCredentialOutcome, error) {
	decoder := realmDecoder{encoded: encoded}
	kindTag, err := decoder.readByte()
	if err != nil {
		return credentialOperationBinding{}, retainedCredentialOutcome{}, err
	}
	actorRealm, err := decoder.readField()
	if err != nil {
		return credentialOperationBinding{}, retainedCredentialOutcome{}, err
	}
	actorPrincipal, err := decoder.readField()
	if err != nil {
		return credentialOperationBinding{}, retainedCredentialOutcome{}, err
	}
	target, err := decoder.readCredentialID()
	if err != nil {
		return credentialOperationBinding{}, retainedCredentialOutcome{}, err
	}
	binding := credentialOperationBinding{
		actor: values.PrincipalRef{
			Realm:     values.RealmID(copyString(actorRealm)),
			Principal: values.PrincipalID(copyString(actorPrincipal)),
		},
		target: target,
	}
	switch kindTag {
	case storedRotateCredential:
		binding.kind = credentialOperationRotate
		binding.replacement, err = decoder.readCredentialID()
		if err != nil {
			return credentialOperationBinding{}, retainedCredentialOutcome{}, err
		}
		verifier, err := decoder.read(len(binding.replacementVerifier))
		if err != nil {
			return credentialOperationBinding{}, retainedCredentialOutcome{}, err
		}
		copy(binding.replacementVerifier[:], verifier)
	case storedRevokeCredential:
		binding.kind = credentialOperationRevoke
	default:
		return credentialOperationBinding{}, retainedCredentialOutcome{},
			errors.New("Realm credential operation kind is unrecognized")
	}
	binding.expected, err = decoder.readCredentialRevision()
	if err != nil {
		return credentialOperationBinding{}, retainedCredentialOutcome{}, err
	}
	outcomeTag, err := decoder.readByte()
	if err != nil {
		return credentialOperationBinding{}, retainedCredentialOutcome{}, err
	}
	var outcome retainedCredentialOutcome
	switch outcomeTag {
	case storedSuccess:
		outcome.kind = credentialOutcomeSuccess
		outcome.revision, err = decoder.readCredentialRevision()
		if err != nil {
			return credentialOperationBinding{}, retainedCredentialOutcome{}, err
		}
	case storedNotFound:
		outcome.kind = credentialOutcomeNotFound
	case storedRevisionConflict:
		outcome.kind = credentialOutcomeRevisionConflict
	case storedCredentialConflict:
		outcome.kind = credentialOutcomeConflict
	default:
		return credentialOperationBinding{}, retainedCredentialOutcome{},
			errors.New("Realm credential operation outcome is unrecognized")
	}
	if !decoder.done() {
		return credentialOperationBinding{}, retainedCredentialOutcome{},
			errors.New("Realm credential operation has trailing bytes")
	}
	canonical, err := encodeCredentialOperation(binding, outcome)
	if err != nil {
		return credentialOperationBinding{}, retainedCredentialOutcome{}, err
	}
	if !bytes.Equal(canonical, encoded) {
		return credentialOperationBinding{}, retainedCredentialOutcome{},
			errors.New("Realm credential operation encoding is noncanonical")
	}
	return binding, outcome, nil
}

func appendCredentialRevision(
	encoded []byte,
	revision contract.CredentialSetRevision,
) []byte {
	encoded = append(encoded, token.ScopeBytes(revision)...)
	return binary.BigEndian.AppendUint64(encoded, token.Generation(revision))
}
