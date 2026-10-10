package commit_test

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"

	"ucupa/shared/values"
	corecontract "ucupa/ucupa-core/contract"
	"ucupa/ucupa-core/internal/commit"
)

// Five real owners share only an explicit-address test transport. The fixture
// retains the original pending-control interleavings and complete reopen proof;
// it is not another Core facade or a public network query implementation.
type nestedMountFixture struct {
	owners     []*mountStoreOwner
	paths      []string
	transports []*mountStoreTransport
	callers    corecontract.LocalCallers
	root       values.ElementID
}

func newNestedMountFixture(t *testing.T, durable bool) *nestedMountFixture {
	t.Helper()
	f := &nestedMountFixture{root: rangeStoreID(9700), callers: make(corecontract.LocalCallers)}
	dir := t.TempDir()
	for i := 0; i < 5; i++ {
		path := filepath.Join(dir, fmt.Sprintf("%d.db", i))
		var node *mountStoreOwner
		if durable {
			node = createMountStore(t, path)
		} else {
			node = newMountControlOwner(t, &mountControlPolicy{})
		}
		f.owners, f.paths = append(f.owners, node), append(f.paths, path)
		f.callers[node.owner.AuthorityRef()] = rangeStoreActor
	}
	t.Cleanup(func() {
		for _, node := range f.owners {
			closeNestedMount(t, node)
		}
	})
	f.connect(t)
	if _, err := f.owners[0].elements.CreateRoot(rangeStoreActor, f.root, "nested-root"); err != nil {
		t.Fatal(err)
	}
	return f
}

func nestedMountAddress(index int) values.NextHop {
	return values.NextHop(fmt.Sprintf("local:nested-%d", index))
}

func (f *nestedMountFixture) connect(t *testing.T) {
	t.Helper()
	endpoints := make(map[values.NextHop]*mountStoreOwner)
	for i, node := range f.owners {
		endpoints[nestedMountAddress(i)] = node
	}
	f.transports = nil
	for _, node := range f.owners {
		transport := &mountStoreTransport{source: node, endpoints: endpoints}
		if err := node.mounts.ConfigureMountTransport(transport); err != nil {
			t.Fatal(err)
		}
		f.transports = append(f.transports, transport)
	}
}

func (f *nestedMountFixture) anchor(t *testing.T, owner int, id, parent values.ElementID, radius float64) corecontract.MountCommand {
	t.Helper()
	result, err := f.owners[owner].elements.CreateAnchor(rangeStoreActor, id, rangeStoreTuple(t, parent, 0, radius), corecontract.OperationID("anchor-"+string(id)))
	if err != nil || result.Record == nil {
		t.Fatalf("anchor: %#v %v", result, err)
	}
	return corecontract.MountCommand{Kind: corecontract.MountEstablish, MountAnchorUUID: id, ExpectedElementRevision: result.Record.Anchor.Revision}
}

func (f *nestedMountFixture) establish(t *testing.T, parent, carrier int, command corecontract.MountCommand, id corecontract.OperationID) corecontract.MountControlResult {
	t.Helper()
	command.CarrierAuthority = f.owners[carrier].owner.AuthorityRef()
	command.CarrierNextHop, command.ParentNextHop = nestedMountAddress(carrier), nestedMountAddress(parent)
	result, err := f.owners[parent].mounts.ApplyMount(context.Background(), f.callers, id, command)
	if err != nil || result.Decision != corecontract.MountCommitted || !result.Settled {
		t.Fatalf("establish %s: %#v %v", id, result, err)
	}
	return result
}

func (f *nestedMountFixture) update(t *testing.T, owner int, id values.ElementID, radius float64) corecontract.MountCommand {
	t.Helper()
	node := f.owners[owner]
	read, err := node.mounts.ReadMount(rangeStoreActor, id)
	if err != nil {
		t.Fatal(err)
	}
	anchor, _ := mountStoreFacts(node).Elements.Anchor(id)
	return corecontract.MountCommand{Kind: corecontract.MountUpdate, MountAnchorUUID: id, ExpectedElementRevision: anchor.Revision,
		ExpectedMountRevision: read.MountRevision, Final: rangeStoreTuple(t, anchor.Parent, 0, radius)}
}

func (f *nestedMountFixture) validate(t *testing.T) {
	t.Helper()
	for i, node := range f.owners {
		if err := commit.ValidateCompleteStateForTest(node.owner); err != nil {
			t.Fatalf("owner %d current/history invalid: %v", i, err)
		}
	}
}

func closeNestedMount(t *testing.T, node *mountStoreOwner) {
	t.Helper()
	node.owner.Lock()
	node.owner.BeginCloseLocked()
	node.owner.Unlock()
	closeMountStore(t, node)
}

func (f *nestedMountFixture) reopen(t *testing.T) {
	t.Helper()
	for i, node := range f.owners {
		before := commit.CaptureCompleteStateForTest(node.owner)
		closeNestedMount(t, node)
		reopened := openMountStore(t, f.paths[i])
		f.owners[i] = reopened
		if !commit.MatchesCompleteStateForTest(reopened.owner, before) {
			t.Fatalf("owner %d changed on reopen", i)
		}
	}
	f.connect(t)
	f.validate(t)
}

// These are the same specific commit boundaries that the original tests used
// to stop before peer I/O or inject an already-denied ancestor decision. They
// retain the availability check and owner lock; no arbitrary callback is added.
func reserveNestedMount(node *mountStoreOwner, callers corecontract.LocalCallers, id corecontract.OperationID, command corecontract.MountCommand) error {
	node.owner.Lock()
	defer node.owner.Unlock()
	if node.owner.ClosedLocked() || !node.owner.AvailableLocked() {
		return corecontract.ErrStore
	}
	return node.owner.ApplyMountIntentLocked(callers, id, commit.MountBinding{Actor: rangeStoreActor, Command: command})
}

func abortNestedAncestor(node *mountStoreOwner) error {
	node.owner.Lock()
	defer node.owner.Unlock()
	if node.owner.ClosedLocked() || !node.owner.AvailableLocked() {
		return corecontract.ErrStore
	}
	return node.owner.ApplyMountEventLocked(commit.MountEvent{Kind: commit.MountEventDecide, Operation: "ancestor", Authorized: false})
}
