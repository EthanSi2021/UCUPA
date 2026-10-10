package commit

import (
	"errors"
	"testing"

	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	elementfact "ucupa/ucupa-core/internal/element/fact"
	"ucupa/ucupa-core/internal/token"
	transferfact "ucupa/ucupa-core/internal/transfer/fact"
)

func bindCarrier(o *Owner, a transferfact.CarrierAcceptance) (transferfact.CarrierBinding, error) {
	o.Lock()
	defer o.Unlock()
	return o.CommitCarrierBindingLocked(a.Fence.Permit.Reservation.Input.Actor, a)
}

func TestCarrierBindingChangesOnlyParentsExactRelationAndSurvivesReopen(t *testing.T) {
	for _, empty := range []bool{false, true} {
		t.Run(map[bool]string{false: "populated", true: "empty"}[empty], func(t *testing.T) {
			p, s, d, f, raw := carrierAcceptanceFixture(t, empty)
			a, err := acceptCarrier(d, f, raw)
			if err != nil {
				t.Fatal(err)
			}
			i := f.Permit.Reservation.Input
			before, source, target := p.state.clone(), s.state.clone(), d.state.clone()
			b, err := bindCarrier(p, a)
			if err != nil || !b.Valid() || b.Acceptance != a || b.At != p.state.currentCut() {
				t.Fatal("binding", b, err)
			}
			r := p.state.mount.facts.Record(i.Mount)
			if r.Key != f.Permit.Reservation.NextKey || r.Admission != (contract.MountAdmissionKey{MountKey: r.Key, OperationID: i.Operation}) || r.Key.CarrierAuthority != d.authority || r.Slot != i.Operation || r.Contact.NextHop != i.TargetAddress || token.NextHopKey(r.Contact.Revision) != r.Key {
				t.Fatal("not one exact new binding", r)
			}
			if p.state.cutSerial != before.cutSerial+1 || p.state.revisionSerial != before.revisionSerial || !p.state.elements.Equal(before.elements) ||
				!sameCompleteCoreState(source, s.state) || !sameCompleteCoreState(target, d.state) {
				t.Fatal("binding installed content, rewrote space, or changed other owner")
			}
			if got, err := bindCarrier(p, a); err != nil || got != b || p.state.cutSerial != before.cutSerial+1 {
				t.Fatal("exact retry", err)
			}
			path, durable := p.store.db.Path(), p.state.clone()
			closeFoundationStore(t, p)
			p = openFoundationStore(t, path)
			defer closeFoundationStore(t, p)
			if !sameCompleteCoreState(durable, p.state) {
				t.Fatal("reopen lost binding")
			}
			p.RLock()
			view, ok := p.CaptureReadLocked()
			got, err := p.CarrierBindingLocked(i.Actor, i.Operation)
			p.RUnlock()
			if !ok || view.Validate() != nil || err != nil || got != b {
				t.Fatal("finite projection/lookup", err)
			}
			bad := *view
			bad.Projection.mountOperations = map[contract.OperationID]MountOperation{i.Operation: {Carrier: f.Permit}}
			if bad.Validate() == nil {
				t.Fatal("accepted new relation without binding proof")
			}
			// Parent control remains occupied until installation, not just until
			// the target has promised to install. Ordinary mutation stays fenced.
			anchor, _ := p.state.elements.Anchor(i.Mount)
			if _, err := applyFoundationStore(p, elementfact.OperationUpdateAnchor, i.Actor, i.Mount, f.Permit.Reservation.Tuple, anchor.Revision, "late-range"); !errors.Is(err, contract.ErrMountBusy) {
				t.Fatal("updated unsettled binding", err)
			}
			if _, err := applyFoundationStore(p, elementfact.OperationCreateEntity, i.Actor, foundationStoreID(9600), foundationStoreTuple(t, foundationStoreID(100), 40, 1), contract.Revision{}, "unrelated-after-binding"); err != nil {
				t.Fatal("blocked unrelated branch", err)
			}
			// New contact is mutable, but identity, decision and original input
			// are not rewritten; a delayed old-key repair must fail.
			p.Lock()
			_, err = p.ApplyMountAddressLocked("repair-new-carrier", NextHopBinding{Actor: i.Actor, Element: i.Mount, Key: r.Key, Expected: r.Contact.Revision, NextHop: "local:repaired-target"})
			p.Unlock()
			if err != nil {
				t.Fatal("new contact repair", err)
			}
			old := before.mount.facts.Record(i.Mount)
			p.Lock()
			_, err = p.ApplyMountAddressLocked("late-old-carrier", NextHopBinding{Actor: i.Actor, Element: i.Mount, Key: old.Key, Expected: old.Contact.Revision, NextHop: "local:old-target"})
			p.Unlock()
			if !errors.Is(err, contract.ErrConflict) {
				t.Fatal("old key repaired new relation", err)
			}
			if got, err := bindCarrier(p, a); err != nil || got != b {
				t.Fatal("contact repair rewrote original binding", err)
			}
			if err := validateCoreState(p.state); err != nil {
				t.Fatal("history after repair", err)
			}
		})
	}
}

