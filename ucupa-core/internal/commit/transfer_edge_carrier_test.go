package commit

import (
	"errors"
	"reflect"
	"testing"

	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	elementfact "ucupa/ucupa-core/internal/element/fact"
	mountfact "ucupa/ucupa-core/internal/mount/fact"
	transferfact "ucupa/ucupa-core/internal/transfer/fact"
)

// A real nested mount, not a fabricated Incoming value. The parent in this
// fixture is itself a carrier; moving its whole content must close the nested
// edge without moving the nested carrier's Element or changing the outer anchor.
func establishEdge(t *testing.T, parent, carrier *Owner, mount values.ElementID) {
	t.Helper()
	a, _ := parent.state.elements.Anchor(mount)
	op := contract.OperationID("external-mount")
	b := MountBinding{Actor: foundationStoreActor, PeerActor: foundationStoreActor, Command: contract.MountCommand{
		Kind: contract.MountEstablish, MountAnchorUUID: mount, ExpectedElementRevision: a.Revision,
		CarrierAuthority: carrier.authority, CarrierNextHop: "local:nested-child", ParentNextHop: "local:carrier-old"}}
	parent.Lock()
	err := parent.ApplyMountIntentLocked(contract.LocalCallers{carrier.authority: foundationStoreActor}, op, b)
	parent.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	request := parent.state.mount.operations[op].Request
	carrier.Lock()
	prepared, err := carrier.ApplyMountPrepareLocked(parent.authority, request, true)
	carrier.Unlock()
	if err != nil || !prepared.Prepared {
		t.Fatal("prepare nested mount", err)
	}
	parent.Lock()
	err = parent.ApplyMountEventLocked(MountEvent{Kind: MountEventDecide, Operation: op, Prepared: prepared, Authorized: true})
	parent.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	carrier.Lock()
	closed, err := carrier.ApplyMountFinishLocked(parent.authority, contract.MountFinishRequest{Evidence: parent.state.mount.operations[op].Evidence()})
	carrier.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	parent.Lock()
	err = parent.ApplyMountEventLocked(MountEvent{Kind: MountEventSettle, Operation: op, Closed: closed})
	parent.Unlock()
	if err != nil {
		t.Fatal(err)
	}
}

func TestTransferExternalEdgeInCarrierReplacementPreservesNestedContent(t *testing.T) {
	p, s, d, i := carrierControlFixture(t)
	c := emptyRootReceiver(t)
	mount, member := foundationStoreID(110), foundationStoreID(111)
	if _, err := applyFoundationStore(s, elementfact.OperationCreateAnchor, foundationStoreActor, mount,
		foundationStoreTuple(t, i.Mount, 0, 10), contract.Revision{}, "nested-anchor"); err != nil {
		t.Fatal(err)
	}
	establishEdge(t, s, c, mount)
	if _, err := applyFoundationStore(c, elementfact.OperationCreateEntity, foundationStoreActor, member,
		foundationStoreTuple(t, mount, 0, 1), contract.Revision{}, "nested-content"); err != nil {
		t.Fatal(err)
	}
	outer, _ := p.state.elements.Anchor(i.Mount)
	control, err := reserveCarrier(t, p, i)
	if err != nil {
		t.Fatal(err)
	}
	control, err = advanceCarrier(t, p, control, true)
	if err != nil {
		t.Fatal(err)
	}
	f, raw, err := fenceCarrier(s, control)
	if err != nil {
		t.Fatal(err)
	}
	edge := transferfact.EdgeFence{Carrier: f, Mount: mount}
	selected, err := decodeEdgeManifest(edge, raw)
	if err != nil || !selected.HasMounts() || len(selected.Members()) != 2 {
		t.Fatal("carrier closure", err)
	}
	for _, m := range selected.Members() {
		if m.ID() == member || m.ID() == i.Mount {
			t.Fatal("copied another owner's content")
		}
	}
	if _, err := acceptCarrier(d, f, raw); !errors.Is(err, contract.ErrMountCoordinationRequired) {
		t.Fatal("installed without nested continuation", err)
	}
	before := c.state.clone()
	closed, err := closeEdge(c, i.Actor, edge, raw)
	if err != nil || !closed.Valid(c.authority) || !reflect.DeepEqual(before.elements, c.state.elements) || c.state.mount.facts.Incoming().Gate != mountfact.GateReparenting {
		t.Fatal("nested edge closure", err)
	}
	if now, _ := p.state.elements.Anchor(i.Mount); now != outer {
		t.Fatal("moved outer anchor with carrier content")
	}
	beforeTarget := d.state.clone()
	if err := acceptClosedEdges(d, edge, raw, []transferfact.EdgeClosure{closed}); err != nil {
		t.Fatal("carrier target did not retain nested obligation", err)
	}
	accepted := d.state.transfers.targets[i.Key()]
	if len(accepted.closures) != 1 || accepted.closures[0] != closed || accepted.installed() ||
		d.state.currentCut() != beforeTarget.currentCut() || d.state.mount.facts.Incoming().Key.Valid() || d.state.elements.Counts() != beforeTarget.elements.Counts() {
		t.Fatal("private carrier acceptance activated content")
	}
	saved := d.state.clone()
	if err := acceptClosedEdges(d, edge, raw, []transferfact.EdgeClosure{closed}); err != nil || !sameCompleteCoreState(saved, d.state) {
		t.Fatal("nested acceptance retry", err)
	}
	binding, err := bindCarrier(p, accepted.carrier)
	if err != nil {
		t.Fatal(err)
	}
	installed, err := installCarrier(d, binding)
	if err != nil {
		t.Fatal("nested target installation", err)
	}
	ready := completeClosedEdge(t, d, c, transferfact.CoreInstallation{Carrier: installed}, closed)
	for _, role := range []bool{true, false} {
		o := s
		if role {
			o = p
		}
		o.Lock()
		if role {
			_, err = o.SettleCarrierParentLocked(ready)
		} else {
			_, err = o.SettleCarrierSourceLocked(ready)
		}
		o.Unlock()
		if err != nil {
			t.Fatal("nested carrier settlement", err)
		}
	}
	for _, o := range []*Owner{p, s, c, d} {
		path, saved := o.store.db.Path(), o.state.clone()
		closeFoundationStore(t, o)
		reopened := openFoundationStore(t, path)
		if !sameCompleteCoreState(saved, reopened.state) {
			t.Fatal("carrier nested evidence changed on restart")
		}
		closeFoundationStore(t, reopened)
	}
}
