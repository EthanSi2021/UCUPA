package acceptance_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"testing"

	"ucupa/composition"
	"ucupa/shared/values"
	corecontract "ucupa/ucupa-core/contract"
	facetcontract "ucupa/ucupa-facet/contract"
	realm "ucupa/ucupa-realm/contract"
	realminstance "ucupa/ucupa-realm/instance"
)

const (
	localRealmID values.RealmID   = "realm-acceptance"
	rootID       values.ElementID = "00000000-0000-4000-8000-000000002001"
	siteID       values.ElementID = "00000000-0000-4000-8000-000000002002"
	entityID     values.ElementID = "00000000-0000-4000-8000-000000002003"
)

var (
	localOperator     = values.PrincipalRef{Realm: localRealmID, Principal: "operator"}
	localFacetService = values.PrincipalRef{Realm: localRealmID, Principal: "facet-service"}
)

func TestG1OwnerLocalPublicProof(t *testing.T) {
	key := values.FacetKey{Element: entityID, Type: fixtureType("acceptance.profile")}
	policy, err := realminstance.New(realm.Seed{ID: localRealmID, LocalPrincipals: []values.PrincipalID{localOperator.Principal, localFacetService.Principal}, Bindings: []realm.PermissionBinding{
		binding(localOperator, realm.PermissionPutBinding, values.RealmScope(localRealmID)),
	}})
	if err != nil {
		t.Fatalf("New Realm: %v", err)
	}
	cluster, err := composition.NewCluster(composition.Seed{
		ID: "cluster-acceptance", RealmNextHop: "local:realm-acceptance", FacetServiceActor: localFacetService, CoreServiceActor: localFacetService, CoreNextHop: "local:core", FacetNextHop: "local:facet", Realm: policy,
	})
	if err != nil {
		t.Fatalf("NewCluster: %v", err)
	}
	owner := cluster.Core().AuthorityRef()
	grants := []realm.PermissionBinding{
		binding(localOperator, facetcontract.PermissionPut, values.FacetKeyScope(cluster.Facets().AuthorityRef(), key.Element, key.Type)),
		binding(localOperator, facetcontract.PermissionGet, values.FacetKeyScope(cluster.Facets().AuthorityRef(), key.Element, key.Type)),
		binding(localOperator, corecontract.PermissionFacetPropose, values.CoreElementScope(owner, key.Element)),
		binding(localOperator, corecontract.PermissionFacetActivate, values.CoreElementScope(owner, key.Element)),
		binding(localOperator, facetcontract.PermissionAccept, values.FacetElementScope(cluster.Facets().AuthorityRef(), key.Element)),
		binding(localFacetService, facetcontract.PermissionReadAcceptance, values.FacetElementScope(cluster.Facets().AuthorityRef(), key.Element)),
		binding(localOperator, corecontract.PermissionCreateRoot, values.CoreElementScope(owner, rootID)),
		binding(localOperator, corecontract.PermissionCreateAnchor, values.CoreElementScope(owner, rootID)),
		binding(localOperator, corecontract.PermissionCreateEntity, values.CoreElementScope(owner, siteID)),
		binding(localOperator, corecontract.PermissionReadElement, values.CoreElementScope(owner, rootID)),
		binding(localOperator, corecontract.PermissionReadElement, values.CoreElementScope(owner, siteID)),
		binding(localOperator, corecontract.PermissionReadElement, values.CoreElementScope(owner, entityID)),
		binding(localOperator, corecontract.PermissionDeleteEntity, values.CoreElementScope(owner, entityID)),
		binding(localFacetService, corecontract.PermissionFacetProposalRead, values.CoreElementScope(owner, entityID)), binding(localFacetService, corecontract.PermissionFacetAttachmentRead, values.CoreElementScope(owner, entityID)),
	}
	for index, grant := range grants {
		if _, err := policy.PutBinding(localOperator, grant, policy.BindingSetRevision(), realm.OperationID(fmt.Sprintf("acceptance-bootstrap-grant-%d", index))); err != nil {
			t.Fatalf("PutBinding: %v", err)
		}
	}
	rotation := mustRotation(t, 1, 0, 0, 0)
	anchorPose := mustPose(t, corecontract.Vec3{X: 10}, rotation)
	entityPose := mustPose(t, corecontract.Vec3{X: 2}, rotation)
	anchorRange := mustSphere(t, 10)
	entityRange := mustSphere(t, 1)
	if _, err := cluster.Core().Elements().CreateRoot(localOperator, rootID, "acceptance-create-root"); err != nil {
		t.Fatal(err)
	}
	if _, err := cluster.Core().Elements().CreateAnchor(localOperator, siteID, corecontract.SpatialTuple{Parent: rootID, LocalPose: anchorPose, Range: anchorRange}, "acceptance-create-site"); err != nil {
		t.Fatal(err)
	}
	created, err := cluster.Core().Elements().CreateEntity(localOperator, entityID, corecontract.SpatialTuple{Parent: siteID, LocalPose: entityPose, Range: entityRange}, "acceptance-create-entity")
	if err != nil || created.Record == nil || created.Record.Entity == nil {
		t.Fatalf("CreateEntity: %#v %v", created, err)
	}
	access := attachFixture(t, cluster.Core(), cluster.Facets(), localOperator, entityID)
	payload := fixturePayload("owner-local")
	if _, err := cluster.Facets().Namespace.Put(context.Background(), localOperator, access, key.Type, payload, facetcontract.ExpectAbsent(), "acceptance-put-facet"); err != nil {
		t.Fatal(err)
	}
	projection, err := cluster.Core().Elements().ProjectToAncestor(localOperator, entityID, rootID, nil)
	if err != nil || projection.Pose.Translation != (corecontract.Vec3{X: 12}) {
		t.Fatalf("projection: %#v %v", projection, err)
	}
	if got, err := cluster.Facets().Namespace.Get(context.Background(), localOperator, access, key.Type); err != nil || !bytes.Equal(got.Payload, payload) {
		t.Fatalf("Facet Get: %q %v", got, err)
	}
	if _, err := cluster.Core().Elements().DeleteEntity(localOperator, entityID, created.Record.Entity.Revision, "acceptance-delete-entity"); err != nil {
		t.Fatal(err)
	}
	if _, err := cluster.Facets().Namespace.Get(context.Background(), localOperator, access, key.Type); !errors.Is(err, facetcontract.ErrElementNotFound) {
		t.Fatalf("Facet after deletion: %v", err)
	}
}

func binding(subject values.PrincipalRef, permission values.PermissionID, scope values.OperationScope) realm.PermissionBinding {
	return realm.PermissionBinding{Subject: subject, Permission: permission, Scope: scope}
}

func mustRotation(t *testing.T, w, x, y, z float64) corecontract.Rotation {
	t.Helper()
	rotation, err := corecontract.NewRotation(w, x, y, z)
	if err != nil {
		t.Fatal(err)
	}
	return rotation
}

func mustPose(t *testing.T, translation corecontract.Vec3, rotation corecontract.Rotation) corecontract.Pose {
	t.Helper()
	pose, err := corecontract.NewPose(translation, rotation)
	if err != nil {
		t.Fatal(err)
	}
	return pose
}

func mustSphere(t *testing.T, radius float64) corecontract.BoundedSphere {
	t.Helper()
	sphere, err := corecontract.NewBoundedSphere(radius)
	if err != nil {
		t.Fatal(err)
	}
	return sphere
}
