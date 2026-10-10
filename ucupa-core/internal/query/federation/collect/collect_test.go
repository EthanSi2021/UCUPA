package collect

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"testing"

	"ucupa/shared/values"
	corecontract "ucupa/ucupa-core/contract"
	federationcontract "ucupa/ucupa-core/contract/federation"
	"ucupa/ucupa-core/internal/geometry"
	coretoken "ucupa/ucupa-core/internal/token"
)

const (
	testOwner   values.FactAuthorityRef = "00000000-0000-4000-8000-000000000001"
	testOwner2  values.FactAuthorityRef = "00000000-0000-4000-8000-000000000002"
	testOwner3  values.FactAuthorityRef = "00000000-0000-4000-8000-000000000003"
	testRoot    values.ElementID        = "00000000-0000-4000-8000-000000000011"
	testElement values.ElementID        = "00000000-0000-4000-8000-000000000012"
	testMount1  values.ElementID        = "00000000-0000-4000-8000-000000000013"
	testMount2  values.ElementID        = "00000000-0000-4000-8000-000000000014"
)

var testCaller = values.CallerContext{Subject: values.PrincipalRef{Realm: "realm", Principal: "caller"}}

type unavailableConnections struct{}

func (unavailableConnections) OpenFederationRead(context.Context, values.CallerContext, values.NextHop, values.FactAuthorityRef, federationcontract.FederationReadRequest) (federationcontract.FederationReadConnection, error) {
	return nil, values.PublicUnavailable
}

type fixedConnections struct {
	part  federationcontract.FederationReadConnection
	opens int
}

func (connections *fixedConnections) OpenFederationRead(context.Context, values.CallerContext, values.NextHop, values.FactAuthorityRef, federationcontract.FederationReadRequest) (federationcontract.FederationReadConnection, error) {
	connections.opens++
	return connections.part, nil
}

type testConnection struct {
	mu       sync.Mutex
	part     federationcontract.FederationReadPart
	record   federationcontract.FederationRecord
	initial  values.PublicToken
	next     values.PublicToken
	changeID values.PublicToken
	basis    values.PublicToken
	checks   int
	releases int
}

func newTestConnection(t *testing.T, owner values.FactAuthorityRef, seed byte, prepare bool) *testConnection {
	t.Helper()
	receipt := token(t, owner, "read-receipt", seed)
	var preparation *values.PublicToken
	if prepare {
		value := token(t, owner, "observation-preparation", seed+1)
		preparation = &value
	}
	sphere, err := corecontract.NewBoundedSphere(1)
	if err != nil {
		t.Fatal(err)
	}
	return &testConnection{
		part: federationcontract.FederationReadPart{
			Subject:            testCaller.Subject,
			CutStamp:           [32]byte{seed, 1},
			AuthorizationStamp: [32]byte{seed, 2},
			Basis:              receipt,
			Boundary:           testRoot,
			Purpose:            [32]byte{9},
			ReadMode:           corecontract.QueryRetained,
			Preparation:        preparation,
		},
		record: federationcontract.FederationRecord{
			ID:       testElement,
			Kind:     corecontract.ElementEntity,
			Parent:   testRoot,
			Range:    geometry.BoundedRange(sphere),
			Revision: token(t, owner, "record-revision", seed+2),
			Path:     []values.ElementID{testElement},
			Attachment: &corecontract.PublicFacetInspection{
				State:    corecontract.FacetNone,
				Revision: token(t, owner, "attachment-revision", seed+3),
			},
		},
		initial:  token(t, owner, "observation-cursor", seed+4),
		next:     token(t, owner, "observation-cursor", seed+5),
		changeID: token(t, owner, "change-id", seed+6),
		basis:    token(t, owner, "change-basis", seed+7),
	}
}

