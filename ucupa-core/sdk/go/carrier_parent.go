package coresdk

import (
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"ucupa/shared/values"
	pb "ucupa/ucupa-core/bindings/go/protobuf"
)

func validateCarrierParentRequest(in any, parent, actor values.FactAuthorityRef) error {
	var b *pb.CarrierApprovalBinding
	switch q := in.(type) {
	case *pb.ReadCarrierPlanRequest:
		b = q.GetBinding()
	case *pb.ReadCarrierDecisionRequest:
		b = q.GetBinding()
	default:
		return nil
	}
	if b == nil || proto.Size(b) > 4096 || b.GetParent() != string(parent) || !actor.Valid() || actor == parent ||
		(b.GetCommand().GetSource() != string(actor) && b.GetCommand().GetTarget() != string(actor)) {
		return status.Error(codes.Unauthenticated, "wrong carrier peer")
	}
	return validateCarrierRequest(b.Command, parent)
}

func validateCarrierParentExchange(in, out any) error {
	bad := func() error { return status.Error(codes.DataLoss, "carrier evidence does not match request") }
	switch q := in.(type) {
	case *pb.ReadCarrierPlanRequest:
		r, ok := out.(*pb.ReadCarrierPlanResponse)
		if !ok || r == nil {
			return bad()
		}
		if p := r.GetResult(); p != nil && (!proto.Equal(p.Binding, q.Binding) || !validCarrierPlan(p)) {
			return bad()
		}
	case *pb.ReadCarrierDecisionRequest:
		r, ok := out.(*pb.ReadCarrierDecisionResponse)
		if !ok || r == nil {
			return bad()
		}
		if d := r.GetResult(); d != nil && (!proto.Equal(d.GetPlan().GetBinding(), q.Binding) || !validCarrierPlan(d.Plan) || d.Plan.PermittedAt == nil || d.Plan.CancelledAt != nil ||
			len(d.ManifestDigest) != 32 || d.GetFenceRevision() == 0 || d.GetAcceptanceRevision() == 0 || d.GetDecisionRevision() == 0 || proto.Size(d) > 16<<10) {
			return bad()
		}
	}
	return nil
}

func validCarrierPlan(p *pb.CarrierPlan) bool {
	if p == nil || p.Reservation == nil || proto.Size(p) > 16<<10 {
		return false
	}
	a := p.Reservation.Actor
	if a == nil || len(a.Realm) == 0 || len(a.Principal) == 0 || len(a.Realm)+len(a.Principal) > 4096 {
		return false
	}
	if p.Refusal != nil && (p.Refusal.GetRevision() == 0 || p.Refusal.At == nil || p.PermittedAt == nil || p.CancelledAt == nil) {
		return false
	}
	return (p.PermittedAt == nil || p.CancelledAt == nil) == (p.Refusal == nil)
}
