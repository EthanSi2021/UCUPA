package acceptance_test

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"ucupa/shared/values"
	corecontract "ucupa/ucupa-core/contract"
)

func TestP2MountEstablishUpdateDetachAndReopen(t *testing.T) {
	f := newMountFixture(t)
	established := f.establish()
	a, b := f.readA(), f.readB()
	if a.State != corecontract.MountActive || b.State != corecontract.MountActive || a.MountKey != b.MountKey ||
		a.AdmissionKey != b.AdmissionKey || a.ControlSlot != "" || !b.WriteGateOpen {
		t.Fatalf("mount did not activate both sides: A=%#v B=%#v", a, b)
	}
	read, err := f.b.Elements().ReadElement(f.actorB, mountAnchor, nil)
	if err != nil || read.State != corecontract.ExistenceNotFound {
		t.Fatalf("carrier fabricated M: %v, %v", read, err)
	}
	entity, err := f.b.Elements().CreateEntity(f.actorB, mountEntity, f.tuple(mountAnchor, 70, 10), "mount-create-entity")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.a.Elements().CreateEntity(f.actorA, mountOther, f.tuple(mountAnchor, 0, 1), "parent-local-child"); !errors.Is(err, corecontract.ErrConflict) {
		t.Fatalf("parent wrote mounted content: %v", err)
	}
	if _, err := f.b.Elements().CreateRoot(f.actorB, mountRoot, "carrier-root"); !errors.Is(err, corecontract.ErrConflict) {
		t.Fatalf("carrier created a second root: %v", err)
	}

	tooSmall, err := f.a.Mounts().ApplyMount(context.Background(), f.callers(), "mount-shrink-rejected", f.rangeCommand(50))
	if err != nil || tooSmall.Decision != corecontract.MountAborted || tooSmall.AbortCause != corecontract.MountAbortConflict || !tooSmall.Settled {
		t.Fatalf("unsafe shrink: %#v, %v", tooSmall, err)
	}
	if got := f.readB(); !got.WriteGateOpen || got.Range.Radius() != 100 {
		t.Fatalf("rejected shrink changed admission: %#v", got)
	}
	updated, err := f.a.Mounts().ApplyMount(context.Background(), f.callers(), "mount-shrink", f.rangeCommand(80))
	if err != nil || updated.Decision != corecontract.MountCommitted || !updated.Settled || !updated.Changed {
		t.Fatalf("valid shrink: %#v, %v", updated, err)
	}
	if _, err := f.b.Elements().CreateEntity(f.actorB, mountOther, f.tuple(mountAnchor, 80, 1), "outside-new-range"); !errors.Is(err, corecontract.ErrConflict) {
		t.Fatalf("new admission ignored: %v", err)
	}

	a, b = f.readA(), f.readB()
	local, err := f.a.Elements().ReadElement(f.actorA, mountAnchor, nil)
	if err != nil {
		t.Fatal(err)
	}
	noOpCommand := f.rangeCommand(80)
	noOp, err := f.a.Mounts().ApplyMount(context.Background(), f.callers(), "mount-noop", noOpCommand)
	if err != nil || noOp.Decision != corecontract.MountCommitted || noOp.Changed || !noOp.Settled || !noOp.DecisionCut.Equal(a.Cut) {
		t.Fatalf("no-op: %#v, %v", noOp, err)
	}
	if after := f.readB(); !after.Cut.Equal(b.Cut) {
		t.Fatal("no-op contacted or changed carrier")
	}
	if !reflect.DeepEqual(noOp.Record, local.Record) {
		t.Fatal("no-op lost original record")
	}

	f.reopen()
	if got := f.readA(); !reflect.DeepEqual(got, a) {
		t.Fatalf("parent changed after reopen: %#v vs %#v", got, a)
	}
	if got := f.readB(); !reflect.DeepEqual(got, b) {
		t.Fatalf("carrier changed after reopen: %#v vs %#v", got, b)
	}
	noOpReplay, err := f.a.Mounts().ApplyMount(context.Background(), f.callers(), "mount-noop", noOpCommand)
	if err != nil || !reflect.DeepEqual(noOpReplay, noOp) {
		t.Fatalf("no-op replay: %#v, %v", noOpReplay, err)
	}
	if got, err := f.b.Elements().ReadElement(f.actorB, mountEntity, nil); err != nil || !reflect.DeepEqual(got.Record, entity.Record) {
		t.Fatalf("carrier entity reopen: %#v, %v", got, err)
	}
	revoked, err := f.a.Mounts().ApplyMount(context.Background(), f.callers(), "mount-revoke", corecontract.MountCommand{Kind: corecontract.MountRevoke, MountAnchorUUID: mountAnchor, ExpectedMountRevision: a.MountRevision})
	if err != nil || revoked.Decision != corecontract.MountCommitted || !revoked.Settled {
		t.Fatalf("revoke: %#v, %v", revoked, err)
	}
	if f.readA().State != corecontract.MountDetached || f.readB().State != corecontract.MountDetached || f.readB().WriteGateOpen {
		t.Fatal("revoke did not detach and close gate")
	}
	if _, err := f.b.Elements().DeleteEntity(f.actorB, mountEntity, entity.Record.Entity.Revision, "delete-detached"); !errors.Is(err, corecontract.ErrMountBusy) {
		t.Fatalf("detached write: %v", err)
	}
	if _, err := f.a.Elements().DeleteAnchor(f.actorA, mountAnchor, updated.Record.Anchor.Revision, "delete-detached-anchor"); !errors.Is(err, corecontract.ErrConflict) {
		t.Fatalf("detached M deletion: %v", err)
	}
	f.reopen()
	replay, err := f.a.Mounts().ApplyMount(context.Background(), f.callers(), "mount-establish", f.command)
	if err != nil || !reflect.DeepEqual(replay, established) || f.readA().State != corecontract.MountDetached {
		t.Fatalf("old success reactivated detached mount: %#v, %v", replay, err)
	}
}

