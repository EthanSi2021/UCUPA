package commit_test

import (
	"context"
	"path/filepath"
	"testing"

	"ucupa/shared/values"
	corecontract "ucupa/ucupa-core/contract"
	"ucupa/ucupa-core/internal/commit"
	"ucupa/ucupa-core/internal/element"
	"ucupa/ucupa-core/internal/mount"
)

// Store tests use only explicit addresses and expected identities. This is a
// trusted test transport, not a Ref resolver or a production implementation.
type mountStoreTransport struct {
	source       *mountStoreOwner
	endpoints    map[values.NextHop]*mountStoreOwner
	afterPrepare func()
	afterFinish  func()
}

func (transport *mountStoreTransport) target(address values.NextHop, expected values.FactAuthorityRef) (corecontract.MountPeerPort, error) {
	core := transport.endpoints[address]
	if core == nil {
		return nil, corecontract.ErrUnavailable
	}
	if core.owner.AuthorityRef() != expected {
		return nil, corecontract.ErrOwnerMismatch
	}
	return core.mounts.MountPeer(), nil
}

func (transport *mountStoreTransport) Prepare(ctx context.Context, address values.NextHop, expected values.FactAuthorityRef, request corecontract.MountPrepareRequest) (corecontract.MountPrepared, error) {
	peer, err := transport.target(address, expected)
	if err != nil {
		return corecontract.MountPrepared{}, err
	}
	result, err := peer.Prepare(ctx, transport.source.owner.AuthorityRef(), request)
	if transport.afterPrepare != nil {
		transport.afterPrepare()
	}
	return result, err
}

func (transport *mountStoreTransport) Finish(ctx context.Context, address values.NextHop, expected values.FactAuthorityRef, request corecontract.MountFinishRequest) (corecontract.MountClosed, error) {
	peer, err := transport.target(address, expected)
	if err != nil {
		return corecontract.MountClosed{}, err
	}
	result, err := peer.Finish(ctx, transport.source.owner.AuthorityRef(), request)
	if transport.afterFinish != nil {
		transport.afterFinish()
	}
	return result, err
}

func (transport *mountStoreTransport) ReadDecision(ctx context.Context, address values.NextHop, expected values.FactAuthorityRef, attempt corecontract.MountAttemptKey) (corecontract.MountDecisionEvidence, error) {
	peer, err := transport.target(address, expected)
	if err != nil {
		return corecontract.MountDecisionEvidence{}, err
	}
	return peer.ReadDecision(ctx, transport.source.owner.AuthorityRef(), attempt)
}

type mountStoreFixture struct {
	a, b                   *mountStoreOwner
	aPath, bPath           string
	root, anchor           values.ElementID
	command                corecontract.MountCommand
	callers                corecontract.LocalCallers
	aTransport, bTransport *mountStoreTransport
}

func newMountStoreFixture(t *testing.T, active bool) *mountStoreFixture {
	t.Helper()
	dir := t.TempDir()
	f := &mountStoreFixture{aPath: filepath.Join(dir, "a.db"), bPath: filepath.Join(dir, "b.db"), root: rangeStoreID(900), anchor: rangeStoreID(901)}
	f.a, f.b = createMountStore(t, f.aPath), createMountStore(t, f.bPath)
	t.Cleanup(func() { closeMountStore(t, f.a); closeMountStore(t, f.b) })
	root, err := f.a.elements.CreateRoot(rangeStoreActor, f.root, "mount-store-root")
	if err != nil || root.Status != corecontract.MutationCommitted {
		t.Fatalf("root: %#v, %v", root, err)
	}
	anchor, err := f.a.elements.CreateAnchor(rangeStoreActor, f.anchor, rangeStoreTuple(t, f.root, 0, 100), "mount-store-anchor")
	if err != nil || anchor.Record == nil {
		t.Fatalf("anchor: %#v, %v", anchor, err)
	}
	f.command = corecontract.MountCommand{Kind: corecontract.MountEstablish, MountAnchorUUID: f.anchor, ExpectedElementRevision: anchor.Record.Anchor.Revision, CarrierAuthority: f.b.owner.AuthorityRef(), CarrierNextHop: "local:b", ParentNextHop: "local:a"}
	f.callers = corecontract.LocalCallers{f.a.owner.AuthorityRef(): rangeStoreActor, f.b.owner.AuthorityRef(): rangeStoreActor}
	f.connect(t)
	if active {
		f.apply(t, "mount-store-establish", f.command)
	}
	return f
}

