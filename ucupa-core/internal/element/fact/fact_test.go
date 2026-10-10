package fact

import (
	"testing"

	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	"ucupa/ucupa-core/internal/geometry"
	"ucupa/ucupa-core/internal/token"
)

const (
	testRootID     values.ElementID = "00000000-0000-4000-8000-000000000001"
	testAnchorID   values.ElementID = "00000000-0000-4000-8000-000000000002"
	testEntityID   values.ElementID = "00000000-0000-4000-8000-000000000003"
	testExternalID values.ElementID = "00000000-0000-4000-8000-000000000004"
)

func testRevision(id values.ElementID, serial uint64) contract.Revision {
	var scope [32]byte
	scope[0] = 1
	return token.RevisionFromParts(scope, id, serial)
}

func testSphere(t *testing.T, radius float64) contract.BoundedSphere {
	t.Helper()
	sphere, err := geometry.NewBoundedSphere(radius)
	if err != nil {
		t.Fatal(err)
	}
	return sphere
}

func TestPlanApplyCloneAndNoOp(t *testing.T) {
	state := New()

	rootPlan, err := state.Plan(Input{
		Kind:    OperationCreateRoot,
		Element: testRootID,
	}, ParentContext{})
	if err != nil {
		t.Fatal(err)
	}
	rootOutcome, err := state.Apply(rootPlan, testRevision(testRootID, 1))
	if err != nil {
		t.Fatal(err)
	}
	if rootOutcome.Status != contract.MutationCommitted || rootOutcome.Kind != contract.ElementAnchor {
		t.Fatalf("unexpected root outcome: %#v", rootOutcome)
	}

	candidate := state.Clone()
	entityPlan, err := candidate.Plan(Input{
		Kind:    OperationCreateEntity,
		Element: testEntityID,
		Final: contract.SpatialTuple{
			Parent:    testRootID,
			LocalPose: geometry.IdentityPose(),
			Range:     testSphere(t, 1),
		},
	}, ParentContext{})
	if err != nil {
		t.Fatal(err)
	}
	entityOutcome, err := candidate.Apply(entityPlan, testRevision(testEntityID, 2))
	if err != nil {
		t.Fatal(err)
	}
	if entityOutcome.Kind != contract.ElementEntity {
		t.Fatalf("unexpected entity outcome: %#v", entityOutcome)
	}
	if state.Claimed(testEntityID) {
		t.Fatal("applying to clone changed original state")
	}

	beforeNoOp := candidate.Clone()
	entity := entityOutcome.Entity
	noOpPlan, err := candidate.Plan(Input{
		Kind:     OperationUpdateEntity,
		Element:  testEntityID,
		Expected: entity.Revision,
		Final: contract.SpatialTuple{
			Parent:    entity.Parent,
			LocalPose: entity.LocalPose,
			Range:     entity.Range,
		},
	}, ParentContext{})
	if err != nil {
		t.Fatal(err)
	}
	if outcome := noOpPlan.Outcome(); outcome.Status != contract.MutationUnchanged ||
		outcome.Entity != entity {
		t.Fatalf("unexpected no-op outcome: %#v", outcome)
	}
	if !candidate.Equal(beforeNoOp) {
		t.Fatal("planning a no-op changed state")
	}

	beforeReject := candidate.Clone()
	rejected, err := candidate.Plan(Input{
		Kind:    OperationCreateEntity,
		Element: testAnchorID,
		Final: contract.SpatialTuple{
			Parent:    testRootID,
			LocalPose: geometry.IdentityPose(),
			Range:     testSphere(t, 1),
		},
	}, ParentContext{ParentHasMountedContent: true})
	if err != nil {
		t.Fatal(err)
	}
	if outcome := rejected.Outcome(); outcome.Status != contract.MutationRejected ||
		outcome.Rejection != Conflict {
		t.Fatalf("unexpected rejection: %#v", outcome)
	}
	if !candidate.Equal(beforeReject) {
		t.Fatal("planning a rejection changed state")
	}
}

