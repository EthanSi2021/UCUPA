package query

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	"ucupa/ucupa-core/internal/commit"
	"ucupa/ucupa-core/internal/element"
	"ucupa/ucupa-core/internal/token"
)

var queryActor = values.PrincipalRef{Realm: "query-owner-test", Principal: "reader"}

type queryAuthorize func(values.PrincipalRef, ...values.Requirement) bool

func queryAuthorizationViewDigest(subject values.PrincipalRef, requirements []values.Requirement) values.AuthorizationView {
	return values.AuthorizationView(sha256.Sum256([]byte(fmt.Sprintf("view:%v:%v", subject, requirements))))
}

func queryAuthorizationDenialDigest(subject values.PrincipalRef, requirement values.Requirement) values.AuthorizationDenialView {
	return values.AuthorizationDenialView(sha256.Sum256([]byte(fmt.Sprintf("denial:%v:%v", subject, requirement))))
}

func (f queryAuthorize) AuthorizePrincipal(actor values.PrincipalRef, requirements ...values.Requirement) values.AuthorizationDecision {
	if f(actor, requirements...) {
		return values.AuthorizationValid
	}
	return values.AuthorizationDenied
}

func (f queryAuthorize) AuthorizeView(ctx context.Context, caller values.CallerContext, requirements ...values.Requirement) (values.AuthorizationDecision, values.AuthorizationView) {
	if f.AuthorizePrincipal(caller.Subject, requirements...) != values.AuthorizationValid {
		return values.AuthorizationDenied, values.AuthorizationView{}
	}
	return values.AuthorizationValid, queryAuthorizationViewDigest(caller.Subject, requirements)
}

func (f queryAuthorize) ValidateView(ctx context.Context, caller values.CallerContext, view values.AuthorizationView, requirements ...values.Requirement) values.AuthorizationDecision {
	decision, current := f.AuthorizeView(ctx, caller, requirements...)
	if decision != values.AuthorizationValid {
		return decision
	}
	if current != view {
		return values.AuthorizationChanged
	}
	return values.AuthorizationValid
}

func (f queryAuthorize) CaptureDenialView(ctx context.Context, caller values.CallerContext, requirement values.Requirement) (values.AuthorizationDecision, values.AuthorizationDenialView) {
	if f.AuthorizePrincipal(caller.Subject, requirement) == values.AuthorizationDenied {
		return values.AuthorizationDenied, queryAuthorizationDenialDigest(caller.Subject, requirement)
	}
	return values.AuthorizationValid, values.AuthorizationDenialView{}
}

func (f queryAuthorize) ValidateDenialView(ctx context.Context, caller values.CallerContext, view values.AuthorizationDenialView, requirement values.Requirement) values.AuthorizationDecision {
	if view == (values.AuthorizationDenialView{}) {
		return values.AuthorizationDenied
	}
	if _, current := f.CaptureDenialView(ctx, caller, requirement); current != view {
		return values.AuthorizationChanged
	}
	return values.AuthorizationValid
}

type queryFixture struct {
	queries  *Operations
	elements *element.Operations
	owner    *commit.Owner
	region   contract.RegionQuery
	ids      []values.ElementID
	path     string
}

func queryID(n uint64) values.ElementID {
	return values.ElementID(fmt.Sprintf("00000000-0000-4000-8000-%012x", n))
}

