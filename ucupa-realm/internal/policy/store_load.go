package policy

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"

	"go.etcd.io/bbolt"

	"ucupa/shared/values"
	contract "ucupa/ucupa-realm/contract"
	token "ucupa/ucupa-realm/internal/token"
)

func loadRealmStore(db *bbolt.DB) (normalizedRealm, error) {
	var value normalizedRealm
	err := db.View(func(tx *bbolt.Tx) error {
		loaded, err := readRealmTransaction(tx)
		if err != nil {
			return err
		}
		value = loaded
		return checkRealmStoreTransaction(tx)
	})
	if err != nil {
		return normalizedRealm{}, err
	}
	return value, nil
}

func readRealmTransaction(tx *bbolt.Tx) (normalizedRealm, error) {
	bucketCount := 0
	if err := tx.ForEach(func(name []byte, _ *bbolt.Bucket) error {
		bucketCount++
		if !bytes.Equal(name, metaBucketName) &&
			!bytes.Equal(name, localPrincipalBucketName) &&
			!bytes.Equal(name, permissionBindingBucketName) &&
			!bytes.Equal(name, bindingOperationBucketName) &&
			!bytes.Equal(name, credentialBucketName) &&
			!bytes.Equal(name, credentialOperationBucketName) {
			return fmt.Errorf("unexpected Realm bucket %q", name)
		}
		return nil
	}); err != nil {
		return normalizedRealm{}, err
	}
	if bucketCount != 6 {
		return normalizedRealm{}, fmt.Errorf("expected six Realm buckets, found %d", bucketCount)
	}

	meta := tx.Bucket(metaBucketName)
	principals := tx.Bucket(localPrincipalBucketName)
	bindings := tx.Bucket(permissionBindingBucketName)
	operations := tx.Bucket(bindingOperationBucketName)
	credentials := tx.Bucket(credentialBucketName)
	credentialOperations := tx.Bucket(credentialOperationBucketName)
	if meta == nil || principals == nil || bindings == nil || operations == nil ||
		credentials == nil || credentialOperations == nil {
		return normalizedRealm{}, errors.New("Realm bucket is missing")
	}
	for _, bucket := range []*bbolt.Bucket{
		meta,
		principals,
		bindings,
		operations,
		credentials,
		credentialOperations,
	} {
		if bucket.Sequence() != 0 {
			return normalizedRealm{}, errors.New("Realm bucket has an unrecognized sequence")
		}
	}

	id, revision, credentialRevision, err := readRealmMetadata(meta)
	if err != nil {
		return normalizedRealm{}, err
	}
	seed := contract.Seed{ID: id}
	if err := principals.ForEach(func(key, value []byte) error {
		if value == nil {
			return fmt.Errorf("nested local Principal bucket %q is not recognized", key)
		}
		if len(key) == 0 || len(value) != 0 {
			return errors.New("local Principal entry is not canonical")
		}
		seed.LocalPrincipals = append(seed.LocalPrincipals, values.PrincipalID(copyString(key)))
		return nil
	}); err != nil {
		return normalizedRealm{}, err
	}
	if err := bindings.ForEach(func(key, value []byte) error {
		if value == nil {
			return fmt.Errorf("nested PermissionBinding bucket %q is not recognized", key)
		}
		if len(key) == 0 || len(value) != 0 {
			return errors.New("PermissionBinding entry is not canonical")
		}
		binding, err := decodePermissionBinding(key)
		if err != nil {
			return err
		}
		seed.Bindings = append(seed.Bindings, binding)
		return nil
	}); err != nil {
		return normalizedRealm{}, err
	}

	value, err := normalizeSeed(seed)
	if err != nil {
		return normalizedRealm{}, fmt.Errorf("stored Realm facts are invalid: %v", err)
	}
	value.revision = revision
	value.operations = make(map[contract.OperationID]retainedOperation)
	if err := operations.ForEach(func(key, encoded []byte) error {
		if encoded == nil || len(key) == 0 {
			return errors.New("Realm operation entry is not recognized")
		}
		binding, outcome, err := decodeOperation(encoded)
		if err != nil {
			return fmt.Errorf("Realm operation %q: %w", key, err)
		}
		value.operations[contract.OperationID(copyString(key))] = retainedOperation{
			binding: binding,
			outcome: outcome,
		}
		return nil
	}); err != nil {
		return normalizedRealm{}, err
	}
	if err := validateOperationHistory(value); err != nil {
		return normalizedRealm{}, err
	}

	value.credentials = make(map[contract.CredentialID]credentialRecord)
	if err := credentials.ForEach(func(key, encoded []byte) error {
		if encoded == nil {
			return fmt.Errorf("nested credential bucket %q is not recognized", key)
		}
		id, err := decodeCredentialID(key)
		if err != nil {
			return fmt.Errorf("Realm credential key %x: %w", key, err)
		}
		record, err := decodeCredentialRecord(encoded)
		if err != nil {
			return fmt.Errorf("Realm credential %x: %w", key, err)
		}
		if _, registered := value.localPrincipals[record.principal]; !registered {
			return fmt.Errorf("Realm credential %x names an unregistered local Principal", key)
		}
		value.credentials[id] = record
		return nil
	}); err != nil {
		return normalizedRealm{}, err
	}
	value.credentialRevision = credentialRevision
	value.credentialOperations = make(map[contract.CredentialOperationID]retainedCredentialOperation)
	if err := credentialOperations.ForEach(func(key, encoded []byte) error {
		if encoded == nil || len(key) == 0 {
			return errors.New("Realm credential operation entry is not recognized")
		}
		operationID := contract.CredentialOperationID(copyString(key))
		canonicalKey, err := encodeCredentialOperationID(operationID)
		if err != nil || !bytes.Equal(canonicalKey, key) {
			return errors.New("Realm credential operation key is noncanonical")
		}
		binding, outcome, err := decodeCredentialOperation(encoded)
		if err != nil {
			return fmt.Errorf("Realm credential operation %q: %w", key, err)
		}
		value.credentialOperations[operationID] = retainedCredentialOperation{
			binding: binding,
			outcome: outcome,
		}
		return nil
	}); err != nil {
		return normalizedRealm{}, err
	}
	if err := validateCredentialOperationHistory(value); err != nil {
		return normalizedRealm{}, err
	}
	return value, nil
}

