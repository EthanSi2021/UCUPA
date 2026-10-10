package commit

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"go.etcd.io/bbolt"
	"os"
	"sort"
	"time"
	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	commitbinding "ucupa/ucupa-core/internal/commit/binding"
	elementfact "ucupa/ucupa-core/internal/element/fact"
	storebytes "ucupa/ucupa-core/internal/store"
	storerecord "ucupa/ucupa-core/internal/store/record"
	transferencoding "ucupa/ucupa-core/internal/transfer/encoding"
)

var (
	elementBucketName    = []byte(storebytes.ElementBucket)
	operationBucketName  = []byte(storebytes.OperationBucket)
	commitBucketName     = []byte(storebytes.CommitBucket)
	mountEventBucketName = []byte(storebytes.MountEventBucket)
	formatMarkerKey      = []byte{storebytes.FormatMarkerTag}
	tokenScopeKey        = []byte{storebytes.TokenScopeTag}
	currentCutKey        = []byte{storebytes.CurrentCutTag}
	revisionSerialKey    = []byte{storebytes.RevisionSerialTag}
	authorityRefKey      = []byte{storebytes.AuthorityRefTag}
	attachmentJournalKey = []byte{storebytes.AttachmentJournalTag}
	transferJournalKey   = []byte{storebytes.TransferJournalTag}
	publicRevisionKey    = []byte{storebytes.PublicRevisionTag}
	realmContactKey      = []byte{storebytes.RealmContactTag}
	formatMarkerValue    = []byte(storebytes.FormatMarker)
)

type localStore struct {
	db        *bbolt.DB
	scope     [32]byte
	authority values.FactAuthorityRef
	writer    writerJournal
	epoch     uint64

	// failBeforeCommit is a package-test seam. Its error is returned from the
	// transaction after the complete next snapshot has been staged, so the
	// transaction is guaranteed to roll back.
	failBeforeCommit func() error
	// failAfterCommit simulates bbolt returning a commit error after the new
	// transaction is already visible. Core must recover from Store authority
	// before it can publish either the prior or committed in-memory snapshot.
	failAfterCommit func() error
}

func initializeStore(db *bbolt.DB, state *coreState, writer writerJournal) error {
	if err := validateCoreState(state); err != nil {
		return err
	}
	return db.Update(func(tx *bbolt.Tx) error {
		buckets, err := storebytes.PrepareCreate(tx)
		if err != nil {
			return err
		}
		if err := writeStoredState(buckets.Elements, buckets.Operations, buckets.Commits, buckets.MountEvents, state); err != nil {
			return err
		}
		return putWriter(tx, writer)
	})
}

