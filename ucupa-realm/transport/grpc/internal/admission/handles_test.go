package admission

import (
	"sync"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"ucupa/shared/values"
)

// fakeClock is the deterministic white-box seam for expiry and lease floors.
// Production always runs on time.Now; only these in-package tests move it.
type fakeClock struct {
	mu sync.Mutex
	t  time.Time
}

func newFakeClock() *fakeClock { return &fakeClock{t: time.Now()} }

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *fakeClock) advance(d time.Duration) {
	c.mu.Lock()
	c.t = c.t.Add(d)
	c.mu.Unlock()
}

const handleTestOwner values.FactAuthorityRef = "00000000-0000-4000-8000-000000000001"
const handleOtherOwner values.FactAuthorityRef = "00000000-0000-4000-8000-000000000002"

func handleTestCaller(principal values.PrincipalID) values.CallerContext {
	return values.CallerContext{Subject: values.PrincipalRef{Realm: "realm-a", Principal: principal}}
}

func TestHandleRegistryLifecycle(t *testing.T) {
	clock := newFakeClock()
	registry := New(Config{}, clock.Now)
	entry := Entry{
		Kind:    Caller,
		Owner:   handleTestOwner,
		Caller:  handleTestCaller("user"),
		Expires: clock.Now().Add(10 * time.Second),
	}
	token, err := registry.Issue(entry, 8, 8)
	if err != nil {
		t.Fatal(err)
	}
	if registry.Size() != 1 {
		t.Fatalf("size = %d", registry.Size())
	}
	stored, err := registry.Lookup(token)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Kind != Caller || stored.Owner != handleTestOwner || stored.Caller.Subject.Principal != "user" {
		t.Fatalf("stored entry = %+v", stored)
	}
	registry.Release(token)
	if registry.Size() != 0 {
		t.Fatalf("size after release = %d", registry.Size())
	}
	if _, err := registry.Lookup(token); status.Code(err) != codes.Unauthenticated {
		t.Fatalf("lookup after release = %v", err)
	}
	// Release stays idempotent and never fails.
	registry.Release(token)
}

func TestHandleRegistryExpiryBoundary(t *testing.T) {
	clock := newFakeClock()
	registry := New(Config{}, clock.Now)
	token, err := registry.Issue(Entry{Kind: Caller, Owner: handleTestOwner, Expires: clock.Now().Add(10 * time.Second)}, 8, 8)
	if err != nil {
		t.Fatal(err)
	}
	clock.advance(10*time.Second - time.Nanosecond)
	if _, err := registry.Lookup(token); err != nil {
		t.Fatalf("lookup before expiry = %v", err)
	}
	clock.advance(time.Nanosecond)
	if _, err := registry.Lookup(token); status.Code(err) != codes.Unauthenticated {
		t.Fatalf("lookup at expiry = %v", err)
	}
	if registry.Size() != 0 {
		t.Fatalf("size at expiry = %d", registry.Size())
	}
}

func TestHandleRegistryTotalCapacity(t *testing.T) {
	clock := newFakeClock()
	registry := New(Config{}, clock.Now)
	entry := Entry{Kind: Caller, Owner: handleTestOwner, Expires: clock.Now().Add(time.Hour)}
	for i := 0; i < 2; i++ {
		if _, err := registry.Issue(entry, 2, 8); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := registry.Issue(entry, 2, 8); status.Code(err) != codes.ResourceExhausted {
		t.Fatalf("third issue = %v", err)
	}
	// Expiring the stored entries frees the bounded capacity again.
	clock.advance(2 * time.Hour)
	if _, err := registry.Issue(entry, 2, 8); err != nil {
		t.Fatal(err)
	}
}

func TestHandleRegistryPerCallerCapacity(t *testing.T) {
	clock := newFakeClock()
	registry := New(Config{}, clock.Now)
	base := Entry{Kind: Caller, Owner: handleTestOwner, Expires: clock.Now().Add(time.Hour)}
	first := base
	first.Caller = handleTestCaller("user")
	if _, err := registry.Issue(first, 8, 1); err != nil {
		t.Fatal(err)
	}
	// The same owner and subject hits the per-caller bound.
	if _, err := registry.Issue(first, 8, 1); status.Code(err) != codes.ResourceExhausted {
		t.Fatalf("second same-subject issue = %v", err)
	}
	second := base
	second.Caller = handleTestCaller("other-user")
	if _, err := registry.Issue(second, 8, 1); err != nil {
		t.Fatalf("different subject rejected: %v", err)
	}
	third := base
	third.Caller = handleTestCaller("user")
	third.Owner = handleOtherOwner
	if _, err := registry.Issue(third, 8, 1); err != nil {
		t.Fatalf("different owner rejected: %v", err)
	}
}

func TestHandleRegistryResetDropsEveryReference(t *testing.T) {
	clock := newFakeClock()
	registry := New(Config{}, clock.Now)
	tokens := make([][32]byte, 0, 3)
	for _, principal := range []values.PrincipalID{"a", "b", "c"} {
		entry := Entry{Kind: Caller, Owner: handleTestOwner, Caller: handleTestCaller(principal), Expires: clock.Now().Add(time.Hour)}
		token, err := registry.Issue(entry, 8, 8)
		if err != nil {
			t.Fatal(err)
		}
		tokens = append(tokens, token)
	}
	registry.Reset()
	if registry.Size() != 0 {
		t.Fatalf("size after reset = %d", registry.Size())
	}
	for _, token := range tokens {
		if _, err := registry.Lookup(token); status.Code(err) != codes.Unauthenticated {
			t.Fatalf("lookup after reset = %v", err)
		}
	}
}

func TestHandleRegistryIssuesDistinctTokens(t *testing.T) {
	clock := newFakeClock()
	registry := New(Config{}, clock.Now)
	entry := Entry{Kind: Caller, Owner: handleTestOwner, Caller: handleTestCaller("user"), Expires: clock.Now().Add(time.Hour)}
	first, err := registry.Issue(entry, 8, 8)
	if err != nil {
		t.Fatal(err)
	}
	second, err := registry.Issue(entry, 8, 8)
	if err != nil {
		t.Fatal(err)
	}
	if first == second || first == ([32]byte{}) || second == ([32]byte{}) {
		t.Fatalf("tokens are not fresh unguessable references: %x %x", first, second)
	}
}
