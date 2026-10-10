package policy

import (
	"errors"
	"fmt"
	"sync"
	"testing"

	"ucupa/shared/values"
	contract "ucupa/ucupa-realm/contract"
	token "ucupa/ucupa-realm/internal/token"
)

func TestAdministrationSeedBindingsRequireExactLocalShape(t *testing.T) {
	manager := values.PrincipalRef{Realm: "realm-a", Principal: "manager"}
	valid := contract.Seed{
		ID:              "realm-a",
		LocalPrincipals: []values.PrincipalID{"manager"},
		Bindings: []contract.PermissionBinding{
			administrationBinding(manager, contract.PermissionPutBinding, "realm-a"),
			administrationBinding(manager, contract.PermissionDeleteBinding, "realm-a"),
		},
	}
	policy := mustRealm(t, valid)
	if !policy.Authorize(manager,
		values.Requirement{Permission: contract.PermissionPutBinding, Scope: values.RealmScope("realm-a")},
		values.Requirement{Permission: contract.PermissionDeleteBinding, Scope: values.RealmScope("realm-a")},
	) {
		t.Fatal("exact registered local bootstrap manager bindings must authorize")
	}

	tests := []struct {
		name    string
		binding contract.PermissionBinding
	}{
		{
			name: "unregistered same-Realm manager",
			binding: administrationBinding(
				values.PrincipalRef{Realm: "realm-a", Principal: "missing"},
				contract.PermissionPutBinding,
				"realm-a",
			),
		},
		{
			name: "remote manager",
			binding: administrationBinding(
				values.PrincipalRef{Realm: "realm-b", Principal: "manager"},
				contract.PermissionPutBinding,
				"realm-a",
			),
		},
		{
			name: "wrong Realm target",
			binding: administrationBinding(
				manager,
				contract.PermissionDeleteBinding,
				"realm-b",
			),
		},
		{
			name: "Put permission with Element scope",
			binding: contract.PermissionBinding{
				Subject: manager, Permission: contract.PermissionPutBinding, Scope: values.CoreElementScope(realmCoreRef, realmSiteID),
			},
		},
		{
			name: "Delete permission with Facet scope",
			binding: contract.PermissionBinding{
				Subject: manager, Permission: contract.PermissionDeleteBinding,
				Scope: values.FacetKeyScope(fixtureFacetOwner, realmEntityAID, fixtureType("demo.profile")),
			},
		},
		{
			name: "operational permission with Realm scope",
			binding: contract.PermissionBinding{
				Subject: manager, Permission: permissionReadElement, Scope: values.RealmScope("realm-a"),
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			seed := contract.Seed{
				ID:              "realm-a",
				LocalPrincipals: []values.PrincipalID{"manager"},
				Bindings:        []contract.PermissionBinding{test.binding},
			}
			if got, err := New(seed); !errors.Is(err, contract.ErrInvalidSeed) || got != nil {
				t.Fatalf("New invalid administration Seed: Realm=%v err=%v, want nil/ErrInvalidSeed", got, err)
			}
		})
	}
}

