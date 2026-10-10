package acceptance_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"

	"ucupa/composition"
	"ucupa/shared/values"
	pb "ucupa/ucupa-core/bindings/go/protobuf"
	corecontract "ucupa/ucupa-core/contract"
	core "ucupa/ucupa-core/instance"
	coresdk "ucupa/ucupa-core/sdk/go"
	mountconn "ucupa/ucupa-core/transport/grpc/mountconn"
	realm "ucupa/ucupa-realm/contract"
)

// The parent enters through its authenticated owner port; all parent/carrier
// approval and control traffic uses the production pinned network adapter.
// Public application command token construction is a separate remaining edge.
func TestNetworkAuthenticatedMountDriverEstablishResizeRevoke(t *testing.T) {
	b, a, service, _ := mountApprovalFixture(t, true)
	ctx := context.Background()
	element := values.ElementID(networkEntity)
	for _, permission := range []values.PermissionID{corecontract.PermissionMountControl, corecontract.PermissionMountRead, corecontract.PermissionUpdateElement} {
		a.grant(t, permission, values.CoreElementScope(a.c.AuthorityRef(), element), false)
	}
	for _, permission := range []values.PermissionID{corecontract.PermissionCreateAnchor, corecontract.PermissionPlaceElement} {
		a.grant(t, permission, values.CoreElementScope(a.c.AuthorityRef(), networkRoot), false)
	}
	b.grant(t, corecontract.PermissionMountAccept, values.CoreElementScope(b.c.AuthorityRef(), element), false)
	sphere, _ := corecontract.NewBoundedSphere(10)
	rotation, _ := corecontract.NewRotation(1, 0, 0, 0)
	tuple := corecontract.SpatialTuple{Parent: networkRoot, LocalPose: corecontract.Pose{Rotation: rotation}, Range: sphere}
	created, err := a.c.Elements().CreateAnchor(a.actor, element, tuple, "network-parent-anchor")
	if err != nil {
		t.Fatal(err)
	}
	command := corecontract.MountCommand{Kind: corecontract.MountEstablish, MountAnchorUUID: element, ExpectedElementRevision: created.Record.Anchor.Revision, CarrierAuthority: b.c.AuthorityRef(), CarrierNextHop: b.config.NextHop, ParentNextHop: a.config.NextHop}
	connections, err := mountconn.NewMountConnections(a.c.AuthorityRef(), a.c.Mounts(), []*coresdk.Client{service})
	if err != nil {
		t.Fatal(err)
	}
	caller, err := (composition.RealmNetworkAuthentication{Connections: a.realmLinks, Owner: a.c.Access()}).Establish(ctx, a.actor.Realm, a.id, a.secret)
	if err != nil {
		t.Fatal(err)
	}
	apply := func(id corecontract.OperationID, command corecontract.MountCommand, approval bool) corecontract.MountControlResult {
		t.Helper()
		invocation := connections
		if approval {
			digest, err := a.c.Mounts().CommandCommitment(a.c.AuthorityRef(), b.c.AuthorityRef(), id, command)
			if err != nil {
				t.Fatal(err)
			}
			grant := createMountApproval(t, b, &pb.MountApprovalBinding{Actor: proto.String(string(a.c.AuthorityRef())), Element: proto.String(string(element)), OperationId: []byte(id), CommandDigest: digest[:]}, 5)
			invocation, err = connections.WithApproval(grant)
			if err != nil {
				t.Fatal(err)
			}
		}
		view, err := a.c.Mounts().AuthorizeMountControl(ctx, caller, command)
		if err != nil {
			t.Fatal(err)
		}
		r, err := a.c.Mounts().ApplyMountControl(ctx, caller, view, id, command, invocation)
		if err != nil || r.Decision != corecontract.MountCommitted || !r.Settled {
			t.Fatalf("%s: %#v %v", id, r, err)
		}
		return r
	}
	apply("network-establish", command, true)
	apply("network-establish", command, false)
	mount, err := a.c.Mounts().ReadMount(a.actor, element)
	if err != nil {
		t.Fatal(err)
	}
	tuple.Range, _ = corecontract.NewBoundedSphere(5)
	resize := corecontract.MountCommand{Kind: corecontract.MountUpdate, MountAnchorUUID: element, ExpectedElementRevision: created.Record.Anchor.Revision, ExpectedMountRevision: mount.MountRevision, Final: tuple}
	resized := apply("network-resize", resize, true)
	mount, err = a.c.Mounts().ReadMount(a.actor, element)
	if err != nil || mount.Range.Radius() != 5 {
		t.Fatalf("resized: %#v %v", mount, err)
	}
	// A position-only change does not ask B for a new acceptance. The range
	// and incoming write gate remain unchanged under the original contract.
	tuple.LocalPose.Translation.X = 1
	poseOnly := corecontract.MountCommand{Kind: corecontract.MountUpdate, MountAnchorUUID: element, ExpectedElementRevision: resized.Record.Anchor.Revision, ExpectedMountRevision: mount.MountRevision, Final: tuple}
	apply("network-pose-only", poseOnly, false)
	mount, err = a.c.Mounts().ReadMount(a.actor, element)
	if err != nil {
		t.Fatal(err)
	}
	revoke := corecontract.MountCommand{Kind: corecontract.MountRevoke, MountAnchorUUID: element, ExpectedMountRevision: mount.MountRevision}
	apply("network-revoke", revoke, false)
	mount, err = a.c.Mounts().ReadMount(a.actor, element)
	if err != nil || mount.State != corecontract.MountDetached {
		t.Fatalf("detached: %#v %v", mount, err)
	}
}

