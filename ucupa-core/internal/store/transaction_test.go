package store_test

import (
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"testing"

	"go.etcd.io/bbolt"
	"ucupa/shared/values"
	store "ucupa/ucupa-core/internal/store"
)

type bucketEntrySnapshot struct {
	Key    []byte
	Value  []byte
	Nested *bucketSnapshot
}

type bucketSnapshot struct {
	Sequence uint64
	Entries  []bucketEntrySnapshot
}

type storeSnapshot struct {
	Buckets map[string]bucketSnapshot
}

func snapshotDB(t *testing.T, db *bbolt.DB) storeSnapshot {
	t.Helper()
	snapshot := storeSnapshot{Buckets: map[string]bucketSnapshot{}}
	if err := db.View(func(tx *bbolt.Tx) error {
		return tx.ForEach(func(name []byte, bucket *bbolt.Bucket) error {
			snapshot.Buckets[string(name)] = snapshotBucket(bucket)
			return nil
		})
	}); err != nil {
		t.Fatal(err)
	}
	return snapshot
}

func snapshotBucket(bucket *bbolt.Bucket) bucketSnapshot {
	snapshot := bucketSnapshot{Sequence: bucket.Sequence()}
	_ = bucket.ForEach(func(key, value []byte) error {
		entry := bucketEntrySnapshot{Key: append([]byte(nil), key...)}
		if value == nil {
			nested := bucket.Bucket(key)
			if nested != nil {
				child := snapshotBucket(nested)
				entry.Nested = &child
			}
		} else {
			entry.Value = append([]byte(nil), value...)
		}
		snapshot.Entries = append(snapshot.Entries, entry)
		return nil
	})
	return snapshot
}

