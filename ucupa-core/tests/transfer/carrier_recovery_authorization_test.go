package transfer_test

import (
	"context"
	"errors"
	"testing"

	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	"ucupa/ucupa-core/contract/transition"
	local "ucupa/ucupa-core/transport/local/transition"
)

type carrierPlanProbe struct {
	transition.CarrierTransport
	called bool
}

func (p *carrierPlanProbe) ReadCarrierPlan(ctx context.Context, a values.NextHop, owner values.FactAuthorityRef, b contract.CarrierApprovalBinding) (transition.CarrierPlan, error) {
	p.called = true
	return p.CarrierTransport.ReadCarrierPlan(ctx, a, owner, b)
}

func TestPublicCarrierFreshFenceValidatesBeforePeerWait(t *testing.T) {
	f := newCarrierRecovery(t, false)
	ctx := context.Background()
	view, err := f.source.Transfers().AuthorizeCarrierExport(ctx, f.parent.caller, f.binding)
	if err != nil {
		t.Fatal(err)
	}
	p := &carrierPlanProbe{CarrierTransport: f.connection(t, true, true)}
	if _, err := f.source.Transfers().FenceCarrierSource(ctx, f.binding.Parent, f.parent.caller, f.binding, values.AuthorizationView{}, p); !errors.Is(err, values.PublicDenied) || p.called {
		t.Fatal("missing view reached peer", err, p.called)
	}
	scope := values.CoreElementScope(f.source.AuthorityRef(), carrierMount)
	f.parent.grant(t, actor, contract.PermissionTransitionExport, scope, true)
	f.parent.grant(t, actor, contract.PermissionTransitionExport, scope, false)
	if _, err := f.source.Transfers().FenceCarrierSource(ctx, f.binding.Parent, f.parent.caller, f.binding, view, p); !errors.Is(err, values.PublicInvalidContext) || p.called {
		t.Fatal("stale view reached peer", err, p.called)
	}
}

func TestPublicCarrierRevalidatesOriginalApprovalsBeforeFence(t *testing.T) {
	for _, peer := range []string{"source", "target"} {
		t.Run(peer, func(t *testing.T) {
			f := newCarrierRecovery(t, false)
			link := f.connection(t, true, true)
			c := carrierBoundary{CarrierTransport: link, afterCheck: func() error {
				if peer == "source" {
					scope := values.CoreElementScope(f.source.AuthorityRef(), carrierMount)
					f.parent.grant(t, actor, contract.PermissionTransitionExport, scope, true)
					f.parent.grant(t, actor, contract.PermissionTransitionExport, scope, false)
				} else {
					f.target.grant(t, contract.PermissionTransitionImport, carrierMount, true)
					f.target.grant(t, contract.PermissionTransitionImport, carrierMount, false)
				}
				return nil
			}}
			if _, err := f.parent.c.Transfers().RecoverCarrierTransition(context.Background(), f.parent.caller, f.binding, c); !errors.Is(err, values.PublicInvalidContext) {
				t.Fatal("stale approval accepted", err)
			}
			if r, err := f.source.Elements().ReadElement(actor, carrierEntity, nil); err != nil || r.Record == nil {
				t.Fatal("unauthorized fence", r, err)
			}
			f.recover(t, f.connection(t, true, true))
		})
	}
}

func TestPublicCarrierTargetGrantIsNotParentsIdentity(t *testing.T) {
	f := newCarrierRecovery(t, false)
	ctx := context.Background()
	if _, err := f.target.c.Transfers().AuthorizeCarrierImport(ctx, f.parent.caller, f.binding); !errors.Is(err, values.PublicDenied) {
		t.Fatal("parent identity used as target importer", err)
	}
	link := f.connection(t, true, false)
	if _, err := f.parent.c.Transfers().RecoverCarrierTransition(ctx, f.parent.caller, f.binding, link); !errors.Is(err, values.PublicDenied) {
		t.Fatal("missing target approval accepted", err)
	}
	r, err := f.parent.c.Transfers().InspectCarrierTransition(ctx, f.parent.caller, f.binding.Operation, carrierMount)
	if err != nil || r != f.begin {
		t.Fatal("failed preflight changed reservation", r, err)
	}
	link = f.connection(t, false, true)
	if _, err := f.parent.c.Transfers().RecoverCarrierTransition(ctx, f.parent.caller, f.binding, link); !errors.Is(err, values.PublicDenied) {
		t.Fatal("missing source approval accepted", err)
	}
	f.recover(t, f.connection(t, true, true))
}