func token(t *testing.T, owner values.FactAuthorityRef, kind string, seed byte) values.PublicToken {
	t.Helper()
	raw := [32]byte{seed}
	result, err := values.PublicTokenFromBytes(owner, kind, raw)
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func (p *testConnection) Part() federationcontract.FederationReadPart {
	p.mu.Lock()
	defer p.mu.Unlock()
	return freezeEvidence(p.part)
}

func (p *testConnection) Project(_ context.Context, request federationcontract.FederationProjectionRequest, _ *values.PublicToken, _ uint16) (federationcontract.FederationProjectionPage, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	page := federationcontract.FederationProjectionPage{Basis: p.part.Basis, Purpose: p.part.Purpose}
	switch request.Kind {
	case "path":
		if request.Path == nil {
			return federationcontract.FederationProjectionPage{}, values.PublicInvalid
		}
		page.Path = &federationcontract.FederationPathResult{Element: p.part.Boundary, Kind: corecontract.ElementAnchor, Spine: []federationcontract.FederationSpineNode{{ID: p.part.Boundary}}}
		if len(request.Path.Down) == 1 && request.Path.Down[0] == p.record.ID {
			page.Path.Element = p.record.ID
			page.Path.Kind = p.record.Kind
			page.Path.Spine = append(page.Path.Spine, federationcontract.FederationSpineNode{ID: p.record.ID, Parent: p.part.Boundary})
		} else if len(request.Path.Down) != 0 {
			return federationcontract.FederationProjectionPage{}, values.PublicNotFound
		}
	case "list", "read-at", "read":
		page.Records = []federationcontract.FederationRecord{cloneRecord(p.record)}
	case "query":
		if request.Query == nil {
			return federationcontract.FederationProjectionPage{}, values.PublicInvalid
		}
		page.Coverage = [32]byte{7}
		page.Parents = []federationcontract.FederationParentEvidence{{Parent: request.Query.Scope, Adjacency: [32]byte{8}}}
		page.Matches = []federationcontract.FederationMatch{{ID: p.record.ID, Kind: p.record.Kind, Revision: p.record.Revision, Path: append([]values.ElementID(nil), p.record.Path...)}}
	default:
		return federationcontract.FederationProjectionPage{}, values.PublicInvalid
	}
	return page, nil
}

func (p *testConnection) Check(context.Context, bool) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.checks++
	return nil
}

func (p *testConnection) Observe(_ context.Context, request federationcontract.FederationObserveRequest) (federationcontract.FederationObservationPage, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	switch request.Operation {
	case "begin":
		return federationcontract.FederationObservationPage{Cursor: p.initial}, nil
	case "poll":
		if request.Cursor == nil {
			return federationcontract.FederationObservationPage{}, values.PublicInvalid
		}
		if *request.Cursor == p.initial {
			return federationcontract.FederationObservationPage{Cursor: p.next, Groups: []federationcontract.FederationChangeGroup{{ID: p.changeID, Basis: p.basis, Categories: []string{"spatial"}}}}, nil
		}
		if *request.Cursor == p.next {
			return federationcontract.FederationObservationPage{Cursor: p.next}, nil
		}
		return federationcontract.FederationObservationPage{}, values.PublicGap
	default:
		return federationcontract.FederationObservationPage{}, values.PublicInvalid
	}
}

func (p *testConnection) Release(context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.releases++
	return nil
}

