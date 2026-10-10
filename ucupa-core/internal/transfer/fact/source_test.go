package fact

import (
	"errors"
	"fmt"
	"reflect"
	"testing"

	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	elementfact "ucupa/ucupa-core/internal/element/fact"
	facetfact "ucupa/ucupa-core/internal/facet/fact"
	"ucupa/ucupa-core/internal/geometry"
	mountfact "ucupa/ucupa-core/internal/mount/fact"
	"ucupa/ucupa-core/internal/token"
)

const sourceOwner values.FactAuthorityRef = "00000000-0000-4000-8000-000000000010"
const targetOwner values.FactAuthorityRef = "00000000-0000-4000-8000-000000000020"

func id(n int) values.ElementID {
	return values.ElementID(fmt.Sprintf("00000000-0000-4000-8000-%012x", n))
}
func revision(n int) contract.Revision { return token.RevisionFromParts([32]byte{1}, id(n), uint64(n)) }
func cut() contract.CoreReadCut        { return token.FromParts([32]byte{1}, 100) }
func sphere(t *testing.T, radius float64) contract.BoundedSphere {
	t.Helper()
	r, err := geometry.NewBoundedSphere(radius)
	if err != nil {
		t.Fatal(err)
	}
	return r
}
func tuple(t *testing.T, parent values.ElementID, x, radius float64) contract.SpatialTuple {
	t.Helper()
	p := geometry.IdentityPose()
	p.Translation.X = x
	return contract.SpatialTuple{Parent: parent, LocalPose: p, Range: sphere(t, radius)}
}

