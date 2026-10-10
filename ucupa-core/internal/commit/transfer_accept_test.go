package commit

import (
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	elementfact "ucupa/ucupa-core/internal/element/fact"
	transferencoding "ucupa/ucupa-core/internal/transfer/encoding"
	transferfact "ucupa/ucupa-core/internal/transfer/fact"
)

func transferTargetFixture(t *testing.T) (*Owner, string, transferfact.SourceFence, string) {
	t.Helper()
	source, _, input := transferIntentFixture(t)
	path := filepath.Join(t.TempDir(), "target.db")
	target := createFoundationStore(t, path)
	t.Cleanup(func() { _ = target.CloseStore() })
	root, parent := foundationStoreID(998), foundationStoreID(999)
	if _, err := applyFoundationStore(target, elementfact.OperationCreateRoot, input.Actor, root, contract.SpatialTuple{}, contract.Revision{}, "target-root"); err != nil {
		t.Fatal(err)
	}
	if _, err := applyFoundationStore(target, elementfact.OperationCreateAnchor, input.Actor, parent, foundationStoreTuple(t, root, 0, 100), contract.Revision{}, "target-parent"); err != nil {
		t.Fatal(err)
	}
	input.Target = target.authority
	f, raw := frozenSource(t, source, input)
	return target, path, f, raw
}

func acceptTransfer(t *testing.T, o *Owner, f transferfact.SourceFence, raw string) (transferfact.TargetAcceptance, error) {
	t.Helper()
	o.Lock()
	defer o.Unlock()
	return o.AcceptTransferTargetLocked(f.Input.Actor, f, raw, nil, transferfact.ParentHold{})
}

func TestTransferTargetAcceptanceDurablePrivateAndIdempotent(t *testing.T) {
	o, path, f, raw := transferTargetFixture(t)
	before := o.state.clone()
	r, err := acceptTransfer(t, o, f, raw)
	if err != nil || r.Fence != f || r.At != before.currentCut() || r.Revision != 1 {
		t.Fatalf("accept: %+v %v", r, err)
	}
	if !sameCoreFacts(before, o.state) || o.state.elements.Claimed(f.Input.Boundary) || len(o.state.operations) != len(before.operations) || len(o.state.commits) != len(before.commits) {
		t.Fatal("acceptance activated facts or changed ordinary outcomes")
	}
	if err := validateCoreState(o.state); err != nil {
		t.Fatal(err)
	}
	// Private target obligations do not turn old valid absence into source
	// unavailability or invalidate an unchanged ordinary target query snapshot.
	root, _ := before.elements.ParentOf(f.Input.Final.Parent)
	o.RLock()
	readErr := o.CheckTransferReadLocked(before.elements.View, before.currentCut(), root, f.Input.Boundary)
	scopeErr := o.CheckTransferReadLocked(before.elements.View, before.currentCut(), root, "")
	visibility := o.SourceVisibilityLocked()
	o.RUnlock()
	if readErr != nil || scopeErr != nil || visibility.Inactive(f.Input.Boundary) {
		t.Fatal("private acceptance leaked into source read effect", readErr, scopeErr)
	}
	for n := 0; n < 2; n++ {
		again, err := acceptTransfer(t, o, f, raw)
		if err != nil || again != r || len(o.state.transfers.events) != 1 {
			t.Fatal("retry changed acceptance", err)
		}
	}
	if _, err := acceptTransfer(t, o, f, raw+" "); !errors.Is(err, contract.ErrOperationConflict) {
		t.Fatal("different retry bytes", err)
	}
	durable := o.state.clone()
	closeFoundationStore(t, o)
	o = openFoundationStore(t, path)
	defer o.CloseStore()
	if !sameCompleteCoreState(durable, o.state) {
		t.Fatal("reopen lost target obligations")
	}
	again, err := acceptTransfer(t, o, f, raw)
	if err != nil || again != r {
		t.Fatal("restart retry", err)
	}
	for _, id := range []values.ElementID{f.Input.Boundary, foundationStoreID(102)} {
		_, err := applyFoundationStore(o, elementfact.OperationCreateEntity, f.Input.Actor, id, foundationStoreTuple(t, f.Input.Final.Parent, 0, 1), contract.Revision{}, contract.OperationID("steal-"+string(id)))
		if !errors.Is(err, contract.ErrConflict) {
			t.Fatalf("reserved UUID %s: %v", id, err)
		}
	}
	parent, _ := o.state.elements.Anchor(f.Input.Final.Parent)
	for _, kind := range []elementfact.OperationKind{elementfact.OperationUpdateAnchor, elementfact.OperationDeleteAnchor} {
		_, err := applyFoundationStore(o, kind, f.Input.Actor, parent.ID, foundationStoreTuple(t, parent.Parent, 1, 100), parent.Revision, "break-parent")
		if !errors.Is(err, contract.ErrConflict) {
			t.Fatalf("parent hold: %v", err)
		}
	}
	if _, err := applyFoundationStore(o, elementfact.OperationCreateEntity, f.Input.Actor, foundationStoreID(1001), foundationStoreTuple(t, parent.ID, 50, 1), contract.Revision{}, "unrelated"); err != nil {
		t.Fatal("unrelated sibling blocked", err)
	}
}

