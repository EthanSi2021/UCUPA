package commit

import (
	"encoding/json"
	"go.etcd.io/bbolt"
	"strings"
	"testing"
	"ucupa/shared/values"
	corecontract "ucupa/ucupa-core/contract"
	"ucupa/ucupa-core/internal/geometry"
	storebytes "ucupa/ucupa-core/internal/store"
	storerecord "ucupa/ucupa-core/internal/store/record"
)

// CorruptCurrentStoreForTest is test-build-only. It can only apply one of
// the original named corruptions to the fixed durable Element fixture.
// It exposes no database, mutable state, arbitrary key or mutation callback.
func CorruptCurrentStoreForTest(t *testing.T, path, fault string) {
	t.Helper()
	corruptCurrentStore(t, path, fault)
}

// corruptCurrentStore preserves each original corruption against the exact
// root/Anchor/Entity fixture IDs. The test entry point accepts only its name.
func corruptCurrentStore(t *testing.T, path, fault string) {
	t.Helper()
	tests := []struct {
		name    string
		corrupt func(*testing.T, *bbolt.Tx, foundationStoreFixture)
	}{
		{
			name: "unrecognized marker",
			corrupt: func(t *testing.T, tx *bbolt.Tx, _ foundationStoreFixture) {
				t.Helper()
				putFoundationStoreValue(
					t, tx.Bucket(elementBucketName), formatMarkerKey,
					[]byte("ucupa.core.local@arbitrary-mismatch"),
				)
			},
		},
		{
			// A P7.3c journal may hold the rejected re-propose of a locally
			// detached Element; P7.3d recomputes that record as committed, so
			// the exact superseded marker must keep refusing to open.
			name: "superseded P7.3c marker",
			corrupt: func(t *testing.T, tx *bbolt.Tx, _ foundationStoreFixture) {
				t.Helper()
				putFoundationStoreValue(
					t, tx.Bucket(elementBucketName), formatMarkerKey,
					[]byte("ucupa.core.local@2c7e9f4a1d8b3c6e5f0a9b2d4e6f8c1a"),
				)
			},
		},
		{
			// A P7.3d journal may hold the committed Element deletion of a
			// detached relationship; P7.3e's disposition protocol recomputes
			// that deletion as refused, so the exact superseded marker must
			// keep refusing to open.
			name: "superseded P7.3d marker",
			corrupt: func(t *testing.T, tx *bbolt.Tx, _ foundationStoreFixture) {
				t.Helper()
				putFoundationStoreValue(
					t, tx.Bucket(elementBucketName), formatMarkerKey,
					[]byte("ucupa.core.local@5c2e8a1f7b4d6039c8e5f2a7d1b9c4e6"),
				)
			},
		},
		{
			// S2P1 owners kept an optional Realm contact; S2P2 makes the
			// contact mandatory with a new required Store projection, so the
			// exact superseded marker must keep refusing to open.
			name: "superseded S2P1 marker",
			corrupt: func(t *testing.T, tx *bbolt.Tx, _ foundationStoreFixture) {
				t.Helper()
				putFoundationStoreValue(
					t, tx.Bucket(elementBucketName), formatMarkerKey,
					[]byte("ucupa.core.local@8d4b7f2e6a9c3501e2f6b8d4c7a3e9c5"),
				)
			},
		},
		{
			name: "missing marker",
			corrupt: func(t *testing.T, tx *bbolt.Tx, _ foundationStoreFixture) {
				t.Helper()
				if err := tx.Bucket(elementBucketName).Delete(formatMarkerKey); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "empty marker",
			corrupt: func(t *testing.T, tx *bbolt.Tx, _ foundationStoreFixture) {
				t.Helper()
				putFoundationStoreValue(t, tx.Bucket(elementBucketName), formatMarkerKey, []byte{})
			},
		},
		{
			name: "unexpected bucket",
			corrupt: func(t *testing.T, tx *bbolt.Tx, _ foundationStoreFixture) {
				t.Helper()
				if _, err := tx.CreateBucket([]byte("unexpected")); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "invalid Element UUID key",
			corrupt: func(t *testing.T, tx *bbolt.Tx, fixture foundationStoreFixture) {
				t.Helper()
				bucket := tx.Bucket(elementBucketName)
				key := currentStoreElementKey(t, fixture.entityID)
				encoded := append([]byte(nil), bucket.Get(key)...)
				if err := bucket.Delete(key); err != nil {
					t.Fatal(err)
				}
				invalidKey := append([]byte{1}, []byte(strings.Repeat("x", 36))...)
				putFoundationStoreValue(t, bucket, invalidKey, encoded)
			},
		},
		{
			name: "unknown Element kind",
			corrupt: func(t *testing.T, tx *bbolt.Tx, fixture foundationStoreFixture) {
				t.Helper()
				bucket := tx.Bucket(elementBucketName)
				key := currentStoreElementKey(t, fixture.entityID)
				stored := currentStoreElement(t, bucket.Get(key))
				stored.Kind = "unknown"
				putFoundationStoreJSON(t, bucket, key, stored)
			},
		},
		{
			name: "root carries Pose",
			corrupt: func(t *testing.T, tx *bbolt.Tx, fixture foundationStoreFixture) {
				t.Helper()
				bucket := tx.Bucket(elementBucketName)
				key := currentStoreElementKey(t, fixture.rootID)
				stored := currentStoreElement(t, bucket.Get(key))
				pose := storerecord.StorePose(geometry.IdentityPose())
				stored.Pose = &pose
				putFoundationStoreJSON(t, bucket, key, stored)
			},
		},
		{
			name: "noncanonical Rotation",
			corrupt: func(t *testing.T, tx *bbolt.Tx, fixture foundationStoreFixture) {
				t.Helper()
				bucket := tx.Bucket(elementBucketName)
				key := currentStoreElementKey(t, fixture.entityID)
				stored := currentStoreElement(t, bucket.Get(key))
				stored.Pose.Pivot = uint8(corecontract.PivotX)
				stored.Pose.Ratios[0] = storebytes.EncodeFloatBits(1)
				putFoundationStoreJSON(t, bucket, key, stored)
			},
		},
		{
			name: "noncanonical Element JSON",
			corrupt: func(t *testing.T, tx *bbolt.Tx, fixture foundationStoreFixture) {
				t.Helper()
				bucket := tx.Bucket(elementBucketName)
				key := currentStoreElementKey(t, fixture.entityID)
				encoded := append([]byte(nil), bucket.Get(key)...)
				putFoundationStoreValue(t, bucket, key, append([]byte{' '}, encoded...))
			},
		},
		{
			name: "unknown Operation field",
			corrupt: func(t *testing.T, tx *bbolt.Tx, _ foundationStoreFixture) {
				t.Helper()
				bucket := tx.Bucket(operationBucketName)
				key, encoded := currentStoreFirstEntry(t, bucket)
				if len(encoded) == 0 || encoded[len(encoded)-1] != '}' {
					t.Fatalf("unexpected Operation encoding %q", encoded)
				}
				encoded = append(encoded[:len(encoded)-1:len(encoded)-1], []byte(`,"unknown":true}`)...)
				putFoundationStoreValue(t, bucket, key, encoded)
			},
		},
		{
			name: "Anchor cycle",
			corrupt: func(t *testing.T, tx *bbolt.Tx, fixture foundationStoreFixture) {
				t.Helper()
				bucket := tx.Bucket(elementBucketName)
				key := currentStoreElementKey(t, fixture.anchorID)
				stored := currentStoreElement(t, bucket.Get(key))
				stored.Parent = string(fixture.anchorID)
				putFoundationStoreJSON(t, bucket, key, stored)
			},
		},
		{
			name: "out of parent Range",
			corrupt: func(t *testing.T, tx *bbolt.Tx, fixture foundationStoreFixture) {
				t.Helper()
				bucket := tx.Bucket(elementBucketName)
				key := currentStoreElementKey(t, fixture.entityID)
				stored := currentStoreElement(t, bucket.Get(key))
				stored.Pose.Translation[0] = storebytes.EncodeFloatBits(100)
				putFoundationStoreJSON(t, bucket, key, stored)
			},
		},
		{
			name: "duplicate Element Revision source",
			corrupt: func(t *testing.T, tx *bbolt.Tx, fixture foundationStoreFixture) {
				t.Helper()
				bucket := tx.Bucket(elementBucketName)
				rootKey := currentStoreElementKey(t, fixture.rootID)
				entityKey := currentStoreElementKey(t, fixture.entityID)
				root := currentStoreElement(t, bucket.Get(rootKey))
				entity := currentStoreElement(t, bucket.Get(entityKey))
				entity.RevisionSerial = root.RevisionSerial
				putFoundationStoreJSON(t, bucket, entityKey, entity)
			},
		},
		{
			name: "forged same-scope expected Revision",
			corrupt: func(t *testing.T, tx *bbolt.Tx, _ foundationStoreFixture) {
				t.Helper()
				bucket := tx.Bucket(operationBucketName)
				key := []byte("fixture-stale")
				stored := currentStoreOperation(t, bucket.Get(key))
				stored.Binding.Expected.Serial = 999
				putFoundationStoreJSON(t, bucket, key, stored)
			},
		},
		{
			name: "canonical impossible retained outcome",
			corrupt: func(t *testing.T, tx *bbolt.Tx, _ foundationStoreFixture) {
				t.Helper()
				bucket := tx.Bucket(operationBucketName)
				key := []byte("fixture-conflict")
				stored := currentStoreOperation(t, bucket.Get(key))
				stored.Outcome.Error = "not-found"
				putFoundationStoreJSON(t, bucket, key, stored)
			},
		},
		{
			name: "legal current fact disagrees with committed history",
			corrupt: func(t *testing.T, tx *bbolt.Tx, fixture foundationStoreFixture) {
				t.Helper()
				bucket := tx.Bucket(elementBucketName)
				key := currentStoreElementKey(t, fixture.entityID)
				stored := currentStoreElement(t, bucket.Get(key))
				stored.Pose.Translation[0] = storebytes.EncodeFloatBits(2)
				putFoundationStoreJSON(t, bucket, key, stored)
			},
		},
		{
			name: "missing committed history",
			corrupt: func(t *testing.T, tx *bbolt.Tx, _ foundationStoreFixture) {
				t.Helper()
				bucket := tx.Bucket(operationBucketName)
				key, _ := currentStoreFirstEntry(t, bucket)
				if err := bucket.Delete(key); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "cut Revision mismatch",
			corrupt: func(t *testing.T, tx *bbolt.Tx, _ foundationStoreFixture) {
				t.Helper()
				putFoundationStoreValue(t, tx.Bucket(elementBucketName), currentCutKey, storebytes.EncodeUint64(99))
			},
		},
		{
			name: "invalid bounded Range",
			corrupt: func(t *testing.T, tx *bbolt.Tx, fixture foundationStoreFixture) {
				t.Helper()
				bucket := tx.Bucket(elementBucketName)
				key := currentStoreElementKey(t, fixture.entityID)
				stored := currentStoreElement(t, bucket.Get(key))
				stored.RadiusBits = "8000000000000000"
				putFoundationStoreJSON(t, bucket, key, stored)
			},
		},
		{
			name: "missing parent",
			corrupt: func(t *testing.T, tx *bbolt.Tx, fixture foundationStoreFixture) {
				t.Helper()
				bucket := tx.Bucket(elementBucketName)
				key := currentStoreElementKey(t, fixture.entityID)
				stored := currentStoreElement(t, bucket.Get(key))
				stored.Parent = string(foundationStoreID(999))
				putFoundationStoreJSON(t, bucket, key, stored)
			},
		},
	}

	for _, test := range tests {
		if test.name != fault {
			continue
		}
		fixture := foundationStoreFixture{rootID: foundationStoreID(100), anchorID: foundationStoreID(101), entityID: foundationStoreID(102)}
		mutateFoundationStore(t, path, func(tx *bbolt.Tx) { test.corrupt(t, tx, fixture) })
		return
	}
	t.Fatalf("unknown current Store corruption %q", fault)
}

func currentStoreElementKey(t *testing.T, id values.ElementID) []byte {
	t.Helper()
	key, err := storebytes.ElementKey(id)
	if err != nil {
		t.Fatal(err)
	}
	return key
}

func currentStoreFirstEntry(t *testing.T, bucket *bbolt.Bucket) ([]byte, []byte) {
	t.Helper()
	if bucket == nil {
		t.Fatal("Store bucket is missing")
	}
	key, encoded := bucket.Cursor().First()
	if key == nil || encoded == nil {
		t.Fatal("Store bucket has no entry")
	}
	return append([]byte(nil), key...), append([]byte(nil), encoded...)
}

func currentStoreElement(t *testing.T, encoded []byte) storerecord.StoredElement {
	t.Helper()
	if encoded == nil {
		t.Fatal("stored Element is missing")
	}
	var stored storerecord.StoredElement
	if err := json.Unmarshal(encoded, &stored); err != nil {
		t.Fatal(err)
	}
	return stored
}

func currentStoreOperation(t *testing.T, encoded []byte) storedOperation {
	t.Helper()
	if encoded == nil {
		t.Fatal("stored Operation is missing")
	}
	var stored storedOperation
	if err := json.Unmarshal(encoded, &stored); err != nil {
		t.Fatal(err)
	}
	return stored
}
