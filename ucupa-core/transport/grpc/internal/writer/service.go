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
	pb "ucupa/ucupa-core/bindings/go/protobuf"
	"ucupa/ucupa-core/transport/grpc/internal/admission"
	"ucupa/ucupa-core/transport/grpc/internal/session"
)

type Owner interface {
	AuthorityRef() values.FactAuthorityRef
	InspectWriter(context.Context, values.CallerContext, string) (values.WriterStatus, error)
	HandoffWriter(context.Context, values.CallerContext, values.WriterHandoffRequest) (values.WriterStatus, error)
}

type Service struct {
	pb.UnimplementedWriterControlServiceServer
	owner    Owner
	expected values.FactAuthorityRef
	sessions Sessions
}

type Sessions interface {
	BindEntrySession(context.Context, values.CallerContext) (admission.EntrySession, error)
	CheckEntrySession(admission.EntrySession) error
}

func New(owner Owner, expected values.FactAuthorityRef, sessions Sessions) *Service {
	return &Service{owner: owner, expected: expected, sessions: sessions}
}
func (s *Service) caller(ctx context.Context) (values.CallerContext, error) {
	if s.owner == nil {
		return values.CallerContext{}, status.Error(codes.Unimplemented, "writer control unavailable")
	}
	if s.owner.AuthorityRef() != s.expected {
		return values.CallerContext{}, status.Error(codes.Unavailable, "owner changed")
	}
	if _, ok := session.Grant(ctx); ok {
		return values.CallerContext{}, status.Error(codes.PermissionDenied, "delegation is not writer approval")
	}
	c, ok := session.Caller(ctx)
	if !ok {
		return c, status.Error(codes.Unauthenticated, "session required")
	}
	return c, nil
}
func (s *Service) InspectWriter(ctx context.Context, r *netpb.WriterInspectRequest) (*netpb.WriterResponse, error) {
	c, e := s.caller(ctx)
	if e != nil {
		return nil, e
	}
	if r == nil || len(r.GetOperation()) > 128 {
		return nil, status.Error(codes.InvalidArgument, "invalid writer operation")
	}
	entry, e := s.sessions.BindEntrySession(ctx, c)
	if e != nil {
		return nil, status.Error(codes.Unauthenticated, "session unavailable")
	}
	out, e := s.owner.InspectWriter(ctx, c, string(r.GetOperation()))
	if ctx.Err() != nil {
		return nil, status.FromContextError(ctx.Err()).Err()
	}
	if s.sessions.CheckEntrySession(entry) != nil {
		return nil, status.Error(codes.Unauthenticated, "session unavailable")
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
	entry, e := s.sessions.BindEntrySession(ctx, c)
	if e != nil {
		return nil, status.Error(codes.Unauthenticated, "session unavailable")
	}
	out, e := s.owner.HandoffWriter(ctx, c, req)
	if ctx.Err() != nil {
		return nil, status.FromContextError(ctx.Err()).Err()
	}
	if s.sessions.CheckEntrySession(entry) != nil {
		return nil, status.Error(codes.Unauthenticated, "session unavailable")
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
