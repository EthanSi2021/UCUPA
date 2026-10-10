package query_test

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"reflect"
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
	"ucupa/ucupa-core/internal/query/connected"
	"ucupa/ucupa-core/internal/query/exchange"
)

type capturePolicy struct {
	mu sync.RWMutex
	fn func(values.PrincipalRef, ...values.Requirement) bool
}

func captureAuthorizationViewDigest(subject values.PrincipalRef, requirements []values.Requirement) values.AuthorizationView {
	return values.AuthorizationView(sha256.Sum256([]byte(fmt.Sprintf("view:%v:%v", subject, requirements))))
}

func captureAuthorizationDenialDigest(subject values.PrincipalRef, requirement values.Requirement) values.AuthorizationDenialView {
	return values.AuthorizationDenialView(sha256.Sum256([]byte(fmt.Sprintf("denial:%v:%v", subject, requirement))))
}

func (p *capturePolicy) set(fn func(values.PrincipalRef, ...values.Requirement) bool) {
	p.mu.Lock()
	p.fn = fn
	p.mu.Unlock()
}

func (p *capturePolicy) AuthorizePrincipal(actor values.PrincipalRef, requirements ...values.Requirement) values.AuthorizationDecision {
	p.mu.RLock()
	fn := p.fn
	p.mu.RUnlock()
	if fn == nil || fn(actor, requirements...) {
		return values.AuthorizationValid
	}
	return values.AuthorizationDenied
}

func (p *capturePolicy) AuthorizeView(ctx context.Context, caller values.CallerContext, requirements ...values.Requirement) (values.AuthorizationDecision, values.AuthorizationView) {
	if p.AuthorizePrincipal(caller.Subject, requirements...) != values.AuthorizationValid {
		return values.AuthorizationDenied, values.AuthorizationView{}
	}
	return values.AuthorizationValid, captureAuthorizationViewDigest(caller.Subject, requirements)
}

func (p *capturePolicy) ValidateView(ctx context.Context, caller values.CallerContext, view values.AuthorizationView, requirements ...values.Requirement) values.AuthorizationDecision {
	decision, current := p.AuthorizeView(ctx, caller, requirements...)
	if decision != values.AuthorizationValid {
		return decision
	}
	if current != view {
		return values.AuthorizationChanged
	}
	return values.AuthorizationValid
}

func (p *capturePolicy) CaptureDenialView(ctx context.Context, caller values.CallerContext, requirement values.Requirement) (values.AuthorizationDecision, values.AuthorizationDenialView) {
	if p.AuthorizePrincipal(caller.Subject, requirement) == values.AuthorizationDenied {
		return values.AuthorizationDenied, captureAuthorizationDenialDigest(caller.Subject, requirement)
	}
	return values.AuthorizationValid, values.AuthorizationDenialView{}
}

func (p *capturePolicy) ValidateDenialView(ctx context.Context, caller values.CallerContext, view values.AuthorizationDenialView, requirement values.Requirement) values.AuthorizationDecision {
	if view == (values.AuthorizationDenialView{}) {
		return values.AuthorizationDenied
	}
	if _, current := p.CaptureDenialView(ctx, caller, requirement); current != view {
		return values.AuthorizationChanged
	}
	return values.AuthorizationValid
}

var captureActor = values.PrincipalRef{Realm: "store-realm", Principal: "store-writer"}

func captureID(serial uint64) values.ElementID {
	return values.ElementID(fmt.Sprintf("20000000-0000-4000-8000-%012x", serial))
}

