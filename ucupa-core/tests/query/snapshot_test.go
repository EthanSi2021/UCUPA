package query_test

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	core "ucupa/ucupa-core/instance"
)

var testActor = values.PrincipalRef{Realm: "snapshot-tests", Principal: "reader"}

func TestCloseZeroAndNilCoreIsIdempotent(t *testing.T) {
	for _, c := range []*core.Core{nil, {}} {
		if err := c.Close(); err != nil {
			t.Fatal(err)
		}
		if err := c.Close(); err != nil {
			t.Fatal(err)
		}
	}
}

type allowAll struct{}

func (allowAll) Authorize(values.PrincipalRef, ...values.Requirement) bool { return true }

var queryContact = contract.RealmInitialization{Realm: "query-test-realm", NextHop: "local:query-realm"}

// queryPolicyContact adapts one fixed bool authorization stub to the Realm
// contact port. Address identity is not a variable in these tests, and the
// view evidence stays unavailable exactly as it was before the port existed.
type queryPolicyContact struct {
	policy interface {
		Authorize(values.PrincipalRef, ...values.Requirement) bool
	}
}

func (queryPolicyContact) Check(context.Context, values.NextHop, values.RealmID) error { return nil }

func (contact queryPolicyContact) AuthorizePrincipal(_ values.NextHop, _ values.RealmID, actor values.PrincipalRef, requirements ...values.Requirement) values.AuthorizationDecision {
	if contact.policy == nil || contact.policy.Authorize(actor, requirements...) {
		return values.AuthorizationValid
	}
	return values.AuthorizationDenied
}

func (queryPolicyContact) AuthorizeView(context.Context, values.NextHop, values.RealmID, values.CallerContext, ...values.Requirement) (values.AuthorizationDecision, values.AuthorizationView) {
	return values.AuthorizationUnavailable, values.AuthorizationView{}
}

func (queryPolicyContact) ValidateView(context.Context, values.NextHop, values.RealmID, values.CallerContext, values.AuthorizationView, ...values.Requirement) values.AuthorizationDecision {
	return values.AuthorizationUnavailable
}

func (queryPolicyContact) CaptureDenialView(context.Context, values.NextHop, values.RealmID, values.CallerContext, values.Requirement) (values.AuthorizationDecision, values.AuthorizationDenialView) {
	return values.AuthorizationUnavailable, values.AuthorizationDenialView{}
}

func (queryPolicyContact) ValidateDenialView(context.Context, values.NextHop, values.RealmID, values.CallerContext, values.AuthorizationDenialView, values.Requirement) values.AuthorizationDecision {
	return values.AuthorizationUnavailable
}

type fixture struct {
	core   *core.Core
	root   values.ElementID
	query  contract.RegionQuery
	ids    []values.ElementID
	range1 contract.BoundedSphere
}

func newFixture(t *testing.T, config contract.Config) fixture {
	t.Helper()
	c, err := core.New(queryPolicyContact{allowAll{}}, queryContact, config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Close() })
	root := id(91000)
	if _, err := c.Elements().CreateRoot(testActor, root, "snapshot-root"); err != nil {
		t.Fatal(err)
	}
	range1 := sphere(t, 0)
	ids := []values.ElementID{id(91001), id(91002), id(91003)}
	for index, element := range ids {
		if _, err := c.Elements().CreateEntity(testActor, element, tuple(t, root, 0, 0, 0, range1), contract.OperationID("snapshot-entity-"+string(rune('a'+index)))); err != nil {
			t.Fatal(err)
		}
	}
	return fixture{core: c, root: root, ids: ids, range1: range1, query: contract.RegionQuery{
		StartAnchorID: root, BasisAnchorID: root, Selector: contract.QueryEntities,
	}}
}

