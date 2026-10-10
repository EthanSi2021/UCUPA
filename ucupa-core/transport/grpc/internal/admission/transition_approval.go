package admission

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	"ucupa/shared/values"
	pb "ucupa/ucupa-core/bindings/go/protobuf"
	"ucupa/ucupa-core/contract"
	"ucupa/ucupa-core/transport/grpc/internal/rpcfailure"
	grpcsession "ucupa/ucupa-core/transport/grpc/internal/session"
	"ucupa/ucupa-core/transport/grpc/internal/wire"
)

// This independently enabled policy shares existing service-key trust, not
// Mount permissions. Runtime approvals never replace a durable acceptance.
type TransitionApprovalConfig struct {
	MaxTTL       time.Duration
	MaxApprovals int
	MaxPerCaller int
}

type TransitionImportOwner interface {
	AuthorityRef() values.FactAuthorityRef
	AuthorizeTransitionImport(context.Context, values.CallerContext, contract.TransitionApprovalBinding) (values.AuthorizationView, error)
	CheckTransitionImport(context.Context, values.CallerContext, contract.TransitionApprovalBinding, values.AuthorizationView) error
}

type transitionApproval struct {
	caller  values.CallerContext
	view    values.AuthorizationView
	binding contract.TransitionApprovalBinding
	session EntrySession
	expires time.Time
}

func ValidateTransitionApprovals(c *TransitionApprovalConfig, peers *ReadDelegationConfig, owner TransitionImportOwner) error {
	if c == nil {
		return nil
	}
	if peers == nil || owner == nil || c.MaxTTL <= 0 || c.MaxTTL%time.Second != 0 || c.MaxApprovals <= 0 || c.MaxPerCaller <= 0 || c.MaxPerCaller > c.MaxApprovals {
		return errors.New("invalid transition approval configuration")
	}
	return nil
}

func (s *Service) expireTransitionApprovalsLocked(now time.Time) {
	for key, a := range s.transitionApprovals {
		if !now.Before(a.expires) {
			delete(s.transitionApprovals, key)
		}
	}
}

func (s *Service) transitionBinding(b *pb.TransitionApprovalBinding) (contract.TransitionApprovalBinding, error) {
	if b == nil || proto.Size(b) > 4096 {
		return contract.TransitionApprovalBinding{}, status.Error(codes.InvalidArgument, "invalid transition binding")
	}
	return wire.TransitionBinding(b, s.config.Expected)
}

func (s *Service) CreateTransitionApproval(ctx context.Context, in *pb.CreateTransitionApprovalRequest) (*pb.CreateTransitionApprovalResponse, error) {
	c := s.config.TransitionApprovals
	if c == nil {
		return nil, status.Error(codes.Unimplemented, "transition approvals disabled")
	}
	b, err := s.transitionBinding(in.GetBinding())
	if err != nil {
		return nil, err
	}
	if in.GetTtlSeconds() == 0 || in.GetTtlSeconds() > uint64(c.MaxTTL/time.Second) {
		return nil, status.Error(codes.InvalidArgument, "invalid transition approval lifetime")
	}
	if _, ok := s.config.ReadDelegation.Peers[b.Source]; !ok {
		return nil, status.Error(codes.Unauthenticated, "unknown source")
	}
	caller, ok := grpcsession.Caller(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "session required")
	}
	session, err := s.BindEntrySession(ctx, caller)
	if err != nil {
		return nil, status.Error(codes.Unauthenticated, "session unavailable")
	}
	view, err := s.config.TransitionImport.AuthorizeTransitionImport(ctx, caller, b)
	if err != nil {
		f, e := rpcfailure.Transition(err, false)
		return &pb.CreateTransitionApprovalResponse{Outcome: &pb.CreateTransitionApprovalResponse_Failure{Failure: f}}, e
	}
	var token [32]byte
	if _, err := rand.Read(token[:]); err != nil {
		return nil, status.Error(codes.Unavailable, "approval unavailable")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	s.expireTransitionApprovalsLocked(now)
	current, live := s.sessions[session.key]
	if s.stopped || !live || current != session.entry || !now.Before(current.expires) || ctx.Err() != nil {
		return nil, status.Error(codes.Unauthenticated, "session unavailable")
	}
	count := 0
	for _, a := range s.transitionApprovals {
		if a.caller.Subject == caller.Subject {
			count++
		}
	}
	if len(s.transitionApprovals) >= c.MaxApprovals || count >= c.MaxPerCaller {
		return nil, status.Error(codes.ResourceExhausted, "approval capacity")
	}
	key := sha256.Sum256(token[:])
	if _, exists := s.transitionApprovals[key]; exists {
		return nil, status.Error(codes.Unavailable, "approval unavailable")
	}
	expires := now.Add(time.Duration(in.GetTtlSeconds()) * time.Second)
	if session.Expires().Before(expires) {
		expires = session.Expires()
	}
	s.transitionApprovals[key] = transitionApproval{caller: caller, view: view, binding: b, session: session, expires: expires}
	seconds := uint64((expires.Sub(now)-1)/time.Second) + 1
	return &pb.CreateTransitionApprovalResponse{Outcome: &pb.CreateTransitionApprovalResponse_Result{Result: &pb.TransitionApproval{
		Target: proto.String(string(s.config.Expected)), Binding: proto.Clone(in.Binding).(*pb.TransitionApprovalBinding), Token: token[:], ExpiresInSeconds: proto.Uint64(seconds),
	}}}, nil
}

