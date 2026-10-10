package fact

import (
	"errors"
	"math"
	"testing"

	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	elementfact "ucupa/ucupa-core/internal/element/fact"
	"ucupa/ucupa-core/internal/token"
)

func TestInstallFreshUsesTargetRevisionRangeWithoutMovingDescendants(t *testing.T) {
	e, f, m := tree(t)
	slice := mustSelect(t, e, f, m)
	target := targetTree(t)
	before := target.Clone()
	final := tuple(t, id(51), 8, 10)
	if _, err := Place(slice, targetOwner, cut(), target, m.View, final, 10, nil); err != nil {
		t.Fatal(err)
	}
	serial, err := Install(slice, &target, final, [32]byte{1}, 51, nil)
	if err != nil || serial != 55 {
		t.Fatal(serial, err)
	}
	if err := target.Validate([32]byte{1}, serial, elementfact.ExternalParent{}); err != nil {
		t.Fatal(err)
	}
	if before.Claimed(id(2)) || !target.Claimed(id(2)) {
		t.Fatal("candidate alias")
	}
	for index, old := range slice.Members() {
		if a, ok := target.Anchor(old.ID()); ok {
			if token.RevisionSerial(a.Revision) != uint64(52+index) {
				t.Fatal("wrong sequence")
			}
			if old.ID() == slice.Boundary() {
				a.Parent, a.LocalPose, a.Range = old.Anchor.Parent, old.Anchor.LocalPose, old.Anchor.Range
			}
			a.Revision = old.Anchor.Revision
			if a != old.Anchor {
				t.Fatal("ancestor contents changed")
			}
		} else {
			v, _ := target.Entity(old.ID())
			if token.RevisionSerial(v.Revision) != uint64(52+index) {
				t.Fatal("wrong sequence")
			}
			v.Revision = old.Entity.Revision
			if v != old.Entity {
				t.Fatal("descendant contents changed")
			}
		}
	}
	installed := target.Clone()
	if _, err := Install(slice, &target, final, [32]byte{1}, serial, nil); !errors.Is(err, contract.ErrConflict) || !installed.Equal(target) {
		t.Fatal("overwrote claimed UUID", err)
	}
}

func TestInstallFreshRefusesOverflowAndUnconnectedFacet(t *testing.T) {
	e, f, m := tree(t)
	slice := mustSelect(t, e, f, m)
	target := targetTree(t)
	before := target.Clone()
	final := tuple(t, id(51), 0, 10)
	if _, err := Install(slice, &target, final, [32]byte{2}, math.MaxUint64-1, nil); !errors.Is(err, values.PublicResourceExhausted) || !target.Equal(before) {
		t.Fatal("overflow partially installed", err)
	}
	for _, state := range []contract.FacetAttachmentState{contract.FacetActive, contract.FacetDetached, contract.FacetRetired} {
		candidate := slice
		candidate.members = slice.Members()
		candidate.members[0].Facet.Attachment.State = state
		if _, err := Place(candidate, targetOwner, cut(), target, m.View, final, 10, nil); !errors.Is(err, contract.ErrAttachmentStateConflict) {
			t.Fatal("accepted external Facet without continuity", err)
		}
	}
}

func TestInstallReturnChecksEntireAllowanceBeforeChangingCandidate(t *testing.T) {
	e, f, m := tree(t)
	slice := mustSelect(t, e, f, m)
	ids := []values.ElementID{id(2), id(3), id(4), id(6)}
	if err := e.Depart(ids, targetOwner, "out"); err != nil {
		t.Fatal(err)
	}
	exact := make(map[values.ElementID]elementfact.Departure)
	for _, member := range ids {
		exact[member], _ = e.Departure(member)
	}
	for _, kind := range []string{"missing", "wrong-generation", "extra"} {
		bad := make(map[values.ElementID]elementfact.Departure)
		for member, departure := range exact {
			bad[member] = departure
		}
		switch kind {
		case "missing":
			delete(bad, id(6))
		case "wrong-generation":
			bad[id(6)] = elementfact.Departure{Operation: "other", Revision: revision(6)}
		case "extra":
			bad[id(999)] = exact[id(2)]
		}
		candidate := e.Clone()
		if _, err := Install(slice, &candidate, tuple(t, id(1), 0, 10), [32]byte{1}, 6, bad); !errors.Is(err, contract.ErrConflict) || !candidate.Equal(e) {
			t.Fatal("partial return on failed allowance", kind, err)
		}
	}
	serial, err := Install(slice, &e, tuple(t, id(1), 0, 10), [32]byte{1}, 6, exact)
	if err != nil || serial != 10 {
		t.Fatal("matched return", err)
	}
	if err := e.Validate([32]byte{1}, serial, elementfact.ExternalParent{}); err != nil {
		t.Fatal("invalid completed return", err)
	}
}