func (f *mountStoreFixture) connect(t *testing.T) {
	t.Helper()
	f.aTransport = &mountStoreTransport{source: f.a, endpoints: map[values.NextHop]*mountStoreOwner{"local:b": f.b, "local:b-new": f.b}}
	f.bTransport = &mountStoreTransport{source: f.b, endpoints: map[values.NextHop]*mountStoreOwner{"local:a": f.a, "local:a-new": f.a}}
	if err := f.a.mounts.ConfigureMountTransport(f.aTransport); err != nil {
		t.Fatal(err)
	}
	if err := f.b.mounts.ConfigureMountTransport(f.bTransport); err != nil {
		t.Fatal(err)
	}
}

func (f *mountStoreFixture) reopen(t *testing.T) {
	t.Helper()
	a, b := commit.CaptureCompleteStateForTest(f.a.owner), commit.CaptureCompleteStateForTest(f.b.owner)
	closeMountStore(t, f.a)
	closeMountStore(t, f.b)
	f.a, f.b = openMountStore(t, f.aPath), openMountStore(t, f.bPath)
	if !commit.MatchesCompleteStateForTest(f.a.owner, a) || !commit.MatchesCompleteStateForTest(f.b.owner, b) {
		t.Fatal("reopen changed complete owner state")
	}
	f.connect(t)
}

func (f *mountStoreFixture) apply(t *testing.T, id corecontract.OperationID, command corecontract.MountCommand) corecontract.MountControlResult {
	t.Helper()
	result, err := f.a.mounts.ApplyMount(context.Background(), f.callers, id, command)
	if err != nil || result.Decision != corecontract.MountCommitted || !result.Settled {
		t.Fatalf("ApplyMount %q: %#v, %v", id, result, err)
	}
	return result
}

func (f *mountStoreFixture) updateCommand(t *testing.T, radius float64) corecontract.MountCommand {
	t.Helper()
	record, err := f.a.elements.ReadElement(rangeStoreActor, f.anchor, nil)
	if err != nil || record.Record == nil {
		t.Fatal(err)
	}
	mount, err := f.a.mounts.ReadMount(rangeStoreActor, f.anchor)
	if err != nil {
		t.Fatal(err)
	}
	return corecontract.MountCommand{Kind: corecontract.MountUpdate, MountAnchorUUID: f.anchor, ExpectedElementRevision: record.Record.Anchor.Revision, ExpectedMountRevision: mount.MountRevision, Final: rangeStoreTuple(t, f.root, 0, radius)}
}

// Each fixture assembles real operations around one durable owner. No Core
// facade, independent state, fake commit or private production accessor is used.
type mountStoreOwner struct {
	owner    *commit.Owner
	elements *element.Operations
	mounts   *mount.Operations
}

func assembleMountStore(spatial *rangeStoreOwner) *mountStoreOwner {
	return &mountStoreOwner{owner: spatial.owner, elements: spatial.elements,
		mounts: mount.New(spatial.owner, writeAllowed{}, nil)}
}
func createMountStore(t *testing.T, path string) *mountStoreOwner {
	t.Helper()
	return assembleMountStore(createRangeStore(t, path))
}
func openMountStore(t *testing.T, path string) *mountStoreOwner {
	t.Helper()
	return assembleMountStore(openRangeStore(t, path))
}
func closeMountStore(t *testing.T, core *mountStoreOwner) {
	t.Helper()
	if err := core.owner.CloseStore(); err != nil {
		t.Fatal(err)
	}
}
func mountStoreFacts(core *mountStoreOwner) commit.Facts {
	core.owner.RLock()
	defer core.owner.RUnlock()
	return core.owner.FactsLocked()
}
func mountStoreOperation(t *testing.T, core *mountStoreOwner, id corecontract.OperationID) commit.MountOperation {
	t.Helper()
	core.owner.RLock()
	defer core.owner.RUnlock()
	operation, exists := core.owner.MountOperationLocked(id)
	if !exists {
		t.Fatalf("missing mount operation %q", id)
	}
	return operation
}
func mountStoreAvailable(core *mountStoreOwner) bool {
	core.owner.RLock()
	defer core.owner.RUnlock()
	return core.owner.AvailableLocked()
}