func newQueryFixture(t *testing.T, config contract.Config, durable bool) queryFixture {
	t.Helper()
	settings, err := NormalizeConfig(config)
	if err != nil {
		t.Fatal(err)
	}
	authority, err := values.NewFactAuthorityRef()
	if err != nil {
		t.Fatal(err)
	}
	owner, err := commit.New(authority, [32]byte{19}, contract.RealmInitialization{Realm: "fixture-realm", NextHop: "local:fixture"})
	if err != nil {
		t.Fatal(err)
	}
	var path string
	if durable {
		path = filepath.Join(t.TempDir(), "query.db")
		owner, err = commit.CreateDurable(path, authority, [32]byte{19}, contract.RealmInitialization{Realm: "fixture-realm", NextHop: "local:fixture"}, nil)
		if err != nil {
			t.Fatal(err)
		}
	}
	authorizer := queryAuthorize(func(values.PrincipalRef, ...values.Requirement) bool { return true })
	f := queryFixture{owner: owner, queries: New(owner, authorizer, [32]byte{23}, settings), elements: element.New(owner, authorizer), path: path}
	t.Cleanup(func() {
		owner.Lock()
		owner.BeginCloseLocked()
		owner.Unlock()
		f.queries.Close()
		_ = owner.CloseStore()
	})
	root := queryID(91000)
	if _, err := f.elements.CreateRoot(queryActor, root, "query-root"); err != nil {
		t.Fatal(err)
	}
	f.region = contract.RegionQuery{StartAnchorID: root, BasisAnchorID: root, Selector: contract.QueryEntities}
	for n := uint64(91001); n <= 91003; n++ {
		id := queryID(n)
		f.ids = append(f.ids, id)
		if _, err := f.elements.CreateEntity(queryActor, id, queryTuple(t, root), contract.OperationID("query-"+string(id))); err != nil {
			t.Fatal(err)
		}
	}
	return f
}

func queryTuple(t *testing.T, parent values.ElementID) contract.SpatialTuple {
	t.Helper()
	r, err := contract.NewRotation(1, 0, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	p, err := contract.NewPose(contract.Vec3{}, r)
	if err != nil {
		t.Fatal(err)
	}
	b, err := contract.NewBoundedSphere(0)
	if err != nil {
		t.Fatal(err)
	}
	return contract.SpatialTuple{Parent: parent, LocalPose: p, Range: b}
}

func requireQueryError(t *testing.T, page contract.QueryPage, got, want error) {
	t.Helper()
	if !errors.Is(got, want) || !reflect.DeepEqual(page, contract.QueryPage{}) {
		t.Fatalf("query disclosed on failure: %#v %v; want %v", page, got, want)
	}
}

func requireNoQueryResources(t *testing.T, q *Operations) {
	t.Helper()
	r := q.reads
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.entries) != 0 || len(r.views) != 0 || r.bytes != 0 || r.active != 0 {
		t.Fatalf("query resources leaked: entries=%d views=%d bytes=%d active=%d", len(r.entries), len(r.views), r.bytes, r.active)
	}
}

func TestQueryEntryConfigurationAndConsumerSurface(t *testing.T) {
	for _, duration := range []time.Duration{0, -1} {
		if _, err := NormalizeConfig(contract.Config{SnapshotRetention: &duration}); !errors.Is(err, contract.ErrInvalid) {
			t.Fatalf("invalid retention accepted: %v", err)
		}
	}
	duration := time.Hour
	f := newQueryFixture(t, contract.Config{SnapshotRetention: &duration}, false)
	duration = -1
	var public contract.QueryAccess = f.queries
	page, err := public.QueryRegion(queryActor, f.region, nil, 1, nil)
	if err != nil || page.Snapshot == nil || page.ReadMode != contract.QueryRetained || f.queries.reads.settings.Retention != time.Hour {
		t.Fatalf("configuration changed after assembly: %#v %v", page, err)
	}
	typ := reflect.TypeOf((*contract.QueryAccess)(nil)).Elem()
	if typ.NumMethod() != 3 || typ.Method(0).Name != "QueryRegion" || typ.Method(1).Name != "ReadElementSnapshot" || typ.Method(2).Name != "ReleaseReadSnapshot" {
		t.Fatal("public query handle exposes resource internals")
	}
	var absent contract.QueryAccess = (*Operations)(nil)
	bad, err := absent.QueryRegion(queryActor, f.region, nil, 1, nil)
	requireQueryError(t, bad, err, contract.ErrInvalid)
	if _, err := absent.ReadElementSnapshot(queryActor, f.ids[0], *page.Snapshot); !errors.Is(err, contract.ErrInvalid) {
		t.Fatal(err)
	}
	if err := absent.ReleaseReadSnapshot(queryActor, *page.Snapshot); !errors.Is(err, contract.ErrInvalid) {
		t.Fatal(err)
	}
}

