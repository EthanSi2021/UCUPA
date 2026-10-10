package connected_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"ucupa/shared/values"
	corecontract "ucupa/ucupa-core/contract"
	peercontract "ucupa/ucupa-core/contract/connected"
	core "ucupa/ucupa-core/instance"
)

var pairActor = values.PrincipalRef{Realm: "store-realm", Principal: "store-writer"}

type pairFixture struct {
	parent, carrier *localCore
	root, mount     values.ElementID
}

func pairID(serial uint64) values.ElementID {
	return values.ElementID(fmt.Sprintf("20000000-0000-4000-8000-%012x", serial))
}

func pairTuple(t *testing.T, parent values.ElementID, x, radius float64) corecontract.SpatialTuple {
	return localTuple(t, parent, x, 0, 0, radius)
}

// Only this test's explicit local addresses are bound. It forwards opaque
// requests to real peer ports and never inspects or manufactures their payload.
type pairReadTransport struct {
	source values.FactAuthorityRef
	peers  map[values.NextHop]peercontract.PeerPort
	after  func()
}

func (tr *pairReadTransport) ReadConnected(ctx context.Context, hop values.NextHop, expected values.FactAuthorityRef, request peercontract.Request) (peercontract.Reply, error) {
	peer := tr.peers[hop]
	if peer == nil {
		return peercontract.Reply{}, corecontract.ErrUnavailable
	}
	if peer.AuthorityRef() != expected {
		return peercontract.Reply{}, corecontract.ErrOwnerMismatch
	}
	reply, err := peer.ReadConnected(ctx, tr.source, request)
	if tr.after != nil {
		tr.after()
	}
	return reply, err
}

type pairMountTransport struct {
	source values.FactAuthorityRef
	peers  map[values.NextHop]corecontract.MountPeerPort
}

func (tr pairMountTransport) peer(hop values.NextHop, expected values.FactAuthorityRef) (corecontract.MountPeerPort, error) {
	peer := tr.peers[hop]
	if peer == nil {
		return nil, corecontract.ErrUnavailable
	}
	if peer.AuthorityRef() != expected {
		return nil, corecontract.ErrOwnerMismatch
	}
	return peer, nil
}
func (tr pairMountTransport) Prepare(ctx context.Context, hop values.NextHop, expected values.FactAuthorityRef, request corecontract.MountPrepareRequest) (corecontract.MountPrepared, error) {
	peer, err := tr.peer(hop, expected)
	if err != nil {
		return corecontract.MountPrepared{}, err
	}
	return peer.Prepare(ctx, tr.source, request)
}
func (tr pairMountTransport) Finish(ctx context.Context, hop values.NextHop, expected values.FactAuthorityRef, request corecontract.MountFinishRequest) (corecontract.MountClosed, error) {
	peer, err := tr.peer(hop, expected)
	if err != nil {
		return corecontract.MountClosed{}, err
	}
	return peer.Finish(ctx, tr.source, request)
}
func (tr pairMountTransport) ReadDecision(ctx context.Context, hop values.NextHop, expected values.FactAuthorityRef, key corecontract.MountAttemptKey) (corecontract.MountDecisionEvidence, error) {
	peer, err := tr.peer(hop, expected)
	if err != nil {
		return corecontract.MountDecisionEvidence{}, err
	}
	return peer.ReadDecision(ctx, tr.source, key)
}

func newPairFixture(t *testing.T, retained bool) (pairFixture, values.ElementID, corecontract.LocalCallers, *pairReadTransport, *pairReadTransport) {
	t.Helper()
	config := corecontract.Config{}
	if retained {
		ttl := time.Minute
		config.SnapshotRetention = &ttl
	}
	create := func() *localCore {
		policy := &switchPolicy{}
		owner, err := core.New(localRealmContact{policy: policy}, connectedContact, config)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = owner.Close() })
		return &localCore{Core: owner, policy: policy}
	}
	f := pairFixture{parent: create(), carrier: create(), root: pairID(9100), mount: pairID(9101)}
	if _, err := f.parent.Elements().CreateRoot(pairActor, f.root, "guard-root"); err != nil {
		t.Fatal(err)
	}
	mount, err := f.parent.Elements().CreateAnchor(pairActor, f.mount, pairTuple(t, f.root, 0, 10), "guard-mount")
	if err != nil {
		t.Fatal(err)
	}
	callers := corecontract.LocalCallers{f.parent.AuthorityRef(): pairActor, f.carrier.AuthorityRef(): pairActor}
	mountPeers := map[values.NextHop]corecontract.MountPeerPort{
		"local:guard-a": f.parent.Mounts().MountPeer(), "local:guard-b": f.carrier.Mounts().MountPeer(),
	}
	for _, owner := range []*localCore{f.parent, f.carrier} {
		if err := owner.Mounts().ConfigureMountTransport(pairMountTransport{source: owner.AuthorityRef(), peers: mountPeers}); err != nil {
			t.Fatal(err)
		}
	}
	// Preserve the original fixture's committed, settled mount and caller/tuple
	// inputs. The public workflow replaces direct writes of private event state.
	result, err := f.parent.Mounts().ApplyMount(context.Background(), callers, "guard-establish", corecontract.MountCommand{
		Kind: corecontract.MountEstablish, MountAnchorUUID: f.mount, ExpectedElementRevision: mount.Record.Anchor.Revision,
		CarrierAuthority: f.carrier.AuthorityRef(), CarrierNextHop: "local:guard-b", ParentNextHop: "local:guard-a",
	})
	if err != nil || result.Decision != corecontract.MountCommitted || !result.Settled {
		t.Fatalf("fixture establish: %#v %v", result, err)
	}
	branch := pairID(9120)
	if _, err := f.carrier.Elements().CreateAnchor(pairActor, branch, pairTuple(t, f.mount, 0, 5), "connected-branch"); err != nil {
		t.Fatal(err)
	}
	if _, err := f.carrier.Elements().CreateEntity(pairActor, pairID(9121), pairTuple(t, branch, 0, 1), "connected-entity"); err != nil {
		t.Fatal(err)
	}
	peers := map[values.NextHop]peercontract.PeerPort{
		"local:guard-a": f.parent.Connected().ConnectedPeer(), "local:guard-b": f.carrier.Connected().ConnectedPeer(),
	}
	a := &pairReadTransport{source: f.parent.AuthorityRef(), peers: peers}
	b := &pairReadTransport{source: f.carrier.AuthorityRef(), peers: peers}
	if err := f.parent.Connected().ConfigureConnectedTransport(a); err != nil {
		t.Fatal(err)
	}
	if err := f.carrier.Connected().ConfigureConnectedTransport(b); err != nil {
		t.Fatal(err)
	}
	return f, branch, callers, a, b
}

func pairRootQuery(root values.ElementID) corecontract.PathRegionQuery {
	path := corecontract.Path{OriginAnchorID: root}
	return corecontract.PathRegionQuery{StartPath: path, BasisPath: path, Sphere: corecontract.QuerySphere{Radius: 100}, Selector: corecontract.QueryBoth}
}

func pairRecordID(record corecontract.ElementRecord) values.ElementID {
	if record.Anchor != nil {
		return record.Anchor.ID
	}
	if record.Entity != nil {
		return record.Entity.ID
	}
	return ""
}
func pairClonePath(path corecontract.Path) corecontract.Path {
	path.Down = append([]values.ElementID(nil), path.Down...)
	return path
}
