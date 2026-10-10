package fact

import (
	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	"ucupa/ucupa-core/internal/geometry"
	"ucupa/ucupa-core/internal/token"
)

// CarrierIntent addresses the parent-owned relation, not an Element at the
// carrier. The mount Anchor itself never moves to the replacement carrier.
type CarrierIntent struct {
	Operation                                   contract.OperationID
	Actor                                       values.PrincipalRef
	Parent, Source, Target                      values.FactAuthorityRef
	Mount                                       values.ElementID
	ExpectedElement                             contract.Revision
	ExpectedMount                               contract.MountRevision
	ParentAddress, SourceAddress, TargetAddress values.NextHop
}

func (i CarrierIntent) Key() TransferKey {
	return TransferKey{Controller: i.Parent, Operation: i.Operation}
}

func (i CarrierIntent) Valid() bool {
	return len(i.Operation) > 0 && len(i.Operation) <= 128 && i.Actor.Valid() && i.Parent.Valid() && i.Source.Valid() && i.Target.Valid() &&
		i.Parent != i.Source && i.Parent != i.Target && i.Source != i.Target && i.Mount.Valid() &&
		token.ValidRevision(i.ExpectedElement) && token.RevisionElement(i.ExpectedElement) == i.Mount &&
		token.ValidMountRevision(i.ExpectedMount) && token.MountRevisionElement(i.ExpectedMount) == i.Mount &&
		token.RevisionScope(i.ExpectedElement) == token.MountRevisionScope(i.ExpectedMount) &&
		i.ParentAddress.Valid() && i.SourceAddress.Valid() && i.TargetAddress.Valid()
}

// Reservation fixes current parent facts and a fresh proposal, but grants no
// source permission to fence. It is not the target's content acceptance.
type CarrierReservation struct {
	Input           CarrierIntent
	Previous        contract.MountAdmissionKey
	PreviousContact contract.NextHopRevision
	Tuple           contract.SpatialTuple
	NextKey         contract.MountKey
	At              contract.CoreReadCut
}

func (r CarrierReservation) Valid() bool {
	i := r.Input
	k := r.Previous.MountKey
	return i.Valid() && k.Valid() && k.ParentAuthority == i.Parent && k.CarrierAuthority == i.Source && k.MountAnchorUUID == i.Mount &&
		len(r.Previous.OperationID) > 0 && len(r.Previous.OperationID) <= 128 && r.Previous.OperationID != i.Operation &&
		token.Valid(r.At) && token.Scope(r.At) == token.MountRevisionScope(i.ExpectedMount) &&
		token.MountRevisionScope(k.ProposalRevision) == token.Scope(r.At) && token.MountRevisionSerial(k.ProposalRevision) <= token.MountRevisionSerial(i.ExpectedMount) &&
		token.Serial(r.At) > token.MountRevisionSerial(i.ExpectedMount) && r.Tuple.Parent.Valid() && r.Tuple.Parent != i.Mount && geometry.CanonicalPose(r.Tuple.LocalPose) && geometry.ValidSphere(r.Tuple.Range) &&
		token.ValidNextHopRevision(r.PreviousContact) && token.NextHopKey(r.PreviousContact) == k && !token.NextHopIncoming(r.PreviousContact) &&
		token.NextHopScope(r.PreviousContact) == token.Scope(r.At) && token.NextHopSerial(r.PreviousContact) < token.Serial(r.At) &&
		r.NextKey == (contract.MountKey{ParentAuthority: i.Parent, CarrierAuthority: i.Target, MountAnchorUUID: i.Mount, ProposalRevision: token.MountRevisionFromParts(token.Scope(r.At), i.Mount, token.Serial(r.At))})
}

// These are finite current parent control facts in the existing mount slot.
// Permit closes local cancellation before any source fence can be dispatched.
// Neither its cut nor its serialized bytes authenticate the parent or caller.
type CarrierControl struct {
	Reservation CarrierReservation
	PermitAt    contract.CoreReadCut
	CancelledAt contract.CoreReadCut
	Refusal     CarrierRefusal // only cancellation after a dispatched permit
}

func (c CarrierControl) Valid() bool {
	if !c.Reservation.Valid() {
		return false
	}
	for _, at := range []contract.CoreReadCut{c.PermitAt, c.CancelledAt} {
		if at != (contract.CoreReadCut{}) && (!token.Valid(at) || token.Scope(at) != token.Scope(c.Reservation.At) || token.Serial(at) <= token.Serial(c.Reservation.At)) {
			return false
		}
	}
	if c.PermitAt != (contract.CoreReadCut{}) && c.CancelledAt != (contract.CoreReadCut{}) {
		return token.Serial(c.CancelledAt) > token.Serial(c.PermitAt) && c.Refusal.Matches(c)
	}
	return c.Refusal == (CarrierRefusal{})
}

// CarrierRefusal is the source's durable promise never to fence this exact
// parent operation. It binds the parent's immutable reservation and permit,
// not an unauthenticated claim that the network timed out. The original source
// journal also retains the full input, so a changed request cannot reuse it.
type CarrierRefusal struct {
	Key                         TransferKey
	Source                      values.FactAuthorityRef
	ReservationAt, PermitAt, At contract.CoreReadCut
	Revision                    uint64 // source control journal sequence
}

func (r CarrierRefusal) Valid() bool {
	return r.Key.Valid() && r.Source.Valid() && r.Source != r.Key.Controller && r.Revision != 0 &&
		token.Valid(r.ReservationAt) && token.Valid(r.PermitAt) && token.Valid(r.At) &&
		token.Scope(r.ReservationAt) == token.Scope(r.PermitAt) && token.Serial(r.PermitAt) > token.Serial(r.ReservationAt) &&
		token.Scope(r.At) != token.Scope(r.PermitAt)
}

func (r CarrierRefusal) Matches(c CarrierControl) bool {
	return r.Valid() && r.Key == c.Reservation.Input.Key() && r.Source == c.Reservation.Input.Source &&
		r.ReservationAt == c.Reservation.At && r.PermitAt == c.PermitAt
}
