package element_test

import (
	"errors"
	"sync"
	"testing"

	corecontract "ucupa/ucupa-core/contract"
	core "ucupa/ucupa-core/instance"
)

func TestOperationReplayAndBindingConflict(t *testing.T) {
	c := newCore(t)
	first, err := c.Elements().CreateRoot(actor, id(101), "same")
	if err != nil {
		t.Fatal(err)
	}
	replay, err := c.Elements().CreateRoot(actor, id(101), "same")
	if err != nil || replay.Status != corecontract.MutationCommitted || !replay.OutcomeCut.Equal(first.OutcomeCut) || !replay.Record.Anchor.Revision.Equal(first.Record.Anchor.Revision) {
		t.Fatalf("replay %#v %v", replay, err)
	}
	conflict, err := c.Elements().CreateRoot(actor, id(102), "same")
	if !errors.Is(err, corecontract.ErrOperationConflict) || conflict.Status != 0 {
		t.Fatalf("binding conflict %#v %v", conflict, err)
	}
}

func TestRetainedNegativeAndCAS(t *testing.T) {
	c := newCore(t)
	root := createRoot(t, c, id(110), "root")
	entity := createEntity(t, c, id(111), tuple(t, root.Anchor.ID, pose(t, 0, 0, 0), 1), "entity")
	foreign := newCore(t)
	foreignRoot := createRoot(t, foreign, id(112), "foreign-root")
	result, err := c.Elements().UpdateEntity(actor, entity.Entity.ID, tuple(t, root.Anchor.ID, pose(t, 1, 0, 0), 1), foreignRoot.Anchor.Revision, "foreign-revision")
	assertRejected(t, result, err, corecontract.ErrRevisionConflict)
	replay, err := c.Elements().UpdateEntity(actor, entity.Entity.ID, tuple(t, root.Anchor.ID, pose(t, 1, 0, 0), 1), foreignRoot.Anchor.Revision, "foreign-revision")
	if !errors.Is(err, corecontract.ErrRevisionConflict) || replay.Status != corecontract.MutationRejected || !replay.OutcomeCut.Equal(result.OutcomeCut) {
		t.Fatalf("negative replay %#v %v", replay, err)
	}

	updated, err := c.Elements().UpdateEntity(actor, entity.Entity.ID, tuple(t, root.Anchor.ID, pose(t, 2, 0, 0), 1), entity.Entity.Revision, "update")
	if err != nil {
		t.Fatal(err)
	}
	stale, err := c.Elements().UpdateEntity(actor, entity.Entity.ID, tuple(t, root.Anchor.ID, pose(t, 3, 0, 0), 1), entity.Entity.Revision, "stale")
	assertRejected(t, stale, err, corecontract.ErrRevisionConflict)
	read, err := c.Elements().ReadElement(actor, entity.Entity.ID, nil)
	if err != nil || read.Record.Entity.LocalPose.Translation.X != 2 || !read.Record.Entity.Revision.Equal(updated.Record.Entity.Revision) {
		t.Fatalf("CAS changed state %#v %v", read, err)
	}
}

