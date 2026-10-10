package commit

import (
	"errors"
	"testing"

	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	elementfact "ucupa/ucupa-core/internal/element/fact"
	mountfact "ucupa/ucupa-core/internal/mount/fact"
	"ucupa/ucupa-core/internal/token"
	transferfact "ucupa/ucupa-core/internal/transfer/fact"
)

// Three real durable owners; M remains in the parent. The old carrier accepts
// the original mount using the same transactions as normal mount operations.
func carrierControlFixture(t *testing.T) (parent, source, target *Owner, i transferfact.CarrierIntent) {
	t.Helper()
	parent, source, target = emptyRootReceiver(t), emptyRootReceiver(t), emptyRootReceiver(t)
	root, ancestor, mount := foundationStoreID(100), foundationStoreID(101), foundationStoreID(104)
	for _, row := range []struct {
		kind  elementfact.OperationKind
		id    values.ElementID
		final contract.SpatialTuple
		op    contract.OperationID
	}{
		{elementfact.OperationCreateRoot, root, contract.SpatialTuple{}, "carrier-root"},
		{elementfact.OperationCreateAnchor, ancestor, foundationStoreTuple(t, root, 0, 100), "carrier-ancestor"},
		{elementfact.OperationCreateAnchor, mount, foundationStoreTuple(t, ancestor, 0, 20), "carrier-mount"},
	} {
		if _, err := applyFoundationStore(parent, row.kind, foundationStoreActor, row.id, row.final, contract.Revision{}, row.op); err != nil {
			t.Fatal(err)
		}
	}
	a, _ := parent.state.elements.Anchor(mount)
	b := MountBinding{Actor: foundationStoreActor, PeerActor: foundationStoreActor, Command: contract.MountCommand{Kind: contract.MountEstablish,
		MountAnchorUUID: mount, ExpectedElementRevision: a.Revision, CarrierAuthority: source.authority, CarrierNextHop: "local:carrier-old", ParentNextHop: "local:carrier-parent"}}
	parent.Lock()
	err := parent.ApplyMountIntentLocked(contract.LocalCallers{source.authority: foundationStoreActor}, "carrier-original", b)
	parent.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	request := parent.state.mount.operations["carrier-original"].Request
	source.Lock()
	prepared, err := source.ApplyMountPrepareLocked(parent.authority, request, true)
	source.Unlock()
	if err != nil || !prepared.Prepared {
		t.Fatal("original prepare", prepared, err)
	}
	parent.Lock()
	err = parent.ApplyMountEventLocked(MountEvent{Kind: MountEventDecide, Operation: "carrier-original", Prepared: prepared, Authorized: true})
	parent.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	evidence := parent.state.mount.operations["carrier-original"].Evidence()
	source.Lock()
	closed, err := source.ApplyMountFinishLocked(parent.authority, contract.MountFinishRequest{Evidence: evidence})
	source.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	parent.Lock()
	err = parent.ApplyMountEventLocked(MountEvent{Kind: MountEventSettle, Operation: "carrier-original", Closed: closed})
	parent.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	if _, err = applyFoundationStore(source, elementfact.OperationCreateEntity, foundationStoreActor, foundationStoreID(105), foundationStoreTuple(t, mount, 0, 1), contract.Revision{}, "carrier-content"); err != nil {
		t.Fatal(err)
	}
	i = transferfact.CarrierIntent{Operation: "carrier-replace", Actor: foundationStoreActor, Parent: parent.authority, Source: source.authority, Target: target.authority,
		Mount: mount, ExpectedElement: a.Revision, ExpectedMount: parent.state.mount.facts.Record(mount).Revision,
		ParentAddress: "local:carrier-parent", SourceAddress: "local:carrier-old", TargetAddress: "local:carrier-new"}
	return
}

func reserveCarrier(t *testing.T, o *Owner, i transferfact.CarrierIntent) (transferfact.CarrierControl, error) {
	t.Helper()
	o.Lock()
	defer o.Unlock()
	return o.ReserveCarrierReplacementLocked(i)
}
func advanceCarrier(t *testing.T, o *Owner, c transferfact.CarrierControl, permit bool) (transferfact.CarrierControl, error) {
	t.Helper()
	o.Lock()
	defer o.Unlock()
	i := c.Reservation.Input
	if permit {
		return o.PermitCarrierFenceLocked(i.Actor, i.Operation, c.Reservation.At)
	}
	return o.CancelCarrierReservationLocked(i.Actor, i.Operation, c.Reservation.At)
}

