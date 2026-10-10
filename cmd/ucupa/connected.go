package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"ucupa/composition"
	"ucupa/shared/values"
	corecontract "ucupa/ucupa-core/contract"
	core "ucupa/ucupa-core/instance"
	realm "ucupa/ucupa-realm/contract"
	realminstance "ucupa/ucupa-realm/instance"
)

const (
	connectedRoot     values.ElementID = "00000000-0000-4000-8000-000000004001"
	connectedMount    values.ElementID = "00000000-0000-4000-8000-000000004002"
	connectedLocal    values.ElementID = "00000000-0000-4000-8000-000000004003"
	connectedRemote   values.ElementID = "00000000-0000-4000-8000-000000004004"
	connectedAddressA values.NextHop   = "local:demo-parent"
	connectedAddressB values.NextHop   = "local:demo-carrier"
)

type connectedDemoOwner struct {
	core             *core.Core
	realmConnections *composition.LocalRealmConnections
	actor            values.PrincipalRef
	path             string
	config           corecontract.Config
}

// runConnected demonstrates the public dual-durable-Core chain. It neither
// accesses private state nor presents process-local endpoints as network RPC.
func runConnected(output io.Writer) error {
	directory, err := os.MkdirTemp("", "ucupa-connected-demo-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(directory)
	ttl := time.Minute
	config := corecontract.Config{SnapshotRetention: &ttl}
	a, err := newConnectedDemoOwner(filepath.Join(directory, "a.db"), "demo-parent", config)
	if err != nil {
		return err
	}
	defer func() { _ = a.core.Close() }()
	b, err := newConnectedDemoOwner(filepath.Join(directory, "b.db"), "demo-carrier", config)
	if err != nil {
		return err
	}
	defer func() { _ = b.core.Close() }()
	if err := connectDemoOwners(a, b); err != nil {
		return err
	}
	if _, err := a.core.Elements().CreateRoot(a.actor, connectedRoot, "root"); err != nil {
		return err
	}
	anchor, err := a.core.Elements().CreateAnchor(a.actor, connectedMount, connectedTuple(connectedRoot, 10, 100), "anchor")
	if err != nil {
		return err
	}
	if _, err := a.core.Elements().CreateEntity(a.actor, connectedLocal, connectedTuple(connectedRoot, 9, 1), "local"); err != nil {
		return err
	}
	ctx := context.Background()
	callers := corecontract.LocalCallers{a.core.AuthorityRef(): a.actor, b.core.AuthorityRef(): b.actor}
	established, err := a.core.Mounts().ApplyMount(ctx, callers, "mount", corecontract.MountCommand{
		Kind: corecontract.MountEstablish, MountAnchorUUID: connectedMount, ExpectedElementRevision: anchor.Record.Anchor.Revision,
		CarrierAuthority: b.core.AuthorityRef(), CarrierNextHop: connectedAddressB, ParentNextHop: connectedAddressA,
	})
	if err != nil || established.Decision != corecontract.MountCommitted || !established.Settled {
		return fmt.Errorf("demo establish: %#v, %v", established, err)
	}
	created, err := b.core.Elements().CreateEntity(b.actor, connectedRemote, connectedTuple(connectedMount, -1, 1), "remote")
	if err != nil {
		return err
	}
	query := corecontract.PathRegionQuery{
		StartPath: corecontract.Path{OriginAnchorID: connectedMount}, UpDepth: -1,
		BasisPath: corecontract.Path{OriginAnchorID: connectedMount, Up: 1},
		Sphere:    corecontract.QuerySphere{Center: corecontract.Vec3{X: 9}}, Selector: corecontract.QueryEntities,
	}
	first, err := b.core.Connected().QueryRegionPath(ctx, callers, query, nil, 1, nil)
	if err != nil || first.ReadMode != corecontract.QueryRetained || first.ActualUpDepth != 1 ||
		first.Status != corecontract.QueryPartial || first.Gap != corecontract.QueryNoGap || len(first.Matches) != 1 ||
		first.Matches[0].ElementID != connectedLocal || first.Continuation == nil {
		return fmt.Errorf("demo first connected page: %#v, %v", first, err)
	}
	defer func() { _ = b.core.Connected().ReleaseConnectedCut(ctx, callers, first.Cut) }()
	if _, err := b.core.Elements().UpdateEntity(b.actor, connectedRemote, connectedTuple(connectedMount, -2, 1), created.Record.Entity.Revision, "move"); err != nil {
		return err
	}
	second, err := b.core.Connected().QueryRegionPath(ctx, callers, query, nil, 1, first.Continuation)
	if err != nil || second.Status != corecontract.QueryComplete || second.Gap != corecontract.QueryNoGap ||
		second.Continuation != nil || second.ReadMode != corecontract.QueryRetained || !second.Cut.Equal(first.Cut) ||
		len(second.Matches) != 1 || second.Matches[0].ElementID != connectedRemote {
		return fmt.Errorf("demo second connected page: %#v, %v", second, err)
	}
	path := second.ResolvedScopePath
	path.Down = append(append([]values.ElementID(nil), path.Down...), second.Matches[0].Path...)
	old, err := b.core.Connected().ResolvePath(ctx, callers, path, &second.Cut)
	if err != nil || old.State != corecontract.ExistencePresent || old.Record == nil || old.Record.Entity == nil ||
		old.Record.Entity.LocalPose.Translation.X != -1 || !old.Record.Entity.Revision.Equal(second.Matches[0].Revision) {
		return fmt.Errorf("demo snapshot exact read: %#v, %v", old, err)
	}
	if err := b.core.Connected().ReleaseConnectedCut(ctx, callers, first.Cut); err != nil {
		return err
	}
	if err := b.core.Close(); err != nil {
		return err
	}
	ref := b.core.AuthorityRef()
	b.core, err = core.OpenDurable(b.realmConnections, b.path, b.config)
	if err != nil {
		return err
	}
	latest, err := b.core.Elements().ReadElement(b.actor, connectedRemote, nil)
	if err != nil || latest.Record == nil || latest.Record.Entity == nil ||
		latest.Record.Entity.LocalPose.Translation.X != -2 || b.core.AuthorityRef() != ref {
		return fmt.Errorf("demo latest persisted after reopen: %#v, %v", latest, err)
	}
	_, err = fmt.Fprintln(output, "connected: two durable Core stores; upward discovery, snapshot pagination+exact read across writes; latest state recovered")
	return err
}

func connectedTuple(parent values.ElementID, x, radius float64) corecontract.SpatialTuple {
	rotation, _ := corecontract.NewRotation(1, 0, 0, 0)
	pose, _ := corecontract.NewPose(corecontract.Vec3{X: x}, rotation)
	sphere, _ := corecontract.NewBoundedSphere(radius)
	return corecontract.SpatialTuple{Parent: parent, LocalPose: pose, Range: sphere}
}

func newConnectedDemoOwner(path string, id values.RealmID, config corecontract.Config) (*connectedDemoOwner, error) {
	actor := values.PrincipalRef{Realm: id, Principal: "operator"}
	policy, err := realminstance.New(realm.Seed{ID: id, LocalPrincipals: []values.PrincipalID{actor.Principal}, Bindings: []realm.PermissionBinding{
		binding(actor, realm.PermissionPutBinding, values.RealmScope(id)),
	}})
	if err != nil {
		return nil, err
	}
	realmConnections := &composition.LocalRealmConnections{}
	realmAddress := values.NextHop("local:realm-" + string(id))
	if err := realmConnections.BindEndpoint(realmAddress, policy); err != nil {
		return nil, err
	}
	initial := corecontract.RealmInitialization{Realm: policy.ID(), NextHop: realmAddress}
	spatial, err := core.CreateDurable(realmConnections, initial, path, config)
	if err != nil {
		return nil, err
	}
	owner := &connectedDemoOwner{core: spatial, realmConnections: realmConnections, actor: actor, path: path, config: config}
	// Explicit, finite owner-qualified grants keep the demo independent of any
	// global administrator, inherited mount permission or wildcard scope.
	for _, element := range []values.ElementID{connectedRoot, connectedMount, connectedLocal, connectedRemote} {
		for _, permission := range []values.PermissionID{
			corecontract.PermissionCreateRoot, corecontract.PermissionCreateAnchor, corecontract.PermissionCreateEntity, corecontract.PermissionReadElement,
			corecontract.PermissionUpdateElement, corecontract.PermissionPlaceElement, corecontract.PermissionQueryRegion, corecontract.PermissionListDirect,
			corecontract.PermissionMountControl, corecontract.PermissionMountAccept, corecontract.PermissionMountRead,
		} {
			grant := binding(actor, permission, values.CoreElementScope(spatial.AuthorityRef(), element))
			if _, err := policy.PutBinding(actor, grant, policy.BindingSetRevision(), realm.OperationID(fmt.Sprintf("grant:%s:%s", element, permission))); err != nil {
				_ = spatial.Close()
				return nil, err
			}
		}
	}
	return owner, nil
}

func connectDemoOwners(a, b *connectedDemoOwner) error {
	control := &composition.LocalMountConnections{}
	reads := &composition.LocalConnectedConnections{}
	for _, target := range []struct {
		address values.NextHop
		owner   *connectedDemoOwner
	}{{connectedAddressA, a}, {connectedAddressB, b}} {
		if err := control.BindEndpoint(target.address, target.owner.core.Mounts().MountPeer()); err != nil {
			return err
		}
		if err := reads.BindEndpoint(target.address, target.owner.core.Connected().ConnectedPeer()); err != nil {
			return err
		}
	}
	for _, owner := range []*connectedDemoOwner{a, b} {
		transport, err := control.TransportFor(owner.core)
		if err != nil {
			return err
		}
		if err := owner.core.Mounts().ConfigureMountTransport(transport); err != nil {
			return err
		}
		readTransport, err := reads.TransportFor(owner.core)
		if err != nil {
			return err
		}
		if err := owner.core.Connected().ConfigureConnectedTransport(readTransport); err != nil {
			return err
		}
	}
	return nil
}
