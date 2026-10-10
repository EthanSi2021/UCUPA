package namespace

import (
	"context"
	"encoding/binary"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"ucupa/shared/values"
	facetcontract "ucupa/ucupa-facet/contract"
)

type acceptanceViewAuth struct {
	decision atomic.Uint32
	version  atomic.Uint64
	checks   atomic.Uint64
	mu       sync.Mutex
	hook     func()
	required []values.Requirement
}

func (a *acceptanceViewAuth) Check(context.Context, values.NextHop, values.RealmID) error { return nil }

func (a *acceptanceViewAuth) AuthorizePrincipal(values.NextHop, values.RealmID, values.PrincipalRef, ...values.Requirement) values.AuthorizationDecision {
	panic("public relationship operation downgraded caller to subject")
}

func (a *acceptanceViewAuth) AuthorizeView(_ context.Context, _ values.NextHop, _ values.RealmID, c values.CallerContext, rs ...values.Requirement) (values.AuthorizationDecision, values.AuthorizationView) {
	a.mu.Lock()
	a.required = append([]values.Requirement(nil), rs...)
	a.mu.Unlock()
	var v values.AuthorizationView
	binary.BigEndian.PutUint64(v[:8], a.version.Load())
	if !c.Subject.Valid() {
		return values.AuthorizationDenied, v
	}
	return values.AuthorizationDecision(a.decision.Load()), v
}

func (a *acceptanceViewAuth) ValidateView(_ context.Context, _ values.NextHop, _ values.RealmID, c values.CallerContext, v values.AuthorizationView, _ ...values.Requirement) values.AuthorizationDecision {
	a.checks.Add(1)
	a.mu.Lock()
	hook := a.hook
	a.mu.Unlock()
	if hook != nil {
		hook()
	}
	if !c.Subject.Valid() {
		return values.AuthorizationDenied
	}
	if d := values.AuthorizationDecision(a.decision.Load()); d != values.AuthorizationValid {
		return d
	}
	if binary.BigEndian.Uint64(v[:8]) != a.version.Load() {
		return values.AuthorizationChanged
	}
	return values.AuthorizationValid
}

func (a *acceptanceViewAuth) CaptureDenialView(_ context.Context, _ values.NextHop, _ values.RealmID, c values.CallerContext, _ values.Requirement) (values.AuthorizationDecision, values.AuthorizationDenialView) {
	var v values.AuthorizationDenialView
	binary.BigEndian.PutUint64(v[:8], a.version.Load())
	if !c.Subject.Valid() {
		return values.AuthorizationDenied, v
	}
	return values.AuthorizationDecision(a.decision.Load()), v
}

func (a *acceptanceViewAuth) ValidateDenialView(_ context.Context, _ values.NextHop, _ values.RealmID, c values.CallerContext, v values.AuthorizationDenialView, _ values.Requirement) values.AuthorizationDecision {
	if !c.Subject.Valid() {
		return values.AuthorizationDenied
	}
	if d := values.AuthorizationDecision(a.decision.Load()); d != values.AuthorizationValid {
		return d
	}
	if binary.BigEndian.Uint64(v[:8]) != a.version.Load() {
		return values.AuthorizationChanged
	}
	return values.AuthorizationValid
}

type acceptanceProposalSource struct {
	*testSource
	mu2      sync.Mutex
	ref      facetcontract.PublicAcceptanceRef
	resolved facetcontract.CoreProposalRead
	hook     func()
	err      error
	reads    atomic.Int64
}

func (s *acceptanceProposalSource) ResolveProposal(ctx context.Context, address values.NextHop, ref facetcontract.PublicAcceptanceRef) (facetcontract.CoreProposalRead, error) {
	s.reads.Add(1)
	s.mu2.Lock()
	resolved, expected, hook, err := s.resolved, s.ref, s.hook, s.err
	s.mu2.Unlock()
	if hook != nil {
		hook()
	}
	if err != nil {
		return facetcontract.CoreProposalRead{}, err
	}
	if address != "local:core" || ref != expected || ctx.Err() != nil {
		return facetcontract.CoreProposalRead{}, facetcontract.ErrUnavailable
	}
	return resolved, nil
}

type publicAcceptanceFixture struct {
	f      *Owner
	auth   *acceptanceViewAuth
	source *acceptanceProposalSource
	caller values.CallerContext
	ref    facetcontract.PublicAcceptanceRef
	key    values.FacetAcceptanceKey
	path   string
	config *values.PublicAccessPolicy
}

func newPublicAcceptanceFixture(t *testing.T, durable bool) *publicAcceptanceFixture {
	t.Helper()
	x := &publicAcceptanceFixture{
		auth:   &acceptanceViewAuth{},
		source: &acceptanceProposalSource{testSource: &testSource{}},
		caller: values.CallerContext{Subject: testActor, Credential: [16]byte{1}, Proof: [32]byte{1}},
		config: &values.PublicAccessPolicy{ContextTTL: time.Minute, ObservationTTL: time.Minute},
	}
	x.auth.decision.Store(uint32(values.AuthorizationValid))
	x.auth.version.Store(1)
	var err error
	if durable {
		x.path = filepath.Join(t.TempDir(), "facet.db")
		x.f, err = CreateDurable(x.auth, x.source, testInitial, x.path, x.config, nil)
	} else {
		x.f, err = New(x.auth, x.source, testInitial, x.config)
	}
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = Close(x.f) })
	x.key = testKey(x.f.AuthorityRef(), testCore, testElement, 2)
	token, _ := values.NewPublicToken(testCore, "attachment-revision")
	x.ref = facetcontract.PublicAcceptanceRef{Core: testCore, Element: testElement, Proposal: token}
	evidence := testProof(x.key, facetcontract.CoreNone, 2)
	evidence.Pending = x.key
	x.source.ref, x.source.resolved = x.ref, facetcontract.CoreProposalRead{Key: x.key, Evidence: evidence, Proposal: token}
	return x
}

func (x *publicAcceptanceFixture) accept(t *testing.T, id facetcontract.OperationID) facetcontract.PublicAcceptanceOutcome {
	t.Helper()
	r, err := x.f.AcceptAttachmentControl(context.Background(), x.caller, x.ref, "local:core", id)
	if err != nil {
		t.Fatal(err)
	}
	return r
}
