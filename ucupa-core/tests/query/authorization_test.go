package query_test

import (
	"errors"
	"math"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
	"time"

	"ucupa/shared/values"
	corecontract "ucupa/ucupa-core/contract"
	core "ucupa/ucupa-core/instance"
)

type queryAuthCall struct {
	regionActor  values.PrincipalRef
	requirements []values.Requirement
}

type queryAuthPolicy struct {
	authority  values.FactAuthorityRef
	mu         sync.Mutex
	setup      bool
	scope      values.ElementID
	allowOther bool
	denied     bool
	denyAt     int
	blockAt    int
	queryCalls int
	calls      []queryAuthCall
	entered    chan struct{}
	release    chan struct{}
}

func (policy *queryAuthPolicy) Authorize(principal values.PrincipalRef, requirements ...values.Requirement) bool {
	policy.mu.Lock()
	policy.calls = append(policy.calls, queryAuthCall{
		regionActor: principal, requirements: append([]values.Requirement(nil), requirements...),
	})
	if policy.setup {
		policy.mu.Unlock()
		return true
	}
	isQuery := false
	for _, requirement := range requirements {
		isQuery = isQuery || requirement.Permission == corecontract.PermissionQueryRegion
	}
	if !isQuery {
		allowed := policy.allowOther
		policy.mu.Unlock()
		return allowed
	}
	policy.queryCalls++
	call := policy.queryCalls
	block := policy.blockAt == call
	entered, release := policy.entered, policy.release
	policy.mu.Unlock()
	if block {
		close(entered)
		<-release
	}
	policy.mu.Lock()
	defer policy.mu.Unlock()
	if len(requirements) != 1 || requirements[0].Permission != corecontract.PermissionQueryRegion {
		return false
	}
	owner, target, ok := requirements[0].Scope.CoreElementTarget()
	return ok && owner == policy.authority && target == policy.scope && !policy.denied && (policy.denyAt == 0 || call != policy.denyAt)
}

func (policy *queryAuthPolicy) configure(scope values.ElementID, denyAt, blockAt int, allowOther bool) {
	policy.mu.Lock()
	defer policy.mu.Unlock()
	policy.setup = false
	policy.scope, policy.denyAt, policy.blockAt, policy.allowOther = scope, denyAt, blockAt, allowOther
	policy.denied = false
	policy.queryCalls = 0
	policy.calls = nil
	policy.entered = make(chan struct{})
	policy.release = make(chan struct{})
}

func (policy *queryAuthPolicy) revoke() {
	policy.mu.Lock()
	defer policy.mu.Unlock()
	policy.denied = true
}

func (policy *queryAuthPolicy) recordedCalls() []queryAuthCall {
	policy.mu.Lock()
	defer policy.mu.Unlock()
	calls := make([]queryAuthCall, len(policy.calls))
	for index, call := range policy.calls {
		calls[index] = queryAuthCall{regionActor: call.regionActor, requirements: append([]values.Requirement(nil), call.requirements...)}
	}
	return calls
}

