package commit_test

import (
	"errors"
	"reflect"
	"testing"

	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	"ucupa/ucupa-core/internal/commit"
	queryoracle "ucupa/ucupa-core/internal/query/oracle"
	coretoken "ucupa/ucupa-core/internal/token"
)

func TestMountGuardLocalReadsDoNotInventRemoteCoverage(t *testing.T) {
	for _, stage := range []string{"establishing", "active", "preparing-range", "detached"} {
		t.Run(stage, func(t *testing.T) {
			f := newMountGuardFixture(t, stage != "establishing")
			a, b := f.parent, f.carrier
			localA := mountGuardID(9140)
			if _, err := a.elements.CreateAnchor(mountGuardActor, localA, mountGuardTuple(t, f.root, 20, 2), "local-a"); err != nil {
				t.Fatal(err)
			}
			if stage == "preparing-range" {
				f.prepareRange(t, "range-read-guard", 9)
			} else if stage == "detached" {
				f.detach(t)
			}
			aBefore, bBefore := commit.CaptureCompleteStateForTest(a.owner), commit.CaptureCompleteStateForTest(b.owner)
			for _, target := range []struct {
				owner *mountGuardOwner
				id    values.ElementID
			}{{a, f.mount}, {b, f.mount}} {
				page, err := target.owner.elements.ListDirect(mountGuardActor, target.id, 10, nil)
				if !errors.Is(err, contract.ErrMountContextRequired) || !reflect.DeepEqual(page, contract.DirectPage{}) {
					t.Fatalf("ListDirect external coverage: %#v, %v", page, err)
				}
				query := contract.RegionQuery{StartAnchorID: target.id, BasisAnchorID: target.id, Sphere: contract.QuerySphere{Radius: 0}, Selector: contract.QueryEntities}
				region, err := target.owner.queries.QueryRegion(mountGuardActor, query, nil, 10, nil)
				assertMountGuardQueryError(t, region, err, contract.ErrMountContextRequired)
			}
			// An ancestor Scope also requires the remote closure, even when only
			// entities are selected or the query sphere is far from the mount.
			query := contract.RegionQuery{StartAnchorID: f.root, BasisAnchorID: localA, Sphere: contract.QuerySphere{Center: contract.Vec3{X: 1e6}}, Selector: contract.QueryEntities}
			page, err := a.queries.QueryRegion(mountGuardActor, query, nil, 10, nil)
			assertMountGuardQueryError(t, page, err, contract.ErrMountContextRequired)
			scan, err := queryoracle.Scan(a.owner, f.root, localA, nil, contract.QuerySphere{}, queryoracle.Entities)
			if !errors.Is(err, contract.ErrMountContextRequired) || !reflect.DeepEqual(scan, queryoracle.Result{}) {
				t.Fatalf("oracle claimed unresolved coverage complete: %#v, %v", scan, err)
			}
			// Direct listing of A's parent lists its owned M record and is not
			// a claim about M's children. A different local branch is complete.
			direct, err := a.elements.ListDirect(mountGuardActor, f.root, 10, nil)
			if err != nil || len(direct.Records) != 2 {
				t.Fatalf("owner-local parent list: %#v, %v", direct, err)
			}
			query.StartAnchorID, query.BasisAnchorID = localA, localA
			page, err = a.queries.QueryRegion(mountGuardActor, query, nil, 10, nil)
			if err != nil || page.Status != contract.QueryComplete || len(page.Matches) != 0 {
				t.Fatalf("independent local closure: %#v, %v", page, err)
			}
			projection, err := a.elements.ProjectToAncestor(mountGuardActor, f.mount, f.root, nil)
			if err != nil || !coretoken.ValidRevision(projection.SubjectRevision) {
				t.Fatalf("owned M's local pose is still readable: %#v, %v", projection, err)
			}
			if !commit.MatchesCompleteStateForTest(a.owner, aBefore) || !commit.MatchesCompleteStateForTest(b.owner, bBefore) {
				t.Fatal("coverage refusal or local reads mutated a Core")
			}
		})
	}
}

