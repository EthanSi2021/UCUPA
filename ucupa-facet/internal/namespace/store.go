package namespace

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"
	accessstate "ucupa/ucupa-facet/internal/access"

	bolt "go.etcd.io/bbolt"

	"ucupa/shared/values"
	facetcontract "ucupa/ucupa-facet/contract"
)

var (
	recordBucketName = []byte("ucupa.facet.owner")
	formatMarkerKey  = []byte{0}
	documentKey      = []byte{1}
	formatMarker     = []byte("ucupa.facet.local@4d8e1a6f3c9b7250e6a2f8d5c1b7e3a9")
)

type localStore struct {
	db     *bolt.DB
	writer writerJournal
	epoch  uint64
	// Package-test seams, called only under the owner's commit lock. A failure
	// before commit rolls back; a failure after commit leaves the durable result
	// readable but forces the normal load/install recovery path before publish.
	failBeforeCommit func() error
	failAfterCommit  func() error
	// loseResponse is later: the owner has already published facts/observation.
	loseResponse func() error
}

// CreateDurable creates one owned Store whose initial document binds the
// generated Facet identity to the one trusted Realm contact. A Store without
// that binding is never created, and no later open can synthesize it.
func CreateDurable(port facetcontract.RealmConnections, cores facetcontract.CoreSource, initial facetcontract.RealmInitialization, path string, config *values.PublicAccessPolicy, startup *values.WriterStartup) (*Owner, error) {
	if !startup.Valid() || (startup != nil && startup.Install != nil) {
		return nil, values.PublicInvalid
	}
	if strings.TrimSpace(path) == "" {
		return nil, facetcontract.ErrInvalid
	}
	f, err := New(port, cores, initial, config)
	if err != nil {
		return nil, err
	}
	created := false
	db, err := bolt.Open(path, 0600, storeOptions(false, func(name string, flag int, mode os.FileMode) (*os.File, error) {
		file, err := os.OpenFile(name, flag|os.O_CREATE|os.O_EXCL, mode)
		if err == nil {
			created = true
		}
		return file, err
	}))
	if err != nil {
		if created {
			_ = os.Remove(path)
		}
		return nil, wrapStoreError("create", err)
	}
	raw, err := encodeDocument(storeDocument{f.authority, f.scope, f.state, f.events})
	if err == nil {
		err = db.Update(func(tx *bolt.Tx) error {
			b, err := tx.CreateBucket(recordBucketName)
			if err != nil {
				return err
			}
			if err = b.Put(formatMarkerKey, formatMarker); err != nil {
				return err
			}
			if err = b.Put(documentKey, raw); err != nil {
				return err
			}
			return putWriter(tx, initialWriter(f.authority, startup))
		})
	}
	if err != nil {
		_ = db.Close()
		_ = os.Remove(path)
		return nil, wrapStoreError("initialize", err)
	}
	j := initialWriter(f.authority, startup)
	f.store = &localStore{db: db, writer: j, epoch: j.Epoch}
	return f, nil
}

// OpenDurable reopens one existing Store. The contact is recovered from the
// original document only; assembly supplies the port but never a replacement
// contact.
func OpenDurable(port facetcontract.RealmConnections, cores facetcontract.CoreSource, path string, config *values.PublicAccessPolicy, startup *values.WriterStartup) (*Owner, error) {
	if !startup.Valid() {
		return nil, values.PublicInvalid
	}
	if nilDependency(port) || nilDependency(cores) || strings.TrimSpace(path) == "" {
		return nil, facetcontract.ErrInvalid
	}
	limits, err := values.NormalizePublicPolicy(config)
	if err != nil {
		return nil, facetcontract.ErrInvalid
	}
	ro, err := bolt.Open(path, 0600, storeOptions(true, openExisting))
	if err != nil {
		return nil, wrapStoreError("open read only", err)
	}
	before, err := loadStore(ro)
	if err == nil {
		j, e := loadWriter(ro, before.Authority)
		err = e
		if e == nil {
			err = j.qualified(startup)
		}
	}
	closeErr := ro.Close()
	if err != nil || closeErr != nil {
		return nil, wrapStoreError("validate read only", errors.Join(err, closeErr))
	}
	db, err := bolt.Open(path, 0600, storeOptions(false, openExisting))
	if err != nil {
		return nil, wrapStoreError("open", err)
	}
	after, err := loadStore(db)
	if err != nil || before.Authority != after.Authority || before.Scope != after.Scope || before.Facts.PublicKey != after.Facts.PublicKey {
		_ = db.Close()
		return nil, wrapStoreError("identity or history", errors.Join(facetcontract.ErrStore, err))
	}
	f := assemble(port, cores, after.Authority, after.Scope)
	f.install(after)
	f.public = accessstate.New(f.authority, limits, &f.mu)
	j, e := loadWriter(db, after.Authority)
	store := &localStore{db: db}
	if e == nil {
		j, e = store.installWriter(j, startup)
	}
	if e != nil {
		_ = db.Close()
		return nil, e
	}
	store.writer, store.epoch = j, j.Epoch
	f.store = store
	return f, nil
}

