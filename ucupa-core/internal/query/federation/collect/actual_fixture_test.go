package collect_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"ucupa/composition"
	"ucupa/shared/values"
	corecontract "ucupa/ucupa-core/contract"
	federationcontract "ucupa/ucupa-core/contract/federation"
	core "ucupa/ucupa-core/instance"
	federationcollect "ucupa/ucupa-core/internal/query/federation/collect"
	federationsource "ucupa/ucupa-core/internal/query/federation/source"
)

var actualActor = values.PrincipalRef{Realm: "store-realm", Principal: "store-writer"}

func actualID(serial uint64) values.ElementID {
	return values.ElementID(fmt.Sprintf("20000000-0000-4000-8000-%012x", serial))
}

func actualTuple(t *testing.T, parent values.ElementID, x, radius float64) corecontract.SpatialTuple {
	t.Helper()
	rotation, err := corecontract.NewRotation(1, 0, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	pose, err := corecontract.NewPose(corecontract.Vec3{X: x}, rotation)
	if err != nil {
		t.Fatal(err)
	}
	sphere, err := corecontract.NewBoundedSphere(radius)
	if err != nil {
		t.Fatal(err)
	}
	return corecontract.SpatialTuple{Parent: parent, LocalPose: pose, Range: sphere}
}

func actualConfig(retained bool) corecontract.Config {
	config := corecontract.Config{PublicAccessPolicy: &values.PublicAccessPolicy{ContextTTL: time.Hour, ObservationTTL: time.Hour}}
	if retained {
		duration := time.Hour
		config.SnapshotRetention = &duration
	}
	return config
}

// actualPolicy is fixed into a Core at construction through
// actualRealmConnections. Tests mutate only its authorization epoch, never the
// assembled operation graph.
type actualPolicy struct {
	mu             sync.Mutex
	epoch          byte
	authorizeViews int
}

func (policy *actualPolicy) AuthorizeView(context.Context, values.CallerContext, ...values.Requirement) (values.AuthorizationDecision, values.AuthorizationView) {
	policy.mu.Lock()
	defer policy.mu.Unlock()
	policy.authorizeViews++
	return values.AuthorizationValid, values.AuthorizationView{1, policy.epoch}
}

func (policy *actualPolicy) ValidateView(_ context.Context, _ values.CallerContext, view values.AuthorizationView, _ ...values.Requirement) values.AuthorizationDecision {
	policy.mu.Lock()
	defer policy.mu.Unlock()
	if view != (values.AuthorizationView{1, policy.epoch}) {
		return values.AuthorizationChanged
	}
	return values.AuthorizationValid
}

func (policy *actualPolicy) advance() {
	policy.mu.Lock()
	policy.epoch++
	policy.mu.Unlock()
}

func (policy *actualPolicy) authorizeViewCalls() int {
	policy.mu.Lock()
	defer policy.mu.Unlock()
	return policy.authorizeViews
}

// actualRealmConnections presents one fixture policy as the Core's trusted
// Realm contact port. The fixture never denies a principal; view decisions
// keep the policy's epoch semantics.
type actualRealmConnections struct {
	policy *actualPolicy
}

func (*actualRealmConnections) Check(context.Context, values.NextHop, values.RealmID) error {
	return nil
}

func (*actualRealmConnections) AuthorizePrincipal(values.NextHop, values.RealmID, values.PrincipalRef, ...values.Requirement) values.AuthorizationDecision {
	return values.AuthorizationValid
}

func (connections *actualRealmConnections) AuthorizeView(_ context.Context, _ values.NextHop, _ values.RealmID, caller values.CallerContext, requirements ...values.Requirement) (values.AuthorizationDecision, values.AuthorizationView) {
	return connections.policy.AuthorizeView(context.Background(), caller, requirements...)
}

func (connections *actualRealmConnections) ValidateView(_ context.Context, _ values.NextHop, _ values.RealmID, caller values.CallerContext, view values.AuthorizationView, requirements ...values.Requirement) values.AuthorizationDecision {
	return connections.policy.ValidateView(context.Background(), caller, view, requirements...)
}

func (*actualRealmConnections) CaptureDenialView(context.Context, values.NextHop, values.RealmID, values.CallerContext, values.Requirement) (values.AuthorizationDecision, values.AuthorizationDenialView) {
	return values.AuthorizationValid, values.AuthorizationDenialView{}
}

func (*actualRealmConnections) ValidateDenialView(_ context.Context, _ values.NextHop, _ values.RealmID, _ values.CallerContext, view values.AuthorizationDenialView, _ values.Requirement) values.AuthorizationDecision {
	if view == (values.AuthorizationDenialView{}) {
		return values.AuthorizationDenied
	}
	return values.AuthorizationChanged
}

type actualFixture struct {
	owners   []*core.Core
	policies []*actualPolicy
	callers  corecontract.LocalCallers
	root     values.ElementID
}

func actualAddress(index int) values.NextHop {
	return values.NextHop(fmt.Sprintf("local:nested-%d", index))
}

func newActualFixture(t *testing.T, retained bool, firstPolicy *actualPolicy) *actualFixture {
	t.Helper()
	if firstPolicy == nil {
		firstPolicy = &actualPolicy{}
	}
	fixture := &actualFixture{
		policies: []*actualPolicy{firstPolicy, {}, {}},
		callers:  make(corecontract.LocalCallers),
		root:     actualID(9700),
	}
	mounts := &composition.LocalMountConnections{}
	for index, policy := range fixture.policies {
		owner, err := core.New(&actualRealmConnections{policy: policy}, corecontract.RealmInitialization{Realm: "store-realm", NextHop: actualAddress(index)}, actualConfig(retained))
		if err != nil {
			t.Fatal(err)
		}
		fixture.owners = append(fixture.owners, owner)
		fixture.callers[owner.AuthorityRef()] = actualActor
		if err := mounts.BindEndpoint(actualAddress(index), owner.Mounts().MountPeer()); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() {
		for _, owner := range fixture.owners {
			if err := owner.Close(); err != nil {
				t.Error(err)
			}
		}
	})
	for _, owner := range fixture.owners {
		transport, err := mounts.TransportFor(owner)
		if err != nil {
			t.Fatal(err)
		}
		if err := owner.Mounts().ConfigureMountTransport(transport); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := fixture.owners[0].Elements().CreateRoot(actualActor, fixture.root, "nested-root"); err != nil {
		t.Fatal(err)
	}
	fixture.establish(t, 0, 1, actualID(9710), fixture.root, 20, "vector-ab")
	fixture.establish(t, 1, 2, actualID(9720), actualID(9710), 10, "vector-bc")
	return fixture
}

func (fixture *actualFixture) establish(t *testing.T, parent, carrier int, id, parentID values.ElementID, radius float64, operation corecontract.OperationID) {
	t.Helper()
	anchor, err := fixture.owners[parent].Elements().CreateAnchor(actualActor, id, actualTuple(t, parentID, 0, radius), corecontract.OperationID("anchor-"+string(id)))
	if err != nil || anchor.Record == nil {
		t.Fatalf("anchor: %+v %v", anchor, err)
	}
	result, err := fixture.owners[parent].Mounts().ApplyMount(context.Background(), fixture.callers, operation, corecontract.MountCommand{
		Kind:                    corecontract.MountEstablish,
		MountAnchorUUID:         id,
		ExpectedElementRevision: anchor.Record.Anchor.Revision,
		CarrierAuthority:        fixture.owners[carrier].AuthorityRef(),
		CarrierNextHop:          actualAddress(carrier),
		ParentNextHop:           actualAddress(parent),
	})
	if err != nil || result.Decision != corecontract.MountCommitted || !result.Settled {
		t.Fatalf("establish %s: %+v %v", operation, result, err)
	}
}

func (fixture *actualFixture) write(t *testing.T, owner int, serial uint64, parent values.ElementID, operation corecontract.OperationID) {
	t.Helper()
	if _, err := fixture.owners[owner].Elements().CreateEntity(actualActor, actualID(serial), actualTuple(t, parent, 0, 1), operation); err != nil {
		t.Fatal(err)
	}
}

type actualSourcePort interface {
	AuthorityRef() values.FactAuthorityRef
	OpenFederationReadPart(context.Context, values.CallerContext, values.FactAuthorityRef, federationcontract.FederationReadRequest) (federationcontract.FederationReadPart, error)
	ProjectFederationReadPart(context.Context, values.CallerContext, values.FactAuthorityRef, values.PublicToken, federationcontract.FederationProjectionRequest, *values.PublicToken, uint16) (federationcontract.FederationProjectionPage, error)
	CheckFederationReadPart(context.Context, values.CallerContext, values.FactAuthorityRef, values.PublicToken, bool) error
	ObserveFederationReadPart(context.Context, values.CallerContext, values.FactAuthorityRef, values.PublicToken, federationcontract.FederationObserveRequest) (federationcontract.FederationObservationPage, error)
	ReleaseFederationReadPart(context.Context, values.CallerContext, values.FactAuthorityRef, values.PublicToken) error
}

type operationSourceConnection struct {
	owner  actualSourcePort
	caller values.CallerContext
	source values.FactAuthorityRef
	part   federationcontract.FederationReadPart
}

func (part *operationSourceConnection) Part() federationcontract.FederationReadPart {
	return cloneActualPart(part.part)
}

func (part *operationSourceConnection) Project(ctx context.Context, request federationcontract.FederationProjectionRequest, continuation *values.PublicToken, limit uint16) (federationcontract.FederationProjectionPage, error) {
	return part.owner.ProjectFederationReadPart(ctx, part.caller, part.source, part.part.Basis, request, continuation, limit)
}

func (part *operationSourceConnection) Check(ctx context.Context, current bool) error {
	return part.owner.CheckFederationReadPart(ctx, part.caller, part.source, part.part.Basis, current)
}

func (part *operationSourceConnection) Observe(ctx context.Context, request federationcontract.FederationObserveRequest) (federationcontract.FederationObservationPage, error) {
	return part.owner.ObserveFederationReadPart(ctx, part.caller, part.source, part.part.Basis, request)
}

func (part *operationSourceConnection) Release(ctx context.Context) error {
	return part.owner.ReleaseFederationReadPart(ctx, part.caller, part.source, part.part.Basis)
}

type presentedSourceConnection struct {
	federationcontract.FederationReadConnection
	part federationcontract.FederationReadPart
}

func (part *presentedSourceConnection) Part() federationcontract.FederationReadPart {
	return cloneActualPart(part.part)
}

func cloneActualPart(part federationcontract.FederationReadPart) federationcontract.FederationReadPart {
	if part.Incoming != nil {
		incoming := *part.Incoming
		part.Incoming = &incoming
	}
	if part.Preparation != nil {
		preparation := *part.Preparation
		part.Preparation = &preparation
	}
	return part
}

type trackedActualSource struct {
	federationcontract.FederationReadConnection
	owner    values.FactAuthorityRef
	mu       sync.Mutex
	releases int
}

func (part *trackedActualSource) Release(ctx context.Context) error {
	part.mu.Lock()
	part.releases++
	part.mu.Unlock()
	return part.FederationReadConnection.Release(ctx)
}

type actualSourceTracker struct {
	mu    sync.Mutex
	parts []*trackedActualSource
}

func (tracker *actualSourceTracker) track(owner values.FactAuthorityRef, part federationcontract.FederationReadConnection) *trackedActualSource {
	tracked := &trackedActualSource{FederationReadConnection: part, owner: owner}
	tracker.mu.Lock()
	tracker.parts = append(tracker.parts, tracked)
	tracker.mu.Unlock()
	return tracked
}

func (tracker *actualSourceTracker) requireReleased(t *testing.T, owners ...values.FactAuthorityRef) {
	t.Helper()
	tracker.mu.Lock()
	parts := append([]*trackedActualSource(nil), tracker.parts...)
	tracker.mu.Unlock()
	if len(parts) != len(owners) {
		t.Fatalf("tracked sources=%d, want %d", len(parts), len(owners))
	}
	want := make(map[values.FactAuthorityRef]int, len(owners))
	for _, owner := range owners {
		want[owner]++
	}
	for index, part := range parts {
		if want[part.owner] == 0 {
			t.Fatalf("unexpected tracked source %s", part.owner)
		}
		want[part.owner]--
		part.mu.Lock()
		releases := part.releases
		part.mu.Unlock()
		if releases != 1 {
			t.Fatalf("source %d Release calls=%d, want 1", index, releases)
		}
		if err := part.Check(context.Background(), false); !errors.Is(err, values.PublicInvalidContext) {
			t.Fatalf("released source %d retained its Basis: %v", index, err)
		}
	}
	for owner, count := range want {
		if count != 0 {
			t.Fatalf("source %s tracked %d fewer times than required", owner, count)
		}
	}
}

func (tracker *actualSourceTracker) requireReleaseState(t *testing.T, owner values.FactAuthorityRef, releases int, check error) {
	t.Helper()
	tracker.mu.Lock()
	parts := append([]*trackedActualSource(nil), tracker.parts...)
	tracker.mu.Unlock()
	var found *trackedActualSource
	for _, part := range parts {
		if part.owner != owner {
			continue
		}
		if found != nil {
			t.Fatalf("source %s was tracked more than once", owner)
		}
		found = part
	}
	if found == nil {
		t.Fatalf("source %s was not tracked", owner)
	}
	found.mu.Lock()
	actualReleases := found.releases
	found.mu.Unlock()
	if actualReleases != releases {
		t.Fatalf("source %s Release calls=%d, want %d", owner, actualReleases, releases)
	}
	err := found.Check(context.Background(), false)
	if check == nil && err != nil || check != nil && !errors.Is(err, check) {
		t.Fatalf("source %s Check=%v, want %v", owner, err, check)
	}
}

type actualSourceEndpoint struct {
	owner actualSourcePort
}

type actualConnections struct {
	source    values.FactAuthorityRef
	owners    map[values.NextHop]actualSourceEndpoint
	tracker   *actualSourceTracker
	alter     func(*federationcontract.FederationReadPart)
	afterOpen func(int)
	mu        sync.Mutex
	opened    int
}

func (connections *actualConnections) OpenFederationRead(ctx context.Context, caller values.CallerContext, hop values.NextHop, target values.FactAuthorityRef, request federationcontract.FederationReadRequest) (federationcontract.FederationReadConnection, error) {
	endpoint, ok := connections.owners[hop]
	if !ok || endpoint.owner == nil || endpoint.owner.AuthorityRef() != target {
		return nil, values.PublicUnavailable
	}
	info, err := endpoint.owner.OpenFederationReadPart(ctx, caller, connections.source, request)
	if err != nil {
		return nil, err
	}
	var part federationcontract.FederationReadConnection = &operationSourceConnection{owner: endpoint.owner, caller: caller, source: connections.source, part: info}
	if connections.alter != nil {
		presented := part.Part()
		connections.alter(&presented)
		part = &presentedSourceConnection{FederationReadConnection: part, part: presented}
	}
	tracked := connections.tracker.track(target, part)
	connections.mu.Lock()
	connections.opened++
	opened := connections.opened
	afterOpen := connections.afterOpen
	connections.mu.Unlock()
	if afterOpen != nil {
		afterOpen(opened)
	}
	return tracked, nil
}

func (connections *actualConnections) openCount() int {
	connections.mu.Lock()
	defer connections.mu.Unlock()
	return connections.opened
}

type actualReadOptions struct {
	alter     func(*federationcontract.FederationReadPart)
	afterOpen func(int)
	wrapLocal func(federationcontract.FederationReadConnection) federationcontract.FederationReadConnection
}

type actualReadAttempt struct {
	read        *federationcollect.Read
	connections *actualConnections
	tracker     *actualSourceTracker
}

func (fixture *actualFixture) openQuery(t *testing.T, request federationcontract.FederationReadRequest, sourceLimit int, options actualReadOptions) (*actualReadAttempt, error) {
	t.Helper()
	ctx := context.Background()
	caller := values.CallerContext{Subject: actualActor}
	tracker := &actualSourceTracker{}
	local, err := federationsource.OpenLocal(fixture.owners[0].Federation(), ctx, caller, request)
	attempt := &actualReadAttempt{tracker: tracker}
	if err != nil {
		return attempt, err
	}
	var localPart federationcontract.FederationReadConnection = tracker.track(fixture.owners[0].AuthorityRef(), local)
	if options.wrapLocal != nil {
		localPart = options.wrapLocal(localPart)
	}
	connections := &actualConnections{
		source:    fixture.owners[0].AuthorityRef(),
		owners:    make(map[values.NextHop]actualSourceEndpoint),
		tracker:   tracker,
		alter:     options.alter,
		afterOpen: options.afterOpen,
	}
	for index, owner := range fixture.owners {
		connections.owners[actualAddress(index)] = actualSourceEndpoint{owner: owner.Federation()}
	}
	attempt.connections = connections
	read, err := federationcollect.New(ctx, federationcollect.Config{
		Caller:      caller,
		Source:      fixture.owners[0].AuthorityRef(),
		PartRequest: request,
		Local:       localPart,
		Connections: connections,
		SourceLimit: sourceLimit,
		Request: federationcollect.Request{
			Operation: "query",
			Query: &federationcollect.Query{
				Start:    federationcollect.Address{Origin: fixture.root},
				Basis:    federationcollect.Address{Origin: fixture.root},
				Sphere:   corecontract.QuerySphere{Radius: 100},
				Selector: corecontract.QueryBoth,
			},
		},
	})
	attempt.read = read
	if read != nil {
		t.Cleanup(func() { _ = read.Retire(context.Background()) })
	}
	return attempt, err
}

func collectActualPage(read *federationcollect.Read, prepare bool) (federationcollect.Page, error) {
	page, lease, err := read.Collect(context.Background(), "", 64, prepare)
	if lease != nil {
		lease.Release()
	}
	return page, err
}

func observeActualPage(read *federationcollect.Read, position uint64, limit int) (federationcollect.ObservationPage, error) {
	page, lease, err := read.Observe(context.Background(), "poll", position, limit)
	if lease != nil {
		lease.Release()
	}
	return page, err
}

func actualOwnerRefs(fixture *actualFixture, count int) []values.FactAuthorityRef {
	result := make([]values.FactAuthorityRef, count)
	for index := range result {
		result[index] = fixture.owners[index].AuthorityRef()
	}
	return result
}
