package commit

import (
	"errors"
	"testing"
	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	elementfact "ucupa/ucupa-core/internal/element/fact"
	"ucupa/ucupa-core/internal/token"
	transferencoding "ucupa/ucupa-core/internal/transfer/encoding"
	transferfact "ucupa/ucupa-core/internal/transfer/fact"
)

func refuseCarrier(o *Owner, c transferfact.CarrierControl) (transferfact.CarrierRefusal, error) {
	o.Lock()
	defer o.Unlock()
	return o.RefuseCarrierFenceLocked(c.Reservation.Input.Actor, c)
}

func cancelPermittedCarrier(o *Owner, c transferfact.CarrierControl, r transferfact.CarrierRefusal) (transferfact.CarrierControl, error) {
	o.Lock()
	defer o.Unlock()
	i := c.Reservation.Input
	return o.CancelPermittedCarrierLocked(i.Actor, i.Operation, r)
}

func carrierPermitFixture(t *testing.T) (parent, source *Owner, c transferfact.CarrierControl) {
	t.Helper()
	parent, source, _, i := carrierControlFixture(t)
	// Same text as an existing source Element operation: the source must not
	// confuse a parent's control identity with its own original CRUD result.
	i.Operation = "carrier-content"
	r, err := reserveCarrier(t, parent, i)
	if err != nil {
		t.Fatal(err)
	}
	c, err = advanceCarrier(t, parent, r, true)
	if err != nil {
		t.Fatal(err)
	}
	return
}

func TestCarrierRefusalDurablyClosesPermitWithoutChangingSourceFacts(t *testing.T) {
	p, s, c := carrierPermitFixture(t)
	before := s.state.clone()
	r, err := refuseCarrier(s, c)
	if err != nil || !r.Matches(c) {
		t.Fatal("source refusal", r, err)
	}
	if s.state.currentCut() != before.currentCut() || !s.state.elements.Equal(before.elements) || s.state.mount.facts.Incoming() != before.mount.facts.Incoming() ||
		len(s.state.transfers.events) != len(before.transfers.events)+1 || len(s.state.transfers.carrierSources) != 1 {
		t.Fatal("refusal fenced, rewrote, or failed to retain source obligation")
	}
	if _, err = advanceCarrier(t, p, c, false); !errors.Is(err, values.PublicUnavailable) {
		t.Fatal("parent cancelled without reading source result", err)
	}
	// Refusal is not a write fence: unrelated writes and original results stay
	// available, and the immutable refusal can be recovered at a later data cut.
	if _, err = applyFoundationStore(s, elementfact.OperationCreateEntity, foundationStoreActor, foundationStoreID(106), foundationStoreTuple(t, c.Reservation.Input.Mount, 0, 1), contract.Revision{}, "after-refusal"); err != nil {
		t.Fatal(err)
	}
	if got, err := refuseCarrier(s, c); err != nil || got != r {
		t.Fatal("retry returned a later source cut", got, err)
	}
	old := p.state.mount.facts.Record(c.Reservation.Input.Mount)
	closed, err := cancelPermittedCarrier(p, c, r)
	if err != nil || !closed.Valid() || closed.Refusal != r || closed.CancelledAt == (contract.CoreReadCut{}) {
		t.Fatal("parent cancellation", closed, err)
	}
	next := p.state.mount.facts.Record(c.Reservation.Input.Mount)
	if next.Key != old.Key || next.Admission != old.Admission || next.Contact != old.Contact || next.Slot != "" || next.Revision == old.Revision {
		t.Fatal("cancel changed carrier or kept slot")
	}
	if got, err := advanceCarrier(t, p, c, true); err != nil || got != c {
		t.Fatal("permit retry leaked later cancellation", got, err)
	}
	if got, err := cancelPermittedCarrier(p, c, r); err != nil || got != closed {
		t.Fatal("cancel retry lost original result", got, err)
	}
	for _, o := range []*Owner{p, s} {
		path, state := o.store.db.Path(), o.state.clone()
		closeFoundationStore(t, o)
		reopened := openFoundationStore(t, path)
		if !sameCompleteCoreState(state, reopened.state) {
			t.Fatal("original journal did not reconstruct refusal/cancellation")
		}
		if o == s {
			reopened.RLock()
			got, err := reopened.CarrierSourceRefusalLocked(c.Reservation.Input.Actor, r.Key)
			reopened.RUnlock()
			if err != nil || got != r {
				t.Fatal("lost exact source lookup", got, err)
			}
		} else if got, err := cancelPermittedCarrier(reopened, c, r); err != nil || got != closed {
			t.Fatal("parent reopened result differs", got, err)
		}
		closeFoundationStore(t, reopened)
	}
}

