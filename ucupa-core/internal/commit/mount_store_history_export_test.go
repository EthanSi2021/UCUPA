package commit

import (
	"errors"
	"fmt"

	"go.etcd.io/bbolt"
	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	mountfact "ucupa/ucupa-core/internal/mount/fact"
)

// ValidateChangedMountOutcomeForTest changes only a clone of the specified
// receipt. Live state and the caller's opaque proof cannot be mutated.
func ValidateChangedMountOutcomeForTest(owner *Owner, id contract.OperationID) error {
	owner.RLock()
	defer owner.RUnlock()
	impossible := owner.state.clone()
	operation := impossible.mount.operations[id]
	operation.Changed = !operation.Changed
	impossible.mount.operations[id] = operation
	return validateOperationHistory(impossible)
}

// CheckMountHistoryFaultForTest preserves the five original corruption cases
// and returns only equality and validation results, never the candidate state.
func CheckMountHistoryFaultForTest(owner *Owner, anchor values.ElementID, fault string) (bool, error) {
	owner.RLock()
	defer owner.RUnlock()
	candidate := owner.state.clone()
	switch fault {
	case "lost contact":
		record := candidate.mount.facts.Record(anchor)
		record.Contact = mountfact.Contact{}
		candidate.mount.facts.ReplaceOutgoing(anchor, record)
	case "changed address":
		record := candidate.mount.facts.Record(anchor)
		record.Contact.NextHop = "local:other"
		candidate.mount.facts.ReplaceOutgoing(anchor, record)
	case "original result altered":
		op := candidate.mount.operations["mount-store-establish"]
		op.Changed = false
		candidate.mount.operations["mount-store-establish"] = op
	case "lost original operation":
		delete(candidate.mount.operations, "mount-store-establish")
	case "lost final event":
		candidate.mount.events = candidate.mount.events[:len(candidate.mount.events)-1]
	default:
		return true, fmt.Errorf("unknown mount history fault %q", fault)
	}
	return sameCompleteCoreState(owner.state, candidate), validateCoreState(candidate)
}

// InjectThirdMountStoreResultForTest installs only the original fixed
// after-commit fault: a different but valid durable result must poison the
// current owner rather than be mistaken for either side of its transaction.
func InjectThirdMountStoreResultForTest(owner *Owner, actor values.PrincipalRef, anchor values.ElementID, key contract.MountKey) {
	owner.Lock()
	defer owner.Unlock()
	owner.store.failAfterCommit = func() error {
		third, err := owner.store.reload()
		if err != nil {
			return err
		}
		contact := third.mount.facts.Record(anchor).Contact
		event := MountEvent{Kind: MountEventAddress, BeforeCut: third.currentCut(), Operation: "third-outcome", Authorized: true,
			Address: NextHopBinding{Actor: actor, Element: anchor, Key: key, Expected: contact.Revision, NextHop: contact.NextHop}}
		if err := applyMountEvent(third, event); err != nil {
			return err
		}
		if err := validateCoreState(third); err != nil {
			return err
		}
		if err := owner.store.db.Update(func(tx *bbolt.Tx) error {
			return writeStoredState(tx.Bucket(elementBucketName), tx.Bucket(operationBucketName), tx.Bucket(commitBucketName), tx.Bucket(mountEventBucketName), third)
		}); err != nil {
			return err
		}
		return errors.New("different valid Store became visible")
	}
}
