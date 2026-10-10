package policy

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"go.etcd.io/bbolt"

	"ucupa/shared/values"
	corecontract "ucupa/ucupa-core/contract"
	core "ucupa/ucupa-core/instance"
	facetcontract "ucupa/ucupa-facet/contract"
	facet "ucupa/ucupa-facet/instance"
	contract "ucupa/ucupa-realm/contract"
)

var (
	durableManagerActor = values.PrincipalRef{Realm: "realm-durable", Principal: "manager"}
	durableLocalActor   = values.PrincipalRef{Realm: "realm-durable", Principal: "local"}
	durableRemoteActor  = values.PrincipalRef{Realm: "realm-remote", Principal: "carrier"}

	durableElementRequirement = values.Requirement{
		Permission: permissionCreateEntity,
		Scope:      values.CoreElementScope(realmCoreRef, realmSiteID),
	}
	durableFacetRequirement = values.Requirement{
		Permission: permissionFacetPut,
		Scope:      values.FacetKeyScope(fixtureFacetOwner, realmEntityAID, fixtureType("demo.profile")),
	}
	durableRemoteRequirement = values.Requirement{
		Permission: permissionRemoteAction,
		Scope:      values.CoreElementScope(realmCoreRef, realmRemoteTargetID),
	}
	durableDynamicRequirement = values.Requirement{
		Permission: permissionReadElement,
		Scope:      values.CoreElementScope(realmCoreRef, realmOtherID),
	}
)

func TestRealmNewCreateAndOpenExposeTheSameSemantics(t *testing.T) {
	seed := durableRealmSeed()
	inMemory := mustRealm(t, seed)
	path := filepath.Join(t.TempDir(), "realm.db")
	created := mustCreateDurableRealm(t, seed, path)

	assertActiveRealmSemantics(t, inMemory)
	assertActiveRealmSemantics(t, created)
	assertRealmStoreLocked(t, path)

	target := durableDynamicBinding()
	inMemoryInitial := inMemory.BindingSetRevision()
	durableInitial := created.BindingSetRevision()
	inMemoryPut := mustPutDurableBinding(
		t,
		inMemory,
		target,
		inMemoryInitial,
		"durability-profile-put",
	)
	durablePut := mustPutDurableBinding(
		t,
		created,
		target,
		durableInitial,
		"durability-profile-put",
	)
	if !inMemory.Authorize(durableLocalActor, durableDynamicRequirement) ||
		!created.Authorize(durableLocalActor, durableDynamicRequirement) {
		t.Fatal("New and CreateDurable did not publish the same Put semantics")
	}
	assertExactPutReplay(t, inMemory, target, inMemoryInitial, inMemoryPut)
	assertExactPutReplay(t, created, target, durableInitial, durablePut)

	if err := created.Close(); err != nil {
		t.Fatalf("Close created owner: %v", err)
	}
	assertRetiredRealm(t, created, durablePut, target)
	assertRealmStoreUnlocked(t, path)

	reopened := mustOpenDurableRealm(t, path)
	assertRealmStoreLocked(t, path)
	assertActiveRealmSemantics(t, reopened)
	if got := reopened.BindingSetRevision(); got != durablePut {
		t.Fatalf("reopened revision: got %#v, want %#v", got, durablePut)
	}
	if !reopened.Authorize(durableLocalActor, durableDynamicRequirement) {
		t.Fatal("reopen lost the dynamically inserted binding")
	}
	assertExactPutReplay(t, reopened, target, durableInitial, durablePut)

	inMemoryDelete := mustDeleteDurableBinding(
		t,
		inMemory,
		target,
		inMemoryPut,
		"durability-profile-delete",
	)
	durableDelete := mustDeleteDurableBinding(
		t,
		reopened,
		target,
		durablePut,
		"durability-profile-delete",
	)
	if inMemory.Authorize(durableLocalActor, durableDynamicRequirement) ||
		reopened.Authorize(durableLocalActor, durableDynamicRequirement) {
		t.Fatal("New and OpenDurable did not publish the same Delete semantics")
	}
	assertExactDeleteReplay(t, inMemory, target, inMemoryPut, inMemoryDelete)
	assertExactDeleteReplay(t, reopened, target, durablePut, durableDelete)

	if err := reopened.Close(); err != nil {
		t.Fatalf("Close reopened owner: %v", err)
	}
	assertRetiredRealm(t, reopened, durableDelete, target)
	assertRealmStoreUnlocked(t, path)

	recovered := mustOpenDurableRealm(t, path)
	if got := recovered.BindingSetRevision(); got != durableDelete {
		t.Fatalf("recovered revision: got %#v, want %#v", got, durableDelete)
	}
	if recovered.Authorize(durableLocalActor, durableDynamicRequirement) {
		t.Fatal("second reopen restored a deleted binding")
	}
	assertExactDeleteReplay(t, recovered, target, durablePut, durableDelete)
	if err := recovered.Close(); err != nil {
		t.Fatalf("Close recovered owner: %v", err)
	}
	assertRealmStoreUnlocked(t, path)
}

