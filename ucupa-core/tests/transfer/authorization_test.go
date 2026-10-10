package transfer_test

import (
	"context"
	"errors"
	"testing"

	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
)

func TestTransitionRequiresOriginalEstablishedCallerAndIndependentPermissions(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	if _, err := f.c.Transfers().BeginTransition(ctx, f.caller, "move", f.command); !errors.Is(err, values.PublicDenied) {
		t.Fatalf("ordinary CRUD/read implied export: %v", err)
	}
	f.permit(t, contract.PermissionTransitionExport)
	if _, err := f.c.Transfers().BeginTransition(ctx, values.CallerContext{Subject: actor}, "move", f.command); !errors.Is(err, values.PublicDenied) {
		t.Fatalf("subject string authenticated itself: %v", err)
	}
	r, err := f.c.Transfers().BeginTransition(ctx, f.caller, "move", f.command)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.c.Transfers().InspectTransition(ctx, f.caller, "move", entity); !errors.Is(err, values.PublicDenied) {
		t.Fatalf("export implied inspect: %v", err)
	}
	f.permit(t, contract.PermissionTransitionInspect)
	other := values.PrincipalRef{Realm: actor.Realm, Principal: "other"}
	for _, permission := range []values.PermissionID{contract.PermissionTransitionExport, contract.PermissionTransitionInspect} {
		f.grant(t, other, permission, values.CoreElementScope(f.c.AuthorityRef(), entity), false)
	}
	caller, err := f.a.TrustLocalCaller(other)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := f.c.Transfers().InspectTransition(ctx, caller, "move", entity); !errors.Is(err, values.PublicDenied) || got != (contract.TransitionStatus{}) {
		t.Fatalf("another actor disclosed original operation: %+v %v", got, err)
	}
	if _, err := f.c.Transfers().CancelTransition(ctx, caller, "move", entity, r.Revision); !errors.Is(err, values.PublicDenied) {
		t.Fatalf("another actor cancelled: %v", err)
	}
	f.grant(t, actor, contract.PermissionTransitionExport, values.CoreElementScope(f.c.AuthorityRef(), entity), true)
	if _, err := f.c.Transfers().CancelTransition(ctx, f.caller, "move", entity, r.Revision); !errors.Is(err, values.PublicDenied) {
		t.Fatalf("revoked export continued cancellation: %v", err)
	}
	if got, err := f.c.Transfers().InspectTransition(ctx, f.caller, "move", entity); err != nil || got != r {
		t.Fatalf("denied cancellation altered intent: %+v %v", got, err)
	}
}

func TestTransitionRevalidatesViewBeforeDurableIntent(t *testing.T) {
	f := newFixture(t)
	f.permit(t, contract.PermissionTransitionExport)
	f.permit(t, contract.PermissionTransitionInspect)
	f.a.afterAuthorize(func() {
		scope := values.CoreElementScope(f.c.AuthorityRef(), entity)
		f.grant(t, actor, contract.PermissionTransitionExport, scope, true)
		f.grant(t, actor, contract.PermissionTransitionExport, scope, false)
	})
	if _, err := f.c.Transfers().BeginTransition(context.Background(), f.caller, "aba", f.command); !errors.Is(err, values.PublicInvalidContext) {
		t.Fatalf("ABA refreshed original view: %v", err)
	}
	f.reopen(t)
	if _, err := f.c.Transfers().InspectTransition(context.Background(), f.caller, "aba", entity); !errors.Is(err, contract.ErrNotFound) {
		t.Fatalf("unauthorized intent persisted: %v", err)
	}
	if _, err := f.c.Transfers().BeginTransition(context.Background(), f.caller, "aba", f.command); err != nil {
		t.Fatalf("new authenticated invocation cannot retry: %v", err)
	}
}

func TestTransitionContextCancellationAndCloseDoNotCreateIntent(t *testing.T) {
	for _, closing := range []bool{false, true} {
		t.Run(map[bool]string{false: "cancel", true: "close"}[closing], func(t *testing.T) {
			f := newFixture(t)
			f.permit(t, contract.PermissionTransitionExport)
			f.permit(t, contract.PermissionTransitionInspect)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			f.a.afterAuthorize(func() {
				if closing {
					if err := f.c.Close(); err != nil {
						t.Fatal(err)
					}
				} else {
					cancel()
				}
			})
			if _, err := f.c.Transfers().BeginTransition(ctx, f.caller, "interrupted", f.command); !errors.Is(err, values.PublicUnavailable) {
				t.Fatalf("closed/interrupted call: %v", err)
			}
			f.reopen(t)
			if _, err := f.c.Transfers().InspectTransition(context.Background(), f.caller, "interrupted", entity); !errors.Is(err, contract.ErrNotFound) {
				t.Fatalf("interrupted intent persisted: %v", err)
			}
		})
	}
}
