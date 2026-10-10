package coresdk

import (
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	"ucupa/shared/values"
	pb "ucupa/ucupa-core/bindings/go/protobuf"
)

// Carrier peer requests bind this client to exactly one role of the binding.
// expected is the connected host, actor this client's own TLS identity: the
// parent drives fence/accept/install/settle and checks retained tokens, the
// target pulls the source content, the source reads the target evidence.
func validateCarrierPeerRequest(in any, expected, actor values.FactAuthorityRef) error {
	var b *pb.CarrierApprovalBinding
	role := ""
	switch q := in.(type) {
	case *pb.CheckCarrierApprovalRequest:
		b, role = q.GetBinding(), "parent"
		if len(q.GetToken()) != 32 {
			return status.Error(codes.InvalidArgument, "invalid carrier approval token")
		}
	case *pb.CreateCarrierApprovalRequest:
		b, role = q.GetBinding(), "issue"
		if q.TtlSeconds == nil || q.GetTtlSeconds() == 0 {
			return status.Error(codes.InvalidArgument, "invalid carrier approval lifetime")
		}
	case *pb.RevokeCarrierApprovalRequest:
		if len(q.GetToken()) != 32 {
			return status.Error(codes.InvalidArgument, "invalid carrier approval token")
		}
		return nil
	case *pb.FenceCarrierSourceRequest:
		b, role = q.GetBinding(), "fence"
		if len(q.GetApprovalToken()) != 32 {
			return status.Error(codes.InvalidArgument, "invalid carrier export approval")
		}
	case *pb.ReadCarrierSourceChunkRequest:
		b, role = q.GetBinding(), "pull"
		if q.Offset == nil || q.MaxBytes == nil || q.GetMaxBytes() == 0 || q.GetMaxBytes() > 64<<10 || q.GetOffset() >= 1<<20 {
			return status.Error(codes.InvalidArgument, "invalid carrier chunk bounds")
		}
	case *pb.SettleCarrierSourceRequest:
		b, role = q.GetBinding(), "settle"
	case *pb.CheckCarrierImportRequest:
		b, role = q.GetBinding(), "import"
		if len(q.GetToken()) != 0 && len(q.GetToken()) != 32 {
			return status.Error(codes.InvalidArgument, "invalid carrier import approval")
		}
	case *pb.AcceptCarrierTargetRequest:
		b, role = q.GetBinding(), "accept"
		if len(q.GetApprovalToken()) != 32 {
			return status.Error(codes.InvalidArgument, "invalid carrier import approval")
		}
	case *pb.InstallCarrierTargetRequest:
		b, role = q.GetBinding(), "install"
	case *pb.ReadCarrierCompletionRequest:
		b, role = q.GetBinding(), "completion"
	default:
		return nil
	}
	if b == nil || proto.Size(b) > 4096 || !values.FactAuthorityRef(b.GetParent()).Valid() ||
		b.GetParent() == b.GetCommand().GetSource() || b.GetParent() == b.GetCommand().GetTarget() ||
		b.GetCommand().GetSource() == b.GetCommand().GetTarget() {
		return status.Error(codes.InvalidArgument, "invalid carrier binding")
	}
	c := b.GetCommand()
	source, target, parent := values.FactAuthorityRef(c.GetSource()), values.FactAuthorityRef(c.GetTarget()), values.FactAuthorityRef(b.GetParent())
	switch role {
	case "parent": // the designated parent checks one retained token on either peer host
		if !actor.Valid() || parent != actor || (expected != source && expected != target) {
			return status.Error(codes.Unauthenticated, "wrong carrier peer")
		}
	case "fence", "settle":
		if !actor.Valid() || parent != actor || expected != source {
			return status.Error(codes.Unauthenticated, "wrong carrier peer")
		}
	case "accept", "install":
		if !actor.Valid() || parent != actor || expected != target {
			return status.Error(codes.Unauthenticated, "wrong carrier peer")
		}
	case "pull": // the target carrier pulls the fenced source content
		if !actor.Valid() || expected != source || actor != target {
			return status.Error(codes.Unauthenticated, "wrong carrier peer")
		}
	case "import": // parent strict check or source preflight, both on the target host
		if !actor.Valid() || expected != target || (actor != parent && actor != source) {
			return status.Error(codes.Unauthenticated, "wrong carrier peer")
		}
	case "completion": // the source carrier reads the target's durable completion
		if !actor.Valid() || expected != target || actor != source {
			return status.Error(codes.Unauthenticated, "wrong carrier peer")
		}
	default: // issuance runs on the bound carrier host under the original session
		if expected != source && expected != target {
			return status.Error(codes.Unauthenticated, "wrong carrier host")
		}
	}
	return nil
}

