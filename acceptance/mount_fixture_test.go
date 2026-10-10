package acceptance_test

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"

	"ucupa/composition"
	"ucupa/shared/values"
	corecontract "ucupa/ucupa-core/contract"
	core "ucupa/ucupa-core/instance"
	realm "ucupa/ucupa-realm/contract"
	realminstance "ucupa/ucupa-realm/instance"
)

const (
	mountRoot     values.ElementID = "00000000-0000-4000-8000-000000009001"
	mountAnchor   values.ElementID = "00000000-0000-4000-8000-000000009002"
	mountEntity   values.ElementID = "00000000-0000-4000-8000-000000009003"
	mountOther    values.ElementID = "00000000-0000-4000-8000-000000009004"
	mountBranch   values.ElementID = "00000000-0000-4000-8000-000000009005"
	mountAddressA values.NextHop   = "local:mount-parent"
	mountAddressB values.NextHop   = "local:mount-carrier"
	mountRealmA   values.NextHop   = "local:mount-parent-realm"
	mountRealmB   values.NextHop   = "local:mount-carrier-realm"
)

type mountFixture struct {
	t                *testing.T
	a, b             *core.Core
	policyA, policyB *realminstance.Realm
	realmLinks       *composition.LocalRealmConnections
	actorA, actorB   values.PrincipalRef
	pathA, pathB     string
	connections      composition.LocalMountConnections
	command          corecontract.MountCommand
}

func newMountFixture(t *testing.T) *mountFixture {
	t.Helper()
	f := &mountFixture{t: t}
	f.pathA, f.pathB = filepath.Join(t.TempDir(), "parent.db"), filepath.Join(t.TempDir(), "carrier.db")
	f.policyA, f.actorA = mountPolicy(t, "mount-parent-realm")
	f.policyB, f.actorB = mountPolicy(t, "mount-carrier-realm")
	f.realmLinks = &composition.LocalRealmConnections{}
	if err := f.realmLinks.BindEndpoint(mountRealmA, f.policyA); err != nil {
		t.Fatal(err)
	}
	if err := f.realmLinks.BindEndpoint(mountRealmB, f.policyB); err != nil {
		t.Fatal(err)
	}
	var err error
	f.a, err = core.CreateDurable(f.realmLinks, corecontract.RealmInitialization{Realm: f.actorA.Realm, NextHop: mountRealmA}, f.pathA, corecontract.Config{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.a.Close() })
	f.b, err = core.CreateDurable(f.realmLinks, corecontract.RealmInitialization{Realm: f.actorB.Realm, NextHop: mountRealmB}, f.pathB, corecontract.Config{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.b.Close() })
	mountGrant(t, f.policyA, f.actorA, f.a.AuthorityRef(), mountRoot,
		corecontract.PermissionCreateRoot, corecontract.PermissionCreateAnchor, corecontract.PermissionPlaceElement, corecontract.PermissionReadElement, corecontract.PermissionQueryRegion)
	mountGrant(t, f.policyA, f.actorA, f.a.AuthorityRef(), mountAnchor,
		corecontract.PermissionMountControl, corecontract.PermissionMountRead, corecontract.PermissionReadElement, corecontract.PermissionUpdateElement,
		corecontract.PermissionDeleteAnchor, corecontract.PermissionListDirect, corecontract.PermissionCreateAnchor, corecontract.PermissionCreateEntity)
	mountGrant(t, f.policyA, f.actorA, f.a.AuthorityRef(), mountOther, corecontract.PermissionReadElement)
	mountGrant(t, f.policyB, f.actorB, f.b.AuthorityRef(), mountAnchor,
		corecontract.PermissionMountAccept, corecontract.PermissionMountControl, corecontract.PermissionMountRead,
		corecontract.PermissionReadElement, corecontract.PermissionListDirect, corecontract.PermissionCreateEntity, corecontract.PermissionCreateAnchor, corecontract.PermissionPlaceElement, corecontract.PermissionQueryRegion)
	mountGrant(t, f.policyB, f.actorB, f.b.AuthorityRef(), mountRoot, corecontract.PermissionCreateRoot)
	for _, id := range []values.ElementID{mountEntity, mountOther, mountBranch} {
		mountGrant(t, f.policyB, f.actorB, f.b.AuthorityRef(), id,
			corecontract.PermissionReadElement, corecontract.PermissionUpdateElement, corecontract.PermissionDeleteEntity)
	}
	if _, err := f.a.Elements().CreateRoot(f.actorA, mountRoot, "mount-create-root"); err != nil {
		t.Fatal(err)
	}
	created, err := f.a.Elements().CreateAnchor(f.actorA, mountAnchor, f.tuple(mountRoot, 10, 100), "mount-create-anchor")
	if err != nil {
		t.Fatal(err)
	}
	f.command = corecontract.MountCommand{
		Kind: corecontract.MountEstablish, MountAnchorUUID: mountAnchor, ExpectedElementRevision: created.Record.Anchor.Revision,
		CarrierAuthority: f.b.AuthorityRef(), CarrierNextHop: mountAddressB, ParentNextHop: mountAddressA,
	}
	f.connect()
	return f
}

func mountPolicy(t *testing.T, id values.RealmID) (*realminstance.Realm, values.PrincipalRef) {
	t.Helper()
	actor := values.PrincipalRef{Realm: id, Principal: "operator"}
	policy, err := realminstance.New(realm.Seed{ID: id, LocalPrincipals: []values.PrincipalID{actor.Principal}, Bindings: []realm.PermissionBinding{
		binding(actor, realm.PermissionPutBinding, values.RealmScope(id)),
		binding(actor, realm.PermissionDeleteBinding, values.RealmScope(id)),
	}})
	if err != nil {
		t.Fatal(err)
	}
	return policy, actor
}

