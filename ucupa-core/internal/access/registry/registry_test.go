package registry

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	accessobserve "ucupa/ucupa-core/internal/access/observe"
	elementfact "ucupa/ucupa-core/internal/element/fact"
	facetfact "ucupa/ucupa-core/internal/facet/fact"
	mountfact "ucupa/ucupa-core/internal/mount/fact"
	transferfact "ucupa/ucupa-core/internal/transfer/fact"
)

func TestRegistryRetiresAuthorizationOutsideFactLock(t *testing.T) {
	for _, mode := range []string{"remove", "expire", "close"} {
		t.Run(mode, func(t *testing.T) {
			r, locker, _, caller, boundary := registryFixture(t, 1<<20)
			locker.Lock()
			space, err := r.IssueSpaceLocked(caller, boundary)
			if err != nil {
				locker.Unlock()
				t.Fatal(err)
			}
			read, err := r.IssueLocalReadLocked(caller, boundary, space.Ref, contract.CoreReadCut{}, values.AuthorizationView{1}, nil, "list", "ab")
			if err != nil {
				locker.Unlock()
				t.Fatal(err)
			}
			calls := 0
			if !r.OwnAuthorizationReleaseLocked(read.Ref, func(ctx context.Context) {
				if ctx.Err() != nil || !locker.TryLock() {
					t.Error("cleanup ran with canceled context or under fact lock")
					return
				}
				locker.Unlock()
				calls++
			}) {
				t.Fatal("receipt did not own its evidence cleanup")
			}
			locker.Unlock()
			r.DrainRetired()
			if calls != 0 {
				t.Fatal("live receipt released its authorization")
			}
			locker.Lock()
			switch mode {
			case "remove":
				r.RemoveLocked(space.Ref)
			case "expire":
				r.now = func() time.Time { return time.Now().Add(2 * time.Hour) }
				r.ExpireLocked()
			case "close":
				r.CloseLocked()
			}
			if calls != 0 {
				t.Error("retirement performed network cleanup under lock")
			}
			locker.Unlock()
			r.DrainRetired()
			r.DrainRetired()
			if calls != 1 {
				t.Fatalf("cleanup calls = %d, want one", calls)
			}
		})
	}
}

type snapshotRecorder struct {
	items []contract.CoreReadSnapshot
}

func (recorder *snapshotRecorder) DiscardOwnedSnapshot(snapshot contract.CoreReadSnapshot) {
	recorder.items = append(recorder.items, snapshot)
}

func registryFixture(t *testing.T, bytes uint64) (*Registry, *sync.Mutex, *snapshotRecorder, values.CallerContext, values.ElementID) {
	t.Helper()
	authority, err := values.NewFactAuthorityRef()
	if err != nil {
		t.Fatal(err)
	}
	locker := new(sync.Mutex)
	snapshots := new(snapshotRecorder)
	limits := values.PublicLimits{
		ContextTTL: time.Hour, ObservationTTL: time.Minute,
		Contexts: 32, PerCaller: 32, Groups: 4, Bytes: bytes,
	}
	caller := values.CallerContext{Subject: values.PrincipalRef{Realm: "registry-test", Principal: "caller"}}
	boundary := values.ElementID("10000000-0000-4000-8000-000000000001")
	return New(authority, limits, locker, snapshots), locker, snapshots, caller, boundary
}

