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
	corepb "ucupa/ucupa-core/bindings/go/protobuf"
	"ucupa/ucupa-core/transport/grpc/internal/rpcfailure"
	grpcsession "ucupa/ucupa-core/transport/grpc/internal/session"
	"ucupa/ucupa-core/transport/grpc/internal/wire"
)

// MountApprovalConfig is an independent opt-in. Explicit service-key trust is
// shared with ReadDelegation.Peers; no read grant gains a write operation.
type MountApprovalConfig struct {
	MaxTTL       time.Duration
	MaxApprovals int
	MaxPerCaller int
}

type mountApproval struct {
	caller  values.CallerContext
	view    values.AuthorizationView
	binding *corepb.MountApprovalBinding
	expires time.Time
}

func ValidateMountApprovals(c *MountApprovalConfig, peers *ReadDelegationConfig, owner MountOwner) error {
	if c == nil {
		return nil
	}
	if peers == nil || c.MaxTTL <= 0 || c.MaxTTL%time.Second != 0 || c.MaxApprovals <= 0 || c.MaxPerCaller <= 0 || c.MaxPerCaller > c.MaxApprovals {
		return errors.New("invalid mount approval configuration")
	}
	if owner == nil {
		return errors.New("authenticated mount owner required")
	}
	return nil
}

func (s *Service) expireApprovalsLocked(now time.Time) {
	for key, a := range s.approvals {
		if !now.Before(a.expires) {
			delete(s.approvals, key)
		}
	}
}

func (s *Service) CreateMountApproval(ctx context.Context, in *corepb.CreateMountApprovalRequest) (*corepb.CreateMountApprovalResponse, error) {
	c := s.config.MountApprovals
	if c == nil {
		return nil, status.Error(codes.Unimplemented, "mount approvals disabled")
	}
	if !wire.ValidApprovalBinding(in.Binding) || in.GetBinding().GetActor() == string(s.config.Expected) || in.GetTtlSeconds() == 0 || in.GetTtlSeconds() > uint64(c.MaxTTL/time.Second) {
		return nil, status.Error(codes.InvalidArgument, "invalid mount approval")
	}
	if _, ok := s.config.ReadDelegation.Peers[values.FactAuthorityRef(in.Binding.GetActor())]; !ok {
		return nil, status.Error(codes.Unauthenticated, "unknown actor")
	}
	caller, ok := grpcsession.Caller(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "session required")
	}
	md, _ := metadata.FromIncomingContext(ctx)
	parentKey, err := sessionKey(md)
	if err != nil {
		return nil, err
	}
	view, err := s.config.MountControl.AuthorizeMountAcceptance(ctx, caller, values.ElementID(in.Binding.GetElement()))
	if err != nil {
		f, e := rpcfailure.Control(err)
		return &corepb.CreateMountApprovalResponse{Outcome: &corepb.CreateMountApprovalResponse_Failure{Failure: f}}, e
	}
	var token [32]byte
	if _, err := rand.Read(token[:]); err != nil {
		return nil, status.Error(codes.Unavailable, "approval unavailable")
	}
	key := sha256.Sum256(token[:])
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	s.expireApprovalsLocked(now)
	parent, ok := s.sessions[parentKey]
	if s.stopped || !ok || parent.caller != caller || !now.Before(parent.expires) || ctx.Err() != nil {
		return nil, status.Error(codes.Unauthenticated, "session unavailable")
	}
	count := 0
	for _, a := range s.approvals {
		if a.caller.Subject == caller.Subject {
			count++
		}
	}
	if len(s.approvals) >= c.MaxApprovals || count >= c.MaxPerCaller {
		return nil, status.Error(codes.ResourceExhausted, "approval capacity")
	}
	if _, exists := s.approvals[key]; exists {
		return nil, status.Error(codes.Unavailable, "approval unavailable")
	}
	expires := now.Add(time.Duration(in.GetTtlSeconds()) * time.Second)
	if parent.expires.Before(expires) {
		expires = parent.expires
	}
	s.approvals[key] = mountApproval{caller: caller, view: view, binding: proto.Clone(in.Binding).(*corepb.MountApprovalBinding), expires: expires}
	remaining := expires.Sub(now)
	seconds := uint64(remaining / time.Second)
	if remaining%time.Second != 0 {
		seconds++
	}
	return &corepb.CreateMountApprovalResponse{Outcome: &corepb.CreateMountApprovalResponse_Result{Result: &corepb.MountApproval{Target: proto.String(string(s.config.Expected)), Binding: proto.Clone(in.Binding).(*corepb.MountApprovalBinding), Token: token[:], ExpiresInSeconds: proto.Uint64(seconds)}}}, nil
}