func mountGrant(t *testing.T, policy *realminstance.Realm, actor values.PrincipalRef, authority values.FactAuthorityRef, id values.ElementID, permissions ...values.PermissionID) {
	t.Helper()
	for _, permission := range permissions {
		grant := binding(actor, permission, values.CoreElementScope(authority, id))
		operation := realm.OperationID(fmt.Sprintf("grant:%s:%s", id, permission))
		if _, err := policy.PutBinding(actor, grant, policy.BindingSetRevision(), operation); err != nil {
			t.Fatal(err)
		}
	}
}

func (f *mountFixture) connect() {
	f.t.Helper()
	for _, pair := range []struct {
		address values.NextHop
		spatial *core.Core
	}{{mountAddressA, f.a}, {mountAddressB, f.b}} {
		if err := f.connections.BindEndpoint(pair.address, pair.spatial.Mounts().MountPeer()); err != nil {
			f.t.Fatal(err)
		}
		transport, err := f.connections.TransportFor(pair.spatial)
		if err != nil {
			f.t.Fatal(err)
		}
		if err := pair.spatial.Mounts().ConfigureMountTransport(transport); err != nil {
			f.t.Fatal(err)
		}
	}
}

func (f *mountFixture) reopen() {
	f.t.Helper()
	refA, refB := f.a.AuthorityRef(), f.b.AuthorityRef()
	if err := f.a.Close(); err != nil {
		f.t.Fatal(err)
	}
	if err := f.b.Close(); err != nil {
		f.t.Fatal(err)
	}
	var err error
	f.a, err = core.OpenDurable(f.realmLinks, f.pathA, corecontract.Config{})
	if err != nil {
		f.t.Fatalf("reopen parent: %v", err)
	}
	f.b, err = core.OpenDurable(f.realmLinks, f.pathB, corecontract.Config{})
	if err != nil {
		f.t.Fatalf("reopen carrier: %v", err)
	}
	if f.a.AuthorityRef() != refA || f.b.AuthorityRef() != refB {
		f.t.Fatal("reopen changed owner identity")
	}
	f.connect()
}

func (f *mountFixture) callers() corecontract.LocalCallers {
	return corecontract.LocalCallers{f.a.AuthorityRef(): f.actorA, f.b.AuthorityRef(): f.actorB}
}

func (f *mountFixture) tuple(parent values.ElementID, x, radius float64) corecontract.SpatialTuple {
	f.t.Helper()
	return corecontract.SpatialTuple{Parent: parent, LocalPose: mustPose(f.t, corecontract.Vec3{X: x}, mustRotation(f.t, 1, 0, 0, 0)), Range: mustSphere(f.t, radius)}
}

func (f *mountFixture) establish() corecontract.MountControlResult {
	f.t.Helper()
	result, err := f.a.Mounts().ApplyMount(context.Background(), f.callers(), "mount-establish", f.command)
	if err != nil || result.Decision != corecontract.MountCommitted || !result.Settled {
		f.t.Fatalf("establish: %#v, %v", result, err)
	}
	return result
}

func (f *mountFixture) readA() corecontract.MountRead {
	f.t.Helper()
	result, err := f.a.Mounts().ReadMount(f.actorA, mountAnchor)
	if err != nil {
		f.t.Fatal(err)
	}
	return result
}

func (f *mountFixture) readB() corecontract.MountRead {
	f.t.Helper()
	result, err := f.b.Mounts().ReadMount(f.actorB, mountAnchor)
	if err != nil {
		f.t.Fatal(err)
	}
	return result
}

func (f *mountFixture) rangeCommand(radius float64) corecontract.MountCommand {
	f.t.Helper()
	read, err := f.a.Elements().ReadElement(f.actorA, mountAnchor, nil)
	if err != nil {
		f.t.Fatal(err)
	}
	return corecontract.MountCommand{Kind: corecontract.MountUpdate, MountAnchorUUID: mountAnchor,
		ExpectedElementRevision: read.Record.Anchor.Revision, ExpectedMountRevision: f.readA().MountRevision,
		Final: f.tuple(mountRoot, 10, radius)}
}

// Faults occur at public control ports, with no owner-state access.
type mountTransportHook struct {
	base         corecontract.MountTransport
	afterPrepare func(corecontract.MountPrepared)
	finishError  error
}

func (hook mountTransportHook) Prepare(ctx context.Context, address values.NextHop, expected values.FactAuthorityRef, request corecontract.MountPrepareRequest) (corecontract.MountPrepared, error) {
	result, err := hook.base.Prepare(ctx, address, expected, request)
	if err == nil && hook.afterPrepare != nil {
		hook.afterPrepare(result)
	}
	return result, err
}

func (hook mountTransportHook) Finish(ctx context.Context, address values.NextHop, expected values.FactAuthorityRef, request corecontract.MountFinishRequest) (corecontract.MountClosed, error) {
	if hook.finishError != nil {
		return corecontract.MountClosed{}, hook.finishError
	}
	return hook.base.Finish(ctx, address, expected, request)
}

func (hook mountTransportHook) ReadDecision(ctx context.Context, address values.NextHop, expected values.FactAuthorityRef, attempt corecontract.MountAttemptKey) (corecontract.MountDecisionEvidence, error) {
	return hook.base.ReadDecision(ctx, address, expected, attempt)
}