func TestStaleRevisionPrecedesMissingDestination(t *testing.T) {
	c := newCore(t)
	root := createRoot(t, c, id(113), "precedence-root")
	entity := createEntity(
		t, c, id(114), tuple(t, root.Anchor.ID, pose(t, 0, 0, 0), 1), "precedence-entity",
	)
	updated, err := c.Elements().UpdateEntity(
		actor, entity.Entity.ID,
		tuple(t, root.Anchor.ID, pose(t, 1, 0, 0), 1),
		entity.Entity.Revision, "precedence-update")

	if err != nil || updated.Record == nil {
		t.Fatalf("seed update: %#v err=%v", updated, err)
	}

	missingParent := id(119)
	result, err := c.Elements().UpdateEntity(
		actor, entity.Entity.ID,
		tuple(t, missingParent, pose(t, 2, 0, 0), 1),
		entity.Entity.Revision, "stale-and-missing")

	assertRejected(t, result, err, corecontract.ErrRevisionConflict)
	read, err := c.Elements().ReadElement(actor, entity.Entity.ID, nil)
	if err != nil || read.Record == nil ||
		!read.Record.Entity.Revision.Equal(updated.Record.Entity.Revision) ||
		read.Record.Entity.ContainingAnchorID != root.Anchor.ID {
		t.Fatalf("precedence failure changed state: %#v err=%v", read, err)
	}
	currentMissing, err := c.Elements().UpdateEntity(
		actor, entity.Entity.ID,
		tuple(t, missingParent, pose(t, 2, 0, 0), 1),
		updated.Record.Entity.Revision, "current-and-missing")

	assertRejected(t, currentMissing, err, corecontract.ErrNotFound)
	afterMissing, err := c.Elements().ReadElement(actor, entity.Entity.ID, nil)
	if err != nil || afterMissing.Record == nil || !afterMissing.Cut.Equal(read.Cut) ||
		!afterMissing.Record.Entity.Revision.Equal(updated.Record.Entity.Revision) ||
		afterMissing.Record.Entity.ContainingAnchorID != root.Anchor.ID {
		t.Fatalf("missing destination changed state: %#v err=%v", afterMissing, err)
	}
	createAnchor(
		t, c, missingParent, root.Anchor.ID,
		tuple(t, root.Anchor.ID, pose(t, 0, 0, 0), 10), "create-missing-parent",
	)
	replay, err := c.Elements().UpdateEntity(
		actor, entity.Entity.ID,
		tuple(t, missingParent, pose(t, 2, 0, 0), 1),
		updated.Record.Entity.Revision, "current-and-missing")

	if !errors.Is(err, corecontract.ErrNotFound) || replay.Status != corecontract.MutationRejected ||
		!replay.OutcomeCut.Equal(currentMissing.OutcomeCut) {
		t.Fatalf("missing destination outcome was not retained: %#v err=%v", replay, err)
	}
}

func TestReplayIsAuthorizedBeforeOutcomeDisclosure(t *testing.T) {
	authorizer := &recordingAuthorizer{}
	c, err := core.New(realmPolicyContact{authorizer}, elementContact, corecontract.Config{})
	if err != nil {
		t.Fatal(err)
	}
	first, err := c.Elements().CreateRoot(actor, id(118), "authorization-replay")
	if err != nil || first.Record == nil {
		t.Fatalf("initial commit: %#v err=%v", first, err)
	}

	authorizer.mu.Lock()
	authorizer.deny = true
	authorizer.mu.Unlock()
	denied, err := c.Elements().CreateRoot(actor, id(118), "authorization-replay")
	if !errors.Is(err, corecontract.ErrDenied) || denied != (corecontract.MutationResult{}) {
		t.Fatalf("revoked replay disclosed outcome: %#v err=%v", denied, err)
	}

	authorizer.mu.Lock()
	authorizer.deny = false
	authorizer.mu.Unlock()
	replay, err := c.Elements().CreateRoot(actor, id(118), "authorization-replay")
	if err != nil || replay.Status != corecontract.MutationCommitted || replay.Record == nil ||
		!replay.OutcomeCut.Equal(first.OutcomeCut) ||
		!replay.Record.Anchor.Revision.Equal(first.Record.Anchor.Revision) {
		t.Fatalf("authorized replay changed receipt: %#v err=%v want=%#v", replay, err, first)
	}
}

func TestRetainedNotFoundDoesNotChangeAfterStateAppears(t *testing.T) {
	c := newCore(t)
	root := createRoot(t, c, id(115), "root")
	missingParent := id(116)
	result, err := c.Elements().CreateEntity(actor, id(117), tuple(t, missingParent, pose(t, 0, 0, 0), 1), "missing-parent")
	assertRejected(t, result, err, corecontract.ErrNotFound)
	createAnchor(t, c, missingParent, root.Anchor.ID, tuple(t, root.Anchor.ID, pose(t, 0, 0, 0), 10), "create-parent")
	replay, err := c.Elements().CreateEntity(actor, id(117), tuple(t, missingParent, pose(t, 0, 0, 0), 1), "missing-parent")
	if !errors.Is(err, corecontract.ErrNotFound) || replay.Status != corecontract.MutationRejected || !replay.OutcomeCut.Equal(result.OutcomeCut) {
		t.Fatalf("negative outcome changed after parent appeared: %#v %v", replay, err)
	}
}