func TestRealmInMemoryCloseIsNoOp(t *testing.T) {
	policy := mustRealm(t, durableRealmSeed())
	before := policy.BindingSetRevision()

	if err := policy.Close(); err != nil {
		t.Fatalf("in-memory Close: %v", err)
	}
	if err := policy.Close(); err != nil {
		t.Fatalf("repeated in-memory Close: %v", err)
	}
	if got := policy.BindingSetRevision(); got != before {
		t.Fatalf("in-memory Close changed revision: got %#v, want %#v", got, before)
	}
	assertActiveRealmSemantics(t, policy)
	mustPutDurableBinding(
		t,
		policy,
		durableDynamicBinding(),
		before,
		"durability-in-memory-after-close",
	)
}

func TestRealmDurableOwnerHoldsLockAndCloseRetires(t *testing.T) {
	path := filepath.Join(t.TempDir(), "realm.db")
	owner := mustCreateDurableRealm(t, durableRealmSeed(), path)
	knownRevision := owner.BindingSetRevision()
	target := durableDynamicBinding()
	assertRealmStoreLocked(t, path)

	opened, openErr := OpenDurable(path)
	if !errors.Is(openErr, contract.ErrStore) || opened != nil {
		t.Fatalf("OpenDurable while owner active: Realm=%v err=%v, want nil/ErrStore", opened, openErr)
	}
	duplicate, createErr := CreateDurable(contract.Seed{ID: "another-realm"}, path)
	if !errors.Is(createErr, contract.ErrStore) || duplicate != nil {
		t.Fatalf("CreateDurable while path exists: Realm=%v err=%v, want nil/ErrStore", duplicate, createErr)
	}

	if err := owner.Close(); err != nil {
		t.Fatalf("Close durable owner: %v", err)
	}
	assertRetiredRealm(t, owner, knownRevision, target)
	assertRealmStoreUnlocked(t, path)

	before := mustReadFile(t, path)
	duplicate, createErr = CreateDurable(contract.Seed{ID: "another-realm"}, path)
	if !errors.Is(createErr, contract.ErrStore) || duplicate != nil {
		t.Fatalf("CreateDurable existing closed Store: Realm=%v err=%v", duplicate, createErr)
	}
	after := mustReadFile(t, path)
	if !bytes.Equal(after, before) {
		t.Fatal("exclusive CreateDurable changed an existing Realm Store")
	}

	reopened := mustOpenDurableRealm(t, path)
	assertRealmStoreLocked(t, path)
	if err := reopened.Close(); err != nil {
		t.Fatalf("Close independently reopened owner: %v", err)
	}
	assertRealmStoreUnlocked(t, path)
}

func TestRealmDurableValueCopySharesRetirementAndStoreOwnership(t *testing.T) {
	path := filepath.Join(t.TempDir(), "realm.db")
	owner := mustCreateDurableRealm(t, durableRealmSeed(), path)
	alias := *owner
	knownRevision := owner.BindingSetRevision()
	target := durableDynamicBinding()

	if err := (&alias).Close(); err != nil {
		t.Fatalf("Close copied durable handle: %v", err)
	}
	assertRetiredRealm(t, owner, knownRevision, target)
	assertRetiredRealm(t, &alias, knownRevision, target)
	assertRealmStoreUnlocked(t, path)

	reopened := mustOpenDurableRealm(t, path)
	assertActiveRealmSemantics(t, reopened)
	if err := reopened.Close(); err != nil {
		t.Fatalf("Close independently reopened owner: %v", err)
	}
	assertRealmStoreUnlocked(t, path)
}

