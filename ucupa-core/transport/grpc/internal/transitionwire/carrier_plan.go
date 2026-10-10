package transitionwire

import (
	"google.golang.org/protobuf/proto"
	"ucupa/shared/values"
	pb "ucupa/ucupa-core/bindings/go/protobuf"
	"ucupa/ucupa-core/contract"
	"ucupa/ucupa-core/internal/token"
	"ucupa/ucupa-core/internal/transfer/fact"
	"ucupa/ucupa-core/transport/grpc/internal/wire"
)

const MaxCarrierEvidenceBytes = 16 << 10

func MatchesCarrier(b contract.CarrierApprovalBinding, i fact.CarrierIntent) bool {
	return i.Valid() && i.Parent == b.Parent && i.Operation == b.Operation && i.Source == b.Command.Source && i.Target == b.Command.Target && i.Mount == b.Command.Mount &&
		i.ParentAddress == b.Command.ParentAddress && i.SourceAddress == b.Command.SourceAddress && i.TargetAddress == b.Command.TargetAddress
}

func CarrierPlan(b contract.CarrierApprovalBinding, w *pb.CarrierPlan) (fact.CarrierControl, error) {
	bad := func() (fact.CarrierControl, error) { return fact.CarrierControl{}, contract.ErrInvalid }
	if w == nil || w.Reservation == nil || proto.Size(w) > MaxCarrierEvidenceBytes {
		return bad()
	}
	actual, err := wire.CarrierBinding(w.Binding)
	if err != nil || actual != b {
		return bad()
	}
	r := w.Reservation
	actor := values.PrincipalRef{Realm: values.RealmID(r.GetActor().GetRealm()), Principal: values.PrincipalID(r.GetActor().GetPrincipal())}
	if !actor.Valid() || len(actor.Realm)+len(actor.Principal) > 4096 {
		return bad()
	}
	e, err := wire.MountRevision(r.ExpectedElement, string(b.Command.Mount))
	if err != nil {
		return bad()
	}
	er, err := e.ElementRevision()
	if err != nil {
		return bad()
	}
	m, err := wire.MountRevision(r.ExpectedMount, string(b.Command.Mount))
	if err != nil {
		return bad()
	}
	mr, err := m.MountRevision()
	if err != nil {
		return bad()
	}
	previous, err := wire.MountAdmission(r.Previous)
	if err != nil {
		return bad()
	}
	contact, err := wire.MountRevision(r.PreviousContact, string(b.Command.Mount))
	if err != nil {
		return bad()
	}
	tuple, err := wire.MountTuple(r.Tuple)
	if err != nil {
		return bad()
	}
	next, err := wire.MountKey(r.NextKey)
	if err != nil {
		return bad()
	}
	at, err := wire.MountCut(r.At)
	if err != nil {
		return bad()
	}
	permit, err := wire.MountCut(w.PermittedAt)
	if err != nil {
		return bad()
	}
	cancelled, err := wire.MountCut(w.CancelledAt)
	if err != nil {
		return bad()
	}
	i := fact.CarrierIntent{Operation: b.Operation, Actor: actor, Parent: b.Parent, Source: b.Command.Source, Target: b.Command.Target, Mount: b.Command.Mount,
		ExpectedElement: er, ExpectedMount: mr, ParentAddress: b.Command.ParentAddress, SourceAddress: b.Command.SourceAddress, TargetAddress: b.Command.TargetAddress}
	c := fact.CarrierControl{Reservation: fact.CarrierReservation{Input: i, Previous: previous,
		PreviousContact: token.NextHopRevisionFromParts(contact.Incarnation, previous.MountKey, false, contact.Sequence), Tuple: tuple, NextKey: next, At: at}, PermitAt: permit, CancelledAt: cancelled}
	if w.Refusal != nil {
		refused, err := wire.MountCut(w.Refusal.At)
		if err != nil {
			return bad()
		}
		c.Refusal = fact.CarrierRefusal{Key: i.Key(), Source: i.Source, ReservationAt: at, PermitAt: permit, At: refused, Revision: w.Refusal.GetRevision()}
	}
	if !c.Valid() {
		return bad()
	}
	return c, nil
}

func CarrierPlanWire(b contract.CarrierApprovalBinding, c fact.CarrierControl) (*pb.CarrierPlan, error) {
	binding, err := CarrierBindingWire(b)
	if err != nil || !c.Valid() || !MatchesCarrier(b, c.Reservation.Input) {
		return nil, contract.ErrInvalid
	}
	r := c.Reservation
	i := r.Input
	contact := r.PreviousContact
	w := &pb.CarrierPlan{Binding: binding, Reservation: &pb.ServiceCarrierReservation{
		Actor:           &pb.TransitionActor{Realm: []byte(i.Actor.Realm), Principal: []byte(i.Actor.Principal)},
		ExpectedElement: wire.MountRevisionWire(i.ExpectedElement.ServiceEvidence()), ExpectedMount: wire.MountRevisionWire(i.ExpectedMount.ServiceEvidence()),
		Previous: wire.MountAdmissionWire(r.Previous), PreviousContact: wire.MountRevisionWire(contract.ServiceRevision{Incarnation: token.NextHopScope(contact), Element: i.Mount, Sequence: token.NextHopSerial(contact)}),
		Tuple: wire.MountTupleWire(r.Tuple), NextKey: wire.MountKeyWire(r.NextKey), At: wire.MountCutWire(r.At)},
		PermittedAt: wire.MountCutWire(c.PermitAt), CancelledAt: wire.MountCutWire(c.CancelledAt)}
	if c.Refusal != (fact.CarrierRefusal{}) {
		w.Refusal = &pb.ServiceCarrierRefusal{At: wire.MountCutWire(c.Refusal.At), Revision: proto.Uint64(c.Refusal.Revision)}
	}
	if actual, err := CarrierPlan(b, w); err != nil || actual != c {
		return nil, contract.ErrInvalid
	}
	return w, nil
}