func TestQueryAuthorizationUsesOnlyExactScopeAndDoesNotGrantRead(t *testing.T) {
	policy := &queryAuthPolicy{setup: true}
	c, fixture := queryAuthorizationFixture(t, policy)
	query := queryAuthorizationRequest(fixture.scope, fixture.basis)
	policy.configure(fixture.scope, 0, 0, false)
	page, err := c.Queries().QueryRegion(regionActor, query, nil, 256, nil)
	if err != nil || page.Status != corecontract.QueryComplete || page.Gap != corecontract.QueryNoGap || len(page.Matches) != 3 {
		t.Fatalf("scope-only query: %#v err=%v", page, err)
	}
	assertQueryAuthorizationCalls(t, c.AuthorityRef(), policy.recordedCalls(), regionActor, fixture.scope, 2)
	var foundLeaf bool
	for _, match := range page.Matches {
		if match.ElementID == fixture.entity {
			foundLeaf = true
			if want := []values.ElementID{fixture.basis, fixture.entity}; !reflect.DeepEqual(match.Path, want) {
				t.Fatalf("query-authorized path: got=%v want=%v", match.Path, want)
			}
		}
	}
	if !foundLeaf {
		t.Fatal("an Element without exact Read permission was incorrectly hidden from the authorized query")
	}
	read, err := c.Elements().ReadElement(regionActor, fixture.entity, &page.Cut)
	if !errors.Is(err, corecontract.ErrDenied) || !reflect.DeepEqual(read, corecontract.ElementRead{}) {
		t.Fatalf("query permission granted exact read: %#v err=%v", read, err)
	}
	if _, err := c.Elements().ListDirect(regionActor, fixture.scope, 256, nil); !errors.Is(err, corecontract.ErrDenied) {
		t.Fatalf("query permission granted ListDirect: %v", err)
	}
	if _, err := c.Elements().ProjectToAncestor(regionActor, fixture.entity, fixture.scope, nil); !errors.Is(err, corecontract.ErrDenied) {
		t.Fatalf("query permission granted projection: %v", err)
	}

	// A grant for the parent query does not authorize a query with a different exact Scope.
	policy.configure(fixture.scope, 0, 0, false)
	other := queryAuthorizationRequest(fixture.basis, fixture.basis)
	page, err = c.Queries().QueryRegion(regionActor, other, nil, 256, nil)
	assertZeroQueryAuthorizationPage(t, page, err, corecontract.ErrDenied)
	assertQueryAuthorizationCalls(t, c.AuthorityRef(), policy.recordedCalls(), regionActor, fixture.basis, 1)
}

func TestQueryAuthorizationDenialPrecedesScopeAndBasisDisclosure(t *testing.T) {
	policy := &queryAuthPolicy{setup: true}
	c, fixture := queryAuthorizationFixture(t, policy)
	tests := []struct {
		name  string
		query corecontract.RegionQuery
		want  error
	}{
		{"valid", queryAuthorizationRequest(fixture.scope, fixture.basis), nil},
		{"missing Scope", queryAuthorizationRequest(regionID(5990), fixture.basis), corecontract.ErrInvalid},
		{"Entity Scope", queryAuthorizationRequest(fixture.entity, fixture.basis), corecontract.ErrInvalid},
		{"missing Basis", queryAuthorizationRequest(fixture.scope, regionID(5991)), corecontract.ErrInvalid},
		{"outside Basis", queryAuthorizationRequest(fixture.scope, fixture.outside), corecontract.ErrInvalid},
		{"Entity Basis", queryAuthorizationRequest(fixture.scope, fixture.entity), corecontract.ErrInvalid},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			for _, denyAt := range []int{1, 2} {
				policy.configure(test.query.StartAnchorID, denyAt, 0, false)
				page, err := c.Queries().QueryRegion(regionActor, test.query, nil, 256, nil)
				assertZeroQueryAuthorizationPage(t, page, err, corecontract.ErrDenied)
				assertQueryAuthorizationCalls(t, c.AuthorityRef(), policy.recordedCalls(), regionActor, test.query.StartAnchorID, denyAt)
			}
			policy.configure(test.query.StartAnchorID, 0, 0, false)
			page, err := c.Queries().QueryRegion(regionActor, test.query, nil, 256, nil)
			if test.want == nil {
				if err != nil || page.Status != corecontract.QueryComplete {
					t.Fatalf("authorized valid query: %#v err=%v", page, err)
				}
			} else {
				assertZeroQueryAuthorizationPage(t, page, err, test.want)
			}
			assertQueryAuthorizationCalls(t, c.AuthorityRef(), policy.recordedCalls(), regionActor, test.query.StartAnchorID, 2)
		})
	}
}

