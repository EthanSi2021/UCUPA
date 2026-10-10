package commit

import (
	"errors"
	"testing"

	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	elementfact "ucupa/ucupa-core/internal/element/fact"
	transferfact "ucupa/ucupa-core/internal/transfer/fact"
)

// The target is an actual mounted carrier. Initial is read from a real source
// intent; these tests do not fabricate authorization, accept or install it.
func transferParentFixture(t *testing.T) (*Owner, *Owner, *Owner, transferfact.ParentIntent) {
	t.Helper()
	p, target, source, carrier := carrierControlFixture(t)
	root, entity := foundationStoreID(200), foundationStoreID(201)
	if _, err := applyFoundationStore(source, elementfact.OperationCreateRoot, foundationStoreActor, root, contract.SpatialTuple{}, contract.Revision{}, "parent-source-root"); err != nil {
		t.Fatal(err)
	}
	if _, err := applyFoundationStore(source, elementfact.OperationCreateEntity, foundationStoreActor, entity, foundationStoreTuple(t, root, 0, 1), contract.Revision{}, "parent-source-entity"); err != nil {
		t.Fatal(err)
	}
	e, _ := source.state.elements.Entity(entity)
	i := transferfact.Intent{Profile: transferfact.SliceTransfer, Operation: "parent-held-slice", Actor: foundationStoreActor, Source: source.authority,
		Target: target.authority, Boundary: entity, Expected: e.Revision, Final: foundationStoreTuple(t, carrier.Mount, 3, 1), SourceAddress: "local:source", TargetAddress: carrier.SourceAddress}
	r, err := beginTransfer(t, source, i)
	if err != nil {
		t.Fatal(err)
	}
	return p, source, target, transferfact.ParentIntent{Initial: r, Role: transferfact.TargetParent, Parent: p.authority,
		Mount: carrier.Mount, ExpectedElement: carrier.ExpectedElement, ExpectedMount: carrier.ExpectedMount, ParentAddress: carrier.ParentAddress}
}

func reserveTransferParent(o *Owner, i transferfact.ParentIntent) (transferfact.ParentHold, error) {
	o.Lock()
	defer o.Unlock()
	return o.ReserveTransferParentLocked(i)
}

func releaseTransferParent(o *Owner, h transferfact.ParentHold, r transferfact.ParentRelease) (transferfact.ParentControl, error) {
	o.Lock()
	defer o.Unlock()
	return o.ReleaseTransferParentLocked(h, r)
}

