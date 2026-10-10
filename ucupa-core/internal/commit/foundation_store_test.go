package commit

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"go.etcd.io/bbolt"
	"os"
	"path/filepath"
	"testing"
	"ucupa/ucupa-core/contract"
	elementfact "ucupa/ucupa-core/internal/element/fact"
)

func TestFoundationCommitAndReceiptAreAtomicAcrossWriteFailures(t *testing.T) {
	for _, afterCommit := range []bool{false, true} {
		name := "rollback"
		if afterCommit {
			name = "lost response"
		}
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "atomic.db")
			fixture := seedFoundationStore(t, path)
			c := openFoundationStore(t, path)
			before := c.state.clone()
			identity := c.AuthorityRef()
			injected := errors.New("foundation write failure")
			fail := func() error { return injected }
			if afterCommit {
				c.store.failAfterCommit = fail
			} else {
				c.store.failBeforeCommit = fail
			}
			entityID := foundationStoreID(8500)
			final := foundationStoreTuple(t, fixture.anchorID, 0, 1)
			result, err := applyFoundationStore(c, elementfact.OperationCreateEntity, foundationStoreActor, entityID, final, contract.Revision{}, "atomic-next")
			if !errors.Is(err, contract.ErrStore) || !errors.Is(err, injected) || result != (contract.MutationResult{}) {
				t.Fatalf("failed mutation: %#v err=%v", result, err)
			}
			if c.AuthorityRef() != identity || c.state == nil {
				t.Fatal("reconcilable failure changed identity or poisoned Core")
			}
			if afterCommit {
				if c.state.cutSerial != before.cutSerial+1 || len(c.state.operations) != len(before.operations)+1 || len(c.state.commits) != len(before.commits)+1 || c.state.commits[len(before.commits)].operation != "atomic-next" {
					t.Fatal("lost response did not publish fact, receipt, and commit together")
				}
			} else if !sameCompleteCoreState(before, c.state) {
				t.Fatal("rollback changed complete state")
			}
			published := c.state.clone()
			loaded, err := c.store.reload()
			if err != nil || !sameCompleteCoreState(published, loaded) {
				t.Fatalf("memory and Store disagree after failure: %v", err)
			}
			closeFoundationStore(t, c)
			reopened := openFoundationStore(t, path)
			defer closeFoundationStore(t, reopened)
			if !sameCompleteCoreState(published, reopened.state) || reopened.AuthorityRef() != identity {
				t.Fatal("reopen changed reconciled state")
			}
			retry, err := applyFoundationStore(reopened, elementfact.OperationCreateEntity, foundationStoreActor, entityID, final, contract.Revision{}, "atomic-next")
			if err != nil || retry.Status != contract.MutationCommitted || retry.Record == nil {
				t.Fatalf("retry: %#v err=%v", retry, err)
			}
			if len(reopened.state.commits) != len(before.commits)+1 || len(reopened.state.operations) != len(before.operations)+1 {
				t.Fatal("retry duplicated or omitted commit/receipt")
			}
		})
	}
}

