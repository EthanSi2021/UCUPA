package commit

import (
	"errors"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	elementfact "ucupa/ucupa-core/internal/element/fact"
	"ucupa/ucupa-core/internal/token"
	transferencoding "ucupa/ucupa-core/internal/transfer/encoding"
	transferfact "ucupa/ucupa-core/internal/transfer/fact"
)

func transferReceiver(t *testing.T, root values.ElementID) *Owner {
	t.Helper()
	o := createFoundationStore(t, filepath.Join(t.TempDir(), "receiver.db"))
	t.Cleanup(func() { _ = o.CloseStore() })
	if _, err := applyFoundationStore(o, elementfact.OperationCreateRoot, foundationStoreActor, root, contract.SpatialTuple{}, contract.Revision{}, "root"); err != nil {
		t.Fatal(err)
	}
	return o
}

func outgoingTransfer(t *testing.T, source, target *Owner, boundary, parent values.ElementID, operation contract.OperationID) (transferfact.SourceFence, string) {
	t.Helper()
	var revision contract.Revision
	if a, ok := source.state.elements.Anchor(boundary); ok {
		revision = a.Revision
	} else {
		e, _ := source.state.elements.Entity(boundary)
		revision = e.Revision
	}
	i := transferfact.Intent{Profile: transferfact.SliceTransfer, Operation: operation, Actor: foundationStoreActor, Source: source.authority, Target: target.authority, Boundary: boundary, Expected: revision,
		Final: foundationStoreTuple(t, parent, 0, 20), SourceAddress: values.NextHop("local:" + string(source.authority)), TargetAddress: values.NextHop("local:" + string(target.authority))}
	return frozenSource(t, source, i)
}

func completeFencedTransfer(t *testing.T, source, target *Owner, f transferfact.SourceFence, raw string) transferfact.TargetInstallation {
	t.Helper()
	a, err := acceptTransfer(t, target, f, raw)
	if err != nil {
		t.Fatal("accept", err)
	}
	d, err := commitSuccessor(source, a, source.state.transfers.records[f.Input.Operation].FenceRevision)
	if err != nil {
		t.Fatal("decide", err)
	}
	installed, err := installTransfer(target, d)
	if err != nil {
		t.Fatal("install", err)
	}
	if _, err := settleTransfer(source, installed); err != nil {
		t.Fatal("settle", err)
	}
	return installed
}

func TestTransferReturnMultipleOwnersAndRepeatedCycles(t *testing.T) {
	a, _, initial := transferIntentFixture(t)
	b, c := transferReceiver(t, foundationStoreID(999)), transferReceiver(t, foundationStoreID(1999))
	root, _ := a.state.elements.ParentOf(initial.Boundary)
	oldView, oldCut, oldOperations := a.state.elements.View, a.state.currentCut(), a.state.clone().operations
	steps := []struct {
		source, target *Owner
		parent         values.ElementID
		operation      contract.OperationID
	}{
		{a, b, foundationStoreID(999), contract.OperationID(string([]byte{'a', 0xff, 0}))},
		{b, c, foundationStoreID(1999), "b-c"},
		{c, a, root, "c-a"},
		{a, b, foundationStoreID(999), "a-b-again"},
		{b, a, root, "b-a-again"},
	}
	var first transferfact.TargetInstallation
	for index, step := range steps {
		f, raw := outgoingTransfer(t, step.source, step.target, initial.Boundary, step.parent, step.operation)
		slice, err := transferencoding.DecodeTransferManifest(f, raw)
		if err != nil {
			t.Fatal("lineage decode", err)
		}
		for _, member := range slice.Members() {
			if len(slice.Lineage(member.ID())) != index {
				t.Fatal("chain not preserved", index, member.ID())
			}
		}
		installed := completeFencedTransfer(t, step.source, step.target, f, raw)
		if index == 0 {
			first = installed
		}
		for _, member := range slice.Members() {
			if step.source.state.elements.Kind(member.ID()) != 0 || step.target.state.elements.Kind(member.ID()) != member.Kind {
				t.Fatal("multiple active locations")
			}
			if _, gone := step.target.state.elements.Departure(member.ID()); gone {
				t.Fatal("returned UUID remains departed")
			}
		}
		if err := validateCoreState(step.source.state); err != nil {
			t.Fatal("source replay", err)
		}
		if err := validateCoreState(step.target.state); err != nil {
			t.Fatal("target replay", err)
		}
	}
	if !reflect.DeepEqual(oldOperations, a.state.operations) {
		t.Fatal("original source outcomes changed")
	}
	if err := a.CheckTransferReadLocked(oldView, oldCut, root, foundationStoreID(102)); !errors.Is(err, contract.ErrReadCutExpired) {
		t.Fatal("returned UUID revived old point snapshot", err)
	}
	if err := a.CheckTransferReadLocked(oldView, oldCut, root, ""); !errors.Is(err, contract.ErrReadCutExpired) {
		t.Fatal("returned UUID revived old scope", err)
	}
	if err := a.CheckTransferReadLocked(a.state.elements.View, a.state.currentCut(), root, foundationStoreID(102)); err != nil {
		t.Fatal("new generation unreadable", err)
	}
	// The old target now has a departure. Replaying its first installed result
	// must return that original result, not install an old generation again.
	again, err := installTransfer(b, first.Decision)
	if err != nil || again != first || b.state.elements.Kind(initial.Boundary) != 0 {
		t.Fatal("old installation revived UUID", err)
	}
	for _, owner := range []*Owner{a, b, c} {
		path, before := owner.store.db.Path(), owner.state.clone()
		closeFoundationStore(t, owner)
		reopened := openFoundationStore(t, path)
		if !sameCompleteCoreState(before, reopened.state) {
			t.Fatal("restart changed custody")
		}
		_ = reopened.CloseStore()
	}
}

