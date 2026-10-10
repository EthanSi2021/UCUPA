package element_test

import (
	"fmt"
	"math"
	"reflect"
	"sync"
	"testing"

	"ucupa/shared/values"
	corecontract "ucupa/ucupa-core/contract"
	core "ucupa/ucupa-core/instance"
)

func TestAnchorRangeUpdateEmptyCanShrinkToZero(t *testing.T) {
	c := newCore(t)
	root := createRoot(t, c, id(4000), "root")
	parent := createAnchor(t, c, id(4001), root.Anchor.ID, tuple(t, root.Anchor.ID, pose(t, 0, 0, 0), 20), "parent")
	subject := createAnchor(t, c, id(4002), parent.Anchor.ID, tuple(t, parent.Anchor.ID, pose(t, 7, 0, 0), 10), "subject")
	before := anchorRangeRead(t, c, subject.Anchor.ID, nil)
	final := tuple(t, parent.Anchor.ID, *subject.Anchor.LocalPose, 0)

	result, err := c.Elements().UpdateAnchor(actor, subject.Anchor.ID, final, subject.Anchor.Revision, "shrink-zero")
	assertAnchorRangeCommitted(t, result, err, subject, final)
	if result.OutcomeCut.Equal(before.Cut) {
		t.Fatal("successful shrink did not advance the fact cut")
	}
	assertAnchorRangeRecordsAt(t, c, result.OutcomeCut, root, parent, *result.Record)
	stale, err := c.Elements().UpdateAnchor(actor, subject.Anchor.ID, tuple(t, parent.Anchor.ID, *subject.Anchor.LocalPose, 1), subject.Anchor.Revision, "stale-after-shrink")
	assertRejected(t, stale, err, corecontract.ErrRevisionConflict)
	if !stale.OutcomeCut.Equal(result.OutcomeCut) {
		t.Fatal("stale update advanced the fact cut")
	}
	assertAnchorRangeRecordsAt(t, c, result.OutcomeCut, root, parent, *result.Record)
}

func TestAnchorRangeUpdateTangentChildrenPreservesOtherFacts(t *testing.T) {
	c := newCore(t)
	root := createRoot(t, c, id(4010), "root")
	parent := createAnchor(t, c, id(4011), root.Anchor.ID, tuple(t, root.Anchor.ID, pose(t, 0, 0, 0), 50), "parent")
	subject := createAnchor(t, c, id(4012), parent.Anchor.ID, tuple(t, parent.Anchor.ID, pose(t, 15, 0, 0), 20), "subject")
	childAnchor := createAnchor(t, c, id(4013), subject.Anchor.ID, tuple(t, subject.Anchor.ID, pose(t, 3, 0, 0), 2), "child-anchor")
	childEntity := createEntity(t, c, id(4014), tuple(t, subject.Anchor.ID, pose(t, -4, 0, 0), 1), "child-entity")
	grandchild := createEntity(t, c, id(4015), tuple(t, childAnchor.Anchor.ID, pose(t, 1, 0, 0), 1), "grandchild")
	sibling := createAnchor(t, c, id(4016), root.Anchor.ID, tuple(t, root.Anchor.ID, pose(t, 100, 0, 0), 6), "sibling")
	before := anchorRangeRead(t, c, subject.Anchor.ID, nil)
	final := tuple(t, parent.Anchor.ID, *subject.Anchor.LocalPose, 5)

	result, err := c.Elements().UpdateAnchor(actor, subject.Anchor.ID, final, subject.Anchor.Revision, "shrink-tangent")
	assertAnchorRangeCommitted(t, result, err, subject, final)
	if result.OutcomeCut.Equal(before.Cut) {
		t.Fatal("successful shrink did not advance the fact cut")
	}
	assertAnchorRangeRecordsAt(t, c, result.OutcomeCut, root, parent, childAnchor, childEntity, grandchild, sibling, *result.Record)
}