func TestRealmCreateDurableRejectsInvalidSeedBeforeDestinationCreation(t *testing.T) {
	validRemote := values.PrincipalRef{Realm: "realm-remote", Principal: "carrier"}
	validScope := values.CoreElementScope(realmCoreRef, realmSiteID)
	validCredential := testCredentialEvidence("realm-a", 1, 11)
	tests := []struct {
		name string
		seed contract.Seed
	}{
		{name: "empty Realm ID", seed: contract.Seed{}},
		{name: "empty local Principal", seed: contract.Seed{ID: "realm-a", LocalPrincipals: []values.PrincipalID{""}}},
		{
			name: "unknown local Principal",
			seed: contract.Seed{ID: "realm-a", Bindings: []contract.PermissionBinding{{
				Subject:    values.PrincipalRef{Realm: "realm-a", Principal: "missing"},
				Permission: permissionReadElement,
				Scope:      validScope,
			}}},
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
			name: "invalid binding scope",
			seed: contract.Seed{ID: "realm-a", Bindings: []contract.PermissionBinding{{
				Subject: validRemote, Permission: permissionReadElement,
				Scope: values.CoreElementScope(realmCoreRef, "not-a-canonical-element"),
			}}},
		},
		{
			name: "credential for unregistered Principal",
			seed: contract.Seed{
				ID: "realm-a",
				Credentials: []contract.CredentialBootstrap{{
					Subject: "missing", Evidence: validCredential,
				}},
			},
		},
		{
			name: "credential for foreign audience",
			seed: contract.Seed{
				ID:              "realm-a",
				LocalPrincipals: []values.PrincipalID{"alice"},
				Credentials: []contract.CredentialBootstrap{{
					Subject: "alice", Evidence: testCredentialEvidence("realm-b", 1, 11),
				}},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "realm.db")
			created, err := CreateDurable(test.seed, path)
			if !errors.Is(err, contract.ErrInvalidSeed) || created != nil {
				t.Fatalf("CreateDurable invalid Seed: Realm=%v err=%v, want nil/ErrInvalidSeed", created, err)
			}
			if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("invalid Seed touched destination: %v", err)
			}
		})
	}
}

func TestRealmDurableLifecycleRejectsWithoutCreateOrModification(t *testing.T) {
	t.Run("missing", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "missing.db")
		opened, err := OpenDurable(path)
		if !errors.Is(err, contract.ErrStore) || opened != nil {
			t.Fatalf("OpenDurable missing: Realm=%v err=%v, want nil/ErrStore", opened, err)
		}
		if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("missing OpenDurable created destination: %v", err)
		}
	})

	t.Run("empty", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "empty.db")
		assertRejectedRealmBytesUnchanged(t, path, []byte{})
	})

	t.Run("malformed", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "malformed.db")
		assertRejectedRealmBytesUnchanged(t, path, []byte("not a Realm Store\n"))
	})

	t.Run("foreign bbolt", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "foreign.db")
		database, err := bbolt.Open(path, 0o600, nil)
		if err != nil {
			t.Fatalf("open foreign bbolt: %v", err)
		}
		if err := database.Update(func(tx *bbolt.Tx) error {
			_, err := tx.CreateBucket([]byte("foreign.bucket"))
			return err
		}); err != nil {
			_ = database.Close()
			t.Fatalf("populate foreign bbolt: %v", err)
		}
		if err := database.Close(); err != nil {
			t.Fatalf("close foreign bbolt: %v", err)
		}
		assertExistingRealmStoreRejectedUnchanged(t, path)
	})
}

