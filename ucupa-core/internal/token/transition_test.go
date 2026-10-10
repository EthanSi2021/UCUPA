package token

import (
	"testing"

	"ucupa/shared/values"
)

func TestTransitionComparisonBindsOwnerStoreOperationBoundaryAndRevision(t *testing.T) {
	scope := [32]byte{1}
	signer := NewPublicSigner(publicRevisionKey, publicRevisionOwner)
	cut := FromParts(scope, 3)
	got := signer.TransitionRevision(cut, "op", publicRevisionElement, 1)
	if !got.Valid() || got.Kind != "transition-revision" || got.Owner != publicRevisionOwner {
		t.Fatalf("invalid comparison: %+v", got)
	}
	if got != signer.TransitionRevision(cut, "op", publicRevisionElement, 1) {
		t.Fatal("same durable inputs did not give exact comparison")
	}
	variants := []values.PublicToken{
		NewPublicSigner(publicRevisionKey, publicRevisionPeer).TransitionRevision(cut, "op", publicRevisionElement, 1),
		NewPublicSigner([32]byte{9}, publicRevisionOwner).TransitionRevision(cut, "op", publicRevisionElement, 1),
		signer.TransitionRevision(FromParts([32]byte{2}, 3), "op", publicRevisionElement, 1),
		signer.TransitionRevision(cut, "other", publicRevisionElement, 1),
		signer.TransitionRevision(cut, "op", values.ElementID(publicRevisionPeer), 1),
		signer.TransitionRevision(cut, "op", publicRevisionElement, 2),
		signer.ElementRevision(RevisionFromParts(scope, publicRevisionElement, 1)),
	}
	for _, changed := range variants {
		if !changed.Valid() || changed == got || changed.Value == got.Value {
			t.Fatalf("domain collision: %+v", changed)
		}
	}
	for _, bad := range []values.PublicToken{
		signer.TransitionRevision(CoreReadCut{}, "op", publicRevisionElement, 1),
		signer.TransitionRevision(cut, "", publicRevisionElement, 1),
		signer.TransitionRevision(cut, "op", "", 1),
		signer.TransitionRevision(cut, "op", publicRevisionElement, 0),
	} {
		if bad != (values.PublicToken{}) {
			t.Fatalf("signed invalid shape: %+v", bad)
		}
	}
}

func TestCarrierComparisonBindsReservationProgressAndDomain(t *testing.T) {
	signer := NewPublicSigner(publicRevisionKey, publicRevisionOwner)
	reservation := FromParts([32]byte{1}, 3)
	progress := FromParts([32]byte{1}, 5)
	got := signer.CarrierRevision(reservation, progress, "op", publicRevisionElement)
	if !got.Valid() || got.Kind != "carrier-revision" || got.Owner != publicRevisionOwner || got != signer.CarrierRevision(reservation, progress, "op", publicRevisionElement) {
		t.Fatal("invalid or unstable carrier comparison", got)
	}
	for _, other := range []values.PublicToken{
		signer.CarrierRevision(reservation, reservation, "op", publicRevisionElement),
		signer.CarrierRevision(FromParts([32]byte{1}, 4), progress, "op", publicRevisionElement),
		signer.CarrierRevision(reservation, progress, "other", publicRevisionElement),
		signer.CarrierRevision(reservation, progress, "op", values.ElementID(publicRevisionPeer)),
		NewPublicSigner(publicRevisionKey, publicRevisionPeer).CarrierRevision(reservation, progress, "op", publicRevisionElement),
		NewPublicSigner([32]byte{9}, publicRevisionOwner).CarrierRevision(reservation, progress, "op", publicRevisionElement),
		signer.TransitionRevision(reservation, "op", publicRevisionElement, 5),
		signer.MountRevision(MountRevisionFromParts([32]byte{1}, publicRevisionElement, 5)),
	} {
		if !other.Valid() || other.Value == got.Value {
			t.Fatal("carrier comparison collision", other)
		}
	}
	for _, bad := range []values.PublicToken{
		signer.CarrierRevision(CoreReadCut{}, progress, "op", publicRevisionElement),
		signer.CarrierRevision(reservation, CoreReadCut{}, "op", publicRevisionElement),
		signer.CarrierRevision(reservation, FromParts([32]byte{2}, 5), "op", publicRevisionElement),
		signer.CarrierRevision(progress, reservation, "op", publicRevisionElement),
		signer.CarrierRevision(reservation, progress, "", publicRevisionElement),
		signer.CarrierRevision(reservation, progress, "op", ""),
	} {
		if bad != (values.PublicToken{}) {
			t.Fatal("invalid carrier input was signed", bad)
		}
	}
}
