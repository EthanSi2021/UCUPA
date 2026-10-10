package composition_test

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"

	"ucupa/composition"
	"ucupa/shared/values"
	corecontract "ucupa/ucupa-core/contract"
	core "ucupa/ucupa-core/instance"
	facetcontract "ucupa/ucupa-facet/contract"
	facet "ucupa/ucupa-facet/instance"
	realm "ucupa/ucupa-realm/contract"
	realminstance "ucupa/ucupa-realm/instance"
)

var (
	admin        = values.PrincipalRef{Realm: "realm-p", Principal: "admin"}
	guest        = values.PrincipalRef{Realm: "realm-p", Principal: "guest"}
	facetService = values.PrincipalRef{Realm: "realm-p", Principal: "facet-service"}
)

const (
	rootID        values.ElementID = "00000000-0000-4000-8000-000000001001"
	siteID        values.ElementID = "00000000-0000-4000-8000-000000001002"
	destinationID values.ElementID = "00000000-0000-4000-8000-000000001003"
	entityID      values.ElementID = "00000000-0000-4000-8000-000000001004"
)

var profileType = fixtureType("demo.profile")

func TestCorePermissionIDsRemainExactAndUnique(t *testing.T) {
	want := map[values.PermissionID]string{
		corecontract.PermissionCreateRoot: "ucupa.core.anchor.create-root", corecontract.PermissionCreateAnchor: "ucupa.core.anchor.create",
		corecontract.PermissionCreateEntity: "ucupa.core.entity.create", corecontract.PermissionReadElement: "ucupa.core.element.read",
		corecontract.PermissionListDirect: "ucupa.core.anchor.list-direct", corecontract.PermissionUpdateElement: "ucupa.core.element.update",
		corecontract.PermissionPlaceElement: "ucupa.core.element.place", corecontract.PermissionDeleteAnchor: "ucupa.core.anchor.delete",
		corecontract.PermissionDeleteEntity: "ucupa.core.entity.delete",
	}
	if len(want) != 9 {
		t.Fatalf("Core permission IDs are not unique: %#v", want)
	}
	for permission, exact := range want {
		if string(permission) != exact {
			t.Fatalf("permission %q: got %q", exact, permission)
		}
	}
}

func TestLocalClusterVerticalSlice(t *testing.T) {
	key := values.FacetKey{Element: entityID, Type: profileType}
	cluster := mustCluster(t, key, true)
	if cluster.ID() != "cluster-p" || cluster.RealmID() != "realm-p" {
		t.Fatalf("Cluster identity: got %q/%q", cluster.ID(), cluster.RealmID())
	}
	spatial, facets := cluster.Core(), cluster.Facets()
	identity, quarterTurn := mustRotation(t, 1, 0, 0, 0), mustRotation(t, 1, 0, 0, 1)
	siteRange, destinationRange, entityRange := mustSphere(t, 20), mustSphere(t, 10), mustSphere(t, 1)
	if _, err := spatial.Elements().CreateRoot(admin, rootID, "composition-core-create-root"); err != nil {
		t.Fatalf("CreateRoot: %v", err)
	}
	if _, err := spatial.Elements().CreateAnchor(admin, siteID, corecontract.SpatialTuple{Parent: rootID, LocalPose: mustPose(t, corecontract.Vec3{X: 10}, identity), Range: siteRange}, "composition-core-create-site"); err != nil {
		t.Fatalf("CreateAnchor(site): %v", err)
	}
	if _, err := spatial.Elements().CreateAnchor(admin, destinationID, corecontract.SpatialTuple{Parent: rootID, LocalPose: mustPose(t, corecontract.Vec3{X: 5}, quarterTurn), Range: destinationRange}, "composition-core-create-destination"); err != nil {
		t.Fatalf("CreateAnchor(destination): %v", err)
	}
	created, err := spatial.Elements().CreateEntity(admin, entityID, corecontract.SpatialTuple{Parent: siteID, LocalPose: mustPose(t, corecontract.Vec3{X: 3}, identity), Range: entityRange}, "composition-core-create-entity")
	if err != nil || created.Record == nil || created.Record.Entity == nil {
		t.Fatalf("CreateEntity: result=%#v err=%v", created, err)
	}
	access := attachFixture(t, spatial, facets, admin, entityID)
	payload := fixturePayload("profile")
	revision, err := facets.Namespace.Put(context.Background(), admin, access, key.Type, payload, facetcontract.ExpectAbsent(), "composition-profile-create")
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
	assertPayload(t, mustGetFacet(t, facets, admin, access, key.Type), payload)
	initial := mustProjection(t, spatial, admin, entityID, rootID)
	assertPose(t, initial.Pose, mustPose(t, corecontract.Vec3{X: 13}, identity))
	if _, err := facets.Namespace.Put(context.Background(), guest, access, key.Type, fixturePayload("unauthorized-overwrite"), facetcontract.ExpectRevision(revision.Revision), "composition-profile-denied"); !errors.Is(err, facetcontract.ErrDenied) {
		t.Fatalf("denied Facet overwrite: got %v", err)
	}
	final := corecontract.SpatialTuple{Parent: destinationID, LocalPose: mustPose(t, corecontract.Vec3{Y: 2}, identity), Range: entityRange}
	if _, err := spatial.Elements().UpdateEntity(guest, entityID, final, created.Record.Entity.Revision, "composition-core-update-denied"); !errors.Is(err, corecontract.ErrDenied) {
		t.Fatalf("denied update: got %v", err)
	}
	unchanged, err := spatial.Elements().ReadElement(admin, entityID, nil)
	if err != nil || unchanged.Record == nil || !reflect.DeepEqual(*unchanged.Record, *created.Record) {
		t.Fatalf("denied update changed Entity: read=%#v err=%v", unchanged, err)
	}
	moved, err := spatial.Elements().UpdateEntity(admin, entityID, final, created.Record.Entity.Revision, "composition-core-update")
	if err != nil || moved.Record == nil || moved.Record.Entity == nil {
		t.Fatalf("UpdateEntity: result=%#v err=%v", moved, err)
	}
	if moved.Record.Entity.ID != entityID || moved.Record.Entity.ContainingAnchorID != destinationID || !reflect.DeepEqual(moved.Record.Entity.LocalPose, final.LocalPose) {
		t.Fatalf("update changed identity or placement unexpectedly: %#v", moved)
	}
	assertPose(t, mustProjection(t, spatial, admin, entityID, rootID).Pose, mustPose(t, corecontract.Vec3{X: 3}, quarterTurn))
	assertPayload(t, mustGetFacet(t, facets, admin, access, key.Type), payload)
	if _, err := spatial.Elements().DeleteEntity(admin, entityID, moved.Record.Entity.Revision, "composition-core-delete"); err != nil {
		t.Fatalf("DeleteEntity: %v", err)
	}
	if _, err := facets.Namespace.Get(context.Background(), admin, access, key.Type); !errors.Is(err, facetcontract.ErrElementNotFound) {
		t.Fatalf("Facet remained visible after Core deletion: got %v", err)
	}
}

