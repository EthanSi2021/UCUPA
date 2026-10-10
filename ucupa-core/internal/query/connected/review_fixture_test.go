package connected

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	peercontract "ucupa/ucupa-core/contract/connected"
	"ucupa/ucupa-core/internal/commit"
	"ucupa/ucupa-core/internal/element"
	"ucupa/ucupa-core/internal/mount"
	querypkg "ucupa/ucupa-core/internal/query"
	"ucupa/ucupa-core/internal/query/exchange"
)

var reviewActor = values.PrincipalRef{Realm: "store-realm", Principal: "store-writer"}

type reviewPolicy struct {
	mu sync.RWMutex
	fn func(values.PrincipalRef, ...values.Requirement) bool
}

func (policy *reviewPolicy) AuthorizePrincipal(actor values.PrincipalRef, requirements ...values.Requirement) values.AuthorizationDecision {
	policy.mu.RLock()
	fn := policy.fn
	policy.mu.RUnlock()
	if fn == nil || fn(actor, requirements...) {
		return values.AuthorizationValid
	}
	return values.AuthorizationDenied
}

func (policy *reviewPolicy) AuthorizeView(ctx context.Context, caller values.CallerContext, requirements ...values.Requirement) (values.AuthorizationDecision, values.AuthorizationView) {
	if policy.AuthorizePrincipal(caller.Subject, requirements...) != values.AuthorizationValid {
		return values.AuthorizationDenied, values.AuthorizationView{}
	}
	return values.AuthorizationValid, connectedAuthorizationViewDigest(caller.Subject, requirements)
}

func (policy *reviewPolicy) ValidateView(ctx context.Context, caller values.CallerContext, view values.AuthorizationView, requirements ...values.Requirement) values.AuthorizationDecision {
	decision, current := policy.AuthorizeView(ctx, caller, requirements...)
	if decision != values.AuthorizationValid {
		return decision
	}
	if current != view {
		return values.AuthorizationChanged
	}
	return values.AuthorizationValid
}

func (policy *reviewPolicy) CaptureDenialView(ctx context.Context, caller values.CallerContext, requirement values.Requirement) (values.AuthorizationDecision, values.AuthorizationDenialView) {
	if policy.AuthorizePrincipal(caller.Subject, requirement) == values.AuthorizationDenied {
		return values.AuthorizationDenied, connectedAuthorizationDenialDigest(caller.Subject, requirement)
	}
	return values.AuthorizationValid, values.AuthorizationDenialView{}
}

func (policy *reviewPolicy) ValidateDenialView(ctx context.Context, caller values.CallerContext, view values.AuthorizationDenialView, requirement values.Requirement) values.AuthorizationDecision {
	if view == (values.AuthorizationDenialView{}) {
		return values.AuthorizationDenied
	}
	if _, current := policy.CaptureDenialView(ctx, caller, requirement); current != view {
		return values.AuthorizationChanged
	}
	return values.AuthorizationValid
}

func (policy *reviewPolicy) set(fn func(values.PrincipalRef, ...values.Requirement) bool) {
	policy.mu.Lock()
	policy.fn = fn
	policy.mu.Unlock()
}

func reviewID(serial uint64) values.ElementID {
	return values.ElementID(fmt.Sprintf("20000000-0000-4000-8000-%012x", serial))
}

