package commit_test

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"ucupa/shared/values"
	corecontract "ucupa/ucupa-core/contract"
	"ucupa/ucupa-core/internal/commit"
	mountfact "ucupa/ucupa-core/internal/mount/fact"
)

func TestMountControlMissingCallerNeverReservesAndPeerDeniedAborts(t *testing.T) {
	f := newMountControlFixture(t, false)
	before := commit.CaptureCompleteStateForTest(f.a.owner)
	callers := corecontract.LocalCallers{f.a.owner.AuthorityRef(): rangeStoreActor}
	result, err := f.a.mounts.ApplyMount(context.Background(), callers, "missing-peer", f.command)
	if !errors.Is(err, corecontract.ErrDenied) || !reflect.DeepEqual(result, corecontract.MountControlResult{}) || !commit.MatchesCompleteStateForTest(f.a.owner, before) {
		t.Fatalf("missing caller was admitted: %#v %v", result, err)
	}
	f.policyB.denied.Store(true)
	result, err = f.a.mounts.ApplyMount(context.Background(), f.callers, "denied-peer", f.command)
	if err != nil || result.Decision != corecontract.MountAborted || result.AbortCause != corecontract.MountAbortDenied || !result.Settled {
		t.Fatalf("peer denial: %#v %v", result, err)
	}
	if mountStoreFacts(f.a).Mount.Record(f.mount).State != corecontract.MountNone || mountStoreFacts(f.b).Mount.Incoming().Key.Valid() {
		t.Fatal("denial left a live relation")
	}
	before = commit.CaptureCompleteStateForTest(f.a.owner)
	got, err := f.a.mounts.ApplyMount(context.Background(), f.callers, "denied-peer", f.command)
	if err != nil || !reflect.DeepEqual(got, result) || !commit.MatchesCompleteStateForTest(f.a.owner, before) {
		t.Fatalf("abort replay mutated state: %#v %v", got, err)
	}
	f.policyB.denied.Store(false)
	f.apply(t, "after-denial", f.command)
}

func TestMountControlPendingAddressRepairAndExactOperationNamespace(t *testing.T) {
	f := newMountControlFixture(t, false)
	delete(f.transportA.endpoints, "local:b")
	pending, err := f.a.mounts.ApplyMount(context.Background(), f.callers, "pending", f.command)
	if err != nil || pending.Decision != corecontract.MountPending || pending.Settled || pending.BlockedBy != corecontract.MountBlockedUnavailable {
		t.Fatalf("unavailable incorrectly decided: %#v %v", pending, err)
	}
	m, err := f.a.mounts.ReadMount(rangeStoreActor, f.mount)
	if err != nil {
		t.Fatal(err)
	}
	cut := mountControlCut(f.a)
	updated, err := f.a.mounts.UpdateMountNextHop(rangeStoreActor, "repair", f.mount, m.MountKey, m.NextHopRevision, "local:b-new")
	if err != nil || !updated.Changed || updated.Cut == cut {
		t.Fatalf("pending address repair: %#v %v", updated, err)
	}
	if mountStoreFacts(f.a).Mount.Record(f.mount).Revision != m.MountRevision {
		t.Fatal("address repair changed mount revision")
	}
	if _, err = f.a.elements.CreateRoot(rangeStoreActor, rangeStoreID(9202), "pending"); !errors.Is(err, corecontract.ErrOperationConflict) {
		t.Fatalf("ordinary/mount ID collision: %v", err)
	}
	if _, err = f.a.mounts.ApplyMount(context.Background(), f.callers, "repair", f.command); !errors.Is(err, corecontract.ErrOperationConflict) {
		t.Fatalf("address/mount ID collision: %v", err)
	}
	f.apply(t, "pending", f.command)
	old, err := f.a.mounts.UpdateMountNextHop(rangeStoreActor, "repair", f.mount, m.MountKey, m.NextHopRevision, "local:b-new")
	if err != nil || old != updated {
		t.Fatalf("repair receipt changed: %#v %v", old, err)
	}
}

