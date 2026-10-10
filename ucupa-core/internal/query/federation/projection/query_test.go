package projection

import (
	"context"
	"fmt"
	"reflect"
	"testing"

	"ucupa/shared/values"
	corecontract "ucupa/ucupa-core/contract"
	elementfact "ucupa/ucupa-core/internal/element/fact"
	facetfact "ucupa/ucupa-core/internal/facet/fact"
	"ucupa/ucupa-core/internal/geometry"
	mountfact "ucupa/ucupa-core/internal/mount/fact"
	federationvalue "ucupa/ucupa-core/internal/query/federation/value"
	coretoken "ucupa/ucupa-core/internal/token"
	transferfact "ucupa/ucupa-core/internal/transfer/fact"
)

func queryID(n int) values.ElementID {
	return values.ElementID(fmt.Sprintf("00000000-0000-4000-8000-%012d", n))
}

func queryPose(t *testing.T, x float64) corecontract.Pose {
	t.Helper()
	rotation, err := corecontract.NewRotation(1, 0, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	pose, err := corecontract.NewPose(corecontract.Vec3{X: x}, rotation)
	if err != nil {
		t.Fatal(err)
	}
	return pose
}

func querySphere(t *testing.T, radius float64) corecontract.BoundedSphere {
	t.Helper()
	sphere, err := corecontract.NewBoundedSphere(radius)
	if err != nil {
		t.Fatal(err)
	}
	return sphere
}

type queryFixture struct {
	elements elementfact.State
	mounts   mountfact.State
	signer   coretoken.PublicSigner
	root     values.ElementID
}

func newQueryFixture(t *testing.T) *queryFixture {
	t.Helper()
	owner := values.FactAuthorityRef(queryID(900001))
	return &queryFixture{elements: elementfact.New(), mounts: mountfact.New(), signer: coretoken.NewPublicSigner([32]byte{9}, owner), root: queryID(1)}
}

func (f *queryFixture) anchor(t *testing.T, id, parent values.ElementID, x, radius float64) {
	t.Helper()
	rangeValue := geometry.BoundedRange(querySphere(t, radius))
	if parent == "" {
		rangeValue = geometry.UnboundedRootRange()
	}
	if err := f.elements.RestoreAnchor(elementfact.Anchor{ID: id, Parent: parent, LocalPose: queryPose(t, x), Range: rangeValue, Revision: coretoken.RevisionFromParts([32]byte{1}, id, uint64(len(id)))}); err != nil {
		t.Fatal(err)
	}
}

func (f *queryFixture) entity(t *testing.T, id, parent values.ElementID, x, radius float64) {
	t.Helper()
	if err := f.elements.RestoreEntity(elementfact.Entity{ID: id, Parent: parent, LocalPose: queryPose(t, x), Range: querySphere(t, radius), Revision: coretoken.RevisionFromParts([32]byte{1}, id, uint64(len(id)))}); err != nil {
		t.Fatal(err)
	}
}

func (f *queryFixture) projection() *Projection {
	attachments := facetfact.NewProjectionBuilder()
	return New(f.elements.View, f.mounts.View, attachments.Build().View, transferfact.Visibility{}, f.signer)
}

func queryRequest(scope values.ElementID, radius float64) federationvalue.FederationProjectionRequest {
	return federationvalue.FederationProjectionRequest{Kind: "query", Query: &federationvalue.FederationQueryRequest{
		Scope: scope, Sphere: corecontract.QuerySphere{Radius: radius}, Selector: corecontract.QueryEntities,
	}}
}

func queryMatchIDs(page federationvalue.FederationProjectionPage) []values.ElementID {
	ids := make([]values.ElementID, 0, len(page.Matches))
	for _, match := range page.Matches {
		ids = append(ids, match.ID)
	}
	return ids
}

func queryParentIDs(page federationvalue.FederationProjectionPage) []values.ElementID {
	ids := make([]values.ElementID, 0, len(page.Parents))
	for _, parent := range page.Parents {
		ids = append(ids, parent.Parent)
	}
	return ids
}

func TestProjectQueryPaginatesStableIDsAndDisclosesOnlyMatchedParents(t *testing.T) {
	f := newQueryFixture(t)
	near, far := queryID(2), queryID(3)
	first, second, missed := queryID(10), queryID(11), queryID(12)
	f.anchor(t, f.root, "", 0, 0)
	f.anchor(t, near, f.root, 0, 1)
	f.anchor(t, far, f.root, 100, 1)
	f.entity(t, second, near, 1, 0)
	f.entity(t, first, near, 0, 0)
	f.entity(t, missed, far, 0, 0)
	p := f.projection()
	r := queryRequest(f.root, 5)

	page, last, deps, err := p.Project(context.Background(), f.root, r, "", 1)
	if err != nil || page.Complete || page.Gap != corecontract.QueryNoGap || last != first || len(deps) != 0 ||
		!reflect.DeepEqual(queryMatchIDs(page), []values.ElementID{first}) ||
		!reflect.DeepEqual(queryParentIDs(page), []values.ElementID{f.root, near}) {
		t.Fatalf("first page mismatch: page=%#v last=%q deps=%#v err=%v", page, last, deps, err)
	}
	for _, parent := range page.Parents {
		if parent.Adjacency == ([32]byte{}) {
			t.Fatal("visible parent lacks adjacency evidence")
		}
	}
	next, nextLast, nextDeps, err := p.Project(context.Background(), f.root, r, last, 1)
	if err != nil || !next.Complete || next.Gap != corecontract.QueryNoGap || nextLast != "" || len(nextDeps) != 0 ||
		!reflect.DeepEqual(queryMatchIDs(next), []values.ElementID{second}) ||
		!reflect.DeepEqual(queryParentIDs(next), []values.ElementID{f.root, near}) {
		t.Fatalf("second page mismatch: page=%#v last=%q deps=%#v err=%v", next, nextLast, nextDeps, err)
	}
	if reflect.DeepEqual(page.Parents, nil) || reflect.DeepEqual(next.Parents, nil) ||
		queryParentIDs(page)[1] == far || queryParentIDs(next)[1] == far {
		t.Fatal("unmatched sibling branch leaked into parent evidence")
	}
}

func (f *queryFixture) mountRecord(t *testing.T, id values.ElementID, state corecontract.MountState, slot corecontract.OperationID) mountfact.Record {
	t.Helper()
	parent, carrier := values.FactAuthorityRef(queryID(900001)), values.FactAuthorityRef(queryID(900002))
	key := coretoken.MountKey{ParentAuthority: parent, MountAnchorUUID: id, CarrierAuthority: carrier,
		ProposalRevision: coretoken.MountRevisionFromParts([32]byte{2}, id, 1)}
	return mountfact.Record{State: state, Key: key, Revision: coretoken.MountRevisionFromParts([32]byte{2}, id, 2),
		Admission: corecontract.MountAdmissionKey{MountKey: key, OperationID: "admit"}, Slot: slot}
}

func TestProjectQueryMountPrunedUnresolvedAndFollowed(t *testing.T) {
	for _, tc := range []struct {
		name       string
		x          float64
		record     func(*queryFixture, *testing.T, values.ElementID) mountfact.Record
		gap        corecontract.QueryGap
		frontiers  int
		dependency bool
		complete   bool
	}{
		{name: "range outside is pruned", x: 100, record: func(f *queryFixture, t *testing.T, id values.ElementID) mountfact.Record {
			return f.mountRecord(t, id, corecontract.MountEstablishing, "busy")
		}, complete: true},
		{name: "intersecting unavailable is unresolved", x: 0, record: func(f *queryFixture, t *testing.T, id values.ElementID) mountfact.Record {
			return f.mountRecord(t, id, corecontract.MountEstablishing, "busy")
		}, gap: corecontract.QueryUnresolvedCoverage},
		{name: "intersecting active is followed", x: 0, record: func(f *queryFixture, t *testing.T, id values.ElementID) mountfact.Record {
			return f.mountRecord(t, id, corecontract.MountActive, "")
		}, frontiers: 1, dependency: true, complete: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newQueryFixture(t)
			mountID := queryID(20)
			f.anchor(t, f.root, "", 0, 0)
			f.anchor(t, mountID, f.root, tc.x, 2)
			record := tc.record(f, t, mountID)
			f.mounts.ReplaceOutgoing(mountID, record)
			page, last, deps, err := f.projection().Project(context.Background(), f.root, queryRequest(f.root, 5), "", 16)
			if err != nil || last != "" || page.Gap != tc.gap || page.Complete != tc.complete || len(page.Frontiers) != tc.frontiers {
				t.Fatalf("projection mismatch: page=%#v last=%q deps=%#v err=%v", page, last, deps, err)
			}
			got, exists := deps[mountID]
			if exists != tc.dependency || exists && got != record || len(deps) != btoi(tc.dependency) {
				t.Fatalf("dependency mismatch: %#v", deps)
			}
			if tc.frontiers == 1 && (page.Frontiers[0].Relation.Key != record.Key || !reflect.DeepEqual(page.Frontiers[0].Path, []values.ElementID{mountID})) {
				t.Fatalf("frontier mismatch: %#v", page.Frontiers[0])
			}
		})
	}
}

func btoi(value bool) int {
	if value {
		return 1
	}
	return 0
}

func TestProjectQueryRepresentationalLimitBecomesGap(t *testing.T) {
	f := newQueryFixture(t)
	f.anchor(t, f.root, "", 0, 0)
	parent := f.root
	for i := 0; i < 1030; i++ {
		id := queryID(1000 + i)
		f.anchor(t, id, parent, 1, 1)
		parent = id
	}
	entity := queryID(3000)
	f.entity(t, entity, parent, 0, 0)
	page, last, deps, err := f.projection().Project(context.Background(), f.root, queryRequest(f.root, 5000), "", 16)
	if err != nil || last != "" || page.Gap != corecontract.QueryExactWorkLimit || page.Complete || len(deps) != 0 || len(page.Matches) != 0 {
		t.Fatalf("representational limit mismatch: page=%#v last=%q deps=%#v err=%v", page, last, deps, err)
	}
}
