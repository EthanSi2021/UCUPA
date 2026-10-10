package commit

import "ucupa/ucupa-core/contract"

// MountControlEventCountForTest reads the original journal count under its
// owner's lock. No event list, state or database is exposed.
func MountControlEventCountForTest(owner *Owner) int {
	owner.RLock()
	defer owner.RUnlock()
	return len(owner.state.mount.events)
}

// CheckUnrelatedMountHistoryForTest reproduces exactly the two original
// malformed clone fixtures. The live owner is never replaced or modified.
func CheckUnrelatedMountHistoryForTest(owner *Owner, peer bool) (error, error) {
	owner.RLock()
	defer owner.RUnlock()
	state := owner.state.clone()
	if peer {
		state.mount.attempts[contract.MountAttemptKey{}] = PeerAttempt{}
	} else {
		state.mount.operations["unrelated-bad-history"] = MountOperation{}
	}
	return validateCurrentSnapshot(state), validateCoreState(state)
}