func TestQueryEntryRetainedPagesAndStorePoison(t *testing.T) {
	ttl := time.Hour
	f := newQueryFixture(t, contract.Config{SnapshotRetention: &ttl}, true)
	first, err := f.queries.QueryRegion(queryActor, f.region, nil, 1, nil)
	if err != nil || first.Continuation == nil || first.Snapshot == nil {
		t.Fatalf("first: %#v %v", first, err)
	}
	old, err := f.queries.ReadElementSnapshot(queryActor, f.ids[1], *first.Snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.elements.DeleteEntity(queryActor, f.ids[1], old.Record.Entity.Revision, "delete-after-capture"); err != nil {
		t.Fatal(err)
	}
	newID := queryID(90999)
	if _, err := f.elements.CreateEntity(queryActor, newID, queryTuple(t, f.region.StartAnchorID), "new-before-cursor"); err != nil {
		t.Fatal(err)
	}
	last, err := f.queries.QueryRegion(queryActor, f.region, nil, 256, first.Continuation)
	if err != nil || last.Cut != first.Cut || last.Status != contract.QueryComplete || len(last.Matches) != 2 || last.Matches[0].ElementID != f.ids[1] || last.Matches[1].ElementID != f.ids[2] {
		t.Fatalf("page mixed current state: %#v %v", last, err)
	}
	historical, err := f.queries.ReadElementSnapshot(queryActor, f.ids[1], *first.Snapshot)
	if err != nil || !reflect.DeepEqual(historical, old) {
		t.Fatalf("deleted historical record lost: %#v %v", historical, err)
	}
	if out, err := f.queries.ReadElementSnapshot(queryActor, newID, *first.Snapshot); !errors.Is(err, contract.ErrInvalid) || out != (contract.ElementRead{}) {
		t.Fatalf("new id disclosed in old snapshot: %#v %v", out, err)
	}
	// A real unreconcilable Store failure poisons the unique owner. No test
	// State setter is needed, and the retained view must not mask that failure.
	if err := f.owner.CloseStore(); err != nil {
		t.Fatal(err)
	}
	if _, err := f.elements.CreateEntity(queryActor, queryID(92000), queryTuple(t, f.region.StartAnchorID), "closed-store"); !errors.Is(err, contract.ErrStore) {
		t.Fatalf("unreconcilable write: %v", err)
	}
	if _, err := f.queries.ReadElementSnapshot(queryActor, f.ids[1], *first.Snapshot); !errors.Is(err, contract.ErrStore) {
		t.Fatalf("snapshot masked unavailable facts: %v", err)
	}
	page, err := f.queries.QueryRegion(queryActor, f.region, nil, 1, first.Continuation)
	requireQueryError(t, page, err, contract.ErrStore)
}

func TestQueryEntryReturnValidationAndCleanup(t *testing.T) {
	for _, reason := range []string{"denied", "expired", "released", "invalid basis"} {
		t.Run(reason, func(t *testing.T) {
			ttl := time.Hour
			f := newQueryFixture(t, contract.Config{SnapshotRetention: &ttl}, false)
			calls := 0
			f.queries.authorizer = queryAuthorize(func(_ values.PrincipalRef, requirements ...values.Requirement) bool {
				if len(requirements) != 1 || requirements[0].Permission != contract.PermissionQueryRegion {
					t.Fatal("query authorization requirements changed")
				}
				calls++
				if calls != 2 {
					return true
				}
				r := f.queries.reads
				switch reason {
				case "denied":
					return false
				case "expired":
					r.mu.Lock()
					future := time.Now().Add(2 * ttl)
					r.now = func() time.Time { return future }
					r.mu.Unlock()
				case "released":
					r.mu.Lock()
					var snapshot contract.CoreReadSnapshot
					for _, entry := range r.entries {
						snapshot = entry.token
					}
					r.mu.Unlock()
					if err := f.queries.ReleaseReadSnapshot(queryActor, snapshot); err != nil {
						t.Fatal(err)
					}
				}
				return true
			})
			want := contract.ErrReadCutExpired
			if reason == "denied" {
				want = contract.ErrDenied
			}
			if reason == "invalid basis" {
				f.region.BasisAnchorID = queryID(94888)
				want = contract.ErrInvalid
			}
			page, err := f.queries.QueryRegion(queryActor, f.region, nil, 1, nil)
			requireQueryError(t, page, err, want)
			requireNoQueryResources(t, f.queries)
		})
	}
}

func TestQueryEntryIndependentDeadlinesAndActiveAccounting(t *testing.T) {
	ttl := time.Hour
	f := newQueryFixture(t, contract.Config{SnapshotRetention: &ttl}, false)
	r := f.queries.reads
	base := time.Now()
	r.now = func() time.Time { return base }
	first, err := f.queries.QueryRegion(queryActor, f.region, nil, 1, nil)
	if err != nil {
		t.Fatal(err)
	}
	r.mu.Lock()
	r.now = func() time.Time { return base.Add(ttl / 2) }
	r.mu.Unlock()
	second, err := f.queries.QueryRegion(queryActor, f.region, nil, 1, nil)
	if err != nil {
		t.Fatal(err)
	}
	borrow, err := f.queries.Borrow(queryActor, *first.Snapshot)
	if err != nil {
		t.Fatal(err)
	}
	r.mu.Lock()
	if len(r.views) != 1 || len(r.entries) != 2 || borrow.view.candidate != nil {
		t.Fatal("views not shared/sealed independently of handles")
	}
	r.now = func() time.Time { return base.Add(ttl) }
	r.expireLocked()
	if len(r.entries) != 1 || len(r.views) != 1 || r.bytes == 0 || r.active != 1 {
		t.Fatal("expired borrowed view lost accounting")
	}
	r.mu.Unlock()
	if err := f.queries.Check(borrow); !errors.Is(err, contract.ErrReadCutExpired) {
		t.Fatalf("expired borrow disclosed: %v", err)
	}
	borrow.Finish()
	borrow.Finish()
	expired, err := f.queries.QueryRegion(queryActor, f.region, nil, 1, first.Continuation)
	requireQueryError(t, expired, err, contract.ErrReadCutExpired)
	if _, err := f.queries.ReadElementSnapshot(queryActor, f.ids[0], *second.Snapshot); err != nil {
		t.Fatalf("new handle inherited old deadline: %v", err)
	}
	if err := f.queries.ReleaseReadSnapshot(queryActor, *second.Snapshot); err != nil {
		t.Fatal(err)
	}
	if err := f.queries.ReleaseReadSnapshot(queryActor, *second.Snapshot); err != nil {
		t.Fatal(err)
	}
	requireNoQueryResources(t, f.queries)
}

func TestQueryEntryRetainedPlanPreservesLockAndScope(t *testing.T) {
	ttl := time.Hour
	f := newQueryFixture(t, contract.Config{SnapshotRetention: &ttl}, false)
	page, err := f.queries.QueryRegion(queryActor, f.region, nil, 1, nil)
	if err != nil {
		t.Fatal(err)
	}
	f.owner.RLock()
	plan, err := f.queries.PlanRetained(queryActor, page.Snapshot)
	if err != nil {
		f.owner.RUnlock()
		t.Fatal(err)
	}
	cutMatches := plan.Projection().CurrentCut() == page.Cut
	// If TryLock unexpectedly succeeds, leave that acquired lock for Finish.
	// Either way release both locks before a failing assertion runs cleanup.
	unlocked := f.queries.reads.mu.TryLock()
	plan.Finish()
	plan.Finish()
	f.owner.RUnlock()
	if !cutMatches || unlocked {
		t.Fatal("replan lost the original resource-lock interval")
	}
	copy := *page.Snapshot
	wrong := token.CoreReadSnapshotFromParts(f.owner.AuthorityRef(), token.SnapshotRun(copy), token.SnapshotID(copy), values.PrincipalRef{Realm: queryActor.Realm, Principal: "other"}, token.SnapshotScope(copy), token.SnapshotCut(copy))
	f.owner.RLock()
	invalidPlan, err := f.queries.PlanRetained(queryActor, &wrong)
	if invalidPlan != nil {
		invalidPlan.Finish()
	}
	f.owner.RUnlock()
	if invalidPlan != nil || !errors.Is(err, contract.ErrInvalid) {
		t.Fatal("foreign caller acquired retained plan")
	}
	f.queries.DiscardOwnedSnapshot(copy)
	if f.queries.SnapshotPresent(&copy) || !errors.Is(f.queries.CheckRetainedFailure(&copy), contract.ErrReadCutExpired) {
		t.Fatal("discarded handle stayed available")
	}
	requireNoQueryResources(t, f.queries)
}
