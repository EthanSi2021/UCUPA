package commit

import (
	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
)

// CompleteStateProofForTest is an opaque, test-build-only clone. Its private
// aggregate cannot be read or mutated by external integration tests.
type CompleteStateProofForTest struct{ state *coreState }

func CaptureCompleteStateForTest(owner *Owner) CompleteStateProofForTest {
	owner.RLock()
	defer owner.RUnlock()
	return CompleteStateProofForTest{state: owner.state.clone()}
}

func MatchesCompleteStateForTest(owner *Owner, proof CompleteStateProofForTest) bool {
	owner.RLock()
	defer owner.RUnlock()
	return proof.state != nil && sameCompleteCoreState(proof.state, owner.state)
}

func ValidateCompleteStateForTest(owner *Owner) error {
	owner.RLock()
	defer owner.RUnlock()
	return validateCoreState(owner.state)
}

type MountGuardInspectionForTest struct {
	Anchors, Entities, Claims, Tombstones int
	RootExists, SelectedClaimed           bool
	SelectedKind                          contract.ElementKind
	OperationRetained                     bool
	IncomingRange                         float64
}

func InspectMountGuardForTest(owner *Owner, element values.ElementID, operation contract.OperationID) MountGuardInspectionForTest {
	owner.RLock()
	defer owner.RUnlock()
	counts := owner.state.elements.Counts()
	_, retained := owner.state.operations[operation]
	incoming := owner.state.mount.facts.Incoming()
	return MountGuardInspectionForTest{
		Anchors: counts.Anchors, Entities: counts.Entities, Claims: counts.Claims, Tombstones: counts.Tombstones,
		RootExists: owner.state.elements.RootExists(), SelectedClaimed: owner.state.elements.Claimed(element),
		SelectedKind: owner.state.elements.Kind(element), OperationRetained: retained,
		IncomingRange: incoming.Admission.RangeValue.Radius(),
	}
}
