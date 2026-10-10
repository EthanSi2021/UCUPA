package policy

import (
	"errors"
	"testing"

	"ucupa/shared/values"
	contract "ucupa/ucupa-realm/contract"
)

const (
	realmCoreRef        values.FactAuthorityRef = "00000000-0000-4000-8000-000000000301"
	realmSiteID         values.ElementID        = "00000000-0000-4000-8000-000000000101"
	realmOtherID        values.ElementID        = "00000000-0000-4000-8000-000000000102"
	realmRemoteTargetID values.ElementID        = "00000000-0000-4000-8000-000000000103"
	realmEntityAID      values.ElementID        = "00000000-0000-4000-8000-000000000104"
	realmEntityBID      values.ElementID        = "00000000-0000-4000-8000-000000000105"
	realmAnchorAID      values.ElementID        = "00000000-0000-4000-8000-000000000106"
)

const (
	permissionCreateEntity        values.PermissionID = "ucupa.core.entity.create"
	permissionDeleteEntity        values.PermissionID = "ucupa.core.entity.delete"
	permissionReadElement         values.PermissionID = "ucupa.core.element.read"
	permissionCompoundSubject     values.PermissionID = "test.realm.compound.subject"
	permissionCompoundDestination values.PermissionID = "test.realm.compound.destination"
	permissionRemoteAction        values.PermissionID = "test.realm.remote.action"
	permissionFacetPut            values.PermissionID = "ucupa.facet.put"
	permissionFacetGet            values.PermissionID = "ucupa.facet.get"
)

func TestLocalAuthorizationRequiresExactBinding(t *testing.T) {
	actor := values.PrincipalRef{Realm: "realm-a", Principal: "admin"}
	requirement := values.Requirement{
		Permission: permissionCreateEntity,
		Scope:      values.CoreElementScope(realmCoreRef, realmSiteID),
	}
	policy := mustRealm(t, contract.Seed{
		ID:              "realm-a",
		LocalPrincipals: []values.PrincipalID{"admin"},
		Bindings: []contract.PermissionBinding{{
			Subject: actor, Permission: requirement.Permission, Scope: requirement.Scope,
		}},
	})

	if !policy.Authorize(actor, requirement) {
		t.Fatal("exact local binding must authorize")
	}
	if policy.Authorize(actor, values.Requirement{
		Permission: permissionDeleteEntity,
		Scope:      values.CoreElementScope(realmCoreRef, realmSiteID),
	}) {
		t.Fatal("wrong permission must deny")
	}
	if policy.Authorize(actor, values.Requirement{
		Permission: permissionCreateEntity,
		Scope:      values.CoreElementScope(realmCoreRef, realmOtherID),
	}) {
		t.Fatal("wrong Element scope must deny")
	}
	if policy.Authorize(actor, values.Requirement{
		Permission: permissionCreateEntity,
		Scope:      values.FacetKeyScope(fixtureFacetOwner, realmSiteID, fixtureType("demo.profile")),
	}) {
		t.Fatal("wrong scope kind must deny")
	}
	if policy.Authorize(actor) {
		t.Fatal("an empty requirement set must deny")
	}
}

func TestEstablishedRemotePrincipalNeedsOnlyExactBinding(t *testing.T) {
	remote := values.PrincipalRef{Realm: "realm-b", Principal: "carrier"}
	requirement := values.Requirement{
		Permission: permissionRemoteAction,
		Scope:      values.CoreElementScope(realmCoreRef, realmRemoteTargetID),
	}

	withoutBinding := mustRealm(t, contract.Seed{ID: "realm-a"})
	if withoutBinding.Authorize(remote, requirement) {
		t.Fatal("an established remote Principal without an exact binding must deny")
	}

	withBinding := mustRealm(t, contract.Seed{
		ID: "realm-a",
		Bindings: []contract.PermissionBinding{{
			Subject: remote, Permission: requirement.Permission, Scope: requirement.Scope,
		}},
	})
	if !withBinding.Authorize(remote, requirement) {
		t.Fatal("an established remote Principal with an exact binding must authorize")
	}
	if withBinding.Authorize(values.PrincipalRef{Realm: "realm-b", Principal: "other"}, requirement) {
		t.Fatal("a binding must not authorize another Principal in the remote Realm")
	}
	if withBinding.Authorize(values.PrincipalRef{Realm: "realm-c", Principal: "carrier"}, requirement) {
		t.Fatal("a binding must not authorize the same Principal text in another Realm")
	}
}

