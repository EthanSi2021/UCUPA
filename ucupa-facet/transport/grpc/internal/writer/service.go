// Package writer adapts writer lifecycle RPCs, without Store or owner internals.
package writer

import (
	"context"
	"errors"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	netpb "ucupa/shared/bindings/go/protobuf"
	"ucupa/shared/values"
	pb "ucupa/ucupa-facet/bindings/go/protobuf"
)

type Owner interface {
	AuthorityRef() values.FactAuthorityRef
	InspectWriter(context.Context, values.CallerContext, string) (values.WriterStatus, error)
	HandoffWriter(context.Context, values.CallerContext, values.WriterHandoffRequest) (values.WriterStatus, error)
}
type Callers interface {
	Caller(context.Context) (values.CallerContext, error)
	CheckWriterCaller(context.Context, values.CallerContext) error
}
type Service struct {
	pb.UnimplementedWriterControlServiceServer
	owner    Owner
	expected values.FactAuthorityRef
	callers  Callers
}

func New(owner Owner, expected values.FactAuthorityRef, callers Callers) *Service {
	return &Service{owner: owner, expected: expected, callers: callers}
}
func (s *Service) caller(ctx context.Context) (values.CallerContext, error) {
	if s.owner == nil {
		return values.CallerContext{}, status.Error(codes.Unimplemented, "writer control unavailable")
	}
	if s.owner.AuthorityRef() != s.expected {
		return values.CallerContext{}, status.Error(codes.Unavailable, "owner changed")
	}
	return s.callers.Caller(ctx)
}
func (s *Service) InspectWriter(ctx context.Context, r *netpb.WriterInspectRequest) (*netpb.WriterResponse, error) {
	c, e := s.caller(ctx)
	if e != nil {
		return nil, e
	}
	if r == nil || len(r.GetOperation()) > 128 {
		return nil, status.Error(codes.InvalidArgument, "invalid writer operation")
	}
	out, e := s.owner.InspectWriter(ctx, c, string(r.GetOperation()))
	if err := s.callers.CheckWriterCaller(ctx, c); err != nil {
		return nil, err
	}
	return response(out, e)
}
func (s *Service) HandoffWriter(ctx context.Context, r *netpb.WriterHandoffRequest) (*netpb.WriterResponse, error) {
	c, e := s.caller(ctx)
	if e != nil {
		return nil, e
	}
	if r == nil || len(r.GetTarget()) != 32 {
		return nil, status.Error(codes.InvalidArgument, "invalid writer target")
	}
	req := values.WriterHandoffRequest{Operation: string(r.GetOperation()), ExpectedEpoch: r.GetExpectedEpoch(), Target: [32]byte(r.GetTarget())}
	if !req.Valid() {
		return nil, status.Error(codes.InvalidArgument, "invalid writer operation")
	}
	out, e := s.owner.HandoffWriter(ctx, c, req)
	if err := s.callers.CheckWriterCaller(ctx, c); err != nil {
		return nil, err
	}
	return response(out, e)
}
func response(s values.WriterStatus, err error) (*netpb.WriterResponse, error) {
	if err != nil {
		var p values.PublicError
		if errors.As(err, &p) {
			if n, ok := netpb.Failure_value[string(p)]; ok && n != 0 {
				return &netpb.WriterResponse{Outcome: &netpb.WriterResponse_Error{Error: netpb.Failure(n)}}, nil
			}
		}
		return nil, status.Error(codes.Internal, "writer result unavailable")
	}
	if !s.Owner.Valid() || s.Epoch == 0 {
		return nil, status.Error(codes.Internal, "invalid writer result")
	}
	out := &netpb.WriterStatus{Owner: proto.String(string(s.Owner)), Epoch: proto.Uint64(s.Epoch), Pending: proto.Bool(s.Pending)}
	if d := s.Decision; d != nil {
		out.Decision = &netpb.WriterDecision{Operation: []byte(d.Operation), ClosedEpoch: proto.Uint64(d.ClosedEpoch), FinalCut: append([]byte(nil), d.FinalCut[:]...), Installed: proto.Bool(d.Installed)}
	}
	return &netpb.WriterResponse{Outcome: &netpb.WriterResponse_Result{Result: out}}, nil
}
