package element_test

import (
	"errors"
	"fmt"
	"math"
	"reflect"
	"testing"

	"ucupa/shared/values"
	corecontract "ucupa/ucupa-core/contract"
	core "ucupa/ucupa-core/instance"
)

func TestRootRecordAndStructure(t *testing.T) {
	c := newCore(t)
	root := createRoot(t, c, id(1), "root")
	if root.Kind != corecontract.ElementAnchor || root.Anchor == nil || root.Entity != nil {
		t.Fatalf("root variant: %#v", root)
	}
	if root.Anchor.ParentAnchorID != nil || root.Anchor.LocalPose != nil || root.Anchor.Range.Kind() != corecontract.RangeUnboundedRoot {
		t.Fatalf("root shape: %#v", root.Anchor)
	}
	if !root.Anchor.Revision.Equal(root.Anchor.Revision) || (corecontract.Revision{}).Equal(root.Anchor.Revision) {
		t.Fatal("revision equality invalid")
	}
	result, err := c.Elements().CreateRoot(actor, id(2), "second-root")
	assertRejected(t, result, err, corecontract.ErrConflict)
	read, err := c.Elements().ReadElement(actor, id(1), nil)
	if err != nil || read.State != corecontract.ExistencePresent || read.Record == nil {
		t.Fatalf("read root: %#v %v", read, err)
	}
	absent, err := c.Elements().ReadElement(actor, id(99), nil)
	if err != nil || absent.State != corecontract.ExistenceNotFound || absent.Record != nil {
		t.Fatalf("read absence: %#v %v", absent, err)
	}
}

func TestRangeContainmentAndAtomicCreate(t *testing.T) {
	c := newCore(t)
	root := createRoot(t, c, id(10), "root")
	parent := createAnchor(t, c, id(11), root.Anchor.ID, tuple(t, root.Anchor.ID, pose(t, 0, 0, 0), 10), "parent")
	tangent := createEntity(t, c, id(12), tuple(t, parent.Anchor.ID, pose(t, 7, 0, 0), 3), "tangent")
	if tangent.Entity.Range.Radius() != 3 {
		t.Fatalf("radius %v", tangent.Entity.Range.Radius())
	}
	outside := tuple(t, parent.Anchor.ID, pose(t, 7, 0, 0), 3)
	outside.LocalPose = pose(t, 7.000000000000001, 0, 0)
	result, err := c.Elements().CreateEntity(actor, id(13), outside, "outside")
	assertRejected(t, result, err, corecontract.ErrConflict)
	read, err := c.Elements().ReadElement(actor, id(13), nil)
	if err != nil || read.State != corecontract.ExistenceNotFound {
		t.Fatalf("failed create leaked state: %#v %v", read, err)
	}
}