func TestTransferTargetAcceptanceFaultRecovery(t *testing.T) {
	for _, after := range []bool{false, true} {
		t.Run(map[bool]string{false: "rollback", true: "response-lost"}[after], func(t *testing.T) {
			o, path, f, raw := transferTargetFixture(t)
			before := o.state.clone()
			fail := func() error { return errors.New("target Store failure") }
			if after {
				o.store.failAfterCommit = fail
			} else {
				o.store.failBeforeCommit = fail
			}
			if _, err := acceptTransfer(t, o, f, raw); !errors.Is(err, contract.ErrStore) {
				t.Fatal(err)
			}
			if sameCompleteCoreState(before, o.state) == after || !o.AvailableLocked() {
				t.Fatal("wrong durable side")
			}
			closeFoundationStore(t, o)
			o = openFoundationStore(t, path)
			defer o.CloseStore()
			if len(o.state.transfers.targets) != map[bool]int{false: 0, true: 1}[after] {
				t.Fatal("wrong restored hold")
			}
			if _, err := acceptTransfer(t, o, f, raw); err != nil {
				t.Fatal(err)
			}
			if len(o.state.transfers.targets) != 1 || len(o.state.transfers.events) != 1 {
				t.Fatal("duplicate acceptance")
			}
		})
	}
}

func TestConcurrentTransferTargetAcceptanceHasOneReceipt(t *testing.T) {
	o, _, f, raw := transferTargetFixture(t)
	var wg sync.WaitGroup
	receipts := make(chan transferfact.TargetAcceptance, 8)
	errs := make(chan error, 8)
	for n := 0; n < 8; n++ {
		wg.Add(1)
		go func() { defer wg.Done(); r, err := acceptTransfer(t, o, f, raw); receipts <- r; errs <- err }()
	}
	wg.Wait()
	close(receipts)
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	var first transferfact.TargetAcceptance
	for r := range receipts {
		if first.Revision == 0 {
			first = r
		}
		if first != r {
			t.Fatal("different accepted results")
		}
	}
	if len(o.state.transfers.events) != 1 {
		t.Fatal("duplicated durable acceptance")
	}
}

func TestDisjointIncomingTransfersShareParentAndQualifyForeignOperations(t *testing.T) {
	o, path, first, raw := transferTargetFixture(t)
	if _, err := acceptTransfer(t, o, first, raw); err != nil {
		t.Fatal(err)
	}
	// A separate source owns disjoint UUIDs but uses the same operation bytes.
	source := createFoundationStore(t, filepath.Join(t.TempDir(), "second-source.db"))
	defer source.CloseStore()
	root, child := foundationStoreID(2100), foundationStoreID(2101)
	if _, err := applyFoundationStore(source, elementfact.OperationCreateRoot, first.Input.Actor, root, contract.SpatialTuple{}, contract.Revision{}, "root"); err != nil {
		t.Fatal(err)
	}
	result, err := applyFoundationStore(source, elementfact.OperationCreateEntity, first.Input.Actor, child, foundationStoreTuple(t, root, 0, 1), contract.Revision{}, "child")
	if err != nil {
		t.Fatal(err)
	}
	i := first.Input
	i.Source, i.Boundary, i.Expected = source.authority, child, result.Record.Entity.Revision
	i.SourceAddress = "local:second-source"
	second, data := frozenSource(t, source, i)
	// Target also acts as source for an unrelated child at the same parent. Its
	// unresolved source footprint must not block disjoint incoming placement.
	unrelated := foundationStoreID(2102)
	written, err := applyFoundationStore(o, elementfact.OperationCreateEntity, i.Actor, unrelated, foundationStoreTuple(t, i.Final.Parent, 50, 1), contract.Revision{}, "create-outgoing")
	if err != nil {
		t.Fatal(err)
	}
	out := i
	out.Operation, out.Source, out.Target = "outgoing-child", o.authority, source.authority
	out.Boundary, out.Expected = unrelated, written.Record.Entity.Revision
	out.SourceAddress, out.TargetAddress = "local:target", "local:second-source"
	out.Final = foundationStoreTuple(t, root, 50, 1)
	frozenSource(t, o, out)
	if _, err := acceptTransfer(t, o, second, data); err != nil {
		t.Fatal("disjoint incoming placement blocked", err)
	}
	if len(o.state.transfers.targets) != 2 || first.Key() == second.Key() {
		t.Fatal("foreign operation namespaces collapsed")
	}
	decoded, err := transferencoding.DecodeTransferManifest(second, data)
	if err != nil {
		t.Fatal(err)
	}
	o.RLock()
	_, err = o.PreflightTransferTargetLocked(decoded, second.Input.Final, 100)
	o.RUnlock()
	if !errors.Is(err, contract.ErrConflict) {
		t.Fatal("preflight ignored already reserved UUID", err)
	}
	durable := o.state.clone()
	closeFoundationStore(t, o)
	reopened := openFoundationStore(t, path)
	defer reopened.CloseStore()
	if !sameCompleteCoreState(durable, reopened.state) {
		t.Fatal("mixed source/target journal did not replay")
	}
}
