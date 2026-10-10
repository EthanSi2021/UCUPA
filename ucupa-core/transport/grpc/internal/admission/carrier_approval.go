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
	"ucupa/ucupa-core/contract/transition"
	"ucupa/ucupa-core/transport/grpc/internal/rpcfailure"
	grpcsession "ucupa/ucupa-core/transport/grpc/internal/session"
	"ucupa/ucupa-core/transport/grpc/internal/wire"
)

// Carrier approvals are issued separately on the source and target carriers.
// Only the designated parent may check a token; a check re-validates the
// retained original caller/view and never acts as a write permit by itself.
type CarrierApprovalConfig struct {
	MaxTTL       time.Duration
	MaxApprovals int
	MaxPerCaller int
}

type CarrierApprovalOwner interface {
	AuthorityRef() values.FactAuthorityRef
	AuthorizeCarrierExport(context.Context, values.CallerContext, contract.CarrierApprovalBinding) (values.AuthorizationView, error)
	CheckCarrierExport(context.Context, values.CallerContext, contract.CarrierApprovalBinding, values.AuthorizationView) error
	AuthorizeCarrierImport(context.Context, values.CallerContext, contract.CarrierApprovalBinding) (values.AuthorizationView, error)
	CheckCarrierImport(context.Context, values.CallerContext, contract.CarrierApprovalBinding, values.AuthorizationView) error
}

// CarrierAcceptanceOwner is the actual target carrier owner. Admission hands it
// the retained original caller/view and pre-fetched evidence only; it owns no
// approval table and never refreshes a runtime approval.
type CarrierAcceptanceOwner interface {
	AuthorityRef() values.FactAuthorityRef
	AcceptCarrierTarget(context.Context, values.FactAuthorityRef, values.CallerContext, contract.CarrierApprovalBinding, values.AuthorizationView, transition.CarrierTransport) (transition.CarrierAcceptance, error)
}

type carrierApproval struct {
	caller    values.CallerContext
	view      values.AuthorizationView
	binding   contract.CarrierApprovalBinding
	session   EntrySession
	expires   time.Time
	importing bool
}

func ValidateCarrierApprovals(c *CarrierApprovalConfig, peers *ReadDelegationConfig, owner CarrierApprovalOwner) error {
	if c == nil {
		return nil
	}
	if peers == nil || owner == nil || c.MaxTTL <= 0 || c.MaxTTL%time.Second != 0 || c.MaxApprovals <= 0 || c.MaxPerCaller <= 0 || c.MaxPerCaller > c.MaxApprovals {
		return errors.New("invalid carrier approval configuration")
	}
	return nil
}

func (s *Service) expireCarrierApprovalsLocked(now time.Time) {
	for key, a := range s.carrierApprovals {
		if !now.Before(a.expires) {
			delete(s.carrierApprovals, key)
		}
	}
}

func (s *Service) carrierBinding(b *pb.CarrierApprovalBinding) (contract.CarrierApprovalBinding, error) {
	if b == nil || proto.Size(b) > 4096 {
		return contract.CarrierApprovalBinding{}, status.Error(codes.InvalidArgument, "invalid carrier binding")
	}
	return wire.CarrierBinding(b)
}

// carrierRole binds this host to exactly one side of the binding. The parent
// neither issues nor retains carrier approvals.
func (s *Service) carrierRole(b contract.CarrierApprovalBinding) (importing bool, ok bool) {
	switch s.config.Expected {
	case b.Command.Source:
		return false, true
	case b.Command.Target:
		return true, true
	default:
		return false, false
	}
}