func TestSnapshotConfigurationIsValidatedAndCopied(t *testing.T) {
	for _, duration := range []time.Duration{0, -time.Second} {
		config := contract.Config{SnapshotRetention: &duration}
		if c, err := core.New(queryPolicyContact{allowAll{}}, queryContact, config); c != nil || !errors.Is(err, contract.ErrInvalid) {
			t.Fatalf("New invalid duration: core=%v err=%v", c, err)
		}
		path := filepath.Join(t.TempDir(), "invalid.db")
		if c, err := core.CreateDurable(queryPolicyContact{allowAll{}}, queryContact, path, config); c != nil || !errors.Is(err, contract.ErrInvalid) {
			t.Fatalf("CreateDurable invalid duration: core=%v err=%v", c, err)
		}
	}
	duration := time.Hour
	f := newFixture(t, contract.Config{SnapshotRetention: &duration})
	duration = -time.Second
	page, err := f.core.Queries().QueryRegion(testActor, f.query, nil, 1, nil)
	if err != nil || page.ReadMode != contract.QueryRetained || page.Snapshot == nil {
		t.Fatalf("mutating config changed running query policy: page=%#v err=%v", page, err)
	}
}

func TestRetainedSnapshotPinsPagesAndExactRecords(t *testing.T) {
	ttl := time.Hour
	f := newFixture(t, contract.Config{SnapshotRetention: &ttl})
	first, err := f.core.Queries().QueryRegion(testActor, f.query, nil, 1, nil)
	if err != nil || first.Snapshot == nil || first.Continuation == nil {
		t.Fatalf("first page: %#v err=%v", first, err)
	}
	other, err := f.core.Queries().QueryRegion(testActor, f.query, nil, 1, nil)
	if err != nil || other.Snapshot == nil {
		t.Fatalf("independent handle: %#v err=%v", other, err)
	}
	old, err := f.core.Elements().ReadElement(testActor, f.ids[1], nil)
	if err != nil || old.Record == nil || old.Record.Entity == nil {
		t.Fatalf("read before delete: %#v err=%v", old, err)
	}
	if _, err := f.core.Elements().DeleteEntity(testActor, f.ids[1], old.Record.Entity.Revision, "snapshot-delete"); err != nil {
		t.Fatal(err)
	}
	newID := id(90999)
	if _, err := f.core.Elements().CreateEntity(testActor, newID, tuple(t, f.root, 0, 0, 0, f.range1), "snapshot-new"); err != nil {
		t.Fatal(err)
	}
	last, err := f.core.Queries().QueryRegion(testActor, f.query, nil, 256, first.Continuation)
	if err != nil || last.Status != contract.QueryComplete || last.Cut != first.Cut || last.Snapshot == nil ||
		!reflect.DeepEqual(matchIDs(last), f.ids[1:]) {
		t.Fatalf("retained continuation changed: page=%#v ids=%v err=%v", last, matchIDs(last), err)
	}
	historical, err := f.core.Queries().ReadElementSnapshot(testActor, f.ids[1], *last.Snapshot)
	if err != nil || !reflect.DeepEqual(historical, old) {
		t.Fatalf("deleted record not pinned exactly: %#v err=%v", historical, err)
	}
	current, err := f.core.Elements().ReadElement(testActor, f.ids[1], nil)
	if err != nil || current.State != contract.ExistenceNotFound || current.Record != nil {
		t.Fatalf("current read did not see deletion: %#v err=%v", current, err)
	}
	if _, err := f.core.Elements().ReadElement(testActor, f.ids[1], &first.Cut); !errors.Is(err, contract.ErrReadCutExpired) {
		t.Fatalf("bare cut became historical read: %v", err)
	}
	for _, unknown := range []values.ElementID{newID, id(91999)} {
		result, err := f.core.Queries().ReadElementSnapshot(testActor, unknown, *last.Snapshot)
		if !errors.Is(err, contract.ErrInvalid) || result != (contract.ElementRead{}) {
			t.Fatalf("unknown old ID disclosed data: result=%#v err=%v", result, err)
		}
	}
	if err := f.core.Queries().ReleaseReadSnapshot(testActor, *first.Snapshot); err != nil {
		t.Fatal(err)
	}
	if err := f.core.Queries().ReleaseReadSnapshot(testActor, *first.Snapshot); err != nil {
		t.Fatalf("release was not idempotent: %v", err)
	}
	if page, err := f.core.Queries().QueryRegion(testActor, f.query, nil, 1, first.Continuation); !errors.Is(err, contract.ErrReadCutExpired) || !reflect.DeepEqual(page, contract.QueryPage{}) {
		t.Fatalf("released handle remained usable: %#v err=%v", page, err)
	}
	if _, err := f.core.Queries().ReadElementSnapshot(testActor, f.ids[0], *other.Snapshot); err != nil {
		t.Fatalf("release invalidated independent handle: %v", err)
	}
}

