package admission

import (
	"context"
	"crypto/sha256"
	"errors"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	"ucupa/ucupa-core/contract/transition"
)

// This finite stub checks only admission's retained identity and time boundary;
// real target transactions and durable replay are covered by TLS owner tests.
type acceptancePort struct {
	call func(context.Context, values.FactAuthorityRef, values.CallerContext, contract.TransitionApprovalBinding, values.AuthorizationView, transition.SourceTransport) (transition.Acceptance, error)
}

func (*acceptancePort) AuthorityRef() values.FactAuthorityRef { return admissionTestOwner }
func (p *acceptancePort) AcceptTransitionImport(ctx context.Context, peer values.FactAuthorityRef, caller values.CallerContext, b contract.TransitionApprovalBinding, view values.AuthorizationView, source transition.SourceTransport) (transition.Acceptance, error) {
	return p.call(ctx, peer, caller, b, view, source)
}

func TestTransitionAcceptanceRetainsOnlyLiveOriginalApproval(t *testing.T) {
	for _, mode := range []string{"live", "revoked", "expired", "missing-session", "expired-session", "replaced-session", "wrong-binding", "stopped"} {
		t.Run(mode, func(t *testing.T) {
			s, _, user, peer, request, sessionKey := transitionApprovalFixture(t)
			issued, err := s.CreateTransitionApproval(user, request)
			if err != nil {
				t.Fatal(err)
			}
			b, err := s.transitionBinding(request.Binding)
			if err != nil {
				t.Fatal(err)
			}
			raw := issued.GetResult().Token
			key := sha256.Sum256(raw)
			a := s.transitionApprovals[key]
			switch mode {
			case "revoked":
				delete(s.transitionApprovals, key)
			case "expired":
				a.expires = time.Now().Add(-time.Second)
				s.transitionApprovals[key] = a
			case "missing-session":
				delete(s.sessions, sessionKey)
			case "expired-session":
				entry := s.sessions[sessionKey]
				entry.expires = time.Now().Add(-time.Second)
				s.sessions[sessionKey] = entry
			case "replaced-session":
				entry := s.sessions[sessionKey]
				entry.caller.Proof[0] ^= 1
				s.sessions[sessionKey] = entry
			case "wrong-binding":
				b.Operation = "other"
			case "stopped":
				s.BeginStop()
			}
			calls := 0
			s.config.TransitionTarget = &acceptancePort{call: func(ctx context.Context, actor values.FactAuthorityRef, caller values.CallerContext, actual contract.TransitionApprovalBinding, view values.AuthorizationView, source transition.SourceTransport) (transition.Acceptance, error) {
				calls++
				if actor != b.Source || actual != b {
					t.Error("original binding changed")
				}
				if mode == "live" {
					if caller != a.caller || view != a.view {
						t.Error("original caller/view changed")
					}
					deadline, ok := ctx.Deadline()
					if !ok || deadline.After(a.expires) {
						t.Error("approval has no final commit deadline")
					}
				} else if caller != (values.CallerContext{}) || view != (values.AuthorizationView{}) {
					t.Error("dead approval forwarded as live")
				}
				if _, err := source.ReadTransitionFence(ctx, b.Command.SourceAddress, b.Source, b); err != nil {
					t.Error(err)
				}
				if _, err := source.ReadTransitionFence(ctx, "local:other", b.Source, b); err == nil {
					t.Error("prefetched reply escaped its address")
				}
				if _, err := source.ReadTransitionDecision(ctx, b.Command.SourceAddress, b.Source, b); err == nil {
					t.Error("prefetched reply manufactured a decision")
				}
				return transition.Acceptance{}, values.PublicDenied
			}}
			_, err = s.AcceptFrozenTransition(peer, b, raw, transition.Frozen{})
			if mode == "stopped" {
				if calls != 0 || status.Code(err) != codes.Unavailable {
					t.Fatal("stopped host admitted work", err)
				}
			} else if calls != 1 || !errors.Is(err, values.PublicDenied) {
				t.Fatal("owner result not preserved", err)
			}
		})
	}
}

func TestTransitionAcceptanceDeadlineCoversOwnerWait(t *testing.T) {
	s, _, user, peer, request, _ := transitionApprovalFixture(t)
	issued, err := s.CreateTransitionApproval(user, request)
	if err != nil {
		t.Fatal(err)
	}
	b, err := s.transitionBinding(request.Binding)
	if err != nil {
		t.Fatal(err)
	}
	key := sha256.Sum256(issued.GetResult().Token)
	a := s.transitionApprovals[key]
	a.expires = time.Now().Add(30 * time.Millisecond)
	s.transitionApprovals[key] = a
	s.config.TransitionTarget = &acceptancePort{call: func(ctx context.Context, _ values.FactAuthorityRef, caller values.CallerContext, _ contract.TransitionApprovalBinding, _ values.AuthorizationView, _ transition.SourceTransport) (transition.Acceptance, error) {
		if caller != a.caller {
			t.Error("lost live original identity")
		}
		select {
		case <-ctx.Done():
			return transition.Acceptance{}, values.PublicUnavailable
		case <-time.After(time.Second):
			t.Error("owner outlived approval")
			return transition.Acceptance{}, nil
		}
	}}
	if _, err := s.AcceptFrozenTransition(peer, b, issued.GetResult().Token, transition.Frozen{}); !errors.Is(err, values.PublicUnavailable) {
		t.Fatal(err)
	}
}
