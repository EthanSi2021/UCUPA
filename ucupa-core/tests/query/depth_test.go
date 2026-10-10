package query_test

import (
	"errors"
	"fmt"
	"math"
	"reflect"
	"sync"
	"testing"
	"time"

	"ucupa/shared/values"
	corecontract "ucupa/ucupa-core/contract"
	core "ucupa/ucupa-core/instance"
)

type snapshotAuthorizeFunc func(values.PrincipalRef, ...values.Requirement) bool

func (f snapshotAuthorizeFunc) Authorize(actor values.PrincipalRef, requirements ...values.Requirement) bool {
	return f(actor, requirements...)
}

func TestQueryDepthSelectsCompleteAncestorClosure(t *testing.T) {
	c, query, rootEntities := depthFixture(t, corecontract.Config{})
	root := query.StartAnchorID
	parent := depthCreateAnchor(t, c, depthID(92000), depthTuple(t, root, 0, 0, 0, 100), "depth-parent")
	start := depthCreateAnchor(t, c, depthID(92001), depthTuple(t, parent, 0, 0, 0, 50), "depth-start")
	peer := depthCreateAnchor(t, c, depthID(92002), depthTuple(t, parent, 0, 0, 0, 10), "depth-peer")
	leaf := depthCreateEntity(t, c, depthID(92003), depthTuple(t, start, 0, 0, 0, 0), "depth-leaf")
	sibling := depthCreateEntity(t, c, depthID(92004), depthTuple(t, peer, 0, 0, 0, 0), "depth-sibling")
	query.StartAnchorID, query.BasisAnchorID = start, start
	for _, test := range []struct {
		depth  int64
		scope  values.ElementID
		actual uint64
		want   []values.ElementID
	}{
		{0, start, 0, []values.ElementID{leaf}},
		{1, parent, 1, []values.ElementID{leaf, sibling}},
		{2, root, 2, append(append([]values.ElementID{}, rootEntities...), leaf, sibling)},
		{-1, root, 2, append(append([]values.ElementID{}, rootEntities...), leaf, sibling)},
		{math.MaxInt64, root, 2, append(append([]values.ElementID{}, rootEntities...), leaf, sibling)},
	} {
		query.UpDepth = test.depth
		page, err := c.Queries().QueryRegion(depthActor, query, nil, 256, nil)
		if err != nil || page.ScopeAnchorID != test.scope || page.ActualUpDepth != test.actual || page.Status != corecontract.QueryComplete || !reflect.DeepEqual(depthIDs(page), test.want) {
			t.Fatalf("depth %d: %#v %v", test.depth, page, err)
		}
		for _, match := range page.Matches {
			assertDepthPath(t, c.Core, test.scope, page.Cut, match)
		}
	}
	query.UpDepth = -1
	page, err := c.Queries().QueryRegion(depthActor, query, nil, 1, nil)
	if err != nil {
		t.Fatal(err)
	}
	query.UpDepth = 2
	bad, err := c.Queries().QueryRegion(depthActor, query, nil, 1, page.Continuation)
	assertDepthError(t, bad, err, corecontract.ErrInvalid)
	for _, depth := range []int64{-2, math.MinInt64} {
		query.UpDepth = depth
		bad, err = c.Queries().QueryRegion(depthActor, query, nil, 1, nil)
		assertDepthError(t, bad, err, corecontract.ErrInvalid)
	}
	query.StartAnchorID, query.BasisAnchorID, query.UpDepth = root, root, -1
	page, err = c.Queries().QueryRegion(depthActor, query, nil, 256, nil)
	if err != nil || page.ScopeAnchorID != root || page.ActualUpDepth != 0 {
		t.Fatalf("root ascent: %#v %v", page, err)
	}
}

func TestQueryDepthPermissionTargetsScopeAndFailureDiagnosticOnly(t *testing.T) {
	c, query, _ := depthFixture(t, corecontract.Config{})
	root := query.StartAnchorID
	child := depthCreateAnchor(t, c, depthID(92500), depthTuple(t, root, 0, 0, 0, 1), "depth-auth-child")
	query.StartAnchorID, query.BasisAnchorID, query.UpDepth = child, child, -1
	var targets []values.ElementID
	c.policy.set(snapshotAuthorizeFunc(func(actor values.PrincipalRef, requirements ...values.Requirement) bool {
		for _, r := range requirements {
			owner, id, ok := r.Scope.CoreElementTarget()
			if !ok || owner != c.AuthorityRef() {
				return false
			}
			targets = append(targets, id)
			if r.Permission != corecontract.PermissionQueryRegion || id != root {
				return false
			}
		}
		return true
	}))
	if _, err := c.Queries().QueryRegion(depthActor, query, nil, 256, nil); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(targets, []values.ElementID{root, root}) {
		t.Fatalf("extra Start permission: %v", targets)
	}
	query.UpDepth = 0
	if _, err := c.Queries().QueryRegion(depthActor, query, nil, 256, nil); !errors.Is(err, corecontract.ErrDenied) {
		t.Fatalf("Scope inherited parent grant: %v", err)
	}
	query.StartAnchorID = depthID(92599)
	if _, err := c.Queries().QueryRegion(depthActor, query, nil, 256, nil); !errors.Is(err, corecontract.ErrDenied) {
		t.Fatalf("missing Start leaked: %v", err)
	}
}