func TestSealedPutOnlyAndDeleteOnlyAdministration(t *testing.T) {
	manager := values.PrincipalRef{Realm: "realm-a", Principal: "manager"}
	target := contract.PermissionBinding{
		Subject: manager, Permission: permissionReadElement, Scope: values.CoreElementScope(realmCoreRef, realmSiteID),
	}

	t.Run("sealed", func(t *testing.T) {
		policy := mustRealm(t, contract.Seed{ID: "realm-a", LocalPrincipals: []values.PrincipalID{"manager"}})
		initial := policy.BindingSetRevision()
		if !token.Valid(initial) {
			t.Fatal("sealed Realm must still expose a valid initial binding-set revision")
		}
		got, err := policy.PutBinding(manager, target, initial, "sealed-put")
		assertBindingMutationError(t, got, err, contract.ErrDenied)
		got, err = policy.DeleteBinding(manager, target, initial, "sealed-delete")
		assertBindingMutationError(t, got, err, contract.ErrDenied)
		if policy.BindingSetRevision() != initial {
			t.Fatal("denied operations changed the sealed Realm revision")
		}
	})

	t.Run("Put only", func(t *testing.T) {
		policy := mustRealm(t, contract.Seed{
			ID:              "realm-a",
			LocalPrincipals: []values.PrincipalID{"manager"},
			Bindings: []contract.PermissionBinding{
				administrationBinding(manager, contract.PermissionPutBinding, "realm-a"),
			},
		})
		initial := policy.BindingSetRevision()
		putRevision, err := policy.PutBinding(manager, target, initial, "put-only-put")
		if err != nil || !token.Valid(putRevision) || putRevision == initial {
			t.Fatalf("Put-only manager Put: revision=%#v err=%v", putRevision, err)
		}
		if !policy.Authorize(manager, bindingRequirement(target)) {
			t.Fatal("Put-only manager did not publish its operational binding")
		}
		got, err := policy.DeleteBinding(manager, target, putRevision, "put-only-delete")
		assertBindingMutationError(t, got, err, contract.ErrDenied)
	})

	t.Run("Delete only", func(t *testing.T) {
		policy := mustRealm(t, contract.Seed{
			ID:              "realm-a",
			LocalPrincipals: []values.PrincipalID{"manager"},
			Bindings: []contract.PermissionBinding{
				administrationBinding(manager, contract.PermissionDeleteBinding, "realm-a"),
				target,
			},
		})
		initial := policy.BindingSetRevision()
		deleteRevision, err := policy.DeleteBinding(manager, target, initial, "delete-only-delete")
		if err != nil || !token.Valid(deleteRevision) || deleteRevision == initial {
			t.Fatalf("Delete-only manager Delete: revision=%#v err=%v", deleteRevision, err)
		}
		if policy.Authorize(manager, bindingRequirement(target)) {
			t.Fatal("Delete-only manager did not remove its operational binding")
		}
		got, err := policy.PutBinding(manager, target, deleteRevision, "delete-only-put")
		assertBindingMutationError(t, got, err, contract.ErrDenied)
	})
}

func TestOnlineAdministrationTargetsAreRejectedBeforeOperationRetention(t *testing.T) {
	manager := values.PrincipalRef{Realm: "realm-a", Principal: "manager"}
	validTarget := contract.PermissionBinding{
		Subject: manager, Permission: permissionReadElement, Scope: values.CoreElementScope(realmCoreRef, realmSiteID),
	}
	tests := []struct {
		name   string
		target contract.PermissionBinding
	}{
		{
			name:   "exact Put manager binding",
			target: administrationBinding(manager, contract.PermissionPutBinding, "realm-a"),
		},
		{
			name:   "exact Delete manager binding",
			target: administrationBinding(manager, contract.PermissionDeleteBinding, "realm-a"),
		},
		{
			name:   "exact Rotate-own lifecycle binding",
			target: administrationBinding(manager, contract.PermissionRotateOwnCredential, "realm-a"),
		},
		{
			name:   "exact Revoke lifecycle binding",
			target: administrationBinding(manager, contract.PermissionRevokeCredential, "realm-a"),
		},
		{
			name: "Realm permission with Element scope",
			target: contract.PermissionBinding{
				Subject: manager, Permission: contract.PermissionDeleteBinding, Scope: values.CoreElementScope(realmCoreRef, realmSiteID),
			},
		},
		{
			name: "operational permission with current Realm scope",
			target: contract.PermissionBinding{
				Subject: manager, Permission: permissionReadElement, Scope: values.RealmScope("realm-a"),
			},
		},
		{
			name: "operational permission with another Realm scope",
			target: contract.PermissionBinding{
				Subject: manager, Permission: permissionReadElement, Scope: values.RealmScope("realm-b"),
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			policy := realmWithManagers(t, manager)
			initial := policy.BindingSetRevision()
			operationID := contract.OperationID("invalid-online-target")
			got, err := policy.PutBinding(manager, test.target, initial, operationID)
			assertBindingMutationError(t, got, err, contract.ErrInvalid)
			got, err = policy.DeleteBinding(manager, test.target, initial, operationID)
			assertBindingMutationError(t, got, err, contract.ErrInvalid)
			if policy.BindingSetRevision() != initial {
				t.Fatal("invalid online target changed the binding-set revision")
			}

			// Invalid requests do not consume an OperationID.
			putRevision, err := policy.PutBinding(manager, validTarget, initial, operationID)
			if err != nil || putRevision == initial {
				t.Fatalf("valid Put after invalid request: revision=%#v err=%v", putRevision, err)
			}
		})
	}
}

