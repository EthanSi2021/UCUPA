package commit

import (
	"fmt"
	"testing"

	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	commitbinding "ucupa/ucupa-core/internal/commit/binding"
	elementfact "ucupa/ucupa-core/internal/element/fact"
	"ucupa/ucupa-core/internal/geometry"
	"ucupa/ucupa-core/internal/token"
)

var linearOracleActor = values.PrincipalRef{Realm: "oracle-realm", Principal: "oracle"}

func linearOracleID(serial uint64) values.ElementID {
	return values.ElementID(fmt.Sprintf("10000000-0000-4000-8000-%012x", serial))
}

// InjectOracleCorruptionForTest applies only the original five oracle faults
// to their fixed fixture. It exposes neither arbitrary state nor a callback.
func InjectOracleCorruptionForTest(owner *Owner, fault string) error {
	owner.Lock()
	defer owner.Unlock()
	state := owner.state
	switch fault {
	case "missing parent":
		anchor, _ := state.elements.Anchor(linearOracleID(51))
		anchor.Parent = linearOracleID(999)
		return state.elements.RestoreAnchor(anchor)
	case "cycle":
		left, _ := state.elements.Anchor(linearOracleID(51))
		right, _ := state.elements.Anchor(linearOracleID(52))
		left.Parent = linearOracleID(52)
		right.Parent = linearOracleID(51)
		if err := state.elements.RestoreAnchor(left); err != nil {
			return err
		}
		return state.elements.RestoreAnchor(right)
	case "root record":
		root, _ := state.elements.Anchor(linearOracleID(50))
		root.Range = contract.Range{}
		return state.elements.RestoreAnchor(root)
	case "filtered Anchor record":
		anchor, _ := state.elements.Anchor(linearOracleID(52))
		anchor.Range = contract.Range{}
		return state.elements.RestoreAnchor(anchor)
	case "filtered Entity record":
		entity, _ := state.elements.Entity(linearOracleID(53))
		entity.Range = contract.BoundedSphere{}
		return state.elements.RestoreEntity(entity)
	default:
		return fmt.Errorf("unknown oracle corruption %q", fault)
	}
}

// BuildOracleEdgeFixtureForTest retains the original exact-depth input and
// matching operation history. It has no caller-supplied mutation callback.
func BuildOracleEdgeFixtureForTest(t *testing.T, owner *Owner) (values.ElementID, values.ElementID) {
	t.Helper()
	zeroRange, err := contract.NewBoundedSphere(0)
	if err != nil {
		t.Fatal(err)
	}
	// Build valid owner truth and matching retained history directly so this
	// oracle boundary test does not spend O(N^2) time cloning growing state in
	// the public mutation harness. Public mutation behavior is covered elsewhere.
	owner.Lock()
	defer owner.Unlock()
	parent := linearOracleID(1000)
	var atLimitParent values.ElementID
	for depth := 1; depth <= geometry.MaxExactProjectionEdges; depth++ {
		anchorID := linearOracleID(uint64(1000 + depth))
		operationID := contract.OperationID(fmt.Sprintf("edge-anchor-%d", depth))
		binding := commitbinding.Binding{
			Actor: linearOracleActor, Kind: elementfact.OperationCreateAnchor, Element: anchorID,
			Final: contract.SpatialTuple{Parent: parent, LocalPose: geometry.IdentityPose(), Range: zeroRange},
		}
		linearOracleCommitDirect(t, owner.state, operationID, binding)
		parent = anchorID
		if depth == geometry.MaxExactProjectionEdges-1 {
			atLimitParent = anchorID
		}
	}
	atLimitEntity := linearOracleID(3000)
	linearOracleCommitDirect(t, owner.state, "edge-at-limit", commitbinding.Binding{
		Actor: linearOracleActor, Kind: elementfact.OperationCreateEntity, Element: atLimitEntity,
		Final: contract.SpatialTuple{Parent: atLimitParent, LocalPose: geometry.IdentityPose(), Range: zeroRange},
	})
	return parent, atLimitEntity
}

// AddOracleOverflowForTest adds exactly the original first-over-limit entity.
func AddOracleOverflowForTest(t *testing.T, owner *Owner) values.ElementID {
	t.Helper()
	zeroRange, err := contract.NewBoundedSphere(0)
	if err != nil {
		t.Fatal(err)
	}
	owner.Lock()
	defer owner.Unlock()
	overLimitEntity := linearOracleID(3001)
	linearOracleCommitDirect(t, owner.state, "edge-over-limit", commitbinding.Binding{
		Actor: linearOracleActor, Kind: elementfact.OperationCreateEntity, Element: overLimitEntity,
		Final: contract.SpatialTuple{Parent: linearOracleID(1000 + geometry.MaxExactProjectionEdges), LocalPose: geometry.IdentityPose(), Range: zeroRange},
	})
	return overLimitEntity
}

func linearOracleCommitDirect(t *testing.T, state *coreState, operationID contract.OperationID, binding commitbinding.Binding) {
	t.Helper()
	outcome, err := decideMutation(state, binding)
	if err != nil || outcome.Status != contract.MutationCommitted {
		t.Fatalf("direct oracle fixture commit %q: outcome=%#v err=%v", operationID, outcome, err)
	}
	state.operations[operationID] = retainedOperation{admission: token.FromParts(state.scope, token.Serial(outcome.cut)-1), binding: binding, outcome: outcome}
	state.commits = append(state.commits, coreCommit{kind: elementCommitted, operation: operationID})
}
