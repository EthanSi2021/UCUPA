package namespace

import (
	"context"
	"strings"
	"testing"

	"ucupa/shared/values"
	"ucupa/ucupa-facet/contract"
)

func TestAuthorizationPrecedesLookupSourceAndState(t *testing.T) {
	x := newFixture(t, false)
	x.auth.deny.Store(true)
	before := x.source.calls.Load()
	_, err := x.f.Get(context.Background(), testActor, x.key, x.typ)
	mustError(t, err, contract.ErrDenied)
	_, err = x.f.Put(context.Background(), testActor, x.key, x.typ, []byte("null"), contract.ExpectAbsent(), "denied")
	mustError(t, err, contract.ErrDenied)
	_, err = x.f.ListTypes(context.Background(), testActor, x.key)
	mustError(t, err, contract.ErrDenied)
	_, err = x.f.ReadAcceptance(testActor, x.key)
	mustError(t, err, contract.ErrDenied)
	_, err = x.f.AcceptAttachment(context.Background(), testActor, x.key, "local:core", "accept")
	mustError(t, err, contract.ErrDenied)
	_, err = x.f.UpdateCoreNextHop(testActor, "contact", x.key, x.accepted.NextHopRevision, "local:other")
	mustError(t, err, contract.ErrDenied)
	if before != x.source.calls.Load() {
		t.Fatal("denied called Core")
	}
	x.auth.deny.Store(false)
	x.put(t, "put", "data", contract.ExpectAbsent())
	x.auth.mu.Lock()
	scope := x.auth.requirements[0].Scope
	x.auth.mu.Unlock()
	owner, key, ok := scope.FacetKeyTarget()
	if !ok || owner != x.f.AuthorityRef() || key.Element != x.key.Element || key.Type != x.typ {
		t.Fatal("unqualified key scope")
	}
	_, err = x.f.ListTypes(context.Background(), testActor, x.key)
	if err != nil {
		t.Fatal(err)
	}
	x.auth.mu.Lock()
	scope = x.auth.requirements[0].Scope
	x.auth.mu.Unlock()
	o, e, ok := scope.FacetElementTarget()
	if !ok || o != x.f.AuthorityRef() || e != x.key.Element {
		t.Fatal("namespace scope")
	}
}
func TestInvalidInputsNeverCallExternalSources(t *testing.T) {
	x := newFixture(t, false)
	ctx := context.Background()
	var emptyPrecondition contract.PutPrecondition
	var emptyContactRevision contract.ContactRevision
	before := x.source.calls.Load()
	bad := x.key
	bad.Facet = fixtureFacetOwner
	_, err := x.f.Get(ctx, testActor, bad, x.typ)
	mustError(t, err, contract.ErrInvalid)
	_, err = x.f.Put(ctx, testActor, x.key, x.typ, nil, emptyPrecondition, "op")
	mustError(t, err, contract.ErrInvalid)
	_, err = x.f.Put(ctx, testActor, x.key, x.typ, []byte("null"), contract.ExpectAbsent(), contract.OperationID(strings.Repeat("x", 129)))
	mustError(t, err, contract.ErrInvalid)
	_, err = x.f.Put(ctx, testActor, x.key, x.typ, make([]byte, maxPayloadBytes+1), contract.ExpectAbsent(), "large")
	mustError(t, err, contract.ErrCapacity)
	_, err = x.f.Get(ctx, testActor, x.key, values.FacetTypeRef{})
	mustError(t, err, contract.ErrInvalid)
	_, err = x.f.Get(ctx, values.PrincipalRef{}, x.key, x.typ)
	mustError(t, err, contract.ErrDenied)
	_, err = x.f.AcceptAttachment(ctx, testActor, x.key, "", "bad")
	mustError(t, err, contract.ErrInvalid)
	_, err = x.f.UpdateCoreNextHop(testActor, "op", x.key, emptyContactRevision, "local:core")
	mustError(t, err, contract.ErrInvalid)
	if before != x.source.calls.Load() {
		t.Fatal("invalid calls reached Core")
	}
	if f, err := New(nil, x.source, testInitial, nil); f != nil || err != contract.ErrInvalid {
		t.Fatal("nil auth")
	}
	if f, err := New(x.auth, nil, testInitial, nil); f != nil || err != contract.ErrInvalid {
		t.Fatal("nil source")
	}
}
func TestWrongAddressIdentityAndMissingAcceptanceFailClosed(t *testing.T) {
	x := newFixture(t, false)
	ctx := context.Background()
	key := testKey(x.key.Facet, x.key.Core, x.key.Element, 7)
	_, err := x.f.Get(ctx, testActor, key, x.typ)
	mustError(t, err, contract.ErrUnavailable)
	x.source.offline = true
	_, err = x.f.Get(ctx, testActor, x.key, x.typ)
	mustError(t, err, contract.ErrUnavailable)
	x.source.offline = false
	x.source.address = "local:different"
	_, err = x.f.Get(ctx, testActor, x.key, x.typ)
	mustError(t, err, contract.ErrUnavailable)
	x.source.address = "local:core"
	x.source.current.Authority = fixtureFacetOwner
	_, err = x.f.Get(ctx, testActor, x.key, x.typ)
	mustError(t, err, contract.ErrUnavailable)
	delete(x.f.state.Contacts, packKey(x.key).id())
	_, err = x.f.ReadAcceptance(testActor, x.key)
	mustError(t, err, contract.ErrUnavailable)
}
