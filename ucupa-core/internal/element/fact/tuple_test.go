package fact

import (
	"errors"
	"testing"

	"ucupa/ucupa-core/contract"
	"ucupa/ucupa-core/internal/geometry"
)

func TestTupleChecksKeepRootAndEmptyContentShortCircuits(t *testing.T) {
	// These helpers preserve their original validation boundary: their caller
	// validates a root or an empty parent's shape, not the containment loop.
	if inside, err := TupleContainedBy(Anchor{ID: testRootID}, contract.SpatialTuple{}); !inside || err != nil {
		t.Fatalf("root containment changed: %v %v", inside, err)
	}
	state := New()
	if inside, err := state.RangeContainsDirectContent(testRootID, contract.BoundedSphere{}); !inside || err != nil {
		t.Fatalf("empty child set changed: %v %v", inside, err)
	}
	parent := Anchor{ID: testAnchorID, Parent: testRootID, Range: geometry.UnboundedRootRange()}
	if inside, err := TupleContainedBy(parent, contract.SpatialTuple{}); inside || !errors.Is(err, contract.ErrStore) {
		t.Fatalf("non-root unbounded range accepted: %v %v", inside, err)
	}
	if inside, err := TupleContainedByRange(testSphere(t, 1), contract.SpatialTuple{}); inside || !errors.Is(err, contract.ErrStore) {
		t.Fatalf("invalid geometry error changed: %v %v", inside, err)
	}
}
