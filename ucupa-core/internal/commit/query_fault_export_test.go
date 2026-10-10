package commit

import (
	"fmt"

	"ucupa/ucupa-core/contract"
)

// InjectQueryRegionCorruptionForTest applies only the seven original named
// faults to their fixed fixture. No state pointer or mutation callback escapes.
func InjectQueryRegionCorruptionForTest(owner *Owner, fault string) error {
	owner.Lock()
	defer owner.Unlock()
	root, anchor, entity := linearOracleID(60000), linearOracleID(60001), linearOracleID(60002)
	switch fault {
	case "missing parent":
		value, _ := owner.state.elements.Entity(entity)
		value.Parent = linearOracleID(69999)
		_ = owner.state.elements.RestoreEntity(value)
	case "anchor cycle":
		value, _ := owner.state.elements.Anchor(anchor)
		value.Parent = anchor
		_ = owner.state.elements.RestoreAnchor(value)
	case "root range":
		value, _ := owner.state.elements.Anchor(root)
		child, _ := owner.state.elements.Anchor(anchor)
		value.Range = child.Range
		_ = owner.state.elements.RestoreAnchor(value)
	case "excluded kind":
		value, _ := owner.state.elements.Entity(entity)
		value.LocalPose = contract.Pose{}
		_ = owner.state.elements.RestoreEntity(value)
	case "outside scope":
		value, _ := owner.state.elements.Anchor(root)
		value.Revision = contract.Revision{}
		_ = owner.state.elements.RestoreAnchor(value)
	case "unavailable state":
		owner.state = nil
	case "closed":
		owner.closed = true
	default:
		return fmt.Errorf("unknown query corruption %q", fault)
	}
	return nil
}

// QueryOutcomesMatchForTest compares against the same opaque complete proof:
// only the original retained-operation delta and full fact predicate escape.
func QueryOutcomesMatchForTest(owner *Owner, before CompleteStateProofForTest) (int, bool) {
	owner.RLock()
	defer owner.RUnlock()
	if owner.state == nil || before.state == nil {
		return 0, false
	}
	return len(owner.state.operations) - len(before.state.operations), sameCoreFacts(owner.state, before.state)
}
