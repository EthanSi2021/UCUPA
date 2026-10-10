package fact

import (
	"errors"
	"testing"

	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	elementfact "ucupa/ucupa-core/internal/element/fact"
	facetfact "ucupa/ucupa-core/internal/facet/fact"
	"ucupa/ucupa-core/internal/geometry"
	mountfact "ucupa/ucupa-core/internal/mount/fact"
)

func TestCarrierSelectionExternalClosureAndExactBounds(t *testing.T) {
	e := elementfact.New()
	f := facetfact.New()
	m := mountfact.New()
	external := elementfact.ExternalParent{ID: id(10), Range: sphere(t, 20), Present: true, Admitted: true}
	s, err := SelectCarrier(sourceOwner, cut(), 0, e.View, f.View, m.View, external, 3)
	if err != nil || len(s.Members()) != 0 || s.Profile() != CarrierReplacement {
		t.Fatal("empty carried closure", err)
	}
	for _, a := range []elementfact.Anchor{
		{ID: id(2), Parent: id(10), LocalPose: geometry.IdentityPose(), Range: geometry.BoundedRange(sphere(t, 10)), Revision: revision(2)},
		{ID: id(3), Parent: id(2), LocalPose: geometry.IdentityPose(), Range: geometry.BoundedRange(sphere(t, 5)), Revision: revision(3)},
	} {
		if err := e.RestoreAnchor(a); err != nil {
			t.Fatal(err)
		}
		f.ElementCreated(a.ID, cut())
	}
	s, err = SelectCarrier(sourceOwner, cut(), 3, e.View, f.View, m.View, external, 2)
	if err != nil || len(s.Members()) != 2 || s.Boundary() != id(10) || len(s.Ancestors()) != 0 {
		t.Fatal("nested carried closure", err)
	}
	if _, err := SelectCarrier(sourceOwner, cut(), 3, e.View, f.View, m.View, external, 1); !errors.Is(err, values.PublicResourceExhausted) {
		t.Fatal("truncated closure", err)
	}
	if _, err := Place(s, targetOwner, cut(), elementfact.New(), m.View, tuple(t, id(10), 0, 1), 10, nil); !errors.Is(err, contract.ErrMountCoordinationRequired) {
		t.Fatal("ordinary placement consumed carrier closure", err)
	}
	bad := e.Clone()
	if err := bad.RestoreEntity(elementfact.Entity{ID: id(4), Parent: id(999), LocalPose: geometry.IdentityPose(), Range: sphere(t, 1), Revision: revision(4)}); err != nil {
		t.Fatal(err)
	}
	if _, err := SelectCarrier(sourceOwner, cut(), 4, bad.View, f.View, m.View, external, 10); err == nil {
		t.Fatal("orphan hidden outside closure")
	}
	m.ReplaceOutgoing(id(3), mountfact.Record{State: contract.MountActive})
	if _, err := SelectCarrier(sourceOwner, cut(), 3, e.View, f.View, m.View, external, 3); !errors.Is(err, contract.ErrMountCoordinationRequired) {
		t.Fatal("external content omitted from manifest", err)
	}
	root := elementfact.New()
	if err := root.RestoreAnchor(elementfact.Anchor{ID: id(10), Revision: revision(10), Range: geometry.UnboundedRootRange()}); err != nil {
		t.Fatal(err)
	}
	if _, err := SelectCarrier(sourceOwner, cut(), 10, root.View, f.View, m.View, external, 10); err == nil {
		t.Fatal("parent mount fabricated as root")
	}
}
