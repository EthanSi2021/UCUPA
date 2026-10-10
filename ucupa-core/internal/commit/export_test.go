package commit

// InjectWriteFailureForTest is compiled only into commit's test binary. It lets
// external composition tests exercise the real Element operations without
// adding a production fault hook, Store accessor or mutable state getter.
func InjectWriteFailureForTest(owner *Owner, after bool, failure error) {
	owner.Lock()
	defer owner.Unlock()
	if after {
		owner.store.failAfterCommit = func() error { return failure }
	} else {
		owner.store.failBeforeCommit = func() error { return failure }
	}
}

// MatchesStoreForTest compares complete facts and retained history, not only the cut.
func MatchesStoreForTest(owner *Owner) bool {
	owner.RLock()
	defer owner.RUnlock()
	loaded, err := owner.store.reload()
	return err == nil && sameCompleteCoreState(owner.state, loaded)
}