func TestForeignCutRejected(t *testing.T) {
	a := newCore(t)
	b := newCore(t)
	ar := createRoot(t, a, id(120), "a-root")
	br := createRoot(t, b, id(120), "b-root")
	read, err := b.Elements().ReadElement(actor, br.Anchor.ID, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = a.Elements().ReadElement(actor, ar.Anchor.ID, &read.Cut); !errors.Is(err, corecontract.ErrInvalid) {
		t.Fatalf("foreign cut %v", err)
	}
}

func TestConcurrentSingleRootAndCAS(t *testing.T) {
	c := newCore(t)
	const count = 16
	var wg sync.WaitGroup
	wg.Add(count)
	results := make(chan error, count)
	for i := 0; i < count; i++ {
		go func(i int) {
			defer wg.Done()
			_, err := c.Elements().CreateRoot(actor, id(uint64(200+i)), corecontract.OperationID("root-"+string(rune('a'+i))))
			results <- err
		}(i)
	}
	wg.Wait()
	close(results)
	success, conflict := 0, 0
	for err := range results {
		if err == nil {
			success++
		} else if errors.Is(err, corecontract.ErrConflict) {
			conflict++
		} else {
			t.Fatalf("unexpected root result %v", err)
		}
	}
	if success != 1 || conflict != count-1 {
		t.Fatalf("success=%d conflict=%d", success, conflict)
	}

}

func TestConcurrentCASHasOneWinner(t *testing.T) {
	c := newCore(t)
	root := createRoot(t, c, id(300), "root")
	entity := createEntity(t, c, id(301), tuple(t, root.Anchor.ID, pose(t, 0, 0, 0), 1), "entity")
	const count = 12
	finals := make([]corecontract.SpatialTuple, count)
	for i := range finals {
		finals[i] = tuple(t, root.Anchor.ID, pose(t, float64(i+1), 0, 0), 1)
	}
	var wg sync.WaitGroup
	wg.Add(count)
	results := make(chan error, count)
	for i := 0; i < count; i++ {
		go func(i int) {
			defer wg.Done()
			_, err := c.Elements().UpdateEntity(actor, entity.Entity.ID, finals[i], entity.Entity.Revision, corecontract.OperationID("update-"+string(rune('a'+i))))
			results <- err
		}(i)
	}
	wg.Wait()
	close(results)
	success, stale := 0, 0
	for err := range results {
		if err == nil {
			success++
		} else if errors.Is(err, corecontract.ErrRevisionConflict) {
			stale++
		} else {
			t.Fatalf("unexpected CAS result %v", err)
		}
	}
	if success != 1 || stale != count-1 {
		t.Fatalf("success=%d stale=%d", success, stale)
	}
}

func TestConcurrentIndependentCommitsHaveUniqueCuts(t *testing.T) {
	c := newCore(t)
	root := createRoot(t, c, id(350), "independent-root")
	const count = 12
	entities := make([]corecontract.ElementRecord, count)
	finals := make([]corecontract.SpatialTuple, count)
	for i := range count {
		entities[i] = createEntity(
			t, c, id(uint64(351+i)),
			tuple(t, root.Anchor.ID, pose(t, float64(i), 0, 0), 1),
			corecontract.OperationID("independent-create-"+string(rune('a'+i))),
		)
		finals[i] = tuple(t, root.Anchor.ID, pose(t, float64(i), 1, 0), 1)
	}

	type observed struct {
		index  int
		result corecontract.MutationResult
		err    error
	}
	start := make(chan struct{})
	results := make(chan observed, count)
	for i := range count {
		go func(i int) {
			<-start
			result, err := c.Elements().UpdateEntity(
				actor,
				entities[i].Entity.ID,
				finals[i],
				entities[i].Entity.Revision,
				corecontract.OperationID("independent-update-"+string(rune('a'+i))))

			results <- observed{index: i, result: result, err: err}
		}(i)
	}
	close(start)

	commits := make([]observed, count)
	cuts := make([]corecontract.CoreReadCut, 0, count)
	for range count {
		observation := <-results
		if observation.err != nil || observation.result.Status != corecontract.MutationCommitted ||
			observation.result.Record == nil || observation.result.Record.Entity == nil {
			t.Fatalf("independent commit %d: %#v err=%v", observation.index, observation.result, observation.err)
		}
		for _, cut := range cuts {
			if observation.result.OutcomeCut.Equal(cut) {
				t.Fatalf("two successful commits shared cut: %#v", observation.result.OutcomeCut)
			}
		}
		cuts = append(cuts, observation.result.OutcomeCut)
		commits[observation.index] = observation
	}
	var finalCut corecontract.CoreReadCut
	for i, commit := range commits {
		read, err := c.Elements().ReadElement(actor, entities[i].Entity.ID, nil)
		if err != nil || read.Record == nil || read.Record.Entity == nil ||
			!read.Record.Entity.Revision.Equal(commit.result.Record.Entity.Revision) ||
			read.Record.Entity.LocalPose.Translation != finals[i].LocalPose.Translation {
			t.Fatalf("independent final truth %d: %#v err=%v", i, read, err)
		}
		if i == 0 {
			finalCut = read.Cut
		} else if !read.Cut.Equal(finalCut) {
			t.Fatalf("final reads selected different cuts: first=%#v current=%#v", finalCut, read.Cut)
		}
	}
	currentIsCommitted := false
	for _, cut := range cuts {
		if finalCut.Equal(cut) {
			currentIsCommitted = true
			break
		}
	}
	if !currentIsCommitted {
		t.Fatalf("current read cut was not produced by a successful commit: %#v", finalCut)
	}
}

func TestConcurrentMutualReparentCannotCreateCycle(t *testing.T) {
	c := newCore(t)
	root := createRoot(t, c, id(400), "cycle-root")
	left := createAnchor(
		t, c, id(401), root.Anchor.ID,
		tuple(t, root.Anchor.ID, pose(t, 0, 0, 0), 10), "cycle-left",
	)
	right := createAnchor(
		t, c, id(402), root.Anchor.ID,
		tuple(t, root.Anchor.ID, pose(t, 0, 0, 0), 10), "cycle-right",
	)

	type observed struct {
		result corecontract.MutationResult
		err    error
	}
	leftFinal := tuple(t, right.Anchor.ID, pose(t, 0, 0, 0), 10)
	rightFinal := tuple(t, left.Anchor.ID, pose(t, 0, 0, 0), 10)
	start := make(chan struct{})
	results := make(chan observed, 2)
	go func() {
		<-start
		result, err := c.Elements().UpdateAnchor(
			actor, left.Anchor.ID, leftFinal,
			left.Anchor.Revision, "left-under-right")

		results <- observed{result: result, err: err}
	}()
	go func() {
		<-start
		result, err := c.Elements().UpdateAnchor(
			actor, right.Anchor.ID, rightFinal,
			right.Anchor.Revision, "right-under-left")

		results <- observed{result: result, err: err}
	}()
	close(start)
	committed, rejected := 0, 0
	for range 2 {
		observation := <-results
		switch {
		case observation.err == nil && observation.result.Status == corecontract.MutationCommitted:
			committed++
		case errors.Is(observation.err, corecontract.ErrConflict) && observation.result.Status == corecontract.MutationRejected:
			rejected++
		default:
			t.Fatalf("unexpected concurrent reparent result: %#v err=%v", observation.result, observation.err)
		}
	}
	if committed != 1 || rejected != 1 {
		t.Fatalf("committed=%d rejected=%d", committed, rejected)
	}
	leftRead, leftErr := c.Elements().ReadElement(actor, left.Anchor.ID, nil)
	rightRead, rightErr := c.Elements().ReadElement(actor, right.Anchor.ID, nil)
	if leftErr != nil || rightErr != nil || leftRead.Record == nil || rightRead.Record == nil {
		t.Fatalf("read concurrent tree: left=%#v/%v right=%#v/%v", leftRead, leftErr, rightRead, rightErr)
	}
	leftParent := *leftRead.Record.Anchor.ParentAnchorID
	rightParent := *rightRead.Record.Anchor.ParentAnchorID
	if leftParent == right.Anchor.ID && rightParent == left.Anchor.ID {
		t.Fatalf("mutual cycle committed: left parent=%s right parent=%s", leftParent, rightParent)
	}
	if _, err := c.Elements().ProjectToAncestor(actor, left.Anchor.ID, root.Anchor.ID, nil); err != nil {
		t.Fatalf("left no longer reaches root: %v", err)
	}
	if _, err := c.Elements().ProjectToAncestor(actor, right.Anchor.ID, root.Anchor.ID, nil); err != nil {
		t.Fatalf("right no longer reaches root: %v", err)
	}
}

func TestConcurrentCreateAndEmptyDeleteCannotProduceOrphan(t *testing.T) {
	c := newCore(t)
	root := createRoot(t, c, id(410), "empty-root")
	anchor := createAnchor(
		t, c, id(411), root.Anchor.ID,
		tuple(t, root.Anchor.ID, pose(t, 0, 0, 0), 10), "empty-anchor",
	)
	entityID := id(412)
	entityFinal := tuple(t, anchor.Anchor.ID, pose(t, 0, 0, 0), 1)

	type observed struct {
		result corecontract.MutationResult
		err    error
	}
	start := make(chan struct{})
	created := make(chan observed, 1)
	deleted := make(chan observed, 1)
	go func() {
		<-start
		result, err := c.Elements().CreateEntity(actor, entityID, entityFinal, "concurrent-child")
		created <- observed{result: result, err: err}
	}()
	go func() {
		<-start
		result, err := c.Elements().DeleteAnchor(actor, anchor.Anchor.ID, anchor.Anchor.Revision, "concurrent-empty-delete")
		deleted <- observed{result: result, err: err}
	}()
	close(start)
	createResult, deleteResult := <-created, <-deleted

	switch {
	case createResult.err == nil && deleteResult.err != nil:
		if createResult.result.Status != corecontract.MutationCommitted ||
			!errors.Is(deleteResult.err, corecontract.ErrConflict) ||
			deleteResult.result.Status != corecontract.MutationRejected {
			t.Fatalf("create-first outcomes: create=%#v/%v delete=%#v/%v", createResult.result, createResult.err, deleteResult.result, deleteResult.err)
		}
	case deleteResult.err == nil && createResult.err != nil:
		if deleteResult.result.Status != corecontract.MutationCommitted ||
			!errors.Is(createResult.err, corecontract.ErrNotFound) ||
			createResult.result.Status != corecontract.MutationRejected {
			t.Fatalf("delete-first outcomes: create=%#v/%v delete=%#v/%v", createResult.result, createResult.err, deleteResult.result, deleteResult.err)
		}
	default:
		t.Fatalf("non-serializable outcomes: create=%#v/%v delete=%#v/%v", createResult.result, createResult.err, deleteResult.result, deleteResult.err)
	}

	anchorRead, anchorErr := c.Elements().ReadElement(actor, anchor.Anchor.ID, nil)
	entityRead, entityErr := c.Elements().ReadElement(actor, entityID, nil)
	if anchorErr != nil || entityErr != nil {
		t.Fatalf("read final state: anchor=%#v/%v entity=%#v/%v", anchorRead, anchorErr, entityRead, entityErr)
	}
	if anchorRead.State != entityRead.State ||
		(anchorRead.State != corecontract.ExistencePresent && anchorRead.State != corecontract.ExistenceNotFound) {
		t.Fatalf("orphan or invalid final state: anchor=%#v entity=%#v", anchorRead, entityRead)
	}
}