func TestCarrierReservationDoesNotSwitchBindingAndSurvivesReplay(t *testing.T) {
	for _, permit := range []bool{false, true} {
		parent, source, target, i := carrierControlFixture(t)
		anchor, _ := parent.state.elements.Anchor(i.Mount)
		old := parent.state.mount.facts.Record(i.Mount)
		c, err := reserveCarrier(t, parent, i)
		if err != nil || !c.Valid() {
			t.Fatal(c, err)
		}
		if c.Reservation.Previous != old.Admission || c.Reservation.NextKey == old.Key || c.Reservation.NextKey.CarrierAuthority != target.authority {
			t.Fatal("incorrect parent reservation")
		}
		if source.state.mount.facts.Incoming().Gate != mountfact.GateOpen || target.state.elements.Counts().Claims != 0 {
			t.Fatal("reserve touched peer truth")
		}
		if _, visible := parent.MountOperationLocked(i.Operation); visible {
			t.Fatal("ordinary mount API disclosed a different control variant")
		}
		ops, _ := parent.PendingMountsLocked()
		if len(ops) != 0 {
			t.Fatal("ordinary mount recovery picked up carrier replacement")
		}
		progress, err := advanceCarrier(t, parent, c, permit)
		if err != nil || !progress.Valid() {
			t.Fatal(err)
		}
		if permit {
			if _, err = advanceCarrier(t, parent, c, false); !errors.Is(err, values.PublicUnavailable) {
				t.Fatal("permit cancelled without source refusal", err)
			}
		} else {
			if _, err = advanceCarrier(t, parent, c, true); !errors.Is(err, contract.ErrConflict) {
				t.Fatal("cancelled permit resurrected", err)
			}
		}
		m := parent.state.mount.facts.Record(i.Mount)
		now, _ := parent.state.elements.Anchor(i.Mount)
		if now != anchor || m.Key != old.Key || m.Admission != old.Admission || m.State != contract.MountActive || m.Contact != old.Contact {
			t.Fatal("preparation changed spatial or serving facts")
		}
		if (m.Slot == i.Operation) != permit {
			t.Fatal("wrong slot retention")
		}
		for _, o := range []*Owner{parent, source, target} {
			path, before := o.store.db.Path(), o.state.clone()
			closeFoundationStore(t, o)
			reopened := openFoundationStore(t, path)
			if !sameCompleteCoreState(before, reopened.state) {
				t.Fatal("replay changed preparation")
			}
			if o == parent {
				got, err := reserveCarrier(t, reopened, i)
				if err != nil || got != c {
					t.Fatal("reservation retry changed original result", got, err)
				}
				got, err = advanceCarrier(t, reopened, c, permit)
				if err != nil || got != progress {
					t.Fatal("progress retry changed original result", err)
				}
				if token.Serial(reopened.state.currentCut()) != token.Serial(before.currentCut()) {
					t.Fatal("retry advanced cut")
				}
			}
			closeFoundationStore(t, reopened)
		}
	}
}

func TestCarrierPreparationSharesControlSlotAndProtectsAncestors(t *testing.T) {
	parent, source, target, i := carrierControlFixture(t)
	c, err := reserveCarrier(t, parent, i)
	if err != nil {
		t.Fatal(err)
	}
	for _, kind := range []contract.MountCommandKind{contract.MountUpdate, contract.MountRevoke} {
		m := parent.state.mount.facts.Record(i.Mount)
		command := contract.MountCommand{Kind: kind, MountAnchorUUID: i.Mount, ExpectedMountRevision: m.Revision}
		if kind == contract.MountUpdate {
			command.ExpectedElementRevision = i.ExpectedElement
			command.Final = c.Reservation.Tuple
			command.Final = foundationStoreTuple(t, c.Reservation.Tuple.Parent, 0, 10)
		}
		parent.Lock()
		err = parent.ApplyMountIntentLocked(contract.LocalCallers{source.authority: i.Actor}, contract.OperationID("blocked-"+string(rune(kind))), MountBinding{Actor: i.Actor, Command: command})
		parent.Unlock()
		if !errors.Is(err, contract.ErrMountBusy) {
			t.Fatal("control not serialized", kind, err)
		}
	}
	other := i
	other.Operation = "another-carrier"
	if _, err = reserveCarrier(t, parent, other); !errors.Is(err, contract.ErrMountBusy) {
		t.Fatal("second carrier held same slot", err)
	}
	a, _ := parent.state.elements.Anchor(c.Reservation.Tuple.Parent)
	if _, err = applyFoundationStore(parent, elementfact.OperationUpdateAnchor, i.Actor, a.ID, foundationStoreTuple(t, a.Parent, 1, 100), a.Revision, "blocked-ancestor"); !errors.Is(err, contract.ErrMountBusy) {
		t.Fatal("ancestor moved", err)
	}
	root := rootTransferIntent(t, parent, target, "blocked-root-transfer")
	if _, err = beginTransfer(t, parent, root); err == nil {
		t.Fatal("root transfer bypassed replacement")
	}
	if _, err = applyFoundationStore(parent, elementfact.OperationCreateEntity, i.Actor, foundationStoreID(109), foundationStoreTuple(t, a.ID, 40, 1), contract.Revision{}, "unrelated-child"); err != nil {
		t.Fatal("unrelated fact was blocked", err)
	}
	if _, err = advanceCarrier(t, parent, c, false); err != nil {
		t.Fatal(err)
	}
	if _, err = applyFoundationStore(parent, elementfact.OperationUpdateAnchor, i.Actor, a.ID, foundationStoreTuple(t, a.Parent, 1, 100), a.Revision, "after-cancel"); err != nil {
		t.Fatal("cancellation did not release ancestors", err)
	}
}

