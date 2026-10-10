package commit

import (
	"go.etcd.io/bbolt"
	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
)

// InstallAuthorityForTest alters only the existing durable identity header.
// It is absent from production builds; no database or writable state is returned.
func InstallAuthorityForTest(owner *Owner, replacement values.FactAuthorityRef) error {
	owner.Lock()
	defer owner.Unlock()
	return owner.store.db.Update(func(tx *bbolt.Tx) error {
		return tx.Bucket(elementBucketName).Put(authorityRefKey, []byte(replacement))
	})
}

// ReloadStateAbsentForTest preserves the original independent reload rejection
// assertion before a real Element operation attempts publication.
func ReloadStateAbsentForTest(owner *Owner) (bool, error) {
	owner.RLock()
	defer owner.RUnlock()
	state, err := owner.store.reload()
	return state == nil, err
}

// StoredAuthorityAndOperationForTest reads only the two durable observations
// required after the poisoned writer/read/retry sequence.
func StoredAuthorityAndOperationForTest(owner *Owner, operation contract.OperationID) (values.FactAuthorityRef, bool, error) {
	owner.RLock()
	defer owner.RUnlock()
	var authority values.FactAuthorityRef
	var retained bool
	err := owner.store.db.View(func(tx *bbolt.Tx) error {
		authority = values.FactAuthorityRef(string(tx.Bucket(elementBucketName).Get(authorityRefKey)))
		retained = tx.Bucket(operationBucketName).Get([]byte(operation)) != nil
		return nil
	})
	return authority, retained, err
}
