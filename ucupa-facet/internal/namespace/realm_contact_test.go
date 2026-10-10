package namespace

import (
	"errors"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	bolt "go.etcd.io/bbolt"

	"ucupa/shared/values"
	"ucupa/ucupa-facet/contract"
)

func TestRealmContactRetainsDecisionsAndRejectsRewrittenHistory(t *testing.T) {
	owner, err := values.NewFactAuthorityRef()
	if err != nil {
		t.Fatal(err)
	}
	scope := [32]byte{8}
	actor := values.PrincipalRef{Realm: "realm-a", Principal: "operator"}
	contact, err := newRealmContact(contract.RealmInitialization{Realm: actor.Realm, NextHop: "local:realm-a"})
	if err != nil {
		t.Fatal(err)
	}
	first := contact.attachment(owner, scope)
	request := contract.RealmContactUpdate{OperationID: "move-b", Owner: owner, Realm: actor.Realm, Expected: first.Revision, NextHop: "local:realm-b"}
	next, result, err := contact.prepareRealmContact(owner, scope, actor, request)
	if err != nil || next == nil || result.Decision != contract.RealmContactCommitted {
		t.Fatalf("first update = %+v, %+v, %v", next, result, err)
	}
	if repeated, got, err := next.prepareRealmContact(owner, scope, actor, request); repeated != nil || got != result || err != nil {
		t.Fatalf("original result retry = %+v, %+v, %v", repeated, got, err)
	}
	other := request
	other.NextHop = "local:realm-c"
	if _, _, err := next.prepareRealmContact(owner, scope, actor, other); !errors.Is(err, contract.ErrOperationConflict) {
		t.Fatalf("changed request reused operation = %v", err)
	}
	back := contract.RealmContactUpdate{OperationID: "back-a", Owner: owner, Realm: actor.Realm, Expected: result.Contact.Revision, NextHop: first.NextHop}
	returned, backResult, err := next.prepareRealmContact(owner, scope, actor, back)
	if err != nil || returned == nil || backResult.Contact.Revision == first.Revision || backResult.Contact.NextHop != first.NextHop {
		t.Fatalf("A to B to A reused first revision: %+v, %+v, %v", returned, backResult, err)
	}
	if err := returned.validateRealmContactHistory(owner, scope); err != nil {
		t.Fatalf("valid history: %v", err)
	}
	bad := returned.clone()
	bad.Events[0].Result.Contact.NextHop = "local:forged"
	if err := bad.validateRealmContactHistory(owner, scope); !errors.Is(err, contract.ErrStore) {
		t.Fatalf("forged original result = %v", err)
	}
}

func TestRealmContactHistoryHasExactEncodedCapacity(t *testing.T) {
	owner, err := values.NewFactAuthorityRef()
	if err != nil {
		t.Fatal(err)
	}
	scope := [32]byte{6}
	state, err := newRealmContact(contract.RealmInitialization{Realm: "realm-a", NextHop: "local:realm-a"})
	if err != nil {
		t.Fatal(err)
	}
	request := contract.RealmContactUpdate{OperationID: "oversize", Owner: owner, Realm: "realm-a",
		Expected: state.attachment(owner, scope).Revision, NextHop: "local:realm-b"}
	actor := values.PrincipalRef{Realm: "realm-a", Principal: values.PrincipalID(strings.Repeat("p", maxRealmContactBytes))}
	if next, _, err := state.prepareRealmContact(owner, scope, actor, request); next != nil || !errors.Is(err, contract.ErrCapacity) {
		t.Fatalf("oversized retained receipt = %+v, %v", next, err)
	}
	if err := state.validateRealmContactHistory(owner, scope); err != nil {
		t.Fatalf("capacity refusal mutated original contact: %v", err)
	}
}

func TestRealmContactPartOfFacetStoreDocument(t *testing.T) {
	owner, err := values.NewFactAuthorityRef()
	if err != nil {
		t.Fatal(err)
	}
	scope := [32]byte{9}
	contact, err := newRealmContact(contract.RealmInitialization{Realm: "realm-a", NextHop: "local:realm-a"})
	if err != nil {
		t.Fatal(err)
	}
	facts := emptyState()
	facts.PublicKey = [32]byte{1}
	facts.RealmContact = &contact
	doc := storeDocument{Authority: owner, Scope: scope, Facts: facts}
	raw, err := encodeDocument(doc)
	if err != nil {
		t.Fatalf("encode owner facts and contact together: %v", err)
	}
	reopened, err := decodeDocument(raw)
	if err != nil || reopened.Facts.RealmContact == nil || !reflect.DeepEqual(*reopened.Facts.RealmContact, contact) {
		t.Fatalf("reopen owner document contact = %+v, %v", reopened.Facts.RealmContact, err)
	}
	reopened.Facts.RealmContact.Current = "local:forged"
	if _, err := encodeDocument(reopened); !errors.Is(err, contract.ErrStore) {
		t.Fatalf("contact projection without decision history = %v", err)
	}
}