func TestCarrierContactRepairDoesNotBypassPreparation(t *testing.T) {
	parent, _, _, i := carrierControlFixture(t)
	c, err := reserveCarrier(t, parent, i)
	if err != nil {
		t.Fatal(err)
	}
	m := parent.state.mount.facts.Record(i.Mount)
	parent.Lock()
	_, err = parent.ApplyMountAddressLocked("repair-carrier", NextHopBinding{Actor: i.Actor, Element: i.Mount, Key: m.Key, Expected: m.Contact.Revision, NextHop: "local:carrier-repaired"})
	parent.Unlock()
	if err != nil {
		t.Fatal("slot blocked address repair", err)
	}
	if _, err = advanceCarrier(t, parent, c, true); !errors.Is(err, contract.ErrRevisionConflict) {
		t.Fatal("old contact admitted a new fence permit", err)
	}
	if _, err = advanceCarrier(t, parent, c, false); err != nil {
		t.Fatal(err)
	}
	i.Operation = "repaired-reservation"
	i.SourceAddress = "local:carrier-repaired"
	i.ExpectedMount = parent.state.mount.facts.Record(i.Mount).Revision
	c, err = reserveCarrier(t, parent, i)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = advanceCarrier(t, parent, c, true); err != nil {
		t.Fatal(err)
	}
	m = parent.state.mount.facts.Record(i.Mount)
	parent.Lock()
	_, err = parent.ApplyMountAddressLocked("repair-after-permit", NextHopBinding{Actor: i.Actor, Element: i.Mount, Key: m.Key, Expected: m.Contact.Revision, NextHop: "local:carrier-again"})
	parent.Unlock()
	if err != nil {
		t.Fatal("permit blocked repair", err)
	}
	path, before := parent.store.db.Path(), parent.state.clone()
	closeFoundationStore(t, parent)
	o := openFoundationStore(t, path)
	defer o.CloseStore()
	if !sameCompleteCoreState(before, o.state) {
		t.Fatal("mixed mount-address/transfer replay diverged")
	}
}

func TestCarrierOperationCannotBeReplayedAsOrdinaryMount(t *testing.T) {
	parent, _, _, i := carrierControlFixture(t)
	if _, err := reserveCarrier(t, parent, i); err != nil {
		t.Fatal(err)
	}
	before := parent.state.clone()
	parent.Lock()
	err := parent.ApplyMountIntentLocked(nil, i.Operation, MountBinding{})
	parent.Unlock()
	if !errors.Is(err, contract.ErrOperationConflict) {
		t.Fatal("empty ordinary binding replayed carrier intent", err)
	}
	for _, kind := range []MountEventKind{MountEventDecide, MountEventSettle} {
		parent.Lock()
		err = parent.ApplyMountEventLocked(MountEvent{Kind: kind, Operation: i.Operation, Authorized: kind == MountEventDecide})
		parent.Unlock()
		if !errors.Is(err, contract.ErrConflict) {
			t.Fatal("ordinary mount step consumed carrier record", kind, err)
		}
	}
	if !sameCompleteCoreState(before, parent.state) {
		t.Fatal("wrong control variant changed truth")
	}
}