func TestCarrierBindingRejectsOtherActorInputAndCancellation(t *testing.T) {
	p, _, d, f, raw := carrierAcceptanceFixture(t, false)
	a, err := acceptCarrier(d, f, raw)
	if err != nil {
		t.Fatal(err)
	}
	i := f.Permit.Reservation.Input
	before := p.state.clone()
	p.Lock()
	_, err = p.CommitCarrierBindingLocked(values.PrincipalRef{Realm: "r", Principal: "other"}, a)
	p.Unlock()
	if !errors.Is(err, values.PublicDenied) || !sameCompleteCoreState(before, p.state) {
		t.Fatal("wrong actor", err)
	}
	for name, change := range map[string]func(*transferfact.CarrierAcceptance){
		"no-source-fence":      func(a *transferfact.CarrierAcceptance) { a.Fence.Revision = 0 },
		"no-target-acceptance": func(a *transferfact.CarrierAcceptance) { a.Revision = 0 },
		"wrong-controller":     func(a *transferfact.CarrierAcceptance) { a.Fence.Permit.Reservation.Input.Parent = d.authority },
		"target-address": func(a *transferfact.CarrierAcceptance) {
			a.Fence.Permit.Reservation.Input.TargetAddress = "local:other-target"
		},
		"same-owner-cut": func(a *transferfact.CarrierAcceptance) { a.At = a.Fence.Permit.PermitAt },
	} {
		bad := a
		change(&bad)
		if _, err := bindCarrier(p, bad); err == nil || !sameCompleteCoreState(before, p.state) {
			t.Fatal(name, "committed", err)
		}
	}
	b, err := bindCarrier(p, a)
	if err != nil {
		t.Fatal(err)
	}
	before = p.state.clone()
	bad := a
	bad.Revision++
	if _, err := bindCarrier(p, bad); !errors.Is(err, contract.ErrOperationConflict) || !sameCompleteCoreState(before, p.state) {
		t.Fatal("second decision", err)
	}
	r := transferfact.CarrierRefusal{Key: i.Key(), Source: i.Source, ReservationAt: f.Permit.Reservation.At, PermitAt: f.Permit.PermitAt, At: f.FinalCut, Revision: f.Revision}
	if !r.Valid() {
		t.Fatal("bad test refusal")
	}
	if _, err := cancelPermittedCarrier(p, f.Permit, r); !errors.Is(err, contract.ErrConflict) || !sameCompleteCoreState(before, p.state) {
		t.Fatal("cancelled binding", err)
	}
	p.RLock()
	_, err = p.CarrierBindingLocked(values.PrincipalRef{Realm: "r", Principal: "other"}, i.Operation)
	p.RUnlock()
	if !errors.Is(err, values.PublicDenied) {
		t.Fatal("unauthorized decision lookup", err)
	}
	if got, err := advanceCarrier(t, p, f.Permit, true); err != nil || got != f.Permit || b.Acceptance.Fence.Permit != got {
		t.Fatal("old permit retry changed", err)
	}
}