func TestFacetServiceActorFailsClosedWithoutExactReadGrant(t *testing.T) {
	key := values.FacetKey{Element: entityID, Type: profileType}
	cluster := mustCluster(t, key, false)
	identity := mustRotation(t, 1, 0, 0, 0)
	if _, err := cluster.Core().Elements().CreateRoot(admin, rootID, "composition-unavailable-root"); err != nil {
		t.Fatal(err)
	}
	if _, err := cluster.Core().Elements().CreateEntity(admin, entityID, corecontract.SpatialTuple{Parent: rootID, LocalPose: mustPose(t, corecontract.Vec3{}, identity), Range: mustSphere(t, 1)}, "composition-unavailable-entity"); err != nil {
		t.Fatal(err)
	}
	read, err := cluster.Core().Elements().ReadElement(admin, entityID, nil)
	if err != nil {
		t.Fatal(err)
	}
	proposal, err := cluster.Core().Facets().ProposeFacet(admin, entityID, cluster.Facets().AuthorityRef(), "local:facet", read.Attachment.Revision, "proposal")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := cluster.Facets().Namespace.AcceptAttachment(context.Background(), admin, proposal.Pending, "local:core", "accept"); !errors.Is(err, facetcontract.ErrUnavailable) {
		t.Fatalf("Facet Put without service read grant: got %v", err)
	}
}

