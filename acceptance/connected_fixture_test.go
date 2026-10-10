package acceptance_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"ucupa/composition"
	"ucupa/shared/values"
	corecontract "ucupa/ucupa-core/contract"
	core "ucupa/ucupa-core/instance"
	realm "ucupa/ucupa-realm/contract"
	realminstance "ucupa/ucupa-realm/instance"
)

const (
	connectedRootEntity    values.ElementID = "00000000-0000-4000-8000-000000009010"
	connectedSiblingEntity values.ElementID = "00000000-0000-4000-8000-000000009011"
	connectedDirectEntity  values.ElementID = "00000000-0000-4000-8000-000000009012"
)

type connectedFixture struct {
	*mountFixture
	reads            composition.LocalConnectedConnections
	configA, configB corecontract.Config
}

func newConnectedFixture(t *testing.T, retainA, retainB bool) *connectedFixture {
	t.Helper()
	f := &connectedFixture{mountFixture: newMountFixture(t)}
	if retainA {
		ttl := time.Minute
		f.configA.SnapshotRetention = &ttl
	}
	if retainB {
		ttl := time.Minute
		f.configB.SnapshotRetention = &ttl
	}
	// The existing durable mount fixture supplies independent Realms and stable
	// owner grants. Reopen selects each startup configuration before any query.
	f.reopenConnected()
	permissions := []values.PermissionID{
		corecontract.PermissionReadElement, corecontract.PermissionListDirect, corecontract.PermissionQueryRegion,
		corecontract.PermissionCreateAnchor, corecontract.PermissionCreateEntity, corecontract.PermissionPlaceElement,
		corecontract.PermissionUpdateElement, corecontract.PermissionDeleteAnchor, corecontract.PermissionDeleteEntity,
	}
	for _, id := range []values.ElementID{mountRoot, mountAnchor, mountOther, mountBranch, mountEntity,
		connectedRootEntity, connectedSiblingEntity, connectedDirectEntity} {
		connectedGrant(t, f.policyA, f.actorA, f.a.AuthorityRef(), id, permissions...)
		connectedGrant(t, f.policyB, f.actorB, f.b.AuthorityRef(), id, permissions...)
	}
	f.establish()
	if _, err := f.a.Elements().CreateAnchor(f.actorA, mountOther, f.tuple(mountRoot, -20, 40), "connected-sibling"); err != nil {
		t.Fatal(err)
	}
	if _, err := f.b.Elements().CreateAnchor(f.actorB, mountBranch, f.tuple(mountAnchor, -3, 40), "connected-branch"); err != nil {
		t.Fatal(err)
	}
	for _, item := range []struct {
		owner  *core.Core
		actor  values.PrincipalRef
		id     values.ElementID
		parent values.ElementID
		x, r   float64
	}{
		{f.a, f.actorA, connectedRootEntity, mountRoot, -2, 5},
		{f.a, f.actorA, connectedSiblingEntity, mountOther, 0, 1},
		{f.b, f.actorB, mountEntity, mountBranch, 2, 3},
		{f.b, f.actorB, connectedDirectEntity, mountAnchor, 15, 4},
	} {
		if _, err := item.owner.Elements().CreateEntity(item.actor, item.id, f.tuple(item.parent, item.x, item.r), corecontract.OperationID("connected-create:"+string(item.id))); err != nil {
			t.Fatal(err)
		}
	}
	return f
}

func connectedGrant(t *testing.T, policy *realminstance.Realm, actor values.PrincipalRef, authority values.FactAuthorityRef, id values.ElementID, permissions ...values.PermissionID) {
	t.Helper()
	scope := values.CoreElementScope(authority, id)
	for _, permission := range permissions {
		if policy.Authorize(actor, values.Requirement{Permission: permission, Scope: scope}) {
			continue
		}
		operation := realm.OperationID(fmt.Sprintf("connected-grant:%s:%s", id, permission))
		if _, err := policy.PutBinding(actor, binding(actor, permission, scope), policy.BindingSetRevision(), operation); err != nil {
			t.Fatal(err)
		}
	}
}

