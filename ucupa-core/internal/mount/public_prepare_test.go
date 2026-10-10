package mount

import (
	"context"
	"errors"
	"testing"

	"ucupa/shared/values"
	corecontract "ucupa/ucupa-core/contract"
)

func TestAuthenticatedMountPrepareRejectsBeforeMutation(t *testing.T) {
	for _, scenario := range []string{"other-principal", "wrong-peer", "denied", "revoked", "aba", "cancelled", "cancel-during-check"} {
		t.Run(scenario, func(t *testing.T) {
			f, request := authenticatedCarrierFixture(t, false)
			caller, source := f.caller, request.Key.ParentAuthority
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			want := error(values.PublicDenied)
			switch scenario {
			case "other-principal":
				caller.Subject.Principal = "another-user"
			case "wrong-peer":
				source = f.b.owner.AuthorityRef()
				want = values.PublicInvalid
			case "denied":
				f.b.auth.decision = values.AuthorizationDenied
			case "revoked":
				f.b.auth.validate = values.AuthorizationDenied
			case "aba":
				f.b.auth.validate = values.AuthorizationChanged
				want = values.PublicInvalidContext
			case "cancelled":
				cancel()
				want = values.PublicUnavailable
			case "cancel-during-check":
				f.b.auth.beforeValidate = cancel
				want = values.PublicUnavailable
			}
			before := mountProbe(f.b.owner, request.OperationID, request.Key.MountAnchorUUID)
			got, err := f.b.mounts.PrepareMountControl(ctx, caller, source, request)
			if !errors.Is(err, want) || got != (corecontract.MountPrepared{}) || !reflectEqual(before, mountProbe(f.b.owner, request.OperationID, request.Key.MountAnchorUUID)) {
				t.Fatalf("rejected changed facts: %#v %v", got, err)
			}
		})
	}
}

func TestAuthenticatedMountPrepareDurableReplay(t *testing.T) {
	f, request := authenticatedCarrierFixture(t, true)
	got, err := f.b.mounts.PrepareMountControl(context.Background(), f.caller, request.Key.ParentAuthority, request)
	if err != nil || !got.Prepared {
		t.Fatalf("prepare: %#v %v", got, err)
	}
	f.b.reopen(t)
	before := mountProbe(f.b.owner, request.OperationID, request.Key.MountAnchorUUID)
	replay, err := f.b.mounts.PrepareMountControl(context.Background(), f.caller, request.Key.ParentAuthority, request)
	if err != nil || replay != got || !reflectEqual(before, mountProbe(f.b.owner, request.OperationID, request.Key.MountAnchorUUID)) {
		t.Fatalf("durable replay: %#v %v", replay, err)
	}
	f.b.owner.RLock()
	err = f.b.owner.ValidateCurrentLocked()
	f.b.owner.RUnlock()
	if err != nil {
		t.Fatal(err)
	}
}

func TestAuthenticatedMountPrepareHonorsOwnerAdmission(t *testing.T) {
	f, request := authenticatedCarrierFixture(t, false)
	for i := 0; i < 4; i++ {
		if err := f.b.admission.Acquire(); err != nil {
			t.Fatal(err)
		}
	}
	before := mountProbe(f.b.owner, request.OperationID, request.Key.MountAnchorUUID)
	_, err := f.b.mounts.PrepareMountControl(context.Background(), f.caller, request.Key.ParentAuthority, request)
	if !errors.Is(err, values.PublicResourceExhausted) || !reflectEqual(before, mountProbe(f.b.owner, request.OperationID, request.Key.MountAnchorUUID)) {
		t.Fatalf("capacity: %v", err)
	}
	for i := 0; i < 4; i++ {
		f.b.admission.Release()
	}
	for i := 0; i < 4; i++ {
		if err := f.b.admission.Acquire(); err != nil {
			t.Fatalf("slot %d not reusable: %v", i, err)
		}
		defer f.b.admission.Release()
	}
}

func TestAuthenticatedMountPrepareExactReplayAndAuthorization(t *testing.T) {
	f, request := authenticatedCarrierFixture(t, false)
	got, err := f.b.mounts.PrepareMountControl(context.Background(), f.caller, request.Key.ParentAuthority, request)
	if err != nil || !got.Prepared {
		t.Fatalf("prepare: %#v %v", got, err)
	}
	wantRequirement := values.Requirement{Permission: corecontract.PermissionMountAccept, Scope: values.CoreElementScope(f.b.owner.AuthorityRef(), request.Key.MountAnchorUUID)}
	if f.b.auth.seen != f.caller || len(f.b.auth.required) != 1 || f.b.auth.required[0] != wantRequirement {
		t.Fatal("wrong caller or scope")
	}
	before := mountProbe(f.b.owner, request.OperationID, request.Key.MountAnchorUUID)
	replay, err := f.b.mounts.PrepareMountControl(context.Background(), f.caller, request.Key.ParentAuthority, request)
	if err != nil || replay != got || !reflectEqual(before, mountProbe(f.b.owner, request.OperationID, request.Key.MountAnchorUUID)) {
		t.Fatalf("replay: %#v %v", replay, err)
	}
	changed := request
	changed.ParentActor.Principal = "changed-parent"
	if _, err := f.b.mounts.PrepareMountControl(context.Background(), f.caller, request.Key.ParentAuthority, changed); !errors.Is(err, corecontract.ErrOperationConflict) {
		t.Fatalf("changed identity: %v", err)
	}
	f.b.auth.validate = values.AuthorizationDenied
	if _, err := f.b.mounts.PrepareMountControl(context.Background(), f.caller, request.Key.ParentAuthority, request); !errors.Is(err, values.PublicDenied) {
		t.Fatalf("revoked replay: %v", err)
	}
	if !reflectEqual(before, mountProbe(f.b.owner, request.OperationID, request.Key.MountAnchorUUID)) {
		t.Fatal("rejected replay changed facts")
	}
}