func TestUpdateReparentAndDelete(t *testing.T) {
	c := newCore(t)
	root := createRoot(t, c, id(20), "root")
	left := createAnchor(t, c, id(21), root.Anchor.ID, tuple(t, root.Anchor.ID, pose(t, 0, 0, 0), 20), "left")
	right := createAnchor(t, c, id(22), root.Anchor.ID, tuple(t, root.Anchor.ID, pose(t, 100, 0, 0), 20), "right")
	subject := createAnchor(t, c, id(23), left.Anchor.ID, tuple(t, left.Anchor.ID, pose(t, 1, 0, 0), 5), "subject")
	leaf := createEntity(t, c, id(24), tuple(t, subject.Anchor.ID, pose(t, 1, 0, 0), 1), "leaf")
	child := createAnchor(t, c, id(25), subject.Anchor.ID, tuple(t, subject.Anchor.ID, pose(t, -1, 0, 0), 2), "child")
	leafBefore, err := c.Elements().ReadElement(actor, leaf.Entity.ID, nil)
	if err != nil || leafBefore.Record == nil {
		t.Fatalf("leaf before reparent: %#v err=%v", leafBefore, err)
	}
	childBefore, err := c.Elements().ReadElement(actor, child.Anchor.ID, nil)
	if err != nil || childBefore.Record == nil {
		t.Fatalf("child before reparent: %#v err=%v", childBefore, err)
	}

	moved, err := c.Elements().UpdateAnchor(actor, subject.Anchor.ID, tuple(t, right.Anchor.ID, pose(t, 2, 0, 0), 5), subject.Anchor.Revision, "move-same-radius")
	if err != nil || moved.Status != corecontract.MutationCommitted || moved.Record == nil ||
		moved.Record.Anchor == nil || moved.Record.Anchor.ID != subject.Anchor.ID ||
		moved.Record.Anchor.Revision.Equal(subject.Anchor.Revision) ||
		moved.Record.Anchor.ParentAnchorID == nil || *moved.Record.Anchor.ParentAnchorID != right.Anchor.ID {
		t.Fatalf("same-radius reparent: %#v %v", moved, err)
	}
	movedRange, ok := moved.Record.Anchor.Range.BoundedSphere()
	if !ok || movedRange.Radius() != 5 {
		t.Fatalf("same-radius reparent changed Range: %#v", moved.Record.Anchor.Range)
	}
	leafAfter, err := c.Elements().ReadElement(actor, leaf.Entity.ID, nil)
	if err != nil || leafAfter.Record == nil || !reflect.DeepEqual(leafAfter.Record, leafBefore.Record) {
		t.Fatalf("Entity descendant changed: before=%#v after=%#v err=%v", leafBefore, leafAfter, err)
	}
	childAfter, err := c.Elements().ReadElement(actor, child.Anchor.ID, nil)
	if err != nil || childAfter.Record == nil || !reflect.DeepEqual(childAfter.Record, childBefore.Record) {
		t.Fatalf("Anchor descendant changed: before=%#v after=%#v err=%v", childBefore, childAfter, err)
	}

	updated, err := c.Elements().UpdateAnchor(actor, subject.Anchor.ID, tuple(t, right.Anchor.ID, pose(t, 2, 0, 0), 6), moved.Record.Anchor.Revision, "expand")
	if err != nil || updated.Status != corecontract.MutationCommitted || updated.Record == nil || updated.Record.Anchor == nil {
		t.Fatalf("expand: %#v err=%v", updated, err)
	}

	unchanged, err := c.Elements().UpdateAnchor(actor, subject.Anchor.ID, tuple(t, right.Anchor.ID, pose(t, 2, 0, 0), 6), updated.Record.Anchor.Revision, "unchanged")
	if err != nil || unchanged.Status != corecontract.MutationUnchanged || unchanged.Record == nil ||
		unchanged.Record.Anchor == nil || !unchanged.OutcomeCut.Equal(updated.OutcomeCut) ||
		!reflect.DeepEqual(unchanged.Record, updated.Record) {
		t.Fatalf("unchanged: %#v %v", unchanged, err)
	}
	unchangedRead, err := c.Elements().ReadElement(actor, subject.Anchor.ID, nil)
	if err != nil || unchangedRead.Record == nil || !unchangedRead.Cut.Equal(updated.OutcomeCut) ||
		!reflect.DeepEqual(unchangedRead.Record, updated.Record) {
		t.Fatalf("unchanged advanced or changed truth: %#v err=%v", unchangedRead, err)
	}
	shrunk, err := c.Elements().UpdateAnchor(actor, subject.Anchor.ID, tuple(t, right.Anchor.ID, pose(t, 2, 0, 0), 5), updated.Record.Anchor.Revision, "shrink")
	if err != nil || shrunk.Status != corecontract.MutationCommitted || shrunk.Record == nil || shrunk.Record.Anchor == nil {
		t.Fatalf("shrink: %#v err=%v", shrunk, err)
	}

	nonempty, err := c.Elements().DeleteAnchor(actor, subject.Anchor.ID, shrunk.Record.Anchor.Revision, "delete-nonempty")
	assertRejected(t, nonempty, err, corecontract.ErrConflict)
	deletedLeaf, err := c.Elements().DeleteEntity(actor, leaf.Entity.ID, leaf.Entity.Revision, "delete-leaf")
	if err != nil || deletedLeaf.Status != corecontract.MutationCommitted || deletedLeaf.Record != nil || deletedLeaf.TombstoneRevision.Equal(corecontract.Revision{}) {
		t.Fatalf("delete leaf: %#v %v", deletedLeaf, err)
	}
	deletedChild, err := c.Elements().DeleteAnchor(actor, child.Anchor.ID, child.Anchor.Revision, "delete-child")
	if err != nil || deletedChild.Status != corecontract.MutationCommitted {
		t.Fatalf("delete child: %#v %v", deletedChild, err)
	}
	deletedAnchor, err := c.Elements().DeleteAnchor(actor, subject.Anchor.ID, shrunk.Record.Anchor.Revision, "delete-anchor")
	if err != nil || deletedAnchor.Status != corecontract.MutationCommitted {
		t.Fatalf("delete anchor: %#v %v", deletedAnchor, err)
	}
	reuse, err := c.Elements().CreateEntity(actor, subject.Anchor.ID, tuple(t, right.Anchor.ID, pose(t, 0, 0, 0), 1), "reuse")
	assertRejected(t, reuse, err, corecontract.ErrConflict)
}

