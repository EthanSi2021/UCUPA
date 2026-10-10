package acceptance_test

import (
	"errors"
	"reflect"
	"testing"

	"ucupa/composition"
	"ucupa/shared/values"
	corecontract "ucupa/ucupa-core/contract"
	core "ucupa/ucupa-core/instance"
	facetcontract "ucupa/ucupa-facet/contract"
	realm "ucupa/ucupa-realm/contract"
	realminstance "ucupa/ucupa-realm/instance"
)

func TestRealmOwnedPermissionBindingAdministrationVerticalGate(t *testing.T) {
	const (
		realmID  values.RealmID   = "realm-administration"
		realmHop values.NextHop   = "local:realm-administration"
		rootID   values.ElementID = "00000000-0000-4000-8000-000000002101"
	)
	facetType := fixtureType("demo.dynamic")
	manager := values.PrincipalRef{Realm: realmID, Principal: "manager"}
	localReader := values.PrincipalRef{Realm: realmID, Principal: "local-reader"}
	remoteReader := values.PrincipalRef{Realm: "realm-remote", Principal: "remote-reader"}

	policy, err := realminstance.New(realm.Seed{
		ID:              realmID,
		LocalPrincipals: []values.PrincipalID{manager.Principal, localReader.Principal},
		Bindings: []realm.PermissionBinding{
			administrationBinding(
				manager,
				realm.PermissionPutBinding,
				values.RealmScope(realmID),
			),
			administrationBinding(
				manager,
				realm.PermissionDeleteBinding,
				values.RealmScope(realmID),
			),
		},
	})
	if err != nil {
		t.Fatalf("realm.New: %v", err)
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
		t.Fatalf("core.New: %v", err)
	}

	revision := policy.BindingSetRevision()
	if revision == (realm.BindingSetRevision{}) {
		t.Fatal("new Realm returned a zero BindingSetRevision")
	}
	put := func(operationID realm.OperationID, target realm.PermissionBinding) {
		t.Helper()
		previous := revision
		next, putErr := policy.PutBinding(manager, target, previous, operationID)
		if putErr != nil {
			t.Fatalf("PutBinding(%q): %v", operationID, putErr)
		}
		if next == (realm.BindingSetRevision{}) || next == previous {
			t.Fatalf(
				"PutBinding(%q) revision: got %#v after %#v",
				operationID,
				next,
				previous,
			)
		}
		revision = next
	}

	readRequirement := values.Requirement{
		Permission: corecontract.PermissionReadElement,
		Scope:      values.CoreElementScope(spatial.AuthorityRef(), rootID),
	}
	readBinding := administrationBinding(
		localReader,
		readRequirement.Permission,
		readRequirement.Scope,
	)
	put("acceptance-administration-put-local-read", readBinding)

	facetRequirement := values.Requirement{
		Permission: facetcontract.PermissionGet,
		Scope:      values.FacetKeyScope(fixtureFacetOwner, rootID, facetType),
	}
	put(
		"acceptance-administration-put-local-facet",
		administrationBinding(
			localReader,
			facetRequirement.Permission,
			facetRequirement.Scope,
		),
	)

	if !policy.Authorize(localReader, readRequirement, facetRequirement) {
		t.Fatal("dynamic exact Core and Facet bindings did not authorize")
	}
	if policy.Authorize(localReader, values.Requirement{
		Permission: facetcontract.PermissionGet,
		Scope:      values.FacetKeyScope(fixtureFacetOwner, rootID, fixtureType("demo.other")),
	}) {
		t.Fatal("dynamic Facet binding authorized another exact Facet scope")
	}
	if read, readErr := spatial.Elements().ReadElement(localReader, rootID, nil); readErr != nil ||
		read.State != corecontract.ExistenceNotFound || read.Record != nil {
		t.Fatalf("authorized prospective read: read=%#v err=%v", read, readErr)
	}

	put(
		"acceptance-administration-put-create-root",
		administrationBinding(
			manager,
			corecontract.PermissionCreateRoot,
			values.CoreElementScope(spatial.AuthorityRef(), rootID),
		),
	)
	created, err := spatial.Elements().CreateRoot(
		manager,
		rootID,
		corecontract.OperationID("acceptance-administration-create-root"))

	if err != nil {
		t.Fatalf("CreateRoot after dynamic authorization: %v", err)
	}
	if got, readErr := spatial.Elements().ReadElement(localReader, rootID, nil); readErr != nil ||
		got.Record == nil || created.Record == nil || !reflect.DeepEqual(*got.Record, *created.Record) {
		t.Fatalf("dynamic Core read: got %#v err=%v, want %#v", got, readErr, created)
	}

	previous := revision
	next, err := policy.DeleteBinding(
		manager,
		readBinding,
		previous,
		"acceptance-administration-delete-local-read",
	)
	if err != nil {
		t.Fatalf("DeleteBinding(local read): %v", err)
	}
	if next == (realm.BindingSetRevision{}) || next == previous {
		t.Fatalf("DeleteBinding revision: got %#v after %#v", next, previous)
	}
	revision = next
	if policy.Authorize(localReader, readRequirement) {
		t.Fatal("deleted exact Core binding remained authorized")
	}
	if !policy.Authorize(localReader, facetRequirement) {
		t.Fatal("deleting Core read changed the independent Facet binding")
	}
	if read, readErr := spatial.Elements().ReadElement(localReader, rootID, nil); !errors.Is(readErr, corecontract.ErrDenied) ||
		read != (corecontract.ElementRead{}) {
		t.Fatalf("Core read after binding deletion: read=%#v err=%v", read, readErr)
	}

	remoteBinding := administrationBinding(
		remoteReader,
		corecontract.PermissionReadElement,
		values.CoreElementScope(spatial.AuthorityRef(), rootID),
	)
	put("acceptance-administration-put-remote-read", remoteBinding)
	if !policy.Authorize(remoteReader, readRequirement) {
		t.Fatal("exact remote binding required an unavailable home-Realm lookup")
	}
	if got, readErr := spatial.Elements().ReadElement(remoteReader, rootID, nil); readErr != nil ||
		got.Record == nil || created.Record == nil || !reflect.DeepEqual(*got.Record, *created.Record) {
		t.Fatalf("remote Core read: got %#v err=%v, want %#v", got, readErr, created)
	}
	if policy.Authorize(
		values.PrincipalRef{Realm: "realm-other", Principal: remoteReader.Principal},
		readRequirement,
	) {
		t.Fatal("remote binding authorized the same Principal text in another Realm")
	}
}

func administrationBinding(
	subject values.PrincipalRef,
	permission values.PermissionID,
	scope values.OperationScope,
) realm.PermissionBinding {
	return realm.PermissionBinding{Subject: subject, Permission: permission, Scope: scope}
}
