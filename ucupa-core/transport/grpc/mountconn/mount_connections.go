package mountconn

import (
	"context"
	"reflect"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	networkpb "ucupa/shared/bindings/go/protobuf"
	"ucupa/shared/values"
	pb "ucupa/ucupa-core/bindings/go/protobuf"
	corecontract "ucupa/ucupa-core/contract"
	coresdk "ucupa/ucupa-core/sdk/go"
	"ucupa/ucupa-core/transport/grpc/internal/wire"
)

// CommandCommitment is the only mount-domain rule needed by this outbound
// adapter. The implementation is supplied by the same fact owner as source.
type CommandCommitment interface {
	AuthorityRef() values.FactAuthorityRef
	CommandCommitment(values.FactAuthorityRef, values.FactAuthorityRef, corecontract.OperationID, corecontract.MountCommand) ([32]byte, error)
}

// MountConnections adapts explicitly installed pinned clients to the Core's
// peer port. The map is keyed only by the relationship's specified address;
// it never searches by authority or falls back to a local Core object.
type MountConnections struct {
	source     values.FactAuthorityRef
	commitment CommandCommitment
	peers      map[values.NextHop]*coresdk.Client
	approval   *pb.MountApproval
}

func NewMountConnections(source values.FactAuthorityRef, commitment CommandCommitment, clients []*coresdk.Client) (*MountConnections, error) {
	if !source.Valid() || nilCommitment(commitment) || commitment.AuthorityRef() != source {
		return nil, corecontract.ErrInvalid
	}
	p := &MountConnections{source: source, commitment: commitment, peers: make(map[values.NextHop]*coresdk.Client, len(clients))}
	for _, client := range clients {
		if client == nil {
			return nil, corecontract.ErrInvalid
		}
		address, target := client.Target()
		if target == source || p.peers[address] != nil {
			return nil, corecontract.ErrInvalid
		}
		p.peers[address] = client
	}
	return p, nil
}

func nilCommitment(commitment CommandCommitment) bool {
	if commitment == nil {
		return true
	}
	v := reflect.ValueOf(commitment)
	return v.Kind() == reflect.Pointer && v.IsNil()
}

// ConfiguredFor accepts only a source-matched, approval-free connector for
// installation in the server configuration.
func (p *MountConnections) ConfiguredFor(expected values.FactAuthorityRef) bool {
	return p != nil && p.source == expected && p.approval == nil && !nilCommitment(p.commitment) && p.commitment.AuthorityRef() == expected
}

// WithApproval creates a single invocation's immutable binding. It does not
// install a process-wide grant or change what autonomous recovery may prepare.
func (p *MountConnections) WithApproval(approval *pb.MountApproval) (*MountConnections, error) {
	if p == nil || approval == nil || !wire.ValidApprovalBinding(approval.Binding) || approval.Binding.GetActor() != string(p.source) || !values.FactAuthorityRef(approval.GetTarget()).Valid() || approval.GetTarget() == string(p.source) || len(approval.Token) != 32 || approval.GetExpiresInSeconds() == 0 {
		return nil, corecontract.ErrInvalid
	}
	return &MountConnections{source: p.source, commitment: p.commitment, peers: p.peers, approval: proto.Clone(approval).(*pb.MountApproval)}, nil
}

func (p *MountConnections) peer(ctx context.Context, address values.NextHop, target values.FactAuthorityRef) (*coresdk.Client, error) {
	if p == nil || ctx == nil || ctx.Err() != nil {
		return nil, corecontract.ErrUnavailable
	}
	client := p.peers[address]
	if client == nil {
		return nil, corecontract.ErrUnavailable
	}
	actualAddress, actualTarget := client.Target()
	if actualAddress != address || actualTarget != target {
		return nil, corecontract.ErrOwnerMismatch
	}
	return client, nil
}

func mountRemoteError(err error, failure networkpb.Failure) error {
	if err != nil {
		if status.Code(err) == codes.DataLoss {
			return corecontract.ErrOwnerMismatch
		}
		return corecontract.ErrUnavailable
	}
	switch failure {
	case networkpb.Failure_INVALID:
		return corecontract.ErrInvalid
	case networkpb.Failure_DENIED:
		return values.PublicDenied
	case networkpb.Failure_INVALID_CONTEXT:
		return values.PublicInvalidContext
	case networkpb.Failure_RESOURCE_EXHAUSTED:
		return values.PublicResourceExhausted
	case networkpb.Failure_NOT_FOUND:
		return corecontract.ErrNotFound
	case networkpb.Failure_CONFLICT:
		return corecontract.ErrConflict
	default:
		return corecontract.ErrUnavailable
	}
}

func (p *MountConnections) exactApproval(target values.FactAuthorityRef, id corecontract.OperationID, command corecontract.MountCommand) (*pb.CheckMountApprovalRequest, error) {
	if p == nil || p.approval == nil {
		return nil, corecontract.ErrUnavailable
	}
	a := p.approval
	if nilCommitment(p.commitment) || p.commitment.AuthorityRef() != p.source {
		return nil, corecontract.ErrOwnerMismatch
	}
	digest, err := p.commitment.CommandCommitment(p.source, target, id, command)
	if p.commitment.AuthorityRef() != p.source {
		return nil, corecontract.ErrOwnerMismatch
	}
	if err != nil || a.GetTarget() != string(target) || a.Binding.GetElement() != string(command.MountAnchorUUID) || string(a.Binding.OperationId) != string(id) || digest != [32]byte(a.Binding.CommandDigest) {
		return nil, corecontract.ErrInvalid
	}
	return &pb.CheckMountApprovalRequest{Binding: proto.Clone(a.Binding).(*pb.MountApprovalBinding), Token: append([]byte(nil), a.Token...)}, nil
}