func TestSamePrincipalTextInAnotherRealmIsDifferent(t *testing.T) {
	local := values.PrincipalRef{Realm: "realm-a", Principal: "worker"}
	remote := values.PrincipalRef{Realm: "realm-b", Principal: "worker"}
	requirement := values.Requirement{
		Permission: permissionFacetPut,
		Scope:      values.FacetKeyScope(fixtureFacetOwner, realmEntityAID, fixtureType("demo.profile")),
	}
	policy := mustRealm(t, contract.Seed{
		ID:              "realm-a",
		LocalPrincipals: []values.PrincipalID{"worker"},
		Bindings: []contract.PermissionBinding{{
			Subject: local, Permission: requirement.Permission, Scope: requirement.Scope,
		}},
	})

	if !policy.Authorize(local, requirement) {
		t.Fatal("local Principal must authorize")
	}
	if policy.Authorize(remote, requirement) {
		t.Fatal("same Principal text in another Realm must not inherit authority")
	}
}

func TestPermissionBindingFieldsCannotBeRecombined(t *testing.T) {
	actor := values.PrincipalRef{Realm: "realm-a", Principal: "worker"}
	policy := mustRealm(t, contract.Seed{
		ID:              "realm-a",
		LocalPrincipals: []values.PrincipalID{"worker"},
		Bindings: []contract.PermissionBinding{
			{Subject: actor, Permission: permissionFacetPut, Scope: values.FacetKeyScope(fixtureFacetOwner, realmEntityAID, fixtureType("demo.profile"))},
			{Subject: actor, Permission: permissionFacetGet, Scope: values.FacetKeyScope(fixtureFacetOwner, realmEntityBID, fixtureType("demo.profile"))},
		},
	})

	if policy.Authorize(actor, values.Requirement{
		Permission: permissionFacetPut,
		Scope:      values.FacetKeyScope(fixtureFacetOwner, realmEntityBID, fixtureType("demo.profile")),
	}) {
		t.Fatal("permission from one binding and scope from another must not combine")
	}
}

func TestAllRequirementsMustBeBound(t *testing.T) {
	actor := values.PrincipalRef{Realm: "realm-a", Principal: "worker"}
	policy := mustRealm(t, contract.Seed{
		ID:              "realm-a",
		LocalPrincipals: []values.PrincipalID{"worker"},
		Bindings: []contract.PermissionBinding{{
			Subject: actor, Permission: permissionCompoundSubject, Scope: values.CoreElementScope(realmCoreRef, realmEntityAID),
		}},
	})

	if policy.Authorize(actor,
		values.Requirement{Permission: permissionCompoundSubject, Scope: values.CoreElementScope(realmCoreRef, realmEntityAID)},
		values.Requirement{Permission: permissionCompoundDestination, Scope: values.CoreElementScope(realmCoreRef, realmAnchorAID)},
	) {
		t.Fatal("a missing requirement must deny the whole operation")
	}
}