func TestRootIsImmutableKindsAreClosedAndCyclesAreRejected(t *testing.T) {
	c := newCore(t)
	root := createRoot(t, c, id(25), "immutable-root")

	updatedRoot, err := c.Elements().UpdateAnchor(
		actor,
		root.Anchor.ID,
		tuple(t, root.Anchor.ID, pose(t, 0, 0, 0), 1),
		root.Anchor.Revision,
		"update-root")

	assertRejected(t, updatedRoot, err, corecontract.ErrConflict)
	deletedRoot, err := c.Elements().DeleteAnchor(actor, root.Anchor.ID, root.Anchor.Revision, "delete-root")
	assertRejected(t, deletedRoot, err, corecontract.ErrConflict)

	parent := createAnchor(
		t, c, id(26), root.Anchor.ID,
		tuple(t, root.Anchor.ID, pose(t, 0, 0, 0), 20), "parent",
	)
	child := createAnchor(
		t, c, id(27), parent.Anchor.ID,
		tuple(t, parent.Anchor.ID, pose(t, 0, 0, 0), 10), "child",
	)
	entity := createEntity(
		t, c, id(28), tuple(t, parent.Anchor.ID, pose(t, 0, 0, 0), 1), "entity",
	)

	crossEntity, err := c.Elements().CreateEntity(
		actor, parent.Anchor.ID,
		tuple(t, root.Anchor.ID, pose(t, 0, 0, 0), 1), "anchor-as-entity")

	assertRejected(t, crossEntity, err, corecontract.ErrConflict)
	crossAnchor, err := c.Elements().CreateAnchor(
		actor, entity.Entity.ID,
		tuple(t, root.Anchor.ID, pose(t, 0, 0, 0), 1), "entity-as-anchor")

	assertRejected(t, crossAnchor, err, corecontract.ErrConflict)

	before, err := c.Elements().ReadElement(actor, parent.Anchor.ID, nil)
	if err != nil || before.Record == nil {
		t.Fatalf("read before cycle attempt: %#v err=%v", before, err)
	}
	cycle, err := c.Elements().UpdateAnchor(
		actor,
		parent.Anchor.ID,
		tuple(t, child.Anchor.ID, pose(t, 0, 0, 0), 20),
		parent.Anchor.Revision,
		"cycle")

	assertRejected(t, cycle, err, corecontract.ErrConflict)
	after, err := c.Elements().ReadElement(actor, parent.Anchor.ID, nil)
	if err != nil || after.Record == nil ||
		!after.Record.Anchor.Revision.Equal(before.Record.Anchor.Revision) ||
		after.Record.Anchor.ParentAnchorID == nil ||
		*after.Record.Anchor.ParentAnchorID != root.Anchor.ID ||
		!after.Cut.Equal(before.Cut) {
		t.Fatalf("cycle rejection changed facts: before=%#v after=%#v err=%v", before, after, err)
	}
}

func TestListDirectStableCutAndOrdering(t *testing.T) {
	c := newCore(t)
	root := createRoot(t, c, id(30), "root")
	createEntity(t, c, id(33), tuple(t, root.Anchor.ID, pose(t, 0, 0, 0), 1), "e33")
	createAnchor(t, c, id(31), root.Anchor.ID, tuple(t, root.Anchor.ID, pose(t, 0, 0, 0), 1), "a31")
	createEntity(t, c, id(32), tuple(t, root.Anchor.ID, pose(t, 0, 0, 0), 1), "e32")
	first, err := c.Elements().ListDirect(actor, root.Anchor.ID, 2, nil)
	if err != nil || len(first.Records) != 2 || first.Continuation == nil {
		t.Fatalf("first page: %#v %v", first, err)
	}
	if recordID(first.Records[0]) != id(31) || recordID(first.Records[1]) != id(32) {
		t.Fatalf("order: %#v", first.Records)
	}
	second, err := c.Elements().ListDirect(actor, root.Anchor.ID, 2, first.Continuation)
	if err != nil || len(second.Records) != 1 || second.Continuation != nil || !second.Cut.Equal(first.Cut) {
		t.Fatalf("second: %#v %v", second, err)
	}

	staleFirst, err := c.Elements().ListDirect(actor, root.Anchor.ID, 1, nil)
	if err != nil {
		t.Fatal(err)
	}
	createEntity(t, c, id(34), tuple(t, root.Anchor.ID, pose(t, 0, 0, 0), 1), "e34")
	page, err := c.Elements().ListDirect(actor, root.Anchor.ID, 1, staleFirst.Continuation)
	if !errors.Is(err, corecontract.ErrReadCutExpired) || len(page.Records) != 0 {
		t.Fatalf("stale continuation: %#v %v", page, err)
	}
}