func TestRegistryOwnsCascadeSnapshotAndExactAccounting(t *testing.T) {
	registry, locker, snapshots, caller, boundary := registryFixture(t, 1<<20)
	locker.Lock()
	space, err := registry.IssueSpaceLocked(caller, boundary)
	if err != nil {
		t.Fatal(err)
	}
	read, err := registry.IssueLocalReadLocked(caller, boundary, space.Ref, contract.CoreReadCut{}, values.AuthorizationView{}, nil, "list", "ab")
	if err != nil {
		t.Fatal(err)
	}
	if !registry.InstallSnapshotLocked(read.Ref, &contract.CoreReadSnapshot{}) {
		t.Fatal("snapshot was not installed")
	}
	continuation, err := registry.IssueContinuationLocked(read.Ref, boundary, "")
	if err != nil {
		t.Fatal(err)
	}
	derived, err := registry.IssueDerivedReadLocked(caller, boundary, space.Ref, read.Ref, values.AuthorizationView{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	preparation, err := registry.PrepareLocalLocked(space.Ref, nil, values.AuthorizationView{}, contract.CoreReadCut{}, boundary, true)
	if err != nil {
		t.Fatal(err)
	}
	if !registry.LinkPreparationLocked(read.Ref, preparation.Ref) {
		t.Fatal("preparation was not linked")
	}
	registry.RemoveLocked(read.Ref)
	if _, ok := registry.ReadLocked(read.Ref); ok {
		t.Fatal("removed read remains live")
	}
	if _, err := registry.ResolveContinuationLocked(caller, continuation.Token); !errors.Is(err, values.PublicInvalidContext) {
		t.Fatalf("continuation error = %v", err)
	}
	if _, ok := registry.ReadLocked(derived.Ref); ok {
		t.Fatal("basis-owned derived read remains live")
	}
	if _, ok := registry.PreparationLocked(preparation.Ref); !ok {
		t.Fatal("separately space-parented local preparation was over-released")
	}
	if len(snapshots.items) != 1 {
		t.Fatalf("discarded snapshots = %d, want 1", len(snapshots.items))
	}
	registry.RemoveLocked(space.Ref)
	if len(registry.entries) != 0 || registry.bytes != 0 {
		t.Fatalf("resources remain: entries=%d bytes=%d", len(registry.entries), registry.bytes)
	}
	locker.Unlock()
}

// The source service test checks the real call and exposed tokens. The exact
// resource bookkeeping follows its one owner, rather than exposing its maps to
// Core/source tests just to preserve old root white-box access.
func TestRegistrySourceReleaseReclaimsPreparationCursorAndSnapshot(t *testing.T) {
	r, locker, snapshots, caller, boundary := registryFixture(t, 1<<20)
	locker.Lock()
	defer locker.Unlock()
	reservation, err := r.ReserveLocked(4096)
	if err != nil {
		t.Fatal(err)
	}
	read, err := r.IssueSourceLocked(reservation, caller, boundary, contract.CoreReadCut{}, values.AuthorizationView{1}, nil, SourceConfig{Source: r.authority})
	if err != nil {
		t.Fatal(err)
	}
	if !r.InstallSnapshotLocked(read.Ref, &contract.CoreReadSnapshot{}) {
		t.Fatal("snapshot not installed")
	}
	prep, err := r.PrepareSourceLocked(read.Ref, nil, values.AuthorizationView{1}, contract.CoreReadCut{}, false)
	if err != nil {
		t.Fatal(err)
	}
	cursor, err := r.BeginLocalLocked(prep.Ref)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.entries) != 3 || r.bytes <= 4096 {
		t.Fatal("fixture did not allocate the real source resources")
	}
	r.RemoveLocked(read.Ref)
	if len(r.entries) != 0 || r.bytes != 0 || len(snapshots.items) != 1 {
		t.Fatalf("source resource leak: entries=%d bytes=%d snapshots=%d", len(r.entries), r.bytes, len(snapshots.items))
	}
	if _, err := r.ResolveCursorLocked(caller, cursor); !errors.Is(err, values.PublicGap) {
		t.Fatal("source cursor not retired", err)
	}
	r.RemoveLocked(read.Ref)
	if len(snapshots.items) != 1 || r.bytes != 0 {
		t.Fatal("repeat retirement changed refund")
	}
}

func TestRegistryExpiryPreservesMissingErrorClasses(t *testing.T) {
	registry, locker, _, caller, boundary := registryFixture(t, 1<<20)
	locker.Lock()
	space, err := registry.IssueSpaceLocked(caller, boundary)
	if err != nil {
		t.Fatal(err)
	}
	prep, err := registry.PrepareLocalLocked(space.Ref, nil, values.AuthorizationView{}, contract.CoreReadCut{}, "", false)
	if err != nil {
		t.Fatal(err)
	}
	space.Ref.entry.expires = registry.now().Add(-time.Nanosecond)
	registry.ExpireLocked()
	if _, err := registry.ResolveSpaceLocked(caller, space.Token); !errors.Is(err, values.PublicInvalidContext) {
		t.Fatalf("expired handle error = %v", err)
	}
	if _, err := registry.ResolvePreparationLocked(caller, prep.Token); !errors.Is(err, values.PublicGap) {
		t.Fatalf("expired preparation error = %v", err)
	}
	if len(registry.entries) != 0 || registry.bytes != 0 {
		t.Fatalf("expired resources remain: entries=%d bytes=%d", len(registry.entries), registry.bytes)
	}
	locker.Unlock()
}

// The original read-at flow rechecked a basis, then issued the derived receipt
// through the common issuer, whose final expiry sweep could retire that basis.
// Preserve that narrow window during this ownership move: issuance still
// succeeds, inherits the retired basis deadline, and the derived receipt is
// reclaimed by the next expiry sweep instead of changing the call's error
// priority at the registry boundary.
func TestDerivedIssuePreservesFinalExpirySweepWindow(t *testing.T) {
	registry, locker, _, caller, boundary := registryFixture(t, 1<<20)
	locker.Lock()
	space, err := registry.IssueSpaceLocked(caller, boundary)
	if err != nil {
		t.Fatal(err)
	}
	basis, err := registry.IssueLocalReadLocked(caller, boundary, space.Ref, contract.CoreReadCut{}, values.AuthorizationView{}, nil, "read", "")
	if err != nil {
		t.Fatal(err)
	}
	basis.Ref.entry.expires = registry.now().Add(-time.Nanosecond)
	derived, err := registry.IssueDerivedReadLocked(caller, boundary, space.Ref, basis.Ref, values.AuthorizationView{}, nil)
	if err != nil {
		t.Fatalf("derived issue changed final-sweep result: %v", err)
	}
	if _, present := registry.ReadLocked(basis.Ref); present {
		t.Fatal("final issuance sweep did not retire expired basis")
	}
	if current, present := registry.ReadLocked(derived.Ref); !present || current.Basis != basis.Ref || !current.HasBasis {
		t.Fatalf("derived receipt did not retain the original basis binding: present=%v current=%#v", present, current)
	}
	registry.ExpireLocked()
	if _, present := registry.ReadLocked(derived.Ref); present {
		t.Fatal("derived receipt survived its inherited expired deadline")
	}
	registry.RemoveLocked(space.Ref)
	locker.Unlock()
}

func TestRegistryPublishesAndConsumesOneAtomicObservationGroup(t *testing.T) {
	registry, locker, _, caller, boundary := registryFixture(t, 1<<20)
	locker.Lock()
	space, err := registry.IssueSpaceLocked(caller, boundary)
	if err != nil {
		t.Fatal(err)
	}
	prep, err := registry.PrepareLocalLocked(space.Ref, nil, values.AuthorizationView{}, contract.CoreReadCut{}, boundary, false)
	if err != nil {
		t.Fatal(err)
	}
	beforeElements := elementfact.New()
	afterElements := elementfact.New()
	if err := afterElements.RestoreAnchor(elementfact.Anchor{ID: boundary}); err != nil {
		t.Fatal(err)
	}
	mounts := mountfact.New()
	attachments := facetfact.New()
	registry.Publish(
		accessobserve.View{Present: true, CutSerial: 1, Elements: beforeElements.View, Mount: mounts.View, Attachments: attachments.View},
		accessobserve.View{Present: true, CutSerial: 2, Elements: afterElements.View, Mount: mounts.View, Attachments: attachments.View},
	)
	queue := prep.Ref.entry.preparation.queue
	if len(queue.groups) != 1 || len(queue.groups[0].Categories) != 1 || queue.groups[0].Categories[0] != "spatial" {
		t.Fatalf("unexpected groups: %#v", queue.groups)
	}
	cursor, err := registry.BeginLocalLocked(prep.Ref)
	if err != nil {
		t.Fatal(err)
	}
	charged := registry.bytes
	resolved, err := registry.ResolveCursorLocked(caller, cursor)
	if err != nil {
		t.Fatal(err)
	}
	groups, next, err := registry.PollLocalLocked(resolved.Ref, 1)
	if err != nil || len(groups) != 1 || !next.Valid() {
		t.Fatalf("poll = groups:%#v next:%#v err:%v", groups, next, err)
	}
	nextCursor, err := registry.ResolveCursorLocked(caller, next)
	if err != nil {
		t.Fatal(err)
	}
	groups, _, err = registry.PollLocalLocked(nextCursor.Ref, 1)
	if err != nil || len(groups) != 0 {
		t.Fatalf("consume poll = groups:%#v err:%v", groups, err)
	}
	if registry.bytes != charged-512 {
		t.Fatalf("consumed group refund bytes=%d want=%d", registry.bytes, charged-512)
	}
	registry.RemoveLocked(space.Ref)
	locker.Unlock()
}

// prepareQueueForTerminalTest issues one local observation over the given
// element facts and returns its preparation, cursor and steady before view.
func prepareQueueForTerminalTest(t *testing.T, registry *Registry, caller values.CallerContext, boundary values.ElementID, elements elementfact.View, exact values.ElementID) (PreparationUse, values.PublicToken, accessobserve.View) {
	t.Helper()
	space, err := registry.IssueSpaceLocked(caller, boundary)
	if err != nil {
		t.Fatal(err)
	}
	prep, err := registry.PrepareLocalLocked(space.Ref, nil, values.AuthorizationView{}, contract.CoreReadCut{}, exact, false)
	if err != nil {
		t.Fatal(err)
	}
	cursor, err := registry.BeginLocalLocked(prep.Ref)
	if err != nil {
		t.Fatal(err)
	}
	before := accessobserve.View{Present: true, CutSerial: 1, Elements: elements,
		Mount: mountfact.New().View, Attachments: facetfact.New().View}
	return prep, cursor, before
}

func TestRegistryRetainsTerminalEvidenceThroughPendingDecision(t *testing.T) {
	registry, locker, _, caller, boundary := registryFixture(t, 1<<20)
	locker.Lock()
	defer locker.Unlock()
	prep, cursor, before := prepareQueueForTerminalTest(t, registry, caller, boundary, elementfact.New().View, boundary)
	fenced := before
	fenced.CutSerial = 2
	fenced.Transfers = transferfact.NewVisibility(map[values.ElementID]values.ElementID{boundary: boundary}, nil, nil, nil,
		map[values.ElementID]bool{boundary: true})
	registry.Publish(before, fenced)
	queue := prep.Ref.entry.preparation.queue
	if queue.gap || queue.terminal || !queue.pending || len(queue.groups) != 0 || len(queue.pendingIDs) != 1 || queue.pendingIDs[0] != boundary {
		t.Fatalf("own unsettled fence did not hold the loss pending: %#v", queue)
	}
	resolved, err := registry.ResolveCursorLocked(caller, cursor)
	if err != nil {
		t.Fatal(err)
	}
	if groups, next, err := registry.PollLocalLocked(resolved.Ref, 4); err != nil || len(groups) != 0 || next != cursor {
		t.Fatalf("pending decision streamed or moved its cursor: %#v %v %v", groups, next, err)
	}
	pendingCharge := registry.bytes
	terminal := contract.TransitionTerminal{Operation: "settle", Source: "00000000-0000-4000-8000-000000000001",
		Target: "00000000-0000-4000-8000-000000000002", FinalCut: 9, Settled: true, NextHop: "local:target"}
	settled := fenced
	settled.CutSerial = 3
	settled.Transfers = transferfact.NewVisibility(map[values.ElementID]values.ElementID{boundary: boundary}, nil, nil,
		map[values.ElementID]contract.TransitionTerminal{boundary: terminal}, nil)
	registry.Publish(fenced, settled)
	if !queue.terminal || queue.pending || queue.pendingIDs != nil || len(queue.groups) != 1 ||
		len(queue.groups[0].Categories) != 1 || queue.groups[0].Categories[0] != "terminal" ||
		queue.groups[0].Terminal == nil || *queue.groups[0].Terminal != terminal {
		t.Fatalf("settled decision did not disclose its terminal evidence: %#v", queue)
	}
	if registry.bytes != pendingCharge-128+512 {
		t.Fatalf("terminal disclosure charge bytes=%d want=%d", registry.bytes, pendingCharge-128+512)
	}
	groups, next, err := registry.PollLocalLocked(resolved.Ref, 4)
	if err != nil || len(groups) != 1 || groups[0].Terminal == nil || *groups[0].Terminal != terminal || !next.Valid() {
		t.Fatalf("terminal poll = %#v %v %v", groups, next, err)
	}
	reread, rereadNext, err := registry.PollLocalLocked(resolved.Ref, 4)
	if err != nil || rereadNext != next || len(reread) != 1 || reread[0].ID != groups[0].ID || reread[0].Basis != groups[0].Basis ||
		reread[0].Terminal == nil || *reread[0].Terminal != terminal {
		t.Fatalf("terminal evidence not idempotent at its cursor: %#v %v", reread, err)
	}
	nextResolved, err := registry.ResolveCursorLocked(caller, next)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := registry.PollLocalLocked(nextResolved.Ref, 4); !errors.Is(err, values.PublicGap) {
		t.Fatalf("exhausted terminal window kept streaming: %v", err)
	}
}

// The contract holds a pending queue exempt from ordinary publications: a
// relevant write in the same observed scope during the unsettled decision
// must not enter the queue, and the later settlement discloses exactly its
// own terminal group without carrying that write.
func TestRegistryPendingQueueWithholdsOrdinaryChanges(t *testing.T) {
	registry, locker, _, caller, boundary := registryFixture(t, 1<<20)
	locker.Lock()
	defer locker.Unlock()
	projection := elementfact.NewProjection()
	projection.AddAnchor(boundary, elementfact.Anchor{ID: boundary})
	departing := values.ElementID("10000000-0000-4000-8000-000000000002")
	projection.AddEntity(departing, elementfact.Entity{ID: departing, Parent: boundary})
	prep, cursor, before := prepareQueueForTerminalTest(t, registry, caller, boundary, projection.Build().View, "")
	fenced := before
	fenced.CutSerial = 2
	fenced.Transfers = transferfact.NewVisibility(map[values.ElementID]values.ElementID{departing: boundary}, nil, nil, nil,
		map[values.ElementID]bool{departing: true})
	registry.Publish(before, fenced)
	queue := prep.Ref.entry.preparation.queue
	if queue.gap || queue.terminal || !queue.pending || len(queue.pendingIDs) != 1 || queue.pendingIDs[0] != departing {
		t.Fatalf("own unsettled fence did not hold the loss pending: %#v", queue)
	}
	pendingCharge := registry.bytes
	// One ordinary relevant write — a new member entering the observed
	// boundary while the recorded decision is still unsettled — must not
	// enter the pending queue as an ordinary change group.
	ordinary := fenced
	ordinary.CutSerial = 3
	written := elementfact.NewProjection()
	written.AddAnchor(boundary, elementfact.Anchor{ID: boundary})
	written.AddEntity(departing, elementfact.Entity{ID: departing, Parent: boundary})
	newcomer := values.ElementID("10000000-0000-4000-8000-000000000003")
	written.AddEntity(newcomer, elementfact.Entity{ID: newcomer, Parent: boundary})
	ordinary.Elements = written.Build().View
	registry.Publish(fenced, ordinary)
	if queue.gap || queue.terminal || !queue.pending || len(queue.groups) != 0 ||
		len(queue.pendingIDs) != 1 || queue.pendingIDs[0] != departing || registry.bytes != pendingCharge {
		t.Fatalf("ordinary write entered the pending queue: %#v bytes=%d", queue, registry.bytes)
	}
	// The settlement then discloses exactly its own terminal group.
	terminal := contract.TransitionTerminal{Operation: "settle", Source: "00000000-0000-4000-8000-000000000001",
		Target: "00000000-0000-4000-8000-000000000002", FinalCut: 9, Settled: true, NextHop: "local:target"}
	settled := ordinary
	settled.CutSerial = 4
	settled.Transfers = transferfact.NewVisibility(map[values.ElementID]values.ElementID{departing: boundary}, nil, nil,
		map[values.ElementID]contract.TransitionTerminal{departing: terminal}, nil)
	registry.Publish(ordinary, settled)
	if !queue.terminal || queue.pending || queue.pendingIDs != nil || len(queue.groups) != 1 ||
		queue.groups[0].Terminal == nil || *queue.groups[0].Terminal != terminal {
		t.Fatalf("settled decision did not disclose its sole terminal evidence: %#v", queue)
	}
	resolved, err := registry.ResolveCursorLocked(caller, cursor)
	if err != nil {
		t.Fatal(err)
	}
	groups, next, err := registry.PollLocalLocked(resolved.Ref, 4)
	if err != nil || len(groups) != 1 || groups[0].Terminal == nil || *groups[0].Terminal != terminal || !next.Valid() {
		t.Fatalf("terminal poll carried ordinary changes: %#v %v %v", groups, next, err)
	}
}

func TestRegistryPendingDecisionLosesWindowAndGaps(t *testing.T) {
	registry, locker, _, caller, boundary := registryFixture(t, 1<<20)
	locker.Lock()
	defer locker.Unlock()
	prep, cursor, before := prepareQueueForTerminalTest(t, registry, caller, boundary, elementfact.New().View, boundary)
	fenced := before
	fenced.CutSerial = 2
	fenced.Transfers = transferfact.NewVisibility(map[values.ElementID]values.ElementID{boundary: boundary}, nil, nil, nil,
		map[values.ElementID]bool{boundary: true})
	registry.Publish(before, fenced)
	if queue := prep.Ref.entry.preparation.queue; !queue.pending {
		t.Fatalf("fenced loss did not hold the queue pending: %#v", queue)
	}
	// A decision that never settles loses its retention window like any other
	// context; the old cursor then resolves to GAP instead of stale evidence.
	registry.now = func() time.Time { return time.Now().Add(2 * time.Hour) }
	registry.ExpireLocked()
	if _, err := registry.ResolveCursorLocked(caller, cursor); !errors.Is(err, values.PublicGap) {
		t.Fatalf("stuck pending decision outlived its retention window: %v", err)
	}
}

func TestRegistryUnprovableMixedOrGateLossGapsImmediately(t *testing.T) {
	registry, locker, _, caller, boundary := registryFixture(t, 1<<20)
	locker.Lock()
	defer locker.Unlock()
	projection := elementfact.NewProjection()
	projection.AddAnchor(boundary, elementfact.Anchor{ID: boundary})
	first := values.ElementID("10000000-0000-4000-8000-000000000002")
	second := values.ElementID("10000000-0000-4000-8000-000000000003")
	projection.AddEntity(first, elementfact.Entity{ID: first, Parent: boundary})
	projection.AddEntity(second, elementfact.Entity{ID: second, Parent: boundary})
	elements := projection.Build().View

	unrelated, _, before := prepareQueueForTerminalTest(t, registry, caller, boundary, elements, "")
	unrelatedAfter := before
	unrelatedAfter.CutSerial = 2
	unrelatedAfter.Transfers = transferfact.NewVisibility(map[values.ElementID]values.ElementID{first: boundary}, nil, nil, nil, nil)
	registry.Publish(before, unrelatedAfter)
	if queue := unrelated.Ref.entry.preparation.queue; !queue.gap {
		t.Fatalf("unprovable departure was not an immediate GAP: %#v", queue)
	}

	mixed, mixedCursor, mixedBefore := prepareQueueForTerminalTest(t, registry, caller, boundary, elements, "")
	mixedAfter := mixedBefore
	mixedAfter.CutSerial = 2
	mixedAfter.Transfers = transferfact.NewVisibility(map[values.ElementID]values.ElementID{first: boundary, second: boundary}, nil, nil, nil,
		map[values.ElementID]bool{first: true})
	registry.Publish(mixedBefore, mixedAfter)
	if queue := mixed.Ref.entry.preparation.queue; !queue.gap {
		t.Fatalf("mixed provable and unprovable losses stayed observable: %#v", queue)
	}
	mixedResolved, err := registry.ResolveCursorLocked(caller, mixedCursor)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := registry.PollLocalLocked(mixedResolved.Ref, 4); !errors.Is(err, values.PublicGap) {
		t.Fatalf("mixed loss poll did not GAP: %v", err)
	}

	gated, gatedCursor, gateBefore := prepareQueueForTerminalTest(t, registry, caller, boundary, elements, "")
	gateAfter := gateBefore
	gateAfter.CutSerial = 2
	closed := mountfact.New()
	closed.ReplaceIncoming(mountfact.Incoming{Gate: mountfact.GateFenced})
	gateAfter.Mount = closed.View
	gateAfter.Transfers = transferfact.NewVisibility(map[values.ElementID]values.ElementID{first: boundary}, nil, nil, nil,
		map[values.ElementID]bool{first: true})
	registry.Publish(gateBefore, gateAfter)
	if queue := gated.Ref.entry.preparation.queue; !queue.gap || queue.pending {
		t.Fatalf("parent-side gate closure was not an immediate GAP: %#v", queue)
	}
	gatedResolved, err := registry.ResolveCursorLocked(caller, gatedCursor)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := registry.PollLocalLocked(gatedResolved.Ref, 4); !errors.Is(err, values.PublicGap) {
		t.Fatalf("gate loss poll did not GAP: %v", err)
	}
}
