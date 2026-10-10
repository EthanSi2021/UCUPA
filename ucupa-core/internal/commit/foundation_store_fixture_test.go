package commit

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"go.etcd.io/bbolt"
	"testing"
	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	commitbinding "ucupa/ucupa-core/internal/commit/binding"
	elementfact "ucupa/ucupa-core/internal/element/fact"
)

var foundationStoreActor = values.PrincipalRef{Realm: "store-realm", Principal: "store-writer"}

type foundationStoreFixture struct{ rootID, anchorID, entityID values.ElementID }

func seedFoundationStore(t *testing.T, path string) foundationStoreFixture {
	t.Helper()
	fixture := foundationStoreFixture{rootID: foundationStoreID(100), anchorID: foundationStoreID(101), entityID: foundationStoreID(102)}
	spatial := createFoundationStore(t, path)
	root, err := applyFoundationStore(spatial, elementfact.OperationCreateRoot, foundationStoreActor, fixture.rootID, contract.SpatialTuple{}, contract.Revision{}, "fixture-root")
	if err != nil || root.Status != contract.MutationCommitted {
		t.Fatalf("fixture root: %#v err=%v", root, err)
	}
	anchor, err := applyFoundationStore(spatial, elementfact.OperationCreateAnchor, foundationStoreActor, fixture.anchorID, foundationStoreTuple(t, fixture.rootID, 0, 20), contract.Revision{}, "fixture-anchor")

	if err != nil || anchor.Status != contract.MutationCommitted {
		t.Fatalf("fixture Anchor: %#v err=%v", anchor, err)
	}
	entity, err := applyFoundationStore(spatial, elementfact.OperationCreateEntity, foundationStoreActor, fixture.entityID, foundationStoreTuple(t, fixture.anchorID, 1, 2), contract.Revision{}, "fixture-entity")

	if err != nil || entity.Status != contract.MutationCommitted {
		t.Fatalf("fixture Entity: %#v err=%v", entity, err)
	}
	stale, staleErr := applyFoundationStore(spatial, elementfact.OperationUpdateEntity, foundationStoreActor, fixture.entityID, foundationStoreTuple(t, fixture.anchorID, 1, 2), root.Record.Anchor.Revision, "fixture-stale")

	if !errors.Is(staleErr, contract.ErrRevisionConflict) || stale.Status != contract.MutationRejected {
		t.Fatalf("fixture stale decision: %#v err=%v", stale, staleErr)
	}
	conflict, conflictErr := applyFoundationStore(spatial, elementfact.OperationCreateRoot, foundationStoreActor, foundationStoreID(103), contract.SpatialTuple{}, contract.Revision{}, "fixture-conflict")
	if !errors.Is(conflictErr, contract.ErrConflict) || conflict.Status != contract.MutationRejected {
		t.Fatalf("fixture conflict decision: %#v err=%v", conflict, conflictErr)
	}
	closeFoundationStore(t, spatial)
	return fixture
}

// These tests target the sole commit and recovery owner. They submit the same
// complete bindings as the original Element fixture, with no synthetic facts.
func applyFoundationStore(owner *Owner, kind elementfact.OperationKind, actor values.PrincipalRef, id values.ElementID, final contract.SpatialTuple, expected contract.Revision, operation contract.OperationID) (contract.MutationResult, error) {
	owner.Lock()
	defer owner.Unlock()
	return owner.ApplyElementLocked(operation, commitbinding.Binding{Actor: actor, Kind: kind, Element: id, Final: final, Expected: expected})
}

func createFoundationStore(t *testing.T, path string) *Owner {
	t.Helper()
	authority, err := values.NewFactAuthorityRef()
	if err != nil {
		t.Fatal(err)
	}
	var scope [32]byte
	if _, err := rand.Read(scope[:]); err != nil {
		t.Fatal(err)
	}
	owner, err := CreateDurable(path, authority, scope, contract.RealmInitialization{Realm: "fixture-realm", NextHop: "local:fixture"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	return owner
}

func openFoundationStore(t *testing.T, path string) *Owner {
	t.Helper()
	owner, err := OpenDurable(path, nil)
	if err != nil {
		t.Fatal(err)
	}
	return owner
}

func closeFoundationStore(t *testing.T, owner *Owner) {
	t.Helper()
	if err := owner.CloseStore(); err != nil {
		t.Fatal(err)
	}
}

func foundationStoreID(serial uint64) values.ElementID {
	return values.ElementID(fmt.Sprintf("20000000-0000-4000-8000-%012x", serial))
}

func foundationStoreTuple(t *testing.T, parent values.ElementID, x, radius float64) contract.SpatialTuple {
	t.Helper()
	rotation, err := contract.NewRotation(1, 0, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	pose, err := contract.NewPose(contract.Vec3{X: x}, rotation)
	if err != nil {
		t.Fatal(err)
	}
	sphere, err := contract.NewBoundedSphere(radius)
	if err != nil {
		t.Fatal(err)
	}
	return contract.SpatialTuple{Parent: parent, LocalPose: pose, Range: sphere}
}

func mutateFoundationStore(t *testing.T, path string, mutate func(*bbolt.Tx)) {
	t.Helper()
	db, err := bbolt.Open(path, 0o600, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Update(func(tx *bbolt.Tx) error {
		mutate(tx)
		return nil
	}); err != nil {
		_ = db.Close()
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
}

func putFoundationStoreJSON(t *testing.T, bucket *bbolt.Bucket, key []byte, value any) {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	putFoundationStoreValue(t, bucket, key, encoded)
}

func putFoundationStoreValue(t *testing.T, bucket *bbolt.Bucket, key, value []byte) {
	t.Helper()
	if bucket == nil {
		t.Fatal("Store bucket is missing")
	}
	if err := bucket.Put(key, value); err != nil {
		t.Fatal(err)
	}
}

func foundationStoreOtherAuthority(t *testing.T, current values.FactAuthorityRef) values.FactAuthorityRef {
	t.Helper()
	ref, err := values.NewFactAuthorityRef()
	if err != nil || ref == current {
		t.Fatalf("new authority: %q err=%v", ref, err)
	}
	return ref
}