func TestAdministrationOrderingAuthorizationBeforeReplayAndCASBeforeNotFound(t *testing.T) {
	manager := values.PrincipalRef{Realm: "realm-a", Principal: "manager"}
	unbound := values.PrincipalRef{Realm: "realm-a", Principal: "unbound"}
	policy := mustRealm(t, contract.Seed{
		ID:              "realm-a",
		LocalPrincipals: []values.PrincipalID{"manager", "unbound"},
		Bindings: []contract.PermissionBinding{
			administrationBinding(manager, contract.PermissionPutBinding, "realm-a"),
			administrationBinding(manager, contract.PermissionDeleteBinding, "realm-a"),
		},
	})
	target := contract.PermissionBinding{
		Subject: manager, Permission: permissionReadElement, Scope: values.CoreElementScope(realmCoreRef, realmSiteID),
	}
	initial := policy.BindingSetRevision()
	committed, err := policy.PutBinding(manager, target, initial, "known-operation")
	if err != nil {
		t.Fatalf("seed retained operation: %v", err)
	}

	// The changed actor would be an operation conflict if replay lookup ran
	// before the current manager authorization check.
	got, err := policy.PutBinding(unbound, target, initial, "known-operation")
	assertBindingMutationError(t, got, err, contract.ErrDenied)

	missingTarget := contract.PermissionBinding{
		Subject:    values.PrincipalRef{Realm: "realm-a", Principal: "missing"},
		Permission: permissionReadElement,
		Scope:      values.CoreElementScope(realmCoreRef, realmOtherID),
	}
	got, err = policy.PutBinding(manager, missingTarget, initial, "stale-before-not-found")
	assertBindingMutationError(t, got, err, contract.ErrRevisionConflict)
	got, err = policy.PutBinding(manager, missingTarget, committed, "fresh-not-found")
	assertBindingMutationError(t, got, err, contract.ErrNotFound)

	// Both definitive negative results replay without reevaluating current state.
	got, err = policy.PutBinding(manager, missingTarget, initial, "stale-before-not-found")
	assertBindingMutationError(t, got, err, contract.ErrRevisionConflict)
	got, err = policy.PutBinding(manager, missingTarget, committed, "fresh-not-found")
	assertBindingMutationError(t, got, err, contract.ErrNotFound)
}

