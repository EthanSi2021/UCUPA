package transfer_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"ucupa/composition"
	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	core "ucupa/ucupa-core/instance"
)

const carrierMount values.ElementID = "00000000-0000-4000-8000-000000000030"

// Real parent/carrier Stores and Realm, connected only by the existing mount
// port. No transfer receipt or parent slot is manufactured by this fixture.
func newCarrierFixture(t *testing.T) (*fixture, *core.Core, contract.CarrierCommand) {
	return newCarrierFixtureAt(t, filepath.Join(t.TempDir(), "carrier.db"))
}

func newCarrierFixtureAt(t *testing.T, path string) (*fixture, *core.Core, contract.CarrierCommand) {
	return newCarrierFixtureWithParent(t, path, "local:parent")
}

func newCarrierFixtureWithParent(t *testing.T, path string, parentAddress values.NextHop) (*fixture, *core.Core, contract.CarrierCommand) {
	t.Helper()
	f := newFixture(t)
	s, err := core.CreateDurable(boundaryContacts{f.a}, transferContact, path, f.config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	f.grant(t, actor, contract.PermissionCreateAnchor, values.CoreElementScope(f.c.AuthorityRef(), root), false)
	anchor, err := f.c.Elements().CreateAnchor(actor, carrierMount, tuple(t, root), "carrier-anchor")
	if err != nil || anchor.Record == nil || anchor.Record.Anchor == nil {
		t.Fatal(anchor, err)
	}
	for _, permission := range []values.PermissionID{contract.PermissionMountControl, contract.PermissionMountRead} {
		f.grant(t, actor, permission, values.CoreElementScope(f.c.AuthorityRef(), carrierMount), false)
	}
	f.grant(t, actor, contract.PermissionMountAccept, values.CoreElementScope(s.AuthorityRef(), carrierMount), false)
	f.grant(t, actor, contract.PermissionMountRead, values.CoreElementScope(s.AuthorityRef(), carrierMount), false)
	connections := &composition.LocalMountConnections{}
	if err := connections.BindEndpoint("local:parent", f.c.Mounts().MountPeer()); err != nil {
		t.Fatal(err)
	}
	if err := connections.BindEndpoint("local:carrier", s.Mounts().MountPeer()); err != nil {
		t.Fatal(err)
	}
	for _, c := range []*core.Core{f.c, s} {
		transport, err := connections.TransportFor(c)
		if err != nil {
			t.Fatal(err)
		}
		if err := c.Mounts().ConfigureMountTransport(transport); err != nil {
			t.Fatal(err)
		}
	}
	mount := contract.MountCommand{Kind: contract.MountEstablish, MountAnchorUUID: carrierMount,
		ExpectedElementRevision: anchor.Record.Anchor.Revision, CarrierAuthority: s.AuthorityRef(),
		ParentNextHop: "local:parent", CarrierNextHop: "local:carrier"}
	r, err := f.c.Mounts().ApplyMount(context.Background(), contract.LocalCallers{f.c.AuthorityRef(): actor, s.AuthorityRef(): actor}, "mount-original", mount)
	if err != nil || !r.Settled || r.Decision != contract.MountCommitted {
		t.Fatal("mount fixture", r, err)
	}
	if parentAddress != "local:parent" {
		// Move the existing incoming contact through its actual public CAS;
		// never weaken the local adapter's prohibition on network addresses.
		contact, err := s.Mounts().InspectMountContact(context.Background(), f.caller, carrierMount, true)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := s.Mounts().RepairMountContact(context.Background(), f.caller, "parent-network-contact", carrierMount, true, contact.Revision, parentAddress); err != nil {
			t.Fatal(err)
		}
	}
	inspection, err := f.c.Mounts().InspectMountControl(context.Background(), f.caller, carrierMount)
	if err != nil {
		t.Fatal(err)
	}
	return f, s, contract.CarrierCommand{Source: s.AuthorityRef(), Target: f.command.Target, Mount: carrierMount,
		ExpectedElement: inspection.ElementRevision, ExpectedMount: inspection.MountRevision,
		ParentAddress: parentAddress, SourceAddress: "local:carrier", TargetAddress: "local:replacement"}
}

func TestCarrierControlRequiresOriginalCallerAndParentPermission(t *testing.T) {
	f, source, command := newCarrierFixture(t)
	ctx := context.Background()
	scope := values.CoreElementScope(f.c.AuthorityRef(), carrierMount)
	if _, err := f.c.Transfers().BeginCarrierTransition(ctx, f.caller, "replace", command); !errors.Is(err, values.PublicDenied) {
		t.Fatal("ordinary mount permission implied replacement", err)
	}
	// Source export is neither a parent authorization nor a parent scope.
	f.grant(t, actor, contract.PermissionTransitionExport, values.CoreElementScope(source.AuthorityRef(), carrierMount), false)
	if _, err := f.c.Transfers().BeginCarrierTransition(ctx, f.caller, "replace", command); !errors.Is(err, values.PublicDenied) {
		t.Fatal("source export implied parent control", err)
	}
	f.grant(t, actor, contract.PermissionTransitionCommit, scope, false)
	if _, err := f.c.Transfers().BeginCarrierTransition(ctx, values.CallerContext{Subject: actor}, "replace", command); !errors.Is(err, values.PublicDenied) {
		t.Fatal("self-asserted subject authenticated", err)
	}
	begin, err := f.c.Transfers().BeginCarrierTransition(ctx, f.caller, "replace", command)
	if err != nil || begin.Progress != contract.CarrierReserved {
		t.Fatal(begin, err)
	}
	if _, err := f.c.Transfers().InspectCarrierTransition(ctx, f.caller, "replace", carrierMount); !errors.Is(err, values.PublicDenied) {
		t.Fatal("commit implied inspection", err)
	}
	f.grant(t, actor, contract.PermissionTransitionInspect, scope, false)
	other := values.PrincipalRef{Realm: actor.Realm, Principal: "other"}
	for _, permission := range []values.PermissionID{contract.PermissionTransitionCommit, contract.PermissionTransitionInspect} {
		f.grant(t, other, permission, scope, false)
	}
	caller, err := f.a.TrustLocalCaller(other)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := f.c.Transfers().InspectCarrierTransition(ctx, caller, "replace", carrierMount); !errors.Is(err, values.PublicDenied) || got != (contract.CarrierStatus{}) {
		t.Fatal("other actor read operation", got, err)
	}
	if _, err := f.c.Transfers().CancelCarrierTransition(ctx, caller, "replace", carrierMount, begin.Revision); !errors.Is(err, values.PublicDenied) {
		t.Fatal("other actor cancelled operation", err)
	}
	read, err := source.Mounts().ReadMount(actor, carrierMount)
	if err != nil || !read.WriteGateOpen {
		t.Fatal("reservation closed source gate", read, err)
	}
	f.grant(t, actor, contract.PermissionTransitionCommit, scope, true)
	if _, err := f.c.Transfers().CancelCarrierTransition(ctx, f.caller, "replace", carrierMount, begin.Revision); !errors.Is(err, values.PublicDenied) {
		t.Fatal("revoked caller cancelled", err)
	}
	f.grant(t, actor, contract.PermissionTransitionCommit, scope, false)
	closed, err := f.c.Transfers().CancelCarrierTransition(ctx, f.caller, "replace", carrierMount, begin.Revision)
	if err != nil || closed.Progress != contract.CarrierCancelled {
		t.Fatal(closed, err)
	}
	if err := f.c.Close(); err != nil {
		t.Fatal(err)
	}
	f.c, err = core.OpenDurable(boundaryContacts{f.a}, f.path, f.config)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := f.c.Transfers().InspectCarrierTransition(ctx, f.caller, "replace", carrierMount); err != nil || got != closed {
		t.Fatal("reopen lost parent result", got, err)
	}
	if got, err := f.c.Transfers().BeginCarrierTransition(ctx, f.caller, "replace", command); err != nil || got != begin {
		t.Fatal("reopen rebound original Begin", got, err)
	}
}

func TestCarrierControlRevalidatesViewAndCancellationBeforeReservation(t *testing.T) {
	for _, revoke := range []bool{false, true} {
		f, _, command := newCarrierFixture(t)
		scope := values.CoreElementScope(f.c.AuthorityRef(), carrierMount)
		f.grant(t, actor, contract.PermissionTransitionCommit, scope, false)
		f.grant(t, actor, contract.PermissionTransitionInspect, scope, false)
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)
		f.a.afterAuthorize(func() {
			if revoke {
				f.grant(t, actor, contract.PermissionTransitionCommit, scope, true)
				f.grant(t, actor, contract.PermissionTransitionCommit, scope, false)
			} else {
				cancel()
			}
		})
		if _, err := f.c.Transfers().BeginCarrierTransition(ctx, f.caller, "rejected", command); err == nil {
			t.Fatal("stale view or cancelled request reserved a slot")
		}
		if _, err := f.c.Transfers().InspectCarrierTransition(context.Background(), f.caller, "rejected", carrierMount); !errors.Is(err, contract.ErrNotFound) {
			t.Fatal("failed request left an operation", err)
		}
		if _, err := f.c.Transfers().BeginCarrierTransition(context.Background(), f.caller, "fresh", command); err != nil {
			t.Fatal("failed request occupied the slot", err)
		}
	}
}