func TestQueryReadModesFinalAuthorizationMutation(t *testing.T) {
	for _, retained := range []bool{false, true} {
		t.Run(map[bool]string{false: "current", true: "retained"}[retained], func(t *testing.T) {
			config := corecontract.Config{}
			ttl := time.Hour
			if retained {
				config.SnapshotRetention = &ttl
			}
			c, query, ids := depthFixture(t, config)
			calls := 0
			c.policy.set(snapshotAuthorizeFunc(func(actor values.PrincipalRef, requirements ...values.Requirement) bool {
				if len(requirements) == 1 && requirements[0].Permission == corecontract.PermissionQueryRegion {
					calls++
					if calls == 2 {
						depthCreateEntity(t, c, depthID(92999), depthTuple(t, query.StartAnchorID, 0, 0, 0, 0), "query-during-auth")
					}
				}
				return true
			}))
			page, err := c.Queries().QueryRegion(depthActor, query, nil, 256, nil)
			if !retained {
				assertDepthError(t, page, err, corecontract.ErrReadCutExpired)
			} else if err != nil || !reflect.DeepEqual(depthIDs(page), ids) {
				t.Fatalf("retained final write changed page: %#v %v", page, err)
			}
		})
	}
}

func TestQueryMissingStartDiagnosticRechecksCut(t *testing.T) {
	c, query, _ := depthFixture(t, corecontract.Config{})
	root := query.StartAnchorID
	query.StartAnchorID = depthID(93000)
	calls := 0
	c.policy.set(snapshotAuthorizeFunc(func(actor values.PrincipalRef, requirements ...values.Requirement) bool {
		if len(requirements) == 1 && requirements[0].Permission == corecontract.PermissionQueryRegion {
			calls++
			if calls == 2 {
				depthCreateAnchor(t, c, query.StartAnchorID, depthTuple(t, root, 0, 0, 0, 1), "diagnostic-created")
			}
		}
		return true
	}))
	page, err := c.Queries().QueryRegion(depthActor, query, nil, 1, nil)
	assertDepthError(t, page, err, corecontract.ErrReadCutExpired)
}