func TestAnchorRangeUpdateRejectsExcludedDirectChildWithoutFactChange(t *testing.T) {
	for _, excludedKind := range []string{"anchor", "entity"} {
		t.Run(excludedKind, func(t *testing.T) {
			c := newCore(t)
			root := createRoot(t, c, id(4020), "root")
			parent := createAnchor(t, c, id(4021), root.Anchor.ID, tuple(t, root.Anchor.ID, pose(t, 0, 0, 0), 30), "parent")
			subject := createAnchor(t, c, id(4022), parent.Anchor.ID, tuple(t, parent.Anchor.ID, pose(t, 0, 0, 0), 10), "subject")
			anchorX, entityX := 1.0, 1.0
			if excludedKind == "anchor" {
				anchorX = 4
			} else {
				entityX = 4
			}
			childAnchor := createAnchor(t, c, id(4023), subject.Anchor.ID, tuple(t, subject.Anchor.ID, pose(t, anchorX, 0, 0), 2), "child-anchor")
			childEntity := createEntity(t, c, id(4024), tuple(t, subject.Anchor.ID, pose(t, entityX, 0, 0), 2), "child-entity")
			grandchild := createEntity(t, c, id(4025), tuple(t, childAnchor.Anchor.ID, pose(t, 1, 0, 0), 1), "grandchild")
			before := anchorRangeRead(t, c, subject.Anchor.ID, nil)

			result, err := c.Elements().UpdateAnchor(actor, subject.Anchor.ID, tuple(t, parent.Anchor.ID, *subject.Anchor.LocalPose, 5), subject.Anchor.Revision, "exclude-child")
			assertRejected(t, result, err, corecontract.ErrConflict)
			if !result.OutcomeCut.Equal(before.Cut) {
				t.Fatal("rejected shrink advanced the fact cut")
			}
			assertAnchorRangeRecordsAt(t, c, before.Cut, root, parent, subject, childAnchor, childEntity, grandchild)
		})
	}
}

func TestAnchorRangeUpdateExactTangencyRejectsOneULPSmaller(t *testing.T) {
	for _, childKind := range []string{"anchor", "entity"} {
		t.Run(childKind, func(t *testing.T) {
			c := newCore(t)
			root := createRoot(t, c, id(4080), "root")
			parent := createAnchor(t, c, id(4081), root.Anchor.ID, tuple(t, root.Anchor.ID, pose(t, 0, 0, 0), 10), "parent")
			childTuple := tuple(t, parent.Anchor.ID, pose(t, 4, 0, 0), 2)
			var child corecontract.ElementRecord
			if childKind == "anchor" {
				child = createAnchor(t, c, id(4082), parent.Anchor.ID, childTuple, "child")
			} else {
				child = createEntity(t, c, id(4082), childTuple, "child")
			}
			final := tuple(t, root.Anchor.ID, *parent.Anchor.LocalPose, 6)

			tangent, err := c.Elements().UpdateAnchor(actor, parent.Anchor.ID, final, parent.Anchor.Revision, "shrink-tangent")
			assertAnchorRangeCommitted(t, tangent, err, parent, final)
			assertAnchorRangeRecordsAt(t, c, tangent.OutcomeCut, root, child, *tangent.Record)

			final.Range = sphere(t, math.Nextafter(6, 0))
			outside, err := c.Elements().UpdateAnchor(actor, parent.Anchor.ID, final, tangent.Record.Anchor.Revision, "shrink-one-ulp")
			assertRejected(t, outside, err, corecontract.ErrConflict)
			if !outside.OutcomeCut.Equal(tangent.OutcomeCut) {
				t.Fatal("one-ULP containment rejection advanced the fact cut")
			}
			assertAnchorRangeRecordsAt(t, c, tangent.OutcomeCut, root, child, *tangent.Record)
		})
	}
}

func TestAnchorRangeUpdateReparentPoseAndShrinkUsesFinalTuple(t *testing.T) {
	c := newCore(t)
	root := createRoot(t, c, id(4030), "root")
	oldParent := createAnchor(t, c, id(4031), root.Anchor.ID, tuple(t, root.Anchor.ID, pose(t, 0, 0, 0), 20), "old-parent")
	newParent := createAnchor(t, c, id(4032), root.Anchor.ID, tuple(t, root.Anchor.ID, pose(t, 100, 0, 0), 6), "new-parent")
	subject := createAnchor(t, c, id(4033), oldParent.Anchor.ID, tuple(t, oldParent.Anchor.ID, pose(t, 10, 0, 0), 10), "subject")
	childAnchor := createAnchor(t, c, id(4034), subject.Anchor.ID, tuple(t, subject.Anchor.ID, pose(t, 2, 0, 0), 2), "child-anchor")
	childEntity := createEntity(t, c, id(4035), tuple(t, subject.Anchor.ID, pose(t, -3, 0, 0), 1), "child-entity")
	grandchild := createEntity(t, c, id(4036), tuple(t, childAnchor.Anchor.ID, pose(t, 1, 0, 0), 1), "grandchild")
	// Neither the old Pose nor the old Range fits the new parent. The final
	// tuple does, with exact tangency on both the parent and child edges.
	final := tuple(t, newParent.Anchor.ID, pose(t, 2, 0, 0), 4)

	result, err := c.Elements().UpdateAnchor(actor, subject.Anchor.ID, final, subject.Anchor.Revision, "reparent-pose-shrink")
	assertAnchorRangeCommitted(t, result, err, subject, final)
	assertAnchorRangeRecordsAt(t, c, result.OutcomeCut, root, oldParent, newParent, childAnchor, childEntity, grandchild, *result.Record)
	oldChildren, err := c.Elements().ListDirect(actor, oldParent.Anchor.ID, 10, nil)
	if err != nil || len(oldChildren.Records) != 0 || !oldChildren.Cut.Equal(result.OutcomeCut) {
		t.Fatalf("old parent retained subject: %#v err=%v", oldChildren, err)
	}
	newChildren, err := c.Elements().ListDirect(actor, newParent.Anchor.ID, 10, nil)
	if err != nil || len(newChildren.Records) != 1 || !reflect.DeepEqual(newChildren.Records[0], *result.Record) || !newChildren.Cut.Equal(result.OutcomeCut) {
		t.Fatalf("new parent missing final subject: %#v err=%v", newChildren, err)
	}
}