func TestNewClusterDoesNotInventOrTransferCoreGrants(t *testing.T) {
	policy, err := realminstance.New(realm.Seed{
		ID: admin.Realm, LocalPrincipals: []values.PrincipalID{admin.Principal, facetService.Principal},
		Bindings: []realm.PermissionBinding{binding(admin, realm.PermissionPutBinding, values.RealmScope(admin.Realm))},
	})
	if err != nil {
		t.Fatal(err)
	}
	seed := composition.Seed{ID: "cluster-p", Realm: policy, RealmNextHop: "local:realm", FacetServiceActor: facetService, CoreServiceActor: facetService, CoreNextHop: "local:core", FacetNextHop: "local:facet"}
	first, err := composition.NewCluster(seed)
	if err != nil {
		t.Fatal(err)
	}
	second, err := composition.NewCluster(seed)
	if err != nil {
		t.Fatal(err)
	}
	if !first.Core().AuthorityRef().Valid() || first.Core().AuthorityRef() == second.Core().AuthorityRef() {
		t.Fatal("independently created Core identities must be valid and distinct")
	}
	for _, cluster := range []*composition.Cluster{first, second} {
		if _, err := cluster.Core().Elements().CreateRoot(admin, rootID, "before-grant"); !errors.Is(err, corecontract.ErrDenied) {
			t.Fatalf("new Core inferred a grant: %v", err)
		}
	}
	grant := binding(admin, corecontract.PermissionCreateRoot, values.CoreElementScope(first.Core().AuthorityRef(), rootID))
	if _, err := policy.PutBinding(admin, grant, policy.BindingSetRevision(), "grant-first-core"); err != nil {
		t.Fatal(err)
	}
	if _, err := first.Core().Elements().CreateRoot(admin, rootID, "create-first-root"); err != nil {
		t.Fatalf("explicit grant did not authorize its Core: %v", err)
	}
	if _, err := second.Core().Elements().CreateRoot(admin, rootID, "create-second-root"); !errors.Is(err, corecontract.ErrDenied) {
		t.Fatalf("grant leaked to another Core in same Realm/Cluster: %v", err)
	}
}