func TestAdministrationRetainsChangedNoopAndNegativeOutcomes(t *testing.T) {
	manager := values.PrincipalRef{Realm: "realm-a", Principal: "manager"}
	policy := realmWithManagers(t, manager)
	targetA := contract.PermissionBinding{
		Subject: manager, Permission: permissionReadElement, Scope: values.CoreElementScope(realmCoreRef, realmSiteID),
	}
	targetB := contract.PermissionBinding{
		Subject: manager, Permission: permissionReadElement, Scope: values.CoreElementScope(realmCoreRef, realmOtherID),
	}
	targetC := contract.PermissionBinding{
		Subject: manager, Permission: permissionDeleteEntity, Scope: values.CoreElementScope(realmCoreRef, realmEntityAID),
	}
	targetD := contract.PermissionBinding{
		Subject: manager, Permission: permissionDeleteEntity, Scope: values.CoreElementScope(realmCoreRef, realmEntityBID),
	}
	missing := contract.PermissionBinding{
		Subject:    values.PrincipalRef{Realm: "realm-a", Principal: "missing"},
		Permission: permissionReadElement,
		Scope:      values.CoreElementScope(realmCoreRef, realmRemoteTargetID),
	}

	revision0 := policy.BindingSetRevision()
	revision1 := mustPutBinding(t, policy, manager, targetA, revision0, "put-changed")
	if revision1 == revision0 {
		t.Fatal("changed Put retained the initial revision")
	}
	if got := mustPutBinding(t, policy, manager, targetA, revision1, "put-noop"); got != revision1 {
		t.Fatalf("Put-present advanced revision: got %#v, want %#v", got, revision1)
	}
	revision2 := mustPutBinding(t, policy, manager, targetB, revision1, "later-put")

	got, err := policy.PutBinding(manager, missing, revision2, "not-found")
	assertBindingMutationError(t, got, err, contract.ErrNotFound)
	got, err = policy.PutBinding(manager, targetC, revision1, "revision-conflict")
	assertBindingMutationError(t, got, err, contract.ErrRevisionConflict)

	revision3 := mustDeleteBinding(t, policy, manager, targetA, revision2, "delete-changed")
	if revision3 == revision2 {
		t.Fatal("changed Delete retained the previous revision")
	}
	if got := mustDeleteBinding(t, policy, manager, targetA, revision3, "delete-noop"); got != revision3 {
		t.Fatalf("Delete-absent advanced revision: got %#v, want %#v", got, revision3)
	}
	revision4 := mustPutBinding(t, policy, manager, targetD, revision3, "later-put-after-delete")

	if got := mustPutBinding(t, policy, manager, targetA, revision0, "put-changed"); got != revision1 {
		t.Fatalf("changed Put replay: got %#v, want %#v", got, revision1)
	}
	if got := mustPutBinding(t, policy, manager, targetA, revision1, "put-noop"); got != revision1 {
		t.Fatalf("Put no-op replay: got %#v, want %#v", got, revision1)
	}
	got, err = policy.PutBinding(manager, missing, revision2, "not-found")
	assertBindingMutationError(t, got, err, contract.ErrNotFound)
	got, err = policy.PutBinding(manager, targetC, revision1, "revision-conflict")
	assertBindingMutationError(t, got, err, contract.ErrRevisionConflict)
	if got := mustDeleteBinding(t, policy, manager, targetA, revision2, "delete-changed"); got != revision3 {
		t.Fatalf("changed Delete replay: got %#v, want %#v", got, revision3)
	}
	if got := mustDeleteBinding(t, policy, manager, targetA, revision3, "delete-noop"); got != revision3 {
		t.Fatalf("Delete no-op replay: got %#v, want %#v", got, revision3)
	}

	if got := policy.BindingSetRevision(); got != revision4 {
		t.Fatalf("historical replay changed current revision: got %#v, want %#v", got, revision4)
	}
	if policy.Authorize(manager, bindingRequirement(targetA)) {
		t.Fatal("historical replay restored the deleted target")
	}
	if !policy.Authorize(manager, bindingRequirement(targetB), bindingRequirement(targetD)) {
		t.Fatal("historical replay changed later bindings")
	}
}

func TestOperationIDConflictsOnEachBoundField(t *testing.T) {
	managerA := values.PrincipalRef{Realm: "realm-a", Principal: "manager-a"}
	managerB := values.PrincipalRef{Realm: "realm-a", Principal: "manager-b"}
	policy := realmWithManagers(t, managerA, managerB)
	targetA := contract.PermissionBinding{
		Subject: managerA, Permission: permissionReadElement, Scope: values.CoreElementScope(realmCoreRef, realmSiteID),
	}
	targetB := contract.PermissionBinding{
		Subject: managerA, Permission: permissionReadElement, Scope: values.CoreElementScope(realmCoreRef, realmOtherID),
	}
	initial := policy.BindingSetRevision()
	committed := mustPutBinding(t, policy, managerA, targetA, initial, "four-fields")

	tests := []struct {
		name   string
		mutate func() (contract.BindingSetRevision, error)
	}{
		{
			name: "actor",
			mutate: func() (contract.BindingSetRevision, error) {
				return policy.PutBinding(managerB, targetA, initial, "four-fields")
			},
		},
		{
			name: "operation kind",
			mutate: func() (contract.BindingSetRevision, error) {
				return policy.DeleteBinding(managerA, targetA, initial, "four-fields")
			},
		},
		{
			name: "complete target binding",
			mutate: func() (contract.BindingSetRevision, error) {
				return policy.PutBinding(managerA, targetB, initial, "four-fields")
			},
		},
		{
			name: "expected revision",
			mutate: func() (contract.BindingSetRevision, error) {
				return policy.PutBinding(managerA, targetA, committed, "four-fields")
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := test.mutate()
			assertBindingMutationError(t, got, err, contract.ErrOperationConflict)
			if policy.BindingSetRevision() != committed {
				t.Fatal("operation conflict changed current revision")
			}
		})
	}
	if got := mustPutBinding(t, policy, managerA, targetA, initial, "four-fields"); got != committed {
		t.Fatalf("exact replay after conflicts: got %#v, want %#v", got, committed)
	}
	if policy.Authorize(managerA, bindingRequirement(targetB)) {
		t.Fatal("conflicting target mutation was applied")
	}
}