func TestTransferReturnSubsetDoesNotExportUnrelatedHistory(t *testing.T) {
	a, _, initial := transferIntentFixture(t)
	b := transferReceiver(t, foundationStoreID(999))
	root, _ := a.state.elements.ParentOf(initial.Boundary)
	f, raw := outgoingTransfer(t, a, b, initial.Boundary, foundationStoreID(999), "a-b")
	completeFencedTransfer(t, a, b, f, raw)
	// Return just one former child, not the entire earlier Anchor closure.
	f, raw = outgoingTransfer(t, b, a, foundationStoreID(102), root, "child-return")
	slice, err := transferencoding.DecodeTransferManifest(f, raw)
	if err != nil || len(slice.Members()) != 1 {
		t.Fatal(err)
	}
	if strings.Contains(raw, string(root)) {
		t.Fatal("exported former source ancestor outside the current selected proof")
	}
	chain := slice.Lineage(f.Input.Boundary)
	if len(chain) != 1 || chain[0].SourceRevision == initial.Expected {
		t.Fatal("lineage did not bind exact member")
	}
	completeFencedTransfer(t, b, a, f, raw)
	if a.state.elements.Kind(initial.Boundary) != 0 || b.state.elements.Kind(initial.Boundary) != contract.ElementAnchor || a.state.elements.Kind(f.Input.Boundary) != contract.ElementEntity {
		t.Fatal("subset was widened")
	}
	if !a.state.elements.Claimed(initial.Boundary) || !b.state.elements.Claimed(f.Input.Boundary) {
		t.Fatal("historical claim erased")
	}
	if e, _ := a.state.elements.Entity(f.Input.Boundary); token.RevisionScope(e.Revision) != a.state.scope {
		t.Fatal("foreign CAS installed")
	}
}

func TestTransferReturnCanIncludeFreshDescendant(t *testing.T) {
	a, _, i := transferIntentFixture(t)
	b := transferReceiver(t, foundationStoreID(999))
	f, raw := outgoingTransfer(t, a, b, i.Boundary, foundationStoreID(999), "out")
	completeFencedTransfer(t, a, b, f, raw)
	id := foundationStoreID(103)
	out, err := applyFoundationStore(b, elementfact.OperationCreateEntity, i.Actor, id, foundationStoreTuple(t, i.Boundary, 0, 1), contract.Revision{}, "new-descendant")
	if err != nil || out.Status != contract.MutationCommitted {
		t.Fatal(err)
	}
	f, raw = outgoingTransfer(t, b, a, i.Boundary, foundationStoreID(100), "return-with-new-child")
	slice, err := transferencoding.DecodeTransferManifest(f, raw)
	if err != nil || len(slice.Members()) != 3 || len(slice.Lineage(id)) != 0 || len(slice.Lineage(i.Boundary)) != 1 {
		t.Fatal("mixed origins were flattened", err)
	}
	completeFencedTransfer(t, b, a, f, raw)
	if entity, ok := a.state.elements.Entity(id); !ok || entity.Parent != i.Boundary || token.RevisionScope(entity.Revision) != a.state.scope {
		t.Fatal("fresh descendant did not join returned closure")
	}
}