// Exercise the real destructive Realm ReleaseHandle, not a local no-op port.
// The Core is reopened in-process only after its product has created the anchor
// and stopped; authorization remains real pinned gRPC/TLS to the Realm product.
func TestNetworkRealmMountResolutionRetainsEvidenceUntilAdapterFinishes(t *testing.T) {
	f := newNetworkRealmContactFixture(t)
	element := values.ElementID(networkContactChild)
	scope := values.CoreElementScope(f.owner, element)
	for _, permission := range []values.PermissionID{corecontract.PermissionMountControl, corecontract.PermissionMountRead} {
		if err := f.grant(permission, scope, false); err != nil {
			t.Fatal(err)
		}
	}
	f.startRealm()
	product := f.startCore()
	client := f.connect(t)
	created, err := client.Core().CreateAnchor(context.Background(), &pb.CreateAnchorRequest{Handle: f.enter(t, client, pb.Intent_CREATE_ANCHOR),
		Element: proto.String(string(element)), OperationId: []byte("mount-evidence-anchor"), Final: networkContactTuple(10)})
	if err != nil || created.GetResult() == nil {
		t.Fatalf("create anchor: %v %v", created, err)
	}
	product.gracefulStop()
	connections := &composition.NetworkRealmConnections{}
	if err := connections.BindNetworkEndpoint(f.realmAddress, f.realm, composition.NetworkRealmEndpointConfig{
		Roots: f.realmRoots, SPKISHA256: f.realmSPKI, Service: &f.serviceCert, RPCDeadline: 5 * time.Second,
		ConnectTimeout: time.Second, MaxResponseBytes: 8 << 20, MaxMetadataBytes: 8192}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = connections.UnbindNetworkEndpoint(f.realmAddress) })
	owner, err := core.OpenDurable(connections, filepath.Join(f.dir, "core.db"), corecontract.Config{
		ExpectedRealm: f.realm, PublicAccessPolicy: &values.PublicAccessPolicy{ContextTTL: time.Minute, ObservationTTL: time.Minute}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = owner.Close() })
	caller, err := connections.EstablishCaller(context.Background(), f.realmAddress, f.realm, realm.CredentialEvidence{
		Audience: f.realm, ID: realm.CredentialID(f.id), Secret: realm.CredentialSecret(f.secret)})
	if err != nil {
		t.Fatal(err)
	}
	inspection, err := owner.Mounts().InspectMountControl(context.Background(), caller, element)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	command, view, release, err := owner.Mounts().ResolveMountControl(ctx, caller, "network-mount-evidence", corecontract.PublicMountCommand{
		Kind: corecontract.MountEstablish, Element: element, ExpectedElement: inspection.ElementRevision,
		Carrier: networkContactForeignUser, CarrierNextHop: "local:unconfigured-carrier", ParentNextHop: f.coreAddress})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if release != nil {
			release()
		}
	}()
	requirement := values.Requirement{Permission: corecontract.PermissionMountControl, Scope: scope}
	if decision := connections.ValidateView(ctx, f.realmAddress, f.realm, caller, view, requirement); decision != values.AuthorizationValid {
		t.Fatalf("resolver prematurely released real Realm view: %v", decision)
	}
	// No carrier transport is configured: fail at the peer boundary, not at the
	// unchanged Realm evidence, and never reserve an intent.
	if _, err := owner.Mounts().ApplyMountControl(ctx, caller, view, "network-mount-evidence", command, nil); !errors.Is(err, values.PublicUnavailable) {
		t.Fatalf("expected unconfigured peer, not invalidated evidence: %v", err)
	}
	cancel()
	release()
	release = nil
	if decision := connections.ValidateView(context.Background(), f.realmAddress, f.realm, caller, view, requirement); decision != values.AuthorizationDenied {
		t.Fatalf("adapter completion failed to release real handle after cancellation: %v", decision)
	}
}
