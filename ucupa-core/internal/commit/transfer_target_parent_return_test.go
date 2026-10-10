package commit

import (
	"testing"

	"ucupa/ucupa-core/contract"
	transferfact "ucupa/ucupa-core/internal/transfer/fact"
)

// A mounted carrier exports a slice, then receives that same UUID back under
// its actual upstream relationship. Both directions use real settled journals.
func TestTransferMountedTargetReturnKeepsUUIDLineage(t *testing.T) {
	p, a, b, pi := sourceParentFixture(t, "entity")
	first, err := reserveTransferParent(p, pi)
	if err != nil {
		t.Fatal(err)
	}
	fenced, err := fenceWithParent(a, first)
	if err != nil {
		t.Fatal(err)
	}
	i := pi.Initial.Input
	f, raw, err := a.TransferSourceFenceLocked(i.Actor, i.Operation)
	if err != nil {
		t.Fatal(err)
	}
	accepted, err := acceptTransfer(t, b, f, raw)
	if err != nil {
		t.Fatal(err)
	}
	d, err := commitSuccessor(a, accepted, fenced.Revision)
	if err != nil {
		t.Fatal(err)
	}
	installed, err := installTransfer(b, d)
	if err != nil {
		t.Fatal(err)
	}
	settled, err := settleTransfer(a, installed)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := releaseTransferParent(p, first, transferfact.ParentRelease{Settled: settled}); err != nil {
		t.Fatal(err)
	}
	oldDeparture, _ := a.state.elements.Departure(i.Boundary)
	oldResult := a.state.transfers.records[i.Operation]
	e, _ := b.state.elements.Entity(i.Boundary)
	back := transferfact.Intent{Profile: transferfact.SliceTransfer, Operation: "return-to-mounted-core", Actor: i.Actor,
		Source: b.authority, Target: a.authority, Boundary: i.Boundary, Expected: e.Revision,
		Final: foundationStoreTuple(t, pi.Mount, 2, 1), SourceAddress: i.TargetAddress, TargetAddress: i.SourceAddress}
	initial, err := beginTransfer(t, b, back)
	if err != nil {
		t.Fatal(err)
	}
	pi.Initial, pi.Role = initial, transferfact.TargetParent
	pi.ExpectedMount = p.state.mount.facts.Record(pi.Mount).Revision
	second, err := reserveTransferParent(p, pi)
	if err != nil {
		t.Fatal(err)
	}
	backFenced, err := fenceTransfer(b, back, initial.Revision)
	if err != nil {
		t.Fatal(err)
	}
	f, raw, err = b.TransferSourceFenceLocked(back.Actor, back.Operation)
	if err != nil {
		t.Fatal(err)
	}
	accepted, err = acceptWithParent(a, f, raw, second)
	if err != nil {
		t.Fatal("returned UUID with exact lineage refused", err)
	}
	d, err = commitSuccessor(b, accepted, backFenced.Revision)
	if err != nil {
		t.Fatal(err)
	}
	installed, err = installTransfer(a, d)
	if err != nil {
		t.Fatal(err)
	}
	settled, err = settleTransfer(b, installed)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := releaseTransferParent(p, second, transferfact.ParentRelease{Settled: settled}); err != nil {
		t.Fatal(err)
	}
	current, exists := a.state.elements.Entity(back.Boundary)
	if !exists || current.Parent != back.Final.Parent || current.LocalPose != back.Final.LocalPose || current.Range != back.Final.Range || current.Revision == back.Expected ||
		!a.state.elements.Claimed(back.Boundary) || oldDeparture.Operation == "" || a.state.transfers.records[i.Operation] != oldResult {
		t.Fatal("return changed identity, old result or final placement")
	}
	if _, departed := a.state.elements.Departure(back.Boundary); departed {
		t.Fatal("returned generation still inactive")
	}
	path, before := a.store.db.Path(), a.state.clone()
	closeFoundationStore(t, a)
	a = openFoundationStore(t, path)
	defer closeFoundationStore(t, a)
	if !sameCompleteCoreState(before, a.state) {
		t.Fatal("return restart lost lineage")
	}
	if original, err := fenceWithParent(a, first); err != nil || original != fenced {
		t.Fatal("old fence retry re-executed against new generation", err)
	}
	if retry, err := acceptWithParent(a, f, raw, second); err != nil || retry != accepted {
		t.Fatal("return acceptance not retained", err)
	}
	if current.Parent == "" || current.Revision == (contract.Revision{}) {
		t.Fatal("bounded return became a root")
	}
}