func TestMountControlRangeGateAddressRepairAndOriginalAdmission(t *testing.T) {
	f := newMountControlFixture(t, true)
	entity := rangeStoreID(9203)
	created, err := f.b.elements.CreateEntity(rangeStoreActor, entity, rangeStoreTuple(t, f.mount, 4, 1), "old-child")
	if err != nil {
		t.Fatal(err)
	}
	acceptance := mountStoreFacts(f.b).Mount.Incoming().Acceptance
	beforeAdmission := mountStoreFacts(f.b).Mount.Incoming().Admission.Key
	command := f.update(t, 5)
	f.transportA.afterPrepare = func() {
		// These callbacks run without either Core mutex held.
		before := mountControlCut(f.b)
		if _, err := f.b.elements.CreateEntity(rangeStoreActor, rangeStoreID(9204), rangeStoreTuple(t, f.mount, 0, 1), "blocked-child"); !errors.Is(err, corecontract.ErrMountBusy) {
			t.Fatalf("write gate: %v", err)
		}
		if mountControlCut(f.b) != before {
			t.Fatal("blocked write advanced cut")
		}
		old, err := f.b.elements.CreateEntity(rangeStoreActor, entity, rangeStoreTuple(t, f.mount, 4, 1), "old-child")
		if err != nil || !reflect.DeepEqual(old, created) {
			t.Fatalf("old receipt blocked: %#v %v", old, err)
		}
		if _, err = f.a.elements.UpdateAnchor(rangeStoreActor, f.mount, command.Final, command.ExpectedElementRevision, "busy-M"); !errors.Is(err, corecontract.ErrMountBusy) {
			t.Fatalf("M not frozen: %v", err)
		}
		if _, err = f.a.elements.CreateEntity(rangeStoreActor, rangeStoreID(9205), rangeStoreTuple(t, f.root, 0, 1), "other-branch"); err != nil {
			t.Fatal(err)
		}
		for _, owner := range []*mountStoreOwner{f.a, f.b} {
			read, err := owner.mounts.ReadMount(rangeStoreActor, f.mount)
			if err != nil {
				t.Fatal(err)
			}
			address := values.NextHop("local:b-new")
			if owner == f.b {
				address = "local:a-new"
			}
			if _, err = owner.mounts.UpdateMountNextHop(rangeStoreActor, "repair-in-prepare", f.mount, read.MountKey, read.NextHopRevision, address); err != nil {
				t.Fatal(err)
			}
		}
	}
	f.apply(t, "shrink", command)
	if mountStoreFacts(f.b).Mount.Incoming().Acceptance != acceptance || mountStoreFacts(f.b).Mount.Incoming().Admission.Key == beforeAdmission {
		t.Fatal("admission/immutable acceptance conflated")
	}
	if commit.InspectMountGuardForTest(f.b.owner, entity, "blocked-child").OperationRetained {
		t.Fatal("gate reserved rejected ID")
	}
	if _, err = f.b.elements.CreateEntity(rangeStoreActor, rangeStoreID(9204), rangeStoreTuple(t, f.mount, 0, 1), "blocked-child"); err != nil {
		t.Fatalf("ID unusable after gate reopened: %v", err)
	}
}

func TestMountControlRevocationDuringPrepareAbortsButClosesObligation(t *testing.T) {
	f := newMountControlFixture(t, true)
	oldAdmission := mountStoreFacts(f.b).Mount.Incoming().Admission
	command := f.update(t, 8)
	f.transportA.afterPrepare = func() { f.policyA.denied.Store(true); f.policyB.denied.Store(true) }
	result, err := f.a.mounts.ApplyMount(context.Background(), f.callers, "revoked-actor", command)
	if !errors.Is(err, corecontract.ErrDenied) || !reflect.DeepEqual(result, corecontract.MountControlResult{}) {
		t.Fatalf("revoked caller saw outcome: %#v %v", result, err)
	}
	op := mountStoreOperation(t, f.a, "revoked-actor")
	if op.Decision != corecontract.MountAborted || op.AbortCause != corecontract.MountAbortDenied || !op.Settled {
		t.Fatalf("revocation did not close abort: %#v", op)
	}
	if mountStoreFacts(f.b).Mount.Incoming().Gate != mountfact.GateOpen || mountStoreFacts(f.b).Mount.Incoming().Admission != oldAdmission {
		t.Fatal("old admission lost or gate retained after abort")
	}
	if _, err = f.a.mounts.ReadMountOperation(rangeStoreActor, f.mount, "revoked-actor"); !errors.Is(err, corecontract.ErrDenied) {
		t.Fatal("read bypassed fresh permission")
	}
}