func (f *connectedFixture) reconnectReads() {
	f.t.Helper()
	for _, pair := range []struct {
		address values.NextHop
		owner   *core.Core
	}{{mountAddressA, f.a}, {mountAddressB, f.b}} {
		if err := f.reads.BindEndpoint(pair.address, pair.owner.Connected().ConnectedPeer()); err != nil {
			f.t.Fatal(err)
		}
		transport, err := f.reads.TransportFor(pair.owner)
		if err != nil {
			f.t.Fatal(err)
		}
		if err := pair.owner.Connected().ConfigureConnectedTransport(transport); err != nil {
			f.t.Fatal(err)
		}
	}
}

func (f *connectedFixture) reopenConnected() {
	f.t.Helper()
	refA, refB := f.a.AuthorityRef(), f.b.AuthorityRef()
	if err := f.a.Close(); err != nil {
		f.t.Fatal(err)
	}
	if err := f.b.Close(); err != nil {
		f.t.Fatal(err)
	}
	var err error
	f.a, err = core.OpenDurable(f.realmLinks, f.pathA, f.configA)
	if err != nil {
		f.t.Fatal(err)
	}
	f.b, err = core.OpenDurable(f.realmLinks, f.pathB, f.configB)
	if err != nil {
		f.t.Fatal(err)
	}
	if f.a.AuthorityRef() != refA || f.b.AuthorityRef() != refB {
		f.t.Fatal("configuration reopen changed stable identity")
	}
	f.connect()
	f.reconnectReads()
}

func (f *connectedFixture) queryFromB() corecontract.PathRegionQuery {
	start := corecontract.Path{OriginAnchorID: mountBranch}
	return corecontract.PathRegionQuery{StartPath: start, UpDepth: -1, BasisPath: start,
		Sphere: corecontract.QuerySphere{Radius: 100}, Selector: corecontract.QueryBoth}
}

func (f *connectedFixture) queryFromA() corecontract.PathRegionQuery {
	start := corecontract.Path{OriginAnchorID: mountRoot}
	return corecontract.PathRegionQuery{StartPath: start, BasisPath: start,
		Sphere: corecontract.QuerySphere{Center: corecontract.Vec3{X: 7}, Radius: 100}, Selector: corecontract.QueryBoth}
}

func (f *connectedFixture) moveBEntity(operation string) {
	f.t.Helper()
	read, err := f.b.Elements().ReadElement(f.actorB, mountEntity, nil)
	if err != nil {
		f.t.Fatal(err)
	}
	if _, err := f.b.Elements().UpdateEntity(f.actorB, mountEntity, f.tuple(mountBranch, 20, 3), read.Record.Entity.Revision, corecontract.OperationID(operation)); err != nil {
		f.t.Fatal(err)
	}
}

func (f *connectedFixture) revokePermission(onA bool, id values.ElementID, permission values.PermissionID) {
	f.t.Helper()
	policy, actor, owner := f.policyB, f.actorB, f.b.AuthorityRef()
	if onA {
		policy, actor, owner = f.policyA, f.actorA, f.a.AuthorityRef()
	}
	grant := binding(actor, permission, values.CoreElementScope(owner, id))
	if _, err := policy.DeleteBinding(actor, grant, policy.BindingSetRevision(), realm.OperationID(fmt.Sprintf("connected-revoke:%s:%s", id, permission))); err != nil {
		f.t.Fatal(err)
	}
}

func connectedReadPath(scope corecontract.Path, match corecontract.ConnectedMatch) corecontract.Path {
	scope.Down = append(append([]values.ElementID(nil), scope.Down...), match.Path...)
	return scope
}

func (f *connectedFixture) queryB(limit uint16) corecontract.ConnectedQueryPage {
	f.t.Helper()
	page, err := f.b.Connected().QueryRegionPath(context.Background(), f.callers(), f.queryFromB(), nil, limit, nil)
	if err != nil {
		f.t.Fatal(err)
	}
	return page
}