func TestRealmCoreAndFacetStoresMutuallyRejectWithoutModification(t *testing.T) {
	authorizer := ownerTestAuthorizer{}
	existence := ownerTestCoreSource{}
	coreInitial := corecontract.RealmInitialization{Realm: "realm-store-test", NextHop: "local:store-test"}
	facetInitial := facetcontract.RealmInitialization{Realm: "realm-store-test", NextHop: "local:store-test"}

	t.Run("Realm rejects Core", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "core.db")
		spatial, err := core.CreateDurable(authorizer, coreInitial, path, corecontract.Config{})
		if err != nil {
			t.Fatalf("create Core Store: %v", err)
		}
		if err := spatial.Close(); err != nil {
			t.Fatalf("close Core Store: %v", err)
		}
		assertExistingRealmStoreRejectedUnchanged(t, path)
		reopened, err := core.OpenDurable(authorizer, path, corecontract.Config{})
		if err != nil {
			t.Fatalf("Core owner reopen after Realm rejection: %v", err)
		}
		if err := reopened.Close(); err != nil {
			t.Fatalf("close reopened Core owner: %v", err)
		}
	})

	t.Run("Realm rejects Facet", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "facet.db")
		facets, err := facet.CreateDurable(authorizer, existence, facetInitial, path, facet.Config{})
		if err != nil {
			t.Fatalf("create Facet Store: %v", err)
		}
		if err := facets.Close(); err != nil {
			t.Fatalf("close Facet Store: %v", err)
		}
		assertExistingRealmStoreRejectedUnchanged(t, path)
		reopened, err := facet.OpenDurable(authorizer, existence, path, facet.Config{})
		if err != nil {
			t.Fatalf("Facet owner reopen after Realm rejection: %v", err)
		}
		if err := reopened.Close(); err != nil {
			t.Fatalf("close reopened Facet owner: %v", err)
		}
	})

	t.Run("Core and Facet reject Realm", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "realm.db")
		owner := mustCreateDurableRealm(t, durableRealmSeed(), path)
		if err := owner.Close(); err != nil {
			t.Fatalf("close Realm Store before foreign probes: %v", err)
		}
		before := mustReadFile(t, path)

		spatial, coreErr := core.OpenDurable(authorizer, path, corecontract.Config{})
		if !errors.Is(coreErr, corecontract.ErrStore) || spatial != nil {
			t.Fatalf("Core opens Realm Store: Core=%v err=%v", spatial, coreErr)
		}
		facets, facetErr := facet.OpenDurable(authorizer, existence, path, facet.Config{})
		if !errors.Is(facetErr, facetcontract.ErrStore) || facets != nil {
			t.Fatalf("Facet opens Realm Store: Facets=%v err=%v", facets, facetErr)
		}

		after := mustReadFile(t, path)
		if !bytes.Equal(after, before) {
			t.Fatal("foreign owner rejection changed Realm Store bytes")
		}
		reopened := mustOpenDurableRealm(t, path)
		assertActiveRealmSemantics(t, reopened)
		if err := reopened.Close(); err != nil {
			t.Fatalf("close Realm owner after foreign rejection: %v", err)
		}
	})
}

func TestRealmCreateDurableRejectsBackendUnrepresentableSeed(t *testing.T) {
	seed := contract.Seed{
		ID: "realm-capacity",
		Bindings: []contract.PermissionBinding{{
			Subject:    durableRemoteActor,
			Permission: values.PermissionID(strings.Repeat("x", bbolt.MaxKeySize)),
			Scope:      values.FacetKeyScope(fixtureFacetOwner, realmEntityAID, fixtureType("capacity")),
		}},
	}
	if _, err := New(seed); err != nil {
		t.Fatalf("otherwise-valid oversized Seed rejected by New: %v", err)
	}

	path := filepath.Join(t.TempDir(), "realm.db")
	created, err := CreateDurable(seed, path)
	if !errors.Is(err, contract.ErrStore) || created != nil {
		t.Fatalf("CreateDurable unrepresentable Seed: Realm=%v err=%v, want nil/ErrStore", created, err)
	}
	opened, openErr := OpenDurable(path)
	if !errors.Is(openErr, contract.ErrStore) || opened != nil {
		t.Fatalf("OpenDurable after unrepresentable Seed: Realm=%v err=%v, want nil/ErrStore", opened, openErr)
	}
}

func durableRealmSeed() contract.Seed {
	return contract.Seed{
		ID: "realm-durable",
		LocalPrincipals: []values.PrincipalID{
			durableManagerActor.Principal,
			durableLocalActor.Principal,
			"unbound",
		},
		Bindings: []contract.PermissionBinding{
			{
				Subject: durableManagerActor, Permission: contract.PermissionPutBinding,
				Scope: values.RealmScope(durableManagerActor.Realm),
			},
			{
				Subject: durableManagerActor, Permission: contract.PermissionDeleteBinding,
				Scope: values.RealmScope(durableManagerActor.Realm),
			},
			{
				Subject: durableLocalActor, Permission: durableElementRequirement.Permission,
				Scope: durableElementRequirement.Scope,
			},
			{
				Subject: durableLocalActor, Permission: durableFacetRequirement.Permission,
				Scope: durableFacetRequirement.Scope,
			},
			{
				Subject: durableRemoteActor, Permission: durableRemoteRequirement.Permission,
				Scope: durableRemoteRequirement.Scope,
			},
		},
	}
}

