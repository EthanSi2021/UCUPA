package scan

import (
	"errors"
	"fmt"
	"reflect"
	"testing"

	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	elementfact "ucupa/ucupa-core/internal/element/fact"
	"ucupa/ucupa-core/internal/geometry"
)

func scanID(n int) values.ElementID {
	return values.ElementID(fmt.Sprintf("00000000-0000-4000-8000-%012d", n))
}
func scanAnchor(t *testing.T, s *elementfact.State, id, parent values.ElementID, x float64) {
	t.Helper()
	pose, err := contract.NewPose(contract.Vec3{X: x}, geometry.IdentityPose().Rotation)
	if err != nil {
		t.Fatal(err)
	}
	r, err := contract.NewBoundedSphere(20)
	if err != nil {
		t.Fatal(err)
	}
	rangeValue := geometry.BoundedRange(r)
	if parent == "" {
		rangeValue = geometry.UnboundedRootRange()
	}
	if err := s.RestoreAnchor(elementfact.Anchor{ID: id, Parent: parent, LocalPose: pose, Range: rangeValue}); err != nil {
		t.Fatal(err)
	}
}
func scanEntity(t *testing.T, s *elementfact.State, id, parent values.ElementID, x, radius float64) elementfact.Entity {
	t.Helper()
	pose, err := contract.NewPose(contract.Vec3{X: x}, geometry.IdentityPose().Rotation)
	if err != nil {
		t.Fatal(err)
	}
	r, err := contract.NewBoundedSphere(radius)
	if err != nil {
		t.Fatal(err)
	}
	entity := elementfact.Entity{ID: id, Parent: parent, LocalPose: pose, Range: r}
	if err := s.RestoreEntity(entity); err != nil {
		t.Fatal(err)
	}
	return entity
}

func TestClosureProjectionCandidateAndPathShareOneFactView(t *testing.T) {
	s := elementfact.New()
	root, scope, child, other, entityID := scanID(1), scanID(2), scanID(3), scanID(4), scanID(5)
	scanAnchor(t, &s, root, "", 0)
	scanAnchor(t, &s, scope, root, 5)
	scanAnchor(t, &s, child, scope, 2)
	scanAnchor(t, &s, other, root, 0)
	entity := scanEntity(t, &s, entityID, child, 3, 1)
	before := s.Clone()
	stats := Stats{}
	closure, err := BoundaryClosure(s.View, scope, &stats)
	if err != nil || len(closure) != 2 || stats.AnchorRecordsScanned != 4 || stats.ClosureAnchorsVisited != 2 {
		t.Fatalf("closure=%v stats=%+v err=%v", closure, stats, err)
	}
	if _, ok := closure[child]; !ok {
		t.Fatal("descendant omitted")
	}
	if _, ok := closure[other]; ok {
		t.Fatal("outside branch included")
	}
	memo := NewProjectionMemo(s.View, scope, &stats)
	projection, err := memo.Entity(entity)
	if err != nil || projection.Edges != 2 {
		t.Fatalf("projection=%+v err=%v", projection, err)
	}
	if _, err := memo.Anchor(child); err != nil || stats.AnchorProjectionEdges != 1 || stats.EntityProjectionEdges != 1 {
		t.Fatalf("memo stats=%+v err=%v", stats, err)
	}
	query := contract.RegionQuery{StartAnchorID: scope, BasisAnchorID: scope, Sphere: contract.QuerySphere{Center: contract.Vec3{X: 7}, Radius: 1}, Selector: contract.QueryEntities}
	queryRange, _ := contract.NewBoundedSphere(1)
	match, hit, err := memo.Candidate(entityID, query, queryRange, geometry.ExactFromPose(geometry.IdentityPose()))
	if err != nil || !hit || match.Kind != contract.ElementEntity || match.ElementID != entityID {
		t.Fatalf("tangent=%+v hit=%v err=%v", match, hit, err)
	}
	query.Sphere.Center.X = 8
	if _, hit, err := memo.Candidate(entityID, query, queryRange, geometry.ExactFromPose(geometry.IdentityPose())); err != nil || hit {
		t.Fatalf("outside hit=%v err=%v", hit, err)
	}
	path, err := Path(s.View, entityID, scope)
	if err != nil || !reflect.DeepEqual(path, []values.ElementID{child, entityID}) {
		t.Fatalf("path=%v err=%v", path, err)
	}
	empty, err := Path(s.View, scope, scope)
	if err != nil || empty == nil || len(empty) != 0 {
		t.Fatalf("empty path must retain non-nil shape: %v %v", empty, err)
	}
	if !s.Equal(before) {
		t.Fatal("pure scan modified fact tables")
	}
}

