package value

import (
	"testing"

	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
)

func TestProjectionTraversalIsPrivateAndPreservedByValueCopy(t *testing.T) {
	var page FederationProjectionPage
	want := [32]byte{1, 2, 3}
	SetProjectionTraversal(&page, want)
	if got := ProjectionTraversal(page); got != want {
		t.Fatalf("traversal: got=%x want=%x", got, want)
	}
	copy := page
	SetProjectionTraversal(&copy, [32]byte{9})
	if ProjectionTraversal(page) != want || ProjectionTraversal(copy) == want {
		t.Fatal("projection traversal did not retain value-copy semantics")
	}
}

func TestFederationRangeRetainsOriginalValidation(t *testing.T) {
	sphere, err := contract.NewBoundedSphere(4)
	if err != nil {
		t.Fatal(err)
	}
	bounded, err := NewFederationRange(contract.RangeBoundedSphere, sphere)
	if got, ok := bounded.BoundedSphere(); err != nil || !ok || got != sphere {
		t.Fatalf("bounded: %#v %v %v", bounded, ok, err)
	}
	unbounded, err := NewFederationRange(contract.RangeUnboundedRoot, contract.BoundedSphere{})
	if err != nil || unbounded.Kind() != contract.RangeUnboundedRoot {
		t.Fatalf("unbounded: %#v %v", unbounded, err)
	}
	for _, input := range []struct {
		kind   contract.RangeKind
		sphere contract.BoundedSphere
	}{
		{contract.RangeKind(255), contract.BoundedSphere{}},
		{contract.RangeBoundedSphere, contract.BoundedSphere{}},
		{contract.RangeUnboundedRoot, sphere},
	} {
		if got, err := NewFederationRange(input.kind, input.sphere); err != values.PublicInvalid || got != (contract.Range{}) {
			t.Fatalf("invalid range accepted: %#v err=%v", got, err)
		}
	}
}
