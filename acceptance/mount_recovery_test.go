package acceptance_test

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"ucupa/shared/values"
	corecontract "ucupa/ucupa-core/contract"
)

func TestP2RangeGateSerializesWritesAndPreservesExactReceiptReplay(t *testing.T) {
	f := newMountFixture(t)
	f.establish()
	initialTuple := f.tuple(mountAnchor, 5, 1)
	created, err := f.b.Elements().CreateEntity(f.actorB, mountEntity, initialTuple, "create-before-gate")
	if err != nil {
		t.Fatal(err)
	}
	command := f.rangeCommand(80)
	base, err := f.connections.TransportFor(f.a)
	if err != nil {
		t.Fatal(err)
	}
	prepared, release := make(chan struct{}), make(chan struct{})
	defer func() {
		select {
		case <-release:
		default:
			close(release)
		}
	}()
	hook := mountTransportHook{base: base, afterPrepare: func(result corecontract.MountPrepared) {
		if result.Prepared {
			close(prepared)
			<-release
		}
	}}
	if err := f.a.Mounts().ConfigureMountTransport(hook); err != nil {
		t.Fatal(err)
	}
	type outcome struct {
		result corecontract.MountControlResult
		err    error
	}
	done := make(chan outcome, 1)
	go func() {
		result, err := f.a.Mounts().ApplyMount(context.Background(), f.callers(), "coordinated-shrink", command)
		done <- outcome{result, err}
	}()
	select {
	case <-prepared:
	case got := <-done:
		t.Fatalf("range did not prepare: %#v", got)
	case <-time.After(5 * time.Second):
		t.Fatal("range preparation blocked")
	}
	if f.readB().WriteGateOpen {
		t.Fatal("prepared carrier gate is open")
	}
	if _, err := f.b.Elements().CreateEntity(f.actorB, mountOther, f.tuple(mountAnchor, 70, 1), "busy-not-claimed"); !errors.Is(err, corecontract.ErrMountBusy) {
		t.Fatalf("concurrent create: %v", err)
	}
	if _, err := f.b.Elements().UpdateEntity(f.actorB, mountEntity, f.tuple(mountAnchor, 20, 1), created.Record.Entity.Revision, "busy-update"); !errors.Is(err, corecontract.ErrMountBusy) {
		t.Fatalf("concurrent update: %v", err)
	}
	if _, err := f.a.Elements().UpdateAnchor(f.actorA, mountAnchor, f.tuple(mountRoot, 10, 80), command.ExpectedElementRevision, "busy-parent"); !errors.Is(err, corecontract.ErrMountBusy) {
		t.Fatalf("concurrent parent update: %v", err)
	}
	replay, err := f.b.Elements().CreateEntity(f.actorB, mountEntity, initialTuple, "create-before-gate")
	if err != nil || !reflect.DeepEqual(replay, created) {
		t.Fatalf("closed gate lost prior receipt: %#v, %v", replay, err)
	}
	// No Core lock is held while waiting on the other owner: unrelated parent
	// writes and the carrier's address repair remain available.
	if _, err := f.a.Elements().CreateAnchor(f.actorA, mountBranch, f.tuple(mountRoot, 0, 1), "unrelated-anchor"); err != nil {
		t.Fatalf("unrelated branch blocked: %v", err)
	}
	b := f.readB()
	if _, err := f.b.Mounts().UpdateMountNextHop(f.actorB, "repair-during-gate", mountAnchor, b.MountKey, b.NextHopRevision, "local:parent-repaired"); err != nil {
		t.Fatalf("address repair blocked by gate: %v", err)
	}
	close(release)
	select {
	case got := <-done:
		if got.err != nil || got.result.Decision != corecontract.MountCommitted || !got.result.Settled {
			t.Fatalf("range completion: %#v", got)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("range completion blocked")
	}
	if _, err := f.b.Elements().CreateEntity(f.actorB, mountOther, f.tuple(mountAnchor, 70, 1), "busy-not-claimed"); err != nil {
		t.Fatalf("busy claimed operation ID: %v", err)
	}
	if got := f.readB(); got.Range.Radius() != 80 || !got.WriteGateOpen {
		t.Fatalf("new range not active: %#v", got)
	}
	f.reopen()
}

func TestP2RecoveryClosesPreparedCarrierAndSettlesOriginalDecision(t *testing.T) {
	f := newMountFixture(t)
	base, err := f.connections.TransportFor(f.a)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.a.Mounts().ConfigureMountTransport(mountTransportHook{base: base, finishError: corecontract.ErrUnavailable}); err != nil {
		t.Fatal(err)
	}
	committed, err := f.a.Mounts().ApplyMount(context.Background(), f.callers(), "mount-establish", f.command)
	if err != nil || committed.Decision != corecontract.MountCommitted || committed.Settled || committed.BlockedBy != corecontract.MountBlockedUnavailable {
		t.Fatalf("lost close: %#v, %v", committed, err)
	}
	if f.readB().WriteGateOpen {
		t.Fatal("carrier opened before learning committed decision")
	}
	f.reopen()
	if f.readB().WriteGateOpen {
		t.Fatal("reopen guessed a prepared decision")
	}
	beforeA := f.readA()
	report, err := f.b.Mounts().RecoverMounts(context.Background())
	if err != nil || len(report.Items) != 1 || report.Items[0].Err != nil || !f.readB().WriteGateOpen {
		t.Fatalf("carrier reverse recovery: %#v, %v", report, err)
	}
	if after := f.readA(); !reflect.DeepEqual(after, beforeA) {
		t.Fatal("decision read mutated parent")
	}
	report, err = f.a.Mounts().RecoverMounts(context.Background())
	if err != nil || len(report.Items) != 1 || report.Items[0].Err != nil {
		t.Fatalf("parent recovery: %#v, %v", report, err)
	}
	settled, err := f.a.Mounts().ReadMountOperation(f.actorA, mountAnchor, "mount-establish")
	if err != nil || !settled.Settled || settled.Decision != committed.Decision || !settled.DecisionCut.Equal(committed.DecisionCut) {
		t.Fatalf("recovery changed decision: %#v, %v", settled, err)
	}
	f.reopen()
	if report, err := f.a.Mounts().RecoverMounts(context.Background()); err != nil || len(report.Items) != 0 {
		t.Fatalf("settled operation recovered again: %#v, %v", report, err)
	}
}

func TestP2RevokedAuthorizationAbortsNewCommitButCannotBlockClosingObligation(t *testing.T) {
	f := newMountFixture(t)
	f.establish()
	command := f.rangeCommand(80)
	base, err := f.connections.TransportFor(f.a)
	if err != nil {
		t.Fatal(err)
	}
	hook := mountTransportHook{base: base, afterPrepare: func(result corecontract.MountPrepared) {
		if !result.Prepared {
			t.Fatal("expected prepared range")
		}
		grantA := binding(f.actorA, corecontract.PermissionMountControl, values.CoreElementScope(f.a.AuthorityRef(), mountAnchor))
		if _, err := f.policyA.DeleteBinding(f.actorA, grantA, f.policyA.BindingSetRevision(), "revoke-parent-control"); err != nil {
			t.Fatal(err)
		}
		grantB := binding(f.actorB, corecontract.PermissionMountAccept, values.CoreElementScope(f.b.AuthorityRef(), mountAnchor))
		if _, err := f.policyB.DeleteBinding(f.actorB, grantB, f.policyB.BindingSetRevision(), "revoke-carrier-accept"); err != nil {
			t.Fatal(err)
		}
	}}
	if err := f.a.Mounts().ConfigureMountTransport(hook); err != nil {
		t.Fatal(err)
	}
	result, err := f.a.Mounts().ApplyMount(context.Background(), f.callers(), "denied-after-prepare", command)
	if !errors.Is(err, corecontract.ErrDenied) || !reflect.DeepEqual(result, corecontract.MountControlResult{}) {
		t.Fatalf("revoked caller saw retained result: %#v, %v", result, err)
	}
	retained, err := f.a.Mounts().ReadMountOperation(f.actorA, mountAnchor, "denied-after-prepare")
	if err != nil || retained.Decision != corecontract.MountAborted || retained.AbortCause != corecontract.MountAbortDenied || !retained.Settled {
		t.Fatalf("permission loss left control stuck: %#v, %v", retained, err)
	}
	if b := f.readB(); !b.WriteGateOpen || b.Range.Radius() != 100 {
		t.Fatalf("revoked acceptance blocked required Abort close: %#v", b)
	}
	f.reopen()
	if b := f.readB(); !b.WriteGateOpen || b.Range.Radius() != 100 {
		t.Fatal("reopen changed aborted admission")
	}
}

func TestP2OfflineRevokeDoesNotClaimCarrierStopped(t *testing.T) {
	f := newMountFixture(t)
	f.establish()
	if err := f.connections.UnbindEndpoint(mountAddressB); err != nil {
		t.Fatal(err)
	}
	result, err := f.a.Mounts().ApplyMount(context.Background(), f.callers(), "offline-revoke", corecontract.MountCommand{Kind: corecontract.MountRevoke, MountAnchorUUID: mountAnchor, ExpectedMountRevision: f.readA().MountRevision})
	if err != nil || result.Decision != corecontract.MountCommitted || result.Settled || result.BlockedBy != corecontract.MountBlockedUnavailable {
		t.Fatalf("offline revoke: %#v, %v", result, err)
	}
	if f.readA().State != corecontract.MountRevoking || !f.readB().WriteGateOpen {
		t.Fatal("offline revoke claimed remote closure")
	}
	if _, err := f.b.Elements().CreateEntity(f.actorB, mountEntity, f.tuple(mountAnchor, 5, 1), "not-yet-informed"); err != nil {
		t.Fatal(err)
	}
	f.reopen()
	if _, err := f.a.Mounts().RecoverMounts(context.Background()); err != nil {
		t.Fatal(err)
	}
	if f.readA().State != corecontract.MountDetached || f.readB().State != corecontract.MountDetached || f.readB().WriteGateOpen {
		t.Fatal("revoke recovery failed")
	}
	if read, err := f.b.Elements().ReadElement(f.actorB, mountEntity, nil); err != nil || read.State != corecontract.ExistencePresent {
		t.Fatalf("detach discarded carrier data: %#v, %v", read, err)
	}
}
