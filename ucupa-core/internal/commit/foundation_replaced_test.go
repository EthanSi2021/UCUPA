package commit_test

import (
	"errors"
	"fmt"
	"path/filepath"
	"testing"
	"ucupa/shared/values"
	corecontract "ucupa/ucupa-core/contract"
	"ucupa/ucupa-core/internal/commit"
	"ucupa/ucupa-core/internal/element"
)

func TestFoundationRunningCoreRejectsReplacedAuthority(t *testing.T) {
	path := filepath.Join(t.TempDir(), "replaced-self.db")
	fixture := seedReplacedStore(t, path)
	owner, err := commit.OpenDurable(path, nil)
	if err != nil {
		t.Fatal(err)
	}
	operations := element.New(owner, writeAllowed{})
	defer owner.CloseStore()
	identity := owner.AuthorityRef()
	replacement := newReplacementAuthority(t, identity)
	if err := commit.InstallAuthorityForTest(owner, replacement); err != nil {
		t.Fatal(err)
	}
	if absent, err := commit.ReloadStateAbsentForTest(owner); !errors.Is(err, corecontract.ErrStore) || !absent {
		t.Fatalf("reload accepted replaced identity: absent=%v err=%v", absent, err)
	}
	result, err := operations.CreateEntity(replacedActor, replacedID(8501), replacedTuple(t, fixture.anchorID, 0, 1), "replaced-self-write")
	owner.RLock()
	available := owner.AvailableLocked()
	owner.RUnlock()
	if !errors.Is(err, corecontract.ErrStore) || result != (corecontract.MutationResult{}) || available {
		t.Fatalf("replaced identity did not poison writer: %#v err=%v", result, err)
	}
	if owner.AuthorityRef() != identity {
		t.Fatal("poisoning replaced instance identity")
	}
	read, err := operations.ReadElement(replacedActor, fixture.rootID, nil)
	if !errors.Is(err, corecontract.ErrStore) || read != (corecontract.ElementRead{}) {
		t.Fatalf("poisoned read: %#v err=%v", read, err)
	}
	retry, err := operations.CreateRoot(replacedActor, fixture.rootID, "fixture-root")
	if !errors.Is(err, corecontract.ErrStore) || retry != (corecontract.MutationResult{}) {
		t.Fatalf("poisoned replay: %#v err=%v", retry, err)
	}
	storedAuthority, retained, err := commit.StoredAuthorityAndOperationForTest(owner, "replaced-self-write")
	if err != nil {
		t.Fatal(err)
	}
	if storedAuthority != replacement || retained {
		t.Fatal("writer repaired identity or retained its rejected write")
	}
}

var replacedActor = values.PrincipalRef{Realm: "store-realm", Principal: "store-writer"}

type replacedFixture struct{ rootID, anchorID, entityID values.ElementID }

func seedReplacedStore(t *testing.T, path string) replacedFixture {
	t.Helper()
	fixture := replacedFixture{rootID: replacedID(100), anchorID: replacedID(101), entityID: replacedID(102)}
	authority, err := values.NewFactAuthorityRef()
	if err != nil {
		t.Fatal(err)
	}
	owner, err := commit.CreateDurable(path, authority, [32]byte{6}, corecontract.RealmInitialization{Realm: "fixture-realm", NextHop: "local:fixture"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	operations := element.New(owner, writeAllowed{})
	root, err := operations.CreateRoot(replacedActor, fixture.rootID, "fixture-root")
	if err != nil || root.Status != corecontract.MutationCommitted {
		t.Fatalf("fixture root: %#v err=%v", root, err)
	}
	anchor, err := operations.CreateAnchor(
		replacedActor, fixture.anchorID, replacedTuple(t, fixture.rootID, 0, 20), "fixture-anchor")

	if err != nil || anchor.Status != corecontract.MutationCommitted {
		t.Fatalf("fixture Anchor: %#v err=%v", anchor, err)
	}
	entity, err := operations.CreateEntity(
		replacedActor, fixture.entityID, replacedTuple(t, fixture.anchorID, 1, 2), "fixture-entity")

	if err != nil || entity.Status != corecontract.MutationCommitted {
		t.Fatalf("fixture Entity: %#v err=%v", entity, err)
	}
	stale, staleErr := operations.UpdateEntity(
		replacedActor, fixture.entityID, replacedTuple(t, fixture.anchorID, 1, 2),
		root.Record.Anchor.Revision, "fixture-stale")

	if !errors.Is(staleErr, corecontract.ErrRevisionConflict) || stale.Status != corecontract.MutationRejected {
		t.Fatalf("fixture stale decision: %#v err=%v", stale, staleErr)
	}
	conflict, conflictErr := operations.CreateRoot(replacedActor, replacedID(103), "fixture-conflict")
	if !errors.Is(conflictErr, corecontract.ErrConflict) || conflict.Status != corecontract.MutationRejected {
		t.Fatalf("fixture conflict decision: %#v err=%v", conflict, conflictErr)
	}
	if err := owner.CloseStore(); err != nil {
		t.Fatal(err)
	}
	return fixture
}

func replacedID(serial uint64) values.ElementID {
	return values.ElementID(fmt.Sprintf("20000000-0000-4000-8000-%012x", serial))
}

func replacedTuple(t *testing.T, parent values.ElementID, x, radius float64) corecontract.SpatialTuple {
	t.Helper()
	rotation, err := corecontract.NewRotation(1, 0, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	pose, err := corecontract.NewPose(corecontract.Vec3{X: x}, rotation)
	if err != nil {
		t.Fatal(err)
	}
	sphere, err := corecontract.NewBoundedSphere(radius)
	if err != nil {
		t.Fatal(err)
	}
	return corecontract.SpatialTuple{Parent: parent, LocalPose: pose, Range: sphere}
}

func newReplacementAuthority(t *testing.T, current values.FactAuthorityRef) values.FactAuthorityRef {
	t.Helper()
	ref, err := values.NewFactAuthorityRef()
	if err != nil || ref == current {
		t.Fatalf("new authority: %q err=%v", ref, err)
	}
	return ref
}
