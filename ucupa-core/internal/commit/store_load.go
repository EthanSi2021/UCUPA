package commit

import (
	"bytes"
	"errors"
	"fmt"
	"go.etcd.io/bbolt"
	"os"
	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	commitbinding "ucupa/ucupa-core/internal/commit/binding"
	storebytes "ucupa/ucupa-core/internal/store"
	storerecord "ucupa/ucupa-core/internal/store/record"
	transferencoding "ucupa/ucupa-core/internal/transfer/encoding"
)

func loadStoredCore(db *bbolt.DB) (*coreState, error) {
	var loaded *coreState
	err := db.View(func(tx *bbolt.Tx) error {
		count := 0
		if err := tx.ForEach(func(name []byte, _ *bbolt.Bucket) error {
			count++
			if !bytes.Equal(name, elementBucketName) && !bytes.Equal(name, operationBucketName) && !bytes.Equal(name, commitBucketName) && !bytes.Equal(name, mountEventBucketName) {
				return fmt.Errorf("unexpected bucket %q", name)
			}
			return nil
		}); err != nil {
			return err
		}
		if count != 4 {
			return fmt.Errorf("expected four Core buckets, found %d", count)
		}
		elements := tx.Bucket(elementBucketName)
		operations := tx.Bucket(operationBucketName)
		commits := tx.Bucket(commitBucketName)
		mounts := tx.Bucket(mountEventBucketName)
		if elements == nil || operations == nil || commits == nil || mounts == nil {
			return errors.New("Core Store bucket is missing")
		}
		if elements.Sequence() != 0 || operations.Sequence() != 0 || commits.Sequence() != 0 || mounts.Sequence() != 0 {
			return errors.New("Core Store bucket sequence is not zero")
		}
		if !bytes.Equal(elements.Get(formatMarkerKey), formatMarkerValue) {
			return errors.New("format marker is missing or unrecognized")
		}
		authority := values.FactAuthorityRef(string(elements.Get(authorityRefKey)))
		if !authority.Valid() {
			return errors.New("Core identity is missing or invalid")
		}
		if _, err := readWriter(tx, authority); err != nil {
			return err
		}
		encodedScope := elements.Get(tokenScopeKey)
		if len(encodedScope) != 32 {
			return errors.New("token scope is missing or unrecognized")
		}
		var scope [32]byte
		copy(scope[:], encodedScope)
		if scope == ([32]byte{}) {
			return errors.New("token scope is zero")
		}
		cutSerial, err := storebytes.DecodeUint64(elements.Get(currentCutKey))
		if err != nil {
			return fmt.Errorf("current cut: %w", err)
		}
		revisionSerial, err := storebytes.DecodeUint64(elements.Get(revisionSerialKey))
		if err != nil {
			return fmt.Errorf("revision serial: %w", err)
		}
		state := newCoreState(authority, scope)
		key := elements.Get(publicRevisionKey)
		if len(key) != 32 {
			return errors.New("public Revision key is missing or invalid")
		}
		copy(state.publicKey[:], key)
		if state.publicKey == ([32]byte{}) {
			return errors.New("public Revision key is zero")
		}
		state.cutSerial = cutSerial
		state.revisionSerial = revisionSerial

		metadataSeen := map[string]bool{}
		err = elements.ForEach(func(key, encoded []byte) error {
			switch {
			case bytes.Equal(key, realmContactKey):
				contact, decodeErr := decodeRealmContact(encoded, authority, scope)
				if decodeErr != nil {
					return decodeErr
				}
				state.realmContact = contact
				metadataSeen["realmContact"] = true
				return nil
			case bytes.Equal(key, transferJournalKey):
				metadataSeen["transfers"] = true
				return nil
			case bytes.Equal(key, writerJournalKey):
				metadataSeen["writer"] = true
				return nil
			case bytes.Equal(key, formatMarkerKey):
				metadataSeen["format"] = true
				return nil
			case bytes.Equal(key, tokenScopeKey):
				metadataSeen["scope"] = true
				return nil
			case bytes.Equal(key, currentCutKey):
				metadataSeen["cut"] = true
				return nil
			case bytes.Equal(key, revisionSerialKey):
				metadataSeen["revision"] = true
				return nil
			case bytes.Equal(key, authorityRefKey):
				metadataSeen["authority"] = true
				return nil
			case bytes.Equal(key, attachmentJournalKey):
				metadataSeen["attachments"] = true
				return nil
			case bytes.Equal(key, publicRevisionKey):
				metadataSeen["publicRevisionKey"] = true
				return nil
			}
			if encoded == nil {
				return fmt.Errorf("nested Element bucket %q is not recognized", key)
			}
			id, err := storebytes.DecodeElementKey(key)
			if err != nil {
				return err
			}
			var stored storerecord.StoredElement
			if err := storebytes.DecodeCanonicalJSON(encoded, &stored); err != nil {
				return fmt.Errorf("Element %q: %w", id, err)
			}
			if err := storerecord.LoadElement(&state.elements, state.scope, id, stored); err != nil {
				return fmt.Errorf("Element %q: %w", id, err)
			}
			return nil
		})
		if err != nil {
			return err
		}
		for _, required := range []string{"realmContact", "transfers", "writer", "format", "scope", "cut", "revision", "authority", "attachments", "publicRevisionKey"} {
			if !metadataSeen[required] {
				return errors.New("Core Store metadata is incomplete")
			}
		}

		err = operations.ForEach(func(key, encoded []byte) error {
			if encoded == nil {
				return fmt.Errorf("nested Operation bucket %q is not recognized", key)
			}
			operationID := contract.OperationID(string(append([]byte(nil), key...)))
			if !commitbinding.ValidOperationID(operationID) {
				return errors.New("stored OperationID is invalid")
			}
			var stored storedOperation
			if err := storebytes.DecodeCanonicalJSON(encoded, &stored); err != nil {
				return fmt.Errorf("Operation %q: %w", key, err)
			}
			operation, err := stored.operation(scope)
			if err != nil {
				return fmt.Errorf("Operation %q: %w", key, err)
			}
			state.operations[operationID] = operation
			return nil
		})
		if err != nil {
			return err
		}
		if err := commits.ForEach(func(key, encoded []byte) error {
			cut, err := storebytes.DecodeUint64(key)
			if err != nil || cut != uint64(len(state.commits))+2 || encoded == nil {
				return errors.New("commit history key is invalid or discontinuous")
			}
			var stored storedCommit
			if err := storebytes.DecodeCanonicalJSON(encoded, &stored); err != nil {
				return fmt.Errorf("commit at cut %d: %w", cut, err)
			}
			event, err := stored.commit()
			if err != nil {
				return err
			}
			state.commits = append(state.commits, event)
			return nil
		}); err != nil {
			return err
		}
		state.mount.events, err = loadMountEvents(mounts)
		if err != nil {
			return err
		}
		state.attachments.events, err = loadAttachmentJournal(elements.Get(attachmentJournalKey), scope)
		if err != nil {
			return err
		}
		state.transfers.events, err = transferencoding.LoadTransferJournal(elements.Get(transferJournalKey), scope)
		if err != nil {
			return err
		}
		replayed, err := replayCoreHistory(state)
		if err != nil {
			return err
		}
		// The control journal is the durable representation of control facts and
		// retained outcomes. Rebuild them only through the live transition rule.
		state.mount = replayed.mount
		state.attachments = replayed.attachments
		state.transfers = replayed.transfers
		if !sameCompleteCoreState(state, replayed) {
			return errors.New("Core history does not reproduce stored facts and outcomes")
		}
		if err := validateCurrentSnapshot(state); err != nil {
			return err
		}
		loaded = state
		return nil
	})
	if err != nil {
		return nil, err
	}
	return loaded, nil
}

