package access

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"testing"
	"time"

	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
)

// localContact is the fixed local-profile contact used by these unit tests.
type localContact struct{}

func (localContact) CurrentContact() (contract.RealmAttachment, bool) {
	return contract.RealmAttachment{Realm: "admission-test", NextHop: "local:admission"}, true
}

type releaseContact struct{ current contract.RealmAttachment }

func (s *releaseContact) CurrentContact() (contract.RealmAttachment, bool) { return s.current, true }

type releasingAuthorizer struct {
	*authFixture
	released contract.RealmAttachment
	evidence [32]byte
	calls    int
	cleanup  error
}

func (a *releasingAuthorizer) ReleaseRealmEvidence(ctx context.Context, contact contract.RealmAttachment, evidence [32]byte) {
	a.released, a.evidence, a.cleanup = contact, evidence, ctx.Err()
	a.calls++
}

func TestAdmissionCleanupUsesOriginalContactAndKeepsRetainedView(t *testing.T) {
	contacts := &releaseContact{current: contract.RealmAttachment{Realm: "realm", NextHop: "https://realm.example:443"}}
	original := contacts.current
	auth := &releasingAuthorizer{authFixture: &authFixture{decision: values.AuthorizationValid, validation: values.AuthorizationValid, view: values.AuthorizationView{9}}}
	a := New(auth, contacts, values.PublicLimits{ContextTTL: time.Minute, ConcurrentReads: 1})
	ctx, cancel := context.WithCancel(context.Background())
	view, permit, err := a.Authorize(ctx, values.CallerContext{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.Recheck(ctx, values.CallerContext{}, view, nil, false); err != nil || auth.calls != 0 {
		t.Fatalf("recheck released retained evidence: %v, calls=%d", err, auth.calls)
	}
	contacts.current.NextHop = "https://replacement.example:443"
	cancel()
	a.ReleasePermit(ctx, permit)
	if auth.calls != 1 || auth.released != original || auth.evidence != [32]byte(view) || auth.cleanup != nil {
		t.Fatalf("cleanup changed contact or inherited cancellation: %+v", auth)
	}
	contacts.current.NextHop = "local:realm"
	_, local, err := a.Authorize(context.Background(), values.CallerContext{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	a.ReleasePermit(context.Background(), local)
	if auth.calls != 1 {
		t.Fatal("local profile acquired remote cleanup")
	}
}

func TestAdmissionContextBindsOnlyTheAcquiredOwner(t *testing.T) {
	limits := values.PublicLimits{ContextTTL: time.Minute, ConcurrentReads: 1}
	a, b := New(boolOnly{}, localContact{}, limits), New(boolOnly{}, localContact{}, limits)
	ctx := context.Background()
	if err := a.Acquire(); err != nil {
		t.Fatal(err)
	}
	defer a.Release()
	borrowed := WithAdmission(ctx, a)
	if !HasAdmission(borrowed, a) || HasAdmission(borrowed, b) || HasAdmission(borrowed, nil) || HasAdmission(ctx, a) {
		t.Fatal("admission context changed owner or leaked into parent")
	}
	if !errors.Is(a.Acquire(), values.PublicResourceExhausted) {
		t.Fatal("marker refunded admission")
	}
	if err := b.Acquire(); err != nil {
		t.Fatal(err)
	}
	b.Release()
	if a.active.Load() != 1 || b.active.Load() != 0 {
		t.Fatal("counters no longer independent")
	}
	owner := values.FactAuthorityRef("50000000-0000-4000-8000-000000000001")
	boundary := values.ElementID("50000000-0000-4000-8000-000000000002")
	got := ClosureRequirements(owner, boundary, "read", "observe", "read")
	want := []values.Requirement{{Permission: "read", Scope: values.AnchorClosureScope(owner, boundary)}, {Permission: "observe", Scope: values.AnchorClosureScope(owner, boundary)}, {Permission: "read", Scope: values.AnchorClosureScope(owner, boundary)}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("closure requirements changed: %v", got)
	}
}

type authFixture struct {
	decision, validation values.AuthorizationDecision
	caller               values.CallerContext
	view                 values.AuthorizationView
	requirements         []values.Requirement
}

func (*authFixture) AuthorizePrincipal(values.PrincipalRef, ...values.Requirement) values.AuthorizationDecision {
	return values.AuthorizationValid
}
func (a *authFixture) AuthorizeView(ctx context.Context, c values.CallerContext, r ...values.Requirement) (values.AuthorizationDecision, values.AuthorizationView) {
	a.caller, a.requirements = c, append([]values.Requirement(nil), r...)
	return a.decision, a.view
}
func (a *authFixture) ValidateView(ctx context.Context, c values.CallerContext, v values.AuthorizationView, r ...values.Requirement) values.AuthorizationDecision {
	a.caller, a.view, a.requirements = c, v, append([]values.Requirement(nil), r...)
	return a.validation
}
func (*authFixture) CaptureDenialView(context.Context, values.CallerContext, values.Requirement) (values.AuthorizationDecision, values.AuthorizationDenialView) {
	return values.AuthorizationValid, values.AuthorizationDenialView{}
}
func (*authFixture) ValidateDenialView(_ context.Context, _ values.CallerContext, view values.AuthorizationDenialView, _ values.Requirement) values.AuthorizationDecision {
	if view == (values.AuthorizationDenialView{}) {
		return values.AuthorizationDenied
	}
	return values.AuthorizationChanged
}

// A bool stub carries no continuity evidence: every port stays unavailable.
type boolOnly struct{}

func (boolOnly) AuthorizePrincipal(values.PrincipalRef, ...values.Requirement) values.AuthorizationDecision {
	return values.AuthorizationUnavailable
}
func (boolOnly) AuthorizeView(context.Context, values.CallerContext, ...values.Requirement) (values.AuthorizationDecision, values.AuthorizationView) {
	return values.AuthorizationUnavailable, values.AuthorizationView{}
}
func (boolOnly) ValidateView(context.Context, values.CallerContext, values.AuthorizationView, ...values.Requirement) values.AuthorizationDecision {
	return values.AuthorizationUnavailable
}
func (boolOnly) CaptureDenialView(context.Context, values.CallerContext, values.Requirement) (values.AuthorizationDecision, values.AuthorizationDenialView) {
	return values.AuthorizationUnavailable, values.AuthorizationDenialView{}
}
func (boolOnly) ValidateDenialView(context.Context, values.CallerContext, values.AuthorizationDenialView, values.Requirement) values.AuthorizationDecision {
	return values.AuthorizationUnavailable
}

func TestAdmissionDisabledAndUnavailable(t *testing.T) {
	a := New(&authFixture{}, localContact{}, values.PublicLimits{ConcurrentReads: 4})
	if a != nil || a.Enabled() || !errors.Is(a.Acquire(), values.PublicUnavailable) {
		t.Fatal("disabled public access acquired capacity")
	}
	for _, a := range []*Admission{nil, New(boolOnly{}, localContact{}, values.PublicLimits{ContextTTL: time.Minute, ConcurrentReads: 1})} {
		v, _, err := a.Authorize(context.Background(), values.CallerContext{}, nil)
		if v != (values.AuthorizationView{}) || !errors.Is(err, values.PublicUnavailable) {
			t.Fatal("bool authorization became continuity evidence")
		}
		if !errors.Is(a.Validate(context.Background(), values.CallerContext{}, v, nil, false, nil), values.PublicUnavailable) {
			t.Fatal("missing continuity validated")
		}
	}
}

func TestAdmissionPreservesAuthorizationAndFailureProjection(t *testing.T) {
	auth := &authFixture{decision: values.AuthorizationValid, validation: values.AuthorizationValid, view: values.AuthorizationView{9}}
	a := New(auth, localContact{}, values.PublicLimits{ContextTTL: time.Minute, ConcurrentReads: 1})
	caller := values.CallerContext{Proof: [32]byte{5}}
	reqs := []values.Requirement{{Permission: "first"}, {Permission: "second"}}
	v, permit, err := a.Authorize(context.Background(), caller, reqs)
	if err != nil || v != auth.view || auth.caller != caller || !reflect.DeepEqual(auth.requirements, reqs) {
		t.Fatal("authorize changed its original input or view")
	}
	if err := a.Validate(context.Background(), caller, v, reqs, false, permit); err != nil || auth.caller != caller || auth.view != v || !reflect.DeepEqual(auth.requirements, reqs) {
		t.Fatal("validate changed its captured input")
	}
	for _, tc := range []struct {
		decision      values.AuthorizationDecision
		read, observe error
	}{
		{values.AuthorizationDenied, values.PublicDenied, values.PublicDenied},
		{values.AuthorizationChanged, values.PublicInvalidContext, values.PublicGap},
		{values.AuthorizationUnavailable, values.PublicUnavailable, values.PublicUnavailable},
		{values.AuthorizationDecision(255), values.PublicUnavailable, values.PublicUnavailable},
	} {
		auth.decision, auth.validation = tc.decision, tc.decision
		if got, _, err := a.Authorize(context.Background(), caller, reqs); got != (values.AuthorizationView{}) || !errors.Is(err, tc.read) {
			t.Fatalf("authorize %v: %v", tc.decision, err)
		}
		if err := a.Validate(context.Background(), caller, v, reqs, false, permit); !errors.Is(err, tc.read) {
			t.Fatalf("read %v: %v", tc.decision, err)
		}
		if err := a.Validate(context.Background(), caller, v, reqs, true, permit); !errors.Is(err, tc.observe) {
			t.Fatalf("observe %v: %v", tc.decision, err)
		}
	}
	if a.active.Load() != 0 {
		t.Fatal("authorization itself acquired a request slot")
	}
}

func TestAdmissionSharesOneBoundAcrossConcurrentConsumers(t *testing.T) {
	limits := values.PublicLimits{ContextTTL: time.Minute, ConcurrentReads: 8}
	a := New(boolOnly{}, localContact{}, limits)
	limits.ConcurrentReads = 64 // startup values are captured, not a live config.
	start, release := make(chan struct{}), make(chan struct{})
	results := make(chan error, 64)
	var workers sync.WaitGroup
	for i := 0; i < 64; i++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			<-start
			err := a.Acquire()
			results <- err
			if err == nil {
				<-release
				a.Release()
			}
		}()
	}
	close(start)
	accepted := 0
	for i := 0; i < 64; i++ {
		if err := <-results; err == nil {
			accepted++
		} else if !errors.Is(err, values.PublicResourceExhausted) {
			t.Errorf("unexpected admission error: %v", err)
		}
	}
	if accepted != 8 || a.active.Load() != 8 {
		t.Errorf("accepted=%d, active=%d", accepted, a.active.Load())
	}
	close(release)
	workers.Wait()
	if a.active.Load() != 0 {
		t.Fatal("rejection or completion leaked capacity")
	}
	if err := a.Acquire(); err != nil {
		t.Fatal(err)
	}
	a.Release()
}