func TestBindingSetCASSerializesTwoWritersAndAllowsWorkAfterNoop(t *testing.T) {
	manager := values.PrincipalRef{Realm: "realm-a", Principal: "manager"}
	policy := realmWithManagers(t, manager)
	target := contract.PermissionBinding{
		Subject: manager, Permission: permissionReadElement, Scope: values.CoreElementScope(realmCoreRef, realmSiteID),
	}
	initial := policy.BindingSetRevision()
	type result struct {
		id       contract.OperationID
		revision contract.BindingSetRevision
		err      error
	}
	start := make(chan struct{})
	results := make(chan result, 2)
	for _, id := range []contract.OperationID{"writer-a", "writer-b"} {
		id := id
		go func() {
			<-start
			revision, err := policy.PutBinding(manager, target, initial, id)
			results <- result{id: id, revision: revision, err: err}
		}()
	}
	close(start)
	first, second := <-results, <-results

	successes := 0
	conflicts := 0
	var committed contract.BindingSetRevision
	for _, got := range []result{first, second} {
		switch {
		case got.err == nil:
			successes++
			committed = got.revision
			if !token.Valid(got.revision) || got.revision == initial {
				t.Fatalf("writer %q returned invalid changed revision %#v", got.id, got.revision)
			}
			if replay := mustPutBinding(t, policy, manager, target, initial, got.id); replay != got.revision {
				t.Fatalf("writer %q replay: got %#v, want %#v", got.id, replay, got.revision)
			}
		case errors.Is(got.err, contract.ErrRevisionConflict):
			conflicts++
			if got.revision != (contract.BindingSetRevision{}) {
				t.Fatalf("writer %q conflict returned revision %#v", got.id, got.revision)
			}
			replay, err := policy.PutBinding(manager, target, initial, got.id)
			assertBindingMutationError(t, replay, err, contract.ErrRevisionConflict)
		default:
			t.Fatalf("writer %q unexpected result: revision=%#v err=%v", got.id, got.revision, got.err)
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("two writers: successes=%d conflicts=%d, want 1/1", successes, conflicts)
	}
	if policy.BindingSetRevision() != committed || !policy.Authorize(manager, bindingRequirement(target)) {
		t.Fatal("two-writer winner was not the complete published state")
	}

	// A distinct no-op does not advance the aggregate revision, so another
	// operation using the same expected revision is still evaluated normally.
	if got := mustPutBinding(t, policy, manager, target, committed, "writer-noop"); got != committed {
		t.Fatalf("no-op writer advanced revision: got %#v, want %#v", got, committed)
	}
	other := contract.PermissionBinding{
		Subject: manager, Permission: permissionReadElement, Scope: values.CoreElementScope(realmCoreRef, realmOtherID),
	}
	afterOther := mustPutBinding(t, policy, manager, other, committed, "writer-after-noop")
	if afterOther == committed || !policy.Authorize(manager, bindingRequirement(other)) {
		t.Fatal("operation after no-op did not commit normally")
	}
}

func TestConcurrentAdministrationAndReadsExposeCompleteInMemoryStates(t *testing.T) {
	manager := values.PrincipalRef{Realm: "realm-a", Principal: "manager"}
	policy := realmWithManagers(t, manager)
	target := contract.PermissionBinding{
		Subject: manager, Permission: permissionReadElement, Scope: values.CoreElementScope(realmCoreRef, realmSiteID),
	}
	initial := policy.BindingSetRevision()

	const writerCount = 32
	type result struct {
		revision contract.BindingSetRevision
		err      error
	}
	start := make(chan struct{})
	stopReaders := make(chan struct{})
	results := make(chan result, writerCount)
	readerErrors := make(chan error, 8)
	var readers sync.WaitGroup
	for reader := 0; reader < 4; reader++ {
		readers.Add(1)
		go func() {
			defer readers.Done()
			<-start
			for {
				select {
				case <-stopReaders:
					return
				default:
				}
				if revision := policy.BindingSetRevision(); !token.Valid(revision) {
					readerErrors <- fmt.Errorf("reader observed invalid revision %#v", revision)
					return
				}
				_ = policy.Authorize(manager, bindingRequirement(target))
				if err := policy.Close(); err != nil {
					readerErrors <- fmt.Errorf("in-memory Close: %w", err)
					return
				}
			}
		}()
	}
	for writer := 0; writer < writerCount; writer++ {
		operationID := contract.OperationID(fmt.Sprintf("concurrent-writer-%d", writer))
		go func() {
			<-start
			revision, err := policy.PutBinding(manager, target, initial, operationID)
			results <- result{revision: revision, err: err}
		}()
	}
	close(start)

	successes := 0
	conflicts := 0
	var committed contract.BindingSetRevision
	for range writerCount {
		got := <-results
		switch {
		case got.err == nil:
			successes++
			committed = got.revision
		case errors.Is(got.err, contract.ErrRevisionConflict):
			conflicts++
			if got.revision != (contract.BindingSetRevision{}) {
				t.Fatalf("concurrent conflict returned revision %#v", got.revision)
			}
		default:
			t.Fatalf("concurrent writer: revision=%#v err=%v", got.revision, got.err)
		}
	}
	close(stopReaders)
	readers.Wait()
	close(readerErrors)
	for err := range readerErrors {
		t.Fatal(err)
	}

	if successes != 1 || conflicts != writerCount-1 {
		t.Fatalf("concurrent writers: successes=%d conflicts=%d, want 1/%d", successes, conflicts, writerCount-1)
	}
	if got := policy.BindingSetRevision(); got != committed {
		t.Fatalf("final concurrent revision: got %#v, want %#v", got, committed)
	}
	if !policy.Authorize(manager, bindingRequirement(target)) {
		t.Fatal("final concurrent binding was not published")
	}
}

func realmWithManagers(t *testing.T, managers ...values.PrincipalRef) *Realm {
	t.Helper()
	seed := contract.Seed{ID: "realm-a"}
	for _, manager := range managers {
		if manager.Realm != seed.ID {
			t.Fatalf("test manager %v does not belong to %q", manager, seed.ID)
		}
		seed.LocalPrincipals = append(seed.LocalPrincipals, manager.Principal)
		seed.Bindings = append(seed.Bindings,
			administrationBinding(manager, contract.PermissionPutBinding, seed.ID),
			administrationBinding(manager, contract.PermissionDeleteBinding, seed.ID),
		)
	}
	return mustRealm(t, seed)
}

func administrationBinding(
	subject values.PrincipalRef,
	permission values.PermissionID,
	realmID values.RealmID,
) contract.PermissionBinding {
	return contract.PermissionBinding{
		Subject: subject, Permission: permission, Scope: values.RealmScope(realmID),
	}
}

func bindingRequirement(binding contract.PermissionBinding) values.Requirement {
	return values.Requirement{Permission: binding.Permission, Scope: binding.Scope}
}

func mustPutBinding(
	t *testing.T,
	policy *Realm,
	actor values.PrincipalRef,
	target contract.PermissionBinding,
	expected contract.BindingSetRevision,
	operationID contract.OperationID,
) contract.BindingSetRevision {
	t.Helper()
	revision, err := policy.PutBinding(actor, target, expected, operationID)
	if err != nil {
		t.Fatalf("PutBinding(%q): %v", operationID, err)
	}
	if !token.Valid(revision) {
		t.Fatalf("PutBinding(%q) returned invalid revision %#v", operationID, revision)
	}
	return revision
}

func mustDeleteBinding(
	t *testing.T,
	policy *Realm,
	actor values.PrincipalRef,
	target contract.PermissionBinding,
	expected contract.BindingSetRevision,
	operationID contract.OperationID,
) contract.BindingSetRevision {
	t.Helper()
	revision, err := policy.DeleteBinding(actor, target, expected, operationID)
	if err != nil {
		t.Fatalf("DeleteBinding(%q): %v", operationID, err)
	}
	if !token.Valid(revision) {
		t.Fatalf("DeleteBinding(%q) returned invalid revision %#v", operationID, revision)
	}
	return revision
}

func assertBindingMutationError(
	t *testing.T,
	revision contract.BindingSetRevision,
	err error,
	want error,
) {
	t.Helper()
	if !errors.Is(err, want) {
		t.Fatalf("mutation error: got %v, want %v", err, want)
	}
	if revision != (contract.BindingSetRevision{}) {
		t.Fatalf("failed mutation returned revision %#v", revision)
	}
}
