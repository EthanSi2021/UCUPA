package transfer_test

import (
	"context"
	"errors"
	"testing"

	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	local "ucupa/ucupa-core/transport/local/transition"
)

func TestRecoveryRequiresSeparateControlPermissionBeforeFence(t *testing.T) {
	s, target := newFixture(t), newTargetFixture(t, false)
	b := beginPair(t, s, target, false)
	s.grant(t, actor, contract.PermissionTransitionCommit, values.CoreElementScope(b.Source, entity), true)
	link := pairConnection(t, s, target, b, true)
	if _, err := s.c.Transfers().RecoverTransition(context.Background(), s.caller, b, link); !errors.Is(err, values.PublicDenied) {
		t.Fatalf("export silently became control change: %v", err)
	}
	if s.revision(t, entity) != b.Command.Expected {
		t.Fatal("unauthorized request fenced source")
	}
}

func TestRecoveryRevalidatesSourceViewAfterExternalWaits(t *testing.T) {
	for _, stage := range []string{"before-fence", "before-commit"} {
		t.Run(stage, func(t *testing.T) {
			s, target := newFixture(t), newTargetFixture(t, false)
			b := beginPair(t, s, target, false)
			link := pairConnection(t, s, target, b, true)
			aba := func() error {
				scope := values.CoreElementScope(b.Source, entity)
				s.grant(t, actor, contract.PermissionTransitionCommit, scope, true)
				s.grant(t, actor, contract.PermissionTransitionCommit, scope, false)
				return nil
			}
			interrupted := recoveryBoundary{TargetTransport: link}
			want := contract.TransitionIntent
			if stage == "before-fence" {
				interrupted.afterCheck = aba
			} else {
				interrupted.afterAccept = aba
				want = contract.TransitionFenced
			}
			if _, err := s.c.Transfers().RecoverTransition(context.Background(), s.caller, b, interrupted); !errors.Is(err, values.PublicInvalidContext) {
				t.Fatalf("ABA reused source view: %v", err)
			}
			status, err := s.c.Transfers().InspectTransition(context.Background(), s.caller, b.Operation, entity)
			if err != nil || status.Progress != want {
				t.Fatalf("unauthorized progress: %+v %v", status, err)
			}
			if stage == "before-fence" && s.revision(t, entity) != b.Command.Expected {
				t.Fatal("source changed before fence authorization")
			}
			// This is a new authenticated retry, not silently renewing the old view.
			recoverPair(t, s, b, link)
		})
	}
}

func TestRecoveryRevalidatesTargetViewAfterSourceFence(t *testing.T) {
	s, target := newFixture(t), newTargetFixture(t, false)
	b := beginPair(t, s, target, false)
	link := pairConnection(t, s, target, b, true)
	interrupted := recoveryBoundary{TargetTransport: link, afterCheck: func() error {
		target.grant(t, contract.PermissionTransitionImport, entity, true)
		target.grant(t, contract.PermissionTransitionImport, entity, false)
		return nil
	}}
	if _, err := s.c.Transfers().RecoverTransition(context.Background(), s.caller, b, interrupted); !errors.Is(err, values.PublicInvalidContext) {
		t.Fatalf("target accepted stale view: %v", err)
	}
	target.grant(t, contract.PermissionReadElement, entity, false)
	if read, err := target.c.Elements().ReadElement(target.caller.Subject, entity, nil); err == nil && read.Record != nil {
		t.Fatal("target exposed unaccepted content")
	}
	status, err := s.c.Transfers().InspectTransition(context.Background(), s.caller, b.Operation, entity)
	if err != nil || status.Progress != contract.TransitionFenced {
		t.Fatalf("source fence lost: %+v %v", status, err)
	}
	if _, err := s.c.Transfers().CancelTransition(context.Background(), s.caller, b.Operation, entity, status.Revision); err == nil {
		t.Fatal("rejection undid source fence")
	}
	recoverPair(t, s, b, pairConnection(t, s, target, b, true))
}

