package admission

import (
	"context"
	"crypto/sha256"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	"ucupa/ucupa-core/contract/transition"
)

// Admission consumes only the actual bounded target acceptance operation.
// Installation and source I/O remain outside its credential lifecycle lock.
type TransitionAcceptanceOwner interface {
	AuthorityRef() values.FactAuthorityRef
	AcceptTransitionImport(context.Context, values.FactAuthorityRef, values.CallerContext, contract.TransitionApprovalBinding, values.AuthorizationView, transition.SourceTransport) (transition.Acceptance, error)
}

// AcceptFrozenTransition is called only after the target RPC has fetched the
// actual source evidence. This call-local immutable reply removes all network
// waits from the approval critical section; it owns no transfer state.
func (s *Service) AcceptFrozenTransition(ctx context.Context, b contract.TransitionApprovalBinding, token []byte, frozen transition.Frozen) (transition.Acceptance, error) {
	owner := s.config.TransitionTarget
	if owner == nil {
		return transition.Acceptance{}, status.Error(codes.Unimplemented, "transition target disabled")
	}
	if len(token) != 32 || b.Command.Target != s.config.Expected || owner.AuthorityRef() != s.config.Expected {
		return transition.Acceptance{}, status.Error(codes.InvalidArgument, "invalid target approval")
	}
	if err := s.CheckTransitionPeer(ctx, b.Source); err != nil {
		return transition.Acceptance{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	if s.stopped || ctx.Err() != nil {
		return transition.Acceptance{}, status.Error(codes.Unavailable, "transition target unavailable")
	}
	s.expireTransitionApprovalsLocked(now)
	a, exists := s.transitionApprovals[sha256.Sum256(token)]
	session, live := s.sessions[a.session.key]
	valid := exists && a.binding == b && live && session == a.session.entry && now.Before(session.expires) && now.Before(a.expires)
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
	return owner.AcceptTransitionImport(ctx, b.Source, caller, b, view, frozenSource{b, frozen})
}

type frozenSource struct {
	binding contract.TransitionApprovalBinding
	frozen  transition.Frozen
}

func (s frozenSource) ReadTransitionFence(ctx context.Context, address values.NextHop, expected values.FactAuthorityRef, b contract.TransitionApprovalBinding) (transition.Frozen, error) {
	if ctx == nil || ctx.Err() != nil {
		return transition.Frozen{}, values.PublicUnavailable
	}
	if b != s.binding || address != b.Command.SourceAddress || expected != b.Source {
		return transition.Frozen{}, contract.ErrConflict
	}
	return s.frozen, nil
}

func (frozenSource) ReadTransitionDecision(context.Context, values.NextHop, values.FactAuthorityRef, contract.TransitionApprovalBinding) (transition.Decision, error) {
	return transition.Decision{}, values.PublicInvalid
}
