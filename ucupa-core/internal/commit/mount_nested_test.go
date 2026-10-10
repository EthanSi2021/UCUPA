package commit_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	corecontract "ucupa/ucupa-core/contract"
	"ucupa/ucupa-core/internal/commit"
	mountfact "ucupa/ucupa-core/internal/mount/fact"
)

func TestNestedMountForestAndDurableReopen(t *testing.T) {
	for _, durable := range []bool{false, true} {
		t.Run(fmt.Sprintf("durable=%v", durable), func(t *testing.T) {
			f := newNestedMountFixture(t, durable)
			m, n, o, sibling := rangeStoreID(9701), rangeStoreID(9702), rangeStoreID(9703), rangeStoreID(9704)
			f.establish(t, 0, 1, f.anchor(t, 0, m, f.root, 10), "a-b")
			f.establish(t, 1, 2, f.anchor(t, 1, n, m, 5), "b-c")
			f.establish(t, 2, 4, f.anchor(t, 2, o, n, 2), "c-e")
			f.establish(t, 0, 3, f.anchor(t, 0, sibling, f.root, 10), "a-d")
			if _, err := f.owners[4].elements.CreateEntity(rangeStoreActor, rangeStoreID(9705), rangeStoreTuple(t, o, 0, 1), "deep-entity"); err != nil {
				t.Fatal(err)
			}
			// A successful deep child mutation does not rewrite any ancestor.
			if mountStoreFacts(f.owners[0]).Mount.Record(m).State != corecontract.MountActive || mountStoreFacts(f.owners[0]).Mount.Record(sibling).State != corecontract.MountActive {
				t.Fatal("siblings were not independently retained")
			}
			f.validate(t)
			if !durable {
				return
			}
			f.reopen(t)
		})
	}
}

func TestNestedMountRejectsCyclesAndSecondParent(t *testing.T) {
	f := newNestedMountFixture(t, false)
	m, n := rangeStoreID(9711), rangeStoreID(9712)
	f.establish(t, 0, 1, f.anchor(t, 0, m, f.root, 10), "a-b")
	f.establish(t, 1, 2, f.anchor(t, 1, n, m, 5), "b-c")
	for _, target := range []int{0, 1, 2} {
		command := f.anchor(t, 2, rangeStoreID(uint64(9720+target)), n, 1)
		command.CarrierAuthority = f.owners[target].owner.AuthorityRef()
		command.CarrierNextHop, command.ParentNextHop = nestedMountAddress(target), nestedMountAddress(2)
		result, err := f.owners[2].mounts.ApplyMount(context.Background(), f.callers, corecontract.OperationID(fmt.Sprintf("cycle-%d", target)), command)
		if err != nil || result.Decision != corecontract.MountAborted || result.AbortCause != corecontract.MountAbortConflict || !result.Settled {
			t.Fatalf("attached existing/cyclic carrier %d: %#v %v", target, result, err)
		}
	}
	f.validate(t)
}

func TestNestedMountAncestorPrepareBlocksPendingChildCommit(t *testing.T) {
	f := newNestedMountFixture(t, true)
	m, n := rangeStoreID(9731), rangeStoreID(9732)
	f.establish(t, 0, 1, f.anchor(t, 0, m, f.root, 10), "a-b")
	f.establish(t, 1, 2, f.anchor(t, 1, n, m, 5), "b-c")
	a, b, c := f.owners[0], f.owners[1], f.owners[2]
	child, ancestor := f.update(t, 1, n, 8), f.update(t, 0, m, 6)
	f.transports[1].afterPrepare = func() {
		f.transports[1].afterPrepare = nil
		// C has durably prepared N=8 but B has not committed it. A's M=6
		// correctly fits N's current envelope 5 and closes B before deciding.
		if err := reserveNestedMount(a, f.callers, "shrink-ancestor", ancestor); err != nil {
			t.Fatal(err)
		}
		request := mountStoreOperation(t, a, "shrink-ancestor").Request
		prepared, err := b.mounts.MountPeer().Prepare(context.Background(), a.owner.AuthorityRef(), request)
		if err != nil || !prepared.Prepared {
			t.Fatalf("ancestor prepare: %#v %v", prepared, err)
		}
	}
	_, err := b.mounts.ApplyMount(context.Background(), f.callers, "grow-child", child)
	if !errors.Is(err, corecontract.ErrMountBusy) || mountStoreOperation(t, b, "grow-child").Decision != corecontract.MountPending || mountStoreFacts(c).Mount.Incoming().Gate != mountfact.GatePreparing {
		t.Fatalf("child committed through closed ancestor gate: %v", err)
	}
	before := commit.CaptureCompleteStateForTest(b.owner)
	if _, err := b.mounts.ApplyMount(context.Background(), f.callers, "new-child-control", child); !errors.Is(err, corecontract.ErrMountBusy) || !commit.MatchesCompleteStateForTest(b.owner, before) {
		t.Fatalf("closed gate reserved new control: %v", err)
	}
	// Both levels survive reopening while A's ancestor intent and B's child
	// intent are pending. Reopening does not silently decide either operation.
	f.reopen(t)
	a, b, c = f.owners[0], f.owners[1], f.owners[2]
	if mountStoreFacts(b).Mount.Incoming().Gate != mountfact.GatePreparing || mountStoreFacts(c).Mount.Incoming().Gate != mountfact.GatePreparing || mountStoreOperation(t, b, "grow-child").Decision != corecontract.MountPending {
		t.Fatal("reopen lost the nested pending operations or write gates")
	}
	result, err := a.mounts.ApplyMount(context.Background(), f.callers, "shrink-ancestor", ancestor)
	if err != nil || result.Decision != corecontract.MountCommitted || !result.Settled {
		t.Fatalf("finish ancestor: %#v %v", result, err)
	}
	result, err = b.mounts.ApplyMount(context.Background(), f.callers, "grow-child", child)
	if err != nil || result.Decision != corecontract.MountAborted || result.AbortCause != corecontract.MountAbortConflict || !result.Settled {
		t.Fatalf("child did not recheck new ancestor envelope: %#v %v", result, err)
	}
	if mountStoreFacts(c).Mount.Incoming().Gate != mountfact.GateOpen || mountStoreFacts(c).Mount.Incoming().Admission.RangeValue.Radius() != 5 {
		t.Fatal("child abort did not restore the original admitted envelope")
	}
	f.validate(t)
}