func TestListDirectEmptyLimitsAndForeignContinuation(t *testing.T) {
	c := newCore(t)
	root := createRoot(t, c, id(35), "list-root")
	empty, err := c.Elements().ListDirect(actor, root.Anchor.ID, 1, nil)
	if err != nil || len(empty.Records) != 0 || empty.Continuation != nil ||
		empty.Cut.Equal(corecontract.CoreReadCut{}) {
		t.Fatalf("empty page: %#v err=%v", empty, err)
	}
	for _, limit := range []uint16{0, 257} {
		page, listErr := c.Elements().ListDirect(actor, root.Anchor.ID, limit, nil)
		if !errors.Is(listErr, corecontract.ErrInvalid) || len(page.Records) != 0 ||
			page.Continuation != nil || !page.Cut.Equal(corecontract.CoreReadCut{}) {
			t.Fatalf("limit %d: %#v err=%v", limit, page, listErr)
		}
	}

	foreign := newCore(t)
	foreignRoot := createRoot(t, foreign, root.Anchor.ID, "foreign-list-root")
	createAnchor(
		t, foreign, id(36), foreignRoot.Anchor.ID,
		tuple(t, foreignRoot.Anchor.ID, pose(t, 0, 0, 0), 1), "foreign-a",
	)
	createEntity(
		t, foreign, id(37),
		tuple(t, foreignRoot.Anchor.ID, pose(t, 0, 0, 0), 1), "foreign-e",
	)
	first, err := foreign.Elements().ListDirect(actor, foreignRoot.Anchor.ID, 1, nil)
	if err != nil || first.Continuation == nil {
		t.Fatalf("foreign first page: %#v err=%v", first, err)
	}
	page, err := c.Elements().ListDirect(actor, root.Anchor.ID, 1, first.Continuation)
	if !errors.Is(err, corecontract.ErrInvalid) || len(page.Records) != 0 ||
		page.Continuation != nil || !page.Cut.Equal(corecontract.CoreReadCut{}) {
		t.Fatalf("foreign continuation: %#v err=%v", page, err)
	}
}

func TestListContinuationBindingIsCheckedAfterParentAuthorization(t *testing.T) {
	authorizer := &recordingAuthorizer{}
	local, err := core.New(realmPolicyContact{authorizer}, elementContact, corecontract.Config{})
	if err != nil {
		t.Fatal(err)
	}
	root := createRoot(t, local, id(38), "binding-local-root")

	foreign := newCore(t)
	foreignRoot := createRoot(t, foreign, root.Anchor.ID, "binding-foreign-root")
	createEntity(t, foreign, id(39), tuple(t, foreignRoot.Anchor.ID, pose(t, 0, 0, 0), 1), "binding-foreign-a")
	createEntity(t, foreign, id(40), tuple(t, foreignRoot.Anchor.ID, pose(t, 0, 0, 0), 1), "binding-foreign-b")
	first, err := foreign.Elements().ListDirect(actor, foreignRoot.Anchor.ID, 1, nil)
	if err != nil || first.Continuation == nil {
		t.Fatalf("foreign continuation fixture: %#v err=%v", first, err)
	}

	authorizer.mu.Lock()
	authorizer.deny = true
	authorizer.calls = nil
	authorizer.mu.Unlock()
	page, err := local.Elements().ListDirect(actor, root.Anchor.ID, 1, first.Continuation)
	if !errors.Is(err, corecontract.ErrDenied) || len(page.Records) != 0 ||
		page.Continuation != nil || !page.Cut.Equal(corecontract.CoreReadCut{}) {
		t.Fatalf("binding leaked before authorization: %#v err=%v", page, err)
	}
	authorizer.mu.Lock()
	calls := append([][]values.Requirement(nil), authorizer.calls...)
	authorizer.deny = false
	authorizer.mu.Unlock()
	if len(calls) != 1 || !reflect.DeepEqual(requirementTargets(local.AuthorityRef(), calls[0]), []values.ElementID{root.Anchor.ID}) {
		t.Fatalf("parent authorization requirements: %#v", calls)
	}
	if _, err := local.Elements().ListDirect(actor, root.Anchor.ID, 1, first.Continuation); !errors.Is(err, corecontract.ErrInvalid) {
		t.Fatalf("authorized foreign continuation: got %v, want ErrInvalid", err)
	}
}

