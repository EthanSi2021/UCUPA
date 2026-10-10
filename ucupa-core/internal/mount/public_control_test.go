package mount

import (
	"context"
	"errors"
	"testing"

	"ucupa/shared/values"
	corecontract "ucupa/ucupa-core/contract"
	mountfact "ucupa/ucupa-core/internal/mount/fact"
)

func TestAuthenticatedParentRejectsBeforeReservation(t *testing.T) {
	for _, scenario := range []string{"denied", "aba", "approval-unavailable", "cancel-during-approval", "revoked-during-approval", "empty-view", "capacity"} {
		t.Run(scenario, func(t *testing.T) {
			f := newMountPublicFixture(t, false)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			view, want := f.view, error(values.PublicDenied)
			switch scenario {
			case "denied":
				f.a.auth.validate = values.AuthorizationDenied
			case "aba":
				f.a.auth.validate = values.AuthorizationChanged
				want = values.PublicInvalidContext
			case "approval-unavailable":
				f.transport.checkErr = values.PublicUnavailable
				want = values.PublicUnavailable
			case "cancel-during-approval":
				f.transport.check = cancel
				want = values.PublicUnavailable
			case "revoked-during-approval":
				f.transport.check = func() { f.a.auth.validate = values.AuthorizationDenied }
			case "empty-view":
				view = values.AuthorizationView{}
				want = values.PublicInvalid
			case "capacity":
				for i := 0; i < 4; i++ {
					if err := f.a.admission.Acquire(); err != nil {
						t.Fatal(err)
					}
					defer f.a.admission.Release()
				}
				want = values.PublicResourceExhausted
			}
			before := mountProbe(f.a.owner, "parent-operation", f.anchor)
			_, err := f.a.mounts.ApplyMountControl(ctx, f.caller, view, "parent-operation", f.command, f.transport)
			if !errors.Is(err, want) || !reflectEqual(before, mountProbe(f.a.owner, "parent-operation", f.anchor)) {
				t.Fatalf("%s: %v; reservation leaked", scenario, err)
			}
		})
	}
}

func TestAuthenticatedParentOriginalViewAcrossPeerPrepare(t *testing.T) {
	for _, scenario := range []string{"valid", "revoked", "aba", "unavailable"} {
		t.Run(scenario, func(t *testing.T) {
			f := newMountPublicFixture(t, false)
			f.transport.afterPrepare = func() {
				switch scenario {
				case "revoked":
					f.a.auth.validate = values.AuthorizationDenied
				case "aba":
					f.a.auth.validate = values.AuthorizationChanged
				case "unavailable":
					f.a.auth.validate = values.AuthorizationUnavailable
				}
			}
			r, err := f.a.mounts.ApplyMountControl(context.Background(), f.caller, f.view, "parent-operation", f.command, f.transport)
			f.a.owner.RLock()
			operation, _ := f.a.owner.MountOperationLocked("parent-operation")
			f.a.owner.RUnlock()
			switch scenario {
			case "valid":
				if err != nil || r.Decision != corecontract.MountCommitted || !r.Settled {
					t.Fatalf("commit: %#v %v", r, err)
				}
				beforeA, beforeB := mountProbe(f.a.owner, "parent-operation", f.anchor), mountProbe(f.b.owner, "parent-operation", f.anchor)
				f.transport.checkErr = values.PublicUnavailable
				if _, err = f.a.mounts.ApplyMountControl(context.Background(), f.caller, f.view, "parent-operation", f.command, f.transport); err != nil || f.transport.checks != 1 || !reflectEqual(beforeA, mountProbe(f.a.owner, "parent-operation", f.anchor)) || !reflectEqual(beforeB, mountProbe(f.b.owner, "parent-operation", f.anchor)) {
					t.Fatalf("decided replay called approval: %v", err)
				}
			case "revoked", "aba":
				f.b.owner.RLock()
				incoming := f.b.owner.FactsLocked().Mount.Incoming()
				f.b.owner.RUnlock()
				if err == nil || operation.Decision != corecontract.MountAborted || !operation.Settled || incoming.Key.Valid() {
					t.Fatalf("authorization loss: %#v %v", operation, err)
				}
			case "unavailable":
				f.b.owner.RLock()
				incoming := f.b.owner.FactsLocked().Mount.Incoming()
				f.b.owner.RUnlock()
				if !errors.Is(err, values.PublicUnavailable) || operation.Decision != corecontract.MountPending || incoming.Gate != mountfact.GatePreparing {
					t.Fatalf("unavailability: %#v %v", operation, err)
				}
				beforeA, beforeB := mountProbe(f.a.owner, "parent-operation", f.anchor), mountProbe(f.b.owner, "parent-operation", f.anchor)
				if err = f.a.mounts.ConfigureMountTransport(f.transport); err != nil {
					t.Fatal(err)
				}
				f.a.auth.validate = values.AuthorizationValid
				if _, err = f.a.mounts.RecoverMounts(context.Background()); err != nil {
					t.Fatal(err)
				}
				if !reflectEqual(beforeA, mountProbe(f.a.owner, "parent-operation", f.anchor)) || !reflectEqual(beforeB, mountProbe(f.b.owner, "parent-operation", f.anchor)) {
					t.Fatal("recovery used bare subject")
				}
			}
			for _, c := range []*mountTestCore{f.a, f.b} {
				c.owner.RLock()
				e := c.owner.ValidateCurrentLocked()
				c.owner.RUnlock()
				if e != nil {
					t.Fatal(e)
				}
			}
		})
	}
}
