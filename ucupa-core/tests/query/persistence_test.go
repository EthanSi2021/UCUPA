package query_test

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"go.etcd.io/bbolt"

	"ucupa/ucupa-core/contract"
	core "ucupa/ucupa-core/instance"
)

// Read resources are run-local. The file copy below is made only at explicit
// fixture quiescence points where this test has no concurrent writer. It is a
// byte-level observation aid, not a supported online backup mechanism.
func TestReadResourcesDoNotPersistOrResurrect(t *testing.T) {
	ttl := time.Minute
	config := contract.Config{SnapshotRetention: &ttl}
	path := filepath.Join(t.TempDir(), "reads.db")
	c, err := core.CreateDurable(queryPolicyContact{allowAll{}}, queryContact, path, config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Close() })
	root, first, second := id(98101), id(98102), id(98103)
	if _, err := c.Elements().CreateRoot(testActor, root, "root"); err != nil {
		t.Fatal(err)
	}
	for _, item := range []struct {
		element   int
		operation contract.OperationID
	}{{98102, "first"}, {98103, "second"}} {
		if _, err := c.Elements().CreateEntity(testActor, id(item.element), tuple(t, root, 0, 0, 0, sphere(t, 1)), item.operation); err != nil {
			t.Fatal(err)
		}
	}
	before := storeImageAtQuiescence(t, path)
	query := contract.RegionQuery{StartAnchorID: root, BasisAnchorID: root, Selector: contract.QueryEntities, Sphere: contract.QuerySphere{Radius: 10}}
	page, err := c.Queries().QueryRegion(testActor, query, nil, 1, nil)
	if err != nil || page.Snapshot == nil || page.Continuation == nil || len(page.Matches) != 1 || page.Matches[0].ElementID != first {
		t.Fatalf("first page: %#v err=%v", page, err)
	}
	next, err := c.Queries().QueryRegion(testActor, query, nil, 1, page.Continuation)
	if err != nil || len(next.Matches) != 1 || next.Matches[0].ElementID != second || next.Status != contract.QueryComplete {
		t.Fatalf("second page: %#v err=%v", next, err)
	}
	if _, err := c.Queries().ReadElementSnapshot(testActor, second, *page.Snapshot); err != nil {
		t.Fatal(err)
	}
	assertStoreImageAtQuiescence(t, before, path)
	if err := c.Queries().ReleaseReadSnapshot(testActor, *page.Snapshot); err != nil {
		t.Fatal(err)
	}
	assertStoreImageAtQuiescence(t, before, path)
	old, err := c.Queries().QueryRegion(testActor, query, nil, 1, nil)
	if err != nil || old.Snapshot == nil || old.Continuation == nil {
		t.Fatalf("before reopen: %#v err=%v", old, err)
	}
	ref := c.AuthorityRef()
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	c, err = core.OpenDurable(queryPolicyContact{allowAll{}}, path, config)
	if err != nil {
		t.Fatal(err)
	}
	if c.AuthorityRef() != ref {
		t.Fatal("reopen changed persistent owner")
	}
	latest, err := c.Elements().ReadElement(testActor, second, &old.Cut)
	if err != nil || latest.State != contract.ExistencePresent {
		t.Fatalf("persistent cut unavailable: %#v err=%v", latest, err)
	}
	got, err := c.Queries().ReadElementSnapshot(testActor, second, *old.Snapshot)
	if !errors.Is(err, contract.ErrReadCutExpired) || !reflect.ValueOf(got).IsZero() {
		t.Fatalf("snapshot resurrected: %#v err=%v", got, err)
	}
	paged, err := c.Queries().QueryRegion(testActor, query, nil, 1, old.Continuation)
	if !errors.Is(err, contract.ErrReadCutExpired) || !reflect.ValueOf(paged).IsZero() {
		t.Fatalf("continuation resurrected: %#v err=%v", paged, err)
	}
	if err := c.Queries().ReleaseReadSnapshot(testActor, *old.Snapshot); err != nil {
		t.Fatalf("retired-run release is not idempotent: %v", err)
	}
	assertStoreImageAtQuiescence(t, before, path)
}

type queryStoreBucket struct {
	sequence uint64
	entries  map[string]string
}

func storeImageAtQuiescence(t *testing.T, source string) map[string]queryStoreBucket {
	t.Helper()
	raw, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	copyPath := filepath.Join(t.TempDir(), "store-copy.db")
	if err := os.WriteFile(copyPath, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	db, err := bbolt.Open(copyPath, 0o600, &bbolt.Options{ReadOnly: true, Timeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	image := make(map[string]queryStoreBucket)
	if err := db.View(func(tx *bbolt.Tx) error {
		return tx.ForEach(func(name []byte, bucket *bbolt.Bucket) error {
			entry := queryStoreBucket{sequence: bucket.Sequence(), entries: make(map[string]string)}
			if err := bucket.ForEach(func(key, value []byte) error {
				if value == nil {
					return errors.New("unexpected nested bucket in current Core schema")
				}
				entry.entries[string(key)] = string(value)
				return nil
			}); err != nil {
				return err
			}
			image[string(name)] = entry
			return nil
		})
	}); err != nil {
		t.Fatal(err)
	}
	return image
}

func assertStoreImageAtQuiescence(t *testing.T, want map[string]queryStoreBucket, path string) {
	t.Helper()
	if !reflect.DeepEqual(want, storeImageAtQuiescence(t, path)) {
		t.Fatal("read lifecycle changed Store buckets, sequences, keys or values")
	}
}
