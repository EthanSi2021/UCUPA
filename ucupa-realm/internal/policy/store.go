package policy

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"go.etcd.io/bbolt"

	"ucupa/shared/values"
	contract "ucupa/ucupa-realm/contract"
	token "ucupa/ucupa-realm/internal/token"
)

const (
	realmStoreLockTimeout    = time.Second
	coreElementScopeTag      = byte(1)
	facetElementScopeTag     = byte(2)
	facetKeyScopeTag         = byte(3)
	realmScopeTag            = byte(4)
	anchorClosureScopeTag    = byte(5)
	authorityScopeTag        = byte(6)
	storedPutBinding         = byte(1)
	storedDeleteBinding      = byte(2)
	storedSuccess            = byte(1)
	storedNotFound           = byte(2)
	storedRevisionConflict   = byte(3)
	storedCredentialActive   = byte(1)
	storedCredentialRevoked  = byte(2)
	storedRotateCredential   = byte(1)
	storedRevokeCredential   = byte(2)
	storedCredentialConflict = byte(4)
)

var (
	metaBucketName                  = []byte("ucupa.realm.meta")
	localPrincipalBucketName        = []byte("ucupa.realm.local-principals")
	permissionBindingBucketName     = []byte("ucupa.realm.permission-bindings")
	bindingOperationBucketName      = []byte("ucupa.realm.binding-operations")
	credentialBucketName            = []byte("ucupa.realm.credentials")
	credentialOperationBucketName   = []byte("ucupa.realm.credential-operations")
	formatKey                       = []byte("format")
	realmIDKey                      = []byte("realm-id")
	revisionScopeKey                = []byte("revision-scope")
	revisionGenerationKey           = []byte("revision-generation")
	credentialRevisionScopeKey      = []byte("credential-revision-scope")
	credentialRevisionGenerationKey = []byte("credential-revision-generation")
	realmFormatID                   = []byte("ucupa.realm.local@a33aa9c69815480491bca89ce0c6d653")
	emptySetValue                   = []byte{}
)

type localStore struct {
	db              *bbolt.DB
	bindingScope    [32]byte
	credentialScope [32]byte

	// failBeforeCommit is the one definite-precommit test seam. The next
	// Realm transaction consumes it after every write has been staged.
	failBeforeCommit func() error
	// failAfterCommit simulates an indeterminate response after the transaction
	// is durable but before the in-memory snapshot can be published.
	failAfterCommit func() error
}

// CreateDurable exclusively creates one Realm-owned Store and returns its
// long-lived read-write owner.
func CreateDurable(seed contract.Seed, path string) (*Realm, error) {
	return createDurable(seed, path, nil)
}

func createDurable(seed contract.Seed, path string, failBeforeCommit func() error) (*Realm, error) {
	normalized, err := normalizeSeed(seed)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(path) == "" {
		return nil, wrapRealmStoreError("create", errors.New("Store path is empty"))
	}
	if err := initializeOwnerState(&normalized); err != nil {
		return nil, err
	}

	created := false
	options := realmStoreOptions(false, func(name string, flag int, mode os.FileMode) (*os.File, error) {
		file, openErr := os.OpenFile(name, flag|os.O_CREATE|os.O_EXCL, mode)
		if openErr == nil {
			created = true
		}
		return file, openErr
	})
	db, err := bbolt.Open(path, 0o600, options)
	if err != nil {
		var removeErr error
		if created {
			removeErr = os.Remove(path)
		}
		return nil, wrapRealmStoreError("create", errors.Join(err, removeErr))
	}
	if err := initializeRealmStore(db, normalized, failBeforeCommit); err != nil {
		closeErr := db.Close()
		removeErr := os.Remove(path)
		return nil, wrapRealmStoreError("initialize", errors.Join(err, closeErr, removeErr))
	}
	loaded, err := loadRealmStore(db)
	if err != nil || !sameNormalizedRealm(normalized, loaded) {
		if err == nil {
			err = errors.New("initialized Realm Store differs from bootstrap state")
		}
		closeErr := db.Close()
		removeErr := os.Remove(path)
		return nil, wrapRealmStoreError("validate initialized Store", errors.Join(err, closeErr, removeErr))
	}

	owner := materializeRealm(loaded)
	owner.state.store = &localStore{
		db:              db,
		bindingScope:    token.Scope(loaded.revision),
		credentialScope: token.Scope(loaded.credentialRevision),
	}
	return owner, nil
}

