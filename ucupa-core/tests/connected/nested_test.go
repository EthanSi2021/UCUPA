package connected_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"ucupa/shared/values"
	corecontract "ucupa/ucupa-core/contract"
	peercontract "ucupa/ucupa-core/contract/connected"
	core "ucupa/ucupa-core/instance"
)

func TestConnectedPairNeverClaimsCompleteForNestedOrSiblingSources(t *testing.T) {
	f := struct {
		owners []*core.Core
		root   values.ElementID
	}{root: pairID(9700)}
	callers := make(corecontract.LocalCallers)
	mountPeers := make(map[values.NextHop]corecontract.MountPeerPort)
	address := func(index int) values.NextHop { return values.NextHop(fmt.Sprintf("local:nested-%d", index)) }
	for i := 0; i < 5; i++ {
		owner, err := core.New(localRealmContact{policy: &switchPolicy{}}, connectedContact, corecontract.Config{})
		if err != nil {
			t.Fatal(err)
		}
		f.owners = append(f.owners, owner)
		callers[owner.AuthorityRef()] = pairActor
		mountPeers[address(i)] = owner.Mounts().MountPeer()
		t.Cleanup(func() { _ = owner.Close() })
	}
	for _, owner := range f.owners {
		if err := owner.Mounts().ConfigureMountTransport(pairMountTransport{source: owner.AuthorityRef(), peers: mountPeers}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := f.owners[0].Elements().CreateRoot(pairActor, f.root, "nested-root"); err != nil {
		t.Fatal(err)
	}
	establish := func(parent, carrier int, id, parentID values.ElementID, radius float64, op corecontract.OperationID) {
		t.Helper()
		anchor, err := f.owners[parent].Elements().CreateAnchor(pairActor, id, pairTuple(t, parentID, 0, radius), corecontract.OperationID("anchor-"+string(id)))
		if err != nil || anchor.Record == nil {
			t.Fatalf("anchor: %#v %v", anchor, err)
		}
		command := corecontract.MountCommand{
			Kind: corecontract.MountEstablish, MountAnchorUUID: id, ExpectedElementRevision: anchor.Record.Anchor.Revision,
			CarrierAuthority: f.owners[carrier].AuthorityRef(), CarrierNextHop: address(carrier), ParentNextHop: address(parent),
		}
		result, err := f.owners[parent].Mounts().ApplyMount(context.Background(), callers, op, command)
		if err != nil || result.Decision != corecontract.MountCommitted || !result.Settled {
			t.Fatalf("establish %s: %#v %v", op, result, err)
		}
	}
	m, n, sibling := pairID(9791), pairID(9792), pairID(9793)
	establish(0, 1, m, f.root, 10, "a-b")
	establish(1, 2, n, m, 5, "b-c")
	peers := make(map[values.NextHop]peercontract.PeerPort)
	for i, owner := range f.owners {
		peers[address(i)] = owner.Connected().ConnectedPeer()
	}
	for _, owner := range f.owners {
		if err := owner.Connected().ConfigureConnectedTransport(&pairReadTransport{source: owner.AuthorityRef(), peers: peers}); err != nil {
			t.Fatal(err)
		}
	}
	query := func(owner int, origin values.ElementID) (corecontract.ConnectedQueryPage, error) {
		path := corecontract.Path{OriginAnchorID: origin}
		callers := corecontract.LocalCallers{f.owners[0].AuthorityRef(): pairActor, f.owners[1].AuthorityRef(): pairActor}
		return f.owners[owner].Connected().QueryRegionPath(context.Background(), callers,
			corecontract.PathRegionQuery{StartPath: path, BasisPath: path, Sphere: corecontract.QuerySphere{Radius: 100}, Selector: corecontract.QueryBoth}, nil, 256, nil)
	}
	page, err := query(0, f.root)
	if err != nil || page.Status != corecontract.QueryPartial || page.Gap&corecontract.QueryUnresolvedCoverage == 0 {
		t.Fatalf("remote nested source lost its coverage gap: %#v %v", page, err)
	}
	if page, err := query(1, n); !errors.Is(err, corecontract.ErrUnavailable) || len(page.Matches) != 0 {
		t.Fatalf("nested pair entry reported a complete graph: %#v %v", page, err)
	}
	establish(0, 3, sibling, f.root, 10, "a-d")
	if page, err := query(0, f.root); !errors.Is(err, corecontract.ErrUnavailable) || len(page.Matches) != 0 {
		t.Fatalf("sibling pair entry selected an arbitrary source: %#v %v", page, err)
	}
}
