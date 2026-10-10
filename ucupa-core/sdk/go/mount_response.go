package coresdk

import (
	"bytes"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"ucupa/shared/values"
	pb "ucupa/ucupa-core/bindings/go/protobuf"
)

// Binding checks complement structural decoding; they do not adjudicate the
// mount transition. A correctly shaped response to another attempt is invalid.
func validateMountExchange(in, out any, target, actor values.FactAuthorityRef) error {
	bad := func() error { return status.Error(codes.DataLoss, "mount response does not match request") }
	switch request := in.(type) {
	case *pb.InspectMountContactRequest:
		response, ok := out.(*pb.InspectMountContactResponse)
		if !ok {
			return bad()
		}
		if r := response.GetResult(); r != nil && (r.GetElement() != request.GetElement() || r.GetIncoming() != request.GetIncoming() || !values.FactAuthorityRef(r.GetPeerCore()).Valid() || r.GetPeerCore() == string(target)) {
			return bad()
		}
	case *pb.RepairMountContactRequest:
		response, ok := out.(*pb.RepairMountContactResponse)
		if !ok {
			return bad()
		}
		if r := response.GetResult(); r != nil && (r.GetElement() != request.GetElement() || r.GetIncoming() != request.GetIncoming() || r.GetNextHop() != request.GetNextHop() || !values.FactAuthorityRef(r.GetPeerCore()).Valid() || r.GetPeerCore() == string(target)) {
			return bad()
		}
	case *pb.InspectMountRequest:
		response, ok := out.(*pb.InspectMountResponse)
		if !ok {
			return bad()
		}
		if r := response.GetResult(); r != nil {
			if r.GetElement() != request.GetElement() || r.GetState() > 4 {
				return bad()
			}
		}
	case *pb.DescribeMountRequest:
		response, ok := out.(*pb.DescribeMountResponse)
		if !ok {
			return bad()
		}
		if r := response.GetResult(); r != nil {
			if !r.GetRequired() {
				if r.Target != nil || r.Binding != nil {
					return bad()
				}
			} else if r.Binding == nil || !values.FactAuthorityRef(r.GetTarget()).Valid() || r.GetTarget() == string(target) || r.Binding.GetActor() != string(target) || r.Binding.GetElement() != request.GetCommand().GetElement() || !bytes.Equal(r.Binding.OperationId, request.OperationId) || len(r.Binding.CommandDigest) != 32 {
				return bad()
			}
		}
	case *pb.ApplyMountRequest:
		response, ok := out.(*pb.ApplyMountResponse)
		if !ok {
			return bad()
		}
		if r := response.GetResult(); r != nil {
			if r.GetElement() != request.GetCommand().GetElement() || !bytes.Equal(r.OperationId, request.OperationId) || r.GetBlockedBy() > 3 || r.GetDecision() == pb.MountDecisionKind_MOUNT_PENDING && r.GetSettled() || r.GetDecision() != pb.MountDecisionKind_MOUNT_ABORTED && r.GetRejection() != pb.MountRejection_MOUNT_NO_REJECTION {
				return bad()
			}
		}
	case *pb.PrepareMountRequest:
		response, ok := out.(*pb.PrepareMountResponse)
		if !ok {
			return bad()
		}
		if r := response.GetResult(); r != nil {
			if !proto.Equal(r.Request, request.Request) || r.Request.GetKey().GetCarrierCore() != string(target) || r.Request.GetKey().GetParentCore() != string(actor) || r.GetPrepared() != (r.GetRejection() == pb.MountRejection_MOUNT_NO_REJECTION) {
				return bad()
			}
		}
	case *pb.FinishMountRequest:
		response, ok := out.(*pb.FinishMountResponse)
		if !ok {
			return bad()
		}
		if r := response.GetResult(); r != nil {
			if !proto.Equal(r.Evidence, request.Evidence) || r.Evidence.GetRequest().GetKey().GetCarrierCore() != string(target) || r.Evidence.GetRequest().GetKey().GetParentCore() != string(actor) {
				return bad()
			}
		}
	case *pb.ReadMountDecisionRequest:
		response, ok := out.(*pb.ReadMountDecisionResponse)
		if !ok {
			return bad()
		}
		if r := response.GetResult(); r != nil {
			if r.Request.GetKey().GetParentCore() != string(target) || r.Request.GetKey().GetCarrierCore() != string(actor) || request.GetParentCore() != string(target) || request.GetActor() != string(actor) || !bytes.Equal(r.Request.GetOperationId(), request.OperationId) {
				return bad()
			}
		}
	}
	return nil
}
