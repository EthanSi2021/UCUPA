package commit

import (
	"testing"

	"ucupa/ucupa-core/contract"
	"ucupa/ucupa-core/internal/token"
	transferfact "ucupa/ucupa-core/internal/transfer/fact"
)

func TestCarrierControlProjectionRetainsOnlyOccupiedSlot(t *testing.T) {
	parent, _, _, i := carrierControlFixture(t)
	c, err := reserveCarrier(t, parent, i)
	if err != nil {
		t.Fatal(err)
	}
	parent.RLock()
	candidate, ok := parent.CaptureReadLocked()
	parent.RUnlock()
	if !ok || candidate.Validate() != nil {
		t.Fatal("parent reservation invalidated current projection")
	}
	view := candidate.Seal()
	if len(view.mountOperations) != 1 || view.mountOperations[i.Operation].Carrier != c {
		t.Fatal("projection did not retain exact current slot")
	}
	if _, err = advanceCarrier(t, parent, c, false); err != nil {
		t.Fatal(err)
	}
	parent.RLock()
	next, ok := parent.CaptureReadLocked()
	parent.RUnlock()
	if !ok || next.Validate() != nil || len(next.Projection.mountOperations) != 0 {
		t.Fatal("closed reservation leaked into current read evidence")
	}
	if err = ValidateMountSnapshotProjection(view); err != nil {
		t.Fatal("later cancel changed historical projection", err)
	}
	badSlot := view
	badSlot.CutSerial = next.Projection.CutSerial
	badControl := c
	badControl.CancelledAt = next.Projection.CurrentCut()
	badSlot.mountOperations = map[contract.OperationID]MountOperation{i.Operation: {Carrier: badControl}}
	if ValidateMountSnapshotProjection(badSlot) == nil {
		t.Fatal("cancelled control retained a live slot")
	}
	for _, change := range []func(*MountOperation){
		func(m *MountOperation) { m.Carrier.PermitAt = token.FromParts(view.Scope, view.CutSerial+1) },
		func(m *MountOperation) { m.Carrier.CancelledAt = token.FromParts(view.Scope, view.CutSerial+1) },
		func(m *MountOperation) { m.Carrier.Reservation.Tuple.Parent = i.Mount },
		func(m *MountOperation) { m.Decision = contract.MountCommitted },
		func(m *MountOperation) {
			m.Carrier.Reservation.Input.ExpectedElement = token.RevisionFromParts(view.Scope, i.Mount, 1)
		},
	} {
		operation := MountOperation{Carrier: c}
		change(&operation)
		bad := view
		bad.mountOperations = map[contract.OperationID]MountOperation{i.Operation: operation}
		if ValidateMountSnapshotProjection(bad) == nil {
			t.Fatal("forged finite carrier proof accepted")
		}
	}
}

func TestCarrierReservationRetryAfterPermitHasOriginalResult(t *testing.T) {
	parent, _, _, i := carrierControlFixture(t)
	c, err := reserveCarrier(t, parent, i)
	if err != nil {
		t.Fatal(err)
	}
	permit, err := advanceCarrier(t, parent, c, true)
	if err != nil {
		t.Fatal(err)
	}
	if permit.PermitAt == (contract.CoreReadCut{}) {
		t.Fatal("missing permit cut")
	}
	if got, err := reserveCarrier(t, parent, i); err != nil || got != (transferfact.CarrierControl{Reservation: c.Reservation}) {
		t.Fatal("begin retry leaked later milestone", got, err)
	}
}
