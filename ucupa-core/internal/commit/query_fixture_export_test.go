package commit

import (
	"fmt"
	"math"
	"testing"

	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	commitbinding "ucupa/ucupa-core/internal/commit/binding"
	elementfact "ucupa/ucupa-core/internal/element/fact"
	"ucupa/ucupa-core/internal/geometry"
)

// BuildQueryEdgeFixtureForTest creates only the original zero-radius chain,
// preserving its fixed IDs, commands and retained history. No arbitrary callback
// or production state access is exported.
func BuildQueryEdgeFixtureForTest(t *testing.T, owner *Owner) []values.ElementID {
	t.Helper()
	root := linearOracleID(20000)
	anchors := []values.ElementID{root}
	zeroRange, err := contract.NewBoundedSphere(0)
	if err != nil {
		t.Fatal(err)
	}
	// These are legal zero-radius owner facts with matching retained history,
	// not a persisted malformed fixture. Direct commits avoid repeatedly cloning
	// the growing state through the public mutation harness. No production hook.
	owner.Lock()
	defer owner.Unlock()
	for depth := 1; depth <= 1025; depth++ {
		id := linearOracleID(uint64(20000 + depth))
		linearOracleCommitDirect(t, owner.state, contract.OperationID(fmt.Sprintf("query-edge-anchor-%d", depth)), commitbinding.Binding{
			Actor: linearOracleActor, Kind: elementfact.OperationCreateAnchor, Element: id,
			Final: contract.SpatialTuple{Parent: anchors[depth-1], LocalPose: geometry.IdentityPose(), Range: zeroRange},
		})
		anchors = append(anchors, id)
	}
	atLimit, shallow := linearOracleID(30000), linearOracleID(30001)
	for _, entity := range []struct{ id, parent values.ElementID }{{atLimit, anchors[1023]}, {shallow, root}} {
		linearOracleCommitDirect(t, owner.state, contract.OperationID("query-edge-"+string(entity.id)), commitbinding.Binding{
			Actor: linearOracleActor, Kind: elementfact.OperationCreateEntity, Element: entity.id,
			Final: contract.SpatialTuple{Parent: entity.parent, LocalPose: geometry.IdentityPose(), Range: zeroRange},
		})
	}
	if err := validateCoreState(owner.state); err != nil {
		t.Fatalf("invalid direct fixture: %v", err)
	}

	return anchors
}

// AddQueryOverflowForTest adds the original first-sorting over-limit entity.
func AddQueryOverflowForTest(t *testing.T, owner *Owner) {
	t.Helper()
	overLimit := linearOracleID(1)
	zeroRange, err := contract.NewBoundedSphere(0)
	if err != nil {
		t.Fatal(err)
	}
	owner.Lock()
	defer owner.Unlock()
	linearOracleCommitDirect(t, owner.state, "query-edge-overflow", commitbinding.Binding{
		Actor: linearOracleActor, Kind: elementfact.OperationCreateEntity, Element: overLimit,
		Final: contract.SpatialTuple{Parent: linearOracleID(21024), LocalPose: geometry.IdentityPose(), Range: zeroRange},
	})
	if err := validateCoreState(owner.state); err != nil {
		t.Fatalf("invalid overflow fixture: %v", err)
	}
}

// BuildQueryRationalFixtureForTest retains the original 40 rotations,
// shrinking ranges and two calculable root siblings.
func BuildQueryRationalFixtureForTest(t *testing.T, owner *Owner) values.ElementID {
	t.Helper()
	root := linearOracleID(40000)
	rotation, err := contract.NewRotation(1, math.SmallestNonzeroFloat64, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	pose, err := contract.NewPose(contract.Vec3{Y: 1}, rotation)
	if err != nil {
		t.Fatal(err)
	}
	parent := root
	owner.Lock()
	defer owner.Unlock()
	for depth := 1; depth <= 40; depth++ {
		id := linearOracleID(uint64(40000 + depth))
		rangeValue, rangeErr := contract.NewBoundedSphere(float64(101 - depth))
		if rangeErr != nil {
			t.Fatal(rangeErr)
		}
		linearOracleCommitDirect(t, owner.state, contract.OperationID(fmt.Sprintf("query-bit-anchor-%d", depth)), commitbinding.Binding{
			Actor: linearOracleActor, Kind: elementfact.OperationCreateAnchor, Element: id,
			Final: contract.SpatialTuple{Parent: parent, LocalPose: pose, Range: rangeValue},
		})
		parent = id
	}
	zeroRange, err := contract.NewBoundedSphere(0)
	if err != nil {
		t.Fatal(err)
	}
	goodA, goodB := linearOracleID(45000), linearOracleID(45001)
	for _, entity := range []struct{ id, parent values.ElementID }{
		{linearOracleID(2), parent}, {linearOracleID(3), parent}, {goodA, root}, {goodB, root},
	} {
		linearOracleCommitDirect(t, owner.state, contract.OperationID("query-bit-entity-"+string(entity.id)), commitbinding.Binding{
			Actor: linearOracleActor, Kind: elementfact.OperationCreateEntity, Element: entity.id,
			Final: contract.SpatialTuple{Parent: entity.parent, LocalPose: geometry.IdentityPose(), Range: zeroRange},
		})
	}
	if err := validateCoreState(owner.state); err != nil {
		t.Fatalf("invalid rational fixture: %v", err)
	}
	return parent
}
