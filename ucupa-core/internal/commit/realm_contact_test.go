package commit

import (
	"errors"
	"strings"
	"testing"

	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
)

func TestRealmContactRetainsExactDecisionsWithoutABAReturn(t *testing.T) {
	owner, err := values.NewFactAuthorityRef()
	if err != nil {
		t.Fatal(err)
	}
	scope := [32]byte{1}
	actor := values.PrincipalRef{Realm: "realm-a", Principal: "operator"}
	state, err := newRealmContact(contract.RealmInitialization{Realm: actor.Realm, NextHop: "local:realm-a"})
	if err != nil {
		t.Fatal(err)
	}
	first := state.attachment(owner, scope)
	if first.Owner != owner || first.Realm != actor.Realm || first.NextHop != "local:realm-a" || first.Revision == (contract.RealmContactRevision{}) {
		t.Fatalf("initial contact = %+v", first)
	}
	change := contract.RealmContactUpdate{OperationID: "move-b", Owner: owner, Realm: actor.Realm, Expected: first.Revision, NextHop: "local:realm-b"}
	updated, original, err := state.prepareRealmContact(owner, scope, actor, change)
	if err != nil || updated == nil || original.Decision != contract.RealmContactCommitted || original.Contact.NextHop != change.NextHop {
		t.Fatalf("A to B = %+v, %+v, %v", updated, original, err)
	}
	if err := updated.validateRealmContactHistory(owner, scope); err != nil {
		t.Fatalf("A to B history: %v", err)
	}
	if retry, got, err := updated.prepareRealmContact(owner, scope, actor, change); err != nil || retry != nil || got != original {
		t.Fatalf("same operation must return original result: %+v, %+v, %v", retry, got, err)
	}
	changedRequest := change
	changedRequest.NextHop = "local:realm-c"
	if _, _, err := updated.prepareRealmContact(owner, scope, actor, changedRequest); !errors.Is(err, contract.ErrOperationConflict) {
		t.Fatalf("reused ID with different address = %v", err)
	}
	stale := contract.RealmContactUpdate{OperationID: "stale", Owner: owner, Realm: actor.Realm, Expected: first.Revision, NextHop: "local:realm-c"}
	rejectedState, rejected, err := updated.prepareRealmContact(owner, scope, actor, stale)
	if err != nil || rejectedState == nil || rejected.Decision != contract.RealmContactRejected || rejected.Contact != original.Contact {
		t.Fatalf("stale comparison = %+v, %+v, %v", rejectedState, rejected, err)
	}
	back := contract.RealmContactUpdate{OperationID: "back-a", Owner: owner, Realm: actor.Realm, Expected: rejected.Contact.Revision, NextHop: first.NextHop}
	returned, backResult, err := rejectedState.prepareRealmContact(owner, scope, actor, back)
	if err != nil || returned == nil || backResult.Decision != contract.RealmContactCommitted || backResult.Contact.NextHop != first.NextHop || backResult.Contact.Revision == first.Revision {
		t.Fatalf("B to A must advance contact generation: %+v, %+v, %v", returned, backResult, err)
	}
	if err := returned.validateRealmContactHistory(owner, scope); err != nil {
		t.Fatalf("A to B to A history: %v", err)
	}
	bad := returned.clone()
	bad.events[0].result.Contact.NextHop = "local:forged"
	if err := bad.validateRealmContactHistory(owner, scope); !errors.Is(err, contract.ErrStore) {
		t.Fatalf("tampered retained result = %v", err)
	}
}

func TestRealmContactHistoryHasExactEncodedCapacity(t *testing.T) {
	owner, err := values.NewFactAuthorityRef()
	if err != nil {
		t.Fatal(err)
	}
	scope := [32]byte{5}
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