// OpenDurable validates an existing Store read-only, closes that handle, then
// reopens and revalidates it read-write before returning one exclusive owner.
func OpenDurable(path string) (*Realm, error) {
	if strings.TrimSpace(path) == "" {
		return nil, wrapRealmStoreError("open", errors.New("Store path is empty"))
	}

	readOnly, err := bbolt.Open(path, 0o600, realmStoreOptions(true, openExistingRealmStore))
	if err != nil {
		return nil, wrapRealmStoreError("open read-only", err)
	}
	readOnlyState, validationErr := loadRealmStore(readOnly)
	closeErr := readOnly.Close()
	if validationErr != nil || closeErr != nil {
		return nil, wrapRealmStoreError("validate read-only", errors.Join(validationErr, closeErr))
	}

	db, err := bbolt.Open(path, 0o600, realmStoreOptions(false, openExistingRealmStore))
	if err != nil {
		return nil, wrapRealmStoreError("open read-write", err)
	}
	state, validationErr := loadRealmStore(db)
	if validationErr != nil || !sameNormalizedRealm(readOnlyState, state) {
		if validationErr == nil {
			validationErr = errors.New("Realm Store changed while opening")
		}
		closeErr := db.Close()
		return nil, wrapRealmStoreError("validate read-write", errors.Join(validationErr, closeErr))
	}

	owner := materializeRealm(state)
	owner.state.store = &localStore{
		db:              db,
		bindingScope:    token.Scope(state.revision),
		credentialScope: token.Scope(state.credentialRevision),
	}
	return owner, nil
}

func (store *localStore) close() error {
	if store == nil || store.db == nil {
		return nil
	}
	db := store.db
	store.db = nil
	if err := db.Close(); err != nil {
		return wrapRealmStoreError("close", err)
	}
	return nil
}

func realmStoreOptions(
	readOnly bool,
	openFile func(string, int, os.FileMode) (*os.File, error),
) *bbolt.Options {
	return &bbolt.Options{
		Timeout:         realmStoreLockTimeout,
		NoGrowSync:      false,
		NoFreelistSync:  false,
		ReadOnly:        readOnly,
		InitialMmapSize: 0,
		MaxSize:         0,
		NoSync:          false,
		OpenFile:        openFile,
	}
}

func openExistingRealmStore(name string, flag int, mode os.FileMode) (*os.File, error) {
	return os.OpenFile(name, flag&^os.O_CREATE, mode)
}

func initializeRealmStore(
	db *bbolt.DB,
	value normalizedRealm,
	failBeforeCommit func() error,
) error {
	return db.Update(func(tx *bbolt.Tx) error {
		meta, err := tx.CreateBucket(metaBucketName)
		if err != nil {
			return err
		}
		principals, err := tx.CreateBucket(localPrincipalBucketName)
		if err != nil {
			return err
		}
		bindings, err := tx.CreateBucket(permissionBindingBucketName)
		if err != nil {
			return err
		}
		if _, err := tx.CreateBucket(bindingOperationBucketName); err != nil {
			return err
		}
		credentials, err := tx.CreateBucket(credentialBucketName)
		if err != nil {
			return err
		}
		if _, err := tx.CreateBucket(credentialOperationBucketName); err != nil {
			return err
		}
		if err := putRealmMetadata(meta, value.id, value.revision, value.credentialRevision); err != nil {
			return err
		}
		if err := putLocalPrincipals(principals, value.localPrincipals); err != nil {
			return err
		}
		if err := putPermissionBindings(bindings, value.bindings); err != nil {
			return err
		}
		if err := putCredentials(credentials, value.credentials); err != nil {
			return err
		}
		if failBeforeCommit != nil {
			return failBeforeCommit()
		}
		return nil
	})
}