func TestNewClusterRejectsInvalidSeeds(t *testing.T) {
	policy, err := realminstance.New(realm.Seed{ID: "realm-p"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := composition.NewCluster(composition.Seed{Realm: policy, RealmNextHop: "local:realm", FacetServiceActor: facetService, CoreServiceActor: facetService, CoreNextHop: "local:core", FacetNextHop: "local:facet"}); !errors.Is(err, composition.ErrInvalidSeed) {
		t.Fatalf("empty Cluster ID: got %v", err)
	}
	if _, err := composition.NewCluster(composition.Seed{ID: "cluster-p", Realm: policy, FacetServiceActor: facetService, CoreServiceActor: facetService, CoreNextHop: "local:core", FacetNextHop: "local:facet"}); !errors.Is(err, composition.ErrInvalidSeed) {
		t.Fatalf("missing Realm next hop: got %v", err)
	}
	if _, err := composition.NewCluster(composition.Seed{ID: "cluster-p", Realm: policy, RealmNextHop: "remote:realm", FacetServiceActor: facetService, CoreServiceActor: facetService, CoreNextHop: "local:core", FacetNextHop: "local:facet"}); !errors.Is(err, composition.ErrInvalidSeed) {
		t.Fatalf("non-local Realm next hop: got %v", err)
	}
	if _, err := composition.NewCluster(composition.Seed{ID: "cluster-p", Realm: policy, RealmNextHop: "local:realm", CoreServiceActor: facetService, CoreNextHop: "local:core", FacetNextHop: "local:facet"}); !errors.Is(err, composition.ErrInvalidSeed) {
		t.Fatalf("empty Facet service actor: got %v", err)
	}
	if _, err := composition.NewCluster(composition.Seed{ID: "cluster-p", RealmNextHop: "local:realm", FacetServiceActor: facetService, CoreServiceActor: facetService, CoreNextHop: "local:core", FacetNextHop: "local:facet"}); !errors.Is(err, composition.ErrInvalidSeed) {
		t.Fatalf("missing Realm owner: got %v", err)
	}
	if _, err := composition.NewCluster(composition.Seed{ID: "cluster-p", Realm: &realminstance.Realm{}, RealmNextHop: "local:realm", FacetServiceActor: facetService, CoreServiceActor: facetService, CoreNextHop: "local:core", FacetNextHop: "local:facet"}); !errors.Is(err, composition.ErrInvalidSeed) {
		t.Fatalf("zero Realm owner: got %v", err)
	}
}

func mustCluster(t *testing.T, key values.FacetKey, grantServiceRead bool) *composition.Cluster {
	t.Helper()
	policy, err := realminstance.New(realm.Seed{
		ID: "realm-p", LocalPrincipals: []values.PrincipalID{admin.Principal, guest.Principal, facetService.Principal},
		Bindings: []realm.PermissionBinding{
			binding(admin, realm.PermissionPutBinding, values.RealmScope(admin.Realm)),
		},
	})
	if err != nil {
		t.Fatalf("New Realm: %v", err)
	}
	cluster, err := composition.NewCluster(composition.Seed{ID: "cluster-p", RealmNextHop: "local:realm", FacetServiceActor: facetService, CoreServiceActor: facetService, CoreNextHop: "local:core", FacetNextHop: "local:facet", Realm: policy})
	if err != nil {
		t.Fatalf("NewCluster: %v", err)
	}
	owner := cluster.Core().AuthorityRef()
	bindings := []realm.PermissionBinding{
		binding(admin, facetcontract.PermissionPut, values.FacetKeyScope(cluster.Facets().AuthorityRef(), key.Element, key.Type)),
		binding(admin, facetcontract.PermissionGet, values.FacetKeyScope(cluster.Facets().AuthorityRef(), key.Element, key.Type)),
		binding(admin, corecontract.PermissionFacetPropose, values.CoreElementScope(owner, key.Element)),
		binding(admin, corecontract.PermissionFacetActivate, values.CoreElementScope(owner, key.Element)),
		binding(admin, facetcontract.PermissionAccept, values.FacetElementScope(cluster.Facets().AuthorityRef(), key.Element)),
		binding(facetService, facetcontract.PermissionReadAcceptance, values.FacetElementScope(cluster.Facets().AuthorityRef(), key.Element)),
		binding(admin, corecontract.PermissionCreateRoot, values.CoreElementScope(owner, rootID)), binding(admin, corecontract.PermissionCreateAnchor, values.CoreElementScope(owner, rootID)),
		binding(admin, corecontract.PermissionCreateEntity, values.CoreElementScope(owner, rootID)), binding(admin, corecontract.PermissionCreateEntity, values.CoreElementScope(owner, siteID)),
		binding(admin, corecontract.PermissionReadElement, values.CoreElementScope(owner, rootID)), binding(admin, corecontract.PermissionReadElement, values.CoreElementScope(owner, siteID)),
		binding(admin, corecontract.PermissionReadElement, values.CoreElementScope(owner, destinationID)), binding(admin, corecontract.PermissionReadElement, values.CoreElementScope(owner, entityID)),
		binding(admin, corecontract.PermissionUpdateElement, values.CoreElementScope(owner, entityID)), binding(admin, corecontract.PermissionPlaceElement, values.CoreElementScope(owner, destinationID)),
		binding(admin, corecontract.PermissionDeleteEntity, values.CoreElementScope(owner, entityID)),
	}
	if grantServiceRead {
		bindings = append(bindings, binding(facetService, corecontract.PermissionFacetProposalRead, values.CoreElementScope(owner, entityID)), binding(facetService, corecontract.PermissionFacetAttachmentRead, values.CoreElementScope(owner, entityID)))
	}
	for index, grant := range bindings {
		if _, err := policy.PutBinding(admin, grant, policy.BindingSetRevision(), realm.OperationID(fmt.Sprintf("composition-bootstrap-grant-%d", index))); err != nil {
			t.Fatalf("PutBinding: %v", err)
		}
	}
	return cluster
}

func binding(actor values.PrincipalRef, permission values.PermissionID, scope values.OperationScope) realm.PermissionBinding {
	return realm.PermissionBinding{Subject: actor, Permission: permission, Scope: scope}
}
func mustRotation(t *testing.T, w, x, y, z float64) corecontract.Rotation {
	t.Helper()
	value, err := corecontract.NewRotation(w, x, y, z)
	if err != nil {
		t.Fatal(err)
	}
	return value
}
func mustPose(t *testing.T, translation corecontract.Vec3, rotation corecontract.Rotation) corecontract.Pose {
	t.Helper()
	value, err := corecontract.NewPose(translation, rotation)
	if err != nil {
		t.Fatal(err)
	}
	return value
}
func mustSphere(t *testing.T, radius float64) corecontract.BoundedSphere {
	t.Helper()
	value, err := corecontract.NewBoundedSphere(radius)
	if err != nil {
		t.Fatal(err)
	}
	return value
}
func mustProjection(t *testing.T, spatial *core.Core, actor values.PrincipalRef, subject, ancestor values.ElementID) corecontract.Projection {
	t.Helper()
	value, err := spatial.Elements().ProjectToAncestor(actor, subject, ancestor, nil)
	if err != nil {
		t.Fatal(err)
	}
	return value
}
func mustGetFacet(t *testing.T, facets *facet.Instance, actor values.PrincipalRef, access values.FacetAcceptanceKey, typ values.FacetTypeRef) []byte {
	t.Helper()
	read, err := facets.Namespace.Get(context.Background(), actor, access, typ)
	if err != nil {
		t.Fatal(err)
	}
	return read.Payload
}
func assertPayload(t *testing.T, got, want []byte) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Facet payload: got %q, want %q", got, want)
	}
}
func assertPose(t *testing.T, got, want corecontract.Pose) {
	t.Helper()
	if got.Translation != want.Translation || got.Rotation.Pivot() != want.Rotation.Pivot() || got.Rotation.Ratios() != want.Rotation.Ratios() {
		t.Fatalf("Pose mismatch: got %#v, want %#v", got, want)
	}
}