func TestNestedMountCommittedChildPreventsAncestorShrink(t *testing.T) {
	f := newNestedMountFixture(t, false)
	m, n := rangeStoreID(9801), rangeStoreID(9802)
	f.establish(t, 0, 1, f.anchor(t, 0, m, f.root, 10), "a-b")
	f.establish(t, 1, 2, f.anchor(t, 1, n, m, 5), "b-c")
	result, err := f.owners[1].mounts.ApplyMount(context.Background(), f.callers, "grow-child", f.update(t, 1, n, 8))
	if err != nil || result.Decision != corecontract.MountCommitted || !result.Settled {
		t.Fatalf("grow child: %#v %v", result, err)
	}
	result, err = f.owners[0].mounts.ApplyMount(context.Background(), f.callers, "shrink-ancestor", f.update(t, 0, m, 6))
	if err != nil || result.Decision != corecontract.MountAborted || result.AbortCause != corecontract.MountAbortConflict || !result.Settled {
		t.Fatalf("ancestor ignored committed descendant envelope: %#v %v", result, err)
	}
	if mountStoreFacts(f.owners[1]).Mount.Incoming().Gate != mountfact.GateOpen || mountStoreFacts(f.owners[1]).Mount.Incoming().Admission.RangeValue.Radius() != 10 {
		t.Fatal("rejected ancestor shrink changed B's admission")
	}
	f.validate(t)
}

func TestNestedMountFrozenEnvelopeContainsContinuingDescendantWrites(t *testing.T) {
	f := newNestedMountFixture(t, false)
	m, n, entity := rangeStoreID(9831), rangeStoreID(9832), rangeStoreID(9833)
	f.establish(t, 0, 1, f.anchor(t, 0, m, f.root, 10), "a-b")
	f.establish(t, 1, 2, f.anchor(t, 1, n, m, 5), "b-c")
	a, b, c := f.owners[0], f.owners[1], f.owners[2]
	ancestor := f.update(t, 0, m, 6)
	if err := reserveNestedMount(a, f.callers, "ancestor", ancestor); err != nil {
		t.Fatal(err)
	}
	prepared, err := b.mounts.MountPeer().Prepare(context.Background(), a.owner.AuthorityRef(), mountStoreOperation(t, a, "ancestor").Request)
	if err != nil || !prepared.Prepared {
		t.Fatalf("prepare: %#v %v", prepared, err)
	}
	// A only needs B's owned N envelope to stay fixed. C's independently
	// admitted content can change within N without expanding that envelope.
	if _, err := c.elements.CreateEntity(rangeStoreActor, entity, rangeStoreTuple(t, n, 0, 4), "descendant-write"); err != nil {
		t.Fatalf("unrelated descendant write was unnecessarily frozen: %v", err)
	}
	poseOnly := f.update(t, 1, n, 5)
	poseOnly.Final = rangeStoreTuple(t, m, 1, 5)
	if _, err := b.mounts.ApplyMount(context.Background(), f.callers, "move-child-envelope", poseOnly); !errors.Is(err, corecontract.ErrMountBusy) {
		t.Fatalf("fast pose-only child control bypassed ancestor gate: %v", err)
	}
	result, err := a.mounts.ApplyMount(context.Background(), f.callers, "ancestor", ancestor)
	if err != nil || result.Decision != corecontract.MountCommitted || !result.Settled {
		t.Fatalf("finish ancestor: %#v %v", result, err)
	}
	if _, exists := mountStoreFacts(c).Elements.Entity(entity); !exists || mountStoreFacts(b).Mount.Incoming().Admission.RangeValue.Radius() != 6 {
		t.Fatal("ancestor update lost descendant content or failed to publish its envelope")
	}
	f.validate(t)
}

