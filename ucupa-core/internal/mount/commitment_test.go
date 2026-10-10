package mount

import (
	"context"
	"errors"
	"testing"

	"ucupa/shared/values"
	corecontract "ucupa/ucupa-core/contract"
	"ucupa/ucupa-core/internal/commit"
	coretoken "ucupa/ucupa-core/internal/token"
)

func TestMountCommandCommitmentBindsEveryIntentField(t *testing.T) {
	f := newMountPublicFixture(t, false)
	base, err := f.a.mounts.CommandCommitment(f.a.owner.AuthorityRef(), f.b.owner.AuthorityRef(), "one", f.command)
	if err != nil || base == ([32]byte{}) {
		t.Fatal(err)
	}
	for _, field := range []string{"operation", "element", "revision-scope", "revision-serial", "carrier-address", "parent-address"} {
		command, id := f.command, corecontract.OperationID("one")
		switch field {
		case "operation":
			id = "two"
		case "element":
			command.MountAnchorUUID = mountTestID(299)
		case "revision-scope":
			r := command.ExpectedElementRevision
			scope := coretoken.RevisionScope(r)
			scope[0] ^= 1
			command.ExpectedElementRevision = coretoken.RevisionFromParts(scope, coretoken.RevisionElement(r), coretoken.RevisionSerial(r))
		case "revision-serial":
			r := command.ExpectedElementRevision
			command.ExpectedElementRevision = coretoken.RevisionFromParts(coretoken.RevisionScope(r), coretoken.RevisionElement(r), ^uint64(0))
		case "carrier-address":
			command.CarrierNextHop = "local:other"
		case "parent-address":
			command.ParentNextHop = "local:other"
		}
		got, err := f.a.mounts.CommandCommitment(f.a.owner.AuthorityRef(), f.b.owner.AuthorityRef(), id, command)
		if err != nil || got == base {
			t.Fatalf("unbound %s: %v", field, err)
		}
	}
	if _, err := f.a.mounts.CommandCommitment(f.a.owner.AuthorityRef(), f.a.owner.AuthorityRef(), "one", f.command); !errors.Is(err, corecontract.ErrInvalid) {
		t.Fatal("self authority accepted")
	}
}

func authenticatedCarrierFixture(t *testing.T, durable bool) (*mountPublicFixture, corecontract.MountPrepareRequest) {
	t.Helper()
	f := newMountPublicFixture(t, false)
	if durable {
		f.b = newMountTestCore(t, true)
		f.command.CarrierAuthority = f.b.owner.AuthorityRef()
		f.callers[f.b.owner.AuthorityRef()] = mountTestActor
	}
	approval, ok := f.a.owner.CurrentContact()
	if !ok {
		t.Fatal("fixture owner has no current contact")
	}
	if err := f.a.mounts.reserveMountIntent(f.callers, "authenticated-prepare", commit.MountBinding{Actor: mountTestActor, Command: f.command}, approval); err != nil {
		t.Fatal(err)
	}
	f.a.owner.RLock()
	op, ok := f.a.owner.MountOperationLocked("authenticated-prepare")
	f.a.owner.RUnlock()
	if !ok {
		t.Fatal("reserved request missing")
	}
	return f, op.Request
}

func TestApprovedMountPrepareKeepsOriginalViewAndCommitment(t *testing.T) {
	for _, scenario := range []string{"correct", "aba", "changed-commitment", "empty-view"} {
		t.Run(scenario, func(t *testing.T) {
			f, request := authenticatedCarrierFixture(t, false)
			view, err := f.b.mounts.AuthorizeMountAcceptance(context.Background(), f.caller, request.Key.MountAnchorUUID)
			if err != nil {
				t.Fatal(err)
			}
			digest, err := f.b.mounts.CommandCommitment(request.Key.ParentAuthority, f.b.owner.AuthorityRef(), request.OperationID, request.Command)
			if err != nil {
				t.Fatal(err)
			}
			before := mountProbe(f.b.owner, request.OperationID, request.Key.MountAnchorUUID)
			var want error
			switch scenario {
			case "aba":
				f.b.auth.validate = values.AuthorizationChanged
				want = values.PublicInvalidContext
			case "changed-commitment":
				digest[0] ^= 1
				want = values.PublicInvalid
			case "empty-view":
				view = values.AuthorizationView{}
				want = values.PublicInvalid
			}
			got, err := f.b.mounts.PrepareApprovedMountControl(context.Background(), f.caller, request.Key.ParentAuthority, request, view, digest)
			if want == nil {
				if err != nil || !got.Prepared {
					t.Fatalf("approved prepare: %#v %v", got, err)
				}
			} else if !errors.Is(err, want) || !reflectEqual(before, mountProbe(f.b.owner, request.OperationID, request.Key.MountAnchorUUID)) {
				t.Fatalf("unsafe %s: %#v %v", scenario, got, err)
			}
		})
	}
}