func TestConcurrentMutationExpiresOldPaginationAndRestartIsComplete(t *testing.T) {
	c := newCore(t)
	root := createRoot(t, c, id(80), "paging-root")
	for offset := 1; offset <= 4; offset++ {
		createEntity(
			t, c, id(uint64(80+offset)),
			tuple(t, root.Anchor.ID, pose(t, float64(offset), 0, 0), 1),
			corecontract.OperationID(fmt.Sprintf("paging-%d", offset)),
		)
	}
	first, err := c.Elements().ListDirect(actor, root.Anchor.ID, 1, nil)
	if err != nil || first.Continuation == nil {
		t.Fatalf("first page: %#v err=%v", first, err)
	}

	type pageOutcome struct {
		page corecontract.DirectPage
		err  error
	}
	start := make(chan struct{})
	mutationDone := make(chan error, 1)
	pageDone := make(chan pageOutcome, 1)
	concurrentFinal := tuple(t, root.Anchor.ID, pose(t, 5, 0, 0), 1)
	go func() {
		<-start
		_, mutationErr := c.Elements().CreateEntity(
			actor, id(85), concurrentFinal, "paging-concurrent")

		mutationDone <- mutationErr
	}()
	go func() {
		<-start
		page, pageErr := c.Elements().ListDirect(actor, root.Anchor.ID, 1, first.Continuation)
		pageDone <- pageOutcome{page: page, err: pageErr}
	}()
	close(start)
	if mutationErr := <-mutationDone; mutationErr != nil {
		t.Fatal(mutationErr)
	}
	concurrent := <-pageDone
	if concurrent.err == nil {
		if concurrent.page.Continuation == nil || !concurrent.page.Cut.Equal(first.Cut) {
			t.Fatalf("successful concurrent page changed cut: %#v", concurrent.page)
		}
		expired, expiredErr := c.Elements().ListDirect(actor, root.Anchor.ID, 1, concurrent.page.Continuation)
		if !errors.Is(expiredErr, corecontract.ErrReadCutExpired) || len(expired.Records) != 0 {
			t.Fatalf("old continuation survived committed mutation: %#v err=%v", expired, expiredErr)
		}
	} else if !errors.Is(concurrent.err, corecontract.ErrReadCutExpired) || len(concurrent.page.Records) != 0 {
		t.Fatalf("concurrent page returned mixed outcome: %#v err=%v", concurrent.page, concurrent.err)
	}

	var (
		continuation *corecontract.ListContinuation
		restartCut   corecontract.CoreReadCut
		gotIDs       []values.ElementID
	)
	for {
		page, pageErr := c.Elements().ListDirect(actor, root.Anchor.ID, 2, continuation)
		if pageErr != nil {
			t.Fatalf("restart page: %#v err=%v", page, pageErr)
		}
		if restartCut.Equal(corecontract.CoreReadCut{}) {
			restartCut = page.Cut
		} else if !page.Cut.Equal(restartCut) {
			t.Fatal("restarted pagination mixed cuts")
		}
		for _, record := range page.Records {
			elementID := recordID(record)
			gotIDs = append(gotIDs, elementID)
			read, readErr := c.Elements().ReadElement(actor, elementID, &page.Cut)
			if readErr != nil || read.State != corecontract.ExistencePresent || read.Record == nil {
				t.Fatalf("same-cut read %q: %#v err=%v", elementID, read, readErr)
			}
			projection, projectionErr := c.Elements().ProjectToAncestor(actor, elementID, root.Anchor.ID, &page.Cut)
			if projectionErr != nil || !projection.Cut.Equal(page.Cut) {
				t.Fatalf("same-cut projection %q: %#v err=%v", elementID, projection, projectionErr)
			}
		}
		continuation = page.Continuation
		if continuation == nil {
			break
		}
	}
	wantIDs := []values.ElementID{id(81), id(82), id(83), id(84), id(85)}
	if !reflect.DeepEqual(gotIDs, wantIDs) {
		t.Fatalf("restarted complete UUID order: got=%v want=%v", gotIDs, wantIDs)
	}
}