func TestQueryAuthorizationExactReadDoesNotGrantQuery(t *testing.T) {
	policy := &queryAuthPolicy{setup: true}
	c, fixture := queryAuthorizationFixture(t, policy)
	policy.configure(fixture.scope, 1, 0, true)
	read, err := c.Elements().ReadElement(regionActor, fixture.entity, nil)
	if err != nil || read.State != corecontract.ExistencePresent || read.Record == nil {
		t.Fatalf("authorized exact read: %#v err=%v", read, err)
	}
	policy.configure(fixture.scope, 1, 0, true)
	page, err := c.Queries().QueryRegion(regionActor, queryAuthorizationRequest(fixture.scope, fixture.basis), nil, 256, nil)
	assertZeroQueryAuthorizationPage(t, page, err, corecontract.ErrDenied)
	assertQueryAuthorizationCalls(t, c.AuthorityRef(), policy.recordedCalls(), regionActor, fixture.scope, 1)
}

func TestQueryAuthorizationCallbacksAreOutsideCoreLockAndCutIsRechecked(t *testing.T) {
	for _, test := range []struct {
		name     string
		blockAt  int
		badScope bool
		badBasis bool
	}{
		{name: "first authorization", blockAt: 1},
		{name: "second authorization", blockAt: 2},
		{name: "pending missing Scope", blockAt: 2, badScope: true},
		{name: "pending invalid Basis", blockAt: 2, badBasis: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			policy := &queryAuthPolicy{setup: true}
			c, fixture := queryAuthorizationFixture(t, policy)
			query := queryAuthorizationRequest(fixture.scope, fixture.basis)
			if test.badScope {
				query.StartAnchorID = regionID(5990)
			}
			if test.badBasis {
				query.BasisAnchorID = fixture.outside
			}
			policy.configure(query.StartAnchorID, 0, test.blockAt, true)
			var releaseOnce sync.Once
			release := func() { releaseOnce.Do(func() { close(policy.release) }) }
			defer release()
			queried := make(chan queryAuthorizationOutcome, 1)
			go func() {
				page, err := c.Queries().QueryRegion(regionActor, query, nil, 256, nil)
				queried <- queryAuthorizationOutcome{page, err}
			}()
			select {
			case <-policy.entered:
			case <-time.After(5 * time.Second):
				t.Fatal("query did not reach the blocking Authorizer")
			}
			final := regionTuple(t, fixture.scope, regionPose(t, 0, 0, 0), 1)
			mutated := make(chan error, 1)
			go func() {
				_, err := c.Elements().CreateEntity(regionActor, regionID(5090), final, "query-auth-concurrent-create")
				mutated <- err
			}()
			select {
			case err := <-mutated:
				if err != nil {
					t.Fatalf("concurrent mutation: %v", err)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("Authorizer retained the Core lock and blocked a concurrent mutation")
			}
			release()
			select {
			case outcome := <-queried:
				// P3 resolves the permission Scope on a provisional cut before
				// authorization; any intervening write requires a fresh query.
				assertZeroQueryAuthorizationPage(t, outcome.page, outcome.err, corecontract.ErrReadCutExpired)
			case <-time.After(5 * time.Second):
				t.Fatal("query did not finish after releasing the Authorizer")
			}
		})
	}
}

func TestQueryAuthorizationRevocationAtSecondCheckDiscardsResult(t *testing.T) {
	policy := &queryAuthPolicy{setup: true}
	c, fixture := queryAuthorizationFixture(t, policy)
	query := queryAuthorizationRequest(fixture.scope, fixture.basis)
	policy.configure(fixture.scope, 0, 2, false)
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(policy.release) }) }
	defer release()
	queried := make(chan queryAuthorizationOutcome, 1)
	go func() {
		page, err := c.Queries().QueryRegion(regionActor, query, nil, 1, nil)
		queried <- queryAuthorizationOutcome{page, err}
	}()
	select {
	case <-policy.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("query did not reach its second authorization check")
	}
	policy.revoke()
	release()
	select {
	case outcome := <-queried:
		assertZeroQueryAuthorizationPage(t, outcome.page, outcome.err, corecontract.ErrDenied)
	case <-time.After(5 * time.Second):
		t.Fatal("revoked query did not finish")
	}
	assertQueryAuthorizationCalls(t, c.AuthorityRef(), policy.recordedCalls(), regionActor, fixture.scope, 2)
}

