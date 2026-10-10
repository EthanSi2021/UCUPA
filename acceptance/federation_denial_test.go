package acceptance_test

import (
	"context"
	"errors"
	"testing"

	"ucupa/shared/values"
	corecontract "ucupa/ucupa-core/contract"
	federationcontract "ucupa/ucupa-core/contract/federation"
	realm "ucupa/ucupa-realm/contract"
)

// Exercises the trusted owner port against a real Realm and real mount. The
// denial protocol is not yet exposed over gRPC by this test.
func TestFederationDenialOwnerRealmBinding(t *testing.T) {
	a, b, _ := publicFederationNetworkFixture(t)
	ctx := context.Background()
	m := values.ElementID(networkEntity)
	b.grant(t, corecontract.PermissionMountRead, values.CoreElementScope(b.config.Expected, m), false)
	mount, err := b.c.Mounts().ReadMount(b.actor, m)
	if err != nil {
		t.Fatal(err)
	}
	caller, err := b.r.TrustLocalCaller(a.actor)
	if err != nil {
		t.Fatal(err)
	}
	request := federationcontract.FederationReadRequest{Boundary: m, Incoming: mount.MountKey, Intent: "query", PrepareObserve: true, Purpose: [32]byte{9}}
	if _, err := b.c.Federation().CaptureFederationDenial(ctx, caller, a.config.Expected, request); !errors.Is(err, values.PublicInvalidContext) {
		t.Fatal("allowed query was hidden", err)
	}
	binding := realm.PermissionBinding{Subject: a.actor, Permission: corecontract.PermissionQueryRegion, Scope: values.AnchorClosureScope(b.config.Expected, m)}
	if _, err := b.r.DeleteBinding(b.actor, binding, b.r.BindingSetRevision(), "deny-discovery"); err != nil {
		t.Fatal(err)
	}
	proof, err := b.c.Federation().CaptureFederationDenial(ctx, caller, a.config.Expected, request)
	if err != nil {
		t.Fatal(err)
	}
	if err := b.c.Federation().CheckFederationDenial(ctx, caller, a.config.Expected, request, proof); err != nil {
		t.Fatal(err)
	}
	if err := b.c.Federation().CheckFederationDenial(ctx, caller, a.config.Expected, request, federationcontract.FederationDenial{}); !errors.Is(err, values.PublicDenied) {
		t.Fatal("zero evidence became a denial proof", err)
	}
	rotation, _ := corecontract.NewRotation(1, 0, 0, 0)
	radius, _ := corecontract.NewBoundedSphere(1)
	if _, err := b.c.Elements().CreateEntity(b.actor, values.ElementID("00000000-0000-4000-8000-000000000008"), corecontract.SpatialTuple{Parent: m, LocalPose: corecontract.Pose{Rotation: rotation}, Range: radius}, "hidden-space-write"); err != nil {
		t.Fatal(err)
	}
	if err := b.c.Federation().CheckFederationDenial(ctx, caller, a.config.Expected, request, proof); err != nil {
		t.Fatal("hidden write invalidated denial", err)
	}
	if err := b.c.Federation().CheckFederationDenial(ctx, caller, b.config.Expected, request, proof); !errors.Is(err, values.PublicDenied) {
		t.Fatal("wrong coordinator", err)
	}
	changed := request
	changed.Purpose[0] ^= 1
	if err := b.c.Federation().CheckFederationDenial(ctx, caller, a.config.Expected, changed, proof); !errors.Is(err, values.PublicDenied) {
		t.Fatal("another query borrowed denial", err)
	}
	if _, err := b.c.Federation().CaptureFederationDenial(ctx, values.CallerContext{Subject: a.actor}, a.config.Expected, request); !errors.Is(err, values.PublicDenied) {
		t.Fatal("unauthenticated absence", err)
	}
	read := request
	read.Intent = "read"
	if _, err := b.c.Federation().CaptureFederationDenial(ctx, caller, a.config.Expected, read); !errors.Is(err, values.PublicInvalid) {
		t.Fatal("Read denial became absent result", err)
	}
	if _, err := b.r.PutBinding(b.actor, binding, b.r.BindingSetRevision(), "restore-discovery"); err != nil {
		t.Fatal(err)
	}
	if err := b.c.Federation().CheckFederationDenial(ctx, caller, a.config.Expected, request, proof); !errors.Is(err, values.PublicInvalidContext) {
		t.Fatal("grant retained denial", err)
	}
	if _, err := b.r.DeleteBinding(b.actor, binding, b.r.BindingSetRevision(), "revoke-discovery"); err != nil {
		t.Fatal(err)
	}
	if err := b.c.Federation().CheckFederationDenial(ctx, caller, a.config.Expected, request, proof); !errors.Is(err, values.PublicInvalidContext) {
		t.Fatal("ABA retained denial", err)
	}
}