func TestTransferParentHoldProtectsMatchingMountAndAncestors(t *testing.T) {
	p, s, target, i := transferParentFixture(t)
	before, sourceBefore, targetBefore := p.state.clone(), s.state.clone(), target.state.clone()
	h, err := reserveTransferParent(p, i)
	if err != nil || !h.Valid() {
		t.Fatal("parent hold", err)
	}
	a, _ := p.state.elements.Anchor(i.Mount)
	original, _ := before.elements.Anchor(i.Mount)
	r := p.state.mount.facts.Record(i.Mount)
	read, valid := p.CaptureReadLocked()
	if !valid || read.Validate() != nil || !read.Projection.Transfers.OutgoingReadable(i.Mount, r) || read.Projection.Mount.Record(i.Mount) != r {
		t.Fatal("held parent read proof was absent or rewrote raw mount facts")
	}
	if a != original || r.Admission != before.mount.facts.Record(i.Mount).Admission || r.Slot != i.Slot() ||
		!sameCompleteCoreState(sourceBefore, s.state) || !sameCompleteCoreState(targetBefore, target.state) {
		t.Fatal("hold changed spatial facts, binding or a different owner")
	}
	if _, found := p.MountOperationLocked(i.Slot()); found {
		t.Fatal("ordinary mount API disclosed transfer control")
	}
	if pending, _ := p.PendingMountsLocked(); len(pending) != 0 {
		t.Fatal("ordinary mount recovery picked parent hold")
	}
	for _, kind := range []contract.MountCommandKind{contract.MountUpdate, contract.MountRevoke} {
		command := contract.MountCommand{Kind: kind, MountAnchorUUID: i.Mount, ExpectedMountRevision: r.Revision}
		if kind == contract.MountUpdate {
			command.ExpectedElementRevision, command.Final = a.Revision, foundationStoreTuple(t, a.Parent, 0, 10)
		}
		p.Lock()
		err := p.ApplyMountIntentLocked(contract.LocalCallers{target.authority: i.Initial.Input.Actor}, contract.OperationID("hold-blocked-"+string(rune(kind))), MountBinding{Actor: i.Initial.Input.Actor, Command: command})
		p.Unlock()
		if !errors.Is(err, contract.ErrMountBusy) {
			t.Fatal("mount control ignored hold", kind, err)
		}
	}
	ancestor, _ := p.state.elements.Anchor(a.Parent)
	if _, err := applyFoundationStore(p, elementfact.OperationUpdateAnchor, foundationStoreActor, ancestor.ID,
		foundationStoreTuple(t, ancestor.Parent, 1, 100), ancestor.Revision, "hold-ancestor"); !errors.Is(err, contract.ErrMountBusy) {
		t.Fatal("ancestor recipe escaped hold", err)
	}
	if _, err := beginTransfer(t, p, rootTransferIntent(t, p, s, "held-root")); !errors.Is(err, contract.ErrMountBusy) {
		t.Fatal("root transfer escaped hold", err)
	}
	if _, err := applyFoundationStore(p, elementfact.OperationCreateEntity, foundationStoreActor, foundationStoreID(109),
		foundationStoreTuple(t, ancestor.ID, 40, 1), contract.Revision{}, "hold-unrelated"); err != nil {
		t.Fatal("unrelated content stopped", err)
	}
	// Repair is independent of the control slot and cannot drop the hold.
	p.Lock()
	_, err = p.ApplyMountAddressLocked("hold-address", NextHopBinding{Actor: foundationStoreActor, Element: i.Mount,
		Key: r.Key, Expected: r.Contact.Revision, NextHop: "local:repaired"})
	p.Unlock()
	if err != nil || p.state.mount.facts.Record(i.Mount).Slot != i.Slot() {
		t.Fatal("contact repair lost or was blocked by hold", err)
	}
	repaired := p.state.mount.facts.Record(i.Mount)
	if read.Projection.Transfers.OutgoingReadable(i.Mount, repaired) || !p.SourceVisibilityLocked().OutgoingReadable(i.Mount, repaired) {
		t.Fatal("repair was not distinguished from original held contact")
	}
	if _, err := releaseTransferParent(p, h, transferfact.ParentRelease{}); !errors.Is(err, contract.ErrInvalid) {
		t.Fatal("silence released parent", err)
	}
	cancelled, err := cancelTransfer(t, s, i.Initial.Input, i.Initial.Revision)
	if err != nil {
		t.Fatal(err)
	}
	terminal := transferfact.ParentRelease{Cancelled: cancelled}
	released, err := releaseTransferParent(p, h, terminal)
	if err != nil || !released.Valid() || p.state.mount.facts.Record(i.Mount).Slot != "" {
		t.Fatal("cancel proof did not release parent", err)
	}
	if _, err := applyFoundationStore(p, elementfact.OperationUpdateAnchor, foundationStoreActor, ancestor.ID,
		foundationStoreTuple(t, ancestor.Parent, 1, 100), ancestor.Revision, "hold-released-ancestor"); err != nil {
		t.Fatal("release kept ancestor blocked", err)
	}
	path, durable := p.store.db.Path(), p.state.clone()
	closeFoundationStore(t, p)
	p = openFoundationStore(t, path)
	defer closeFoundationStore(t, p)
	if !sameCompleteCoreState(durable, p.state) {
		t.Fatal("reopen changed hold/release")
	}
	if _, _, held := p.SourceVisibilityLocked().Counts(); held != 0 {
		t.Fatal("released hold reappeared as read evidence")
	}
	if got, err := reserveTransferParent(p, i); err != nil || got != h {
		t.Fatal("old reserve recreated a slot", err)
	}
	if got, err := releaseTransferParent(p, h, terminal); err != nil || got != released || !sameCompleteCoreState(durable, p.state) {
		t.Fatal("terminal exact retry changed history", err)
	}
}

func TestTransferParentInputConflictAndDisclosure(t *testing.T) {
	p, _, _, i := transferParentFixture(t)
	h, err := reserveTransferParent(p, i)
	if err != nil {
		t.Fatal(err)
	}
	before := p.state.clone()
	changed := i
	changed.Initial.Input.Final = foundationStoreTuple(t, i.Mount, 4, 1)
	if _, err := reserveTransferParent(p, changed); !errors.Is(err, contract.ErrOperationConflict) {
		t.Fatal("same operation was rebound", err)
	}
	p.Lock()
	_, err = p.TransferParentLocked(values.PrincipalRef{Realm: "other", Principal: "other"}, i.Slot())
	p.Unlock()
	if !errors.Is(err, values.PublicDenied) || !sameCompleteCoreState(before, p.state) {
		t.Fatal("foreign principal inspected/mutated hold", err)
	}
	changed = i
	changed.Initial.Input.Operation = "other-transfer"
	if _, err := reserveTransferParent(p, changed); !errors.Is(err, contract.ErrMountBusy) {
		t.Fatal("second operation acquired occupied slot", err)
	}
	bad := h
	bad.Revision++
	terminal := transferfact.ParentRelease{Cancelled: i.Initial}
	terminal.Cancelled.Progress, terminal.Cancelled.Revision = transferfact.IntentCancelled, i.Initial.Revision+1
	if _, err := releaseTransferParent(p, bad, terminal); !errors.Is(err, contract.ErrConflict) || !sameCompleteCoreState(before, p.state) {
		t.Fatal("changed hold released original", err)
	}
}