func durableDynamicBinding() contract.PermissionBinding {
	return contract.PermissionBinding{
		Subject:    durableLocalActor,
		Permission: durableDynamicRequirement.Permission,
		Scope:      durableDynamicRequirement.Scope,
	}
}

func assertActiveRealmSemantics(t *testing.T, policy *Realm) {
	t.Helper()
	if policy == nil {
		t.Fatal("active Realm is nil")
	}
	if got, want := policy.ID(), values.RealmID("realm-durable"); got != want {
		t.Fatalf("Realm ID: got %q, want %q", got, want)
	}
	if policy.BindingSetRevision() == (contract.BindingSetRevision{}) {
		t.Fatal("active Realm returned a zero BindingSetRevision")
	}
	assertDurableDecisionMatrix(t, policy)
}

func assertDurableDecisionMatrix(t *testing.T, policy *Realm) {
	t.Helper()
	managerPut := values.Requirement{
		Permission: contract.PermissionPutBinding,
		Scope:      values.RealmScope(durableManagerActor.Realm),
	}
	managerDelete := values.Requirement{
		Permission: contract.PermissionDeleteBinding,
		Scope:      values.RealmScope(durableManagerActor.Realm),
	}
	tests := []struct {
		name         string
		actor        values.PrincipalRef
		requirements []values.Requirement
		want         bool
	}{
		{name: "manager Put", actor: durableManagerActor, requirements: []values.Requirement{managerPut}, want: true},
		{name: "manager Delete", actor: durableManagerActor, requirements: []values.Requirement{managerDelete}, want: true},
		{name: "exact local Element", actor: durableLocalActor, requirements: []values.Requirement{durableElementRequirement}, want: true},
		{name: "exact local Facet", actor: durableLocalActor, requirements: []values.Requirement{durableFacetRequirement}, want: true},
		{name: "exact remote", actor: durableRemoteActor, requirements: []values.Requirement{durableRemoteRequirement}, want: true},
		{name: "complete all-of", actor: durableLocalActor, requirements: []values.Requirement{durableElementRequirement, durableFacetRequirement}, want: true},
		{name: "wrong Realm", actor: values.PrincipalRef{Realm: "realm-other", Principal: "local"}, requirements: []values.Requirement{durableElementRequirement}},
		{name: "wrong Principal", actor: values.PrincipalRef{Realm: durableLocalActor.Realm, Principal: "other"}, requirements: []values.Requirement{durableElementRequirement}},
		{name: "wrong Permission", actor: durableLocalActor, requirements: []values.Requirement{{Permission: permissionDeleteEntity, Scope: durableElementRequirement.Scope}}},
		{name: "wrong scope kind", actor: durableLocalActor, requirements: []values.Requirement{{Permission: durableElementRequirement.Permission, Scope: values.FacetKeyScope(fixtureFacetOwner, realmSiteID, fixtureType("demo.profile"))}}},
		{name: "wrong Element", actor: durableLocalActor, requirements: []values.Requirement{{Permission: durableElementRequirement.Permission, Scope: values.CoreElementScope(realmCoreRef, realmOtherID)}}},
		{name: "wrong FacetType", actor: durableLocalActor, requirements: []values.Requirement{{Permission: durableFacetRequirement.Permission, Scope: values.FacetKeyScope(fixtureFacetOwner, realmEntityAID, fixtureType("demo.other"))}}},
		{name: "wrong manager Realm scope", actor: durableManagerActor, requirements: []values.Requirement{{Permission: contract.PermissionPutBinding, Scope: values.RealmScope("realm-other")}}},
		{name: "missing all-of member", actor: durableLocalActor, requirements: []values.Requirement{durableElementRequirement, {Permission: permissionCompoundDestination, Scope: values.CoreElementScope(realmCoreRef, realmAnchorAID)}}},
		{name: "invalid actor", actor: values.PrincipalRef{}, requirements: []values.Requirement{durableElementRequirement}},
		{name: "invalid requirement", actor: durableLocalActor, requirements: []values.Requirement{{Scope: durableElementRequirement.Scope}}},
		{name: "empty requirements", actor: durableLocalActor},
		{name: "default denial", actor: values.PrincipalRef{Realm: "realm-unbound", Principal: "nobody"}, requirements: []values.Requirement{durableElementRequirement}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := policy.Authorize(test.actor, test.requirements...); got != test.want {
				t.Fatalf("Authorize: got %t, want %t", got, test.want)
			}
		})
	}
}

