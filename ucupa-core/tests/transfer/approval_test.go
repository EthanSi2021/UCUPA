package transfer_test

import (
	"context"
	"errors"
	"testing"

	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	core "ucupa/ucupa-core/instance"
)

func importBinding(f *fixture) contract.TransitionApprovalBinding {
	c := f.command
	source := c.Target
	c.Target = f.c.AuthorityRef()
	c.Expected.Owner = source
	return contract.TransitionApprovalBinding{Source: source, Operation: contract.OperationID(string([]byte{255, 0, 128})), Command: c}
}

func TestImportApprovalRequiresDurableTarget(t *testing.T) {
	f := newFixture(t)
	memory, err := core.New(boundaryContacts{f.a}, transferContact, f.config)
	if err != nil {
		t.Fatal(err)
	}
	defer memory.Close()
	b := importBinding(f)
	b.Command.Target = memory.AuthorityRef()
	f.grant(t, actor, contract.PermissionTransitionImport, values.CoreElementScope(memory.AuthorityRef(), entity), false)
	f.grant(t, actor, contract.PermissionPlaceElement, values.CoreElementScope(memory.AuthorityRef(), root), false)
	if _, err := memory.Transfers().AuthorizeTransitionImport(context.Background(), f.caller, b); !errors.Is(err, values.PublicUnavailable) {
		t.Fatalf("non-durable target approved: %v", err)
	}
}

func TestTargetImportUsesOriginalViewAndOwnPermissions(t *testing.T) {
	f := newFixture(t)
	b := importBinding(f)
	ctx := context.Background()
	f.permit(t, contract.PermissionTransitionExport)
	f.permit(t, contract.PermissionMountAccept)
	before := f.revision(t, entity)
	if _, err := f.c.Transfers().AuthorizeTransitionImport(ctx, f.caller, b); !errors.Is(err, values.PublicDenied) {
		t.Fatalf("CRUD/export/mount became import: %v", err)
	}
	f.permit(t, contract.PermissionTransitionImport)
	placeScope := values.CoreElementScope(f.c.AuthorityRef(), root)
	f.grant(t, actor, contract.PermissionPlaceElement, placeScope, true)
	if _, err := f.c.Transfers().AuthorizeTransitionImport(ctx, f.caller, b); !errors.Is(err, values.PublicDenied) {
		t.Fatalf("import bypassed final parent permission: %v", err)
	}
	f.grant(t, actor, contract.PermissionPlaceElement, placeScope, false)
	view, err := f.c.Transfers().AuthorizeTransitionImport(ctx, f.caller, b)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.c.Transfers().CheckTransitionImport(ctx, f.caller, b, view); err != nil {
		t.Fatal(err)
	}
	if after := f.revision(t, entity); after != before {
		t.Fatal("approval changed target facts")
	}
	f.grant(t, actor, contract.PermissionTransitionImport, values.CoreElementScope(f.c.AuthorityRef(), entity), true)
	f.permit(t, contract.PermissionTransitionImport)
	if err := f.c.Transfers().CheckTransitionImport(ctx, f.caller, b, view); !errors.Is(err, values.PublicInvalidContext) {
		t.Fatalf("regrant revived old view: %v", err)
	}
	other, err := f.a.TrustLocalCaller(values.PrincipalRef{Realm: actor.Realm, Principal: "other"})
	if err != nil {
		t.Fatal(err)
	}
	if err := f.c.Transfers().CheckTransitionImport(ctx, other, b, view); err == nil {
		t.Fatal("substituted original caller")
	}
}

func TestImportApprovalRevalidatesBeforeDisclosure(t *testing.T) {
	for _, action := range []string{"ABA", "close", "cancel"} {
		t.Run(action, func(t *testing.T) {
			f := newFixture(t)
			f.permit(t, contract.PermissionTransitionImport)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			f.a.afterAuthorize(func() {
				switch action {
				case "ABA":
					f.grant(t, actor, contract.PermissionTransitionImport, values.CoreElementScope(f.c.AuthorityRef(), entity), true)
					f.permit(t, contract.PermissionTransitionImport)
				case "close":
					_ = f.c.Close()
				case "cancel":
					cancel()
				}
			})
			if view, err := f.c.Transfers().AuthorizeTransitionImport(ctx, f.caller, importBinding(f)); err == nil || view != (values.AuthorizationView{}) {
				t.Fatalf("disclosed approval after %s: %v %v", action, view, err)
			}
		})
	}
}

func TestImportApprovalShapeRootAndUnavailable(t *testing.T) {
	f := newFixture(t)
	f.permit(t, contract.PermissionTransitionImport)
	b := importBinding(f)
	for name, change := range map[string]func(*contract.TransitionApprovalBinding){
		"target":     func(b *contract.TransitionApprovalBinding) { b.Command.Target = b.Source },
		"source":     func(b *contract.TransitionApprovalBinding) { b.Source = "bad" },
		"operation":  func(b *contract.TransitionApprovalBinding) { b.Operation = "" },
		"comparison": func(b *contract.TransitionApprovalBinding) { b.Command.Expected.Owner = b.Command.Target },
		"kind":       func(b *contract.TransitionApprovalBinding) { b.Command.Kind = 99 },
		"root tuple": func(b *contract.TransitionApprovalBinding) { b.Command.Kind = contract.TransitionRoot },
	} {
		t.Run(name, func(t *testing.T) {
			changed := b
			change(&changed)
			if _, err := f.c.Transfers().AuthorizeTransitionImport(context.Background(), f.caller, changed); !errors.Is(err, values.PublicInvalid) {
				t.Fatalf("accepted malformed input: %v", err)
			}
		})
	}
	b.Command.Kind, b.Command.Final = contract.TransitionRoot, contract.SpatialTuple{}
	view, err := f.c.Transfers().AuthorizeTransitionImport(context.Background(), f.caller, b)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := f.c.Transfers().CheckTransitionImport(ctx, f.caller, b, view); !errors.Is(err, values.PublicUnavailable) {
		t.Fatalf("cancelled check: %v", err)
	}
	_ = f.c.Close()
	if err := f.c.Transfers().CheckTransitionImport(context.Background(), f.caller, b, view); !errors.Is(err, values.PublicUnavailable) {
		t.Fatalf("closed target: %v", err)
	}
}
