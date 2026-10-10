package coresdk

import (
	"bytes"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	netpb "ucupa/shared/bindings/go/protobuf"
	"ucupa/shared/values"
	pb "ucupa/ucupa-core/bindings/go/protobuf"
)

// Request and response checks bind the transport target to Parent, not Source.
// No permission, relation or placement decision is implemented in the SDK.
func validateCarrierRequest(in any, owner values.FactAuthorityRef) error {
	var id []byte
	var mount string
	comparison := func(t *netpb.Token, kind netpb.TokenKind) bool {
		return t != nil && t.GetOwner() == string(owner) && t.GetKind() == kind && len(t.Value) == 32
	}
	bad := func() error { return status.Error(codes.InvalidArgument, "invalid carrier request") }
	switch r := in.(type) {
	case *pb.BeginCarrierTransitionRequest:
		id, mount = r.GetOperationId(), r.GetMount()
		source, target := values.FactAuthorityRef(r.GetSource()), values.FactAuthorityRef(r.GetTarget())
		if proto.Size(r) > 4096 || !source.Valid() || !target.Valid() || source == target || source == owner || target == owner ||
			!comparison(r.GetExpectedElement(), netpb.TokenKind_RECORD_REVISION) || !comparison(r.GetExpectedMount(), netpb.TokenKind_MOUNT_REVISION) ||
			!values.NextHop(r.GetParentAddress()).Valid() || !values.NextHop(r.GetSourceAddress()).Valid() || !values.NextHop(r.GetTargetAddress()).Valid() {
			return bad()
		}
	case *pb.InspectCarrierTransitionRequest:
		id, mount = r.GetOperationId(), r.GetMount()
	case *pb.CancelCarrierTransitionRequest:
		id, mount = r.GetOperationId(), r.GetMount()
		if !comparison(r.GetExpected(), netpb.TokenKind_CARRIER_REVISION) {
			return bad()
		}
	case *pb.RecoverCarrierTransitionRequest:
		b := r.GetBinding()
		c := b.GetCommand()
		if b.GetParent() != string(owner) || len(r.GetSourceToken()) != 32 || len(r.GetTargetToken()) != 32 || proto.Size(r) > 8192 {
			return bad()
		}
		return validateCarrierRequest(c, owner)
	default:
		return nil
	}
	if !owner.Valid() || len(id) == 0 || len(id) > 128 || !values.ElementID(mount).Valid() {
		return bad()
	}
	return nil
}

// carrierEvidenceBinding extracts the parent-signed binding embedded in the
// carrier evidence messages that cross the source, target and parent channels.
func carrierEvidenceBinding(m protoreflect.Message) *pb.CarrierApprovalBinding {
	switch v := m.Interface().(type) {
	case *pb.CarrierPlan:
		return v.GetBinding()
	case *pb.CarrierSourceChunk:
		return v.GetBinding()
	case *pb.CarrierTargetAcceptance:
		return v.GetBinding()
	case *pb.ServiceCarrierFence:
		return v.GetPermit().GetBinding()
	case *pb.CarrierDecision:
		return v.GetPlan().GetBinding()
	case *pb.CarrierCompletion:
		return v.GetDecision().GetPlan().GetBinding()
	}
	return nil
}

// validateCarrierApprovalOwners handles the approval and evidence messages that
// echo the parent-signed binding command. Its revision tokens belong to the
// parent, not to the source, target or parent host of the authenticated channel.
func validateCarrierApprovalOwners(m protoreflect.Message, owner values.FactAuthorityRef) (bool, error) {
	var approval interface {
		GetIssuer() string
		GetBinding() *pb.CarrierApprovalBinding
	}
	switch v := m.Interface().(type) {
	case *pb.CarrierApproval:
		approval = v
	case *pb.CarrierApprovalCheck:
		approval = v
	case *pb.CarrierImportPreflight:
		approval = v
	}
	var binding *pb.CarrierApprovalBinding
	checkIssuer := false
	if approval != nil {
		binding, checkIssuer = approval.GetBinding(), true
	} else {
		binding = carrierEvidenceBinding(m)
	}
	if binding == nil {
		return false, nil
	}
	command := binding.GetCommand()
	parent, source, target := values.FactAuthorityRef(binding.GetParent()), values.FactAuthorityRef(command.GetSource()), values.FactAuthorityRef(command.GetTarget())
	if !parent.Valid() || parent == source || parent == target || !source.Valid() || !target.Valid() || source == target ||
		len(command.GetOperationId()) == 0 || len(command.GetOperationId()) > 128 || !values.ElementID(command.GetMount()).Valid() {
		return true, status.Error(codes.DataLoss, "invalid carrier approval binding")
	}
	if checkIssuer && (approval.GetIssuer() != string(owner) || (owner != source && owner != target)) {
		return true, status.Error(codes.DataLoss, "invalid carrier approval binding")
	}
	return true, validateTokenOwners(command.ProtoReflect(), parent)
}

func validateCarrierExchange(in, out any, owner values.FactAuthorityRef) error {
	r, ok := out.(*pb.CarrierTransitionResponse)
	if !ok {
		return nil
	}
	var id []byte
	var mount string
	var begin *pb.BeginCarrierTransitionRequest
	cancel := false
	switch q := in.(type) {
	case *pb.BeginCarrierTransitionRequest:
		id, mount, begin = q.GetOperationId(), q.GetMount(), q
	case *pb.InspectCarrierTransitionRequest:
		id, mount = q.GetOperationId(), q.GetMount()
	case *pb.CancelCarrierTransitionRequest:
		id, mount, cancel = q.GetOperationId(), q.GetMount(), true
	case *pb.RecoverCarrierTransitionRequest:
		c := q.GetBinding().GetCommand()
		id, mount = c.GetOperationId(), c.GetMount()
	default:
		return status.Error(codes.DataLoss, "unexpected carrier response")
	}
	if r.GetResult() == nil {
		return nil
	}
	v := r.GetResult()
	if len(id) == 0 || len(id) > 128 || !values.ElementID(mount).Valid() || v.GetParent() != string(owner) || !bytes.Equal(v.OperationId, id) || v.GetMount() != mount ||
		!values.FactAuthorityRef(v.GetSource()).Valid() || !values.FactAuthorityRef(v.GetTarget()).Valid() || v.GetSource() == v.GetTarget() || v.GetSource() == string(owner) || v.GetTarget() == string(owner) ||
		(begin != nil && (v.GetSource() != begin.GetSource() || v.GetTarget() != begin.GetTarget() || v.GetProgress() != pb.CarrierProgress_CARRIER_RESERVED)) ||
		(cancel && v.GetProgress() != pb.CarrierProgress_CARRIER_CANCELLED) {
		return status.Error(codes.DataLoss, "carrier response does not match request")
	}
	return nil
}