func TestQueryAuthorizationEveryContinuationRequiresCurrentPermission(t *testing.T) {
	policy := &queryAuthPolicy{setup: true}
	c, fixture := queryAuthorizationFixture(t, policy)
	query := queryAuthorizationRequest(fixture.scope, fixture.basis)
	policy.configure(fixture.scope, 0, 0, false)
	first, err := c.Queries().QueryRegion(regionActor, query, nil, 1, nil)
	if err != nil || first.Continuation == nil {
		t.Fatalf("continuation fixture: %#v err=%v", first, err)
	}
	for _, denyAt := range []int{1, 2} {
		policy.configure(fixture.scope, denyAt, 0, false)
		page, err := c.Queries().QueryRegion(regionActor, query, nil, 1, first.Continuation)
		assertZeroQueryAuthorizationPage(t, page, err, corecontract.ErrDenied)
		assertQueryAuthorizationCalls(t, c.AuthorityRef(), policy.recordedCalls(), regionActor, fixture.scope, denyAt)
	}
}

func TestQueryAuthorizationTokenBindingPrecedesExpiredCut(t *testing.T) {
	policy := &queryAuthPolicy{setup: true}
	c, fixture := queryAuthorizationFixture(t, policy)
	query := queryAuthorizationRequest(fixture.scope, fixture.basis)
	policy.configure(fixture.scope, 0, 0, true)
	first, err := c.Queries().QueryRegion(regionActor, query, nil, 1, nil)
	if err != nil || first.Continuation == nil {
		t.Fatalf("continuation fixture: %#v err=%v", first, err)
	}
	regionCreateEntity(t, c, regionID(5091), regionTuple(t, fixture.scope, regionPose(t, 0, 0, 0), 1), "expire-query-token")
	otherActor := values.PrincipalRef{Realm: regionActor.Realm, Principal: "another-query-reader"}
	changedQuery := query
	changedQuery.Sphere.Center.X = 1
	missingScope := query
	missingScope.StartAnchorID = regionID(5990)
	for _, test := range []struct {
		name      string
		principal values.PrincipalRef
		query     corecontract.RegionQuery
		want      error
	}{
		{"matching binding", regionActor, query, corecontract.ErrReadCutExpired},
		{"another actor", otherActor, query, corecontract.ErrInvalid},
		{"another query", regionActor, changedQuery, corecontract.ErrInvalid},
		{"another missing Scope", regionActor, missingScope, corecontract.ErrInvalid},
	} {
		t.Run(test.name, func(t *testing.T) {
			policy.configure(fixture.scope, 0, 0, false)
			page, err := c.Queries().QueryRegion(test.principal, test.query, nil, 1, first.Continuation)
			assertZeroQueryAuthorizationPage(t, page, err, test.want)
			assertQueryAuthorizationCalls(t, c.AuthorityRef(), policy.recordedCalls(), test.principal, fixture.scope, 2)
			policy.configure(fixture.scope, 1, 0, false)
			page, err = c.Queries().QueryRegion(test.principal, test.query, nil, 1, first.Continuation)
			assertZeroQueryAuthorizationPage(t, page, err, corecontract.ErrDenied)
		})
	}

	foreignPolicy := &queryAuthPolicy{setup: true}
	foreign, foreignFixture := queryAuthorizationFixture(t, foreignPolicy)
	foreignPolicy.configure(foreignFixture.scope, 0, 0, false)
	foreignPage, err := foreign.Queries().QueryRegion(regionActor, query, nil, 1, nil)
	if err != nil || foreignPage.Continuation == nil {
		t.Fatalf("foreign continuation fixture: %#v err=%v", foreignPage, err)
	}
	policy.configure(fixture.scope, 0, 0, false)
	page, err := c.Queries().QueryRegion(regionActor, query, nil, 1, foreignPage.Continuation)
	assertZeroQueryAuthorizationPage(t, page, err, corecontract.ErrInvalid)
	assertQueryAuthorizationCalls(t, c.AuthorityRef(), policy.recordedCalls(), regionActor, fixture.scope, 2)
}

