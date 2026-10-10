package fact

import (
	"errors"
	"math"
	"reflect"
	"testing"

	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	elementfact "ucupa/ucupa-core/internal/element/fact"
	"ucupa/ucupa-core/internal/geometry"
	mountfact "ucupa/ucupa-core/internal/mount/fact"
)

func targetTree(t *testing.T) elementfact.State {
	t.Helper()
	s := elementfact.New()
	for _, a := range []elementfact.Anchor{
		{ID: id(50), Range: geometry.UnboundedRootRange(), Revision: revision(50)},
		{ID: id(51), Parent: id(50), LocalPose: geometry.IdentityPose(), Range: geometry.BoundedRange(sphere(t, 20)), Revision: revision(51)},
	} {
		if err := s.RestoreAnchor(a); err != nil {
			t.Fatal(err)
		}
	}
	return s
}

func TestPlacementKeepsExactTupleAndOnlyObserves(t *testing.T) {
	e, f, m := tree(t)
	source, err := Select(sourceOwner, cut(), e.View, f.View, m.View, id(2), 5)
	if err != nil {
		t.Fatal(err)
	}
	target := targetTree(t)
	before := target.Clone()
	final := tuple(t, id(51), 8, 10)
	p, err := Place(source, targetOwner, cut(), target, m.View, final, 6, nil)
	if err != nil || p.Final() != final || p.Target() != targetOwner || p.Cut() != cut() {
		t.Fatalf("placement: %+v %v", p, err)
	}
	if !target.Equal(before) {
		t.Fatal("preflight created/reserved target facts")
	}
	ancestors := p.Ancestors()
	if len(ancestors) != 2 || ancestors[0].ID != id(51) || ancestors[1].ID != id(50) {
		t.Fatalf("dependencies %+v", ancestors)
	}
	ancestors[0].ID = id(99)
	if p.Ancestors()[0].ID != id(51) {
		t.Fatal("mutable dependency alias")
	}
	if !reflect.DeepEqual(source.Members(), mustSelect(t, e, f, m).Members()) {
		t.Fatal("placement rewrote source")
	}
}

func TestPlacementRejectsBoundsLineageAndInvalidParent(t *testing.T) {
	e, f, m := tree(t)
	source := mustSelect(t, e, f, m)
	for _, tc := range []struct {
		name  string
		final contract.SpatialTuple
		limit int
		want  error
	}{
		{"parent-overflow", tuple(t, id(51), 11, 10), 6, contract.ErrConflict},
		{"child-not-contained", tuple(t, id(51), 0, 4), 6, contract.ErrConflict},
		{"exact-child-boundary", tuple(t, id(51), 0, 5), 6, nil},
		{"self-cycle", tuple(t, id(2), 0, 10), 6, contract.ErrConflict},
		{"descendant-cycle", tuple(t, id(3), 0, 10), 6, contract.ErrConflict},
		{"absent-parent", tuple(t, id(99), 0, 10), 6, contract.ErrNotFound},
		{"records-exhausted", tuple(t, id(51), 0, 10), 5, values.PublicResourceExhausted},
		{"invalid-range", contract.SpatialTuple{Parent: id(51), LocalPose: geometry.IdentityPose()}, 6, contract.ErrInvalid},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p, err := Place(source, targetOwner, cut(), targetTree(t), m.View, tc.final, tc.limit, nil)
			if !errors.Is(err, tc.want) {
				t.Fatalf("%v", err)
			}
			if err != nil && (p.Target() != "" || len(p.Ancestors()) != 0) {
				t.Fatal("failed placement leaked a partial success")
			}
		})
	}
	for _, deleted := range []bool{false, true} {
		target := targetTree(t)
		if deleted {
			if err := target.RestoreTombstone(id(4), revision(4)); err != nil {
				t.Fatal(err)
			}
		} else {
			entity, _ := e.Entity(id(4))
			entity.Parent = id(51)
			if err := target.RestoreEntity(entity); err != nil {
				t.Fatal(err)
			}
		}
		if _, err := Place(source, targetOwner, cut(), target, m.View, tuple(t, id(51), 0, 10), 6, nil); !errors.Is(err, contract.ErrConflict) {
			t.Fatalf("claimed lineage deleted=%v: %v", deleted, err)
		}
	}
	invalid := tuple(t, id(51), 0, 10)
	invalid.LocalPose.Translation.X = math.Inf(1)
	if _, err := Place(source, targetOwner, cut(), targetTree(t), m.View, invalid, 6, nil); !errors.Is(err, contract.ErrInvalid) {
		t.Fatalf("invalid pose: %v", err)
	}
	if _, err := Place(source, sourceOwner, cut(), targetTree(t), m.View, tuple(t, id(51), 0, 10), 6, nil); !errors.Is(err, contract.ErrConflict) {
		t.Fatalf("same owner: %v", err)
	}
}

func TestPlacementRejectsExternalOrUnfinishedParent(t *testing.T) {
	e, f, m := tree(t)
	source := mustSelect(t, e, f, m)
	for _, tc := range []struct {
		at     values.ElementID
		record mountfact.Record
		want   error
	}{
		{id(51), mountfact.Record{State: contract.MountActive}, contract.ErrMountCoordinationRequired},
		{id(50), mountfact.Record{State: contract.MountNone, Slot: "pending"}, contract.ErrMountBusy},
	} {
		x := m.Clone()
		x.ReplaceOutgoing(tc.at, tc.record)
		if _, err := Place(source, targetOwner, cut(), targetTree(t), x.View, tuple(t, id(51), 0, 10), 6, nil); !errors.Is(err, tc.want) {
			t.Fatalf("parent relation: %v", err)
		}
	}
}