func putRealmMetadata(
	meta *bbolt.Bucket,
	realmID values.RealmID,
	revision contract.BindingSetRevision,
	credentialRevision contract.CredentialSetRevision,
) error {
	if meta == nil || realmID == "" || !token.Valid(revision) || !token.Valid(credentialRevision) {
		return errors.New("Realm metadata is invalid")
	}
	if err := meta.Put(formatKey, realmFormatID); err != nil {
		return err
	}
	if err := meta.Put(realmIDKey, []byte(realmID)); err != nil {
		return err
	}
	if err := meta.Put(revisionScopeKey, token.ScopeBytes(revision)); err != nil {
		return err
	}
	var generation [8]byte
	binary.BigEndian.PutUint64(generation[:], token.Generation(revision))
	if err := meta.Put(revisionGenerationKey, generation[:]); err != nil {
		return err
	}
	if err := meta.Put(credentialRevisionScopeKey, token.ScopeBytes(credentialRevision)); err != nil {
		return err
	}
	binary.BigEndian.PutUint64(generation[:], token.Generation(credentialRevision))
	return meta.Put(credentialRevisionGenerationKey, generation[:])
}

func putLocalPrincipals(
	bucket *bbolt.Bucket,
	principals map[values.PrincipalID]struct{},
) error {
	ids := make([]string, 0, len(principals))
	for principal := range principals {
		ids = append(ids, string(principal))
	}
	sort.Strings(ids)
	for _, principal := range ids {
		if err := bucket.Put([]byte(principal), emptySetValue); err != nil {
			return err
		}
	}
	return nil
}

func putPermissionBindings(
	bucket *bbolt.Bucket,
	bindings map[bindingKey]struct{},
) error {
	encodedBindings := make([][]byte, 0, len(bindings))
	for key := range bindings {
		encoded, err := encodePermissionBinding(permissionBinding(key))
		if err != nil {
			return err
		}
		encodedBindings = append(encodedBindings, encoded)
	}
	sort.Slice(encodedBindings, func(left, right int) bool {
		return bytes.Compare(encodedBindings[left], encodedBindings[right]) < 0
	})
	for index, encoded := range encodedBindings {
		if index != 0 && bytes.Equal(encodedBindings[index-1], encoded) {
			return errors.New("duplicate encoded PermissionBinding")
		}
		if err := bucket.Put(encoded, emptySetValue); err != nil {
			return err
		}
	}
	return nil
}

func putCredentials(
	bucket *bbolt.Bucket,
	credentials map[contract.CredentialID]credentialRecord,
) error {
	if bucket == nil {
		return errors.New("credential bucket is missing")
	}
	ids := make([]contract.CredentialID, 0, len(credentials))
	for id := range credentials {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(left, right int) bool {
		return bytes.Compare(ids[left][:], ids[right][:]) < 0
	})
	for _, id := range ids {
		encodedID, err := encodeCredentialID(id)
		if err != nil {
			return err
		}
		encodedRecord, err := encodeCredentialRecord(credentials[id])
		if err != nil {
			return err
		}
		if err := bucket.Put(encodedID, encodedRecord); err != nil {
			return err
		}
	}
	return nil
}

func permissionBinding(key bindingKey) contract.PermissionBinding {
	return contract.PermissionBinding{
		Subject:    key.subject,
		Permission: key.requirement.Permission,
		Scope:      key.requirement.Scope,
	}
}

func wrapRealmStoreError(operation string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%w: %s: %w", contract.ErrStore, operation, err)
}