func TestProjectToAncestorUsesSE3AndCut(t *testing.T) {
	c := newCore(t)
	root := createRoot(t, c, id(40), "root")
	quarter, err := corecontract.NewRotation(1, 0, 0, 1)
	if err != nil {
		t.Fatal(err)
	}
	anchorPose, err := corecontract.NewPose(corecontract.Vec3{Y: 2}, quarter)
	if err != nil {
		t.Fatal(err)
	}
	anchor := createAnchor(t, c, id(41), root.Anchor.ID, tupleWith(t, root.Anchor.ID, anchorPose, 10), "anchor")
	entity := createEntity(t, c, id(42), tuple(t, anchor.Anchor.ID, pose(t, 1, 0, 0), 1), "entity")
	projection, err := c.Elements().ProjectToAncestor(actor, entity.Entity.ID, root.Anchor.ID, nil)
	if err != nil {
		t.Fatal(err)
	}
	if projection.Pose.Translation != (corecontract.Vec3{Y: 3}) || projection.Pose.Rotation.Pivot() != corecontract.PivotW || projection.Pose.Rotation.Ratios() != [3]float64{0, 0, 1} {
		t.Fatalf("projection %#v", projection)
	}
	if !projection.SubjectRevision.Equal(entity.Entity.Revision) {
		t.Fatal("projection revision mismatch")
	}
	read, err := c.Elements().ReadElement(actor, entity.Entity.ID, &projection.Cut)
	if err != nil || read.Record == nil {
		t.Fatalf("same-cut read: %#v %v", read, err)
	}
	createEntity(t, c, id(43), tuple(t, root.Anchor.ID, pose(t, 0, 0, 0), 1), "mutate")
	if _, err = c.Elements().ReadElement(actor, entity.Entity.ID, &projection.Cut); !errors.Is(err, corecontract.ErrReadCutExpired) {
		t.Fatalf("old cut error %v", err)
	}
	expiredProjection, err := c.Elements().ProjectToAncestor(actor, entity.Entity.ID, root.Anchor.ID, &projection.Cut)
	if !errors.Is(err, corecontract.ErrReadCutExpired) || expiredProjection != (corecontract.Projection{}) {
		t.Fatalf("old cut projection: %#v err=%v", expiredProjection, err)
	}
	if _, err = c.Elements().ProjectToAncestor(actor, entity.Entity.ID, id(43), nil); !errors.Is(err, corecontract.ErrConflict) {
		t.Fatalf("entity ancestor error %v", err)
	}
}

func TestAuthorizationPrecedesDisclosure(t *testing.T) {
	a := &recordingAuthorizer{}
	c, err := core.New(realmPolicyContact{a}, elementContact, corecontract.Config{})
	if err != nil {
		t.Fatal(err)
	}
	root := createRoot(t, c, id(50), "root")
	a.mu.Lock()
	a.deny = true
	a.mu.Unlock()
	if _, err = c.Elements().ReadElement(actor, root.Anchor.ID, nil); !errors.Is(err, corecontract.ErrDenied) {
		t.Fatalf("denied read %v", err)
	}
	if _, err = c.Elements().ReadElement(actor, id(999), nil); !errors.Is(err, corecontract.ErrDenied) {
		t.Fatalf("denied absence leaked %v", err)
	}
}

