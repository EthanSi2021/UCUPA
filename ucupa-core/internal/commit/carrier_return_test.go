package commit

import (
	"errors"
	"fmt"
	"testing"
	"ucupa/ucupa-core/contract"
	elementfact "ucupa/ucupa-core/internal/element/fact"
	mountfact "ucupa/ucupa-core/internal/mount/fact"
	transferencoding "ucupa/ucupa-core/internal/transfer/encoding"
	transferfact "ucupa/ucupa-core/internal/transfer/fact"
)

func nextCarrierFence(t *testing.T, parent, source, target *Owner, previous transferfact.CarrierInstallation, op contract.OperationID) (transferfact.CarrierFence, string) {
	t.Helper()
	old := previous.Binding.Acceptance.Fence.Permit.Reservation.Input
	a, _ := parent.state.elements.Anchor(old.Mount)
	i := transferfact.CarrierIntent{Operation: op, Actor: old.Actor, Parent: parent.authority, Source: source.authority, Target: target.authority,
		Mount: old.Mount, ExpectedElement: a.Revision, ExpectedMount: parent.state.mount.facts.Record(old.Mount).Revision,
		ParentAddress: old.ParentAddress, SourceAddress: old.TargetAddress, TargetAddress: old.SourceAddress}
	c, err := reserveCarrier(t, parent, i)
	if err != nil {
		t.Fatal("reserve next", err)
	}
	c, err = advanceCarrier(t, parent, c, true)
	if err != nil {
		t.Fatal("permit next", err)
	}
	f, raw, err := fenceCarrier(source, c)
	if err != nil {
		t.Fatal("fence next", err)
	}
	return f, raw
}

func TestCarrierReturnAndRepeatedDepartureKeepOneWriterAndOriginalResults(t *testing.T) {
	for _, empty := range []bool{false, true} {
		t.Run(fmt.Sprint(empty), func(t *testing.T) {
			p, s, d, b := boundCarrierFixture(t, empty)
			first, err := installCarrier(d, b)
			if err != nil {
				t.Fatal(err)
			}
			input := b.Acceptance.Fence.Permit.Reservation.Input
			if _, err := settleCarrier(p, first, true); err != nil {
				t.Fatal(err)
			}
			if _, err := settleCarrier(s, first, false); err != nil {
				t.Fatal(err)
			}
			oldParentAnchor, _ := p.state.elements.Anchor(input.Mount)
			from, to, last := d, s, first
			for generation := 2; generation <= 4; generation++ {
				f, raw := nextCarrierFence(t, p, from, to, last, contract.OperationID(fmt.Sprintf("carrier-cycle-%d", generation)))
				manifest, err := transferencoding.DecodeCarrierManifest(f, raw)
				if err != nil {
					t.Fatal(err)
				}
				if !empty && len(manifest.Lineage(foundationStoreID(105))) != generation-1 {
					t.Fatal("lost custody generations")
				}
				a, err := acceptCarrier(to, f, raw)
				if err != nil {
					t.Fatal("return acceptance", generation, err)
				}
				decision, err := bindCarrier(p, a)
				if err != nil {
					t.Fatal("return binding", err)
				}
				installed, err := installCarrier(to, decision)
				if err != nil {
					t.Fatal("return installation", err)
				}
				if to.state.mount.facts.Incoming().Gate != mountfact.GateOpen || from.state.mount.facts.Incoming().Gate != mountfact.GateFenced {
					t.Fatal("wrong write gates")
				}
				if !empty {
					if _, ok := to.state.elements.Entity(foundationStoreID(105)); !ok {
						t.Fatal("lost returning UUID")
					}
					if from.state.elements.Kind(foundationStoreID(105)) != 0 {
						t.Fatal("two active copies")
					}
				}
				if _, err := settleCarrier(p, installed, true); err != nil {
					t.Fatal(err)
				}
				if _, err := settleCarrier(from, installed, false); err != nil {
					t.Fatal(err)
				}
				for _, o := range []*Owner{p, from, to} {
					if err := validateCoreState(o.state); err != nil {
						t.Fatal("replay", generation, err)
					}
				}
				if anchor, _ := p.state.elements.Anchor(input.Mount); anchor != oldParentAnchor {
					t.Fatal("rewrote parent anchor")
				}
				from, to, last = to, from, installed
			}
			// Late retries after several changes of carrier recover original
			// receipts, never replay the old activation or reopen a source gate.
			for _, o := range []*Owner{p, s, d} {
				path, before := o.store.db.Path(), o.state.clone()
				closeFoundationStore(t, o)
				o = openFoundationStore(t, path)
				if !sameCompleteCoreState(before, o.state) {
					t.Fatal("reopen changed cycles")
				}
				switch o.authority {
				case p.authority:
					if got, err := bindCarrier(o, b.Acceptance); err != nil || got != b {
						t.Fatal("old binding", err)
					}
					if _, err := settleCarrier(o, first, true); err != nil {
						t.Fatal(err)
					}
				case s.authority:
					if _, err := settleCarrier(o, first, false); err != nil {
						t.Fatal(err)
					}
					if err := o.CheckTransferReadLocked(o.state.elements.View, b.Acceptance.Fence.FinalCut, input.Mount, ""); !errors.Is(err, contract.ErrReadCutExpired) {
						t.Fatal("revived old read", err)
					}
				case d.authority:
					if got, err := installCarrier(o, b); err != nil || got != first {
						t.Fatal("old install", err)
					}
				}
				if !sameCompleteCoreState(before, o.state) {
					t.Fatal("late retry changed current generation")
				}
				closeFoundationStore(t, o)
			}
		})
	}
}

func TestCarrierReturnRequiresCompletedDepartureAndAcceptsChangedContent(t *testing.T) {
	p, s, d, b := boundCarrierFixture(t, false)
	first, err := installCarrier(d, b)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := settleCarrier(p, first, true); err != nil {
		t.Fatal(err)
	}
	// A deletion remains a deletion; a newly created UUID may return alongside
	// old custody. The original owner retains its absent claims, not live data.
	id := foundationStoreID(105)
	v, _ := d.state.elements.Entity(id)
	if _, err := applyFoundationStore(d, elementfact.OperationDeleteEntity, foundationStoreActor, id, contract.SpatialTuple{}, v.Revision, "delete-away"); err != nil {
		t.Fatal(err)
	}
	input := b.Acceptance.Fence.Permit.Reservation.Input
	if _, err := applyFoundationStore(d, elementfact.OperationCreateEntity, foundationStoreActor, foundationStoreID(109), foundationStoreTuple(t, input.Mount, 0, 1), contract.Revision{}, "create-away"); err != nil {
		t.Fatal(err)
	}
	f, raw := nextCarrierFence(t, p, d, s, first, "return-changed")
	before := s.state.clone()
	if _, err := acceptCarrier(s, f, raw); !errors.Is(err, contract.ErrMountCoordinationRequired) || !sameCompleteCoreState(before, s.state) {
		t.Fatal("accepted unfinished old departure", err)
	}
	if _, err := settleCarrier(s, first, false); err != nil {
		t.Fatal(err)
	}
	a, err := acceptCarrier(s, f, raw)
	if err != nil {
		t.Fatal(err)
	}
	decision, err := bindCarrier(p, a)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := installCarrier(s, decision); err != nil {
		t.Fatal(err)
	}
	if s.state.elements.Kind(id) != 0 || s.state.elements.Kind(foundationStoreID(109)) != contract.ElementEntity {
		t.Fatal("wrong returned membership")
	}
	if err := validateCoreState(s.state); err != nil {
		t.Fatal(err)
	}
}
