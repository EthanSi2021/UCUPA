package request

import (
	"math"
	"testing"

	"ucupa/shared/values"
	"ucupa/ucupa-core/internal/geometry"
	"ucupa/ucupa-core/internal/token"
)

const origin values.ElementID = "00000000-0000-4000-8000-000000000001"
const child values.ElementID = "00000000-0000-4000-8000-000000000002"
const owner values.FactAuthorityRef = "00000000-0000-4000-8000-000000000003"

var actor = values.PrincipalRef{Realm: "realm", Principal: "query-reader"}

func TestCanonicalQueryAndPathCopies(t *testing.T) {
	q := RegionQuery{StartAnchorID: origin, BasisAnchorID: origin, UpDepth: -1, Selector: QueryBoth, Sphere: QuerySphere{Center: geometry.Vec3{X: math.Copysign(0, -1)}, Radius: math.Copysign(0, -1)}}
	got, ok := CanonicalRegionQuery(q)
	if !ok || math.Signbit(got.Sphere.Center.X) || math.Signbit(got.Sphere.Radius) {
		t.Fatal("negative zero not canonicalized")
	}
	for _, mutate := range []func(*RegionQuery){
		func(q *RegionQuery) { q.StartAnchorID = "bad" }, func(q *RegionQuery) { q.BasisAnchorID = "bad" }, func(q *RegionQuery) { q.UpDepth = -2 },
		func(q *RegionQuery) { q.Sphere.Radius = -1 }, func(q *RegionQuery) { q.Sphere.Radius = math.Inf(1) }, func(q *RegionQuery) { q.Sphere.Center.Y = math.NaN() },
		func(q *RegionQuery) { q.Selector = 0 },
	} {
		bad := got
		mutate(&bad)
		if _, ok := CanonicalRegionQuery(bad); ok {
			t.Fatal("invalid query accepted")
		}
	}
	path := Path{OriginAnchorID: origin, Up: math.MaxUint64, Down: []values.ElementID{child}}
	if !ValidPath(path) || !EqualPath(Path{OriginAnchorID: origin}, Path{OriginAnchorID: origin, Down: []values.ElementID{}}) {
		t.Fatal("path limits or nil/empty equality changed")
	}
	clone := ClonePath(path)
	clone.Down[0] = origin
	if path.Down[0] != child || EqualPath(path, clone) {
		t.Fatal("path clone shares slice")
	}
	bad := ClonePath(path)
	bad.Down[0] = "bad"
	if ValidPath(bad) {
		t.Fatal("invalid path accepted")
	}
	pq := PathRegionQuery{StartPath: path, BasisPath: path, UpDepth: -1, Selector: QueryBoth}
	copy := CloneQuery(pq)
	copy.StartPath.Down[0] = origin
	if pq.StartPath.Down[0] != child || copy.BasisPath.Down[0] != child || EqualQuery(pq, copy) {
		t.Fatal("query path copies alias")
	}
}

func TestContinuationBindingAndSnapshotIsolation(t *testing.T) {
	q := RegionQuery{StartAnchorID: origin, BasisAnchorID: origin, Selector: QueryBoth}
	scope, run := [32]byte{1}, [32]byte{2}
	cut := token.FromParts(scope, 1)
	partial := QueryContinuationFromParts("", scope, [32]byte{}, q, values.PrincipalRef{}, cut, child, 0, "", 0, nil)
	if ValidQueryContinuation(*partial) || QueryLast(*partial) != child {
		t.Fatal("partial scan became a valid continuation")
	}
	pageSnapshot := token.CoreReadSnapshotFromParts(owner, run, 1, actor, origin, cut)
	retained := BindQueryContinuation(*partial, owner, q, actor, run, QueryRetained, origin, 0, &pageSnapshot)
	if !ValidQueryContinuation(retained) || QuerySnapshot(retained) == &pageSnapshot {
		t.Fatal("retained snapshot binding or independent copy lost")
	}
	pageSnapshot = token.CoreReadSnapshot{}
	if !ValidQueryContinuation(retained) {
		t.Fatal("mutating returned page snapshot changed continuation")
	}
	current := BindQueryContinuation(*partial, owner, q, actor, run, QueryCurrentVerified, origin, 0, nil)
	if !ValidQueryContinuation(current) || QuerySnapshot(current) != nil {
		t.Fatal("current binding changed")
	}
	for _, mutate := range []func(*QueryContinuation){
		func(c *QueryContinuation) { c.owner = "" }, func(c *QueryContinuation) { c.scope = [32]byte{} }, func(c *QueryContinuation) { c.run = [32]byte{} },
		func(c *QueryContinuation) { c.actor = values.PrincipalRef{} }, func(c *QueryContinuation) { c.last = "" }, func(c *QueryContinuation) { c.root = "" },
		func(c *QueryContinuation) { c.cut = token.FromParts([32]byte{3}, 1) }, func(c *QueryContinuation) { c.mode = 0 },
		func(c *QueryContinuation) { c.snapshot = nil }, func(c *QueryContinuation) { c.actor.Principal = "other" },
		func(c *QueryContinuation) { c.mode = QueryCurrentVerified },
	} {
		bad := retained
		mutate(&bad)
		if ValidQueryContinuation(bad) {
			t.Fatal("mismatched continuation accepted")
		}
	}
}
