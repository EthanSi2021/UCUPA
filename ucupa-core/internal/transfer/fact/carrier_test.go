package fact

import (
	"testing"

	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	"ucupa/ucupa-core/internal/token"
)

func TestCarrierControlExactParentAndMilestoneShape(t *testing.T) {
	parent, err := values.NewFactAuthorityRef()
	if err != nil {
		t.Fatal(err)
	}
	scope := [32]byte{7}
	i := CarrierIntent{Operation: "replace", Actor: values.PrincipalRef{Realm: "r", Principal: "p"}, Parent: parent, Source: sourceOwner, Target: targetOwner, Mount: id(2),
		ExpectedElement: token.RevisionFromParts(scope, id(2), 2), ExpectedMount: token.MountRevisionFromParts(scope, id(2), 9), ParentAddress: "local:parent", SourceAddress: "local:source", TargetAddress: "local:target"}
	key := contract.MountKey{ParentAuthority: parent, CarrierAuthority: sourceOwner, MountAnchorUUID: i.Mount, ProposalRevision: token.MountRevisionFromParts(scope, i.Mount, 3)}
	c := CarrierControl{Reservation: CarrierReservation{Input: i, Previous: contract.MountAdmissionKey{MountKey: key, OperationID: "original"},
		PreviousContact: token.NextHopRevisionFromParts(scope, key, false, 3), Tuple: tuple(t, id(1), 0, 20),
		NextKey: contract.MountKey{ParentAuthority: parent, CarrierAuthority: targetOwner, MountAnchorUUID: i.Mount, ProposalRevision: token.MountRevisionFromParts(scope, i.Mount, 10)}, At: token.FromParts(scope, 10)}}
	if !c.Valid() {
		t.Fatal("valid parent reservation rejected")
	}
	for _, change := range []func(*CarrierControl){
		func(c *CarrierControl) { c.Reservation.Input.Parent = c.Reservation.Input.Source },
		func(c *CarrierControl) { c.Reservation.Previous.OperationID = "" },
		func(c *CarrierControl) { c.Reservation.Previous.OperationID = c.Reservation.Input.Operation },
		func(c *CarrierControl) { c.Reservation.Tuple.Parent = c.Reservation.Input.Mount },
		func(c *CarrierControl) { c.Reservation.Previous.MountKey.CarrierAuthority = targetOwner },
		func(c *CarrierControl) {
			c.Reservation.PreviousContact = token.NextHopRevisionFromParts(scope, key, true, 3)
		},
		func(c *CarrierControl) {
			c.Reservation.PreviousContact = token.NextHopRevisionFromParts(scope, key, false, 10)
		},
		func(c *CarrierControl) { c.Reservation.NextKey = c.Reservation.Previous.MountKey },
		func(c *CarrierControl) { c.Reservation.Tuple = contract.SpatialTuple{} },
		func(c *CarrierControl) { c.Reservation.At = token.FromParts(scope, 9) },
		func(c *CarrierControl) { c.PermitAt = c.Reservation.At },
		func(c *CarrierControl) { c.PermitAt = token.FromParts([32]byte{8}, 11) },
		func(c *CarrierControl) {
			c.PermitAt = token.FromParts(scope, 11)
			c.CancelledAt = token.FromParts(scope, 12)
		},
	} {
		bad := c
		change(&bad)
		if bad.Valid() {
			t.Fatal("invalid carrier control accepted", bad)
		}
	}
	for _, permit := range []bool{false, true} {
		v := c
		if permit {
			v.PermitAt = token.FromParts(scope, 11)
		} else {
			v.CancelledAt = token.FromParts(scope, 11)
		}
		if !v.Valid() {
			t.Fatal("valid one-way milestone rejected")
		}
	}
	c.PermitAt, c.CancelledAt = token.FromParts(scope, 11), token.FromParts(scope, 12)
	c.Refusal = CarrierRefusal{Key: i.Key(), Source: sourceOwner, ReservationAt: c.Reservation.At, PermitAt: c.PermitAt, At: token.FromParts([32]byte{8}, 5), Revision: 1}
	if !c.Valid() || c.Refusal.Key.Controller != parent {
		t.Fatal("source-proved cancellation not parent-controlled")
	}
	for _, change := range []func(*CarrierControl){
		func(c *CarrierControl) { c.Refusal.Key.Controller = sourceOwner },
		func(c *CarrierControl) { c.Refusal.Source = targetOwner },
		func(c *CarrierControl) { c.Refusal.At = c.PermitAt },
		func(c *CarrierControl) { c.Refusal.PermitAt = c.Reservation.At },
		func(c *CarrierControl) { c.CancelledAt = c.PermitAt },
		func(c *CarrierControl) { c.CancelledAt = contract.CoreReadCut{} },
		func(c *CarrierControl) { c.PermitAt = contract.CoreReadCut{} },
	} {
		bad := c
		change(&bad)
		if bad.Valid() {
			t.Fatal("unbound refusal or cancellation accepted")
		}
	}
}

func TestTransferKeyUsesControlOwnerNotCarrierOrBareID(t *testing.T) {
	i := CarrierIntent{Parent: values.FactAuthorityRef(id(30)), Source: sourceOwner, Operation: "opaque\x00\xff"}
	j := i
	j.Parent = values.FactAuthorityRef(id(31))
	k := Intent{Source: sourceOwner, Operation: i.Operation}.Key()
	if !i.Key().Valid() || i.Key() == j.Key() || i.Key() == k {
		t.Fatal("operation identity lost actual controller")
	}
	keys := map[TransferKey]bool{i.Key(): true, j.Key(): true, k: true}
	if len(keys) != 3 {
		t.Fatal("distinct controllers collided")
	}
}