func TestAnchorRangeUpdateShrinkOutsideNewParentIsRejected(t *testing.T) {
	c := newCore(t)
	root := createRoot(t, c, id(4040), "root")
	oldParent := createAnchor(t, c, id(4041), root.Anchor.ID, tuple(t, root.Anchor.ID, pose(t, 0, 0, 0), 20), "old-parent")
	newParent := createAnchor(t, c, id(4042), root.Anchor.ID, tuple(t, root.Anchor.ID, pose(t, 100, 0, 0), 6), "new-parent")
	subject := createAnchor(t, c, id(4043), oldParent.Anchor.ID, tuple(t, oldParent.Anchor.ID, pose(t, 10, 0, 0), 10), "subject")
	childAnchor := createAnchor(t, c, id(4044), subject.Anchor.ID, tuple(t, subject.Anchor.ID, pose(t, 2, 0, 0), 2), "child-anchor")
	childEntity := createEntity(t, c, id(4045), tuple(t, subject.Anchor.ID, pose(t, -3, 0, 0), 1), "child-entity")
	before := anchorRangeRead(t, c, subject.Anchor.ID, nil)

	result, err := c.Elements().UpdateAnchor(actor, subject.Anchor.ID, tuple(t, newParent.Anchor.ID, pose(t, 3, 0, 0), 4), subject.Anchor.Revision, "outside-new-parent")
	assertRejected(t, result, err, corecontract.ErrConflict)
	if !result.OutcomeCut.Equal(before.Cut) {
		t.Fatal("rejected reparent and shrink advanced the fact cut")
	}
	assertAnchorRangeRecordsAt(t, c, before.Cut, root, oldParent, newParent, subject, childAnchor, childEntity)
}

func TestAnchorRangeUpdateSameRadiusExpansionAndUnchanged(t *testing.T) {
	c := newCore(t)
	root := createRoot(t, c, id(4050), "root")
	parent := createAnchor(t, c, id(4051), root.Anchor.ID, tuple(t, root.Anchor.ID, pose(t, 0, 0, 0), 20), "parent")
	subject := createAnchor(t, c, id(4052), parent.Anchor.ID, tuple(t, parent.Anchor.ID, pose(t, 0, 0, 0), 5), "subject")
	child := createEntity(t, c, id(4053), tuple(t, subject.Anchor.ID, pose(t, 4, 0, 0), 1), "child")
	final := tuple(t, parent.Anchor.ID, pose(t, 1, 0, 0), 5)
	moved, err := c.Elements().UpdateAnchor(actor, subject.Anchor.ID, final, subject.Anchor.Revision, "same-radius-move")
	assertAnchorRangeCommitted(t, moved, err, subject, final)
	assertAnchorRangeRecordsAt(t, c, moved.OutcomeCut, root, parent, child, *moved.Record)

	final.Range = sphere(t, 8)
	expanded, err := c.Elements().UpdateAnchor(actor, subject.Anchor.ID, final, moved.Record.Anchor.Revision, "expand")
	assertAnchorRangeCommitted(t, expanded, err, *moved.Record, final)
	assertAnchorRangeRecordsAt(t, c, expanded.OutcomeCut, root, parent, child, *expanded.Record)

	unchanged, err := c.Elements().UpdateAnchor(actor, subject.Anchor.ID, final, expanded.Record.Anchor.Revision, "unchanged")
	if err != nil || unchanged.Status != corecontract.MutationUnchanged || !unchanged.OutcomeCut.Equal(expanded.OutcomeCut) || !reflect.DeepEqual(unchanged.Record, expanded.Record) {
		t.Fatalf("identical tuple changed facts: %#v err=%v", unchanged, err)
	}
	assertAnchorRangeRecordsAt(t, c, expanded.OutcomeCut, root, parent, child, *expanded.Record)

	inward, err := c.Elements().UpdateEntity(actor, child.Entity.ID, tuple(t, subject.Anchor.ID, pose(t, 1, 0, 0), 1), child.Entity.Revision, "move-child-inward")
	if err != nil || inward.Status != corecontract.MutationCommitted || inward.Record == nil || inward.Record.Entity == nil {
		t.Fatalf("move child inward: %#v err=%v", inward, err)
	}
	assertAnchorRangeRecordsAt(t, c, inward.OutcomeCut, root, parent, *expanded.Record, *inward.Record)
	deleted, err := c.Elements().DeleteEntity(actor, child.Entity.ID, inward.Record.Entity.Revision, "delete-child")
	if err != nil || deleted.Status != corecontract.MutationCommitted || deleted.Record != nil {
		t.Fatalf("delete child: %#v err=%v", deleted, err)
	}
	assertAnchorRangeRecordsAt(t, c, deleted.OutcomeCut, root, parent, *expanded.Record)
}

