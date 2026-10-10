package namespace

import (
	"context"
	"encoding/binary"
	"errors"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"

	"ucupa/shared/values"
	"ucupa/ucupa-facet/contract"
)

var testActor = values.PrincipalRef{Realm: "test-realm", Principal: "writer"}

const testElement values.ElementID = "00000000-0000-4000-8000-00000000fa01"
const testCore values.FactAuthorityRef = "00000000-0000-4000-8000-00000000ca01"

var testInitial = contract.RealmInitialization{Realm: "test-realm", NextHop: "local:test-realm"}

type testAuthorization struct {
	deny         atomic.Bool
	calls        atomic.Int64
	mu           sync.Mutex
	requirements []values.Requirement
	hook         func()
}

func (a *testAuthorization) Check(context.Context, values.NextHop, values.RealmID) error { return nil }

func (a *testAuthorization) AuthorizePrincipal(_ values.NextHop, _ values.RealmID, _ values.PrincipalRef, rs ...values.Requirement) values.AuthorizationDecision {
	a.calls.Add(1)
	a.mu.Lock()
	a.requirements = append([]values.Requirement(nil), rs...)
	a.mu.Unlock()
	if a.hook != nil {
		a.hook()
	}
	if a.deny.Load() {
		return values.AuthorizationDenied
	}
	return values.AuthorizationValid
}

func (a *testAuthorization) AuthorizeView(_ context.Context, address values.NextHop, realm values.RealmID, caller values.CallerContext, rs ...values.Requirement) (values.AuthorizationDecision, values.AuthorizationView) {
	decision := a.AuthorizePrincipal(address, realm, caller.Subject, rs...)
	if decision != values.AuthorizationValid {
		return decision, values.AuthorizationView{}
	}
	return values.AuthorizationValid, values.AuthorizationView{1}
}

func (a *testAuthorization) ValidateView(_ context.Context, _ values.NextHop, _ values.RealmID, _ values.CallerContext, _ values.AuthorizationView, _ ...values.Requirement) values.AuthorizationDecision {
	if a.deny.Load() {
		return values.AuthorizationDenied
	}
	return values.AuthorizationValid
}

func (a *testAuthorization) CaptureDenialView(_ context.Context, _ values.NextHop, _ values.RealmID, _ values.CallerContext, _ values.Requirement) (values.AuthorizationDecision, values.AuthorizationDenialView) {
	return values.AuthorizationDenied, values.AuthorizationDenialView{}
}

func (a *testAuthorization) ValidateDenialView(_ context.Context, _ values.NextHop, _ values.RealmID, _ values.CallerContext, _ values.AuthorizationDenialView, _ values.Requirement) values.AuthorizationDecision {
	return values.AuthorizationDenied
}

type testSource struct {
	mu      sync.Mutex
	current contract.CoreEvidence
	address values.NextHop
	offline bool
	calls   atomic.Int64
	hook    func()
}

func (s *testSource) read(ctx context.Context, address values.NextHop, k values.FacetAcceptanceKey) (contract.CoreEvidence, error) {
	s.calls.Add(1)
	s.mu.Lock()
	e := s.current
	offline := s.offline
	expected := s.address
	hook := s.hook
	s.mu.Unlock()
	if hook != nil {
		hook()
	}
	if offline || address != expected || e.Authority != k.Core {
		return contract.CoreEvidence{}, contract.ErrUnavailable
	}
	return e, nil
}
func (s *testSource) ReadProposal(c context.Context, a values.NextHop, k values.FacetAcceptanceKey) (contract.CoreEvidence, error) {
	return s.read(c, a, k)
}
func (s *testSource) ReadAttachment(c context.Context, a values.NextHop, k values.FacetAcceptanceKey) (contract.CoreEvidence, error) {
	e, err := s.read(c, a, k)
	e.Pending = values.FacetAcceptanceKey{}
	return e, err
}

func (s *testSource) ReadSuccessor(context.Context, values.NextHop, values.FactAuthorityRef, values.ElementID) (contract.CoreSuccessorRead, error) {
	panic("unexpected successor read")
}

type fixture struct {
	f        *Owner
	auth     *testAuthorization
	source   *testSource
	key      values.FacetAcceptanceKey
	typ      values.FacetTypeRef
	path     string
	accepted contract.MutationResult
}

func newFixture(t *testing.T, durable bool) *fixture {
	t.Helper()
	x := &fixture{auth: &testAuthorization{}, source: &testSource{address: "local:core"}, typ: fixtureType("example")}
	var err error
	if durable {
		x.path = filepath.Join(t.TempDir(), "facet.db")
		x.f, err = CreateDurable(x.auth, x.source, testInitial, x.path, nil, nil)
	} else {
		x.f, err = New(x.auth, x.source, testInitial, nil)
	}
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = Close(x.f) })
	x.key = testKey(x.f.AuthorityRef(), testCore, testElement, 2)
	x.source.current = testProof(x.key, contract.CoreNone, 2)
	x.source.current.Pending = x.key
	x.accepted, err = x.f.AcceptAttachment(context.Background(), testActor, x.key, "local:core", "accept")
	if err != nil {
		t.Fatal(err)
	}
	x.source.current = testProof(x.key, contract.CoreActive, 3)
	return x
}
func testKey(f, c values.FactAuthorityRef, e values.ElementID, serial uint64) values.FacetAcceptanceKey {
	var raw [76]byte
	raw[0] = 1
	copy(raw[32:68], e)
	binary.BigEndian.PutUint64(raw[68:], serial)
	p, _ := values.NewAttachmentRevision(raw[:])
	return values.FacetAcceptanceKey{Core: c, Facet: f, Element: e, Proposal: p}
}
func testProof(k values.FacetAcceptanceKey, state contract.CoreState, serial uint64) contract.CoreEvidence {
	var raw [40]byte
	raw[0] = 1
	binary.BigEndian.PutUint64(raw[32:], serial)
	cut, _ := values.NewCoreCutEvidence(raw[:])
	e := contract.CoreEvidence{Authority: k.Core, Element: k.Element, State: state, Cut: cut}
	if state != contract.CoreMissing {
		e.AttachmentRevision = testKey(k.Facet, k.Core, k.Element, serial).Proposal
	}
	if state == contract.CoreActive || state == contract.CoreDetached {
		e.Active = k
	}
	return e
}
func (x *fixture) setState(state contract.CoreState) {
	x.source.mu.Lock()
	x.source.current = testProof(x.key, state, 4)
	x.source.mu.Unlock()
}
func (x *fixture) put(t *testing.T, op contract.OperationID, payload string, pre contract.PutPrecondition) contract.MutationResult {
	t.Helper()
	r, err := x.f.Put(context.Background(), testActor, x.key, x.typ, fixturePayload(payload), pre, op)
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
	return r
}
func mustError(t *testing.T, err, want error) {
	t.Helper()
	if !errors.Is(err, want) {
		t.Fatalf("got %v; want %v", err, want)
	}
}
func profiles(t *testing.T, run func(*testing.T, *fixture)) {
	t.Helper()
	for _, d := range []bool{false, true} {
		name := "memory"
		if d {
			name = "durable"
		}
		t.Run(name, func(t *testing.T) { run(t, newFixture(t, d)) })
	}
}