func (s *Service) CreateCarrierApproval(ctx context.Context, in *pb.CreateCarrierApprovalRequest) (*pb.CreateCarrierApprovalResponse, error) {
	c := s.config.CarrierApprovals
	if c == nil {
		return nil, status.Error(codes.Unimplemented, "carrier approvals disabled")
	}
	b, err := s.carrierBinding(in.GetBinding())
	if err != nil {
		return nil, err
	}
	importing, ok := s.carrierRole(b)
	if !ok {
		return nil, status.Error(codes.InvalidArgument, "wrong carrier approval host")
	}
	if in.GetTtlSeconds() == 0 || in.GetTtlSeconds() > uint64(c.MaxTTL/time.Second) {
		return nil, status.Error(codes.InvalidArgument, "invalid carrier approval lifetime")
	}
	if _, known := s.config.ReadDelegation.Peers[b.Parent]; !known {
		return nil, status.Error(codes.Unauthenticated, "unknown parent")
	}
	caller, ok := grpcsession.Caller(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "session required")
	}
	session, err := s.BindEntrySession(ctx, caller)
	if err != nil {
		return nil, status.Error(codes.Unauthenticated, "session unavailable")
	}
	var view values.AuthorizationView
	if importing {
		view, err = s.config.CarrierApproval.AuthorizeCarrierImport(ctx, caller, b)
	} else {
		view, err = s.config.CarrierApproval.AuthorizeCarrierExport(ctx, caller, b)
	}
	if err != nil {
		f, e := rpcfailure.Transition(err, false)
		return &pb.CreateCarrierApprovalResponse{Outcome: &pb.CreateCarrierApprovalResponse_Failure{Failure: f}}, e
	}
	var token [32]byte
	if _, err := rand.Read(token[:]); err != nil {
		return nil, status.Error(codes.Unavailable, "approval unavailable")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	s.expireCarrierApprovalsLocked(now)
	current, live := s.sessions[session.key]
	if s.stopped || !live || current != session.entry || !now.Before(current.expires) || ctx.Err() != nil {
		return nil, status.Error(codes.Unauthenticated, "session unavailable")
	}
	count := 0
	for _, a := range s.carrierApprovals {
		if a.caller.Subject == caller.Subject {
			count++
		}
	}
	if len(s.carrierApprovals) >= c.MaxApprovals || count >= c.MaxPerCaller {
		return nil, status.Error(codes.ResourceExhausted, "approval capacity")
	}
	key := sha256.Sum256(token[:])
	if _, exists := s.carrierApprovals[key]; exists {
		return nil, status.Error(codes.Unavailable, "approval unavailable")
	}
	expires := now.Add(time.Duration(in.GetTtlSeconds()) * time.Second)
	if session.Expires().Before(expires) {
		expires = session.Expires()
	}
	s.carrierApprovals[key] = carrierApproval{caller: caller, view: view, binding: b, session: session, expires: expires, importing: importing}
	seconds := uint64((expires.Sub(now)-1)/time.Second) + 1
	return &pb.CreateCarrierApprovalResponse{Outcome: &pb.CreateCarrierApprovalResponse_Result{Result: &pb.CarrierApproval{
		Issuer: proto.String(string(s.config.Expected)), Binding: proto.Clone(in.Binding).(*pb.CarrierApprovalBinding), Token: token[:], ExpiresInSeconds: proto.Uint64(seconds),
	}}}, nil
}