// Accept consumes this retained caller/view and exact binding again at
// the target transaction. A successful Check is not an offline write permit.
func (s *Service) checkedTransitionApproval(ctx context.Context, in *pb.CheckTransitionApprovalRequest) (transitionApproval, error) {
	if s.config.TransitionApprovals == nil {
		return transitionApproval{}, status.Error(codes.Unimplemented, "transition approvals disabled")
	}
	b, err := s.transitionBinding(in.GetBinding())
	if err != nil {
		return transitionApproval{}, err
	}
	if len(in.GetToken()) != 32 {
		return transitionApproval{}, status.Error(codes.InvalidArgument, "invalid approval token")
	}
	md, _ := metadata.FromIncomingContext(ctx)
	if len(md.Get("authorization")) != 0 || !s.authenticatedActor(ctx, b.Source) {
		return transitionApproval{}, status.Error(codes.Unauthenticated, "authenticated source required")
	}
	key := sha256.Sum256(in.Token)
	s.mu.Lock()
	s.expireTransitionApprovalsLocked(time.Now())
	a, ok := s.transitionApprovals[key]
	stopped := s.stopped
	s.mu.Unlock()
	if stopped || !ok || a.binding != b {
		return transitionApproval{}, status.Error(codes.Unauthenticated, "approval unavailable")
	}
	if err := s.config.TransitionImport.CheckTransitionImport(ctx, a.caller, a.binding, a.view); err != nil {
		return transitionApproval{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	current, ok := s.transitionApprovals[key]
	session, live := s.sessions[a.session.key]
	if s.stopped || !ok || current != a || !live || session != a.session.entry || !time.Now().Before(a.expires) || ctx.Err() != nil {
		return transitionApproval{}, status.Error(codes.Unauthenticated, "approval unavailable")
	}
	return a, nil
}

func (s *Service) CheckTransitionApproval(ctx context.Context, in *pb.CheckTransitionApprovalRequest) (*pb.CheckTransitionApprovalResponse, error) {
	a, err := s.checkedTransitionApproval(ctx, in)
	if err != nil {
		if _, ok := status.FromError(err); ok {
			return nil, err
		}
		f, e := rpcfailure.Transition(err, false)
		return &pb.CheckTransitionApprovalResponse{Outcome: &pb.CheckTransitionApprovalResponse_Failure{Failure: f}}, e
	}
	return &pb.CheckTransitionApprovalResponse{Outcome: &pb.CheckTransitionApprovalResponse_Result{Result: &pb.TransitionApprovalCheck{
		Target: proto.String(string(s.config.Expected)), Binding: proto.Clone(in.Binding).(*pb.TransitionApprovalBinding),
		Realm: proto.String(string(a.caller.Subject.Realm)), Principal: proto.String(string(a.caller.Subject.Principal)),
	}}}, nil
}

func (s *Service) RevokeTransitionApproval(ctx context.Context, in *pb.RevokeTransitionApprovalRequest) (*pb.RevokeTransitionApprovalResponse, error) {
	if s.config.TransitionApprovals == nil {
		return nil, status.Error(codes.Unimplemented, "transition approvals disabled")
	}
	if len(in.GetToken()) != 32 {
		return nil, status.Error(codes.InvalidArgument, "invalid approval token")
	}
	caller, ok := grpcsession.Caller(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "session required")
	}
	session, err := s.BindEntrySession(ctx, caller)
	if err != nil {
		return nil, status.Error(codes.Unauthenticated, "session unavailable")
	}
	if err := s.checkCaller(ctx, caller); err != nil {
		return nil, identityFailure(err)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	current, live := s.sessions[session.key]
	if s.stopped || !live || current != session.entry || !time.Now().Before(current.expires) || ctx.Err() != nil {
		return nil, status.Error(codes.Unauthenticated, "session unavailable")
	}
	s.expireTransitionApprovalsLocked(time.Now())
	key := sha256.Sum256(in.Token)
	if a, exists := s.transitionApprovals[key]; exists && a.caller.Subject == caller.Subject {
		delete(s.transitionApprovals, key)
	}
	return &pb.RevokeTransitionApprovalResponse{Released: proto.Bool(true)}, nil
}