func TestPublicCarrierParentDecisionNeedsCurrentPermission(t *testing.T) {
	f := newCarrierRecovery(t, false)
	scope := values.CoreElementScope(f.binding.Parent, carrierMount)
	c := carrierBoundary{CarrierTransport: f.connection(t, true, true), afterAccept: func() error {
		f.parent.grant(t, actor, contract.PermissionTransitionCommit, scope, true)
		f.parent.grant(t, actor, contract.PermissionTransitionCommit, scope, false)
		return nil
	}}
	if _, err := f.parent.c.Transfers().RecoverCarrierTransition(context.Background(), f.parent.caller, f.binding, c); !errors.Is(err, values.PublicInvalidContext) {
		t.Fatal("stale parent approval switched binding", err)
	}
	r, err := f.parent.c.Transfers().InspectCarrierTransition(context.Background(), f.parent.caller, f.binding.Operation, carrierMount)
	if err != nil || r.Progress != contract.CarrierPermitted {
		t.Fatal(r, err)
	}
	if r, err := f.target.c.Elements().ReadElement(f.target.caller.Subject, carrierEntity, nil); err == nil && r.Record != nil {
		t.Fatal("acceptance published before parent decision")
	}
	f.target.grant(t, contract.PermissionTransitionImport, carrierMount, true)
	f.reopen(t)
	f.recover(t, f.connection(t, false, false))
}

func TestPublicCarrierCancellationAndDisclosure(t *testing.T) {
	for _, cancelCommand := range []bool{true, false} {
		t.Run(map[bool]string{true: "reservation-cancel", false: "context-cancel"}[cancelCommand], func(t *testing.T) {
			f := newCarrierRecovery(t, false)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			var cancelled contract.CarrierStatus
			c := carrierBoundary{CarrierTransport: f.connection(t, true, true), afterCheck: func() error {
				if cancelCommand {
					var err error
					cancelled, err = f.parent.c.Transfers().CancelCarrierTransition(ctx, f.parent.caller, f.binding.Operation, carrierMount, f.begin.Revision)
					if err != nil {
						t.Fatal(err)
					}
				} else {
					cancel()
				}
				return nil
			}}
			if _, err := f.parent.c.Transfers().RecoverCarrierTransition(ctx, f.parent.caller, f.binding, c); err == nil {
				t.Fatal("cancelled recovery proceeded")
			}
			if r, err := f.source.Elements().ReadElement(actor, carrierEntity, nil); err != nil || r.Record == nil {
				t.Fatal("cancelled recovery fenced source", r, err)
			}
			if cancelCommand {
				got, err := f.parent.c.Transfers().RecoverCarrierTransition(context.Background(), f.parent.caller, f.binding, f.connection(t, false, false))
				if err != nil || got != cancelled {
					t.Fatal("cancel replay", got, err)
				}
			}
		})
	}
	f := newCarrierRecovery(t, false)
	c := carrierBoundary{CarrierTransport: f.connection(t, true, true), afterInstall: func() error {
		f.parent.grant(t, actor, contract.PermissionTransitionInspect, values.CoreElementScope(f.binding.Parent, carrierMount), true)
		return nil
	}}
	if r, err := f.parent.c.Transfers().RecoverCarrierTransition(context.Background(), f.parent.caller, f.binding, c); !errors.Is(err, values.PublicDenied) || r != (contract.CarrierStatus{}) {
		t.Fatal("disclosed after inspect revoked", r, err)
	}
	f.parent.grant(t, actor, contract.PermissionTransitionInspect, values.CoreElementScope(f.binding.Parent, carrierMount), false)
	f.recover(t, f.connection(t, false, false))
}

func TestPublicCarrierDesignatedContactsAndPeerRoles(t *testing.T) {
	f := newCarrierRecovery(t, false)
	link := f.connection(t, true, true)
	ctx := context.Background()
	b := f.binding
	for _, address := range []values.NextHop{b.Command.ParentAddress, b.Command.SourceAddress, "local:unknown"} {
		if err := link.CheckCarrierImport(ctx, address, b.Command.Target, b); !errors.Is(err, values.PublicUnavailable) {
			t.Fatal("wrong designated contact accepted", address, err)
		}
	}
	if err := link.CheckCarrierImport(ctx, b.Command.TargetAddress, b.Command.Source, b); !errors.Is(err, values.PublicUnavailable) {
		t.Fatal("wrong identity accepted", err)
	}
	changed := b
	changed.Operation = "another"
	if err := link.CheckCarrierExport(ctx, b.Command.SourceAddress, b.Command.Source, changed); !errors.Is(err, contract.ErrOperationConflict) {
		t.Fatal("binding changed", err)
	}
	if _, err := link.ReadCarrierPlan(ctx, b.Command.ParentAddress, b.Parent, b); !errors.Is(err, values.PublicDenied) {
		t.Fatal("parent client acquired source/target peer identity", err)
	}
	if _, err := f.parent.c.Transfers().ReadCarrierPlan(ctx, b.Parent, b); !errors.Is(err, values.PublicDenied) {
		t.Fatal("wrong peer read proof", err)
	}
	if _, err := local.NewCarrier(f.parent.c.Transfers(), f.target.c.Transfers(), f.source.Transfers(), b, values.CallerContext{}, values.AuthorizationView{}, values.CallerContext{}, values.AuthorizationView{}); !errors.Is(err, values.PublicInvalid) {
		t.Fatal("swapped owners accepted", err)
	}
	f.recover(t, link)
}