// The pure-rule fixture is a finite value graph, not a mock owner or public
// transfer. Actual durable owner capture is tested separately in commit.
func tree(t *testing.T) (elementfact.State, facetfact.State, mountfact.State) {
	t.Helper()
	s := elementfact.New()
	anchors := []elementfact.Anchor{
		{ID: id(1), Range: geometry.UnboundedRootRange(), Revision: revision(1)},
		{ID: id(2), Parent: id(1), LocalPose: geometry.IdentityPose(), Range: geometry.BoundedRange(sphere(t, 10)), Revision: revision(2)},
		{ID: id(3), Parent: id(2), LocalPose: geometry.IdentityPose(), Range: geometry.BoundedRange(sphere(t, 5)), Revision: revision(3)},
		{ID: id(5), Parent: id(1), LocalPose: geometry.IdentityPose(), Range: geometry.BoundedRange(sphere(t, 20)), Revision: revision(5)},
	}
	for _, a := range anchors {
		if err := s.RestoreAnchor(a); err != nil {
			t.Fatal(err)
		}
	}
	for _, e := range []elementfact.Entity{
		{ID: id(4), Parent: id(3), LocalPose: geometry.IdentityPose(), Range: sphere(t, 2), Revision: revision(4)},
		{ID: id(6), Parent: id(2), LocalPose: geometry.IdentityPose(), Range: sphere(t, 1), Revision: revision(6)},
	} {
		if err := s.RestoreEntity(e); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.Validate([32]byte{1}, 6, elementfact.ExternalParent{}); err != nil {
		t.Fatal(err)
	}
	facets := facetfact.New()
	for n := 1; n <= 6; n++ {
		facets.ElementCreated(id(n), cut())
	}
	return s, facets, mountfact.New()
}

func mustSelect(t *testing.T, e elementfact.State, f facetfact.State, m mountfact.State) Slice {
	t.Helper()
	s, err := Select(sourceOwner, cut(), e.View, f.View, m.View, id(2), 5)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestSelectOwnerLocalClosureAndDetachedCopies(t *testing.T) {
	elements, facets, mounts := tree(t)
	s, err := Select(sourceOwner, cut(), elements.View, facets.View, mounts.View, id(2), 5)
	if err != nil {
		t.Fatal(err)
	}
	if s.Source() != sourceOwner || s.Boundary() != id(2) || s.Cut() != cut() {
		t.Fatal("source binding lost")
	}
	var got []values.ElementID
	for _, m := range s.Members() {
		got = append(got, m.ID())
	}
	if !reflect.DeepEqual(got, []values.ElementID{id(2), id(3), id(4), id(6)}) {
		t.Fatalf("closure: %v", got)
	}
	if a := s.Ancestors(); len(a) != 1 || a[0].ID != id(1) {
		t.Fatalf("ancestors: %+v", a)
	}
	before := s.Members()
	x := s.Members()
	x[0].Anchor.ID = id(99)
	a := s.Ancestors()
	a[0].ID = id(98)
	e, _ := elements.Entity(id(4))
	e.Parent = id(5)
	if err := elements.RestoreEntity(e); err != nil {
		t.Fatal(err)
	}
	facets.ElementDeleted(id(4))
	if !reflect.DeepEqual(s.Members(), before) || s.Ancestors()[0].ID != id(1) {
		t.Fatal("observation aliases caller or source state")
	}
	// A fresh selection sees the change; the old observation does not pretend
	// to be a durable fence that prevented the update.
	fresh, err := Select(sourceOwner, cut(), elements.View, facets.View, mounts.View, id(2), 5)
	if err != nil || len(fresh.Members()) != 3 {
		t.Fatalf("fresh closure: %+v %v", fresh, err)
	}
}

func TestSelectEntityAndExactCapacity(t *testing.T) {
	e, f, m := tree(t)
	for _, tc := range []struct {
		boundary     values.ElementID
		limit, count int
		want         error
	}{
		{id(4), 4, 1, nil}, {id(4), 3, 0, values.PublicResourceExhausted},
		{id(2), 5, 4, nil}, {id(2), 4, 0, values.PublicResourceExhausted},
		{id(2), 2, 0, values.PublicResourceExhausted}, {id(2), 0, 0, contract.ErrInvalid},
		{id(1), 6, 6, nil}, {id(1), 5, 0, values.PublicResourceExhausted}, {id(99), 10, 0, contract.ErrNotFound},
	} {
		s, err := Select(sourceOwner, cut(), e.View, f.View, m.View, tc.boundary, tc.limit)
		if !errors.Is(err, tc.want) || len(s.Members()) != tc.count {
			t.Fatalf("%+v: %d %v", tc, len(s.Members()), err)
		}
		if err != nil && s.Source() != "" {
			t.Fatal("failed selection leaked partial result")
		}
	}
}

func TestSelectDoesNotCarryUnfinishedOrExternalRelations(t *testing.T) {
	for _, tc := range []struct {
		name   string
		member values.ElementID
		state  contract.MountState
		slot   contract.OperationID
		want   error
	}{
		{"active-boundary", id(3), contract.MountActive, "", contract.ErrMountCoordinationRequired},
		{"pending-member", id(2), contract.MountNone, "pending", contract.ErrMountBusy},
		{"pending-ancestor", id(1), contract.MountNone, "pending", contract.ErrMountBusy},
		{"unrelated", id(5), contract.MountActive, "pending", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e, f, m := tree(t)
			m.ReplaceOutgoing(tc.member, mountfact.Record{State: tc.state, Slot: tc.slot})
			_, err := Select(sourceOwner, cut(), e.View, f.View, m.View, id(2), 10)
			if !errors.Is(err, tc.want) {
				t.Fatalf("%v", err)
			}
		})
	}
	e, f, m := tree(t)
	b := facetfact.NewProjectionBuilder()
	for key, r := range f.Records() {
		if key == id(4) {
			r.Pending = values.FacetAcceptanceKey{Core: sourceOwner}
		}
		b.Add(key, r)
	}
	_, err := Select(sourceOwner, cut(), e.View, b.Build().View, m.View, id(2), 10)
	if !errors.Is(err, contract.ErrAttachmentStateConflict) {
		t.Fatalf("pending Facet: %v", err)
	}
}

func TestSelectPreservesSourceFacetRelationship(t *testing.T) {
	e, f, m := tree(t)
	b := facetfact.NewProjectionBuilder()
	for key, r := range f.Records() {
		if key == id(4) {
			r.Attachment.State = contract.FacetActive
			r.Attachment.Key = values.FacetAcceptanceKey{Core: sourceOwner, Facet: targetOwner, Element: key, Proposal: r.Attachment.Revision}
			r.NextHop = "local:facet"
		}
		b.Add(key, r)
	}
	updated := b.Build()
	s, err := Select(sourceOwner, cut(), e.View, updated.View, m.View, id(4), 4)
	want, _ := updated.Record(id(4))
	if err != nil || len(s.Members()) != 1 || s.Members()[0].Facet != want {
		t.Fatalf("source relationship changed: %+v %v", s, err)
	}
}

func TestSelectCycleFailsWithoutRecursion(t *testing.T) {
	e, f, m := tree(t)
	a, _ := e.Anchor(id(2))
	a.Parent = id(3)
	if err := e.RestoreAnchor(a); err != nil {
		t.Fatal(err)
	}
	if _, err := Select(sourceOwner, cut(), e.View, f.View, m.View, id(2), 20); !errors.Is(err, contract.ErrStore) {
		t.Fatalf("cycle: %v", err)
	}
}