func TestSnapshotExactReadReauthorizesAndDoesNotExpandScope(t *testing.T) {
	ttl := time.Hour
	c, query, ids := depthFixture(t, corecontract.Config{SnapshotRetention: &ttl})
	root := query.StartAnchorID
	child := depthCreateAnchor(t, c, depthID(93100), depthTuple(t, root, 0, 0, 0, 10), "snapshot-scope-child")
	leaf := depthCreateEntity(t, c, depthID(93101), depthTuple(t, child, 0, 0, 0, 0), "snapshot-scope-leaf")
	query.StartAnchorID, query.BasisAnchorID = child, child
	page, err := c.Queries().QueryRegion(depthActor, query, nil, 256, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []values.ElementID{root, ids[0], depthID(93199)} {
		if read, err := c.Queries().ReadElementSnapshot(depthActor, id, *page.Snapshot); !errors.Is(err, corecontract.ErrInvalid) || read != (corecontract.ElementRead{}) {
			t.Fatalf("Scope expansion: %#v %v", read, err)
		}
	}
	calls := 0
	c.policy.set(snapshotAuthorizeFunc(func(actor values.PrincipalRef, requirements ...values.Requirement) bool { calls++; return calls == 1 }))
	if read, err := c.Queries().ReadElementSnapshot(depthActor, leaf, *page.Snapshot); !errors.Is(err, corecontract.ErrDenied) || read != (corecontract.ElementRead{}) {
		t.Fatalf("read skipped final authorization: %#v %v", read, err)
	}
	if err := c.Queries().ReleaseReadSnapshot(depthActor, *page.Snapshot); err != nil {
		t.Fatalf("release needs revoked query grant: %v", err)
	}
}

type depthCore struct {
	*core.Core
	policy *depthPolicy
}
type depthPolicy struct {
	mu sync.RWMutex
	fn snapshotAuthorizeFunc
}

func (p *depthPolicy) Authorize(a values.PrincipalRef, r ...values.Requirement) bool {
	p.mu.RLock()
	fn := p.fn
	p.mu.RUnlock()
	if fn == nil {
		return true
	}
	return fn(a, r...)
}
func (p *depthPolicy) set(fn snapshotAuthorizeFunc) { p.mu.Lock(); p.fn = fn; p.mu.Unlock() }

var depthActor = values.PrincipalRef{Realm: "oracle-realm", Principal: "oracle"}

func depthFixture(t *testing.T, config corecontract.Config) (*depthCore, corecontract.RegionQuery, []values.ElementID) {
	t.Helper()
	p := &depthPolicy{}
	c, err := core.New(queryPolicyContact{p}, queryContact, config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Close() })
	d := &depthCore{Core: c, policy: p}
	root := depthCreateRoot(t, d, depthID(91000), "snapshot-root")
	ids := []values.ElementID{depthID(91001), depthID(91002), depthID(91003)}
	for _, id := range ids {
		depthCreateEntity(t, d, id, depthTuple(t, root, 0, 0, 0, 0), corecontract.OperationID("snapshot-"+string(id)))
	}
	return d, corecontract.RegionQuery{StartAnchorID: root, BasisAnchorID: root, Selector: corecontract.QueryEntities}, ids
}
func depthID(n uint64) values.ElementID {
	return values.ElementID(fmt.Sprintf("10000000-0000-4000-8000-%012x", n))
}
func depthTuple(t *testing.T, parent values.ElementID, x, y, z, radius float64) corecontract.SpatialTuple {
	t.Helper()
	rotation, err := corecontract.NewRotation(1, 0, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	p, err := corecontract.NewPose(corecontract.Vec3{X: x, Y: y, Z: z}, rotation)
	if err != nil {
		t.Fatal(err)
	}
	sphere, err := corecontract.NewBoundedSphere(radius)
	if err != nil {
		t.Fatal(err)
	}
	return corecontract.SpatialTuple{Parent: parent, LocalPose: p, Range: sphere}
}
func depthCreateRoot(t *testing.T, c *depthCore, id values.ElementID, op corecontract.OperationID) values.ElementID {
	t.Helper()
	r, err := c.Elements().CreateRoot(depthActor, id, op)
	if err != nil || r.Record == nil {
		t.Fatalf("root %#v err=%v", r, err)
	}
	return r.Record.Anchor.ID
}
func depthCreateAnchor(t *testing.T, c *depthCore, id values.ElementID, tuple corecontract.SpatialTuple, op corecontract.OperationID) values.ElementID {
	t.Helper()
	r, err := c.Elements().CreateAnchor(depthActor, id, tuple, op)
	if err != nil || r.Record == nil {
		t.Fatalf("anchor %#v err=%v", r, err)
	}
	return r.Record.Anchor.ID
}
func depthCreateEntity(t *testing.T, c *depthCore, id values.ElementID, tuple corecontract.SpatialTuple, op corecontract.OperationID) values.ElementID {
	t.Helper()
	r, err := c.Elements().CreateEntity(depthActor, id, tuple, op)
	if err != nil || r.Record == nil {
		t.Fatalf("entity %#v err=%v", r, err)
	}
	return r.Record.Entity.ID
}
func depthIDs(page corecontract.QueryPage) []values.ElementID {
	var ids []values.ElementID
	for _, m := range page.Matches {
		ids = append(ids, m.ElementID)
	}
	return ids
}
func assertDepthError(t *testing.T, page corecontract.QueryPage, got, want error) {
	t.Helper()
	if !errors.Is(got, want) || !reflect.DeepEqual(page, corecontract.QueryPage{}) {
		t.Fatalf("query failure: page=%#v err=%v want=%v", page, got, want)
	}
}
func assertDepthPath(t *testing.T, c *core.Core, scope values.ElementID, cut corecontract.CoreReadCut, match corecontract.QueryMatch) {
	t.Helper()
	current := scope
	for _, id := range match.Path {
		read, err := c.Elements().ReadElement(depthActor, id, &cut)
		if err != nil || read.Record == nil || !read.Cut.Equal(cut) {
			t.Fatalf("same-cut path Element %q: %#v err=%v", id, read, err)
		}
		var parent values.ElementID
		if read.Record.Anchor != nil && read.Record.Anchor.ParentAnchorID != nil {
			parent = *read.Record.Anchor.ParentAnchorID
		} else if read.Record.Entity != nil {
			parent = read.Record.Entity.ContainingAnchorID
		} else {
			t.Fatalf("path references invalid Element %q", id)
		}
		if parent != current {
			t.Fatalf("non-edge path: current=%q next=%q parent=%q", current, id, parent)
		}
		current = id
	}
	if current != match.ElementID || (match.ElementID == scope && len(match.Path) != 0) {
		t.Fatalf("path is not relative to Scope with included terminal: %#v", match)
	}
}