func TestAuthorizationInvalidInputsDeny(t *testing.T) {
	actor := values.PrincipalRef{Realm: "realm-a", Principal: "worker"}
	requirement := values.Requirement{
		Permission: permissionReadElement,
		Scope:      values.CoreElementScope(realmCoreRef, realmAnchorAID),
	}
	policy := mustRealm(t, contract.Seed{
		ID:              "realm-a",
		LocalPrincipals: []values.PrincipalID{"worker"},
		Bindings: []contract.PermissionBinding{{
			Subject: actor, Permission: requirement.Permission, Scope: requirement.Scope,
		}},
	})

	var nilRealm *Realm
	if nilRealm.Authorize(actor, requirement) {
		t.Fatal("nil Realm must deny")
	}
	if policy.Authorize(values.PrincipalRef{}, requirement) {
		t.Fatal("invalid PrincipalRef must deny")
	}
	if policy.Authorize(actor, values.Requirement{Scope: requirement.Scope}) {
		t.Fatal("zero PermissionID must deny")
	}
	if policy.Authorize(actor, values.Requirement{Permission: requirement.Permission}) {
		t.Fatal("zero OperationScope must deny")
	}
	if policy.Authorize(values.PrincipalRef{Realm: "realm-a", Principal: "missing"}, requirement) {
		t.Fatal("unknown local Principal must deny")
	}
}

func TestIndependentRealmsKeepIndependentBindings(t *testing.T) {
	actor := values.PrincipalRef{Realm: "realm-b", Principal: "carrier"}
	requirement := values.Requirement{
		Permission: permissionRemoteAction,
		Scope:      values.CoreElementScope(realmCoreRef, realmRemoteTargetID),
	}
	withBinding := mustRealm(t, contract.Seed{
		ID:       "realm-a",
		Bindings: []contract.PermissionBinding{{Subject: actor, Permission: requirement.Permission, Scope: requirement.Scope}},
	})
	withoutBinding := mustRealm(t, contract.Seed{ID: "realm-a"})

	if !withBinding.Authorize(actor, requirement) {
		t.Fatal("first Realm must preserve its exact binding")
	}
	if withoutBinding.Authorize(actor, requirement) {
		t.Fatal("another Realm instance must not inherit bindings")
	}
}

func TestInvalidSeedFailsClosed(t *testing.T) {
	validRemote := values.PrincipalRef{Realm: "realm-b", Principal: "carrier"}
	validScope := values.CoreElementScope(realmCoreRef, realmAnchorAID)

	tests := []struct {
		name string
		seed contract.Seed
	}{
		{name: "empty Realm ID", seed: contract.Seed{}},
		{name: "empty local Principal", seed: contract.Seed{ID: "realm-a", LocalPrincipals: []values.PrincipalID{""}}},
		{
			name: "unknown local Principal",
			seed: contract.Seed{
				ID: "realm-a",
				Bindings: []contract.PermissionBinding{{
					Subject:    values.PrincipalRef{Realm: "realm-a", Principal: "missing"},
					Permission: permissionReadElement,
					Scope:      validScope,
				}},
			},
		},
		{
			name: "invalid binding subject",
			seed: contract.Seed{ID: "realm-a", Bindings: []contract.PermissionBinding{{
				Permission: permissionReadElement, Scope: validScope,
			}}},
		},
		{
			name: "empty binding permission",
			seed: contract.Seed{ID: "realm-a", Bindings: []contract.PermissionBinding{{
				Subject: validRemote, Scope: validScope,
			}}},
		},
		{
			name: "noncanonical binding scope",
			seed: contract.Seed{ID: "realm-a", Bindings: []contract.PermissionBinding{{
				Subject: validRemote, Permission: permissionReadElement,
				Scope: values.CoreElementScope(realmCoreRef, "human-readable-element"),
			}}},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := New(test.seed); !errors.Is(err, contract.ErrInvalidSeed) {
				t.Fatalf("got %v", err)
			}
		})
	}
}

func mustRealm(t *testing.T, seed contract.Seed) *Realm {
	t.Helper()
	value, err := New(seed)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return value
}