func TestMountControlCanceledWaitKeepsPendingAndNoPreparedAbortDoesNotRevive(t *testing.T) {
	f := newMountControlFixture(t, false)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	pending, err := f.a.mounts.ApplyMount(ctx, f.callers, "cancel-wait", f.command)
	if err != nil || pending.Decision != corecontract.MountPending || pending.Settled {
		t.Fatalf("canceled wait chose abort: %#v %v", pending, err)
	}
	request := mountStoreOperation(t, f.a, "cancel-wait").Request
	f.policyA.denied.Store(true)
	report, err := f.a.mounts.RecoverMounts(context.Background())
	if err != nil || len(report.Items) != 1 || report.Items[0].Err != nil {
		t.Fatalf("recovery: %#v %v", report, err)
	}
	op := mountStoreOperation(t, f.a, "cancel-wait")
	if op.Decision != corecontract.MountAborted || !op.Settled || mountStoreFacts(f.b).Mount.Incoming().Key.Valid() {
		t.Fatal("unprepared abort did not settle")
	}
	before := commit.CaptureCompleteStateForTest(f.b.owner)
	late, err := f.b.mounts.MountPeer().Prepare(context.Background(), f.a.owner.AuthorityRef(), request)
	if err != nil || late.Prepared || !commit.MatchesCompleteStateForTest(f.b.owner, before) {
		t.Fatalf("late Prepare reopened canceled attempt: %#v %v", late, err)
	}
	f.policyA.denied.Store(false)
	f.apply(t, "new-establish", f.command)
	if mountStoreFacts(f.a).Mount.Record(f.mount).Key == request.Key {
		t.Fatal("canceled proposal generation reused")
	}
}

func TestMountControlPurePoseAndUnchangedNeverContactPeer(t *testing.T) {
	f := newMountControlFixture(t, true)
	if err := f.a.mounts.ConfigureMountTransport(nil); err != nil {
		t.Fatal(err)
	}
	beforeB := commit.CaptureCompleteStateForTest(f.b.owner)
	beforeMount := mountStoreFacts(f.a).Mount.Record(f.mount)
	command := f.update(t, 10)
	beforeCut := mountControlCut(f.a)
	unchanged := f.apply(t, "unchanged", command)
	if unchanged.Changed || unchanged.DecisionCut != beforeCut || mountControlCut(f.a) != beforeCut {
		t.Fatal("unchanged advanced cut")
	}
	command = f.update(t, 10)
	command.Final = rangeStoreTuple(t, f.root, 2, 10)
	changed := f.apply(t, "pose", command)
	if !changed.Changed || changed.Record == nil || !commit.MatchesCompleteStateForTest(f.b.owner, beforeB) || mountStoreFacts(f.a).Mount.Record(f.mount) != beforeMount {
		t.Fatal("Pose-only changed remote admission or controls")
	}
}

func TestMountControlDeniedLocalUpdateAndRevokeNeverAdvance(t *testing.T) {
	for _, kind := range []corecontract.MountCommandKind{corecontract.MountUpdate, corecontract.MountRevoke} {
		t.Run(map[corecontract.MountCommandKind]string{corecontract.MountUpdate: "local-update", corecontract.MountRevoke: "revoke"}[kind], func(t *testing.T) {
			f := newMountControlFixture(t, true)
			command := f.update(t, 10)
			command.Final = rangeStoreTuple(t, f.root, 2, 10)
			if kind == corecontract.MountRevoke {
				command = corecontract.MountCommand{Kind: corecontract.MountRevoke, MountAnchorUUID: f.mount, ExpectedMountRevision: command.ExpectedMountRevision}
			}
			beforeA, beforeB := commit.CaptureCompleteStateForTest(f.a.owner), commit.CaptureCompleteStateForTest(f.b.owner)
			f.policyA.denied.Store(true)
			result, err := f.a.mounts.ApplyMount(context.Background(), f.callers, "denied-local", command)
			if !errors.Is(err, corecontract.ErrDenied) || !reflect.DeepEqual(result, corecontract.MountControlResult{}) || !commit.MatchesCompleteStateForTest(f.a.owner, beforeA) || !commit.MatchesCompleteStateForTest(f.b.owner, beforeB) {
				t.Fatalf("denied local intent advanced: %#v %v", result, err)
			}
		})
	}
}