func TestP2MountKeepsLocalReadsFromClaimingRemoteCompleteness(t *testing.T) {
	f := newMountFixture(t)
	f.establish()
	if _, err := f.a.Elements().ListDirect(f.actorA, mountAnchor, 10, nil); !errors.Is(err, corecontract.ErrMountContextRequired) {
		t.Fatalf("parent ListDirect: %v", err)
	}
	if _, err := f.b.Elements().ListDirect(f.actorB, mountAnchor, 10, nil); !errors.Is(err, corecontract.ErrMountContextRequired) {
		t.Fatalf("carrier ListDirect: %v", err)
	}
	query := corecontract.RegionQuery{StartAnchorID: mountRoot, BasisAnchorID: mountRoot, Sphere: corecontract.QuerySphere{Radius: 1000}, Selector: corecontract.QueryBoth}
	page, err := f.a.Queries().QueryRegion(f.actorA, query, nil, 10, nil)
	if !errors.Is(err, corecontract.ErrMountContextRequired) || !reflect.DeepEqual(page, corecontract.QueryPage{}) {
		t.Fatalf("parent query claimed complete: %#v, %v", page, err)
	}
	query.StartAnchorID, query.BasisAnchorID = mountAnchor, mountAnchor
	page, err = f.b.Queries().QueryRegion(f.actorB, query, nil, 10, nil)
	if !errors.Is(err, corecontract.ErrMountContextRequired) || !reflect.DeepEqual(page, corecontract.QueryPage{}) {
		t.Fatalf("carrier query claimed complete: %#v, %v", page, err)
	}
}

func TestP2MissingPeerCallerDoesNotReserveOrClaimOperation(t *testing.T) {
	f := newMountFixture(t)
	before := f.readA()
	_, err := f.a.Mounts().ApplyMount(context.Background(), corecontract.LocalCallers{f.a.AuthorityRef(): f.actorA}, "same-intent", f.command)
	if !errors.Is(err, corecontract.ErrDenied) {
		t.Fatal(err)
	}
	if after := f.readA(); !reflect.DeepEqual(before, after) {
		t.Fatal("missing peer caller reserved state")
	}
	result, err := f.a.Mounts().ApplyMount(context.Background(), f.callers(), "same-intent", f.command)
	if err != nil || result.Decision != corecontract.MountCommitted || !result.Settled {
		t.Fatalf("missing peer caller claimed operation ID: %#v, %v", result, err)
	}
	if _, err := f.a.Mounts().ReadMount(f.actorB, mountAnchor); !errors.Is(err, corecontract.ErrDenied) {
		t.Fatalf("peer principal inherited permission: %v", err)
	}
}

func TestP2AddressRepairDuringPendingEstablishAndIdentityMismatch(t *testing.T) {
	f := newMountFixture(t)
	if err := f.connections.UnbindEndpoint(mountAddressB); err != nil {
		t.Fatal(err)
	}
	pending, err := f.a.Mounts().ApplyMount(context.Background(), f.callers(), "mount-establish", f.command)
	if err != nil || pending.Decision != corecontract.MountPending || pending.Settled || pending.BlockedBy != corecontract.MountBlockedUnavailable {
		t.Fatalf("offline became absent: %#v, %v", pending, err)
	}
	before := f.readA()
	const repaired values.NextHop = "local:carrier-repaired"
	changed, err := f.a.Mounts().UpdateMountNextHop(f.actorA, "repair-address", mountAnchor, before.MountKey, before.NextHopRevision, repaired)
	if err != nil || !changed.Changed {
		t.Fatalf("pending address repair: %#v, %v", changed, err)
	}
	after := f.readA()
	if !after.MountRevision.Equal(before.MountRevision) || after.MountKey != before.MountKey || after.Cut.Equal(before.Cut) {
		t.Fatal("address update changed mount identity/revision or failed to advance cut")
	}
	if err := f.connections.BindEndpoint(repaired, f.a.Mounts().MountPeer()); err != nil {
		t.Fatal(err)
	}
	wrong, err := f.a.Mounts().ApplyMount(context.Background(), f.callers(), "mount-establish", f.command)
	if err != nil || wrong.Decision != corecontract.MountPending || wrong.BlockedBy != corecontract.MountBlockedOwnerMismatch {
		t.Fatalf("wrong Core accepted: %#v, %v", wrong, err)
	}
	if err := f.connections.BindEndpoint(repaired, f.b.Mounts().MountPeer()); err != nil {
		t.Fatal(err)
	}
	f.reopen()
	if err := f.connections.BindEndpoint(repaired, f.b.Mounts().MountPeer()); err != nil {
		t.Fatal(err)
	}
	result, err := f.a.Mounts().ApplyMount(context.Background(), f.callers(), "mount-establish", f.command)
	if err != nil || result.Decision != corecontract.MountCommitted || !result.Settled {
		t.Fatalf("repaired retry: %#v, %v", result, err)
	}
	if got := f.readA(); got.NextHop != repaired {
		t.Fatal("original Establish address overwrote repaired contact")
	}
	replay, err := f.a.Mounts().UpdateMountNextHop(f.actorA, "repair-address", mountAnchor, before.MountKey, before.NextHopRevision, repaired)
	if err != nil || replay != changed {
		t.Fatalf("address receipt changed: %#v, %v", replay, err)
	}
}