func TestInMemoryCloseIsNoop(t *testing.T) {
	manager := values.PrincipalRef{Realm: "realm-a", Principal: "manager"}
	policy := realmWithManagers(t, manager)
	initial := policy.BindingSetRevision()
	if err := policy.Close(); err != nil {
		t.Fatalf("first in-memory Close: %v", err)
	}
	if got := policy.BindingSetRevision(); got != initial {
		t.Fatalf("in-memory Close changed revision: got %#v, want %#v", got, initial)
	}
	target := contract.PermissionBinding{
		Subject: manager, Permission: permissionReadElement, Scope: values.CoreElementScope(realmCoreRef, realmSiteID),
	}
	putRevision := mustPutBinding(t, policy, manager, target, initial, "after-close-put")
	if err := policy.Close(); err != nil {
		t.Fatalf("second in-memory Close: %v", err)
	}
	mustDeleteBinding(t, policy, manager, target, putRevision, "after-close-delete")
	if policy.Authorize(manager, bindingRequirement(target)) {
		t.Fatal("in-memory Realm stopped applying mutations after Close")
	}
	var nilRealm *Realm
	if err := nilRealm.Close(); err != nil {
		t.Fatalf("nil Realm Close: %v", err)
	}
}

func TestInMemoryRealmCopiesShareOneOwnerSession(t *testing.T) {
	manager := values.PrincipalRef{Realm: "realm-a", Principal: "manager"}
	policy := realmWithManagers(t, manager)
	copied := *policy
	target := contract.PermissionBinding{
		Subject: manager, Permission: permissionReadElement, Scope: values.CoreElementScope(realmCoreRef, realmSiteID),
	}

	initial := policy.BindingSetRevision()
	putRevision := mustPutBinding(t, policy, manager, target, initial, "copy-shared-put")
	if got := copied.BindingSetRevision(); got != putRevision {
		t.Fatalf("copied handle retained a stale revision after Put: got %#v, want %#v", got, putRevision)
	}
	if !copied.Authorize(manager, bindingRequirement(target)) {
		t.Fatal("copied handle retained a stale relation after Put")
	}
	if got := mustPutBinding(t, &copied, manager, target, initial, "copy-shared-put"); got != putRevision {
		t.Fatalf("exact replay through copied handle: got %#v, want %#v", got, putRevision)
	}

	if err := copied.Close(); err != nil {
		t.Fatalf("in-memory copied handle Close: %v", err)
	}
	if got := policy.BindingSetRevision(); got != putRevision {
		t.Fatalf("copied handle Close changed shared in-memory revision: got %#v, want %#v", got, putRevision)
	}

	deleteRevision := mustDeleteBinding(t, &copied, manager, target, putRevision, "copy-shared-delete")
	if got := policy.BindingSetRevision(); got != deleteRevision {
		t.Fatalf("original handle retained a stale revision after copied Delete: got %#v, want %#v", got, deleteRevision)
	}
	if policy.Authorize(manager, bindingRequirement(target)) {
		t.Fatal("original handle authorized a binding deleted through its copy")
	}
}

func TestZeroRealmHandleFailsClosed(t *testing.T) {
	var policy Realm
	manager := values.PrincipalRef{Realm: "realm-a", Principal: "manager"}
	target := contract.PermissionBinding{
		Subject: manager, Permission: permissionReadElement, Scope: values.CoreElementScope(realmCoreRef, realmSiteID),
	}
	validRevision := realmWithManagers(t, manager).BindingSetRevision()

	if policy.ID() != "" || policy.BindingSetRevision() != (contract.BindingSetRevision{}) {
		t.Fatal("zero Realm handle exposed an identity or revision")
	}
	if policy.Authorize(manager, bindingRequirement(target)) {
		t.Fatal("zero Realm handle authorized a binding")
	}
	if got, err := policy.PutBinding(manager, target, validRevision, "zero-put"); !errors.Is(err, contract.ErrStore) || got != (contract.BindingSetRevision{}) {
		t.Fatalf("zero Realm PutBinding: revision=%#v err=%v, want zero/ErrStore", got, err)
	}
	if got, err := policy.DeleteBinding(manager, target, validRevision, "zero-delete"); !errors.Is(err, contract.ErrStore) || got != (contract.BindingSetRevision{}) {
		t.Fatalf("zero Realm DeleteBinding: revision=%#v err=%v, want zero/ErrStore", got, err)
	}
	if err := policy.Close(); err != nil {
		t.Fatalf("zero Realm Close: %v", err)
	}
}