func readRealmMetadata(
	meta *bbolt.Bucket,
) (values.RealmID, contract.BindingSetRevision, contract.CredentialSetRevision, error) {
	var id values.RealmID
	var revision contract.BindingSetRevision
	var credentialRevision contract.CredentialSetRevision
	seen := make(map[string]bool, 6)
	entryCount := 0
	err := meta.ForEach(func(key, value []byte) error {
		entryCount++
		if value == nil {
			return fmt.Errorf("nested Realm metadata bucket %q is not recognized", key)
		}
		name := string(key)
		if seen[name] {
			return errors.New("duplicate Realm metadata key")
		}
		seen[name] = true
		switch {
		case bytes.Equal(key, formatKey):
			if !bytes.Equal(value, realmFormatID) {
				return errors.New("Realm FormatID is missing or unrecognized")
			}
		case bytes.Equal(key, realmIDKey):
			if len(value) == 0 {
				return errors.New("RealmID is missing or invalid")
			}
			id = values.RealmID(copyString(value))
		case bytes.Equal(key, revisionScopeKey):
			if len(value) != len(token.Scope(revision)) {
				return errors.New("BindingSetRevision scope is missing or invalid")
			}
			revision = token.Binding([32]byte(value), token.Generation(revision))
		case bytes.Equal(key, revisionGenerationKey):
			if len(value) != 8 {
				return errors.New("BindingSetRevision generation is missing or invalid")
			}
			revision = token.Binding(token.Scope(revision), binary.BigEndian.Uint64(value))
		case bytes.Equal(key, credentialRevisionScopeKey):
			if len(value) != len(token.Scope(credentialRevision)) {
				return errors.New("CredentialSetRevision scope is missing or invalid")
			}
			credentialRevision = token.Credential([32]byte(value), token.Generation(credentialRevision))
		case bytes.Equal(key, credentialRevisionGenerationKey):
			if len(value) != 8 {
				return errors.New("CredentialSetRevision generation is missing or invalid")
			}
			credentialRevision = token.Credential(token.Scope(credentialRevision), binary.BigEndian.Uint64(value))
		default:
			return fmt.Errorf("unexpected Realm metadata key %q", key)
		}
		return nil
	})
	if err != nil {
		return "", contract.BindingSetRevision{}, contract.CredentialSetRevision{}, err
	}
	if entryCount != 6 || !seen[string(formatKey)] || !seen[string(realmIDKey)] ||
		!seen[string(revisionScopeKey)] || !seen[string(revisionGenerationKey)] ||
		!seen[string(credentialRevisionScopeKey)] ||
		!seen[string(credentialRevisionGenerationKey)] || id == "" ||
		!token.Valid(revision) || !token.Valid(credentialRevision) {
		return "", contract.BindingSetRevision{}, contract.CredentialSetRevision{}, errors.New("Realm metadata is incomplete")
	}
	return id, revision, credentialRevision, nil
}

func checkRealmStoreTransaction(tx *bbolt.Tx) error {
	var first error
	for err := range tx.Check() {
		if err != nil && first == nil {
			first = err
		}
	}
	if first != nil {
		return fmt.Errorf("bbolt consistency check failed: %w", first)
	}
	return nil
}

func sameNormalizedRealm(left, right normalizedRealm) bool {
	if left.id != right.id || left.revision != right.revision ||
		left.credentialRevision != right.credentialRevision ||
		len(left.localPrincipals) != len(right.localPrincipals) ||
		len(left.bindings) != len(right.bindings) ||
		len(left.operations) != len(right.operations) ||
		len(left.credentials) != len(right.credentials) ||
		len(left.credentialOperations) != len(right.credentialOperations) {
		return false
	}
	for principal := range left.localPrincipals {
		if _, exists := right.localPrincipals[principal]; !exists {
			return false
		}
	}
	for binding := range left.bindings {
		if _, exists := right.bindings[binding]; !exists {
			return false
		}
	}
	for operationID, operation := range left.operations {
		other, exists := right.operations[operationID]
		if !exists || other != operation {
			return false
		}
	}
	for id, credential := range left.credentials {
		other, exists := right.credentials[id]
		if !exists || other != credential {
			return false
		}
	}
	for operationID, operation := range left.credentialOperations {
		other, exists := right.credentialOperations[operationID]
		if !exists || other != operation {
			return false
		}
	}
	return true
}