func TestRealmContactCommitUsesOriginalFacetStore(t *testing.T) {
	owner, err := values.NewFactAuthorityRef()
	if err != nil {
		t.Fatal(err)
	}
	scope := [32]byte{10}
	contact, err := newRealmContact(contract.RealmInitialization{Realm: "realm-a", NextHop: "local:realm-a"})
	if err != nil {
		t.Fatal(err)
	}
	facts := emptyState()
	facts.PublicKey = [32]byte{3}
	facts.RealmContact = &contact
	f := &Owner{ownerState: &ownerState{authority: owner, scope: scope, state: facts, operations: map[byteString]int{}}}
	db, err := bolt.Open(filepath.Join(t.TempDir(), "facet.db"), 0o600, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	raw, err := encodeDocument(storeDocument{Authority: owner, Scope: scope, Facts: facts})
	if err != nil {
		t.Fatal(err)
	}
	j := initialWriter(owner, nil)
	if err := db.Update(func(tx *bolt.Tx) error {
		bucket, err := tx.CreateBucket(recordBucketName)
		if err != nil {
			return err
		}
		if err := bucket.Put(formatMarkerKey, formatMarker); err != nil {
			return err
		}
		if err := bucket.Put(documentKey, raw); err != nil {
			return err
		}
		return putWriter(tx, j)
	}); err != nil {
		t.Fatal(err)
	}
	f.store = &localStore{db: db, writer: j, epoch: j.Epoch}
	initial, err := f.RealmContact()
	if err != nil {
		t.Fatal(err)
	}
	actor := values.PrincipalRef{Realm: "realm-a", Principal: "operator"}
	request := contract.RealmContactUpdate{OperationID: "move-b", Owner: owner, Realm: initial.Realm, Expected: initial.Revision, NextHop: "local:realm-b"}
	result, err := f.commitRealmContact(actor, request)
	if err != nil || result.Decision != contract.RealmContactCommitted {
		t.Fatalf("same Store contact commit = %+v, %v", result, err)
	}
	loaded, err := loadStore(db)
	if err != nil || loaded.Facts.RealmContact == nil || loaded.Facts.RealmContact.Current != request.NextHop ||
		!reflect.DeepEqual(loaded.Facts, f.state) {
		t.Fatalf("contact commit did not survive original Store reopen: %v", err)
	}
	if repeated, err := f.commitRealmContact(actor, request); err != nil || repeated != result {
		t.Fatalf("same operation did not return original receipt: %+v, %v", repeated, err)
	}
	current, err := f.RealmContact()
	if err != nil {
		t.Fatal(err)
	}
	failure := contract.RealmContactUpdate{OperationID: "move-c", Owner: owner, Realm: current.Realm,
		Expected: current.Revision, NextHop: "local:realm-c"}
	f.store.failBeforeCommit = func() error { return errors.New("injected rollback") }
	if _, err := f.commitRealmContact(actor, failure); !errors.Is(err, contract.ErrUnavailable) || f.state.RealmContact.Current != current.NextHop {
		t.Fatalf("precommit failure published contact: %v", err)
	}
	f.store.failAfterCommit = func() error { return errors.New("lost commit response") }
	if _, err := f.commitRealmContact(actor, failure); !errors.Is(err, contract.ErrIndeterminate) || f.state.RealmContact.Current != failure.NextHop {
		t.Fatalf("response loss did not reconcile original Store: %v", err)
	}
	if original, err := f.commitRealmContact(actor, failure); err != nil || original.Decision != contract.RealmContactCommitted || original.Contact.NextHop != failure.NextHop {
		t.Fatalf("original result not recoverable after response loss: %+v, %v", original, err)
	}
}

func TestRealmContactDocumentPreservesOpaqueIdentityAndOperationBytes(t *testing.T) {
	owner, err := values.NewFactAuthorityRef()
	if err != nil {
		t.Fatal(err)
	}
	scope := [32]byte{11}
	realmID := values.RealmID(string([]byte{'r', 0xff}))
	actor := values.PrincipalRef{Realm: realmID, Principal: values.PrincipalID(string([]byte{'p', 0xfe}))}
	contact, err := newRealmContact(contract.RealmInitialization{Realm: realmID, NextHop: "local:realm"})
	if err != nil {
		t.Fatal(err)
	}
	request := contract.RealmContactUpdate{OperationID: contract.OperationID(string([]byte{'o', 0xfd})), Owner: owner,
		Realm: realmID, Expected: contact.attachment(owner, scope).Revision, NextHop: "local:other"}
	next, _, err := contact.prepareRealmContact(owner, scope, actor, request)
	if err != nil {
		t.Fatal(err)
	}
	facts := emptyState()
	facts.PublicKey = [32]byte{4}
	facts.RealmContact = next
	raw, err := encodeDocument(storeDocument{Authority: owner, Scope: scope, Facts: facts})
	if err != nil {
		t.Fatal(err)
	}
	reopened, err := decodeDocument(raw)
	if err != nil || reopened.Facts.RealmContact == nil || !reflect.DeepEqual(*reopened.Facts.RealmContact, *next) {
		t.Fatalf("opaque contact document lost exact bytes: %v", err)
	}
}

// The public Instance surface must never grow an unauthenticated contact
// entry again: committing a contact change and reading another operation's
// commit receipt stay behind the authenticated maintenance operations.
func TestOwnerPublicSurfaceHasNoUnauthenticatedContactEntry(t *testing.T) {
	typ := reflect.TypeOf((*Owner)(nil))
	if _, found := typ.MethodByName("UpdateRealmContact"); !found {
		t.Fatal("surface check does not include the authenticated pointer methods")
	}
	for i := 0; i < typ.NumMethod(); i++ {
		switch name := typ.Method(i).Name; name {
		case "CommitRealmContact", "RealmContactOutcome":
			t.Fatalf("Owner re-exports unauthenticated contact entry %s", name)
		}
	}
}
