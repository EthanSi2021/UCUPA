package composition_test

import (
	"context"
	"errors"
	"fmt"
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

// Independent-Realm assembly: the Core trusts one Realm, the Facet another,
// and every authorization still flows through the owner's current contact.
var (
	coreRealmAdmin                       = values.PrincipalRef{Realm: "realm-core", Principal: "admin"}
	coreRealmFacetSvcs                   = values.PrincipalRef{Realm: "realm-core", Principal: "facet-service"}
	facetRealmAdmin                      = values.PrincipalRef{Realm: "realm-facet", Principal: "admin"}
	facetRealmCoreSvcs                   = values.PrincipalRef{Realm: "realm-facet", Principal: "core-service"}
	independentRootID   values.ElementID = "00000000-0000-4000-8000-000000002001"
	independentEntityID values.ElementID = "00000000-0000-4000-8000-000000002002"
)

type independentAssembly struct {
	spatial    *core.Core
	facets     *facet.Instance
	coreRealm  *realminstance.Realm
	facetRealm *realminstance.Realm
	coreConns  *composition.LocalRealmConnections
	facetConns *composition.LocalRealmConnections
}

// assembleIndependentRealms mirrors NewCluster but binds the Core and the
// Facet to two different Realms. Neither Realm can authorize the other side's
// facts: the service sessions cross the boundary through explicit caller
// proofs issued by the Realm that owns the target side.
func assembleIndependentRealms(t *testing.T) *independentAssembly {
	t.Helper()
	coreRealm, err := realminstance.New(realm.Seed{ID: coreRealmAdmin.Realm,
		LocalPrincipals: []values.PrincipalID{coreRealmAdmin.Principal, coreRealmFacetSvcs.Principal},
		Bindings:        []realm.PermissionBinding{{Subject: coreRealmAdmin, Permission: realm.PermissionPutBinding, Scope: values.RealmScope(coreRealmAdmin.Realm)}}})
	if err != nil {
		t.Fatalf("core Realm: %v", err)
	}
	facetRealm, err := realminstance.New(realm.Seed{ID: facetRealmAdmin.Realm,
		LocalPrincipals: []values.PrincipalID{facetRealmAdmin.Principal, facetRealmCoreSvcs.Principal},
		Bindings:        []realm.PermissionBinding{{Subject: facetRealmAdmin, Permission: realm.PermissionPutBinding, Scope: values.RealmScope(facetRealmAdmin.Realm)}}})
	if err != nil {
		t.Fatalf("facet Realm: %v", err)
	}
	coreConns := &composition.LocalRealmConnections{}
	if err = coreConns.BindEndpoint("local:core-realm", coreRealm); err != nil {
		t.Fatal(err)
	}
	spatial, err := core.New(coreConns, corecontract.RealmInitialization{Realm: coreRealm.ID(), NextHop: "local:core-realm"}, corecontract.Config{})
	if err != nil {
		t.Fatalf("core assembly: %v", err)
	}
	facetCore := &composition.LocalFacetCoreConnections{}
	if err = facetCore.BindEndpoint("local:core", composition.LocalFacetCoreEndpoint{Read: spatial.Facets(), Control: spatial.Facets(), Access: spatial.Access()}); err != nil {
		t.Fatal(err)
	}
	serviceCaller, err := coreConns.TrustLocalCaller("local:core-realm", coreRealm.ID(), coreRealmFacetSvcs)
	if err != nil {
		t.Fatal(err)
	}
	source, err := facetCore.ForFacetCaller(serviceCaller)
	if err != nil {
		t.Fatal(err)
	}
	facetConns := &composition.LocalRealmConnections{}
	if err = facetConns.BindEndpoint("local:facet-realm", facetRealm); err != nil {
		t.Fatal(err)
	}
	facets, err := facet.New(facetConns, source, facetcontract.RealmInitialization{Realm: facetRealm.ID(), NextHop: "local:facet-realm"}, facet.Config{})
	if err != nil {
		t.Fatalf("facet assembly: %v", err)
	}
	facetEndpoints := &composition.LocalFacetConnections{}
	if err = facetEndpoints.BindEndpoint("local:facet", facets); err != nil {
		t.Fatal(err)
	}
	acceptance, err := facetEndpoints.ForCore(facetRealmCoreSvcs)
	if err != nil {
		t.Fatal(err)
	}
	if err = spatial.Facets().SetFacetAcceptanceSource(acceptance); err != nil {
		t.Fatal(err)
	}
	coreCaller, err := facetConns.TrustLocalCaller("local:facet-realm", facetRealm.ID(), facetRealmCoreSvcs)
	if err != nil {
		t.Fatal(err)
	}
	disposition, err := facetEndpoints.ForCoreCaller(coreCaller)
	if err != nil {
		t.Fatal(err)
	}
	if err = spatial.Facets().SetFacetDispositionSource(disposition); err != nil {
		t.Fatal(err)
	}
	return &independentAssembly{spatial: spatial, facets: facets, coreRealm: coreRealm, facetRealm: facetRealm, coreConns: coreConns, facetConns: facetConns}
}

// grant seeds each Realm with only its own side's permissions and drives the
// attachment chain across the two Realms.
func (a *independentAssembly) grant(t *testing.T) (values.FacetAcceptanceKey, []byte, corecontract.Revision) {
	t.Helper()
	coreOwner, facetOwner := a.spatial.AuthorityRef(), a.facets.AuthorityRef()
	coreBindings := []realm.PermissionBinding{
		{Subject: coreRealmAdmin, Permission: corecontract.PermissionCreateRoot, Scope: values.CoreElementScope(coreOwner, independentRootID)},
		{Subject: coreRealmAdmin, Permission: corecontract.PermissionCreateEntity, Scope: values.CoreElementScope(coreOwner, independentRootID)},
		{Subject: coreRealmAdmin, Permission: corecontract.PermissionReadElement, Scope: values.CoreElementScope(coreOwner, independentEntityID)},
		{Subject: coreRealmAdmin, Permission: corecontract.PermissionFacetPropose, Scope: values.CoreElementScope(coreOwner, independentEntityID)},
		{Subject: coreRealmAdmin, Permission: corecontract.PermissionFacetActivate, Scope: values.CoreElementScope(coreOwner, independentEntityID)},
		{Subject: coreRealmAdmin, Permission: corecontract.PermissionDeleteEntity, Scope: values.CoreElementScope(coreOwner, independentEntityID)},
		{Subject: coreRealmFacetSvcs, Permission: corecontract.PermissionFacetProposalRead, Scope: values.CoreElementScope(coreOwner, independentEntityID)},
		{Subject: coreRealmFacetSvcs, Permission: corecontract.PermissionFacetAttachmentRead, Scope: values.CoreElementScope(coreOwner, independentEntityID)},
	}
	facetBindings := []realm.PermissionBinding{
		{Subject: facetRealmAdmin, Permission: facetcontract.PermissionAccept, Scope: values.FacetElementScope(facetOwner, independentEntityID)},
		{Subject: facetRealmAdmin, Permission: facetcontract.PermissionPut, Scope: values.FacetKeyScope(facetOwner, independentEntityID, profileType)},
		{Subject: facetRealmAdmin, Permission: facetcontract.PermissionGet, Scope: values.FacetKeyScope(facetOwner, independentEntityID, profileType)},
		{Subject: facetRealmAdmin, Permission: facetcontract.PermissionRealmContactUpdate, Scope: values.AuthorityScope(facetOwner)},
		{Subject: facetRealmCoreSvcs, Permission: facetcontract.PermissionReadAcceptance, Scope: values.FacetElementScope(facetOwner, independentEntityID)},
	}
	for index, grant := range coreBindings {
		if _, err := a.coreRealm.PutBinding(coreRealmAdmin, grant, a.coreRealm.BindingSetRevision(), realm.OperationID(fmt.Sprintf("independent-core-grant-%d", index))); err != nil {
			t.Fatalf("core grant %d: %v", index, err)
		}
	}
	for index, grant := range facetBindings {
		if _, err := a.facetRealm.PutBinding(facetRealmAdmin, grant, a.facetRealm.BindingSetRevision(), realm.OperationID(fmt.Sprintf("independent-facet-grant-%d", index))); err != nil {
			t.Fatalf("facet grant %d: %v", index, err)
		}
	}
	if _, err := a.spatial.Elements().CreateRoot(coreRealmAdmin, independentRootID, "independent-create-root"); err != nil {
		t.Fatalf("CreateRoot: %v", err)
	}
	identity := mustRotation(t, 1, 0, 0, 0)
	created, err := a.spatial.Elements().CreateEntity(coreRealmAdmin, independentEntityID, corecontract.SpatialTuple{Parent: independentRootID, LocalPose: mustPose(t, corecontract.Vec3{}, identity), Range: mustSphere(t, 1)}, "independent-create-entity")
	if err != nil || created.Record == nil || created.Record.Entity == nil {
		t.Fatalf("CreateEntity: %+v, %v", created, err)
	}
	read, err := a.spatial.Elements().ReadElement(coreRealmAdmin, independentEntityID, nil)
	if err != nil {
		t.Fatal(err)
	}
	proposed, err := a.spatial.Facets().ProposeFacet(coreRealmAdmin, independentEntityID, facetOwner, "local:facet", read.Attachment.Revision, "independent-propose")
	if err != nil {
		t.Fatalf("ProposeFacet: %v", err)
	}
	if _, err = a.facets.Namespace.AcceptAttachment(context.Background(), facetRealmAdmin, proposed.Pending, "local:core", "independent-accept"); err != nil {
		t.Fatalf("AcceptAttachment: %v", err)
	}
	if _, err = a.spatial.Facets().ActivateFacet(context.Background(), coreRealmAdmin, independentEntityID, proposed.Attachment.Revision, "independent-activate"); err != nil {
		t.Fatalf("ActivateFacet: %v", err)
	}
	payload := fixturePayload("profile")
	if _, err = a.facets.Namespace.Put(context.Background(), facetRealmAdmin, proposed.Pending, profileType, payload, facetcontract.ExpectAbsent(), "independent-put"); err != nil {
		t.Fatalf("Put: %v", err)
	}
	return proposed.Pending, payload, created.Record.Entity.Revision
}

func TestIndependentRealmClusterVerticalSlice(t *testing.T) {
	a := assembleIndependentRealms(t)
	t.Cleanup(func() { _ = a.facets.Close() })
	t.Cleanup(func() { _ = a.spatial.Close() })
	if a.coreRealm.ID() == a.facetRealm.ID() {
		t.Fatal("test assembly must use two independent Realms")
	}
	access, payload, entityRevision := a.grant(t)
	read, err := a.facets.Namespace.Get(context.Background(), facetRealmAdmin, access, profileType)
	if err != nil || string(read.Payload) != string(payload) {
		t.Fatalf("Get through independent Realm contact = %+v, %v", read, err)
	}

	// Each side's principals are strangers to the other Realm: the denial is
	// proven by the responsible Realm, not by an outage.
	if _, err := a.spatial.Elements().ReadElement(facetRealmAdmin, independentEntityID, nil); !errors.Is(err, corecontract.ErrDenied) {
		t.Fatalf("facet-Realm principal on Core = %v", err)
	}
	if _, err := a.facets.Namespace.Get(context.Background(), coreRealmAdmin, access, profileType); !errors.Is(err, facetcontract.ErrDenied) {
		t.Fatalf("core-Realm principal on Facet = %v", err)
	}

	// The disposition chain crosses the boundary through the Facet Realm's
	// service session, not through a shared grant.
	if _, err := a.spatial.Elements().DeleteEntity(coreRealmAdmin, independentEntityID, entityRevision, "independent-delete"); err != nil {
		t.Fatalf("DeleteEntity: %v", err)
	}
	if _, err := a.facets.Namespace.Get(context.Background(), facetRealmAdmin, access, profileType); !errors.Is(err, facetcontract.ErrElementNotFound) {
		t.Fatalf("disposition through independent Realms = %v", err)
	}
}

func TestConsumerDisconnectStopsAtUnavailable(t *testing.T) {
	a := assembleIndependentRealms(t)
	t.Cleanup(func() { _ = a.facets.Close() })
	t.Cleanup(func() { _ = a.spatial.Close() })
	access, _, _ := a.grant(t)

	// A disconnected Facet Realm is an outage, never a proven denial.
	if err := a.facetConns.UnbindEndpoint("local:facet-realm"); err != nil {
		t.Fatal(err)
	}
	if _, err := a.facets.Namespace.Get(context.Background(), facetRealmAdmin, access, profileType); !errors.Is(err, facetcontract.ErrUnavailable) {
		t.Fatalf("Facet outage compressed into: %v", err)
	}
	if _, err := a.facets.Namespace.Put(context.Background(), facetRealmAdmin, access, profileType, fixturePayload("later"), facetcontract.ExpectAbsent(), "unavailable-put"); !errors.Is(err, facetcontract.ErrUnavailable) {
		t.Fatalf("Facet outage compressed into: %v", err)
	}
	if err := a.facetConns.BindEndpoint("local:facet-realm", a.facetRealm); err != nil {
		t.Fatal(err)
	}
	if _, err := a.facets.Namespace.Get(context.Background(), facetRealmAdmin, access, profileType); err != nil {
		t.Fatalf("re-bound Realm did not resume: %v", err)
	}

	// The same holds for the Core side and for the Facet's relationship reads.
	if err := a.coreConns.UnbindEndpoint("local:core-realm"); err != nil {
		t.Fatal(err)
	}
	if _, err := a.spatial.Elements().ReadElement(coreRealmAdmin, independentEntityID, nil); !errors.Is(err, corecontract.ErrUnavailable) {
		t.Fatalf("Core outage compressed into: %v", err)
	}
	if _, err := a.facets.Namespace.Get(context.Background(), facetRealmAdmin, access, profileType); !errors.Is(err, facetcontract.ErrUnavailable) {
		t.Fatalf("relationship read during Core outage compressed into: %v", err)
	}
	if err := a.coreConns.BindEndpoint("local:core-realm", a.coreRealm); err != nil {
		t.Fatal(err)
	}
	if _, err := a.spatial.Elements().ReadElement(coreRealmAdmin, independentEntityID, nil); err != nil {
		t.Fatalf("re-bound Core Realm did not resume: %v", err)
	}
}

func TestContactMoveKeepsConsumersRunning(t *testing.T) {
	a := assembleIndependentRealms(t)
	t.Cleanup(func() { _ = a.facets.Close() })
	t.Cleanup(func() { _ = a.spatial.Close() })
	access, payload, _ := a.grant(t)
	if err := a.facetConns.BindEndpoint("local:facet-realm-2", a.facetRealm); err != nil {
		t.Fatal(err)
	}
	contact, err := a.facets.Namespace.RealmContact()
	if err != nil {
		t.Fatal(err)
	}
	move := facetcontract.RealmContactUpdate{OperationID: "consumer-move", Owner: contact.Owner, Realm: contact.Realm,
		Expected: contact.Revision, NextHop: "local:facet-realm-2"}
	caller, err := a.facetConns.TrustLocalCaller("local:facet-realm", a.facetRealm.ID(), facetRealmAdmin)
	if err != nil {
		t.Fatal(err)
	}
	result, err := a.facets.Namespace.UpdateRealmContact(context.Background(), caller, move)
	if err != nil || result.Decision != facetcontract.RealmContactCommitted || result.Contact.NextHop != "local:facet-realm-2" {
		t.Fatalf("public-chain contact move = %+v, %v", result, err)
	}

	// Consumers keep running through the current contact without restart.
	read, err := a.facets.Namespace.Get(context.Background(), facetRealmAdmin, access, profileType)
	if err != nil || string(read.Payload) != string(payload) {
		t.Fatalf("Get after contact move = %+v, %v", read, err)
	}
	// The old address is gone; only the moved contact keeps the chain alive.
	if err := a.facetConns.UnbindEndpoint("local:facet-realm"); err != nil {
		t.Fatal(err)
	}
	if _, err := a.facets.Namespace.Get(context.Background(), facetRealmAdmin, access, profileType); err != nil {
		t.Fatalf("Get through the moved contact = %v", err)
	}
	if err := a.facetConns.UnbindEndpoint("local:facet-realm-2"); err != nil {
		t.Fatal(err)
	}
	if _, err := a.facets.Namespace.Get(context.Background(), facetRealmAdmin, access, profileType); !errors.Is(err, facetcontract.ErrUnavailable) {
		t.Fatalf("fully disconnected Facet Realm compressed into: %v", err)
	}
}
