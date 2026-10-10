package commit

import (
	"path/filepath"
	"testing"
	"ucupa/ucupa-core/contract"
	elementfact "ucupa/ucupa-core/internal/element/fact"
)

func TestFoundationCommitSequenceDoesNotIncludeDecisionsOrRetries(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sequence.db")
	fixture := seedFoundationStore(t, path)
	c := openFoundationStore(t, path)
	defer closeFoundationStore(t, c)
	want := []coreCommit{
		{kind: elementCommitted, operation: "fixture-root"},
		{kind: elementCommitted, operation: "fixture-anchor"},
		{kind: elementCommitted, operation: "fixture-entity"},
	}
	assertFoundationCommits(t, c.state, want)
	before := c.state.clone()
	replay, err := applyFoundationStore(c, elementfact.OperationCreateRoot, foundationStoreActor, fixture.rootID, contract.SpatialTuple{}, contract.Revision{}, "fixture-root")
	if err != nil || replay.Status != contract.MutationCommitted || !sameCompleteCoreState(before, c.state) {
		t.Fatalf("exact replay changed durable history: %#v err=%v", replay, err)
	}
	entity, _ := c.state.elements.Entity(fixture.entityID)
	unchanged, err := applyFoundationStore(c, elementfact.OperationUpdateEntity, foundationStoreActor, fixture.entityID, foundationStoreTuple(t, fixture.anchorID, 1, 2), entity.Revision, "unchanged")
	if err != nil || unchanged.Status != contract.MutationUnchanged || !sameCoreFacts(before, c.state) || len(c.state.operations) != len(before.operations)+1 {
		t.Fatalf("unchanged decision: %#v err=%v", unchanged, err)
	}
	assertFoundationCommits(t, c.state, want)
	loaded, err := c.store.reload()
	if err != nil || !sameCompleteCoreState(c.state, loaded) {
		t.Fatalf("decision-only persistence lost state: %v", err)
	}
}

func TestFoundationCompleteEqualityAndCloneIncludeCommitSequence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "equality.db")
	seedFoundationStore(t, path)
	c := openFoundationStore(t, path)
	defer closeFoundationStore(t, c)
	before := c.state
	for _, test := range []struct {
		name   string
		change func(*coreState)
	}{
		{"missing commit", func(s *coreState) { s.commits = s.commits[:len(s.commits)-1] }},
		{"wrong operation", func(s *coreState) { s.commits[0].operation = "fixture-conflict" }},
		{"unknown kind", func(s *coreState) { s.commits[0].kind = commitKind(255) }},
		{"reordered events", func(s *coreState) { s.commits[0], s.commits[1] = s.commits[1], s.commits[0] }},
	} {
		t.Run(test.name, func(t *testing.T) {
			candidate := before.clone()
			if !sameCompleteCoreState(before, candidate) {
				t.Fatal("clone is not equal to original")
			}
			test.change(candidate)
			if !sameCoreFacts(before, candidate) || sameCompleteCoreState(before, candidate) {
				t.Fatal("complete equality failed to distinguish commit-only change")
			}
			if before.commits[0] != (coreCommit{kind: elementCommitted, operation: "fixture-root"}) {
				t.Fatal("clone mutation aliased original commit history")
			}
			if err := validateCoreState(candidate); err == nil {
				t.Fatal("full validation accepted an impossible commit history")
			}
		})
	}
	otherOwner := before.clone()
	otherOwner.authority = foundationStoreOtherAuthority(t, before.authority)
	if sameCoreFacts(before, otherOwner) || sameCompleteCoreState(before, otherOwner) {
		t.Fatal("state equality ignored stable owner identity")
	}
}

func TestFoundationSnapshotCutBoundsDoNotReplayHistory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "snapshot-bounds.db")
	seedFoundationStore(t, path)
	c := openFoundationStore(t, path)
	defer closeFoundationStore(t, c)

	// A current-snapshot check must not assume every owner-local cut issued an
	// Element Revision. This deliberately impossible extra historical event
	// preserves valid current facts but must still fail full Store validation.
	candidate := c.state.clone()
	candidate.cutSerial++
	candidate.commits = append(candidate.commits, candidate.commits[0])
	if err := validateCurrentSnapshot(candidate); err != nil {
		t.Fatalf("snapshot retained the cut == revision+1 assumption: %v", err)
	}
	if err := validateCoreState(candidate); err == nil {
		t.Fatal("full Store validation accepted an unreplayable extra commit")
	}
	for _, test := range []struct {
		name   string
		change func(*coreState)
	}{
		{"zero cut", func(s *coreState) { s.cutSerial = 0 }},
		{"revision reaches cut", func(s *coreState) { s.revisionSerial = s.cutSerial }},
		{"revision overflow", func(s *coreState) { s.revisionSerial = ^uint64(0) }},
		{"history length mismatch", func(s *coreState) { s.cutSerial++ }},
		{"missing identity", func(s *coreState) { s.authority = "" }},
	} {
		t.Run(test.name, func(t *testing.T) {
			invalid := c.state.clone()
			test.change(invalid)
			if err := validateCurrentSnapshot(invalid); err == nil {
				t.Fatal("snapshot accepted invalid owner-local metadata")
			}
		})
	}
}

func assertFoundationCommits(t *testing.T, state *coreState, want []coreCommit) {
	t.Helper()
	if uint64(len(state.commits)) != state.cutSerial-1 || len(state.commits) != len(want) {
		t.Fatalf("commit sequence length: got=%#v cut=%d want=%#v", state.commits, state.cutSerial, want)
	}
	for index, commit := range want {
		if state.commits[index] != commit {
			t.Fatalf("commit %d: got=%#v want=%#v", index, state.commits[index], commit)
		}
	}
}