func TestSnapshotHandleBindingsAndCloseSemantics(t *testing.T) {
	ttl := time.Hour
	f := newFixture(t, contract.Config{SnapshotRetention: &ttl})
	page, err := f.core.Queries().QueryRegion(testActor, f.query, nil, 1, nil)
	if err != nil || page.Snapshot == nil || page.Continuation == nil {
		t.Fatalf("query: %#v err=%v", page, err)
	}
	if err := f.core.Queries().ReleaseReadSnapshot(testActor, contract.CoreReadSnapshot{}); !errors.Is(err, contract.ErrInvalid) {
		t.Fatalf("zero handle: %v", err)
	}
	foreign, err := core.New(queryPolicyContact{allowAll{}}, queryContact, contract.Config{SnapshotRetention: &ttl})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = foreign.Close() })
	if result, err := foreign.Queries().ReadElementSnapshot(testActor, f.ids[0], *page.Snapshot); !errors.Is(err, contract.ErrInvalid) || result != (contract.ElementRead{}) {
		t.Fatalf("foreign owner accepted handle: %#v err=%v", result, err)
	}
	if result, err := foreign.Queries().QueryRegion(testActor, f.query, nil, 1, page.Continuation); !errors.Is(err, contract.ErrInvalid) || !reflect.DeepEqual(result, contract.QueryPage{}) {
		t.Fatalf("foreign owner accepted continuation: %#v err=%v", result, err)
	}
	otherActor := values.PrincipalRef{Realm: testActor.Realm, Principal: "other"}
	if result, err := f.core.Queries().ReadElementSnapshot(otherActor, f.ids[0], *page.Snapshot); !errors.Is(err, contract.ErrInvalid) || result != (contract.ElementRead{}) {
		t.Fatalf("foreign actor accepted handle: %#v err=%v", result, err)
	}
	if err := f.core.Close(); err != nil {
		t.Fatal(err)
	}
	if result, err := f.core.Queries().QueryRegion(testActor, f.query, nil, 256, page.Continuation); !errors.Is(err, contract.ErrReadCutExpired) || !reflect.DeepEqual(result, contract.QueryPage{}) {
		t.Fatalf("closed cursor: %#v err=%v", result, err)
	}
	if _, err := f.core.Elements().ReadElement(testActor, f.ids[0], nil); !errors.Is(err, contract.ErrStore) {
		t.Fatalf("ordinary read after close: %v", err)
	}
}

func TestCurrentVerifiedCloseExpiresCursor(t *testing.T) {
	f := newFixture(t, contract.Config{})
	page, err := f.core.Queries().QueryRegion(testActor, f.query, nil, 1, nil)
	if err != nil || page.Snapshot != nil || page.ReadMode != contract.QueryCurrentVerified || page.Continuation == nil {
		t.Fatalf("current mode: %#v err=%v", page, err)
	}
	if _, err := f.core.Queries().QueryRegion(testActor, f.query, nil, 256, page.Continuation); err != nil {
		t.Fatal(err)
	}
	if err := f.core.Close(); err != nil {
		t.Fatal(err)
	}
	result, err := f.core.Queries().QueryRegion(testActor, f.query, nil, 256, page.Continuation)
	if !errors.Is(err, contract.ErrReadCutExpired) || !reflect.DeepEqual(result, contract.QueryPage{}) {
		t.Fatalf("closed current cursor: %#v err=%v", result, err)
	}
	if _, err := f.core.Elements().ReadElement(testActor, f.ids[0], nil); !errors.Is(err, contract.ErrStore) {
		t.Fatalf("ordinary read after close: %v", err)
	}
}

