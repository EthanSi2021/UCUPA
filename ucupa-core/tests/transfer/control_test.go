package transfer_test

import (
	"context"
	"errors"
	"sync"
	"testing"

	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
)

func TestSourceIntentPublicRetryCancellationAndRestart(t *testing.T) {
	f := newFixture(t)
	f.permit(t, contract.PermissionTransitionExport)
	f.permit(t, contract.PermissionTransitionInspect)
	ctx := context.Background()
	id := contract.OperationID(string([]byte{0xff, 0, 't'}))
	r, err := f.c.Transfers().BeginTransition(ctx, f.caller, id, f.command)
	if err != nil || r.Progress != contract.TransitionIntent || !r.Revision.Valid() || r.Revision.Kind != "transition-revision" || r.Source != f.c.AuthorityRef() {
		t.Fatalf("begin: %+v %v", r, err)
	}
	if f.revision(t, entity) != f.command.Expected {
		t.Fatal("intent changed or hid the active source")
	}
	var wg sync.WaitGroup
	for n := 0; n < 8; n++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			got, err := f.c.Transfers().BeginTransition(ctx, f.caller, id, f.command)
			if err != nil || got != r {
				t.Errorf("exact concurrent retry: %+v %v", got, err)
			}
		}()
	}
	wg.Wait()
	f.reopen(t)
	if got, err := f.c.Transfers().InspectTransition(ctx, f.caller, id, entity); err != nil || got != r {
		t.Fatalf("reopen: %+v %v", got, err)
	}
	changed := f.command
	changed.TargetAddress = "local:other"
	if _, err := f.c.Transfers().BeginTransition(ctx, f.caller, id, changed); !errors.Is(err, contract.ErrOperationConflict) {
		t.Fatalf("changed intent reused operation: %v", err)
	}
	cancelled, err := f.c.Transfers().CancelTransition(ctx, f.caller, id, entity, r.Revision)
	if err != nil || cancelled.Progress != contract.TransitionCancelled || cancelled.Revision == r.Revision {
		t.Fatalf("cancel: %+v %v", cancelled, err)
	}
	f.reopen(t)
	if got, err := f.c.Transfers().CancelTransition(ctx, f.caller, id, entity, r.Revision); err != nil || got != cancelled {
		t.Fatalf("original cancellation CAS: %+v %v", got, err)
	}
	if _, err := f.c.Transfers().CancelTransition(ctx, f.caller, id, entity, cancelled.Revision); !errors.Is(err, contract.ErrRevisionConflict) {
		t.Fatalf("new cancellation CAS replayed old command: %v", err)
	}
	if got, err := f.c.Transfers().BeginTransition(ctx, f.caller, id, f.command); err != nil || got != cancelled {
		t.Fatalf("begin retry hid terminal outcome: %+v %v", got, err)
	}
	if f.revision(t, entity) != f.command.Expected {
		t.Fatal("cancelled intent changed source content")
	}
}

func TestSourceIntentOriginalCASRemainsRecoverableAfterElementChange(t *testing.T) {
	f := newFixture(t)
	f.permit(t, contract.PermissionTransitionExport)
	ctx := context.Background()
	original, err := f.c.Transfers().BeginTransition(ctx, f.caller, "move", f.command)
	if err != nil {
		t.Fatal(err)
	}
	f.grant(t, actor, contract.PermissionReadElement, values.CoreElementScope(f.c.AuthorityRef(), entity), false)
	f.grant(t, actor, contract.PermissionUpdateElement, values.CoreElementScope(f.c.AuthorityRef(), entity), false)
	read, err := f.c.Elements().ReadElement(actor, entity, nil)
	if err != nil {
		t.Fatal(err)
	}
	final := tuple(t, root)
	final.Range, _ = contract.NewBoundedSphere(2)
	if _, err := f.c.Elements().UpdateEntity(actor, entity, final, read.Record.Entity.Revision, "change"); err != nil {
		t.Fatal(err)
	}
	f.reopen(t)
	if got, err := f.c.Transfers().BeginTransition(ctx, f.caller, "move", f.command); err != nil || got != original {
		t.Fatalf("original CAS lost: %+v %v", got, err)
	}
	changed := f.command
	changed.Expected = f.revision(t, entity)
	if changed.Expected == f.command.Expected {
		t.Fatal("test did not change element")
	}
	if _, err := f.c.Transfers().BeginTransition(ctx, f.caller, "move", changed); !errors.Is(err, contract.ErrOperationConflict) {
		t.Fatalf("retry silently refreshed CAS: %v", err)
	}
}

func TestRootIntentUsesExplicitProfile(t *testing.T) {
	f := newFixture(t)
	f.command.Kind, f.command.Boundary, f.command.Final = contract.TransitionRoot, root, contract.SpatialTuple{}
	f.command.Expected = f.revision(t, root)
	f.permit(t, contract.PermissionTransitionExport)
	r, err := f.c.Transfers().BeginTransition(context.Background(), f.caller, "root-transfer", f.command)
	if err != nil || r.Kind != contract.TransitionRoot || r.Progress != contract.TransitionIntent {
		t.Fatalf("root intent: %+v %v", r, err)
	}
	bad := f.command
	bad.Kind, bad.Final = contract.TransitionSlice, tuple(t, entity)
	if _, err := f.c.Transfers().BeginTransition(context.Background(), f.caller, "bad-root", bad); !errors.Is(err, contract.ErrConflict) {
		t.Fatalf("root disguised as slice: %v", err)
	}
}