func TestQueryAuthorizationCanonicalZeroKeepsContinuationBinding(t *testing.T) {
	policy := &queryAuthPolicy{setup: true}
	c, fixture := queryAuthorizationFixture(t, policy)
	query := queryAuthorizationRequest(fixture.scope, fixture.basis)
	policy.configure(fixture.scope, 0, 0, false)
	first, err := c.Queries().QueryRegion(regionActor, query, nil, 1, nil)
	if err != nil || first.Continuation == nil {
		t.Fatalf("continuation fixture: %#v err=%v", first, err)
	}
	normal, err := c.Queries().QueryRegion(regionActor, query, nil, 1, first.Continuation)
	if err != nil {
		t.Fatal(err)
	}
	query.Sphere.Center = corecontract.Vec3{
		X: math.Copysign(0, -1), Y: math.Copysign(0, -1), Z: math.Copysign(0, -1),
	}
	canonical, err := c.Queries().QueryRegion(regionActor, query, nil, 1, first.Continuation)
	if err != nil || !reflect.DeepEqual(canonical, normal) {
		t.Fatalf("canonical zero changed token binding: got=%#v err=%v want=%#v", canonical, err, normal)
	}
}

func TestQueryAuthorizationInvalidInputsDoNotCallAuthorizer(t *testing.T) {
	policy := &queryAuthPolicy{setup: true}
	c, fixture := queryAuthorizationFixture(t, policy)
	query := queryAuthorizationRequest(fixture.scope, fixture.basis)
	policy.configure(fixture.scope, 0, 0, false)
	first, err := c.Queries().QueryRegion(regionActor, query, nil, 1, nil)
	if err != nil || first.Continuation == nil {
		t.Fatalf("continuation fixture: %#v err=%v", first, err)
	}
	zeroCut := corecontract.CoreReadCut{}
	zeroContinuation := corecontract.QueryContinuation{}
	tests := []struct {
		name         string
		change       func(*corecontract.RegionQuery)
		limit        uint16
		at           *corecontract.CoreReadCut
		continuation *corecontract.QueryContinuation
	}{
		{name: "zero limit", limit: 0},
		{name: "large limit", limit: 257},
		{name: "invalid Scope", limit: 1, change: func(q *corecontract.RegionQuery) { q.StartAnchorID = "not-a-uuid" }},
		{name: "invalid Basis", limit: 1, change: func(q *corecontract.RegionQuery) { q.BasisAnchorID = "" }},
		{name: "zero selector", limit: 1, change: func(q *corecontract.RegionQuery) { q.Selector = 0 }},
		{name: "unknown selector", limit: 1, change: func(q *corecontract.RegionQuery) { q.Selector = 255 }},
		{name: "negative radius", limit: 1, change: func(q *corecontract.RegionQuery) { q.Sphere.Radius = -1 }},
		{name: "NaN radius", limit: 1, change: func(q *corecontract.RegionQuery) { q.Sphere.Radius = math.NaN() }},
		{name: "infinite radius", limit: 1, change: func(q *corecontract.RegionQuery) { q.Sphere.Radius = math.Inf(1) }},
		{name: "NaN center", limit: 1, change: func(q *corecontract.RegionQuery) { q.Sphere.Center.X = math.NaN() }},
		{name: "positive infinite center", limit: 1, change: func(q *corecontract.RegionQuery) { q.Sphere.Center.Y = math.Inf(1) }},
		{name: "negative infinite center", limit: 1, change: func(q *corecontract.RegionQuery) { q.Sphere.Center.Z = math.Inf(-1) }},
		{name: "invalid cut shape", limit: 1, at: &zeroCut},
		{name: "invalid token shape", limit: 1, continuation: &zeroContinuation},
		{name: "at with continuation", limit: 1, at: &first.Cut, continuation: first.Continuation},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			candidate := query
			if test.change != nil {
				test.change(&candidate)
			}
			policy.configure(fixture.scope, 1, 0, false)
			page, err := c.Queries().QueryRegion(regionActor, candidate, test.at, test.limit, test.continuation)
			assertZeroQueryAuthorizationPage(t, page, err, corecontract.ErrInvalid)
			if calls := policy.recordedCalls(); len(calls) != 0 {
				t.Fatalf("invalid input reached Authorizer: %#v", calls)
			}
		})
	}
}