func reviewTuple(t *testing.T, parent values.ElementID, x, radius float64) contract.SpatialTuple {
	t.Helper()
	rotation, err := contract.NewRotation(1, 0, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	pose, err := contract.NewPose(contract.Vec3{X: x}, rotation)
	if err != nil {
		t.Fatal(err)
	}
	rangeValue, err := contract.NewBoundedSphere(radius)
	if err != nil {
		t.Fatal(err)
	}
	return contract.SpatialTuple{Parent: parent, LocalPose: pose, Range: rangeValue}
}

type reviewOwner struct {
	owner     *commit.Owner
	policy    *reviewPolicy
	elements  *element.Operations
	mounts    *mount.Operations
	queries   *querypkg.Operations
	connected *Operations
	closeOnce sync.Once
}

func newReviewOwner(t *testing.T, authority values.FactAuthorityRef, scope, run [32]byte, retained bool, handles uint32) *reviewOwner {
	t.Helper()
	owner, err := commit.New(authority, scope, contract.RealmInitialization{Realm: "fixture-realm", NextHop: "local:fixture"})
	if err != nil {
		t.Fatal(err)
	}
	return assembleReviewOwner(owner, run, retained, handles)
}

func assembleReviewOwner(owner *commit.Owner, run [32]byte, retained bool, handles uint32) *reviewOwner {
	policy := &reviewPolicy{}
	retention := time.Duration(0)
	if retained {
		retention = time.Minute
	}
	queries := querypkg.New(owner, policy, run, querypkg.Settings{
		Retention: retention, Handles: handles, Bytes: 64 << 20, Concurrent: 64,
	})
	return &reviewOwner{
		owner: owner, policy: policy, elements: element.New(owner, policy),
		mounts: mount.New(owner, policy, nil), queries: queries,
		connected: New(owner, policy, queries),
	}
}

func (owner *reviewOwner) close() {
	owner.closeOnce.Do(func() {
		owner.owner.Lock()
		if owner.owner.BeginCloseLocked() {
			CloseLocked(owner.connected)
		}
		owner.owner.Unlock()
		owner.queries.Close()
		_ = owner.owner.CloseStore()
	})
}

type reviewMountTransport struct {
	source values.FactAuthorityRef
	peers  map[values.NextHop]contract.MountPeerPort
}

func (transport reviewMountTransport) peer(hop values.NextHop, expected values.FactAuthorityRef) (contract.MountPeerPort, error) {
	peer := transport.peers[hop]
	if peer == nil {
		return nil, contract.ErrUnavailable
	}
	if peer.AuthorityRef() != expected {
		return nil, contract.ErrOwnerMismatch
	}
	return peer, nil
}

func (transport reviewMountTransport) Prepare(ctx context.Context, hop values.NextHop, expected values.FactAuthorityRef, request contract.MountPrepareRequest) (contract.MountPrepared, error) {
	peer, err := transport.peer(hop, expected)
	if err != nil {
		return contract.MountPrepared{}, err
	}
	return peer.Prepare(ctx, transport.source, request)
}

func (transport reviewMountTransport) Finish(ctx context.Context, hop values.NextHop, expected values.FactAuthorityRef, request contract.MountFinishRequest) (contract.MountClosed, error) {
	peer, err := transport.peer(hop, expected)
	if err != nil {
		return contract.MountClosed{}, err
	}
	return peer.Finish(ctx, transport.source, request)
}

func (transport reviewMountTransport) ReadDecision(ctx context.Context, hop values.NextHop, expected values.FactAuthorityRef, key contract.MountAttemptKey) (contract.MountDecisionEvidence, error) {
	peer, err := transport.peer(hop, expected)
	if err != nil {
		return contract.MountDecisionEvidence{}, err
	}
	return peer.ReadDecision(ctx, transport.source, key)
}

type reviewTransport struct {
	source  values.FactAuthorityRef
	peers   map[values.NextHop]peercontract.PeerPort
	mu      sync.RWMutex
	offline bool
	armed   bool
	entered chan struct{}
	release chan struct{}
	after   func(exchange.RequestData, exchange.ReplyData, error)
}

func (transport *reviewTransport) setOffline(value bool) {
	transport.mu.Lock()
	transport.offline = value
	transport.mu.Unlock()
}

func (transport *reviewTransport) armCheck(entered, release chan struct{}) {
	transport.mu.Lock()
	transport.armed, transport.entered, transport.release = true, entered, release
	transport.mu.Unlock()
}

func (transport *reviewTransport) setAfter(after func(exchange.RequestData, exchange.ReplyData, error)) {
	transport.mu.Lock()
	transport.after = after
	transport.mu.Unlock()
}

func (transport *reviewTransport) ReadConnected(ctx context.Context, hop values.NextHop, expected values.FactAuthorityRef, request peercontract.Request) (peercontract.Reply, error) {
	data := exchange.ReadRequest(request)
	transport.mu.Lock()
	if transport.offline {
		transport.mu.Unlock()
		return peercontract.Reply{}, contract.ErrUnavailable
	}
	var entered, release chan struct{}
	if data.Action == exchange.Check && transport.armed {
		transport.armed = false
		entered, release = transport.entered, transport.release
	}
	transport.mu.Unlock()
	if entered != nil {
		close(entered)
		select {
		case <-release:
		case <-ctx.Done():
			return peercontract.Reply{}, ctx.Err()
		}
	}
	peer := transport.peers[hop]
	if peer == nil {
		return peercontract.Reply{}, contract.ErrUnavailable
	}
	if peer.AuthorityRef() != expected {
		return peercontract.Reply{}, contract.ErrOwnerMismatch
	}
	reply, err := peer.ReadConnected(ctx, transport.source, request)
	transport.mu.RLock()
	after := transport.after
	transport.mu.RUnlock()
	if after != nil {
		after(data, exchange.ReadReply(reply), err)
	}
	return reply, err
}

type reviewFixture struct {
	parent, carrier  *reviewOwner
	root, mount      values.ElementID
	branch           values.ElementID
	callers          contract.LocalCallers
	parentTransport  *reviewTransport
	carrierTransport *reviewTransport
}

func newReviewFixture(t *testing.T, retained bool, parentHandles uint32) reviewFixture {
	return newReviewFixtureWithParentStore(t, retained, parentHandles, "")
}

func newReviewFixtureWithParentStore(t *testing.T, retained bool, parentHandles uint32, path string) reviewFixture {
	t.Helper()
	parent := newReviewOwner(t, values.FactAuthorityRef(reviewID(9301)), [32]byte{1}, [32]byte{11}, retained, parentHandles)
	if path != "" {
		parent.close()
		owner, err := commit.CreateDurable(path, values.FactAuthorityRef(reviewID(9301)), [32]byte{1}, contract.RealmInitialization{Realm: "fixture-realm", NextHop: "local:fixture"}, nil)
		if err != nil {
			t.Fatal(err)
		}
		parent = assembleReviewOwner(owner, [32]byte{11}, retained, parentHandles)
	}
	carrier := newReviewOwner(t, values.FactAuthorityRef(reviewID(9302)), [32]byte{2}, [32]byte{12}, retained, 128)
	t.Cleanup(parent.close)
	t.Cleanup(carrier.close)
	f := reviewFixture{parent: parent, carrier: carrier, root: reviewID(9100), mount: reviewID(9101), branch: reviewID(9120)}
	if _, err := parent.elements.CreateRoot(reviewActor, f.root, "guard-root"); err != nil {
		t.Fatal(err)
	}
	mountResult, err := parent.elements.CreateAnchor(reviewActor, f.mount, reviewTuple(t, f.root, 0, 10), "guard-mount")
	if err != nil {
		t.Fatal(err)
	}
	f.callers = contract.LocalCallers{parent.owner.AuthorityRef(): reviewActor, carrier.owner.AuthorityRef(): reviewActor}
	mountPeers := map[values.NextHop]contract.MountPeerPort{"local:guard-a": parent.mounts.MountPeer(), "local:guard-b": carrier.mounts.MountPeer()}
	for _, item := range []*reviewOwner{parent, carrier} {
		if err := item.mounts.ConfigureMountTransport(reviewMountTransport{source: item.owner.AuthorityRef(), peers: mountPeers}); err != nil {
			t.Fatal(err)
		}
	}
	settled, err := parent.mounts.ApplyMount(context.Background(), f.callers, "guard-establish", contract.MountCommand{
		Kind: contract.MountEstablish, MountAnchorUUID: f.mount, ExpectedElementRevision: mountResult.Record.Anchor.Revision,
		CarrierAuthority: carrier.owner.AuthorityRef(), CarrierNextHop: "local:guard-b", ParentNextHop: "local:guard-a",
	})
	if err != nil || settled.Decision != contract.MountCommitted || !settled.Settled {
		t.Fatalf("fixture establish: %#v %v", settled, err)
	}
	if _, err := carrier.elements.CreateAnchor(reviewActor, f.branch, reviewTuple(t, f.mount, 0, 5), "connected-branch"); err != nil {
		t.Fatal(err)
	}
	if _, err := carrier.elements.CreateEntity(reviewActor, reviewID(9121), reviewTuple(t, f.branch, 0, 1), "connected-entity"); err != nil {
		t.Fatal(err)
	}
	peers := map[values.NextHop]peercontract.PeerPort{"local:guard-a": parent.connected.ConnectedPeer(), "local:guard-b": carrier.connected.ConnectedPeer()}
	f.parentTransport = &reviewTransport{source: parent.owner.AuthorityRef(), peers: peers}
	f.carrierTransport = &reviewTransport{source: carrier.owner.AuthorityRef(), peers: peers}
	if err := parent.connected.ConfigureConnectedTransport(f.parentTransport); err != nil {
		t.Fatal(err)
	}
	if err := carrier.connected.ConfigureConnectedTransport(f.carrierTransport); err != nil {
		t.Fatal(err)
	}
	return f
}

func reviewRootQuery(root values.ElementID) contract.PathRegionQuery {
	path := contract.Path{OriginAnchorID: root}
	return contract.PathRegionQuery{StartPath: path, BasisPath: path, Sphere: contract.QuerySphere{Radius: 100}, Selector: contract.QueryBoth}
}

func reviewRecordID(record contract.ElementRecord) values.ElementID {
	if record.Anchor != nil {
		return record.Anchor.ID
	}
	if record.Entity != nil {
		return record.Entity.ID
	}
	return ""
}
