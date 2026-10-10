package transfer_test

import (
	"context"
	"errors"
	"sync"
	"testing"

	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
)

func TestPublicCarrierTransferCompletesAndRetainsOriginalResults(t *testing.T) {
	for _, empty := range []bool{false, true} {
		t.Run(map[bool]string{false: "populated", true: "empty"}[empty], func(t *testing.T) {
			f := newCarrierRecovery(t, empty)
			done := f.recover(t, f.connection(t, true, true))
			mount, err := f.parent.c.Mounts().InspectMountControl(context.Background(), f.parent.caller, carrierMount)
			if err != nil || mount.Carrier != f.target.c.AuthorityRef() {
				t.Fatal("parent binding", mount, err)
			}
			var revision contract.Revision
			if !empty {
				read, err := f.target.c.Elements().ReadElement(f.target.caller.Subject, carrierEntity, nil)
				if err != nil || read.Record.Entity == nil || read.Record.Entity.ContainingAnchorID != carrierMount {
					t.Fatal("target content", read, err)
				}
				revision = read.Record.Entity.Revision
				if _, err := f.source.Elements().ReadElement(actor, carrierEntity, nil); err == nil {
					t.Fatal("source remained readable")
				}
			}
			if _, err := f.source.Elements().CreateEntity(actor, "00000000-0000-4000-8000-000000000041", tuple(t, carrierMount), "late-source-write"); err == nil {
				t.Fatal("source still writable")
			}
			f.reopen(t)
			if got := f.recover(t, f.connection(t, false, false)); got != done {
				t.Fatal("restart changed original result", got, done)
			}
			if got, err := f.parent.c.Transfers().BeginCarrierTransition(context.Background(), f.parent.caller, f.binding.Operation, f.binding.Command); err != nil || got != f.begin {
				t.Fatal("original Begin changed", got, err)
			}
			if !empty {
				read, err := f.target.c.Elements().ReadElement(f.target.caller.Subject, carrierEntity, nil)
				if err != nil || !read.Record.Entity.Revision.Equal(revision) {
					t.Fatal("target reinstalled", read, err)
				}
			}
			changed := f.binding
			changed.Command.TargetAddress = "local:other"
			if _, err := f.parent.c.Transfers().RecoverCarrierTransition(context.Background(), f.parent.caller, changed, f.connection(t, false, false)); !errors.Is(err, contract.ErrOperationConflict) {
				t.Fatal("changed completed input accepted", err)
			}
		})
	}
}

func TestPublicCarrierLostRepliesRecoverAcrossThreeStoreRestarts(t *testing.T) {
	for _, stage := range []string{"fence", "accept", "install", "settle"} {
		t.Run(stage, func(t *testing.T) {
			f := newCarrierRecovery(t, false)
			c := carrierBoundary{CarrierTransport: f.connection(t, true, true)}
			fail := func() error { return values.PublicUnavailable }
			switch stage {
			case "fence":
				c.afterFence = fail
			case "accept":
				c.afterAccept = fail
			case "install":
				c.afterInstall = fail
			case "settle":
				c.afterSettle = fail
			}
			if _, err := f.parent.c.Transfers().RecoverCarrierTransition(context.Background(), f.parent.caller, f.binding, c); !errors.Is(err, values.PublicUnavailable) {
				t.Fatal(err)
			}
			if _, err := f.source.Elements().ReadElement(actor, carrierEntity, nil); err == nil {
				t.Fatal("failure reactivated source")
			}
			if stage == "fence" || stage == "accept" {
				if r, err := f.target.c.Elements().ReadElement(f.target.caller.Subject, carrierEntity, nil); err == nil && r.Record != nil {
					t.Fatal("target visible before decision")
				}
			}
			// Once accepted, target permission may be revoked without erasing its promise.
			if stage != "fence" {
				f.target.grant(t, contract.PermissionTransitionImport, carrierMount, true)
			}
			f.parent.grant(t, actor, contract.PermissionTransitionExport, values.CoreElementScope(f.source.AuthorityRef(), carrierMount), true)
			if stage == "install" || stage == "settle" {
				f.parent.grant(t, actor, contract.PermissionTransitionCommit, values.CoreElementScope(f.binding.Parent, carrierMount), true)
			}
			f.reopen(t)
			done := f.recover(t, f.connection(t, false, stage == "fence"))
			if got := f.recover(t, f.connection(t, false, false)); got != done {
				t.Fatal("retry changed settlement")
			}
		})
	}
}

func TestPublicCarrierConcurrentExactRecovery(t *testing.T) {
	f := newCarrierRecovery(t, false)
	link := f.connection(t, true, true)
	const count = 6
	results := make(chan contract.CarrierStatus, count)
	errs := make(chan error, count)
	var wg sync.WaitGroup
	for i := 0; i < count; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r, err := f.parent.c.Transfers().RecoverCarrierTransition(context.Background(), f.parent.caller, f.binding, link)
			results <- r
			errs <- err
		}()
	}
	wg.Wait()
	close(results)
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	want := f.recover(t, link)
	for r := range results {
		if r != want {
			t.Fatal("concurrent calls changed result", r, want)
		}
	}
}
