package acceptance_test

import (
	"context"
	"errors"
	"fmt"
	"math"
	"reflect"
	"testing"
	"time"

	"ucupa/shared/values"
	corecontract "ucupa/ucupa-core/contract"
)

func TestP3ConnectedRetainedDeadlineExpiresWithoutLatestFallback(t *testing.T) {
	f := newConnectedFixture(t, true, true)
	ttl := time.Second
	f.configA.SnapshotRetention, f.configB.SnapshotRetention = &ttl, &ttl
	f.reopenConnected()
	first := f.queryB(1)
	// This waits for the real public retention policy, not for a racing event.
	// The per-owner deterministic clock cases belong to the Core tests.
	timer := time.NewTimer(2 * ttl)
	defer timer.Stop()
	<-timer.C
	page, err := f.b.Connected().QueryRegionPath(context.Background(), f.callers(), f.queryFromB(), nil, 1, first.Continuation)
	assertConnectedQueryError(t, page, err, corecontract.ErrReadCutExpired)
	if err := f.b.Connected().ReleaseConnectedCut(context.Background(), f.callers(), first.Cut); err != nil {
		t.Fatalf("expired context cannot be released: %v", err)
	}
	if fresh := f.queryB(256); fresh.Status != corecontract.QueryComplete || fresh.ReadMode != corecontract.QueryRetained {
		t.Fatalf("fresh request changed mode after expiration: %#v", fresh)
	}
}

func TestP3ConnectedCapacityDoesNotDowngradeRetainedMode(t *testing.T) {
	f := newConnectedFixture(t, true, true)
	f.configA.SnapshotCapacity, f.configB.SnapshotCapacity = 1, 1
	f.reopenConnected()
	first := f.queryB(1)
	second, err := f.b.Connected().QueryRegionPath(context.Background(), f.callers(), f.queryFromB(), nil, 1, nil)
	assertConnectedQueryError(t, second, err, corecontract.ErrSnapshotCapacity)
	page, err := f.b.Connected().QueryRegionPath(context.Background(), f.callers(), f.queryFromB(), nil, 1, first.Continuation)
	if err != nil || page.ReadMode != corecontract.QueryRetained || !page.Cut.Equal(first.Cut) {
		t.Fatalf("failed admission replaced existing context: %#v, %v", page, err)
	}
	if err := f.b.Connected().ReleaseConnectedCut(context.Background(), f.callers(), first.Cut); err != nil {
		t.Fatal(err)
	}
	if fresh := f.queryB(1); fresh.ReadMode != corecontract.QueryRetained {
		t.Fatal("released capacity silently selected current mode")
	}
}

func TestP3ConnectedExactWorkGapPreservesIndependentHits(t *testing.T) {
	f := newConnectedFixture(t, true, true)
	rotation := mustRotation(t, 1, math.SmallestNonzeroFloat64, 0, 0)
	pose := mustPose(t, corecontract.Vec3{Y: 1}, rotation)
	parent := mountAnchor
	for depth := 1; depth <= 40; depth++ {
		id := values.ElementID(fmt.Sprintf("00000000-0000-4000-8000-%012d", 9500+depth))
		connectedGrant(t, f.policyB, f.actorB, f.b.AuthorityRef(), parent,
			corecontract.PermissionCreateAnchor, corecontract.PermissionCreateEntity, corecontract.PermissionPlaceElement)
		if _, err := f.b.Elements().CreateAnchor(f.actorB, id, corecontract.SpatialTuple{Parent: parent, LocalPose: pose,
			Range: mustSphere(t, float64(100-depth))}, corecontract.OperationID(fmt.Sprintf("connected-deep-%d", depth))); err != nil {
			t.Fatal(err)
		}
		parent = id
	}
	const tooDeep values.ElementID = "00000000-0000-4000-8000-000000009599"
	connectedGrant(t, f.policyB, f.actorB, f.b.AuthorityRef(), parent, corecontract.PermissionCreateEntity)
	if _, err := f.b.Elements().CreateEntity(f.actorB, tooDeep, f.tuple(parent, 0, 0), "connected-deep-entity"); err != nil {
		t.Fatal(err)
	}
	q := f.queryFromA()
	q.Selector = corecontract.QueryEntities
	var continuation *corecontract.ConnectedQueryContinuation
	var ids []values.ElementID
	for count := 0; count < 8; count++ {
		page, err := f.a.Connected().QueryRegionPath(context.Background(), f.callers(), q, nil, 1, continuation)
		if err != nil || page.Status != corecontract.QueryPartial || page.Gap&corecontract.QueryExactWorkLimit == 0 || page.Gap&corecontract.QueryUnresolvedCoverage != 0 {
			t.Fatalf("numeric uncertainty was dropped or called source failure: %#v, %v", page, err)
		}
		ids = append(ids, connectedIDs(page.Matches)...)
		if page.Continuation == nil {
			want := []values.ElementID{mountEntity, connectedRootEntity, connectedSiblingEntity, connectedDirectEntity}
			if !reflect.DeepEqual(ids, want) {
				t.Fatalf("numeric gap removed independent hits: %v, want %v", ids, want)
			}
			return
		}
		continuation = page.Continuation
	}
	t.Fatal("numeric partial cursor made no terminal progress")
}

func TestP3ConnectedForeignCallerCannotReuseOrReleaseContext(t *testing.T) {
	f := newConnectedFixture(t, true, true)
	first := f.queryB(1)
	foreign := f.callers()
	foreign[f.b.AuthorityRef()] = f.actorA
	page, err := f.b.Connected().QueryRegionPath(context.Background(), foreign, f.queryFromB(), nil, 1, first.Continuation)
	assertConnectedQueryError(t, page, err, corecontract.ErrInvalid)
	if err := f.b.Connected().ReleaseConnectedCut(context.Background(), foreign, first.Cut); !errors.Is(err, corecontract.ErrInvalid) {
		t.Fatalf("foreign caller released context: %v", err)
	}
	if _, err := f.b.Connected().QueryRegionPath(context.Background(), f.callers(), f.queryFromB(), nil, 1, first.Continuation); err != nil {
		t.Fatalf("foreign release invalidated owner's context: %v", err)
	}
}