func assertRetiredRealm(
	t *testing.T,
	policy *Realm,
	knownRevision contract.BindingSetRevision,
	target contract.PermissionBinding,
) {
	t.Helper()
	if got := policy.BindingSetRevision(); got != (contract.BindingSetRevision{}) {
		t.Fatalf("retired BindingSetRevision: got %#v, want zero", got)
	}
	if policy.Authorize(durableLocalActor, durableElementRequirement) {
		t.Fatal("retired Realm authorized a binding")
	}
	if revision, err := policy.PutBinding(
		durableManagerActor,
		target,
		knownRevision,
		"durability-retired-put",
	); !errors.Is(err, contract.ErrStore) || revision != (contract.BindingSetRevision{}) {
		t.Fatalf("retired PutBinding: revision=%#v err=%v", revision, err)
	}
	if revision, err := policy.DeleteBinding(
		durableManagerActor,
		target,
		knownRevision,
		"durability-retired-delete",
	); !errors.Is(err, contract.ErrStore) || revision != (contract.BindingSetRevision{}) {
		t.Fatalf("retired DeleteBinding: revision=%#v err=%v", revision, err)
	}
	if err := policy.Close(); err != nil {
		t.Fatalf("repeated retired Close: %v", err)
	}
}

func mustPutDurableBinding(
	t *testing.T,
	policy *Realm,
	target contract.PermissionBinding,
	expected contract.BindingSetRevision,
	operationID contract.OperationID,
) contract.BindingSetRevision {
	t.Helper()
	revision, err := policy.PutBinding(
		durableManagerActor,
		target,
		expected,
		operationID,
	)
	if err != nil {
		t.Fatalf("PutBinding(%q): %v", operationID, err)
	}
	if revision == (contract.BindingSetRevision{}) || revision == expected {
		t.Fatalf("PutBinding(%q) revision: got %#v after %#v", operationID, revision, expected)
	}
	return revision
}

func mustDeleteDurableBinding(
	t *testing.T,
	policy *Realm,
	target contract.PermissionBinding,
	expected contract.BindingSetRevision,
	operationID contract.OperationID,
) contract.BindingSetRevision {
	t.Helper()
	revision, err := policy.DeleteBinding(
		durableManagerActor,
		target,
		expected,
		operationID,
	)
	if err != nil {
		t.Fatalf("DeleteBinding(%q): %v", operationID, err)
	}
	if revision == (contract.BindingSetRevision{}) || revision == expected {
		t.Fatalf("DeleteBinding(%q) revision: got %#v after %#v", operationID, revision, expected)
	}
	return revision
}

func assertExactPutReplay(
	t *testing.T,
	policy *Realm,
	target contract.PermissionBinding,
	expected contract.BindingSetRevision,
	want contract.BindingSetRevision,
) {
	t.Helper()
	got, err := policy.PutBinding(
		durableManagerActor,
		target,
		expected,
		"durability-profile-put",
	)
	if err != nil || got != want {
		t.Fatalf("exact Put replay: got %#v err=%v, want %#v", got, err, want)
	}
}

func assertExactDeleteReplay(
	t *testing.T,
	policy *Realm,
	target contract.PermissionBinding,
	expected contract.BindingSetRevision,
	want contract.BindingSetRevision,
) {
	t.Helper()
	got, err := policy.DeleteBinding(
		durableManagerActor,
		target,
		expected,
		"durability-profile-delete",
	)
	if err != nil || got != want {
		t.Fatalf("exact Delete replay: got %#v err=%v, want %#v", got, err, want)
	}
}

func assertRealmStoreLocked(t *testing.T, path string) {
	t.Helper()
	database, err := bbolt.Open(path, 0o600, &bbolt.Options{Timeout: 100 * time.Millisecond})
	if err == nil {
		_ = database.Close()
		t.Fatal("active Realm owner did not hold the Store lock")
	}
}

func assertRealmStoreUnlocked(t *testing.T, path string) {
	t.Helper()
	database, err := bbolt.Open(path, 0o600, &bbolt.Options{Timeout: 250 * time.Millisecond})
	if err != nil {
		t.Fatalf("Realm Store remained locked: %v", err)
	}
	if err := database.Close(); err != nil {
		t.Fatalf("close lock probe: %v", err)
	}
}

