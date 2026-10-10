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
	core "ucupa/ucupa-core/instance"
	facetcontract "ucupa/ucupa-facet/contract"
	facet "ucupa/ucupa-facet/instance"
	realm "ucupa/ucupa-realm/contract"
	realminstance "ucupa/ucupa-realm/instance"
)

func TestRealmAuthenticationEstablishesCallerForExistingOwnerAuthorization(t *testing.T) {
	const (
		realmID  values.RealmID   = "realm-authentication-acceptance"
		realmHop values.NextHop   = "local:realm-authentication-acceptance"
		rootID   values.ElementID = "00000000-0000-4000-8000-00000000a001"
	)
	actor := values.PrincipalRef{Realm: realmID, Principal: "local-operator"}
	serviceActor := values.PrincipalRef{Realm: realmID, Principal: "facet-service"}
	key := values.FacetKey{Element: rootID, Type: fixtureType("acceptance.authentication")}
	original := acceptanceCredentialEvidence(realmID, 1, 33)
	revoker := acceptanceCredentialEvidence(realmID, 2, 65)
	replacement := acceptanceCredentialEvidence(realmID, 3, 97)

	policy, err := realminstance.New(realm.Seed{
		ID:              realmID,
		LocalPrincipals: []values.PrincipalID{actor.Principal, serviceActor.Principal},
		Credentials: []realm.CredentialBootstrap{
			{Subject: actor.Principal, Evidence: original},
			{Subject: actor.Principal, Evidence: revoker},
		},
		Bindings: []realm.PermissionBinding{
			binding(actor, realm.PermissionPutBinding, values.RealmScope(realmID)),
			binding(actor, realm.PermissionRotateOwnCredential, values.RealmScope(realmID)),
			binding(actor, realm.PermissionRevokeCredential, values.RealmScope(realmID)),
		},
	})
	if err != nil {
		t.Fatalf("New Realm: %v", err)
	}

	established, err := policy.Authenticate(original)
	if err != nil || established != actor {
		t.Fatalf("Authenticate original: got %#v, err %v; want %#v", established, err, actor)
	}

	realmLinks := &composition.LocalRealmConnections{}
	if err = realmLinks.BindEndpoint(realmHop, policy); err != nil {
		t.Fatalf("BindEndpoint: %v", err)
	}
	spatial, err := core.New(
		realmLinks,
		corecontract.RealmInitialization{Realm: realmID, NextHop: realmHop},
		corecontract.Config{})
	if err != nil {
		t.Fatalf("New Core: %v", err)
	}
	owner := spatial.AuthorityRef()
	grants := []realm.PermissionBinding{
		binding(actor, corecontract.PermissionCreateRoot, values.CoreElementScope(owner, rootID)),
		binding(actor, corecontract.PermissionReadElement, values.CoreElementScope(owner, rootID)),
		binding(serviceActor, corecontract.PermissionFacetProposalRead, values.CoreElementScope(owner, rootID)),
		binding(serviceActor, corecontract.PermissionFacetAttachmentRead, values.CoreElementScope(owner, rootID)),
		binding(actor, corecontract.PermissionFacetPropose, values.CoreElementScope(owner, rootID)),
		binding(actor, corecontract.PermissionFacetActivate, values.CoreElementScope(owner, rootID)),
	}
	for index, grant := range grants {
		if _, err := policy.PutBinding(established, grant, policy.BindingSetRevision(), realm.OperationID(fmt.Sprintf("authentication-bootstrap-grant-%d", index))); err != nil {
			t.Fatalf("PutBinding: %v", err)
		}
	}
	cores := &composition.LocalFacetCoreConnections{}
	if err = cores.BindEndpoint("local:core", composition.LocalFacetCoreEndpoint{Read: spatial.Facets(), Control: spatial.Facets(), Access: spatial.Access()}); err != nil {
		t.Fatal(err)
	}
	source, err := cores.ForFacet(serviceActor)
	if err != nil {
		t.Fatal(err)
	}
	facets, err := facet.New(
		realmLinks,
		source,
		facetcontract.RealmInitialization{Realm: realmID, NextHop: realmHop},
		facet.Config{})
	if err != nil {
		t.Fatalf("New Facets: %v", err)
	}
	connections := &composition.LocalFacetConnections{}
	if err = connections.BindEndpoint("local:facet", facets); err != nil {
		t.Fatal(err)
	}
	acceptanceSource, err := connections.ForCore(serviceActor)
	if err != nil {
		t.Fatal(err)
	}
	if err = spatial.Facets().SetFacetAcceptanceSource(acceptanceSource); err != nil {
		t.Fatal(err)
	}
	for i, grant := range []realm.PermissionBinding{
		binding(actor, facetcontract.PermissionAccept, values.FacetElementScope(facets.AuthorityRef(), rootID)),
		binding(serviceActor, facetcontract.PermissionReadAcceptance, values.FacetElementScope(facets.AuthorityRef(), rootID)),
	} {
		if _, err = policy.PutBinding(actor, grant, policy.BindingSetRevision(), realm.OperationID(fmt.Sprint("handshake", i))); err != nil {
			t.Fatal(err)
		}
	}
	for index, permission := range []values.PermissionID{facetcontract.PermissionPut, facetcontract.PermissionGet} {
		grant := binding(actor, permission, values.FacetKeyScope(facets.AuthorityRef(), key.Element, key.Type))
		if _, err := policy.PutBinding(established, grant, policy.BindingSetRevision(), realm.OperationID(fmt.Sprintf("authentication-facet-grant-%d", index))); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := spatial.Elements().CreateRoot(
		established,
		rootID,
		corecontract.OperationID("acceptance-authentication-create-root")); err != nil {
		t.Fatalf("Core CreateRoot with established PrincipalRef: %v", err)
	}
	access := attachFixture(t, spatial, facets, established, rootID)
	payload := fixturePayload("established-caller-facet")
	if _, err := facets.Namespace.Put(context.Background(), established, access, key.Type,
		payload,
		facetcontract.ExpectAbsent(),
		facetcontract.OperationID("acceptance-authentication-put-facet"),
	); err != nil {
		t.Fatalf("Facet Put with established PrincipalRef: %v", err)
	}

	revision := policy.CredentialSetRevision()
	rotatedRevision, err := policy.RotateCredential(
		original,
		original.ID,
		replacement,
		revision,
		realm.CredentialOperationID("acceptance-authentication-rotate"),
	)
	if err != nil {
		t.Fatalf("RotateCredential: %v", err)
	}
	if rotatedRevision == (realm.CredentialSetRevision{}) || rotatedRevision == revision {
		t.Fatal("rotation did not advance the credential-set revision")
	}
	if got, err := policy.Authenticate(original); !errors.Is(err, realm.ErrUnauthenticated) || got != (values.PrincipalRef{}) {
		t.Fatalf("Authenticate rotated proof: got %#v, err %v", got, err)
	}
	replacementActor, err := policy.Authenticate(replacement)
	if err != nil || replacementActor != actor {
		t.Fatalf("Authenticate replacement: got %#v, err %v; want %#v", replacementActor, err, actor)
	}
	assertOwnerAuthorizationFlow(t, spatial, facets, replacementActor, rootID, key, payload)

	revokedRevision, err := policy.RevokeCredential(
		revoker,
		replacement.ID,
		rotatedRevision,
		realm.CredentialOperationID("acceptance-authentication-revoke"),
	)
	if err != nil {
		t.Fatalf("RevokeCredential: %v", err)
	}
	if revokedRevision == (realm.CredentialSetRevision{}) || revokedRevision == rotatedRevision {
		t.Fatal("active revocation did not advance the credential-set revision")
	}
	if got, err := policy.Authenticate(replacement); !errors.Is(err, realm.ErrUnauthenticated) || got != (values.PrincipalRef{}) {
		t.Fatalf("Authenticate revoked replacement: got %#v, err %v", got, err)
	}

	if replacementActor != actor {
		t.Fatalf("previously established PrincipalRef changed after revocation: got %#v, want %#v", replacementActor, actor)
	}
	// Revocation governs later establishment. It cannot mutate an ordinary
	// PrincipalRef value that was already returned to a trusted in-process caller.
	assertOwnerAuthorizationFlow(t, spatial, facets, replacementActor, rootID, key, payload)
}

func acceptanceCredentialEvidence(
	audience values.RealmID,
	idStart byte,
	secretStart byte,
) realm.CredentialEvidence {
	var id realm.CredentialID
	for index := range id {
		id[index] = idStart + byte(index)
	}
	var secret realm.CredentialSecret
	for index := range secret {
		secret[index] = secretStart + byte(index)
	}
	return realm.CredentialEvidence{Audience: audience, ID: id, Secret: secret}
}

func assertOwnerAuthorizationFlow(
	t *testing.T,
	spatial *core.Core,
	facets *facet.Instance,
	actor values.PrincipalRef,
	rootID values.ElementID,
	key values.FacetKey,
	wantPayload []byte,
) {
	t.Helper()
	read, err := spatial.Elements().ReadElement(actor, rootID, nil)
	if err != nil || read.State != corecontract.ExistencePresent || read.Record == nil ||
		read.Record.Anchor == nil || read.Record.Anchor.ID != rootID {
		t.Fatalf("Core read with established PrincipalRef: got %#v, err %v", read, err)
	}
	result, err := facets.Namespace.Get(context.Background(), actor, read.Attachment.Key, key.Type)
	payload := result.Payload
	if err != nil || !bytes.Equal(payload, wantPayload) {
		t.Fatalf("Facet Get with established PrincipalRef: got %q, err %v; want %q", payload, err, wantPayload)
	}
}
