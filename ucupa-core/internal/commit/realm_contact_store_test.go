package commit

import (
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"

	bolt "go.etcd.io/bbolt"

	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
)

func TestRealmContactSharesCoreStoreAndRejectsRewrittenProjection(t *testing.T) {
	owner, err := values.NewFactAuthorityRef()
	if err != nil {
		t.Fatal(err)
	}
	scope := [32]byte{33}
	state := newCoreState(owner, scope)
	state.realmContact, err = newRealmContact(contract.RealmInitialization{Realm: "realm-a", NextHop: "local:realm-a"})
	if err != nil {
		t.Fatal(err)
	}
	actor := values.PrincipalRef{Realm: "realm-a", Principal: "operator"}
	first := state.realmContact.attachment(owner, scope)
	request := contract.RealmContactUpdate{OperationID: "move-b", Owner: owner, Realm: first.Realm, Expected: first.Revision, NextHop: "local:realm-b"}
	next, _, err := state.realmContact.prepareRealmContact(owner, scope, actor, request)
	if err != nil {
		t.Fatal(err)
	}
	state.realmContact = *next
	db, err := bolt.Open(filepath.Join(t.TempDir(), "core.db"), 0o600, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := initializeStore(db, state, initialWriter(owner, nil)); err != nil {
		t.Fatalf("initialize with original contact history: %v", err)
	}
	loaded, err := loadStoredCore(db)
	if err != nil {
		t.Fatalf("reopen contact through Core Store: %v", err)
	}
	if !sameCompleteCoreState(loaded, state) {
		t.Fatal("reopened contact differs from committed Core state")
	}
	j := initialWriter(owner, nil)
	runtimeOwner := &Owner{authority: owner, state: loaded, store: &localStore{db: db, scope: scope, authority: owner, writer: j, epoch: j.Epoch}}
	current, err := runtimeOwner.RealmContactLocked()
	if err != nil {
		t.Fatal(err)
	}
	back := contract.RealmContactUpdate{OperationID: "back-a", Owner: owner, Realm: current.Realm, Expected: current.Revision, NextHop: "local:realm-a"}
	runtimeOwner.Lock()
	committed, err := runtimeOwner.ApplyRealmContactLocked(actor, back)
	runtimeOwner.Unlock()
	if err != nil || committed.Decision != contract.RealmContactCommitted {
		t.Fatalf("publish contact through original Core Store: %+v, %v", committed, err)
	}
	reopenedAfterCommit, err := loadStoredCore(db)
	if err != nil || reopenedAfterCommit.realmContact.current != back.NextHop || !sameCompleteCoreState(reopenedAfterCommit, runtimeOwner.state) {
		t.Fatalf("contact update did not survive original Store reload: %v", err)
	}
	current = runtimeOwner.state.realmContact.attachment(owner, scope)
	failed := contract.RealmContactUpdate{OperationID: "precommit-failure", Owner: owner, Realm: current.Realm,
		Expected: current.Revision, NextHop: "local:realm-c"}
	runtimeOwner.store.failBeforeCommit = func() error { return errors.New("injected rollback") }
	runtimeOwner.Lock()
	_, err = runtimeOwner.ApplyRealmContactLocked(actor, failed)
	runtimeOwner.Unlock()
	if !errors.Is(err, contract.ErrStore) || runtimeOwner.state.realmContact.current != current.NextHop {
		t.Fatalf("precommit failure published contact: %v", err)
	}
	runtimeOwner.store.failAfterCommit = func() error { return errors.New("lost commit response") }
	runtimeOwner.Lock()
	_, err = runtimeOwner.ApplyRealmContactLocked(actor, failed)
	runtimeOwner.Unlock()
	if !errors.Is(err, contract.ErrStore) || runtimeOwner.state.realmContact.current != failed.NextHop {
		t.Fatalf("committed response loss did not reconcile original Store: %v", err)
	}
	runtimeOwner.Lock()
	recovered, err := runtimeOwner.ApplyRealmContactLocked(actor, failed)
	runtimeOwner.Unlock()
	if err != nil || recovered.Decision != contract.RealmContactCommitted || recovered.Contact.NextHop != failed.NextHop {
		t.Fatalf("original contact result not recoverable after response loss: %+v, %v", recovered, err)
	}
	if err := db.Update(func(tx *bolt.Tx) error {
		wire := runtimeOwner.state.realmContact.wire()
		wire.CurrentHop = "local:forged"
		encoded, err := json.Marshal(wire)
		if err != nil {
			return err
		}
		return tx.Bucket(elementBucketName).Put(realmContactKey, encoded)
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := loadStoredCore(db); err == nil || !errors.Is(err, contract.ErrStore) {
		t.Fatalf("rewritten current address must not bypass contact history: %v", err)
	}
}

func TestRealmContactRecordIsRequiredAtLoad(t *testing.T) {
	owner, err := values.NewFactAuthorityRef()
	if err != nil {
		t.Fatal(err)
	}
	scope := [32]byte{55}
	state := newCoreState(owner, scope)
	state.realmContact, err = newRealmContact(contract.RealmInitialization{Realm: "realm-a", NextHop: "local:realm-a"})
	if err != nil {
		t.Fatal(err)
	}
	db, err := bolt.Open(filepath.Join(t.TempDir(), "core.db"), 0o600, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := initializeStore(db, state, initialWriter(owner, nil)); err != nil {
		t.Fatalf("initialize with original contact record: %v", err)
	}
	if err := db.Update(func(tx *bolt.Tx) error {
		return tx.Bucket(elementBucketName).Delete(realmContactKey)
	}); err != nil {
		t.Fatal(err)
	}
	// A Store without the bound contact record is never opened and never
	// reseeded from assembly configuration.
	if _, err := loadStoredCore(db); err == nil {
		t.Fatal("load accepted a Store without a Realm contact record")
	}
}

func TestRealmContactStorePreservesOpaqueOperationAndIdentityBytes(t *testing.T) {
	owner, err := values.NewFactAuthorityRef()
	if err != nil {
		t.Fatal(err)
	}
	scope := [32]byte{44}
	realmID := values.RealmID(string([]byte{'r', 0xff}))
	actor := values.PrincipalRef{Realm: realmID, Principal: values.PrincipalID(string([]byte{'p', 0xfe}))}
	state, err := newRealmContact(contract.RealmInitialization{Realm: realmID, NextHop: "local:realm"})
	if err != nil {
		t.Fatal(err)
	}
	request := contract.RealmContactUpdate{OperationID: contract.OperationID(string([]byte{'o', 0xfd})), Owner: owner,
		Realm: realmID, Expected: state.attachment(owner, scope).Revision, NextHop: "local:other"}
	updated, _, err := state.prepareRealmContact(owner, scope, actor, request)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := encodeRealmContact(*updated, owner, scope)
	if err != nil {
		t.Fatal(err)
	}
	reopened, err := decodeRealmContact(encoded, owner, scope)
	if err != nil || !sameRealmContact(reopened, *updated) {
		t.Fatalf("opaque contact history lost exact bytes: %v", err)
	}
}