func TestNestedMountAncestorAbortResumesExactChildIntent(t *testing.T) {
	f := newNestedMountFixture(t, false)
	m, n := rangeStoreID(9811), rangeStoreID(9812)
	f.establish(t, 0, 1, f.anchor(t, 0, m, f.root, 10), "a-b")
	f.establish(t, 1, 2, f.anchor(t, 1, n, m, 5), "b-c")
	a, b := f.owners[0], f.owners[1]
	child, ancestor := f.update(t, 1, n, 8), f.update(t, 0, m, 6)
	f.transports[1].afterPrepare = func() {
		f.transports[1].afterPrepare = nil
		if err := reserveNestedMount(a, f.callers, "ancestor", ancestor); err != nil {
			t.Fatal(err)
		}
		prepared, err := b.mounts.MountPeer().Prepare(context.Background(), a.owner.AuthorityRef(), mountStoreOperation(t, a, "ancestor").Request)
		if err != nil || !prepared.Prepared {
			t.Fatalf("prepare: %#v %v", prepared, err)
		}
	}
	if _, err := b.mounts.ApplyMount(context.Background(), f.callers, "child", child); !errors.Is(err, corecontract.ErrMountBusy) {
		t.Fatalf("did not suspend child decision: %v", err)
	}
	// The existing denied decision path must still release a preparation. It
	// does not need the ancestor's write gate to become open first.
	err := abortNestedAncestor(a)
	if err != nil {
		t.Fatal(err)
	}
	if result, err := a.mounts.ApplyMount(context.Background(), f.callers, "ancestor", ancestor); err != nil || result.Decision != corecontract.MountAborted || !result.Settled {
		t.Fatalf("ancestor abort cleanup: %#v %v", result, err)
	}
	if result, err := b.mounts.ApplyMount(context.Background(), f.callers, "child", child); err != nil || result.Decision != corecontract.MountCommitted || !result.Settled {
		t.Fatalf("exact child intent did not resume: %#v %v", result, err)
	}
	f.validate(t)
}

func TestNestedMountDetachedAncestorAbortsPendingChildWithoutStrandingPeer(t *testing.T) {
	f := newNestedMountFixture(t, false)
	m, n := rangeStoreID(9821), rangeStoreID(9822)
	f.establish(t, 0, 1, f.anchor(t, 0, m, f.root, 10), "a-b")
	f.establish(t, 1, 2, f.anchor(t, 1, n, m, 5), "b-c")
	a, b, c := f.owners[0], f.owners[1], f.owners[2]
	child := f.update(t, 1, n, 8)
	f.transports[1].afterPrepare = func() {
		f.transports[1].afterPrepare = nil
		read, err := a.mounts.ReadMount(rangeStoreActor, m)
		if err != nil {
			t.Fatal(err)
		}
		result, err := a.mounts.ApplyMount(context.Background(), f.callers, "detach", corecontract.MountCommand{Kind: corecontract.MountRevoke, MountAnchorUUID: m, ExpectedMountRevision: read.MountRevision})
		if err != nil || result.Decision != corecontract.MountCommitted || !result.Settled {
			t.Fatalf("ancestor detach: %#v %v", result, err)
		}
	}
	result, err := b.mounts.ApplyMount(context.Background(), f.callers, "child", child)
	if err != nil || result.Decision != corecontract.MountAborted || result.AbortCause != corecontract.MountAbortConflict || !result.Settled {
		t.Fatalf("detached ancestor permitted child commit or stranded intent: %#v %v", result, err)
	}
	if !mountStoreFacts(b).Mount.Incoming().Detached || mountStoreFacts(b).Mount.Incoming().Gate != mountfact.GateClosed || mountStoreFacts(c).Mount.Incoming().Gate != mountfact.GateOpen || mountStoreFacts(c).Mount.Incoming().Admission.RangeValue.Radius() != 5 {
		t.Fatal("cleanup changed ancestor detach or failed to release child preparation")
	}
	f.validate(t)
}
