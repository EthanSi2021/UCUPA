package facet_test

import (
	"context"
	"reflect"
	"testing"
	"time"

	"ucupa/shared/values"
	"ucupa/ucupa-core/internal/access"
	"ucupa/ucupa-core/internal/access/registry"
	"ucupa/ucupa-core/internal/facet"
)

// The service ports share the actual fact owner, admission counter and
// publication registry. No alternate service state or observation publisher
// is assembled for these tests.
type serviceFixture struct {
	fixture
	resources *registry.Registry
	policy    *servicePolicy
}

type servicePolicy struct {
	*testAuthorizer
	authorized [][]values.Requirement
	validated  [][]values.Requirement
}

func (p *servicePolicy) AuthorizeView(ctx context.Context, c values.CallerContext, reqs ...values.Requirement) (values.AuthorizationDecision, values.AuthorizationView) {
	p.authorized = append(p.authorized, append([]values.Requirement(nil), reqs...))
	return p.testAuthorizer.AuthorizeView(ctx, c, reqs...)
}

func (p *servicePolicy) ValidateView(ctx context.Context, c values.CallerContext, v values.AuthorizationView, reqs ...values.Requirement) values.AuthorizationDecision {
	p.validated = append(p.validated, append([]values.Requirement(nil), reqs...))
	return p.testAuthorizer.ValidateView(ctx, c, v, reqs...)
}

func serviceLimits() values.PublicLimits {
	return values.PublicLimits{ContextTTL: time.Minute, ObservationTTL: time.Minute,
		Contexts: 32, PerCaller: 32, Groups: 4, Bytes: 1 << 20, ConcurrentReads: 1}
}

func newServiceFixture(t *testing.T, durable bool, limits values.PublicLimits) serviceFixture {
	t.Helper()
	f := newFixture(t, durable)
	f.auth.caller = publicCaller
	policy := &servicePolicy{testAuthorizer: f.auth}
	f.admission = access.New(policy, f.owner, limits)
	resources := registry.New(f.owner.AuthorityRef(), limits, f.owner, nil)
	f.owner.Lock()
	err := f.owner.SetPublicationSinkLocked(resources)
	f.owner.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	f.facets = facet.New(f.owner, policy, f.admission, resources)
	result := serviceFixture{fixture: f, resources: resources, policy: policy}
	t.Cleanup(result.close)
	return result
}

func (f serviceFixture) close() {
	f.owner.Lock()
	f.owner.BeginCloseLocked()
	f.resources.CloseLocked()
	f.owner.Unlock()
	f.resources.DrainRetired()
}

func (f serviceFixture) resetCalls() {
	f.policy.authorized, f.policy.validated = nil, nil
}

func (f serviceFixture) expectCalls(t *testing.T, authorize bool, permissions ...values.PermissionID) {
	t.Helper()
	want := make([]values.Requirement, len(permissions))
	for i, permission := range permissions {
		want[i] = values.Requirement{Permission: permission, Scope: values.CoreElementScope(f.owner.AuthorityRef(), facetEntity)}
	}
	var authorized [][]values.Requirement
	if authorize {
		authorized = [][]values.Requirement{want}
	}
	if !reflect.DeepEqual(f.policy.authorized, authorized) || !reflect.DeepEqual(f.policy.validated, [][]values.Requirement{want}) {
		t.Fatalf("authorization calls = %v / %v, want %v / %v", f.policy.authorized, f.policy.validated, authorized, want)
	}
}

// expectGuardedCalls asserts one retained-view operation: no fresh
// authorization, while the online recheck and the in-lock guard each validated
// exactly the same requirement set.
func (f serviceFixture) expectGuardedCalls(t *testing.T, permissions ...values.PermissionID) {
	t.Helper()
	want := make([]values.Requirement, len(permissions))
	for i, permission := range permissions {
		want[i] = values.Requirement{Permission: permission, Scope: values.CoreElementScope(f.owner.AuthorityRef(), facetEntity)}
	}
	if len(f.policy.authorized) != 0 || !reflect.DeepEqual(f.policy.validated, [][]values.Requirement{want, want}) {
		t.Fatalf("authorization calls = %v / %v, want none / [%v %v]", f.policy.authorized, f.policy.validated, want, want)
	}
}

func (f serviceFixture) begin(t *testing.T, preparation values.PublicToken) values.PublicToken {
	t.Helper()
	f.owner.Lock()
	defer f.owner.Unlock()
	prep, err := f.resources.ResolvePreparationLocked(publicCaller, preparation)
	if err != nil {
		t.Fatal(err)
	}
	cursor, err := f.resources.BeginLocalLocked(prep.Ref)
	if err != nil {
		t.Fatal(err)
	}
	return cursor
}

func (f serviceFixture) poll(t *testing.T, token values.PublicToken) ([]registry.ChangeGroup, values.PublicToken) {
	t.Helper()
	f.owner.Lock()
	defer f.owner.Unlock()
	cursor, err := f.resources.ResolveCursorLocked(publicCaller, token)
	if err != nil {
		t.Fatal(err)
	}
	groups, next, err := f.resources.PollLocalLocked(cursor.Ref, 4)
	if err != nil {
		t.Fatal(err)
	}
	return groups, next
}

// This uses the registry's real issue/remove operations to prove an aborted
// preparation refunded its invisible parent; it does not inspect counters.
func (f serviceFixture) probeSingleContext() error {
	f.owner.Lock()
	defer f.owner.Unlock()
	read, err := f.resources.IssueLocalReadLocked(publicCaller, facetEntity, registry.Ref{}, f.owner.CurrentCutLocked(), values.AuthorizationView{}, nil, "", "")
	if err == nil {
		f.resources.RemoveLocked(read.Ref)
	}
	return err
}

func (f serviceFixture) removePreparation(t *testing.T, token values.PublicToken) {
	t.Helper()
	f.owner.Lock()
	defer f.owner.Unlock()
	prep, err := f.resources.ResolvePreparationLocked(publicCaller, token)
	if err != nil {
		t.Fatal(err)
	}
	f.resources.RemoveLocked(prep.Ref)
}

func serviceToken(t *testing.T, owner values.FactAuthorityRef, kind string) values.PublicToken {
	t.Helper()
	token, err := values.NewPublicToken(owner, kind)
	if err != nil {
		t.Fatal(err)
	}
	return token
}

func serviceProposalToken(t *testing.T, f serviceFixture) values.PublicToken {
	t.Helper()
	r, err := f.facets.InspectFacetControl(context.Background(), publicCaller, facetEntity)
	if err != nil || !r.PendingProposal.Valid() {
		t.Fatalf("inspect pending = %#v, %v", r, err)
	}
	return r.PendingProposal
}