func TestRecoveryCancellationAndCloseDuringApprovalDoNotFence(t *testing.T) {
	for _, closeOwner := range []bool{false, true} {
		t.Run(map[bool]string{false: "cancel-context", true: "close-source"}[closeOwner], func(t *testing.T) {
			s, target := newFixture(t), newTargetFixture(t, false)
			b := beginPair(t, s, target, false)
			link := pairConnection(t, s, target, b, true)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			interrupted := recoveryBoundary{TargetTransport: link, afterCheck: func() error {
				if closeOwner {
					return s.c.Close()
				}
				cancel()
				return nil
			}}
			if _, err := s.c.Transfers().RecoverTransition(ctx, s.caller, b, interrupted); !errors.Is(err, values.PublicUnavailable) {
				t.Fatalf("continued after interruption: %v", err)
			}
			s.reopen(t)
			if s.revision(t, entity) != b.Command.Expected {
				t.Fatal("interruption fenced source")
			}
		})
	}
}

func TestTransitionPeersUseExactBindingAndDesignatedAddress(t *testing.T) {
	s, target := newFixture(t), newTargetFixture(t, false)
	b := beginPair(t, s, target, false)
	link := pairConnection(t, s, target, b, true)
	ctx := context.Background()
	if err := link.CheckImport(ctx, "local:wrong", b.Command.Target, b); !errors.Is(err, values.PublicUnavailable) {
		t.Fatalf("address fallback: %v", err)
	}
	if err := link.CheckImport(ctx, b.Command.TargetAddress, b.Source, b); !errors.Is(err, values.PublicUnavailable) {
		t.Fatalf("wrong owner accepted: %v", err)
	}
	for _, change := range []func(*contract.TransitionApprovalBinding){
		func(v *contract.TransitionApprovalBinding) { v.Command.SourceAddress = "local:other" },
		func(v *contract.TransitionApprovalBinding) { v.Command.Expected.Value = "forged" },
		func(v *contract.TransitionApprovalBinding) { v.Operation = "other" },
		func(v *contract.TransitionApprovalBinding) { v.Command.Boundary = root },
	} {
		bad := b
		change(&bad)
		if _, err := s.c.Transfers().ReadTransitionFence(ctx, target.c.AuthorityRef(), bad); err == nil {
			t.Fatal("disclosed mismatched source fence")
		}
		if err := link.CheckImport(ctx, b.Command.TargetAddress, b.Command.Target, bad); !errors.Is(err, contract.ErrOperationConflict) {
			t.Fatalf("changed binding accepted: %v", err)
		}
	}
	if _, err := s.c.Transfers().ReadTransitionFence(ctx, b.Source, b); !errors.Is(err, values.PublicDenied) {
		t.Fatalf("caller chose arbitrary peer: %v", err)
	}
	if _, err := target.c.Transfers().AcceptTransitionImport(ctx, b.Command.Target, target.caller, b, values.AuthorizationView{}, link); !errors.Is(err, values.PublicInvalid) {
		t.Fatalf("target called with wrong source: %v", err)
	}
	if _, err := target.c.Transfers().InstallTransition(ctx, b.Source, b, link); !errors.Is(err, contract.ErrConflict) {
		t.Fatalf("installed without durable acceptance: %v", err)
	}
	if s.revision(t, entity) != b.Command.Expected {
		t.Fatal("invalid calls changed source")
	}
	// A different real Core cannot be substituted for the intended target.
	other := newTargetFixture(t, false)
	if _, err := local.New(s.c.Transfers(), other.c.Transfers(), b, target.caller, values.AuthorizationView{}); !errors.Is(err, values.PublicInvalid) {
		t.Fatalf("misconnected target: %v", err)
	}
	if _, err := local.New(s.c.Transfers(), target.c.Transfers(), b, target.caller, values.AuthorizationView{}); err != nil {
		t.Fatal(err)
	}
}