// Structural response checks only; the receiving owner still validates the
// assembled evidence against its own durable transfer state.
func validateCarrierPeerExchange(in, out any, expected values.FactAuthorityRef) error {
	bad := func() error { return status.Error(codes.DataLoss, "carrier evidence does not match request") }
	switch q := in.(type) {
	case *pb.CreateCarrierApprovalRequest:
		r, ok := out.(*pb.CreateCarrierApprovalResponse)
		if !ok || r == nil {
			return bad()
		}
		if v := r.GetResult(); v != nil && (v.GetIssuer() != string(expected) || !proto.Equal(v.Binding, q.Binding) || len(v.Token) != 32 ||
			v.GetExpiresInSeconds() == 0 || v.GetExpiresInSeconds() > q.GetTtlSeconds()) {
			return bad()
		}
	case *pb.CheckCarrierApprovalRequest:
		r, ok := out.(*pb.CheckCarrierApprovalResponse)
		if !ok || r == nil {
			return bad()
		}
		if v := r.GetResult(); v != nil && (v.GetIssuer() != string(expected) || !proto.Equal(v.Binding, q.Binding)) {
			return bad()
		}
	case *pb.RevokeCarrierApprovalRequest:
		if _, ok := out.(*pb.RevokeCarrierApprovalResponse); !ok {
			return bad()
		}
	case *pb.FenceCarrierSourceRequest:
		r, ok := out.(*pb.FenceCarrierSourceResponse)
		if !ok || r == nil {
			return bad()
		}
		if v := r.GetResult(); v != nil && !validCarrierFence(v, q.Binding) {
			return bad()
		}
	case *pb.ReadCarrierSourceChunkRequest:
		r, ok := out.(*pb.ReadCarrierSourceChunkResponse)
		if !ok || r == nil {
			return bad()
		}
		if v := r.GetResult(); v != nil {
			if !proto.Equal(v.Binding, q.Binding) || v.GetTotalBytes() == 0 || v.GetTotalBytes() > 1<<20 ||
				v.GetOffset() != q.GetOffset() || v.GetOffset() >= v.GetTotalBytes() || q.GetMaxBytes() == 0 || q.GetMaxBytes() > 64<<10 ||
				uint64(len(v.Data)) != min(uint64(q.GetMaxBytes()), v.GetTotalBytes()-v.GetOffset()) || !validCarrierFence(v.Fence, q.Binding) {
				return bad()
			}
		}
	case *pb.SettleCarrierSourceRequest:
		r, ok := out.(*pb.SettleCarrierSourceResponse)
		if !ok || r == nil || !r.GetSettled() {
			return bad()
		}
	case *pb.CheckCarrierImportRequest:
		r, ok := out.(*pb.CheckCarrierImportResponse)
		if !ok || r == nil {
			return bad()
		}
		if v := r.GetResult(); v != nil && (v.GetIssuer() != string(expected) || !proto.Equal(v.Binding, q.Binding)) {
			return bad()
		}
		if v := r.GetPreflight(); v != nil && (v.GetIssuer() != string(expected) || !proto.Equal(v.Binding, q.Binding)) {
			return bad()
		}
	case *pb.AcceptCarrierTargetRequest:
		r, ok := out.(*pb.AcceptCarrierTargetResponse)
		if !ok || r == nil {
			return bad()
		}
		if v := r.GetResult(); v != nil && (!proto.Equal(v.Binding, q.Binding) || !validCarrierFence(v.Fence, q.Binding) || v.GetAcceptanceRevision() == 0) {
			return bad()
		}
	case *pb.InstallCarrierTargetRequest:
		r, ok := out.(*pb.InstallCarrierTargetResponse)
		if !ok || r == nil {
			return bad()
		}
		if v := r.GetResult(); v != nil && !validCarrierCompletion(v, q.Binding) {
			return bad()
		}
	case *pb.ReadCarrierCompletionRequest:
		r, ok := out.(*pb.ReadCarrierCompletionResponse)
		if !ok || r == nil {
			return bad()
		}
		if v := r.GetResult(); v != nil && !validCarrierCompletion(v, q.Binding) {
			return bad()
		}
	}
	return nil
}

func validCarrierFence(v *pb.ServiceCarrierFence, b *pb.CarrierApprovalBinding) bool {
	if v == nil || len(v.ManifestDigest) != 32 || v.GetRevision() == 0 || proto.Size(v) > 16<<10 || !proto.Equal(v.GetPermit().GetBinding(), b) {
		return false
	}
	return validCarrierPlan(v.Permit)
}

func validCarrierCompletion(v *pb.CarrierCompletion, b *pb.CarrierApprovalBinding) bool {
	if v == nil || proto.Size(v) > 16<<10 || v.GetInstallationRevision() == 0 || v.GetCompletionRevision() == 0 {
		return false
	}
	d := v.Decision
	return d != nil && proto.Equal(d.GetPlan().GetBinding(), b) && validCarrierPlan(d.Plan) && d.Plan.PermittedAt != nil && d.Plan.CancelledAt == nil &&
		len(d.ManifestDigest) == 32 && d.GetFenceRevision() != 0 && d.GetAcceptanceRevision() != 0 && d.GetDecisionRevision() != 0
}
