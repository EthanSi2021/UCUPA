package commit

import (
	"sync/atomic"

	"ucupa/ucupa-core/contract"
)

// MountStoreFailureForTest exposes only a read-only counter, never the owner,
// Store or hook. Both the type and its constructor exist only in test builds.
type MountStoreFailureForTest struct{ calls atomic.Int64 }

func (fault *MountStoreFailureForTest) Calls() int { return int(fault.calls.Load()) }

// InjectMountStoreFailureForTest preserves the original one-shot hook rearming
// so each reserve/prepare/decide/close/settle commit can fail independently.
func InjectMountStoreFailureForTest(owner *Owner, after bool, ordinal int, failure error) *MountStoreFailureForTest {
	owner.Lock()
	defer owner.Unlock()
	fault := &MountStoreFailureForTest{}
	store := owner.store
	var hook func() error
	hook = func() error {
		if int(fault.calls.Add(1)) == ordinal {
			return failure
		}
		if after {
			store.failAfterCommit = hook
		} else {
			store.failBeforeCommit = hook
		}
		return nil
	}
	if after {
		store.failAfterCommit = hook
	} else {
		store.failBeforeCommit = hook
	}
	return fault
}

// MountAddressRetainedForTest answers only the original receipt-presence
// assertion. It does not reveal the address table or an operation's contents.
func MountAddressRetainedForTest(owner *Owner, id contract.OperationID) bool {
	owner.RLock()
	defer owner.RUnlock()
	_, retained := owner.state.mount.addresses[id]
	return retained
}