func TestFoundationStoreRejectsIdentityAndCommitCorruption(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(*testing.T, *bbolt.Tx)
	}{
		{"missing identity", func(t *testing.T, tx *bbolt.Tx) { foundationDelete(t, tx.Bucket(elementBucketName), authorityRefKey) }},
		{"invalid identity", func(t *testing.T, tx *bbolt.Tx) {
			putFoundationStoreValue(t, tx.Bucket(elementBucketName), authorityRefKey, []byte("not-an-authority"))
		}},
		{"missing history bucket", func(t *testing.T, tx *bbolt.Tx) {
			if err := tx.DeleteBucket(commitBucketName); err != nil {
				t.Fatal(err)
			}
		}},
		{"missing first commit", func(t *testing.T, tx *bbolt.Tx) {
			foundationDelete(t, tx.Bucket(commitBucketName), foundationCutKey(2))
		}},
		{"interior gap", func(t *testing.T, tx *bbolt.Tx) {
			foundationDelete(t, tx.Bucket(commitBucketName), foundationCutKey(3))
		}},
		{"missing last commit", func(t *testing.T, tx *bbolt.Tx) {
			foundationDelete(t, tx.Bucket(commitBucketName), foundationCutKey(4))
		}},
		{"short key", func(t *testing.T, tx *bbolt.Tx) {
			putFoundationStoreJSON(t, tx.Bucket(commitBucketName), []byte{2}, storedCommit{Kind: "element", Operation: []byte("fixture-root")})
		}},
		{"initial cut event", func(t *testing.T, tx *bbolt.Tx) {
			putFoundationStoreJSON(t, tx.Bucket(commitBucketName), foundationCutKey(1), storedCommit{Kind: "element", Operation: []byte("fixture-root")})
		}},
		{"unknown kind", func(t *testing.T, tx *bbolt.Tx) {
			putFoundationStoreJSON(t, tx.Bucket(commitBucketName), foundationCutKey(2), storedCommit{Kind: "unknown", Operation: []byte("fixture-root")})
		}},
		{"missing receipt", func(t *testing.T, tx *bbolt.Tx) {
			foundationDelete(t, tx.Bucket(operationBucketName), []byte("fixture-root"))
		}},
		{"wrong receipt", func(t *testing.T, tx *bbolt.Tx) {
			putFoundationStoreJSON(t, tx.Bucket(commitBucketName), foundationCutKey(2), storedCommit{Kind: "element", Operation: []byte("fixture-anchor")})
		}},
		{"rejected receipt", func(t *testing.T, tx *bbolt.Tx) {
			putFoundationStoreJSON(t, tx.Bucket(commitBucketName), foundationCutKey(2), storedCommit{Kind: "element", Operation: []byte("fixture-conflict")})
		}},
		{"empty operation", func(t *testing.T, tx *bbolt.Tx) {
			putFoundationStoreJSON(t, tx.Bucket(commitBucketName), foundationCutKey(2), storedCommit{Kind: "element", Operation: []byte{}})
		}},
		{"oversized operation", func(t *testing.T, tx *bbolt.Tx) {
			putFoundationStoreJSON(t, tx.Bucket(commitBucketName), foundationCutKey(2), storedCommit{Kind: "element", Operation: bytes.Repeat([]byte{'x'}, 129)})
		}},
		{"duplicate receipt", func(t *testing.T, tx *bbolt.Tx) {
			putFoundationStoreJSON(t, tx.Bucket(commitBucketName), foundationCutKey(3), storedCommit{Kind: "element", Operation: []byte("fixture-root")})
		}},
		{"history sequence", func(t *testing.T, tx *bbolt.Tx) {
			if err := tx.Bucket(commitBucketName).SetSequence(1); err != nil {
				t.Fatal(err)
			}
		}},
		{"nested history bucket", func(t *testing.T, tx *bbolt.Tx) {
			if _, err := tx.Bucket(commitBucketName).CreateBucket([]byte("nested")); err != nil {
				t.Fatal(err)
			}
		}},
		{"unknown field", func(t *testing.T, tx *bbolt.Tx) {
			putFoundationStoreValue(t, tx.Bucket(commitBucketName), foundationCutKey(2), []byte(`{"kind":"element","operation":"Zml4dHVyZS1yb290","extra":true}`))
		}},
		{"noncanonical JSON", func(t *testing.T, tx *bbolt.Tx) {
			putFoundationStoreValue(t, tx.Bucket(commitBucketName), foundationCutKey(2), []byte(`{ "kind":"element","operation":"Zml4dHVyZS1yb290"}`))
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "corrupt.db")
			seedFoundationStore(t, path)
			mutateFoundationStore(t, path, func(tx *bbolt.Tx) { test.mutate(t, tx) })
			before, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			opened, err := OpenDurable(path, nil)
			if opened != nil {
				_ = opened.CloseStore()
				t.Fatal("corrupt Store returned a Core")
			}
			if !errors.Is(err, contract.ErrStore) {
				t.Fatalf("corrupt Store: %v", err)
			}
			after, err := os.ReadFile(path)
			if err != nil || !bytes.Equal(before, after) {
				t.Fatalf("rejected open mutated Store: %v", err)
			}
		})
	}
}

func TestFoundationCommitOperationRetainsOpaqueBytes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "opaque-commit.db")
	c := createFoundationStore(t, path)
	op := contract.OperationID(string([]byte{0, 0xff, 'o', 0x80}))
	result, err := applyFoundationStore(c, elementfact.OperationCreateRoot, foundationStoreActor, foundationStoreID(8502), contract.SpatialTuple{}, contract.Revision{}, op)
	if err != nil || result.Status != contract.MutationCommitted {
		t.Fatalf("opaque operation: %#v err=%v", result, err)
	}
	if err := c.store.db.View(func(tx *bbolt.Tx) error {
		var stored storedCommit
		if err := json.Unmarshal(tx.Bucket(commitBucketName).Get(foundationCutKey(2)), &stored); err != nil {
			return err
		}
		if stored.Kind != "element" || !bytes.Equal(stored.Operation, []byte(op)) {
			t.Fatalf("lossy commit operation: %#v", stored)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	closeFoundationStore(t, c)
	reopened := openFoundationStore(t, path)
	defer closeFoundationStore(t, reopened)
	assertFoundationCommits(t, reopened.state, []coreCommit{{kind: elementCommitted, operation: op}})
	replay, err := applyFoundationStore(reopened, elementfact.OperationCreateRoot, foundationStoreActor, foundationStoreID(8502), contract.SpatialTuple{}, contract.Revision{}, op)
	if err != nil || replay.Status != contract.MutationCommitted || !replay.OutcomeCut.Equal(result.OutcomeCut) {
		t.Fatalf("opaque replay: %#v err=%v", replay, err)
	}
}

func foundationCutKey(cut uint64) []byte {
	key := make([]byte, 8)
	binary.BigEndian.PutUint64(key, cut)
	return key
}

func foundationDelete(t *testing.T, bucket *bbolt.Bucket, key []byte) {
	t.Helper()
	if err := bucket.Delete(key); err != nil {
		t.Fatal(err)
	}
}