func TestSnapshotKeepsOldScopeAfterAncestorReparentAndRangeChange(t *testing.T) {
	ttl := time.Hour
	f := newFixture(t, contract.Config{SnapshotRetention: &ttl})
	target := createAnchor(t, f.core, id(95000), f.root, 100, "snapshot-target")
	scope := createAnchor(t, f.core, id(95001), f.root, 10, "snapshot-scope")
	leaves := []values.ElementID{id(95002), id(95003)}
	for index, leaf := range leaves {
		if _, err := f.core.Elements().CreateEntity(testActor, leaf, tuple(t, scope, 0, 0, 0, sphere(t, 0)), contract.OperationID("snapshot-leaf-"+string(rune('a'+index)))); err != nil {
			t.Fatal(err)
		}
	}
	query := contract.RegionQuery{StartAnchorID: scope, BasisAnchorID: scope, Selector: contract.QueryEntities}
	first, err := f.core.Queries().QueryRegion(testActor, query, nil, 1, nil)
	if err != nil || first.Snapshot == nil || first.Continuation == nil {
		t.Fatalf("first: %#v err=%v", first, err)
	}
	old, err := f.core.Elements().ReadElement(testActor, scope, nil)
	if err != nil || old.Record == nil || old.Record.Anchor == nil {
		t.Fatalf("old scope read: %#v err=%v", old, err)
	}
	if _, err := f.core.Elements().UpdateAnchor(testActor, scope, tuple(t, target, 0, 0, 0, sphere(t, 5)), old.Record.Anchor.Revision, "snapshot-reparent"); err != nil {
		t.Fatal(err)
	}
	second, err := f.core.Queries().QueryRegion(testActor, query, nil, 256, first.Continuation)
	if err != nil || second.ScopeAnchorID != scope || second.Cut != first.Cut || !reflect.DeepEqual(matchIDs(second), leaves[1:]) {
		t.Fatalf("old scope changed: page=%#v ids=%v err=%v", second, matchIDs(second), err)
	}
	historical, err := f.core.Queries().ReadElementSnapshot(testActor, scope, *second.Snapshot)
	if err != nil || !reflect.DeepEqual(historical, old) {
		t.Fatalf("old parent or Range changed: %#v err=%v", historical, err)
	}
	current, err := f.core.Elements().ReadElement(testActor, scope, nil)
	if err != nil || current.Record == nil || current.Record.Anchor == nil || current.Record.Anchor.ParentAnchorID == nil || *current.Record.Anchor.ParentAnchorID != target {
		t.Fatalf("current parent did not change: %#v err=%v", current, err)
	}
	if current.Record.Anchor.Range.Kind() != contract.RangeBoundedSphere {
		t.Fatalf("current Range kind: %#v", current.Record.Anchor.Range)
	}
	currentSphere, ok := current.Record.Anchor.Range.BoundedSphere()
	if !ok || currentSphere.Radius() != 5 {
		t.Fatalf("current Range did not change: %#v", current.Record.Anchor.Range)
	}
}

func createAnchor(t *testing.T, c *core.Core, element, parent values.ElementID, radius float64, operation contract.OperationID) values.ElementID {
	t.Helper()
	if _, err := c.Elements().CreateAnchor(testActor, element, tuple(t, parent, 0, 0, 0, sphere(t, radius)), operation); err != nil {
		t.Fatal(err)
	}
	return element
}

func tuple(t *testing.T, parent values.ElementID, x, y, z float64, extent contract.BoundedSphere) contract.SpatialTuple {
	t.Helper()
	rotation, err := contract.NewRotation(1, 0, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	pose, err := contract.NewPose(contract.Vec3{X: x, Y: y, Z: z}, rotation)
	if err != nil {
		t.Fatal(err)
	}
	return contract.SpatialTuple{Parent: parent, LocalPose: pose, Range: extent}
}

func sphere(t *testing.T, radius float64) contract.BoundedSphere {
	t.Helper()
	result, err := contract.NewBoundedSphere(radius)
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func matchIDs(page contract.QueryPage) []values.ElementID {
	result := make([]values.ElementID, len(page.Matches))
	for index := range page.Matches {
		result[index] = page.Matches[index].ElementID
	}
	return result
}

func id(number int) values.ElementID {
	return values.ElementID(fmt.Sprintf("10000000-0000-4000-8000-%012x", number))
}