func (p *MountConnections) CheckApproval(ctx context.Context, address values.NextHop, target values.FactAuthorityRef, id corecontract.OperationID, command corecontract.MountCommand) (values.PrincipalRef, error) {
	client, err := p.peer(ctx, address, target)
	if err != nil {
		return values.PrincipalRef{}, err
	}
	in, err := p.exactApproval(target, id, command)
	if err != nil {
		return values.PrincipalRef{}, err
	}
	out, err := client.MountApprovals().CheckMountApproval(ctx, in)
	if err != nil || out.GetResult() == nil {
		return values.PrincipalRef{}, mountRemoteError(err, out.GetFailure())
	}
	r := out.GetResult()
	return values.PrincipalRef{Realm: values.RealmID(r.GetRealm()), Principal: values.PrincipalID(r.GetPrincipal())}, nil
}

func (p *MountConnections) Prepare(ctx context.Context, address values.NextHop, target values.FactAuthorityRef, request corecontract.MountPrepareRequest) (corecontract.MountPrepared, error) {
	client, err := p.peer(ctx, address, target)
	if err != nil {
		return corecontract.MountPrepared{}, err
	}
	if request.Key.ParentAuthority != p.source || request.Key.CarrierAuthority != target {
		return corecontract.MountPrepared{}, corecontract.ErrOwnerMismatch
	}
	approval, err := p.exactApproval(target, request.OperationID, request.Command)
	if err != nil {
		return corecontract.MountPrepared{}, err
	}
	out, err := client.MountPeer().PrepareMount(ctx, &pb.PrepareMountRequest{Request: wire.MountRequestWire(request), Approval: approval})
	if err != nil || out.GetResult() == nil {
		return corecontract.MountPrepared{}, mountRemoteError(err, out.GetFailure())
	}
	r := out.GetResult()
	decoded, err := wire.MountRequest(r.Request)
	if err != nil || decoded != request {
		return corecontract.MountPrepared{}, corecontract.ErrOwnerMismatch
	}
	cut, err := wire.MountCut(r.Cut)
	if err != nil || cut == (corecontract.CoreReadCut{}) {
		return corecontract.MountPrepared{}, corecontract.ErrOwnerMismatch
	}
	return corecontract.MountPrepared{Request: request, Prepared: r.GetPrepared(), Rejection: corecontract.MountAbortCause(r.GetRejection() - 1), Cut: cut}, nil
}

func (p *MountConnections) Finish(ctx context.Context, address values.NextHop, target values.FactAuthorityRef, request corecontract.MountFinishRequest) (corecontract.MountClosed, error) {
	client, err := p.peer(ctx, address, target)
	if err != nil {
		return corecontract.MountClosed{}, err
	}
	if request.Evidence.Request.Key.ParentAuthority != p.source || request.Evidence.Request.Key.CarrierAuthority != target {
		return corecontract.MountClosed{}, corecontract.ErrOwnerMismatch
	}
	out, err := client.MountPeer().FinishMount(ctx, &pb.FinishMountRequest{Evidence: wire.MountDecisionWire(request.Evidence)})
	if err != nil || out.GetResult() == nil {
		return corecontract.MountClosed{}, mountRemoteError(err, out.GetFailure())
	}
	r := out.GetResult()
	evidence, err := wire.MountDecision(r.Evidence)
	if err != nil || evidence != request.Evidence {
		return corecontract.MountClosed{}, corecontract.ErrOwnerMismatch
	}
	cut, err := wire.MountCut(r.Cut)
	if err != nil || cut == (corecontract.CoreReadCut{}) {
		return corecontract.MountClosed{}, corecontract.ErrOwnerMismatch
	}
	return corecontract.MountClosed{Evidence: evidence, Cut: cut}, nil
}

func (p *MountConnections) ReadDecision(ctx context.Context, address values.NextHop, target values.FactAuthorityRef, key corecontract.MountAttemptKey) (corecontract.MountDecisionEvidence, error) {
	client, err := p.peer(ctx, address, target)
	if err != nil {
		return corecontract.MountDecisionEvidence{}, err
	}
	if key.ParentAuthority != target {
		return corecontract.MountDecisionEvidence{}, corecontract.ErrOwnerMismatch
	}
	out, err := client.MountPeer().ReadMountDecision(ctx, &pb.ReadMountDecisionRequest{ParentCore: proto.String(string(target)), Actor: proto.String(string(p.source)), OperationId: []byte(key.OperationID)})
	if err != nil || out.GetResult() == nil {
		return corecontract.MountDecisionEvidence{}, mountRemoteError(err, out.GetFailure())
	}
	evidence, err := wire.MountDecision(out.GetResult())
	if err != nil || evidence.Request.Key.ParentAuthority != target || evidence.Request.Key.CarrierAuthority != p.source || evidence.Request.OperationID != key.OperationID {
		return corecontract.MountDecisionEvidence{}, corecontract.ErrOwnerMismatch
	}
	return evidence, nil
}

var _ corecontract.MountControlTransport = (*MountConnections)(nil)