func TestAnchorRangeUpdateRootRemainsImmutable(t *testing.T) {
	c := newCore(t)
	root := createRoot(t, c, id(4060), "root")
	before := anchorRangeRead(t, c, root.Anchor.ID, nil)

	updated, err := c.Elements().UpdateAnchor(actor, root.Anchor.ID, tuple(t, root.Anchor.ID, pose(t, 0, 0, 0), 0), root.Anchor.Revision, "shrink-root")
	assertRejected(t, updated, err, corecontract.ErrConflict)
	if !updated.OutcomeCut.Equal(before.Cut) {
		t.Fatal("rejected root update advanced the fact cut")
	}
	assertAnchorRangeRecordsAt(t, c, before.Cut, root)
	deleted, err := c.Elements().DeleteAnchor(actor, root.Anchor.ID, root.Anchor.Revision, "delete-root")
	assertRejected(t, deleted, err, corecontract.ErrConflict)
	if !deleted.OutcomeCut.Equal(before.Cut) {
		t.Fatal("rejected root delete advanced the fact cut")
	}
	assertAnchorRangeRecordsAt(t, c, before.Cut, root)
}

func TestAnchorRangeUpdateConcurrentShrinkAndChildMoveHaveOneWinner(t *testing.T) {
	for _, childKind := range []string{"anchor", "entity"} {
		t.Run(childKind, func(t *testing.T) {
			for iteration := 0; iteration < 32; iteration++ {
				t.Run(fmt.Sprint(iteration), func(t *testing.T) {
					c := newCore(t)
					root := createRoot(t, c, id(4070), "root")
					parent := createAnchor(t, c, id(4071), root.Anchor.ID, tuple(t, root.Anchor.ID, pose(t, 0, 0, 0), 10), "parent")
					initialChild := tuple(t, parent.Anchor.ID, pose(t, 0, 0, 0), 1)
					var child corecontract.ElementRecord
					var childRevision corecontract.Revision
					if childKind == "anchor" {
						child = createAnchor(t, c, id(4072), parent.Anchor.ID, initialChild, "child")
						childRevision = child.Anchor.Revision
					} else {
						child = createEntity(t, c, id(4072), initialChild, "child")
						childRevision = child.Entity.Revision
					}
					shrink := tuple(t, root.Anchor.ID, *parent.Anchor.LocalPose, 5)
					move := tuple(t, parent.Anchor.ID, pose(t, 8, 0, 0), 1)
					type outcome struct {
						result corecontract.MutationResult
						err    error
					}
					results := make(chan outcome, 2)
					start := make(chan struct{})
					var ready sync.WaitGroup
					ready.Add(2)
					go func() {
						ready.Done()
						<-start
						result, err := c.Elements().UpdateAnchor(actor, parent.Anchor.ID, shrink, parent.Anchor.Revision, "shrink")
						results <- outcome{result, err}
					}()
					go func() {
						ready.Done()
						<-start
						var result corecontract.MutationResult
						var err error
						if childKind == "anchor" {
							result, err = c.Elements().UpdateAnchor(actor, recordID(child), move, childRevision, "move")
						} else {
							result, err = c.Elements().UpdateEntity(actor, recordID(child), move, childRevision, "move")
						}
						results <- outcome{result, err}
					}()
					ready.Wait()
					close(start)
					outcomes := []outcome{<-results, <-results}
					var winner, loser corecontract.MutationResult
					successes := 0
					for _, got := range outcomes {
						if got.err == nil {
							if got.result.Status != corecontract.MutationCommitted || got.result.Record == nil {
								t.Fatalf("unexpected successful outcome: %#v", got.result)
							}
							successes++
							winner = got.result
						} else {
							assertRejected(t, got.result, got.err, corecontract.ErrConflict)
							loser = got.result
						}
					}
					if successes != 1 || !loser.OutcomeCut.Equal(winner.OutcomeCut) {
						t.Fatalf("mutually incompatible writes: successes=%d outcomes=%#v", successes, outcomes)
					}
					parentAfter := anchorRangeRead(t, c, parent.Anchor.ID, &winner.OutcomeCut)
					childAfter := anchorRangeRead(t, c, recordID(child), &winner.OutcomeCut)
					parentSphere, _ := parentAfter.Record.Anchor.Range.BoundedSphere()
					var childPose corecontract.Pose
					var childSphere corecontract.BoundedSphere
					if childKind == "anchor" {
						childPose = *childAfter.Record.Anchor.LocalPose
						childSphere, _ = childAfter.Record.Anchor.Range.BoundedSphere()
					} else {
						childPose = childAfter.Record.Entity.LocalPose
						childSphere = childAfter.Record.Entity.Range
					}
					if math.Abs(childPose.Translation.X)+childSphere.Radius() > parentSphere.Radius() {
						t.Fatalf("committed state lost containment: parent=%#v child=%#v", parentAfter.Record, childAfter.Record)
					}
					if recordID(*winner.Record) == parent.Anchor.ID {
						assertAnchorRangeRecordsAt(t, c, winner.OutcomeCut, root, child, *winner.Record)
						if parentSphere.Radius() != 5 || childPose.Translation.X != 0 {
							t.Fatal("winning shrink did not preserve the child")
						}
					} else {
						assertAnchorRangeRecordsAt(t, c, winner.OutcomeCut, root, parent, *winner.Record)
						if parentSphere.Radius() != 10 || childPose.Translation.X != 8 {
							t.Fatal("winning move did not preserve the parent")
						}
					}
				})
			}
		})
	}
}

