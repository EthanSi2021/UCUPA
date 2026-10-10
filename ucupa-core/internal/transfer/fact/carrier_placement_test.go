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
	"ucupa/ucupa-core/internal/token"
)

func TestCarrierPlacementRequiresOneUnactivatedCompleteReceiver(t *testing.T) {
	e, f, m := elementfact.New(), facetfact.New(), mountfact.New()
	external := elementfact.ExternalParent{ID: id(10), Range: sphere(t, 20), Present: true, Admitted: true}
	empty, err := SelectCarrier(sourceOwner, cut(), 0, e.View, f.View, m.View, external, 2)
	if err != nil {
		t.Fatal(err)
	}
	if err := PlaceCarrier(empty, targetOwner, cut(), elementfact.New(), m.View, 2, nil, CarrierSettlement{}, CarrierControl{}); err != nil {
		t.Fatal("empty closure", err)
	}
	a := elementfact.Anchor{ID: id(2), Parent: id(10), LocalPose: geometry.IdentityPose(), Range: geometry.BoundedRange(sphere(t, 10)), Revision: revision(2)}
	if err := e.RestoreAnchor(a); err != nil {
		t.Fatal(err)
	}
	f.ElementCreated(a.ID, cut())
	b := a
	b.ID = id(3)
	b.Parent = a.ID
	b.Revision = revision(3)
	if err := e.RestoreAnchor(b); err != nil {
		t.Fatal(err)
	}
	f.ElementCreated(b.ID, cut())
	s, err := SelectCarrier(sourceOwner, cut(), 3, e.View, f.View, m.View, external, 2)
	if err != nil {
		t.Fatal(err)
	}
	if err := PlaceCarrier(s, targetOwner, cut(), elementfact.New(), m.View, 2, nil, CarrierSettlement{}, CarrierControl{}); err != nil {
		t.Fatal("complete closure", err)
	}
	if err := PlaceCarrier(s, targetOwner, cut(), elementfact.New(), m.View, 1, nil, CarrierSettlement{}, CarrierControl{}); !errors.Is(err, values.PublicResourceExhausted) {
		t.Fatal("truncation", err)
	}
	if err := PlaceCarrier(s, sourceOwner, cut(), elementfact.New(), m.View, 2, nil, CarrierSettlement{}, CarrierControl{}); !errors.Is(err, contract.ErrConflict) {
		t.Fatal("same owner", err)
	}
	if err := PlaceCarrier(s, targetOwner, cut(), e, m.View, 2, nil, CarrierSettlement{}, CarrierControl{}); !errors.Is(err, contract.ErrConflict) {
		t.Fatal("overwrite live space", err)
	}
	if err := PlaceCarrier(Slice{}, targetOwner, cut(), elementfact.New(), m.View, 2, nil, CarrierSettlement{}, CarrierControl{}); !errors.Is(err, contract.ErrInvalid) {
		t.Fatal("invalid profile", err)
	}
	// Even an empty receiver with another retained external relation is not
	// fresh; content emptiness does not prove that relation can be superseded.
	k := contract.MountKey{ParentAuthority: sourceOwner, CarrierAuthority: targetOwner, MountAnchorUUID: id(10), ProposalRevision: token.MountRevisionFromParts(token.Scope(cut()), id(10), 1)}
	m.ReplaceIncoming(mountfact.Incoming{Key: k})
	if !k.Valid() {
		t.Fatal("invalid test mount key")
	}
	if err := PlaceCarrier(empty, targetOwner, cut(), elementfact.New(), m.View, 2, nil, CarrierSettlement{}, CarrierControl{}); !errors.Is(err, contract.ErrMountCoordinationRequired) {
		t.Fatal("unproved previous relation", err)
	}
	m = mountfact.New()
	m.ReplaceOutgoing(id(11), mountfact.Record{State: contract.MountDetached})
	if err := PlaceCarrier(empty, targetOwner, cut(), elementfact.New(), m.View, 2, nil, CarrierSettlement{}, CarrierControl{}); !errors.Is(err, contract.ErrMountCoordinationRequired) {
		t.Fatal("unresolved outgoing content", err)
	}
}