func TestMountGuardCarrierLocalReadsRemainLocalWhenDetached(t *testing.T) {
	f := newMountGuardFixture(t, true)
	b := f.carrier
	local, inner, entity := mountGuardID(9150), mountGuardID(9151), mountGuardID(9152)
	if _, err := b.elements.CreateAnchor(mountGuardActor, local, mountGuardTuple(t, f.mount, 1, 5), "b-local"); err != nil {
		t.Fatal(err)
	}
	if _, err := b.elements.CreateAnchor(mountGuardActor, inner, mountGuardTuple(t, local, 2, 2), "b-inner"); err != nil {
		t.Fatal(err)
	}
	created, err := b.elements.CreateEntity(mountGuardActor, entity, mountGuardTuple(t, inner, 1, 1), "b-entity")
	if err != nil {
		t.Fatal(err)
	}
	for _, detached := range []bool{false, true} {
		if detached {
			f.detach(t)
		}
		before := commit.CaptureCompleteStateForTest(b.owner)
		projected, err := b.elements.ProjectToAncestor(mountGuardActor, entity, local, nil)
		if err != nil || projected.Pose.Translation.X != 3 || projected.SubjectRevision != created.Record.Entity.Revision {
			t.Fatalf("owned prefix projection (detached=%v): %#v, %v", detached, projected, err)
		}
		for _, pair := range [][2]values.ElementID{{entity, f.mount}, {f.mount, local}, {f.mount, f.mount}} {
			projected, err := b.elements.ProjectToAncestor(mountGuardActor, pair[0], pair[1], nil)
			if !errors.Is(err, contract.ErrMountContextRequired) || !reflect.DeepEqual(projected, contract.Projection{}) {
				t.Fatalf("external M projection: %#v, %v", projected, err)
			}
		}
		direct, err := b.elements.ListDirect(mountGuardActor, local, 10, nil)
		if err != nil || len(direct.Records) != 1 || direct.Records[0].Anchor.ID != inner {
			t.Fatalf("local B list: %#v, %v", direct, err)
		}
		query := contract.RegionQuery{StartAnchorID: local, BasisAnchorID: local, Sphere: contract.QuerySphere{Radius: 5}, Selector: contract.QueryEntities}
		page, err := b.queries.QueryRegion(mountGuardActor, query, nil, 10, nil)
		if err != nil || page.Status != contract.QueryComplete || len(page.Matches) != 1 || page.Matches[0].ElementID != entity {
			t.Fatalf("owned B closure: %#v, %v", page, err)
		}
		if !reflect.DeepEqual(page.Matches[0].Path, []values.ElementID{inner, entity}) {
			t.Fatalf("local path invented a root or external record: %v", page.Matches[0].Path)
		}
		query.BasisAnchorID = f.mount
		page, err = b.queries.QueryRegion(mountGuardActor, query, nil, 10, nil)
		assertMountGuardQueryError(t, page, err, contract.ErrInvalid) // Basis must remain inside this local Scope.
		if !commit.MatchesCompleteStateForTest(b.owner, before) {
			t.Fatal("local/external reads mutated B")
		}
	}
}

func TestMountGuardAuthorizationStillPrecedesGateAndContext(t *testing.T) {
	f := newMountGuardFixture(t, true)
	entity := mountGuardID(9160)
	tuple := mountGuardTuple(t, f.mount, 0, 1)
	if _, err := f.carrier.elements.CreateEntity(mountGuardActor, entity, tuple, "prior-receipt"); err != nil {
		t.Fatal(err)
	}
	f.prepareRange(t, "gate-denied", 9)
	b := f.carrier
	b.policy.setDenied(true)
	before := commit.CaptureCompleteStateForTest(b.owner)
	for _, id := range []contract.OperationID{"prior-receipt", "new-id"} {
		result, err := b.elements.CreateEntity(mountGuardActor, entity, tuple, id)
		if !errors.Is(err, contract.ErrDenied) || !reflect.DeepEqual(result, contract.MutationResult{}) {
			t.Fatalf("receipt or gate bypassed current authorization: %#v, %v", result, err)
		}
	}
	direct, err := b.elements.ListDirect(mountGuardActor, f.mount, 10, nil)
	if !errors.Is(err, contract.ErrDenied) || !reflect.DeepEqual(direct, contract.DirectPage{}) {
		t.Fatalf("list context disclosed before authorization: %#v, %v", direct, err)
	}
	projected, err := b.elements.ProjectToAncestor(mountGuardActor, entity, f.mount, nil)
	if !errors.Is(err, contract.ErrDenied) || !reflect.DeepEqual(projected, contract.Projection{}) {
		t.Fatalf("projection context disclosed before authorization: %#v, %v", projected, err)
	}
	page, err := b.queries.QueryRegion(mountGuardActor, contract.RegionQuery{StartAnchorID: f.mount, BasisAnchorID: f.mount, Selector: contract.QueryBoth}, nil, 10, nil)
	assertMountGuardQueryError(t, page, err, contract.ErrDenied)
	if !commit.MatchesCompleteStateForTest(b.owner, before) {
		t.Fatal("denied calls mutated B")
	}
}
