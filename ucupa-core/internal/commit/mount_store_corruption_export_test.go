package commit

import (
	"encoding/json"
	"testing"

	"go.etcd.io/bbolt"
	storebytes "ucupa/ucupa-core/internal/store"
)

// CorruptMountJournalForTest applies exactly one original named journal fault.
// The caller cannot access a database or supply a transaction callback.
func CorruptMountJournalForTest(t *testing.T, path, fault string) {
	t.Helper()
	cases := []struct {
		name    string
		corrupt func(*testing.T, *bbolt.Tx)
	}{
		{"missing journal", func(t *testing.T, tx *bbolt.Tx) {
			if err := tx.DeleteBucket(mountEventBucketName); err != nil {
				t.Fatal(err)
			}
		}},
		{"journal sequence", func(t *testing.T, tx *bbolt.Tx) {
			if err := tx.Bucket(mountEventBucketName).SetSequence(1); err != nil {
				t.Fatal(err)
			}
		}},
		{"nested journal bucket", func(t *testing.T, tx *bbolt.Tx) {
			if _, err := tx.Bucket(mountEventBucketName).CreateBucket([]byte("nested")); err != nil {
				t.Fatal(err)
			}
		}},
		{"event gap", func(t *testing.T, tx *bbolt.Tx) {
			if err := tx.Bucket(mountEventBucketName).Delete(storebytes.EncodeUint64(2)); err != nil {
				t.Fatal(err)
			}
		}},
		{"unknown event", func(t *testing.T, tx *bbolt.Tx) {
			mutateStoredMountEvent(t, tx, 1, func(e *storedMountEvent) { e.Kind = 255 })
		}},
		{"missing before cut", func(t *testing.T, tx *bbolt.Tx) {
			mutateStoredMountEvent(t, tx, 1, func(e *storedMountEvent) { e.Before = nil })
		}},
		{"wrong after cut", func(t *testing.T, tx *bbolt.Tx) {
			mutateStoredMountEvent(t, tx, 1, func(e *storedMountEvent) { e.After.Serial++ })
		}},
		{"out of order", func(t *testing.T, tx *bbolt.Tx) {
			bucket := tx.Bucket(mountEventBucketName)
			first := append([]byte(nil), bucket.Get(storebytes.EncodeUint64(1))...)
			second := append([]byte(nil), bucket.Get(storebytes.EncodeUint64(2))...)
			putFoundationStoreValue(t, bucket, storebytes.EncodeUint64(1), second)
			putFoundationStoreValue(t, bucket, storebytes.EncodeUint64(2), first)
		}},
		{"commit references another control", func(t *testing.T, tx *bbolt.Tx) {
			putFoundationStoreJSON(t, tx.Bucket(commitBucketName), storebytes.EncodeUint64(4), storedCommit{Kind: "mount", ControlIndex: 2})
		}},
		{"commit duplicates ordinary outcome", func(t *testing.T, tx *bbolt.Tx) {
			putFoundationStoreJSON(t, tx.Bucket(commitBucketName), storebytes.EncodeUint64(4), storedCommit{Kind: "element", Operation: []byte("mount-store-root")})
		}},
		{"ordinary and control operation collision", func(t *testing.T, tx *bbolt.Tx) {
			mutateStoredMountEvent(t, tx, 1, func(e *storedMountEvent) { e.Operation = []byte("mount-store-root") })
		}},
		{"undeclared event field", func(t *testing.T, tx *bbolt.Tx) {
			mutateStoredMountEvent(t, tx, 1, func(e *storedMountEvent) { e.Source = e.Binding.Command.Carrier })
		}},
		{"invalid specified address", func(t *testing.T, tx *bbolt.Tx) {
			mutateStoredMountEvent(t, tx, 1, func(e *storedMountEvent) { e.Binding.Command.CarrierNextHop = "LOCAL:b" })
		}},
		{"forged own expected revision", func(t *testing.T, tx *bbolt.Tx) {
			mutateStoredMountEvent(t, tx, 1, func(e *storedMountEvent) { e.Binding.Command.ExpectedElement.Serial += 100 })
		}},
		{"changed decision evidence", func(t *testing.T, tx *bbolt.Tx) {
			mutateStoredMountEvent(t, tx, 2, func(e *storedMountEvent) { e.Prepared.Request.Tuple.RadiusBits = storebytes.EncodeFloatBits(99) })
		}},
		{"new decision without authorization", func(t *testing.T, tx *bbolt.Tx) {
			mutateStoredMountEvent(t, tx, 1, func(e *storedMountEvent) { e.Authorized = false })
		}},
		{"success altered to abort", func(t *testing.T, tx *bbolt.Tx) {
			mutateStoredMountEvent(t, tx, 2, func(e *storedMountEvent) { e.Authorized = false })
		}},
		{"unknown JSON field", func(t *testing.T, tx *bbolt.Tx) {
			bucket := tx.Bucket(mountEventBucketName)
			key := storebytes.EncodeUint64(1)
			encoded := append([]byte(nil), bucket.Get(key)...)
			encoded = append(encoded[:len(encoded)-1], []byte(`,"extra":true}`)...)
			putFoundationStoreValue(t, bucket, key, encoded)
		}},
	}

	for _, test := range cases {
		if test.name == fault {
			mutateFoundationStore(t, path, func(tx *bbolt.Tx) { test.corrupt(t, tx) })
			return
		}
	}
	t.Fatalf("unknown mount journal fault %q", fault)
}

func mutateStoredMountEvent(t *testing.T, tx *bbolt.Tx, index uint64, change func(*storedMountEvent)) {
	t.Helper()
	bucket := tx.Bucket(mountEventBucketName)
	key := storebytes.EncodeUint64(index)
	var event storedMountEvent
	if err := json.Unmarshal(bucket.Get(key), &event); err != nil {
		t.Fatal(err)
	}
	change(&event)
	putFoundationStoreJSON(t, bucket, key, event)
}
