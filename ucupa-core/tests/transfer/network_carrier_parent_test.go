package transfer_test

import (
	"context"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"ucupa/shared/values"
	pb "ucupa/ucupa-core/bindings/go/protobuf"
	"ucupa/ucupa-core/contract"
	"ucupa/ucupa-core/contract/transition"
	coresdk "ucupa/ucupa-core/sdk/go"
)

func TestCarrierReadsActualParentPlanAndDecisionOverTLS(t *testing.T) {
	f, ctx := newCarrierParentNetwork(t), context.Background()
	b := f.binding
	var reserved transition.CarrierPlan
	for _, peer := range []values.FactAuthorityRef{b.Command.Source, b.Command.Target} {
		p, err := f.parent.ReadCarrierPlan(ctx, peer, b)
		if err != nil || p == (transition.CarrierPlan{}) {
			t.Fatal("reservation", err)
		}
		if reserved != (transition.CarrierPlan{}) && reserved != p {
			t.Fatal("participants saw different reservation")
		}
		reserved = p
		if _, err := f.parent.ReadCarrierDecision(ctx, peer, b); err == nil {
			t.Fatal("reservation was reported as a switch decision")
		}
	}
	// Parent install reads execute through the real, authenticated parent host.
	f.recover(t, f.link(t))
	plan, err := f.parent.ReadCarrierPlan(ctx, b.Command.Source, b)
	if err != nil || plan == reserved {
		t.Fatal("permit missing", err)
	}
	decision, err := f.parent.ReadCarrierDecision(ctx, b.Command.Target, b)
	if err != nil || decision == (transition.CarrierDecision{}) {
		t.Fatal("decision", err)
	}
	r, err := f.target.c.Elements().ReadElement(f.target.caller.Subject, carrierEntity, nil)
	if err != nil || r.Record == nil || r.Record.Entity == nil {
		t.Fatal("target did not install real source content", err)
	}
	f.reopenParent(t)
	for _, peer := range []values.FactAuthorityRef{b.Command.Source, b.Command.Target} {
		if got, err := f.parent.ReadCarrierPlan(ctx, peer, b); err != nil || got != plan {
			t.Fatal("plan changed on reopen", err)
		}
		if got, err := f.parent.ReadCarrierDecision(ctx, peer, b); err != nil || got != decision {
			t.Fatal("decision changed on reopen", err)
		}
	}
	changed := b
	changed.Operation = "other-operation"
	if _, err := f.parent.ReadCarrierPlan(ctx, b.Command.Source, changed); err == nil {
		t.Fatal("different operation disclosed")
	}
	changed = b
	changed.Command.TargetAddress = "local:other"
	if _, err := f.parent.ReadCarrierDecision(ctx, b.Command.Source, changed); err == nil {
		t.Fatal("different address disclosed")
	}
	reads := f.parent.reads[b.Command.Source]
	if _, err := reads.ReadCarrierPlan(ctx, "https://127.0.0.1:1", b.Parent, b); err == nil {
		t.Fatal("implicit address fallback")
	}
	if _, err := reads.ReadCarrierPlan(ctx, b.Command.ParentAddress, b.Command.Target, b); err == nil {
		t.Fatal("wrong parent connected")
	}
	f.host.Stop()
	if _, err := f.parent.ReadCarrierPlan(ctx, b.Command.Source, b); err == nil {
		t.Fatal("disconnected parent returned cached evidence")
	}
}

func TestCarrierParentRejectsIncorrectTLSIdentityAndUserSession(t *testing.T) {
	f, ctx := newCarrierParentNetwork(t), context.Background()
	b := f.binding
	for _, mode := range []string{"participant-label-is-not-identity", "self-asserted-outsider", "user-session"} {
		t.Run(mode, func(t *testing.T) {
			cfg := f.clientConfigs[b.Command.Source]
			q := proto.Clone(f.w).(*pb.CarrierApprovalBinding)
			switch mode {
			case "participant-label-is-not-identity":
				cfg.Actor = b.Command.Target
			case "self-asserted-outsider":
				cfg.Actor = "00000000-0000-4000-8000-000000000099"
				q.Command.Source = proto.String(string(cfg.Actor))
			case "user-session":
				cfg.Actor = ""
				cfg.ActorCertificate = nil
			}
			client, err := coresdk.New(cfg)
			if err != nil {
				t.Fatal(err)
			}
			defer client.Close()
			if mode == "user-session" {
				if err := client.Establish(ctx, actor.Realm, [16]byte{62}, [32]byte{63}); err != nil {
					t.Fatal(err)
				}
			}
			want := codes.Unauthenticated
			if mode == "participant-label-is-not-identity" {
				// The actual TLS key remains the source's, which may read this
				// same plan. A client-side label cannot impersonate the target.
				want = codes.OK
			}
			if _, err := client.TransitionParent().ReadCarrierPlan(ctx, &pb.ReadCarrierPlanRequest{Binding: q}); status.Code(err) != want {
				t.Fatal("wrong peer admitted", err)
			}
			if mode == "participant-label-is-not-identity" {
				return
			}
			if _, err := client.TransitionParent().ReadCarrierDecision(ctx, &pb.ReadCarrierDecisionRequest{Binding: q}); status.Code(err) != codes.Unauthenticated {
				t.Fatal("wrong peer admitted to decision", err)
			}
		})
	}
}

func TestCarrierParentReportsOriginalCancellationWithoutDecision(t *testing.T) {
	f, ctx := newCarrierParentNetwork(t), context.Background()
	p, b := f.carrierFixture.parent, f.binding
	r, err := p.c.Transfers().CancelCarrierTransition(ctx, p.caller, b.Operation, b.Command.Mount, f.begin.Revision)
	if err != nil || r.Progress != contract.CarrierCancelled {
		t.Fatal(r, err)
	}
	plan, err := f.parent.ReadCarrierPlan(ctx, b.Command.Target, b)
	if err != nil {
		t.Fatal(err)
	}
	f.reopenParent(t)
	if got, err := f.parent.ReadCarrierPlan(ctx, b.Command.Source, b); err != nil || got != plan {
		t.Fatal("cancelled plan changed", err)
	}
	if _, err := f.parent.ReadCarrierDecision(ctx, b.Command.Target, b); err == nil {
		t.Fatal("cancelled plan fabricated decision")
	}
}
