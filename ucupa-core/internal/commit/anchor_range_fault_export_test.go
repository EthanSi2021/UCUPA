package commit

import (
	"encoding/json"
	"errors"
	"fmt"

	"go.etcd.io/bbolt"
	storebytes "ucupa/ucupa-core/internal/store"
)

// CorruptAnchorShrinkHistoryForTest changes only the original fixture's
// intermediate shrink, leaving its legal final expanded snapshot untouched.
// It is test-build-only and accepts no state, database or mutation callback.
func CorruptAnchorShrinkHistoryForTest(path string) error {
	db, err := bbolt.Open(path, 0600, nil)
	if err != nil {
		return err
	}
	err = db.Update(func(tx *bbolt.Tx) error {
		bucket := tx.Bucket(operationBucketName)
		key := []byte("historical-shrink")
		var stored storedOperation
		if err := json.Unmarshal(bucket.Get(key), &stored); err != nil {
			return err
		}
		if stored.Binding.Final == nil {
			return fmt.Errorf("historical shrink has no final tuple")
		}
		// Radius=2 excludes the then-current child at x=1, radius=2.
		// The final radius=20 snapshot is not changed by this corruption.
		stored.Binding.Final.RadiusBits = storebytes.EncodeFloatBits(2)
		encoded, err := json.Marshal(stored)
		if err != nil {
			return err
		}
		return bucket.Put(key, encoded)
	})
	return errors.Join(err, db.Close())
}