func anchorRangeRead(t *testing.T, c *core.Core, elementID values.ElementID, at *corecontract.CoreReadCut) corecontract.ElementRead {
	t.Helper()
	read, err := c.Elements().ReadElement(actor, elementID, at)
	if err != nil || read.State != corecontract.ExistencePresent || read.Record == nil {
		t.Fatalf("read %q: %#v err=%v", elementID, read, err)
	}
	return read
}

func assertAnchorRangeRecordsAt(t *testing.T, c *core.Core, cut corecontract.CoreReadCut, records ...corecontract.ElementRecord) {
	t.Helper()
	for _, record := range records {
		read := anchorRangeRead(t, c, recordID(record), &cut)
		if !read.Cut.Equal(cut) || !reflect.DeepEqual(read.Record, &record) {
			t.Fatalf("record or revision changed at cut: want=%#v got=%#v", record, read)
		}
	}
}

func assertAnchorRangeCommitted(t *testing.T, result corecontract.MutationResult, err error, previous corecontract.ElementRecord, final corecontract.SpatialTuple) {
	t.Helper()
	if err != nil || result.Status != corecontract.MutationCommitted || result.Record == nil || result.Record.Kind != corecontract.ElementAnchor || result.Record.Anchor == nil || result.Record.Entity != nil {
		t.Fatalf("Anchor update: %#v err=%v", result, err)
	}
	anchor := result.Record.Anchor
	rangeValue, bounded := anchor.Range.BoundedSphere()
	if anchor.ID != previous.Anchor.ID || anchor.ParentAnchorID == nil || *anchor.ParentAnchorID != final.Parent || anchor.LocalPose == nil || *anchor.LocalPose != final.LocalPose || !bounded || rangeValue != final.Range || anchor.Revision.Equal(previous.Anchor.Revision) || result.OutcomeCut.Equal(corecontract.CoreReadCut{}) {
		t.Fatalf("committed Anchor differs from final tuple or retained revision: result=%#v final=%#v", result, final)
	}
}