func newListRead(t *testing.T, connection *testConnection, prepare bool) *Read {
	t.Helper()
	request := federationcontract.FederationReadRequest{Boundary: testRoot, Intent: "list", Purpose: [32]byte{9}, PrepareObserve: prepare}
	read, err := New(context.Background(), Config{
		Caller:      testCaller,
		Source:      connection.part.Basis.Owner,
		PartRequest: request,
		Local:       connection,
		Connections: unavailableConnections{},
		SourceLimit: 1,
		Request:     Request{Operation: "list", Address: Address{Origin: testRoot}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return read
}

func newQueryRead(t *testing.T, connection *testConnection) *Read {
	t.Helper()
	request := federationcontract.FederationReadRequest{Boundary: testRoot, Intent: "query", Purpose: [32]byte{9}}
	read, err := New(context.Background(), Config{
		Caller:      testCaller,
		Source:      connection.part.Basis.Owner,
		PartRequest: request,
		Local:       connection,
		Connections: unavailableConnections{},
		SourceLimit: 1,
		Request: Request{Operation: "query", Query: &Query{
			Start:    Address{Origin: testRoot},
			Basis:    Address{Origin: testRoot},
			Sphere:   corecontract.QuerySphere{Radius: 10},
			Selector: corecontract.QueryBoth,
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return read
}

func collectOnce(read *Read, after values.ElementID, limit int, prepare bool) (Page, error) {
	page, lease, err := read.Collect(context.Background(), after, limit, prepare)
	if lease != nil {
		lease.Release()
	}
	return page, err
}

func readAtOnce(read *Read, address Address, prior []*ExactBasis) (ExactResult, *ExactBasis, error) {
	result, basis, lease, err := read.ReadAt(context.Background(), address, prior)
	if lease != nil {
		lease.Release()
	}
	return result, basis, err
}

func observeOnce(read *Read, operation string, position uint64, limit int) (ObservationPage, error) {
	page, lease, err := read.Observe(context.Background(), operation, position, limit)
	if lease != nil {
		lease.Release()
	}
	return page, err
}

func TestReadCollectExactAndRetire(t *testing.T) {
	connection := newTestConnection(t, testOwner, 1, false)
	read := newListRead(t, connection, false)
	first, err := collectOnce(read, "", 64, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Records) != 1 || first.Records[0].ID != testElement || first.Closure == ([32]byte{}) || len(first.SourceGraph.Sources) != 1 || first.ReadMode != corecontract.QueryRetained {
		t.Fatalf("incomplete detached page: %+v", first)
	}
	first.Records[0].Path[0] = testRoot
	first.SourceGraph.Sources[0].CutStamp[0] ^= 1
	second, err := collectOnce(read, "", 64, false)
	if err != nil || second.Records[0].Path[0] != testElement || second.Closure != first.Closure {
		t.Fatal("detached output changed retained recipe", err)
	}
	exactAddress := Address{Origin: testRoot, Down: []values.ElementID{testElement}}
	exact, basis, err := readAtOnce(read, exactAddress, nil)
	if err != nil || exact.Record.ID != testElement || basis == nil || exact.Closure == ([32]byte{}) {
		t.Fatal("exact read failed", err)
	}
	if _, _, err := readAtOnce(read, exactAddress, []*ExactBasis{basis}); err != nil {
		t.Fatal("same-read exact basis rejected", err)
	}
	other := newListRead(t, newTestConnection(t, testOwner2, 21, false), false)
	if _, _, err := readAtOnce(other, exactAddress, []*ExactBasis{basis}); !errors.Is(err, values.PublicInvalidContext) {
		t.Fatal("cross-read exact basis accepted", err)
	}
	if err := other.Retire(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := read.Retire(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := read.Retire(context.Background()); err != nil || connection.releases != 1 {
		t.Fatal("retirement not idempotent", err, connection.releases)
	}
	if _, err := collectOnce(read, "", 1, false); !errors.Is(err, values.PublicInvalidContext) {
		t.Fatal("retired read remained usable", err)
	}
}

func TestReadRejectsChangedSourceEvidence(t *testing.T) {
	connection := newTestConnection(t, testOwner, 41, false)
	read := newListRead(t, connection, false)
	connection.mu.Lock()
	connection.part.CutStamp[0] ^= 1
	connection.mu.Unlock()
	if _, err := collectOnce(read, "", 1, false); !errors.Is(err, values.PublicInvalidContext) {
		t.Fatal("changed evidence was disclosed", err)
	}
	if err := read.Retire(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestObservationDeliveryAcknowledgementAndIsolation(t *testing.T) {
	connection := newTestConnection(t, testOwner, 61, true)
	read := newListRead(t, connection, true)
	if _, err := collectOnce(read, "", 64, true); err != nil {
		t.Fatal(err)
	}
	begin, err := observeOnce(read, "begin-observe", 0, 64)
	if err != nil || begin.Position != 0 || len(begin.Groups) != 0 {
		t.Fatal("begin failed", begin, err)
	}
	first, err := observeOnce(read, "poll", 0, 1)
	if err != nil || first.Position != 1 || first.Base != 0 || len(first.Groups) != 1 {
		t.Fatal("first poll failed", first, err)
	}
	repeat, err := observeOnce(read, "poll", 0, 1)
	if err != nil || !reflect.DeepEqual(first, repeat) {
		t.Fatal("retry changed pending delivery", repeat, err)
	}
	first.Groups[0].Categories[0] = "changed-by-consumer"
	repeat, err = observeOnce(read, "poll", 0, 1)
	if err != nil || repeat.Groups[0].Categories[0] != "spatial" {
		t.Fatal("delivery aliased collector state", err)
	}
	empty, err := observeOnce(read, "poll", 1, 1)
	if err != nil || empty.Position != 1 || empty.Base != 1 || len(empty.Groups) != 0 {
		t.Fatal("acknowledgement failed", empty, err)
	}
	if _, err := observeOnce(read, "poll", 0, 1); !errors.Is(err, values.PublicGap) {
		t.Fatal("acknowledged position reused", err)
	}
	if err := read.Retire(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := observeOnce(read, "poll", 1, 1); !errors.Is(err, values.PublicGap) {
		t.Fatal("retired observation remained usable", err)
	}
}

func testRelation(t *testing.T, parent, carrier values.FactAuthorityRef, anchor values.ElementID, seed byte) (federationcontract.FederationMountRelation, federationcontract.FederationMountRelation) {
	t.Helper()
	sphere, err := corecontract.NewBoundedSphere(float64(seed) + 1)
	if err != nil {
		t.Fatal(err)
	}
	key := corecontract.MountKey{
		ParentAuthority:  parent,
		MountAnchorUUID:  anchor,
		ProposalRevision: coretoken.MountRevisionFromParts([32]byte{seed}, anchor, 1),
		CarrierAuthority: carrier,
	}
	outgoing := federationcontract.FederationMountRelation{
		State:         corecontract.MountActive,
		Key:           key,
		Admission:     corecontract.MountAdmissionKey{MountKey: key, OperationID: corecontract.OperationID("mount-operation")},
		NextHop:       values.NextHop("local:outgoing"),
		Range:         sphere,
		WriteGateOpen: true,
	}
	incoming := outgoing
	incoming.Incoming = true
	incoming.NextHop = values.NextHop("local:incoming")
	return outgoing, incoming
}

func TestSourcesFollowBindsOneOwnerAndCannotGrowAfterSeal(t *testing.T) {
	root := newTestConnection(t, testOwner, 71, false)
	child := newTestConnection(t, testOwner2, 91, false)
	child.part.Boundary = testMount1
	outgoing, incoming := testRelation(t, testOwner, testOwner2, testMount1, 1)
	child.part.Incoming = &incoming
	connections := &fixedConnections{part: child}
	request := federationcontract.FederationReadRequest{Boundary: testRoot, Intent: "query", Purpose: [32]byte{9}}
	sourceSet, err := newSources(Config{
		Caller:      testCaller,
		Source:      testOwner,
		PartRequest: request,
		Local:       root,
		Connections: connections,
		SourceLimit: 3,
	})
	if err != nil {
		t.Fatal(err)
	}
	admitted, err := sourceSet.follow(context.Background(), root, outgoing)
	if err != nil || admitted != child || connections.opens != 1 || len(sourceSet.parts) != 2 {
		t.Fatal("valid source was not admitted exactly once", err)
	}
	node, ok := sourceSet.graph[child.part.Basis.Value]
	if !ok || node.parent != root.part.Basis || node.outgoing != outgoing || !reflect.DeepEqual(node.part, freezeEvidence(child.part)) {
		t.Fatal("source graph lost the exact admitted edge or evidence")
	}
	if _, err := sourceSet.follow(context.Background(), root, outgoing); !errors.Is(err, values.PublicInvalidContext) || connections.opens != 1 || len(sourceSet.parts) != 2 || len(sourceSet.graph) != 2 {
		t.Fatal("duplicate target dialed or mutated the vector", err)
	}
	sourceSet.sealed = true
	other, _ := testRelation(t, testOwner, testOwner3, testMount2, 2)
	if _, err := sourceSet.follow(context.Background(), root, other); !errors.Is(err, values.PublicInvalidContext) || connections.opens != 1 || len(sourceSet.parts) != 2 {
		t.Fatal("sealed vector grew", err)
	}
	if err := sourceSet.release(context.Background()); err != nil || root.releases != 1 || child.releases != 1 || !sourceSet.closed || sourceSet.parts != nil || sourceSet.owners != nil || sourceSet.graph != nil {
		t.Fatal("release retained source state", err)
	}
	if err := sourceSet.check(context.Background()); !errors.Is(err, values.PublicInvalidContext) {
		t.Fatal("released vector remained readable", err)
	}
	if err := sourceSet.release(context.Background()); err != nil || root.releases != 1 || child.releases != 1 {
		t.Fatal("release was not idempotent", err)
	}
}

func evidenceSources(t *testing.T, prepare bool) (*sources, []*testConnection) {
	t.Helper()
	parts := []*testConnection{
		newTestConnection(t, testOwner, 81, prepare),
		newTestConnection(t, testOwner2, 101, prepare),
		newTestConnection(t, testOwner3, 121, prepare),
	}
	parts[1].part.Boundary = testMount1
	parts[2].part.Boundary = testMount2
	out1, in1 := testRelation(t, testOwner, testOwner2, testMount1, 1)
	out2, in2 := testRelation(t, testOwner2, testOwner3, testMount2, 2)
	parts[1].part.Incoming = &in1
	parts[2].part.Incoming = &in2
	request := federationcontract.FederationReadRequest{Boundary: testRoot, Intent: "query", Purpose: [32]byte{9}, PrepareObserve: prepare}
	result := &sources{
		caller:  testCaller,
		source:  testOwner,
		request: request,
		parts: []federationcontract.FederationReadConnection{
			parts[0], parts[1], parts[2],
		},
		owners: map[values.FactAuthorityRef]struct{}{testOwner: {}, testOwner2: {}, testOwner3: {}},
		limit:  3,
		sealed: true,
		graph: map[string]evidenceNode{
			parts[0].part.Basis.Value: {part: freezeEvidence(parts[0].part)},
			parts[1].part.Basis.Value: {part: freezeEvidence(parts[1].part), parent: parts[0].part.Basis, outgoing: out1},
			parts[2].part.Basis.Value: {part: freezeEvidence(parts[2].part), parent: parts[1].part.Basis, outgoing: out2},
		},
	}
	if err := result.checkEvidenceGraph(); err != nil {
		t.Fatal(err)
	}
	return result, parts
}

func TestEvidenceGraphRejectsReplacementCycleAndReparent(t *testing.T) {
	sourceSet, parts := evidenceSources(t, true)
	defer sourceSet.retire()
	target := parts[2]
	original := freezeEvidence(target.part)
	for name, mutate := range map[string]func(*federationcontract.FederationReadPart){
		"cut":         func(part *federationcontract.FederationReadPart) { part.CutStamp[0] ^= 1 },
		"view":        func(part *federationcontract.FederationReadPart) { part.AuthorizationStamp[0] ^= 1 },
		"subject":     func(part *federationcontract.FederationReadPart) { part.Subject.Principal = "another" },
		"purpose":     func(part *federationcontract.FederationReadPart) { part.Purpose[0] ^= 1 },
		"contact":     func(part *federationcontract.FederationReadPart) { part.Incoming.NextHop = "local:replacement" },
		"preparation": func(part *federationcontract.FederationReadPart) { part.Preparation.Value = "replacement" },
	} {
		t.Run(name, func(t *testing.T) {
			target.mu.Lock()
			target.part = freezeEvidence(original)
			mutate(&target.part)
			target.mu.Unlock()
			if err := sourceSet.check(context.Background()); !errors.Is(err, values.PublicInvalidContext) {
				t.Fatal("accepted changed evidence", err)
			}
			target.mu.Lock()
			target.part = freezeEvidence(original)
			target.mu.Unlock()
			if err := sourceSet.check(context.Background()); err != nil {
				t.Fatal("captured evidence was aliased", err)
			}
		})
	}
	key := original.Basis.Value
	node := sourceSet.graph[key]
	changed := node
	changed.parent = original.Basis
	sourceSet.graph[key] = changed
	if err := sourceSet.check(context.Background()); !errors.Is(err, values.PublicInvalidContext) {
		t.Fatal("accepted cycle", err)
	}
	sourceSet.graph[key] = node
	changed = node
	changed.parent = parts[0].part.Basis
	sourceSet.graph[key] = changed
	if err := sourceSet.check(context.Background()); !errors.Is(err, values.PublicInvalidContext) {
		t.Fatal("accepted reparented evidence", err)
	}
	sourceSet.graph[key] = node
	if err := sourceSet.check(context.Background()); err != nil {
		t.Fatal(err)
	}
}

type changedCoveragePart struct {
	federationcontract.FederationReadConnection
}
type changedParentPart struct {
	federationcontract.FederationReadConnection
}

func (part changedCoveragePart) Project(ctx context.Context, request federationcontract.FederationProjectionRequest, continuation *values.PublicToken, limit uint16) (federationcontract.FederationProjectionPage, error) {
	page, err := part.FederationReadConnection.Project(ctx, request, continuation, limit)
	page.Coverage[0] ^= 1
	return page, err
}

func (part changedParentPart) Project(ctx context.Context, request federationcontract.FederationProjectionRequest, continuation *values.PublicToken, limit uint16) (federationcontract.FederationProjectionPage, error) {
	page, err := part.FederationReadConnection.Project(ctx, request, continuation, limit)
	if len(page.Parents) > 0 {
		page.Parents[0].Adjacency[0] ^= 1
	}
	return page, err
}

func TestClosureBindsGapDepthCoverageAndParentEvidence(t *testing.T) {
	read := newQueryRead(t, newTestConnection(t, testOwner, 141, false))
	defer func() {
		read.closed = false
		_ = read.Retire(context.Background())
	}()
	first, err := read.closureCommitment()
	if err != nil || first == ([32]byte{}) {
		t.Fatal(err)
	}
	second, err := read.closureCommitment()
	if err != nil || second != first {
		t.Fatal("unstable closure", err)
	}
	read.gap = corecontract.QueryUnresolvedCoverage
	second, err = read.closureCommitment()
	if err != nil || second == first {
		t.Fatal("coverage gap not bound", err)
	}
	read.gap = corecontract.QueryNoGap
	read.actual++
	second, err = read.closureCommitment()
	if err != nil || second == first {
		t.Fatal("depth not bound", err)
	}
	read.actual--
	original := read.plans[0].coverage
	read.plans[0].coverage[0] ^= 1
	second, err = read.closureCommitment()
	if err != nil || second == first {
		t.Fatal("scan evidence not bound", err)
	}
	read.plans[0].coverage = [32]byte{}
	if _, err := read.closureCommitment(); !errors.Is(err, values.PublicInvalidContext) {
		t.Fatal("missing scan evidence accepted", err)
	}
	read.plans[0].coverage = original
	plan := read.plans[0]
	plan.part = changedCoveragePart{plan.part}
	if _, err := read.scan(context.Background(), plan, "", 1); !errors.Is(err, values.PublicInvalidContext) {
		t.Fatal("changed source scan proof accepted", err)
	}
	plan.part = changedParentPart{read.plans[0].part}
	if _, err := read.scan(context.Background(), plan, "", 1); !errors.Is(err, values.PublicInvalidContext) {
		t.Fatal("changed parent proof accepted", err)
	}
	read.closed = true
	if _, err := read.closureCommitment(); !errors.Is(err, values.PublicInvalidContext) {
		t.Fatal("closed recipe accepted", err)
	}
}

type observationFaultPart struct {
	federationcontract.FederationReadConnection
	fail    bool
	corrupt bool
	revoked bool
	after   func()
}

func (part *observationFaultPart) Observe(ctx context.Context, request federationcontract.FederationObserveRequest) (federationcontract.FederationObservationPage, error) {
	if part.revoked {
		return federationcontract.FederationObservationPage{}, values.PublicGap
	}
	if request.Operation == "poll" && part.fail {
		return federationcontract.FederationObservationPage{}, values.PublicUnavailable
	}
	page, err := part.FederationReadConnection.Observe(ctx, request)
	if request.Operation == "poll" && part.after != nil {
		part.after()
	}
	if request.Operation == "poll" && part.corrupt && len(page.Groups) > 0 {
		page.Groups[0].Categories = []string{"content"}
	}
	return page, err
}

func observationFixture(t *testing.T) (*Read, *observation, []*testConnection) {
	t.Helper()
	sourceSet, parts := evidenceSources(t, true)
	read := &Read{sources: sourceSet, prefix: map[string][]values.ElementID{}, required: map[string]bool{}, operation: "query"}
	observation, err := read.beginObservation(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = read.Retire(context.Background()) })
	return read, observation, parts
}

func TestObservationFailedCollectionDoesNotConsume(t *testing.T) {
	read, observation, parts := observationFixture(t)
	fault := &observationFaultPart{FederationReadConnection: read.sources.parts[1], fail: true}
	read.sources.parts[1] = fault
	if _, err := observation.poll(context.Background(), 0, 2); !errors.Is(err, values.PublicUnavailable) {
		t.Fatal(err)
	}
	if observation.pending != nil || observation.position != 0 || observation.gap {
		t.Fatal("failed poll consumed delivery state")
	}
	fault.fail = false
	batch, err := observation.poll(context.Background(), 0, 2)
	if err != nil || len(batch.groups) != 2 || batch.groups[0].ID.Owner != parts[0].part.Basis.Owner || batch.groups[1].ID.Owner != parts[1].part.Basis.Owner {
		t.Fatal("retry lost first source", batch, err)
	}
}

func TestObservationCorruptSourceGapsWithoutDisclosure(t *testing.T) {
	read, observation, _ := observationFixture(t)
	read.sources.parts[1] = &observationFaultPart{FederationReadConnection: read.sources.parts[1], corrupt: true}
	if batch, err := observation.poll(context.Background(), 0, 2); !errors.Is(err, values.PublicGap) || len(batch.groups) != 0 {
		t.Fatal(batch, err)
	}
	if _, err := observation.poll(context.Background(), 0, 2); !errors.Is(err, values.PublicGap) {
		t.Fatal("permanent source gap renewed", err)
	}
}

func TestObservationRechecksEvidenceForCachedAndNewBatch(t *testing.T) {
	for _, cached := range []bool{false, true} {
		t.Run(map[bool]string{false: "during-collection", true: "cached-retry"}[cached], func(t *testing.T) {
			read, observation, _ := observationFixture(t)
			fault := &observationFaultPart{FederationReadConnection: read.sources.parts[0]}
			read.sources.parts[0] = fault
			if cached {
				if batch, err := observation.poll(context.Background(), 0, 1); err != nil || len(batch.groups) != 1 {
					t.Fatal(batch, err)
				}
				fault.revoked = true
			} else {
				fault.after = func() { fault.revoked = true }
			}
			batch, err := observation.poll(context.Background(), 0, 1)
			if !errors.Is(err, values.PublicGap) || len(batch.groups) != 0 || !observation.gap {
				t.Fatal("changed evidence disclosed or renewed", batch, err)
			}
		})
	}
}

func TestObservationCancellationDoesNotAcknowledgeCollectedGroups(t *testing.T) {
	read, observation, _ := observationFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	fault := &observationFaultPart{FederationReadConnection: read.sources.parts[0], after: cancel}
	read.sources.parts[0] = fault
	if batch, err := observation.poll(ctx, 0, 1); !errors.Is(err, values.PublicUnavailable) || len(batch.groups) != 0 || observation.pending != nil {
		t.Fatal(batch, err)
	}
	fault.after = nil
	batch, err := observation.poll(context.Background(), 0, 1)
	if err != nil || len(batch.groups) != 1 || batch.position != 1 {
		t.Fatal("cancelled collection lost group", batch, err)
	}
}

func TestObservationBeginAccountingAndReuse(t *testing.T) {
	read, observation, _ := observationFixture(t)
	used := read.used
	again, err := read.beginObservation(context.Background())
	if err != nil || again != observation || read.used != used {
		t.Fatal("Begin renewed or duplicated retained state", err)
	}
	probe := &Read{sources: read.sources, used: RecipeBytes}
	if _, err := probe.beginObservation(context.Background()); !errors.Is(err, values.PublicResourceExhausted) || probe.used != RecipeBytes {
		t.Fatal("unaccounted observation allocation", err)
	}
	probe.used = 0
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := probe.beginObservation(ctx); !errors.Is(err, values.PublicUnavailable) || probe.used != 0 || probe.observation != nil {
		t.Fatal("failed Begin retained quota", err)
	}
}