func captureTuple(t *testing.T, parent values.ElementID, x, radius float64) contract.SpatialTuple {
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

type captureOwner struct {
	policy    *capturePolicy
	owner     *commit.Owner
	elements  *element.Operations
	mounts    *mount.Operations
	queries   *querypkg.Operations
	connected *connected.Operations
}

func newCaptureOwner(t *testing.T, authority values.FactAuthorityRef, scope, run [32]byte, retention time.Duration, handles uint32) *captureOwner {
	t.Helper()
	policy := &capturePolicy{}
	owner, err := commit.New(authority, scope, contract.RealmInitialization{Realm: "fixture-realm", NextHop: "local:fixture"})
	if err != nil {
		t.Fatal(err)
	}
	queries := querypkg.New(owner, policy, run, querypkg.Settings{
		Retention: retention, Handles: handles, Bytes: 64 << 20, Concurrent: 64,
	})
	return &captureOwner{
		policy: policy, owner: owner, elements: element.New(owner, policy),
		mounts: mount.New(owner, policy, nil), queries: queries,
		connected: connected.New(owner, policy, queries),
	}
}

func (owner *captureOwner) close() {
	owner.owner.Lock()
	if owner.owner.BeginCloseLocked() {
		connected.CloseLocked(owner.connected)
	}
	owner.owner.Unlock()
	owner.queries.Close()
	_ = owner.owner.CloseStore()
}

type captureMountTransport struct {
	source values.FactAuthorityRef
	peers  map[values.NextHop]contract.MountPeerPort
}

func (transport captureMountTransport) peer(hop values.NextHop, expected values.FactAuthorityRef) (contract.MountPeerPort, error) {
	peer := transport.peers[hop]
	if peer == nil {
		return nil, contract.ErrUnavailable
	}
	if peer.AuthorityRef() != expected {
		return nil, contract.ErrOwnerMismatch
	}
	return peer, nil
}
func (transport captureMountTransport) Prepare(ctx context.Context, hop values.NextHop, expected values.FactAuthorityRef, request contract.MountPrepareRequest) (contract.MountPrepared, error) {
	peer, err := transport.peer(hop, expected)
	if err != nil {
		return contract.MountPrepared{}, err
	}
	return peer.Prepare(ctx, transport.source, request)
}
func (transport captureMountTransport) Finish(ctx context.Context, hop values.NextHop, expected values.FactAuthorityRef, request contract.MountFinishRequest) (contract.MountClosed, error) {
	peer, err := transport.peer(hop, expected)
	if err != nil {
		return contract.MountClosed{}, err
	}
	return peer.Finish(ctx, transport.source, request)
}
func (transport captureMountTransport) ReadDecision(ctx context.Context, hop values.NextHop, expected values.FactAuthorityRef, key contract.MountAttemptKey) (contract.MountDecisionEvidence, error) {
	peer, err := transport.peer(hop, expected)
	if err != nil {
		return contract.MountDecisionEvidence{}, err
	}
	return peer.ReadDecision(ctx, transport.source, key)
}

type captureConnectedTransport struct {
	offline bool
	source  values.FactAuthorityRef
	peers   map[values.NextHop]peercontract.PeerPort
	after   func(exchange.RequestData)
}

func (transport *captureConnectedTransport) ReadConnected(ctx context.Context, hop values.NextHop, expected values.FactAuthorityRef, request peercontract.Request) (peercontract.Reply, error) {
	if transport.offline {
		return peercontract.Reply{}, contract.ErrUnavailable
	}
	peer := transport.peers[hop]
	if peer == nil {
		return peercontract.Reply{}, contract.ErrUnavailable
	}
	if peer.AuthorityRef() != expected {
		return peercontract.Reply{}, contract.ErrOwnerMismatch
	}
	reply, err := peer.ReadConnected(ctx, transport.source, request)
	if transport.after != nil {
		transport.after(exchange.ReadRequest(request))
	}
	return reply, err
}

type captureFixture struct {
	parent, carrier     *captureOwner
	root, mount, branch values.ElementID
	callers             contract.LocalCallers
	transport           *captureConnectedTransport
}

// This fixture is shared only by query's external Connected resource tests.
// Capacities and retention are fixed before the actual owners begin serving.
func newCaptureFixture(t *testing.T, retained bool, parentHandles uint32) captureFixture {
	t.Helper()
	var retention time.Duration
	if retained {
		retention = time.Minute
	}
	parent := newCaptureOwner(t, values.FactAuthorityRef(captureID(9301)), [32]byte{1}, [32]byte{11}, retention, parentHandles)
	carrier := newCaptureOwner(t, values.FactAuthorityRef(captureID(9302)), [32]byte{2}, [32]byte{12}, retention, 128)
	t.Cleanup(parent.close)
	t.Cleanup(carrier.close)
	root, mountID, branch := captureID(9100), captureID(9101), captureID(9120)
	if _, err := parent.elements.CreateRoot(captureActor, root, "guard-root"); err != nil {
		t.Fatal(err)
	}
	mountResult, err := parent.elements.CreateAnchor(captureActor, mountID, captureTuple(t, root, 0, 10), "guard-mount")
	if err != nil {
		t.Fatal(err)
	}
	callers := contract.LocalCallers{parent.owner.AuthorityRef(): captureActor, carrier.owner.AuthorityRef(): captureActor}
	mountPeers := map[values.NextHop]contract.MountPeerPort{"local:guard-a": parent.mounts.MountPeer(), "local:guard-b": carrier.mounts.MountPeer()}
	for _, item := range []*captureOwner{parent, carrier} {
		if err := item.mounts.ConfigureMountTransport(captureMountTransport{source: item.owner.AuthorityRef(), peers: mountPeers}); err != nil {
			t.Fatal(err)
		}
	}
	settled, err := parent.mounts.ApplyMount(context.Background(), callers, "guard-establish", contract.MountCommand{
		Kind: contract.MountEstablish, MountAnchorUUID: mountID, ExpectedElementRevision: mountResult.Record.Anchor.Revision,
		CarrierAuthority: carrier.owner.AuthorityRef(), CarrierNextHop: "local:guard-b", ParentNextHop: "local:guard-a",
	})
	if err != nil || settled.Decision != contract.MountCommitted || !settled.Settled {
		t.Fatalf("fixture establish: %#v %v", settled, err)
	}
	if _, err := carrier.elements.CreateAnchor(captureActor, branch, captureTuple(t, mountID, 0, 5), "connected-branch"); err != nil {
		t.Fatal(err)
	}
	if _, err := carrier.elements.CreateEntity(captureActor, captureID(9121), captureTuple(t, branch, 0, 1), "connected-entity"); err != nil {
		t.Fatal(err)
	}
	peers := map[values.NextHop]peercontract.PeerPort{"local:guard-a": parent.connected.ConnectedPeer(), "local:guard-b": carrier.connected.ConnectedPeer()}
	parentTransport := &captureConnectedTransport{source: parent.owner.AuthorityRef(), peers: peers}
	carrierTransport := &captureConnectedTransport{source: carrier.owner.AuthorityRef(), peers: peers}
	if err := parent.connected.ConfigureConnectedTransport(parentTransport); err != nil {
		t.Fatal(err)
	}
	if err := carrier.connected.ConfigureConnectedTransport(carrierTransport); err != nil {
		t.Fatal(err)
	}
	return captureFixture{parent: parent, carrier: carrier, root: root, mount: mountID, branch: branch, callers: callers, transport: parentTransport}
}

func TestConnectedCaptureWitnessRejectsParentWriteAndReleasesQueryResources(t *testing.T) {
	f := newCaptureFixture(t, true, 128)
	parent, carrier, parentTransport := f.parent, f.carrier, f.transport
	root, mountID, branch, callers := f.root, f.mount, f.branch, f.callers
	captureWrite := false
	parentTransport.after = func(request exchange.RequestData) {
		if request.Action == exchange.Capture {
			parentTransport.after = nil
			if _, err := parent.elements.CreateEntity(captureActor, captureID(9122), captureTuple(t, root, 0, 1), "during-pin"); err != nil {
				t.Fatal(err)
			}
			captureWrite = true
		}
	}
	path := contract.Path{OriginAnchorID: root}
	query := contract.PathRegionQuery{StartPath: path, BasisPath: contract.Path{OriginAnchorID: root, Down: []values.ElementID{mountID, branch}}, Sphere: contract.QuerySphere{Radius: 100}, Selector: contract.QueryBoth}
	page, err := parent.connected.QueryRegionPath(context.Background(), callers, query, nil, 256, nil)
	if !captureWrite {
		t.Fatal("fixture did not execute the write after the actual peer Capture")
	}
	if !errors.Is(err, contract.ErrReadCutExpired) || len(page.Matches) != 0 || !reflect.DeepEqual(page, contract.ConnectedQueryPage{}) {
		t.Fatalf("mixed capture: %#v %v", page, err)
	}
	for _, item := range []*captureOwner{parent, carrier} {
		counts := querypkg.ReadResourceCountsForTest(item.queries)
		if counts.Views != 0 || counts.Handles != 0 || counts.Active != 0 || counts.Bytes != 0 {
			t.Fatalf("failed capture retained resources: %#v", counts)
		}
	}
}