func TestCarrierRefusalRejectsWrongIdentityAndDoesNotOccupyLocalNamespace(t *testing.T) {
	p, s, c := carrierPermitFixture(t)
	r, err := refuseCarrier(s, c)
	if err != nil {
		t.Fatal(err)
	}
	before := s.state.clone()
	changed := c
	changed.Reservation.Input.TargetAddress = "local:substituted"
	if _, err := refuseCarrier(s, changed); !errors.Is(err, contract.ErrOperationConflict) || !sameCompleteCoreState(before, s.state) {
		t.Fatal("changed input reused parent ID", err)
	}
	wrongActor := values.PrincipalRef{Realm: "r", Principal: "other"}
	s.RLock()
	_, err = s.CarrierSourceRefusalLocked(wrongActor, r.Key)
	s.RUnlock()
	if !errors.Is(err, values.PublicDenied) {
		t.Fatal("foreign actor read source result", err)
	}
	s.Lock()
	_, err = s.RefuseCarrierFenceLocked(wrongActor, c)
	s.Unlock()
	if !errors.Is(err, values.PublicDenied) || !sameCompleteCoreState(before, s.state) {
		t.Fatal("foreign actor admitted source refusal", err)
	}
	for _, mutate := range []func(*transferfact.CarrierRefusal){
		func(r *transferfact.CarrierRefusal) { r.Key.Controller = s.authority },
		func(r *transferfact.CarrierRefusal) { r.Key.Operation = "another" },
		func(r *transferfact.CarrierRefusal) { r.Source = p.authority },
		func(r *transferfact.CarrierRefusal) { r.ReservationAt = c.PermitAt },
		func(r *transferfact.CarrierRefusal) { r.PermitAt = c.Reservation.At },
		func(r *transferfact.CarrierRefusal) { r.At = c.PermitAt },
		func(r *transferfact.CarrierRefusal) { r.Revision = 0 },
	} {
		bad, before := r, p.state.clone()
		mutate(&bad)
		if _, err := cancelPermittedCarrier(p, c, bad); err == nil || !sameCompleteCoreState(before, p.state) {
			t.Fatal("parent consumed a mismatched refusal", err)
		}
	}
	// A different controller's same text does not identify this source record.
	other := r.Key
	other.Controller = c.Reservation.Input.Target
	s.RLock()
	_, err = s.CarrierSourceRefusalLocked(c.Reservation.Input.Actor, other)
	s.RUnlock()
	if !errors.Is(err, contract.ErrNotFound) {
		t.Fatal("lookup silently fell back to bare operation", err)
	}
	if s.state.operations[c.Reservation.Input.Operation].outcome.Status != contract.MutationCommitted {
		t.Fatal("foreign refusal replaced original local Element outcome")
	}
}

func TestCarrierRefusalUsesExistingControlCapacityAndKeepsReadProjectionFinite(t *testing.T) {
	_, s, c := carrierPermitFixture(t)
	before := s.state.clone()
	// Capacity check executes on the unpublished candidate, using the same
	// allowance as local source and parent controls. No hidden extra allowance.
	for n := 0; n < transferencoding.MaxTransferIntents; n++ {
		key := transferfact.TransferKey{Controller: c.Reservation.Input.Parent, Operation: contract.OperationID(string(rune(n + 1)))}
		s.state.transfers.carrierSources[key] = carrierSourceRecord{}
	}
	full := s.state.clone()
	if _, err := refuseCarrier(s, c); !errors.Is(err, values.PublicResourceExhausted) || !sameCompleteCoreState(full, s.state) {
		t.Fatal("refusal bypassed capacity or left a partial record", err)
	}
	s.state = before
	r, err := refuseCarrier(s, c)
	if err != nil {
		t.Fatal(err)
	}
	s.RLock()
	view, ok := s.CaptureReadLocked()
	s.RUnlock()
	if !ok || view.Validate() != nil || view.Projection.CurrentCut() != r.At || len(view.Projection.mountOperations) != 0 {
		t.Fatal("source refusal polluted spatial projection")
	}
	if token.Serial(r.At) != token.Serial(before.currentCut()) {
		t.Fatal("refusal invented spatial change")
	}
}

func TestCarrierRefusalRetainsOpaqueControllerInputBytes(t *testing.T) {
	p, s, _, i := carrierControlFixture(t)
	i.Operation = "opaque\x00\xff"
	i.Actor = values.PrincipalRef{Realm: "realm\xff", Principal: "actor\x00\xfe"}
	c, err := reserveCarrier(t, p, i)
	if err != nil {
		t.Fatal(err)
	}
	c, err = advanceCarrier(t, p, c, true)
	if err != nil {
		t.Fatal(err)
	}
	r, err := refuseCarrier(s, c)
	if err != nil {
		t.Fatal(err)
	}
	for _, o := range []*Owner{p, s} {
		path, before := o.store.db.Path(), o.state.clone()
		closeFoundationStore(t, o)
		next := openFoundationStore(t, path)
		if !sameCompleteCoreState(before, next.state) {
			t.Fatal("opaque identity changed in original replay")
		}
		if o == s {
			if got, err := refuseCarrier(next, c); err != nil || got != r {
				t.Fatal("opaque input retry differs", got, err)
			}
		} else if _, err := cancelPermittedCarrier(next, c, r); err != nil {
			t.Fatal("reopened parent rejected exact source receipt", err)
		}
		closeFoundationStore(t, next)
	}
}