func (s *Service) checkedMountApproval(ctx context.Context, in *corepb.CheckMountApprovalRequest) (mountApproval, error) {
	if s.config.MountApprovals == nil {
		return mountApproval{}, status.Error(codes.Unimplemented, "mount approvals disabled")
	}
	if !wire.ValidApprovalBinding(in.Binding) || len(in.Token) != 32 {
		return mountApproval{}, status.Error(codes.InvalidArgument, "invalid approval")
	}
	md, _ := metadata.FromIncomingContext(ctx)
	if len(md.Get("authorization")) != 0 || !s.authenticatedActor(ctx, values.FactAuthorityRef(in.Binding.GetActor())) {
		return mountApproval{}, status.Error(codes.Unauthenticated, "authenticated actor required")
	}
	key := sha256.Sum256(in.Token)
	s.mu.Lock()
	s.expireApprovalsLocked(time.Now())
	a, ok := s.approvals[key]
	stopped := s.stopped
	s.mu.Unlock()
	if stopped || !ok || !proto.Equal(a.binding, in.Binding) {
		return mountApproval{}, status.Error(codes.Unauthenticated, "approval unavailable")
	}
	if err := s.config.MountControl.CheckMountAcceptance(ctx, a.caller, values.ElementID(a.binding.GetElement()), a.view); err != nil {
		return mountApproval{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	current, ok := s.approvals[key]
	if s.stopped || !ok || current.caller != a.caller || current.view != a.view || !time.Now().Before(a.expires) || ctx.Err() != nil {
		return mountApproval{}, status.Error(codes.Unauthenticated, "approval unavailable")
	}
	return a, nil
}

func (s *Service) CheckMountApproval(ctx context.Context, in *corepb.CheckMountApprovalRequest) (*corepb.CheckMountApprovalResponse, error) {
	a, err := s.checkedMountApproval(ctx, in)
	if err != nil {
		var p values.PublicError
		if !errors.As(err, &p) {
			return nil, err
		}
		f, e := rpcfailure.Control(err)
		return &corepb.CheckMountApprovalResponse{Outcome: &corepb.CheckMountApprovalResponse_Failure{Failure: f}}, e
	}
	return &corepb.CheckMountApprovalResponse{Outcome: &corepb.CheckMountApprovalResponse_Result{Result: &corepb.MountApprovalCheck{Target: proto.String(string(s.config.Expected)), Realm: proto.String(string(a.caller.Subject.Realm)), Principal: proto.String(string(a.caller.Subject.Principal))}}}, nil
}

func (s *Service) RevokeMountApproval(ctx context.Context, in *corepb.RevokeMountApprovalRequest) (*corepb.RevokeMountApprovalResponse, error) {
	if s.config.MountApprovals == nil {
		return nil, status.Error(codes.Unimplemented, "mount approvals disabled")
	}
	if len(in.Token) != 32 {
		return nil, status.Error(codes.InvalidArgument, "invalid approval")
	}
	caller, ok := grpcsession.Caller(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "session required")
	}
	// Revocation remains credential-authenticated even if the user's mount
	// permission was removed. It cannot remove another principal's approval.
	if err := s.checkCaller(ctx, caller); err != nil {
		if errors.Is(err, values.PublicUnavailable) {
			return nil, status.Error(codes.Unavailable, "authentication unavailable")
		}
		return nil, status.Error(codes.Unauthenticated, "session unavailable")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.expireApprovalsLocked(time.Now())
	key := sha256.Sum256(in.Token)
	if a, exists := s.approvals[key]; exists && a.caller.Subject == caller.Subject {
		delete(s.approvals, key)
	}
	return &corepb.RevokeMountApprovalResponse{Released: proto.Bool(true)}, nil
}