// persist replaces the complete private Store projection of next in one
// transaction. Core calls it while holding its write lock. Every returned
// error is reconciled against Store authority before any later call can use an
// in-memory snapshot, because a commit-response error can expose either side.
func (store *localStore) persist(next *coreState) error {
	if store == nil || store.db == nil || next == nil || next.scope != store.scope || next.authority != store.authority {
		return WrapStoreError("persist", errors.New("Store is closed or state scope is invalid"))
	}
	if err := validateCoreState(next); err != nil {
		return WrapStoreError("validate next state", err)
	}
	err := store.db.Update(func(tx *bbolt.Tx) error {
		if err := store.writerGate(tx); err != nil {
			return err
		}
		buckets, err := storebytes.PrepareReplace(tx, storebytes.Identity{Authority: store.authority, Scope: store.scope, PublicKey: next.publicKey})
		if err != nil {
			return err
		}
		if err := writeStoredState(buckets.Elements, buckets.Operations, buckets.Commits, buckets.MountEvents, next); err != nil {
			return err
		}
		if err := putWriter(tx, store.writer); err != nil {
			return err
		}
		if store.failBeforeCommit != nil {
			fail := store.failBeforeCommit
			store.failBeforeCommit = nil
			if err := fail(); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return WrapStoreError("persist", err)
	}
	if store.failAfterCommit != nil {
		fail := store.failAfterCommit
		store.failAfterCommit = nil
		if err := fail(); err != nil {
			return WrapStoreError("persist response", err)
		}
	}
	return nil
}

func (store *localStore) reload() (*coreState, error) {
	if store == nil || store.db == nil {
		return nil, WrapStoreError("reload", errors.New("Store is closed"))
	}
	state, err := loadStoredCore(store.db)
	if err != nil {
		return nil, WrapStoreError("reload", err)
	}
	if state.scope != store.scope || state.authority != store.authority {
		return nil, WrapStoreError("reload", errors.New("Store identity or scope changed"))
	}
	return state, nil
}

// Store encoders only put existing canonical bytes. The control path retains
// commit/rollback; neither these encoders nor the Store receives coreState via
// an arbitrary transaction callback.
type storeByteSink interface {
	Put(key, value []byte) error
}

func writeStoredState(elements, operations, commits, mounts storeByteSink, state *coreState) error {
	encoded, err := encodeRealmContact(state.realmContact, state.authority, state.scope)
	if err != nil {
		return err
	}
	if err := elements.Put(realmContactKey, encoded); err != nil {
		return err
	}
	transferJournal, err := transferencoding.EncodeTransferJournal(state.transfers.events)
	if err != nil {
		return err
	}
	if err := elements.Put(transferJournalKey, transferJournal); err != nil {
		return err
	}
	if err := writeAttachmentJournal(elements, state.attachments.events); err != nil {
		return err
	}
	if err := elements.Put(formatMarkerKey, formatMarkerValue); err != nil {
		return err
	}
	if err := elements.Put(tokenScopeKey, state.scope[:]); err != nil {
		return err
	}
	if state.publicKey == ([32]byte{}) {
		return errors.New("public Revision key is missing")
	}
	if err := elements.Put(publicRevisionKey, state.publicKey[:]); err != nil {
		return err
	}
	if err := elements.Put(authorityRefKey, []byte(state.authority)); err != nil {
		return err
	}
	if err := elements.Put(currentCutKey, storebytes.EncodeUint64(state.cutSerial)); err != nil {
		return err
	}
	if err := elements.Put(revisionSerialKey, storebytes.EncodeUint64(state.revisionSerial)); err != nil {
		return err
	}

	ids := make([]values.ElementID, 0, state.elements.Counts().Claims)
	for id := range state.elements.Claims() {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(left, right int) bool {
		return bytes.Compare(elementfact.UUIDBytes(ids[left]), elementfact.UUIDBytes(ids[right])) < 0
	})
	for _, id := range ids {
		var stored storerecord.StoredElement
		if anchor, exists := state.elements.Anchor(id); exists {
			stored = storerecord.StoreAnchor(anchor)
		} else if entity, exists := state.elements.Entity(id); exists {
			stored = storerecord.StoreEntity(entity)
		} else if revision, exists := state.elements.Tombstone(id); exists {
			stored = storerecord.StoreTombstone(revision)
		} else if departure, exists := state.elements.Departure(id); exists {
			stored = storerecord.StoreDeparture(departure)
		} else {
			return fmt.Errorf("claim %q has no record or tombstone", id)
		}
		encoded, err := json.Marshal(stored)
		if err != nil {
			return err
		}
		key, err := storebytes.ElementKey(id)
		if err != nil {
			return err
		}
		if err := elements.Put(key, encoded); err != nil {
			return err
		}
	}

	operationIDs := make([]contract.OperationID, 0, len(state.operations))
	for id := range state.operations {
		operationIDs = append(operationIDs, id)
	}
	sort.Slice(operationIDs, func(left, right int) bool {
		return bytes.Compare([]byte(operationIDs[left]), []byte(operationIDs[right])) < 0
	})
	for _, id := range operationIDs {
		key, err := encodeOperationID(id)
		if err != nil {
			return err
		}
		stored, err := storeOperation(state.operations[id])
		if err != nil {
			return err
		}
		encoded, err := json.Marshal(stored)
		if err != nil {
			return err
		}
		if err := operations.Put(key, encoded); err != nil {
			return err
		}
	}
	for index, event := range state.commits {
		stored, err := storeCommit(event)
		if err != nil {
			return err
		}
		encoded, err := json.Marshal(stored)
		if err != nil {
			return err
		}
		if err := commits.Put(storebytes.EncodeUint64(uint64(index)+2), encoded); err != nil {
			return err
		}
	}
	return writeMountEvents(mounts, state.mount.events)
}

func WrapStoreError(operation string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%w: %s: %w", contract.ErrStore, operation, err)
}

// A commit identifies exactly one decision source. Control indices refer to
// the retained control journal, including its non-cut-advancing decisions.
func storeCommit(event coreCommit) (storedCommit, error) {
	switch event.kind {
	case elementCommitted:
		if !commitbinding.ValidOperationID(event.operation) || event.controlIndex != 0 {
			return storedCommit{}, errors.New("invalid Element commit binding")
		}
		return storedCommit{Kind: "element", Operation: []byte(event.operation)}, nil
	case mountCommitted:
		if event.operation != "" || event.controlIndex == 0 {
			return storedCommit{}, errors.New("invalid control commit binding")
		}
		return storedCommit{Kind: "mount", ControlIndex: event.controlIndex}, nil
	case attachmentCommitted:
		if event.operation != "" || event.controlIndex == 0 {
			return storedCommit{}, errors.New("invalid attachment commit binding")
		}
		return storedCommit{Kind: "attachment", ControlIndex: event.controlIndex}, nil
	case transferCommitted:
		if event.operation != "" || event.controlIndex == 0 {
			return storedCommit{}, errors.New("invalid transfer commit binding")
		}
		return storedCommit{Kind: "transfer", ControlIndex: event.controlIndex}, nil
	default:
		return storedCommit{}, errors.New("unknown commit kind")
	}
}

func (stored storedCommit) commit() (coreCommit, error) {
	switch stored.Kind {
	case "element":
		if !commitbinding.ValidOperationID(contract.OperationID(string(stored.Operation))) || stored.ControlIndex != 0 {
			return coreCommit{}, errors.New("invalid stored Element commit binding")
		}
		return coreCommit{kind: elementCommitted, operation: contract.OperationID(string(stored.Operation))}, nil
	case "mount":
		if len(stored.Operation) != 0 || stored.ControlIndex == 0 {
			return coreCommit{}, errors.New("invalid stored control commit binding")
		}
		return coreCommit{kind: mountCommitted, controlIndex: stored.ControlIndex}, nil
	case "attachment":
		if len(stored.Operation) != 0 || stored.ControlIndex == 0 {
			return coreCommit{}, errors.New("invalid stored attachment commit binding")
		}
		return coreCommit{kind: attachmentCommitted, controlIndex: stored.ControlIndex}, nil
	case "transfer":
		if len(stored.Operation) != 0 || stored.ControlIndex == 0 {
			return coreCommit{}, errors.New("invalid stored transfer commit binding")
		}
		return coreCommit{kind: transferCommitted, controlIndex: stored.ControlIndex}, nil
	default:
		return coreCommit{}, errors.New("unknown stored commit kind")
	}
}

func storeOptions(
	readOnly bool,
	openFile func(string, int, os.FileMode) (*os.File, error),
) *bbolt.Options {
	return &bbolt.Options{
		Timeout: storeLockTimeout, NoGrowSync: false, NoFreelistSync: false,
		ReadOnly: readOnly, InitialMmapSize: 0, NoSync: false, OpenFile: openFile,
	}
}

func openExisting(name string, flag int, mode os.FileMode) (*os.File, error) {
	return os.OpenFile(name, flag&^os.O_CREATE, mode)
}

const storeLockTimeout = time.Second
