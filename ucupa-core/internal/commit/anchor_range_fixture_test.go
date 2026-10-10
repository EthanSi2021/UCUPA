package commit_test

import (
	"crypto/rand"
	"errors"
	"fmt"
	"testing"

	"ucupa/shared/values"
	corecontract "ucupa/ucupa-core/contract"
	"ucupa/ucupa-core/internal/commit"
	"ucupa/ucupa-core/internal/element"
)

var rangeStoreActor = values.PrincipalRef{Realm: "store-realm", Principal: "store-writer"}

type rangeStoreOwner struct {
	owner    *commit.Owner
	elements *element.Operations
}

type rangeStoreFixture struct {
	rootID   values.ElementID
	anchorID values.ElementID
	entityID values.ElementID
}

func seedRangeStore(t *testing.T, path string) rangeStoreFixture {
	t.Helper()
	fixture := rangeStoreFixture{rootID: rangeStoreID(100), anchorID: rangeStoreID(101), entityID: rangeStoreID(102)}
	spatial := createRangeStore(t, path)
	root, err := spatial.elements.CreateRoot(rangeStoreActor, fixture.rootID, "fixture-root")
	if err != nil || root.Status != corecontract.MutationCommitted {
		t.Fatalf("fixture root: %#v err=%v", root, err)
	}
	anchor, err := spatial.elements.CreateAnchor(
		rangeStoreActor, fixture.anchorID, rangeStoreTuple(t, fixture.rootID, 0, 20), "fixture-anchor")

	if err != nil || anchor.Status != corecontract.MutationCommitted {
		t.Fatalf("fixture Anchor: %#v err=%v", anchor, err)
	}
	entity, err := spatial.elements.CreateEntity(
		rangeStoreActor, fixture.entityID, rangeStoreTuple(t, fixture.anchorID, 1, 2), "fixture-entity")

	if err != nil || entity.Status != corecontract.MutationCommitted {
		t.Fatalf("fixture Entity: %#v err=%v", entity, err)
	}
	stale, staleErr := spatial.elements.UpdateEntity(
		rangeStoreActor, fixture.entityID, rangeStoreTuple(t, fixture.anchorID, 1, 2),
		root.Record.Anchor.Revision, "fixture-stale")

	if !errors.Is(staleErr, corecontract.ErrRevisionConflict) || stale.Status != corecontract.MutationRejected {
		t.Fatalf("fixture stale decision: %#v err=%v", stale, staleErr)
	}
	conflict, conflictErr := spatial.elements.CreateRoot(rangeStoreActor, rangeStoreID(103), "fixture-conflict")
	if !errors.Is(conflictErr, corecontract.ErrConflict) || conflict.Status != corecontract.MutationRejected {
		t.Fatalf("fixture conflict decision: %#v err=%v", conflict, conflictErr)
	}
	closeRangeStore(t, spatial)
	return fixture
}

func createRangeStore(t *testing.T, path string) *rangeStoreOwner {
	t.Helper()
	authority, err := values.NewFactAuthorityRef()
	if err != nil {
		t.Fatal(err)
	}
	var scope [32]byte
	if _, err := rand.Read(scope[:]); err != nil {
		t.Fatal(err)
	}
	owner, err := commit.CreateDurable(path, authority, scope, corecontract.RealmInitialization{Realm: "fixture-realm", NextHop: "local:fixture"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = owner.CloseStore() })
	return &rangeStoreOwner{owner: owner, elements: element.New(owner, writeAllowed{})}
}

func openRangeStore(t *testing.T, path string) *rangeStoreOwner {
	t.Helper()
	owner, err := commit.OpenDurable(path, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = owner.CloseStore() })
	return &rangeStoreOwner{owner: owner, elements: element.New(owner, writeAllowed{})}
}

func closeRangeStore(t *testing.T, spatial *rangeStoreOwner) {
	t.Helper()
	if err := spatial.owner.CloseStore(); err != nil {
		t.Fatal(err)
	}
}

func rangeStoreID(serial uint64) values.ElementID {
	return values.ElementID(fmt.Sprintf("20000000-0000-4000-8000-%012x", serial))
}

func rangeStoreTuple(t *testing.T, parent values.ElementID, x, radius float64) corecontract.SpatialTuple {
	t.Helper()
	return storeTuple(t, parent, x, radius)
}