func TestQueryAuthorizationClosedCoreIsGatedBeforeDisclosure(t *testing.T) {
	policy := &queryAuthPolicy{setup: true}
	c, err := core.CreateDurable(queryPolicyContact{policy}, queryContact, filepath.Join(t.TempDir(), "query-closed.db"), corecontract.Config{})
	if err != nil {
		t.Fatal(err)
	}
	policy.authority = c.AuthorityRef()
	root := regionCreateRoot(t, c, regionID(5100), "query-closed-root")
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	query := queryAuthorizationRequest(root.Anchor.ID, root.Anchor.ID)
	for _, denyAt := range []int{1, 2, 0} {
		policy.configure(root.Anchor.ID, denyAt, 0, false)
		page, err := c.Queries().QueryRegion(regionActor, query, nil, 1, nil)
		want, calls := corecontract.ErrDenied, denyAt
		if denyAt == 0 {
			want, calls = corecontract.ErrStore, 2
		}
		assertZeroQueryAuthorizationPage(t, page, err, want)
		assertQueryAuthorizationCalls(t, c.AuthorityRef(), policy.recordedCalls(), regionActor, root.Anchor.ID, calls)
	}
	policy.configure(root.Anchor.ID, 1, 0, false)
	query.Sphere.Radius = -1
	page, err := c.Queries().QueryRegion(regionActor, query, nil, 1, nil)
	assertZeroQueryAuthorizationPage(t, page, err, corecontract.ErrInvalid)
	if calls := policy.recordedCalls(); len(calls) != 0 {
		t.Fatalf("invalid closed-Core input reached Authorizer: %#v", calls)
	}
}