func newStoreDB(t *testing.T) *bbolt.DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "core.db")
	db, err := bbolt.Open(path, 0600, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func newIdentity(t *testing.T) store.Identity {
	t.Helper()
	authority, err := values.NewFactAuthorityRef()
	if err != nil {
		t.Fatal(err)
	}
	var scope, publicKey [32]byte
	for index := range scope {
		scope[index] = byte(index + 1)
		publicKey[index] = byte(0xa0 + index)
	}
	return store.Identity{Authority: authority, Scope: scope, PublicKey: publicKey}
}

func seedStore(t *testing.T, db *bbolt.DB, identity store.Identity) {
	t.Helper()
	if err := db.Update(func(tx *bbolt.Tx) error {
		elements, err := tx.CreateBucket([]byte(store.ElementBucket))
		if err != nil {
			return err
		}
		for _, name := range []string{store.OperationBucket, store.CommitBucket, store.MountEventBucket} {
			if _, err := tx.CreateBucket([]byte(name)); err != nil {
				return err
			}
		}
		for _, header := range []struct {
			key, value []byte
		}{
			{[]byte{store.FormatMarkerTag}, []byte(store.FormatMarker)},
			{[]byte{store.TokenScopeTag}, identity.Scope[:]},
			{[]byte{store.PublicRevisionTag}, identity.PublicKey[:]},
			{[]byte{store.AuthorityRefTag}, []byte(identity.Authority)},
		} {
			if err := elements.Put(header.key, header.value); err != nil {
				return err
			}
		}
		for _, entry := range []struct {
			bucket, key, value string
		}{
			{store.ElementBucket, "old-element", "old-element-value"},
			{store.OperationBucket, "old-operation", "old-operation-value"},
			{store.CommitBucket, "old-commit", "old-commit-value"},
			{store.MountEventBucket, "old-mount", "old-mount-value"},
		} {
			if err := tx.Bucket([]byte(entry.bucket)).Put([]byte(entry.key), []byte(entry.value)); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func putFour(sinks *store.WriteSinks, suffix string) error {
	for _, entry := range []struct {
		sink  store.Sink
		key   string
		value string
	}{
		{sinks.Elements, "element-" + suffix, "element-value-" + suffix},
		{sinks.Operations, "operation-" + suffix, "operation-value-" + suffix},
		{sinks.Commits, "commit-" + suffix, "commit-value-" + suffix},
		{sinks.MountEvents, "mount-" + suffix, "mount-value-" + suffix},
	} {
		if err := entry.sink.Put([]byte(entry.key), []byte(entry.value)); err != nil {
			return err
		}
	}
	return nil
}

func replaceHeaders(sinks *store.WriteSinks, identity store.Identity) error {
	for _, header := range []struct {
		key, value []byte
	}{
		{[]byte{store.FormatMarkerTag}, []byte(store.FormatMarker)},
		{[]byte{store.TokenScopeTag}, identity.Scope[:]},
		{[]byte{store.PublicRevisionTag}, identity.PublicKey[:]},
		{[]byte{store.AuthorityRefTag}, []byte(identity.Authority)},
	} {
		if err := sinks.Elements.Put(header.key, header.value); err != nil {
			return err
		}
	}
	return nil
}

func bucketValues(db *bbolt.DB, name string) (map[string][]byte, uint64, error) {
	entries := map[string][]byte{}
	var sequence uint64
	err := db.View(func(tx *bbolt.Tx) error {
		bucket := tx.Bucket([]byte(name))
		if bucket == nil {
			return fmt.Errorf("bucket %q is missing", name)
		}
		sequence = bucket.Sequence()
		return bucket.ForEach(func(key, value []byte) error {
			if value == nil {
				return fmt.Errorf("bucket %q contains nested bucket %q", name, key)
			}
			entries[string(key)] = append([]byte(nil), value...)
			return nil
		})
	})
	return entries, sequence, err
}

func assertBucket(t *testing.T, db *bbolt.DB, name string, want map[string][]byte, wantSequence uint64) {
	t.Helper()
	got, sequence, err := bucketValues(db, name)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) || sequence != wantSequence {
		t.Fatalf("%s = entries %#v sequence %d, want entries %#v sequence %d", name, got, sequence, want, wantSequence)
	}
}

func assertFourBuckets(t *testing.T, db *bbolt.DB, suffix string) {
	t.Helper()
	assertBucket(t, db, store.ElementBucket, map[string][]byte{"element-" + suffix: []byte("element-value-" + suffix)}, 0)
	assertBucket(t, db, store.OperationBucket, map[string][]byte{"operation-" + suffix: []byte("operation-value-" + suffix)}, 0)
	assertBucket(t, db, store.CommitBucket, map[string][]byte{"commit-" + suffix: []byte("commit-value-" + suffix)}, 0)
	assertBucket(t, db, store.MountEventBucket, map[string][]byte{"mount-" + suffix: []byte("mount-value-" + suffix)}, 0)
}

func assertUnchanged(t *testing.T, db *bbolt.DB, before storeSnapshot) {
	t.Helper()
	if after := snapshotDB(t, db); !reflect.DeepEqual(after, before) {
		t.Fatalf("failed preparation changed disk: before=%#v after=%#v", before, after)
	}
	if err := db.Update(func(*bbolt.Tx) error { return nil }); err != nil {
		t.Fatalf("database transaction was not released: %v", err)
	}
}

func TestPrepareCreateCommitWritesAllFourBuckets(t *testing.T) {
	db := newStoreDB(t)
	err := db.Update(func(tx *bbolt.Tx) error {
		sinks, err := store.PrepareCreate(tx)
		if err != nil {
			return err
		}
		return putFour(sinks, "create")
	})
	if err != nil {
		t.Fatal(err)
	}
	assertFourBuckets(t, db, "create")
}

func TestManagedUpdateErrorRollsBackAllFourBuckets(t *testing.T) {
	db := newStoreDB(t)
	marker := errors.New("abort after staged writes")
	err := db.Update(func(tx *bbolt.Tx) error {
		sinks, err := store.PrepareCreate(tx)
		if err != nil {
			return err
		}
		if err := putFour(sinks, "rollback"); err != nil {
			return err
		}
		return marker
	})
	if !errors.Is(err, marker) {
		t.Fatalf("managed update error = %v", err)
	}
	if snapshot := snapshotDB(t, db); len(snapshot.Buckets) != 0 {
		t.Fatalf("managed rollback left buckets: %#v", snapshot)
	}
	if err := db.Update(func(*bbolt.Tx) error { return nil }); err != nil {
		t.Fatalf("database transaction was not released: %v", err)
	}
}

func TestManagedUpdatePanicRollsBackAllFourBuckets(t *testing.T) {
	db := newStoreDB(t)
	var recovered any
	func() {
		defer func() { recovered = recover() }()
		err := db.Update(func(tx *bbolt.Tx) error {
			sinks, err := store.PrepareCreate(tx)
			if err != nil {
				return err
			}
			if err := putFour(sinks, "panic"); err != nil {
				return err
			}
			panic("panic after staged writes")
		})
		t.Fatalf("managed update returned after panic: %v", err)
	}()
	if recovered == nil {
		t.Fatal("panic was not propagated")
	}
	if snapshot := snapshotDB(t, db); len(snapshot.Buckets) != 0 {
		t.Fatalf("panic rollback left buckets: %#v", snapshot)
	}
	if err := db.Update(func(*bbolt.Tx) error { return nil }); err != nil {
		t.Fatalf("database transaction was not released: %v", err)
	}
}

func TestManagedReplaceRollbackRestoresAllFourBuckets(t *testing.T) {
	for _, panicCase := range []bool{false, true} {
		name := "callback error"
		if panicCase {
			name = "callback panic"
		}
		t.Run(name, func(t *testing.T) {
			db := newStoreDB(t)
			identity := newIdentity(t)
			seedStore(t, db, identity)
			before := snapshotDB(t, db)
			marker := errors.New("replace callback abort")
			update := func(tx *bbolt.Tx) error {
				sinks, err := store.PrepareReplace(tx, identity)
				if err != nil {
					return err
				}
				if err := replaceHeaders(sinks, identity); err != nil {
					return err
				}
				if err := putFour(sinks, "replace-rollback"); err != nil {
					return err
				}
				if panicCase {
					panic("replace callback panic")
				}
				return marker
			}
			if panicCase {
				var recovered any
				func() {
					defer func() { recovered = recover() }()
					err := db.Update(update)
					t.Fatalf("managed replace returned after panic: %v", err)
				}()
				if recovered == nil {
					t.Fatal("replace panic was not propagated")
				}
			} else if err := db.Update(update); !errors.Is(err, marker) {
				t.Fatalf("managed replace error = %v", err)
			}
			assertUnchanged(t, db, before)
		})
	}
}

func TestPrepareReplaceCommitClearsAndRewritesAllFourBuckets(t *testing.T) {
	db := newStoreDB(t)
	identity := newIdentity(t)
	seedStore(t, db, identity)
	err := db.Update(func(tx *bbolt.Tx) error {
		sinks, err := store.PrepareReplace(tx, identity)
		if err != nil {
			return err
		}
		if err := replaceHeaders(sinks, identity); err != nil {
			return err
		}
		return putFour(sinks, "replace")
	})
	if err != nil {
		t.Fatal(err)
	}
	assertBucket(t, db, store.ElementBucket, map[string][]byte{
		string([]byte{store.FormatMarkerTag}):   []byte(store.FormatMarker),
		string([]byte{store.TokenScopeTag}):     identity.Scope[:],
		string([]byte{store.PublicRevisionTag}): identity.PublicKey[:],
		string([]byte{store.AuthorityRefTag}):   []byte(identity.Authority),
		"element-replace":                       []byte("element-value-replace"),
	}, 0)
	assertBucket(t, db, store.OperationBucket, map[string][]byte{"operation-replace": []byte("operation-value-replace")}, 0)
	assertBucket(t, db, store.CommitBucket, map[string][]byte{"commit-replace": []byte("commit-value-replace")}, 0)
	assertBucket(t, db, store.MountEventBucket, map[string][]byte{"mount-replace": []byte("mount-value-replace")}, 0)
}

func TestPrepareCreateRejectsRepeatedInitializationWithoutMutation(t *testing.T) {
	db := newStoreDB(t)
	if err := db.Update(func(tx *bbolt.Tx) error {
		sinks, err := store.PrepareCreate(tx)
		if err != nil {
			return err
		}
		return putFour(sinks, "initial")
	}); err != nil {
		t.Fatal(err)
	}
	before := snapshotDB(t, db)
	err := db.Update(func(tx *bbolt.Tx) error {
		_, err := store.PrepareCreate(tx)
		return err
	})
	if !errors.Is(err, bbolt.ErrBucketExists) {
		t.Fatalf("repeated PrepareCreate = %v", err)
	}
	assertUnchanged(t, db, before)
}

func TestPrepareReplaceRejectsIdentityAndHeaderErrorsWithoutMutation(t *testing.T) {
	tests := []struct {
		name     string
		identity func(store.Identity) store.Identity
		mutate   func(*bbolt.DB, store.Identity) error
		wantErr  string
	}{
		{name: "wrong authority", identity: func(id store.Identity) store.Identity {
			id.Authority = values.FactAuthorityRef("00000000-0000-4000-8000-000000000099")
			return id
		}, wantErr: "Core Store marker, identity or scope changed"},
		{name: "wrong scope", identity: func(id store.Identity) store.Identity {
			id.Scope[0]++
			return id
		}, wantErr: "Core Store marker, identity or scope changed"},
		{name: "wrong public key", identity: func(id store.Identity) store.Identity {
			id.PublicKey[0]++
			return id
		}, wantErr: "Core Store marker, identity or scope changed"},
		{name: "marker header", mutate: func(db *bbolt.DB, _ store.Identity) error {
			return db.Update(func(tx *bbolt.Tx) error {
				return tx.Bucket([]byte(store.ElementBucket)).Put([]byte{store.FormatMarkerTag}, []byte("wrong-marker"))
			})
		}, wantErr: "Core Store marker, identity or scope changed"},
		{name: "scope header", mutate: func(db *bbolt.DB, id store.Identity) error {
			return db.Update(func(tx *bbolt.Tx) error {
				bad := append([]byte(nil), id.Scope[:]...)
				bad[0]++
				return tx.Bucket([]byte(store.ElementBucket)).Put([]byte{store.TokenScopeTag}, bad)
			})
		}, wantErr: "Core Store marker, identity or scope changed"},
		{name: "public key header", mutate: func(db *bbolt.DB, id store.Identity) error {
			return db.Update(func(tx *bbolt.Tx) error {
				bad := append([]byte(nil), id.PublicKey[:]...)
				bad[0]++
				return tx.Bucket([]byte(store.ElementBucket)).Put([]byte{store.PublicRevisionTag}, bad)
			})
		}, wantErr: "Core Store marker, identity or scope changed"},
		{name: "authority header", mutate: func(db *bbolt.DB, _ store.Identity) error {
			return db.Update(func(tx *bbolt.Tx) error {
				return tx.Bucket([]byte(store.ElementBucket)).Put([]byte{store.AuthorityRefTag}, []byte("wrong-authority"))
			})
		}, wantErr: "Core Store marker, identity or scope changed"},
		{name: "missing bucket", mutate: func(db *bbolt.DB, _ store.Identity) error {
			return db.Update(func(tx *bbolt.Tx) error { return tx.DeleteBucket([]byte(store.CommitBucket)) })
		}, wantErr: "Core Store bucket is missing"},
		{name: "header precedes sequence and nested bucket", mutate: func(db *bbolt.DB, _ store.Identity) error {
			return db.Update(func(tx *bbolt.Tx) error {
				if err := tx.Bucket([]byte(store.ElementBucket)).Put([]byte{store.FormatMarkerTag}, []byte("wrong-marker")); err != nil {
					return err
				}
				tx.Bucket([]byte(store.OperationBucket)).NextSequence()
				_, err := tx.Bucket([]byte(store.MountEventBucket)).CreateBucket([]byte("zz-nested"))
				return err
			})
		}, wantErr: "Core Store marker, identity or scope changed"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			db := newStoreDB(t)
			identity := newIdentity(t)
			seedStore(t, db, identity)
			if tc.mutate != nil {
				if err := tc.mutate(db, identity); err != nil {
					t.Fatal(err)
				}
			}
			before := snapshotDB(t, db)
			requested := identity
			if tc.identity != nil {
				requested = tc.identity(identity)
			}
			err := db.Update(func(tx *bbolt.Tx) error {
				_, err := store.PrepareReplace(tx, requested)
				return err
			})
			if err == nil || err.Error() != tc.wantErr {
				t.Fatalf("invalid replacement error = %v, want %q", err, tc.wantErr)
			}
			assertUnchanged(t, db, before)
		})
	}
}

func TestPrepareReplaceRejectsNonZeroBucketSequenceWithoutMutation(t *testing.T) {
	for _, name := range []string{store.ElementBucket, store.OperationBucket, store.CommitBucket, store.MountEventBucket} {
		t.Run(name, func(t *testing.T) {
			db := newStoreDB(t)
			identity := newIdentity(t)
			seedStore(t, db, identity)
			if err := db.Update(func(tx *bbolt.Tx) error {
				tx.Bucket([]byte(name)).NextSequence()
				return nil
			}); err != nil {
				t.Fatal(err)
			}
			before := snapshotDB(t, db)
			err := db.Update(func(tx *bbolt.Tx) error {
				_, err := store.PrepareReplace(tx, identity)
				return err
			})
			if err == nil || err.Error() != "Core Store bucket sequence is not zero" {
				t.Fatalf("replacement with non-zero %s sequence = %v", name, err)
			}
			assertUnchanged(t, db, before)
		})
	}
}

func TestPrepareReplaceRejectsNestedBucketWithoutMutation(t *testing.T) {
	db := newStoreDB(t)
	identity := newIdentity(t)
	seedStore(t, db, identity)
	if err := db.Update(func(tx *bbolt.Tx) error {
		_, err := tx.Bucket([]byte(store.MountEventBucket)).CreateBucket([]byte("zz-nested"))
		return err
	}); err != nil {
		t.Fatal(err)
	}
	before := snapshotDB(t, db)
	var sawPartialClear bool
	err := db.Update(func(tx *bbolt.Tx) error {
		_, err := store.PrepareReplace(tx, identity)
		if err != nil {
			elements := snapshotBucket(tx.Bucket([]byte(store.ElementBucket)))
			operations := snapshotBucket(tx.Bucket([]byte(store.OperationBucket)))
			commits := snapshotBucket(tx.Bucket([]byte(store.CommitBucket)))
			mountEvents := snapshotBucket(tx.Bucket([]byte(store.MountEventBucket)))
			sawPartialClear = len(elements.Entries) == 0 && len(operations.Entries) == 0 && len(commits.Entries) == 0 &&
				len(mountEvents.Entries) == 1 && string(mountEvents.Entries[0].Key) == "zz-nested" && mountEvents.Entries[0].Nested != nil
		}
		return err
	})
	if err == nil || err.Error() != `nested bucket "zz-nested" is not recognized` {
		t.Fatalf("replacement with nested bucket = %v", err)
	}
	if !sawPartialClear {
		t.Fatal("nested bucket error did not occur after the first three buckets were cleared")
	}
	assertUnchanged(t, db, before)
}

func TestWriteSinksAreRetiredAfterManagedUpdate(t *testing.T) {
	db := newStoreDB(t)
	var sinks *store.WriteSinks
	if err := db.Update(func(tx *bbolt.Tx) error {
		var err error
		sinks, err = store.PrepareCreate(tx)
		if err != nil {
			return err
		}
		return putFour(sinks, "retire")
	}); err != nil {
		t.Fatal(err)
	}
	for _, put := range []func() error{
		func() error { return sinks.Elements.Put([]byte("late-element"), []byte("late")) },
		func() error { return sinks.Operations.Put([]byte("late-operation"), []byte("late")) },
		func() error { return sinks.Commits.Put([]byte("late-commit"), []byte("late")) },
		func() error { return sinks.MountEvents.Put([]byte("late-mount"), []byte("late")) },
	} {
		if err := put(); !errors.Is(err, bbolt.ErrTxClosed) {
			t.Fatalf("retired sink write = %v, want %v", err, bbolt.ErrTxClosed)
		}
	}
	assertFourBuckets(t, db, "retire")
}

func TestPrepareWriteRejectsNilAndClosedTransactions(t *testing.T) {
	identity := newIdentity(t)
	if sinks, err := store.PrepareCreate(nil); sinks != nil || !errors.Is(err, bbolt.ErrTxClosed) {
		t.Fatalf("PrepareCreate(nil) = sinks %v err %v", sinks, err)
	}
	if sinks, err := store.PrepareReplace(nil, identity); sinks != nil || !errors.Is(err, bbolt.ErrTxClosed) {
		t.Fatalf("PrepareReplace(nil) = sinks %v err %v", sinks, err)
	}
	db := newStoreDB(t)
	var sinks *store.WriteSinks
	var retired *bbolt.Tx
	if err := db.Update(func(tx *bbolt.Tx) error {
		retired = tx
		var err error
		sinks, err = store.PrepareCreate(tx)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if sinks == nil {
		t.Fatal("successful preparation returned nil sinks")
	}
	if got, err := store.PrepareCreate(retired); got != nil || !errors.Is(err, bbolt.ErrTxClosed) {
		t.Fatalf("PrepareCreate(retired) = sinks %v err %v", got, err)
	}
	if got, err := store.PrepareReplace(retired, identity); got != nil || !errors.Is(err, bbolt.ErrTxClosed) {
		t.Fatalf("PrepareReplace(retired) = sinks %v err %v", got, err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if err := db.Update(func(*bbolt.Tx) error { return nil }); !errors.Is(err, bbolt.ErrDatabaseNotOpen) {
		t.Fatalf("Update(closed) = %v", err)
	}
}