func TestMalformedProjectionAndPathKeepStoreErrors(t *testing.T) {
	s := elementfact.New()
	root, a, b := scanID(10), scanID(11), scanID(12)
	scanAnchor(t, &s, root, "", 0)
	scanAnchor(t, &s, a, b, 0)
	scanAnchor(t, &s, b, a, 0)
	// An unrelated disconnected cycle does not broaden the closure helper's
	// job into whole-owner validation. The owner performs that at its old point.
	closure, err := BoundaryClosure(s.View, root, nil)
	if err != nil || len(closure) != 1 {
		t.Fatalf("closure=%v err=%v", closure, err)
	}
	memo := NewProjectionMemo(s.View, root, nil)
	if _, err := memo.Anchor(a); !errors.Is(err, contract.ErrStore) {
		t.Fatalf("cycle projection=%v", err)
	}
	if _, err := memo.Anchor(scanID(99)); !errors.Is(err, contract.ErrStore) {
		t.Fatalf("missing projection=%v", err)
	}
	if _, err := Path(s.View, a, root); !errors.Is(err, contract.ErrStore) {
		t.Fatalf("cycle path=%v", err)
	}
	if _, err := Path(s.View, scanID(99), root); !errors.Is(err, contract.ErrStore) {
		t.Fatalf("missing path=%v", err)
	}
	r, _ := contract.NewBoundedSphere(0)
	inverse := geometry.ExactFromPose(geometry.IdentityPose())
	if _, hit, err := memo.Candidate(root, contract.RegionQuery{}, r, inverse); err != nil || !hit {
		t.Fatalf("unbounded root hit=%v err=%v", hit, err)
	}
	if _, _, err := memo.Candidate(scanID(99), contract.RegionQuery{}, r, inverse); !errors.Is(err, contract.ErrStore) {
		t.Fatalf("missing candidate=%v", err)
	}
	bad := elementfact.Anchor{ID: scanID(13), Parent: root, LocalPose: geometry.IdentityPose()}
	if err := s.RestoreAnchor(bad); err != nil {
		t.Fatal(err)
	}
	if _, _, err := NewProjectionMemo(s.View, root, nil).Candidate(bad.ID, contract.RegionQuery{}, r, inverse); !errors.Is(err, contract.ErrStore) {
		t.Fatalf("invalid range=%v", err)
	}
}

func TestProjectionRetainsExactEdgeLimit(t *testing.T) {
	s := elementfact.New()
	root := scanID(100)
	scanAnchor(t, &s, root, "", 0)
	parent := root
	for n := 1; n <= geometry.MaxExactProjectionEdges+1; n++ {
		id := scanID(100 + n)
		scanAnchor(t, &s, id, parent, 0)
		parent = id
	}
	entity := scanEntity(t, &s, scanID(9999), scanID(100+geometry.MaxExactProjectionEdges), 0, 0)
	stats := Stats{}
	memo := NewProjectionMemo(s.View, root, &stats)
	at, err := memo.Anchor(scanID(100 + geometry.MaxExactProjectionEdges))
	if err != nil || at.Edges != geometry.MaxExactProjectionEdges {
		t.Fatalf("at limit=%+v err=%v", at, err)
	}
	if _, err := memo.Anchor(parent); !errors.Is(err, contract.ErrUnrepresentable) {
		t.Fatalf("past limit=%v", err)
	}
	if _, err := memo.Entity(entity); !errors.Is(err, contract.ErrUnrepresentable) {
		t.Fatalf("entity past limit=%v", err)
	}
	if stats.AnchorProjectionEdges != geometry.MaxExactProjectionEdges || stats.EntityProjectionEdges != 0 {
		t.Fatalf("stats=%+v", stats)
	}
}