func assertRejectedRealmBytesUnchanged(t *testing.T, path string, content []byte) {
	t.Helper()
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatalf("write rejected Realm fixture: %v", err)
	}
	assertExistingRealmStoreRejectedUnchanged(t, path)
}

func assertExistingRealmStoreRejectedUnchanged(t *testing.T, path string) {
	t.Helper()
	before := mustReadFile(t, path)
	opened, err := OpenDurable(path)
	if !errors.Is(err, contract.ErrStore) || opened != nil {
		t.Fatalf("OpenDurable rejected fixture: Realm=%v err=%v, want nil/ErrStore", opened, err)
	}
	if errors.Is(err, contract.ErrInvalidSeed) {
		t.Fatalf("stored invalidity leaked caller-only ErrInvalidSeed: %v", err)
	}
	after := mustReadFile(t, path)
	if !bytes.Equal(after, before) {
		t.Fatal("rejected OpenDurable changed existing bytes")
	}
}

func mustCreateDurableRealm(t *testing.T, seed contract.Seed, path string) *Realm {
	t.Helper()
	value, err := CreateDurable(seed, path)
	if err != nil {
		t.Fatalf("CreateDurable(%q): %v", path, err)
	}
	if value == nil {
		t.Fatal("CreateDurable returned nil Realm without error")
	}
	t.Cleanup(func() {
		_ = value.Close()
	})
	return value
}

func mustOpenDurableRealm(t *testing.T, path string) *Realm {
	t.Helper()
	value, err := OpenDurable(path)
	if err != nil {
		t.Fatalf("OpenDurable(%q): %v", path, err)
	}
	if value == nil {
		t.Fatal("OpenDurable returned nil Realm without error")
	}
	t.Cleanup(func() {
		_ = value.Close()
	})
	return value
}

func mustReadFile(t *testing.T, path string) []byte {
	t.Helper()
	value, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %q: %v", path, err)
	}
	return value
}

// ownerTestAuthorizer answers every Realm contact port question allow. Store
// lifecycle probes never exercise address, view or denial semantics.
type ownerTestAuthorizer struct{}

func (ownerTestAuthorizer) Check(context.Context, values.NextHop, values.RealmID) error { return nil }
func (ownerTestAuthorizer) AuthorizePrincipal(values.NextHop, values.RealmID, values.PrincipalRef, ...values.Requirement) values.AuthorizationDecision {
	return values.AuthorizationValid
}
func (ownerTestAuthorizer) AuthorizeView(context.Context, values.NextHop, values.RealmID, values.CallerContext, ...values.Requirement) (values.AuthorizationDecision, values.AuthorizationView) {
	return values.AuthorizationValid, values.AuthorizationView{1}
}
func (ownerTestAuthorizer) ValidateView(context.Context, values.NextHop, values.RealmID, values.CallerContext, values.AuthorizationView, ...values.Requirement) values.AuthorizationDecision {
	return values.AuthorizationValid
}
func (ownerTestAuthorizer) CaptureDenialView(context.Context, values.NextHop, values.RealmID, values.CallerContext, values.Requirement) (values.AuthorizationDecision, values.AuthorizationDenialView) {
	return values.AuthorizationUnavailable, values.AuthorizationDenialView{}
}
func (ownerTestAuthorizer) ValidateDenialView(context.Context, values.NextHop, values.RealmID, values.CallerContext, values.AuthorizationDenialView, values.Requirement) values.AuthorizationDecision {
	return values.AuthorizationUnavailable
}

type ownerTestCoreSource struct{}

func (ownerTestCoreSource) ReadProposal(context.Context, values.NextHop, values.FacetAcceptanceKey) (facetcontract.CoreEvidence, error) {
	panic("Store lifecycle must not call Core")
}
func (ownerTestCoreSource) ReadAttachment(context.Context, values.NextHop, values.FacetAcceptanceKey) (facetcontract.CoreEvidence, error) {
	panic("Store lifecycle must not call Core")
}
func (ownerTestCoreSource) ReadSuccessor(context.Context, values.NextHop, values.FactAuthorityRef, values.ElementID) (facetcontract.CoreSuccessorRead, error) {
	panic("Store lifecycle must not call Core")
}
