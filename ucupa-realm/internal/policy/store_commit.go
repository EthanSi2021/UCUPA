package policy

import (
	"encoding/binary"
	"errors"

	"go.etcd.io/bbolt"

	contract "ucupa/ucupa-realm/contract"
	token "ucupa/ucupa-realm/internal/token"
)

func (store *localStore) commitOperation(
	operationID contract.OperationID,
	binding operationBinding,
	state mutationState,
) (mutationPlan, storeFailure, error) {
	if store == nil || store.db == nil {
		return mutationPlan{}, storeFailureIndeterminate,
			wrapRealmStoreError("commit operation", errors.New("Store is closed"))
	}
	encodedID, err := encodeOperationID(operationID)
	if err != nil {
		return mutationPlan{}, storeFailureDefinitePrecommit,
			wrapRealmStoreError("encode operation ID", err)
	}

	tx, err := store.db.Begin(true)
	if err != nil {
		return mutationPlan{}, storeFailureDefinitePrecommit,
			wrapRealmStoreError("begin operation", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	if err := validateCommitState(tx, store.bindingScope, store.credentialScope, state, encodedID); err != nil {
		return mutationPlan{}, storeFailureIndeterminate,
			wrapRealmStoreError("validate operation state", err)
	}
	plan, err := planMutation(binding, state)
	if err != nil {
		return mutationPlan{}, storeFailureDefinitePrecommit, err
	}
	encodedOperation, err := encodeOperation(binding, plan.outcome)
	if err != nil {
		return mutationPlan{}, storeFailureDefinitePrecommit,
			wrapRealmStoreError("encode operation", err)
	}

	bindings := tx.Bucket(permissionBindingBucketName)
	operations := tx.Bucket(bindingOperationBucketName)
	meta := tx.Bucket(metaBucketName)
	if bindings == nil || operations == nil || meta == nil {
		return mutationPlan{}, storeFailureIndeterminate,
			wrapRealmStoreError("stage operation", errors.New("Realm Store bucket is missing"))
	}
	if plan.change != bindingUnchanged {
		encodedBinding, err := encodePermissionBinding(binding.target)
		if err != nil {
			return mutationPlan{}, storeFailureDefinitePrecommit,
				wrapRealmStoreError("encode target binding", err)
		}
		switch plan.change {
		case bindingPut:
			err = bindings.Put(encodedBinding, emptySetValue)
		case bindingDelete:
			err = bindings.Delete(encodedBinding)
		}
		if err != nil {
			return mutationPlan{}, storeFailureDefinitePrecommit,
				wrapRealmStoreError("stage binding change", err)
		}
		var generation [8]byte
		binary.BigEndian.PutUint64(generation[:], token.Generation(plan.outcome.revision))
		if err := meta.Put(revisionGenerationKey, generation[:]); err != nil {
			return mutationPlan{}, storeFailureDefinitePrecommit,
				wrapRealmStoreError("stage revision", err)
		}
	}
	if err := operations.Put(encodedID, encodedOperation); err != nil {
		return mutationPlan{}, storeFailureDefinitePrecommit,
			wrapRealmStoreError("stage operation outcome", err)
	}
	if store.failBeforeCommit != nil {
		fail := store.failBeforeCommit
		store.failBeforeCommit = nil
		if err := fail(); err != nil {
			return mutationPlan{}, storeFailureDefinitePrecommit,
				wrapRealmStoreError("precommit operation", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return mutationPlan{}, storeFailureIndeterminate,
			wrapRealmStoreError("commit operation", err)
	}
	committed = true
	if store.failAfterCommit != nil {
		fail := store.failAfterCommit
		store.failAfterCommit = nil
		if err := fail(); err != nil {
			return mutationPlan{}, storeFailureIndeterminate,
				wrapRealmStoreError("complete committed operation", err)
		}
	}
	return plan, storeFailureNone, nil
}

func (store *localStore) commitCredentialOperation(
	operationID contract.CredentialOperationID,
	binding credentialOperationBinding,
	state credentialMutationState,
) (credentialMutationPlan, storeFailure, error) {
	if store == nil || store.db == nil {
		return credentialMutationPlan{}, storeFailureIndeterminate,
			wrapRealmStoreError("commit credential operation", errors.New("Store is closed"))
	}
	encodedID, err := encodeCredentialOperationID(operationID)
	if err != nil {
		return credentialMutationPlan{}, storeFailureDefinitePrecommit,
			wrapRealmStoreError("encode credential operation ID", err)
	}

	tx, err := store.db.Begin(true)
	if err != nil {
		return credentialMutationPlan{}, storeFailureDefinitePrecommit,
			wrapRealmStoreError("begin credential operation", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	if err := validateCredentialCommitState(
		tx,
		store.bindingScope,
		store.credentialScope,
		state,
		encodedID,
	); err != nil {
		return credentialMutationPlan{}, storeFailureIndeterminate,
			wrapRealmStoreError("validate credential operation state", err)
	}
	plan, err := planCredentialMutation(binding, state)
	if err != nil {
		return credentialMutationPlan{}, storeFailureDefinitePrecommit, err
	}
	encodedOperation, err := encodeCredentialOperation(binding, plan.outcome)
	if err != nil {
		return credentialMutationPlan{}, storeFailureDefinitePrecommit,
			wrapRealmStoreError("encode credential operation", err)
	}

	credentials := tx.Bucket(credentialBucketName)
	operations := tx.Bucket(credentialOperationBucketName)
	meta := tx.Bucket(metaBucketName)
	if credentials == nil || operations == nil || meta == nil {
		return credentialMutationPlan{}, storeFailureIndeterminate,
			wrapRealmStoreError("stage credential operation", errors.New("Realm Store bucket is missing"))
	}
	if plan.change != credentialUnchanged {
		switch plan.change {
		case credentialRotate:
			old, exists := state.credentials[plan.target]
			if !exists || !old.active {
				return credentialMutationPlan{}, storeFailureIndeterminate,
					wrapRealmStoreError("stage credential rotation", errors.New("rotation source is missing"))
			}
			oldID, err := encodeCredentialID(plan.target)
			if err != nil {
				return credentialMutationPlan{}, storeFailureDefinitePrecommit,
					wrapRealmStoreError("encode rotation source", err)
			}
			tombstone, err := encodeCredentialRecord(credentialRecord{principal: old.principal})
			if err != nil {
				return credentialMutationPlan{}, storeFailureDefinitePrecommit,
					wrapRealmStoreError("encode rotation tombstone", err)
			}
			replacementID, err := encodeCredentialID(plan.replacement)
			if err != nil {
				return credentialMutationPlan{}, storeFailureDefinitePrecommit,
					wrapRealmStoreError("encode rotation replacement", err)
			}
			replacement, err := encodeCredentialRecord(plan.record)
			if err != nil {
				return credentialMutationPlan{}, storeFailureDefinitePrecommit,
					wrapRealmStoreError("encode rotation replacement", err)
			}
			if err := credentials.Put(oldID, tombstone); err != nil {
				return credentialMutationPlan{}, storeFailureDefinitePrecommit,
					wrapRealmStoreError("stage rotation tombstone", err)
			}
			if err := credentials.Put(replacementID, replacement); err != nil {
				return credentialMutationPlan{}, storeFailureDefinitePrecommit,
					wrapRealmStoreError("stage rotation replacement", err)
			}
		case credentialRevoke:
			targetID, err := encodeCredentialID(plan.target)
			if err != nil {
				return credentialMutationPlan{}, storeFailureDefinitePrecommit,
					wrapRealmStoreError("encode revocation target", err)
			}
			tombstone, err := encodeCredentialRecord(plan.record)
			if err != nil {
				return credentialMutationPlan{}, storeFailureDefinitePrecommit,
					wrapRealmStoreError("encode revocation tombstone", err)
			}
			if err := credentials.Put(targetID, tombstone); err != nil {
				return credentialMutationPlan{}, storeFailureDefinitePrecommit,
					wrapRealmStoreError("stage revocation tombstone", err)
			}
		default:
			return credentialMutationPlan{}, storeFailureIndeterminate,
				wrapRealmStoreError("stage credential operation", errors.New("credential change is invalid"))
		}
		var generation [8]byte
		binary.BigEndian.PutUint64(generation[:], token.Generation(plan.outcome.revision))
		if err := meta.Put(credentialRevisionGenerationKey, generation[:]); err != nil {
			return credentialMutationPlan{}, storeFailureDefinitePrecommit,
				wrapRealmStoreError("stage credential revision", err)
		}
	}
	if err := operations.Put(encodedID, encodedOperation); err != nil {
		return credentialMutationPlan{}, storeFailureDefinitePrecommit,
			wrapRealmStoreError("stage credential operation outcome", err)
	}
	if store.failBeforeCommit != nil {
		fail := store.failBeforeCommit
		store.failBeforeCommit = nil
		if err := fail(); err != nil {
			return credentialMutationPlan{}, storeFailureDefinitePrecommit,
				wrapRealmStoreError("precommit credential operation", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return credentialMutationPlan{}, storeFailureIndeterminate,
			wrapRealmStoreError("commit credential operation", err)
	}
	committed = true
	if store.failAfterCommit != nil {
		fail := store.failAfterCommit
		store.failAfterCommit = nil
		if err := fail(); err != nil {
			return credentialMutationPlan{}, storeFailureIndeterminate,
				wrapRealmStoreError("complete committed credential operation", err)
		}
	}
	return plan, storeFailureNone, nil
}

func validateCredentialCommitState(
	tx *bbolt.Tx,
	bindingScope [32]byte,
	credentialScope [32]byte,
	state credentialMutationState,
	operationID []byte,
) error {
	meta := tx.Bucket(metaBucketName)
	principals := tx.Bucket(localPrincipalBucketName)
	bindings := tx.Bucket(permissionBindingBucketName)
	bindingOperations := tx.Bucket(bindingOperationBucketName)
	credentials := tx.Bucket(credentialBucketName)
	operations := tx.Bucket(credentialOperationBucketName)
	if meta == nil || principals == nil || bindings == nil || bindingOperations == nil ||
		credentials == nil || operations == nil {
		return errors.New("Realm Store bucket is missing")
	}
	id, bindingRevision, credentialRevision, err := readRealmMetadata(meta)
	if err != nil {
		return err
	}
	if id != state.id || bindingRevision != state.bindingRevision ||
		token.Scope(bindingRevision) != bindingScope || credentialRevision != state.revision ||
		token.Scope(credentialRevision) != credentialScope {
		return errors.New("Realm Store state differs from serving snapshot")
	}
	if operations.Get(operationID) != nil {
		return errors.New("Realm credential operation exists outside serving snapshot")
	}
	return nil
}

func validateCommitState(
	tx *bbolt.Tx,
	bindingScope [32]byte,
	credentialScope [32]byte,
	state mutationState,
	operationID []byte,
) error {
	meta := tx.Bucket(metaBucketName)
	principals := tx.Bucket(localPrincipalBucketName)
	bindings := tx.Bucket(permissionBindingBucketName)
	operations := tx.Bucket(bindingOperationBucketName)
	credentials := tx.Bucket(credentialBucketName)
	credentialOperations := tx.Bucket(credentialOperationBucketName)
	if meta == nil || principals == nil || bindings == nil || operations == nil ||
		credentials == nil || credentialOperations == nil {
		return errors.New("Realm Store bucket is missing")
	}
	id, revision, credentialRevision, err := readRealmMetadata(meta)
	if err != nil {
		return err
	}
	if id != state.id || revision != state.revision || token.Scope(revision) != bindingScope ||
		token.Scope(credentialRevision) != credentialScope {
		return errors.New("Realm Store state differs from serving snapshot")
	}
	if operations.Get(operationID) != nil {
		return errors.New("Realm operation exists outside serving snapshot")
	}
	return nil
}