// checkedCarrierApproval re-validates the retained original caller/view at the
// actual owner before disclosing anything. Only the designated parent checks.
func (s *Service) checkedCarrierApproval(ctx context.Context, b contract.CarrierApprovalBinding, token []byte) (carrierApproval, error) {
	if s.config.CarrierApprovals == nil {
		return carrierApproval{}, status.Error(codes.Unimplemented, "carrier approvals disabled")
	}
	importing, ok := s.carrierRole(b)
	if !ok || len(token) != 32 {
		return carrierApproval{}, status.Error(codes.InvalidArgument, "invalid carrier approval")
	}
	md, _ := metadata.FromIncomingContext(ctx)
	if len(md.Get("authorization")) != 0 || !s.authenticatedActor(ctx, b.Parent) {
		return carrierApproval{}, status.Error(codes.Unauthenticated, "authenticated parent required")
	}
	key := sha256.Sum256(token)
	s.mu.Lock()
	s.expireCarrierApprovalsLocked(time.Now())
	a, found := s.carrierApprovals[key]
	stopped := s.stopped
	s.mu.Unlock()
	if stopped || !found || a.binding != b || a.importing != importing {
		return carrierApproval{}, status.Error(codes.Unauthenticated, "approval unavailable")
	}
	if importing {
		err := s.config.CarrierApproval.CheckCarrierImport(ctx, a.caller, a.binding, a.view)
		if err != nil {
			return carrierApproval{}, err
		}
	} else if err := s.config.CarrierApproval.CheckCarrierExport(ctx, a.caller, a.binding, a.view); err != nil {
		return carrierApproval{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	current, found := s.carrierApprovals[key]
	session, live := s.sessions[a.session.key]
	if s.stopped || !found || current != a || !live || session != a.session.entry || !time.Now().Before(a.expires) || ctx.Err() != nil {
		return carrierApproval{}, status.Error(codes.Unauthenticated, "approval unavailable")
	}
	return a, nil
}

func (s *Service) CheckCarrierApproval(ctx context.Context, in *pb.CheckCarrierApprovalRequest) (*pb.CheckCarrierApprovalResponse, error) {
	b, err := s.carrierBinding(in.GetBinding())
	if err != nil {
		return nil, err
	}
	a, err := s.checkedCarrierApproval(ctx, b, in.GetToken())
	if err != nil {
		if _, ok := status.FromError(err); ok {
			return nil, err
		}
		f, e := rpcfailure.Transition(err, false)
		return &pb.CheckCarrierApprovalResponse{Outcome: &pb.CheckCarrierApprovalResponse_Failure{Failure: f}}, e
	}
	return &pb.CheckCarrierApprovalResponse{Outcome: &pb.CheckCarrierApprovalResponse_Result{Result: &pb.CarrierApprovalCheck{
		Issuer: proto.String(string(s.config.Expected)), Binding: proto.Clone(in.Binding).(*pb.CarrierApprovalBinding),
		Realm: proto.String(string(a.caller.Subject.Realm)), Principal: proto.String(string(a.caller.Subject.Principal)),
	}}}, nil
}

func (s *Service) RevokeCarrierApproval(ctx context.Context, in *pb.RevokeCarrierApprovalRequest) (*pb.RevokeCarrierApprovalResponse, error) {
	if s.config.CarrierApprovals == nil {
		return nil, status.Error(codes.Unimplemented, "carrier approvals disabled")
	}
	if len(in.GetToken()) != 32 {
		return nil, status.Error(codes.InvalidArgument, "invalid carrier approval")
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
	s.expireCarrierApprovalsLocked(time.Now())
	key := sha256.Sum256(in.Token)
	if a, exists := s.carrierApprovals[key]; exists && a.caller.Subject == caller.Subject {
		delete(s.carrierApprovals, key)
	}
	return &pb.RevokeCarrierApprovalResponse{Released: proto.Bool(true)}, nil
}

// ResolveCarrierExport returns the retained original caller/view for one fence
// dispatch. An absent or expired approval yields zero values: the source owner
// may still replay an existing durable fence, but cannot fence for the first
// time without a live original approval.
func (s *Service) ResolveCarrierExport(ctx context.Context, b contract.CarrierApprovalBinding, token []byte) (values.CallerContext, values.AuthorizationView, time.Time, error) {
	if s.config.CarrierApprovals == nil {
		return values.CallerContext{}, values.AuthorizationView{}, time.Time{}, status.Error(codes.Unimplemented, "carrier approvals disabled")
	}
	if b.Command.Source != s.config.Expected || len(token) != 32 {
		return values.CallerContext{}, values.AuthorizationView{}, time.Time{}, status.Error(codes.InvalidArgument, "invalid carrier export approval")
	}
	key := sha256.Sum256(token)
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	s.expireCarrierApprovalsLocked(now)
	a, ok := s.carrierApprovals[key]
	session, live := s.sessions[a.session.key]
	if s.stopped || ctx.Err() != nil || !ok || a.binding != b || a.importing || !live || session != a.session.entry || !now.Before(session.expires) || !now.Before(a.expires) {
		return values.CallerContext{}, values.AuthorizationView{}, time.Time{}, nil
	}
	return a.caller, a.view, a.expires, nil
}

// CheckCarrierImportPreflight serves the source carrier's stateless preflight
// before its first withdrawal. It proves one live original import approval
// exists for this exact binding and re-validates it; it is not the target's
// durable content acceptance and grants the source nothing else.
func (s *Service) CheckCarrierImportPreflight(ctx context.Context, b contract.CarrierApprovalBinding) error {
	if s.config.CarrierApprovals == nil || s.config.CarrierApproval == nil {
		return status.Error(codes.Unimplemented, "carrier approvals disabled")
	}
	if b.Command.Target != s.config.Expected {
		return status.Error(codes.InvalidArgument, "invalid carrier binding")
	}
	if err := s.CheckTransitionPeer(ctx, b.Command.Source); err != nil {
		return err
	}
	s.mu.Lock()
	s.expireCarrierApprovalsLocked(time.Now())
	var live []carrierApproval
	for _, a := range s.carrierApprovals {
		if a.importing && a.binding == b {
			live = append(live, a)
		}
	}
	stopped := s.stopped
	s.mu.Unlock()
	if stopped || ctx.Err() != nil || len(live) == 0 {
		return status.Error(codes.Unauthenticated, "carrier import approval unavailable")
	}
	for _, a := range live {
		if err := s.config.CarrierApproval.CheckCarrierImport(ctx, a.caller, a.binding, a.view); err != nil {
			return err
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.stopped || ctx.Err() != nil {
		return status.Error(codes.Unavailable, "service stopped")
	}
	now := time.Now()
	for _, a := range live {
		session, ok := s.sessions[a.session.key]
		if ok && session == a.session.entry && now.Before(session.expires) && now.Before(a.expires) {
			return nil
		}
	}
	return status.Error(codes.Unauthenticated, "carrier import approval unavailable")
}

// CheckedCarrierImport re-validates one live original import approval for the
// authenticated designated parent and returns the retained original caller for
// disclosure. It is the target service's strict check entry.
func (s *Service) CheckedCarrierImport(ctx context.Context, b contract.CarrierApprovalBinding, token []byte) (values.CallerContext, error) {
	a, err := s.checkedCarrierApproval(ctx, b, token)
	if err != nil {
		return values.CallerContext{}, err
	}
	return a.caller, nil
}

// AcceptFrozenCarrier is called only after the target RPC has fetched the
// actual parent plan and source evidence. This call-local immutable reply
// removes all network waits from the approval critical section; it owns no
// transfer state.
func (s *Service) AcceptFrozenCarrier(ctx context.Context, b contract.CarrierApprovalBinding, token []byte, plan transition.CarrierPlan, frozen transition.CarrierFrozen) (transition.CarrierAcceptance, error) {
	owner := s.config.CarrierTarget
	if owner == nil || s.config.CarrierApprovals == nil {
		return transition.CarrierAcceptance{}, status.Error(codes.Unimplemented, "carrier target disabled")
	}
	if len(token) != 32 || b.Command.Target != s.config.Expected || owner.AuthorityRef() != s.config.Expected {
		return transition.CarrierAcceptance{}, status.Error(codes.InvalidArgument, "invalid carrier target approval")
	}
	if err := s.CheckTransitionPeer(ctx, b.Parent); err != nil {
		return transition.CarrierAcceptance{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	if s.stopped || ctx.Err() != nil {
		return transition.CarrierAcceptance{}, status.Error(codes.Unavailable, "carrier target unavailable")
	}
	s.expireCarrierApprovalsLocked(now)
	a, exists := s.carrierApprovals[sha256.Sum256(token)]
	session, live := s.sessions[a.session.key]
	valid := exists && a.binding == b && a.importing && live && session == a.session.entry && now.Before(session.expires) && now.Before(a.expires)
	caller, view := values.CallerContext{}, values.AuthorizationView{}
	if valid {
		caller, view = a.caller, a.view
		var cancel context.CancelFunc
		ctx, cancel = context.WithDeadline(ctx, a.expires)
		defer cancel()
	}
	// Without a live original approval the actual owner may only replay an
	// existing, exactly matching durable acceptance. A zero caller/view cannot
	// authorize the first acceptance, even if the service peer is trusted.
	return owner.AcceptCarrierTarget(ctx, b.Parent, caller, b, view, frozenCarrierSource{b, plan, frozen})
}

type frozenCarrierSource struct {
	binding contract.CarrierApprovalBinding
	plan    transition.CarrierPlan
	frozen  transition.CarrierFrozen
}

func (s frozenCarrierSource) ReadCarrierPlan(ctx context.Context, address values.NextHop, expected values.FactAuthorityRef, b contract.CarrierApprovalBinding) (transition.CarrierPlan, error) {
	if ctx == nil || ctx.Err() != nil {
		return transition.CarrierPlan{}, values.PublicUnavailable
	}
	if b != s.binding || address != b.Command.ParentAddress || expected != b.Parent {
		return transition.CarrierPlan{}, contract.ErrConflict
	}
	return s.plan, nil
}

func (s frozenCarrierSource) ReadCarrierSource(ctx context.Context, address values.NextHop, expected values.FactAuthorityRef, b contract.CarrierApprovalBinding) (transition.CarrierFrozen, error) {
	if ctx == nil || ctx.Err() != nil {
		return transition.CarrierFrozen{}, values.PublicUnavailable
	}
	if b != s.binding || address != b.Command.SourceAddress || expected != b.Command.Source {
		return transition.CarrierFrozen{}, contract.ErrConflict
	}
	return s.frozen, nil
}

func (frozenCarrierSource) ReadCarrierDecision(context.Context, values.NextHop, values.FactAuthorityRef, contract.CarrierApprovalBinding) (transition.CarrierDecision, error) {
	return transition.CarrierDecision{}, values.PublicInvalid
}
func (frozenCarrierSource) CheckCarrierExport(context.Context, values.NextHop, values.FactAuthorityRef, contract.CarrierApprovalBinding) error {
	return values.PublicInvalid
}
func (frozenCarrierSource) CheckCarrierImport(context.Context, values.NextHop, values.FactAuthorityRef, contract.CarrierApprovalBinding) error {
	return values.PublicInvalid
}
func (frozenCarrierSource) FenceCarrierSource(context.Context, values.NextHop, values.FactAuthorityRef, contract.CarrierApprovalBinding) (transition.CarrierFrozen, error) {
	return transition.CarrierFrozen{}, values.PublicInvalid
}
func (frozenCarrierSource) AcceptCarrierTarget(context.Context, values.NextHop, values.FactAuthorityRef, contract.CarrierApprovalBinding) (transition.CarrierAcceptance, error) {
	return transition.CarrierAcceptance{}, values.PublicInvalid
}
func (frozenCarrierSource) InstallCarrierTarget(context.Context, values.NextHop, values.FactAuthorityRef, contract.CarrierApprovalBinding) (transition.Completion, error) {
	return transition.Completion{}, values.PublicInvalid
}
func (frozenCarrierSource) ReadCarrierCompletion(context.Context, values.NextHop, values.FactAuthorityRef, contract.CarrierApprovalBinding) (transition.Completion, error) {
	return transition.Completion{}, values.PublicInvalid
}
func (frozenCarrierSource) SettleCarrierSource(context.Context, values.NextHop, values.FactAuthorityRef, contract.CarrierApprovalBinding) error {
	return values.PublicInvalid
}