func TestInvalidUUIDIsRejectedBeforeAuthorization(t *testing.T) {
	a := &recordingAuthorizer{}
	c, err := core.New(realmPolicyContact{a}, elementContact, corecontract.Config{})
	if err != nil {
		t.Fatal(err)
	}
	invalid := values.ElementID("not-a-uuid")
	created, err := c.Elements().CreateRoot(actor, invalid, "invalid-root")
	if !errors.Is(err, corecontract.ErrInvalid) || created != (corecontract.MutationResult{}) {
		t.Fatalf("invalid CreateRoot: %#v err=%v", created, err)
	}
	read, err := c.Elements().ReadElement(actor, invalid, nil)
	if !errors.Is(err, corecontract.ErrInvalid) || read != (corecontract.ElementRead{}) {
		t.Fatalf("invalid ReadElement: %#v err=%v", read, err)
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if len(a.calls) != 0 {
		t.Fatalf("invalid UUID reached Authorizer: %#v", a.calls)
	}
}

func TestUpdateRequiresAllPermissionsWithoutBindingOrSideEffect(t *testing.T) {
	authorizer := &selectiveAuthorizer{}
	c, err := core.New(realmPolicyContact{authorizer}, elementContact, corecontract.Config{})
	if err != nil {
		t.Fatal(err)
	}
	root := createRoot(t, c, id(55), "all-of-root")
	entity := createEntity(t, c, id(56), tuple(t, root.Anchor.ID, pose(t, 0, 0, 0), 1), "all-of-entity")
	before, err := c.Elements().ReadElement(actor, entity.Entity.ID, nil)
	if err != nil || before.Record == nil {
		t.Fatalf("before denied update: %#v err=%v", before, err)
	}
	final := tuple(t, root.Anchor.ID, pose(t, 1, 0, 0), 1)

	authorizer.denied = corecontract.PermissionPlaceElement
	denied, err := c.Elements().UpdateEntity(actor, entity.Entity.ID, final, entity.Entity.Revision, "all-of-update")
	if !errors.Is(err, corecontract.ErrDenied) || denied != (corecontract.MutationResult{}) {
		t.Fatalf("missing placement permission: %#v err=%v", denied, err)
	}
	authorizer.denied = ""
	after, err := c.Elements().ReadElement(actor, entity.Entity.ID, nil)
	if err != nil || after.Record == nil || after.Record.Entity == nil ||
		before.Record.Entity == nil || !after.Cut.Equal(before.Cut) ||
		!after.Record.Entity.Revision.Equal(before.Record.Entity.Revision) ||
		after.Record.Entity.LocalPose.Translation != before.Record.Entity.LocalPose.Translation {
		t.Fatalf("denied update changed facts: before=%#v after=%#v err=%v", before, after, err)
	}
	committed, err := c.Elements().UpdateEntity(actor, entity.Entity.ID, final, entity.Entity.Revision, "all-of-update")
	if err != nil || committed.Status != corecontract.MutationCommitted || committed.Record == nil {
		t.Fatalf("denied OperationID was retained: %#v err=%v", committed, err)
	}
}

func TestProjectionAuthorizesInputsThenResolvedPath(t *testing.T) {
	a := &recordingAuthorizer{}
	c, err := core.New(realmPolicyContact{a}, elementContact, corecontract.Config{})
	if err != nil {
		t.Fatal(err)
	}
	root := createRoot(t, c, id(60), "root")
	middle := createAnchor(t, c, id(61), root.Anchor.ID, tuple(t, root.Anchor.ID, pose(t, 0, 0, 0), 10), "middle")
	entity := createEntity(t, c, id(62), tuple(t, middle.Anchor.ID, pose(t, 0, 0, 0), 1), "entity")
	a.mu.Lock()
	a.calls = nil
	a.mu.Unlock()

	if _, err := c.Elements().ProjectToAncestor(actor, entity.Entity.ID, root.Anchor.ID, nil); err != nil {
		t.Fatal(err)
	}
	a.mu.Lock()
	calls := append([][]values.Requirement(nil), a.calls...)
	a.mu.Unlock()
	if len(calls) != 2 {
		t.Fatalf("authorization calls: got %d, want 2", len(calls))
	}
	if got := requirementTargets(c.AuthorityRef(), calls[0]); !reflect.DeepEqual(got, []values.ElementID{entity.Entity.ID, root.Anchor.ID}) {
		t.Fatalf("input authorization targets: %#v", got)
	}
	if got := requirementTargets(c.AuthorityRef(), calls[1]); !reflect.DeepEqual(got, []values.ElementID{entity.Entity.ID, middle.Anchor.ID, root.Anchor.ID}) {
		t.Fatalf("path authorization targets: %#v", got)
	}

	sibling := createAnchor(t, c, id(63), root.Anchor.ID, tuple(t, root.Anchor.ID, pose(t, 0, 0, 0), 10), "sibling")
	a.mu.Lock()
	a.calls = nil
	a.mu.Unlock()
	nonAncestor, err := c.Elements().ProjectToAncestor(actor, entity.Entity.ID, sibling.Anchor.ID, nil)
	if !errors.Is(err, corecontract.ErrConflict) || nonAncestor != (corecontract.Projection{}) {
		t.Fatalf("valid non-ancestor Anchor: %#v err=%v", nonAncestor, err)
	}
	a.mu.Lock()
	calls = append([][]values.Requirement(nil), a.calls...)
	a.calls = nil
	a.denyAt = 2
	a.mu.Unlock()
	if len(calls) != 2 || !reflect.DeepEqual(
		requirementTargets(c.AuthorityRef(), calls[1]),
		[]values.ElementID{entity.Entity.ID, middle.Anchor.ID, root.Anchor.ID, sibling.Anchor.ID},
	) {
		t.Fatalf("non-ancestor proof path authorization: %#v", calls)
	}
	denied, err := c.Elements().ProjectToAncestor(actor, entity.Entity.ID, sibling.Anchor.ID, nil)
	if !errors.Is(err, corecontract.ErrDenied) || denied != (corecontract.Projection{}) {
		t.Fatalf("non-ancestor path leaked through denied scope: %#v err=%v", denied, err)
	}
	a.mu.Lock()
	a.denyAt = 0
	a.mu.Unlock()
}

func TestBlockedProjectionAuthorizerDoesNotHoldCoreLock(t *testing.T) {
	a := &blockingAuthorizer{}
	c, err := core.New(realmPolicyContact{a}, elementContact, corecontract.Config{})
	if err != nil {
		t.Fatal(err)
	}
	root := createRoot(t, c, id(70), "root")
	middle := createAnchor(t, c, id(71), root.Anchor.ID, tuple(t, root.Anchor.ID, pose(t, 0, 0, 0), 10), "middle")
	entity := createEntity(t, c, id(72), tuple(t, middle.Anchor.ID, pose(t, 0, 0, 0), 1), "entity")

	a.mu.Lock()
	a.calls = 0
	a.blockAt = 2
	a.entered = make(chan struct{})
	a.release = make(chan struct{})
	entered, release := a.entered, a.release
	a.mu.Unlock()

	result := make(chan error, 1)
	go func() {
		_, projectionErr := c.Elements().ProjectToAncestor(actor, entity.Entity.ID, root.Anchor.ID, nil)
		result <- projectionErr
	}()
	<-entered
	if _, err := c.Elements().CreateEntity(actor, id(73), tuple(t, root.Anchor.ID, pose(t, 0, 0, 0), 1), "concurrent"); err != nil {
		t.Fatalf("concurrent mutation was blocked: %v", err)
	}
	close(release)
	if err := <-result; !errors.Is(err, corecontract.ErrReadCutExpired) {
		t.Fatalf("projection after concurrent mutation: got %v, want ErrReadCutExpired", err)
	}
}

func TestProjectionExactEdgeLimitAndFiniteOutput(t *testing.T) {
	const maxExactProjectionEdges = 1024
	c := newCore(t)
	root := createRoot(t, c, id(1000), "deep-root")
	parent := root.Anchor.ID
	var atLimit values.ElementID
	for edge := 1; edge <= maxExactProjectionEdges+1; edge++ {
		childID := id(uint64(1000 + edge))
		child := createAnchor(
			t, c, childID, parent,
			tuple(t, parent, pose(t, 0, 0, 0), 1),
			corecontract.OperationID(fmt.Sprintf("deep-%d", edge)),
		)
		parent = child.Anchor.ID
		if edge == maxExactProjectionEdges {
			atLimit = parent
		}
	}
	if _, err := c.Elements().ProjectToAncestor(actor, atLimit, root.Anchor.ID, nil); err != nil {
		t.Fatalf("projection at edge limit: %v", err)
	}
	if result, err := c.Elements().ProjectToAncestor(actor, parent, root.Anchor.ID, nil); !errors.Is(err, corecontract.ErrUnrepresentable) || result != (corecontract.Projection{}) {
		t.Fatalf("projection over edge limit: %#v err=%v", result, err)
	}
	deepest, err := c.Elements().ReadElement(actor, parent, nil)
	if err != nil || deepest.State != corecontract.ExistencePresent || deepest.Record == nil {
		t.Fatalf("over-limit projection truncated deepest truth: %#v err=%v", deepest, err)
	}
	deepestChildren, err := c.Elements().ListDirect(actor, parent, 1, nil)
	if err != nil || len(deepestChildren.Records) != 0 || deepestChildren.Continuation != nil {
		t.Fatalf("over-limit projection damaged Structure: %#v err=%v", deepestChildren, err)
	}

	overflowCore := newCore(t)
	overflowRoot := createRoot(t, overflowCore, id(3000), "overflow-root")
	outer := createAnchor(
		t, overflowCore, id(3001), overflowRoot.Anchor.ID,
		tuple(t, overflowRoot.Anchor.ID, pose(t, math.MaxFloat64, 0, 0), math.MaxFloat64),
		"overflow-outer",
	)
	inner := createAnchor(
		t, overflowCore, id(3002), outer.Anchor.ID,
		tuple(t, outer.Anchor.ID, pose(t, math.MaxFloat64, 0, 0), 0),
		"overflow-inner",
	)
	if result, err := overflowCore.Elements().ProjectToAncestor(actor, inner.Anchor.ID, overflowRoot.Anchor.ID, nil); !errors.Is(err, corecontract.ErrUnrepresentable) || result != (corecontract.Projection{}) {
		t.Fatalf("unrepresentable finite truth: %#v err=%v", result, err)
	}
	read, err := overflowCore.Elements().ReadElement(actor, inner.Anchor.ID, nil)
	if err != nil || read.State != corecontract.ExistencePresent {
		t.Fatalf("unrepresentable projection damaged truth: %#v err=%v", read, err)
	}
}