func TestQueryAuthorizationConcurrentCloseDiscardsResult(t *testing.T) {
	for _, revoke := range []bool{false, true} {
		name := "still authorized"
		if revoke {
			name = "revoked before disclosure"
		}
		t.Run(name, func(t *testing.T) {
			policy := &queryAuthPolicy{setup: true}
			c, err := core.CreateDurable(queryPolicyContact{policy}, queryContact, filepath.Join(t.TempDir(), "query-closing.db"), corecontract.Config{})
			if err != nil {
				t.Fatal(err)
			}
			defer c.Close()
			policy.authority = c.AuthorityRef()
			root := regionCreateRoot(t, c, regionID(5110), "query-closing-root")
			query := queryAuthorizationRequest(root.Anchor.ID, root.Anchor.ID)
			policy.configure(root.Anchor.ID, 0, 2, false)
			var releaseOnce sync.Once
			release := func() { releaseOnce.Do(func() { close(policy.release) }) }
			defer release()
			queried := make(chan queryAuthorizationOutcome, 1)
			go func() {
				page, err := c.Queries().QueryRegion(regionActor, query, nil, 1, nil)
				queried <- queryAuthorizationOutcome{page, err}
			}()
			select {
			case <-policy.entered:
			case <-time.After(5 * time.Second):
				t.Fatal("query did not reach its second authorization check")
			}
			closed := make(chan error, 1)
			go func() { closed <- c.Close() }()
			select {
			case err := <-closed:
				if err != nil {
					t.Fatalf("concurrent Close: %v", err)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("Authorizer retained the Core lock and blocked Close")
			}
			want := corecontract.ErrReadCutExpired
			if revoke {
				policy.revoke()
				want = corecontract.ErrDenied
			}
			release()
			select {
			case outcome := <-queried:
				assertZeroQueryAuthorizationPage(t, outcome.page, outcome.err, want)
			case <-time.After(5 * time.Second):
				t.Fatal("query did not finish after Close and authorization release")
			}
			assertQueryAuthorizationCalls(t, c.AuthorityRef(), policy.recordedCalls(), regionActor, root.Anchor.ID, 2)
		})
	}
}

type queryAuthorizationOutcome struct {
	page corecontract.QueryPage
	err  error
}

type queryAuthorizationIDs struct {
	root, scope, basis, entity, outside values.ElementID
}

func queryAuthorizationFixture(t *testing.T, policy *queryAuthPolicy) (*core.Core, queryAuthorizationIDs) {
	t.Helper()
	c, err := core.New(queryPolicyContact{policy}, queryContact, corecontract.Config{})
	if err != nil {
		t.Fatal(err)
	}
	fixture := queryAuthorizationIDs{root: regionID(5000), scope: regionID(5001), basis: regionID(5002), entity: regionID(5003), outside: regionID(5004)}
	policy.authority = c.AuthorityRef()
	regionCreateRoot(t, c, fixture.root, "query-auth-root")
	regionCreateAnchor(t, c, fixture.scope, fixture.root, regionTuple(t, fixture.root, regionPose(t, 0, 0, 0), 20), "query-auth-scope")
	regionCreateAnchor(t, c, fixture.basis, fixture.scope, regionTuple(t, fixture.scope, regionPose(t, 0, 0, 0), 10), "query-auth-basis")
	regionCreateEntity(t, c, fixture.entity, regionTuple(t, fixture.basis, regionPose(t, 1, 0, 0), 1), "query-auth-entity")
	regionCreateAnchor(t, c, fixture.outside, fixture.root, regionTuple(t, fixture.root, regionPose(t, 100, 0, 0), 20), "query-auth-outside")
	return c, fixture
}

func queryAuthorizationRequest(scope, basis values.ElementID) corecontract.RegionQuery {
	return corecontract.RegionQuery{
		StartAnchorID: scope, BasisAnchorID: basis,
		Sphere: corecontract.QuerySphere{Radius: 20}, Selector: corecontract.QueryBoth,
	}
}

func assertZeroQueryAuthorizationPage(t *testing.T, page corecontract.QueryPage, err, want error) {
	t.Helper()
	if !errors.Is(err, want) || !reflect.DeepEqual(page, corecontract.QueryPage{}) {
		t.Fatalf("query error must not disclose results: page=%#v err=%v want=%v", page, err, want)
	}
}

func assertQueryAuthorizationCalls(t *testing.T, authority values.FactAuthorityRef, calls []queryAuthCall, principal values.PrincipalRef, scope values.ElementID, count int) {
	t.Helper()
	if len(calls) != count {
		t.Fatalf("authorization calls: got=%d want=%d (%#v)", len(calls), count, calls)
	}
	want := []values.Requirement{{Permission: corecontract.PermissionQueryRegion, Scope: values.CoreElementScope(authority, scope)}}
	for index, call := range calls {
		if call.regionActor != principal || !reflect.DeepEqual(call.requirements, want) {
			t.Fatalf("authorization call %d: regionActor=%#v requirements=%#v want regionActor=%#v requirements=%#v", index, call.regionActor, call.requirements, principal, want)
		}
	}
}