// OpenRealmMaintenance reopens one existing Store exclusively for Realm
// contact maintenance. It enforces the same format, lock and writer rules as
// an ordinary open, but assembles no Core source and no public surface, and it
// does not require the persisted address to be verifiable: repair authorizes
// through an explicit same-Realm candidate at control time. The returned owner
// is only complete enough for contact maintenance and Close.
func OpenRealmMaintenance(port facetcontract.RealmConnections, path string, startup *values.WriterStartup) (*Owner, error) {
	if !startup.Valid() {
		return nil, values.PublicInvalid
	}
	if nilDependency(port) || strings.TrimSpace(path) == "" {
		return nil, facetcontract.ErrInvalid
	}
	ro, err := bolt.Open(path, 0600, storeOptions(true, openExisting))
	if err != nil {
		return nil, wrapStoreError("open read only", err)
	}
	before, err := loadStore(ro)
	if err == nil {
		j, e := loadWriter(ro, before.Authority)
		err = e
		if e == nil {
			err = j.qualified(startup)
		}
	}
	closeErr := ro.Close()
	if err != nil || closeErr != nil {
		return nil, wrapStoreError("validate read only", errors.Join(err, closeErr))
	}
	db, err := bolt.Open(path, 0600, storeOptions(false, openExisting))
	if err != nil {
		return nil, wrapStoreError("open", err)
	}
	after, err := loadStore(db)
	if err != nil || before.Authority != after.Authority || before.Scope != after.Scope || before.Facts.PublicKey != after.Facts.PublicKey {
		_ = db.Close()
		return nil, wrapStoreError("identity or history", errors.Join(facetcontract.ErrStore, err))
	}
	f := assemble(port, nil, after.Authority, after.Scope)
	f.install(after)
	j, e := loadWriter(db, after.Authority)
	store := &localStore{db: db}
	if e == nil {
		j, e = store.installWriter(j, startup)
	}
	if e != nil {
		_ = db.Close()
		return nil, e
	}
	store.writer, store.epoch = j, j.Epoch
	f.store = store
	return f, nil
}

// Close retires a durable instance; in-memory Close does not discard facts.
func Close(f *Owner) error {
	if f == nil || f.ownerState == nil {
		return nil
	}
	defer f.public.DrainRetired(context.Background())
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.public != nil {
		f.public.CloseLocked()
	}
	if f.store == nil || f.store.db == nil {
		return nil
	}
	db := f.store.db
	f.store.db = nil
	return wrapStoreError("close", db.Close())
}
func storeOptions(readOnly bool, open func(string, int, os.FileMode) (*os.File, error)) *bolt.Options {
	return &bolt.Options{Timeout: time.Second, ReadOnly: readOnly, OpenFile: open}
}
func openExisting(name string, flag int, mode os.FileMode) (*os.File, error) {
	return os.OpenFile(name, flag&^os.O_CREATE, mode)
}
func wrapStoreError(op string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%w: %s: %w", facetcontract.ErrStore, op, err)
}
func readTransaction(tx *bolt.Tx) (storeDocument, error) {
	var doc storeDocument
	count := 0
	err := tx.ForEach(func(name []byte, b *bolt.Bucket) error {
		count++
		if !bytes.Equal(name, recordBucketName) || b.Sequence() != 0 {
			return facetcontract.ErrStore
		}
		n := 0
		err := b.ForEach(func(k, v []byte) error {
			n++
			if v == nil {
				return facetcontract.ErrStore
			}
			switch {
			case bytes.Equal(k, writerJournalKey):
				return nil
			case bytes.Equal(k, formatMarkerKey):
				if !bytes.Equal(v, formatMarker) {
					return facetcontract.ErrStore
				}
			case bytes.Equal(k, documentKey):
				var err error
				doc, err = decodeDocument(v)
				return err
			default:
				return facetcontract.ErrStore
			}
			return nil
		})
		if err != nil {
			return err
		}
		if n != 3 {
			return facetcontract.ErrStore
		}
		return nil
	})
	if err != nil || count != 1 || !doc.Authority.Valid() {
		return storeDocument{}, errors.Join(facetcontract.ErrStore, err)
	}
	if _, err := readWriter(tx, doc.Authority); err != nil {
		return storeDocument{}, errors.Join(facetcontract.ErrStore, err)
	}
	return doc, nil
}
func loadStore(db *bolt.DB) (storeDocument, error) {
	var doc storeDocument
	err := db.View(func(tx *bolt.Tx) error { var e error; doc, e = readTransaction(tx); return e })
	return doc, err
}
