package commit

import (
	"testing"

	"ucupa/ucupa-core/contract"
	elementfact "ucupa/ucupa-core/internal/element/fact"
	transferfact "ucupa/ucupa-core/internal/transfer/fact"
)

// Two already-mounted sibling carriers share one real parent. Independent
// role/mount slots retain both dependencies for one source-controlled transfer.
func TestTransferBetweenMountedCoresUsesBothParentHolds(t *testing.T) {
	p, source, target, carrier := carrierControlFixture(t)
	targetMount := foundationStoreID(300)
	if _, err := applyFoundationStore(p, elementfact.OperationCreateAnchor, foundationStoreActor, targetMount,
		foundationStoreTuple(t, foundationStoreID(100), 50, 20), contract.Revision{}, "second-carrier-mount"); err != nil {
		t.Fatal(err)
	}
	establishEdge(t, p, target, targetMount)
	entity, _ := source.state.elements.Entity(foundationStoreID(105))
	i := transferfact.Intent{Profile: transferfact.SliceTransfer, Operation: "between-carriers", Actor: foundationStoreActor,
		Source: source.authority, Target: target.authority, Boundary: entity.ID, Expected: entity.Revision,
		Final: foundationStoreTuple(t, targetMount, 4, 1), SourceAddress: carrier.SourceAddress, TargetAddress: "local:nested-child"}
	initial, err := beginTransfer(t, source, i)
	if err != nil {
		t.Fatal(err)
	}
	from := transferfact.ParentIntent{Initial: initial, Role: transferfact.SourceParent, Parent: p.authority, Mount: carrier.Mount,
		ExpectedElement: carrier.ExpectedElement, ExpectedMount: carrier.ExpectedMount, ParentAddress: carrier.ParentAddress}
	a, _ := p.state.elements.Anchor(targetMount)
	to := transferfact.ParentIntent{Initial: initial, Role: transferfact.TargetParent, Parent: p.authority, Mount: targetMount,
		ExpectedElement: a.Revision, ExpectedMount: p.state.mount.facts.Record(targetMount).Revision, ParentAddress: "local:carrier-old"}
	h1, err := reserveTransferParent(p, from)
	if err != nil {
		t.Fatal(err)
	}
	h2, err := reserveTransferParent(p, to)
	if err != nil || h1.Input.Slot() == h2.Input.Slot() {
		t.Fatal("parent could not retain two distinct relations", err)
	}
	fenced, err := fenceWithParent(source, h1)
	if err != nil {
		t.Fatal(err)
	}
	f, raw, err := source.TransferSourceFenceLocked(i.Actor, i.Operation)
	if err != nil {
		t.Fatal(err)
	}
	accepted, err := acceptWithParent(target, f, raw, h2)
	if err != nil {
		t.Fatal(err)
	}
	d, err := commitSuccessor(source, accepted, fenced.Revision)
	if err != nil {
		t.Fatal(err)
	}
	installed, err := installTransfer(target, d)
	if err != nil {
		t.Fatal(err)
	}
	settled, err := settleTransfer(source, installed)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := releaseTransferParent(p, h1, transferfact.ParentRelease{Settled: settled}); err != nil {
		t.Fatal(err)
	}
	if p.state.mount.facts.Record(targetMount).Slot != to.Slot() {
		t.Fatal("releasing source relation also released target relation")
	}
	if _, err := releaseTransferParent(p, h2, transferfact.ParentRelease{Settled: settled}); err != nil {
		t.Fatal(err)
	}
	current, exists := target.state.elements.Entity(entity.ID)
	if !exists || current.Parent != targetMount || current.LocalPose != i.Final.LocalPose || source.state.elements.Kind(entity.ID) != 0 {
		t.Fatal("final placement changed or source remained active")
	}
	for _, o := range []*Owner{p, source, target} {
		path, before := o.store.db.Path(), o.state.clone()
		closeFoundationStore(t, o)
		reopened := openFoundationStore(t, path)
		if !sameCompleteCoreState(before, reopened.state) {
			t.Fatal("restart changed two-sided parent holds")
		}
		closeFoundationStore(t, reopened)
	}
}