func TestExternalParentAndTombstone(t *testing.T) {
	state := New()
	external := ExternalParent{
		Present:  true,
		ID:       testExternalID,
		Range:    testSphere(t, 3),
		Admitted: true,
	}
	entityPlan, err := state.Plan(Input{
		Kind:    OperationCreateEntity,
		Element: testEntityID,
		Final: contract.SpatialTuple{
			Parent:    testExternalID,
			LocalPose: geometry.IdentityPose(),
			Range:     testSphere(t, 1),
		},
	}, ParentContext{External: external})
	if err != nil {
		t.Fatal(err)
	}
	entityOutcome, err := state.Apply(entityPlan, testRevision(testEntityID, 1))
	if err != nil {
		t.Fatal(err)
	}
	if !state.Claimed(testEntityID) {
		t.Fatal("created entity was not claimed")
	}

	deletePlan, err := state.Plan(Input{
		Kind:     OperationDeleteEntity,
		Element:  testEntityID,
		Expected: entityOutcome.Entity.Revision,
	}, ParentContext{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := state.Apply(deletePlan, testRevision(testEntityID, 2)); err != nil {
		t.Fatal(err)
	}
	if !state.Claimed(testEntityID) {
		t.Fatal("deletion removed claim")
	}
	if _, ok := state.Tombstone(testEntityID); !ok {
		t.Fatal("deletion did not add tombstone")
	}

	rootPlan, err := state.Plan(Input{
		Kind:    OperationCreateRoot,
		Element: testRootID,
	}, ParentContext{External: external})
	if err != nil {
		t.Fatal(err)
	}
	if outcome := rootPlan.Outcome(); outcome.Status != contract.MutationRejected ||
		outcome.Rejection != Conflict {
		t.Fatalf("incoming parent did not block root: %#v", outcome)
	}
}

func TestAnchorCoverageAndMountedDelete(t *testing.T) {
	state := New()
	rootPlan, err := state.Plan(Input{Kind: OperationCreateRoot, Element: testRootID}, ParentContext{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := state.Apply(rootPlan, testRevision(testRootID, 1)); err != nil {
		t.Fatal(err)
	}

	anchorPlan, err := state.Plan(Input{
		Kind:    OperationCreateAnchor,
		Element: testAnchorID,
		Final: contract.SpatialTuple{
			Parent:    testRootID,
			LocalPose: geometry.IdentityPose(),
			Range:     testSphere(t, 5),
		},
	}, ParentContext{})
	if err != nil {
		t.Fatal(err)
	}
	anchorOutcome, err := state.Apply(anchorPlan, testRevision(testAnchorID, 2))
	if err != nil {
		t.Fatal(err)
	}

	entityPlan, err := state.Plan(Input{
		Kind:    OperationCreateEntity,
		Element: testEntityID,
		Final: contract.SpatialTuple{
			Parent:    testAnchorID,
			LocalPose: geometry.IdentityPose(),
			Range:     testSphere(t, 1),
		},
	}, ParentContext{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := state.Apply(entityPlan, testRevision(testEntityID, 3)); err != nil {
		t.Fatal(err)
	}

	shrinkPlan, err := state.Plan(Input{
		Kind:     OperationUpdateAnchor,
		Element:  testAnchorID,
		Expected: anchorOutcome.Anchor.Revision,
		Final: contract.SpatialTuple{
			Parent:    testRootID,
			LocalPose: geometry.IdentityPose(),
			Range:     testSphere(t, 0.5),
		},
	}, ParentContext{})
	if err != nil {
		t.Fatal(err)
	}
	if outcome := shrinkPlan.Outcome(); outcome.Status != contract.MutationRejected ||
		outcome.Rejection != Conflict {
		t.Fatalf("child coverage was not enforced: %#v", outcome)
	}

	deletePlan, err := state.Plan(Input{
		Kind:     OperationDeleteAnchor,
		Element:  testAnchorID,
		Expected: anchorOutcome.Anchor.Revision,
	}, ParentContext{ElementHasMountedContent: true})
	if err != nil {
		t.Fatal(err)
	}
	if outcome := deletePlan.Outcome(); outcome.Status != contract.MutationRejected ||
		outcome.Rejection != Conflict {
		t.Fatalf("mounted-content deletion was not rejected: %#v", outcome)
	}
}