// CreateDurable opens only a new owned Store; creation failure removes only that new file.
// The one Realm contact is part of the initial transaction: a Store without a
// bound contact is never created, and no later open can synthesize one.
func CreateDurable(path string, authority values.FactAuthorityRef, scope [32]byte, initial contract.RealmInitialization, cfg *values.WriterStartup) (*Owner, error) {
	if !cfg.Valid() || (cfg != nil && cfg.Install != nil) {
		return nil, values.PublicInvalid
	}
	contact, err := newRealmContact(initial)
	if err != nil {
		return nil, err
	}
	created := false
	options := storeOptions(false, func(name string, flag int, mode os.FileMode) (*os.File, error) {
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
		return nil, WrapStoreError("create", errors.Join(err, removeErr))
	}

	state := newCoreState(authority, scope)
	state.realmContact = contact
	j := initialWriter(authority, cfg)
	store := &localStore{db: db, scope: scope, authority: authority, writer: j, epoch: j.Epoch}
	if err := initializeStore(db, state, j); err != nil {
		closeErr := db.Close()
		removeErr := os.Remove(path)
		return nil, WrapStoreError("initialize", errors.Join(err, closeErr, removeErr))
	}
	owner := &Owner{authority: authority, state: state, store: store}
	owner.refreshContactLocked()
	return owner, nil
}

// OpenDurable validates read-only first, then revalidates the same identity in the writable Store.
func OpenDurable(path string, cfg *values.WriterStartup) (*Owner, error) {
	if !cfg.Valid() {
		return nil, values.PublicInvalid
	}
	readOnly, err := bbolt.Open(path, 0o600, storeOptions(true, openExisting))
	if err != nil {
		return nil, WrapStoreError("open read-only", err)
	}
	readOnlyState, validationErr := loadStoredCore(readOnly)
	if validationErr == nil {
		j, e := loadWriter(readOnly, readOnlyState.authority)
		validationErr = e
		if e == nil {
			validationErr = j.qualified(cfg)
		}
	}
	closeErr := readOnly.Close()
	if validationErr != nil || closeErr != nil {
		return nil, WrapStoreError("validate read-only", errors.Join(validationErr, closeErr))
	}

	db, err := bbolt.Open(path, 0o600, storeOptions(false, openExisting))
	if err != nil {
		return nil, WrapStoreError("open read-write", err)
	}
	state, validationErr := loadStoredCore(db)
	if validationErr != nil || state.scope != readOnlyState.scope || state.authority != readOnlyState.authority || state.publicKey != readOnlyState.publicKey {
		if validationErr == nil {
			validationErr = errors.New("Core Store identity changed while opening")
		}
		closeErr := db.Close()
		return nil, WrapStoreError("validate read-write", errors.Join(validationErr, closeErr))
	}
	j, e := loadWriter(db, state.authority)
	store := &localStore{db: db, scope: state.scope, authority: state.authority}
	if e == nil {
		j, e = store.installWriter(j, cfg)
	}
	if e != nil {
		_ = db.Close()
		return nil, e
	}
	store.writer, store.epoch = j, j.Epoch
	owner := &Owner{authority: state.authority, state: state, store: store}
	owner.refreshContactLocked()
	return owner, nil
}
