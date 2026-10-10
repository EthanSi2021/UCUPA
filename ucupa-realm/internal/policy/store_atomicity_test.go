package policy

import (
	"bytes"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"go.etcd.io/bbolt"

	"ucupa/shared/values"
	contract "ucupa/ucupa-realm/contract"
	token "ucupa/ucupa-realm/internal/token"
)

func TestOpenDurableRejectsMalformedOrForeignRealmStoreWithoutModification(t *testing.T) {
	validNoop := atomicIndependentOperation(
		1, atomicManager, atomicIndependentBinding(atomicElementBinding), atomicFixtureRevision(),
		1, atomicRevisionPointer(atomicFixtureRevision()),
	)
	foreignResult := atomicRevision(0x44, 1)
	foreignSuccess := atomicIndependentOperation(
		1, atomicManager, atomicIndependentBinding(atomicElementBinding), atomicFixtureRevision(),
		1, &foreignResult,
	)
	ordinaryRealmBinding := atomicIndependentBinding(contract.PermissionBinding{
		Subject: atomicSubject, Permission: atomicPermission, Scope: values.RealmScope(atomicRealmID),
	})

	tests := []struct {
		name   string
		mutate func(*bbolt.Tx) error
	}{
		{
			name: "arbitrary foreign marker",
			mutate: func(tx *bbolt.Tx) error {
				return tx.Bucket(atomicMetaBucket).Put(atomicFormatKey, []byte("ucupa.realm.local@foreign"))
			},
		},
		{
			name: "missing marker",
			mutate: func(tx *bbolt.Tx) error {
				return tx.Bucket(atomicMetaBucket).Delete(atomicFormatKey)
			},
		},
		{
			name:   "missing operation bucket",
			mutate: func(tx *bbolt.Tx) error { return tx.DeleteBucket(atomicOperationBucket) },
		},
		{
			name: "extra top-level bucket",
			mutate: func(tx *bbolt.Tx) error {
				_, err := tx.CreateBucket([]byte("extra"))
				return err
			},
		},
		{
			name: "malformed revision scope",
			mutate: func(tx *bbolt.Tx) error {
				return tx.Bucket(atomicMetaBucket).Put(atomicRevisionScopeKey, bytes.Repeat([]byte{1}, 31))
			},
		},
		{
			name: "maximum revision generation without history",
			mutate: func(tx *bbolt.Tx) error {
				return tx.Bucket(atomicMetaBucket).Put(
					atomicRevisionGenerationKey,
					bytes.Repeat([]byte{0xff}, 8),
				)
			},
		},
		{
			name: "operation bucket sequence",
			mutate: func(tx *bbolt.Tx) error {
				return tx.Bucket(atomicOperationBucket).SetSequence(1)
			},
		},
		{
			name: "malformed Realm binding",
			mutate: func(tx *bbolt.Tx) error {
				return tx.Bucket(atomicBindingBucket).Put(ordinaryRealmBinding, nil)
			},
		},
		{
			name: "malformed operation trailing byte",
			mutate: func(tx *bbolt.Tx) error {
				return tx.Bucket(atomicOperationBucket).Put([]byte("op-malformed"), append(validNoop, 0))
			},
		},
		{
			name: "foreign success revision",
			mutate: func(tx *bbolt.Tx) error {
				return tx.Bucket(atomicOperationBucket).Put([]byte("op-foreign"), foreignSuccess)
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "realm.db")
			writeAtomicIndependentFixture(t, path, test.mutate)
			assertAtomicOpenRejectedUnchanged(t, path)
		})
	}
}

func TestRealmDefinitePrecommitFailureDoesNotConsumeChangedOperation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "realm.db")
	owner := mustCreateAtomicRealm(t, path)
	defer func() { _ = owner.Close() }()
	before := owner.BindingSetRevision()
	operationID := contract.OperationID("changed-precommit")
	owner.state.store.failBeforeCommit = atomicInjectedFailure("changed precommit")

	got, err := owner.PutBinding(atomicManager, atomicElementBinding, before, operationID)
	if !errors.Is(err, contract.ErrStore) || got != (contract.BindingSetRevision{}) {
		t.Fatalf("failed changed operation: revision=%#v err=%v, want zero/ErrStore", got, err)
	}
	assertAtomicOwnerActiveAt(t, owner, before)
	assertAtomicBindingDecision(t, owner, atomicElementBinding, false)
	assertAtomicOperationCount(t, owner, 0)

	got, err = owner.PutBinding(atomicManager, atomicElementBinding, before, operationID)
	if err != nil || token.Scope(got) != token.Scope(before) || token.Generation(got) != token.Generation(before)+1 {
		t.Fatalf("retry changed operation: revision=%#v err=%v", got, err)
	}
	assertAtomicBindingDecision(t, owner, atomicElementBinding, true)
	assertAtomicOperationCount(t, owner, 1)
}

func TestRealmOnlineCapacityFailureIsDefinitePrecommit(t *testing.T) {
	t.Run("oversized OperationID remains unconsumed", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "realm.db")
		owner := mustCreateAtomicRealm(t, path)
		defer func() { _ = owner.Close() }()
		before := owner.BindingSetRevision()
		operationID := contract.OperationID(strings.Repeat("o", bbolt.MaxKeySize+1))

		for attempt := 1; attempt <= 2; attempt++ {
			got, err := owner.PutBinding(atomicManager, atomicElementBinding, before, operationID)
			if !errors.Is(err, contract.ErrStore) || got != (contract.BindingSetRevision{}) {
				t.Fatalf("oversized OperationID attempt %d: revision=%#v err=%v, want zero/ErrStore",
					attempt, got, err)
			}
			assertAtomicOwnerActiveAt(t, owner, before)
			assertAtomicBindingDecision(t, owner, atomicElementBinding, false)
			assertAtomicOperationCount(t, owner, 0)
		}
	})

	t.Run("oversized binding leaves OperationID reusable", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "realm.db")
		owner := mustCreateAtomicRealm(t, path)
		defer func() { _ = owner.Close() }()
		before := owner.BindingSetRevision()
		operationID := contract.OperationID("binding-capacity-reuse")
		oversized := contract.PermissionBinding{
			Subject:    atomicSubject,
			Permission: values.PermissionID(strings.Repeat("f", bbolt.MaxKeySize)),
			Scope: values.FacetKeyScope(fixtureFacetOwner,
				atomicElementID,
				fixtureType("capacity"),
			),
		}

		got, err := owner.PutBinding(atomicManager, oversized, before, operationID)
		if !errors.Is(err, contract.ErrStore) || got != (contract.BindingSetRevision{}) {
			t.Fatalf("oversized binding: revision=%#v err=%v, want zero/ErrStore", got, err)
		}
		assertAtomicOwnerActiveAt(t, owner, before)
		assertAtomicBindingDecision(t, owner, oversized, false)
		assertAtomicOperationCount(t, owner, 0)

		got, err = owner.PutBinding(atomicManager, atomicElementBinding, before, operationID)
		if err != nil || got == (contract.BindingSetRevision{}) || got == before {
			t.Fatalf("reuse OperationID after capacity failure: revision=%#v err=%v", got, err)
		}
		assertAtomicBindingDecision(t, owner, atomicElementBinding, true)
		assertAtomicOperationCount(t, owner, 1)
	})
}

func TestRealmDefinitePrecommitFailureDoesNotConsumeOutcomeOnlyOperation(t *testing.T) {
	tests := []struct {
		name           string
		prepare        func(*testing.T, *Realm) (contract.PermissionBinding, contract.BindingSetRevision, int)
		wantRevision   bool
		wantError      error
		wantAuthorized bool
	}{
		{
			name: "no-op",
			prepare: func(_ *testing.T, owner *Realm) (contract.PermissionBinding, contract.BindingSetRevision, int) {
				return atomicElementBinding, owner.BindingSetRevision(), 0
			},
			wantRevision: true, wantAuthorized: true,
		},
		{
			name: "not found",
			prepare: func(_ *testing.T, owner *Realm) (contract.PermissionBinding, contract.BindingSetRevision, int) {
				target := atomicElementBinding
				target.Subject.Principal = "missing"
				return target, owner.BindingSetRevision(), 0
			},
			wantError: contract.ErrNotFound,
		},
		{
			name: "revision conflict",
			prepare: func(t *testing.T, owner *Realm) (contract.PermissionBinding, contract.BindingSetRevision, int) {
				stale := owner.BindingSetRevision()
				advance := atomicLaterBinding("advance-precommit")
				got, err := owner.PutBinding(atomicManager, advance, stale, "prepare-stale-precommit")
				if err != nil || token.Generation(got) != token.Generation(stale)+1 {
					t.Fatalf("prepare stale revision: revision=%#v err=%v", got, err)
				}
				return atomicElementBinding, stale, 1
			},
			wantError: contract.ErrRevisionConflict,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "realm.db")
			extra := []contract.PermissionBinding(nil)
			if test.name == "no-op" {
				extra = append(extra, atomicElementBinding)
			}
			owner := mustCreateAtomicRealm(t, path, extra...)
			defer func() { _ = owner.Close() }()
			target, expected, operationCount := test.prepare(t, owner)
			current := owner.BindingSetRevision()
			operationID := contract.OperationID("outcome-precommit-" + test.name)
			owner.state.store.failBeforeCommit = atomicInjectedFailure(test.name + " precommit")

			got, err := owner.PutBinding(atomicManager, target, expected, operationID)
			if !errors.Is(err, contract.ErrStore) || got != (contract.BindingSetRevision{}) {
				t.Fatalf("failed outcome-only operation: revision=%#v err=%v, want zero/ErrStore", got, err)
			}
			assertAtomicOwnerActiveAt(t, owner, current)
			assertAtomicBindingDecision(t, owner, target, test.wantAuthorized)
			assertAtomicOperationCount(t, owner, operationCount)

			got, err = owner.PutBinding(atomicManager, target, expected, operationID)
			if !errors.Is(err, test.wantError) {
				t.Fatalf("retry outcome-only operation: got err %v, want %v", err, test.wantError)
			}
			if test.wantRevision {
				if got != current {
					t.Fatalf("retry no-op revision: got %#v, want %#v", got, current)
				}
			} else if got != (contract.BindingSetRevision{}) {
				t.Fatalf("retry error revision: got %#v, want zero", got)
			}
			assertAtomicOperationCount(t, owner, operationCount+1)
		})
	}
}

func TestRealmFailAfterCommitRetiresAndReopenReplaysChangedOperation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "realm.db")
	owner := mustCreateAtomicRealm(t, path)
	before := owner.BindingSetRevision()
	operationID := contract.OperationID("changed-lost-response")
	owner.state.store.failAfterCommit = atomicInjectedFailure("changed lost response")

	got, err := owner.PutBinding(atomicManager, atomicElementBinding, before, operationID)
	if !errors.Is(err, contract.ErrStore) || got != (contract.BindingSetRevision{}) {
		t.Fatalf("indeterminate changed operation: revision=%#v err=%v, want zero/ErrStore", got, err)
	}
	assertAtomicOwnerRetired(t, owner)

	reopened := mustOpenAtomicRealm(t, path)
	defer func() { _ = reopened.Close() }()
	want := token.Binding(token.Scope(before), token.Generation(before)+1)
	if got := reopened.BindingSetRevision(); got != want {
		t.Fatalf("reopened changed revision: got %#v, want %#v", got, want)
	}
	assertAtomicBindingDecision(t, reopened, atomicElementBinding, true)
	got, err = reopened.PutBinding(atomicManager, atomicElementBinding, before, operationID)
	if err != nil || got != want {
		t.Fatalf("exact changed replay: revision=%#v err=%v, want %#v/nil", got, err, want)
	}
	assertAtomicOperationCount(t, reopened, 1)
}

func TestRealmFailAfterCommitRetiresAndReopenReplaysOutcomeOnlyOperation(t *testing.T) {
	tests := []struct {
		name           string
		seedTarget     bool
		prepare        func(*testing.T, *Realm) (contract.PermissionBinding, contract.BindingSetRevision, int)
		wantRevision   bool
		wantError      error
		wantAuthorized bool
	}{
		{
			name: "no-op", seedTarget: true,
			prepare: func(_ *testing.T, owner *Realm) (contract.PermissionBinding, contract.BindingSetRevision, int) {
				return atomicElementBinding, owner.BindingSetRevision(), 0
			},
			wantRevision: true, wantAuthorized: true,
		},
		{
			name: "not found",
			prepare: func(_ *testing.T, owner *Realm) (contract.PermissionBinding, contract.BindingSetRevision, int) {
				target := atomicElementBinding
				target.Subject.Principal = "missing"
				return target, owner.BindingSetRevision(), 0
			},
			wantError: contract.ErrNotFound,
		},
		{
			name: "revision conflict",
			prepare: func(t *testing.T, owner *Realm) (contract.PermissionBinding, contract.BindingSetRevision, int) {
				stale := owner.BindingSetRevision()
				advance := atomicLaterBinding("advance-lost-response")
				got, err := owner.PutBinding(atomicManager, advance, stale, "prepare-stale-lost-response")
				if err != nil || token.Generation(got) != token.Generation(stale)+1 {
					t.Fatalf("prepare stale revision: revision=%#v err=%v", got, err)
				}
				return atomicElementBinding, stale, 1
			},
			wantError: contract.ErrRevisionConflict,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "realm.db")
			extra := []contract.PermissionBinding(nil)
			if test.seedTarget {
				extra = append(extra, atomicElementBinding)
			}
			owner := mustCreateAtomicRealm(t, path, extra...)
			target, expected, operationCount := test.prepare(t, owner)
			committedRevision := owner.BindingSetRevision()
			operationID := contract.OperationID("outcome-lost-response-" + test.name)
			owner.state.store.failAfterCommit = atomicInjectedFailure(test.name + " lost response")

			got, err := owner.PutBinding(atomicManager, target, expected, operationID)
			if !errors.Is(err, contract.ErrStore) || got != (contract.BindingSetRevision{}) {
				t.Fatalf("indeterminate outcome-only operation: revision=%#v err=%v", got, err)
			}
			assertAtomicOwnerRetired(t, owner)

			reopened := mustOpenAtomicRealm(t, path)
			if got := reopened.BindingSetRevision(); got != committedRevision {
				_ = reopened.Close()
				t.Fatalf("outcome-only operation advanced revision: got %#v, want %#v", got, committedRevision)
			}
			assertAtomicBindingDecision(t, reopened, target, test.wantAuthorized)
			got, err = reopened.PutBinding(atomicManager, target, expected, operationID)
			if !errors.Is(err, test.wantError) {
				_ = reopened.Close()
				t.Fatalf("exact outcome-only replay: got err %v, want %v", err, test.wantError)
			}
			if test.wantRevision {
				if got != committedRevision {
					_ = reopened.Close()
					t.Fatalf("exact no-op replay revision: got %#v, want %#v", got, committedRevision)
				}
			} else if got != (contract.BindingSetRevision{}) {
				_ = reopened.Close()
				t.Fatalf("exact error replay revision: got %#v, want zero", got)
			}
			assertAtomicOperationCount(t, reopened, operationCount+1)

			later := atomicLaterBinding("later-" + test.name)
			latest, laterErr := reopened.PutBinding(
				atomicManager, later, reopened.BindingSetRevision(), contract.OperationID("later-"+test.name),
			)
			if laterErr != nil {
				_ = reopened.Close()
				t.Fatalf("later changed operation: %v", laterErr)
			}
			got, err = reopened.PutBinding(atomicManager, target, expected, operationID)
			if !errors.Is(err, test.wantError) || (test.wantRevision && got != committedRevision) ||
				(!test.wantRevision && got != (contract.BindingSetRevision{})) {
				_ = reopened.Close()
				t.Fatalf("historical replay after later change: revision=%#v err=%v", got, err)
			}
			if err := reopened.Close(); err != nil {
				t.Fatalf("close after later change: %v", err)
			}

			reopened = mustOpenAtomicRealm(t, path)
			defer func() { _ = reopened.Close() }()
			if got := reopened.BindingSetRevision(); got != latest {
				t.Fatalf("second reopen revision: got %#v, want %#v", got, latest)
			}
			got, err = reopened.PutBinding(atomicManager, target, expected, operationID)
			if !errors.Is(err, test.wantError) || (test.wantRevision && got != committedRevision) ||
				(!test.wantRevision && got != (contract.BindingSetRevision{})) {
				t.Fatalf("historical replay after second reopen: revision=%#v err=%v", got, err)
			}
		})
	}
}

func TestRealmForeignExpectedRevisionConflictSurvivesReopenAndLaterMutation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "realm.db")
	owner := mustCreateAtomicRealm(t, path)
	localInitial := owner.BindingSetRevision()

	foreignPath := filepath.Join(t.TempDir(), "foreign.db")
	foreignOwner, err := CreateDurable(contract.Seed{ID: "realm-foreign"}, foreignPath)
	if err != nil {
		_ = owner.Close()
		t.Fatalf("CreateDurable foreign Realm: %v", err)
	}
	foreignExpected := foreignOwner.BindingSetRevision()
	if foreignExpected == localInitial {
		_ = foreignOwner.Close()
		_ = owner.Close()
		t.Fatal("independent Realms unexpectedly shared a revision token")
	}
	if err := foreignOwner.Close(); err != nil {
		_ = owner.Close()
		t.Fatalf("close foreign Realm: %v", err)
	}

	operationID := contract.OperationID("foreign-expected-conflict")
	got, err := owner.PutBinding(atomicManager, atomicElementBinding, foreignExpected, operationID)
	if !errors.Is(err, contract.ErrRevisionConflict) || got != (contract.BindingSetRevision{}) {
		_ = owner.Close()
		t.Fatalf("foreign expected conflict: revision=%#v err=%v", got, err)
	}
	if got := owner.BindingSetRevision(); got != localInitial {
		_ = owner.Close()
		t.Fatalf("foreign conflict advanced revision: got %#v, want %#v", got, localInitial)
	}
	assertAtomicBindingDecision(t, owner, atomicElementBinding, false)
	assertAtomicOperationCount(t, owner, 1)
	if err := owner.Close(); err != nil {
		t.Fatalf("close foreign-conflict owner: %v", err)
	}

	reopened := mustOpenAtomicRealm(t, path)
	got, err = reopened.PutBinding(atomicManager, atomicElementBinding, foreignExpected, operationID)
	if !errors.Is(err, contract.ErrRevisionConflict) || got != (contract.BindingSetRevision{}) {
		_ = reopened.Close()
		t.Fatalf("foreign conflict replay after reopen: revision=%#v err=%v", got, err)
	}
	committed, err := reopened.PutBinding(
		atomicManager,
		atomicElementBinding,
		reopened.BindingSetRevision(),
		"foreign-conflict-later-put",
	)
	if err != nil || committed == (contract.BindingSetRevision{}) || committed == localInitial {
		_ = reopened.Close()
		t.Fatalf("later mutation after foreign conflict: revision=%#v err=%v", committed, err)
	}
	assertAtomicBindingDecision(t, reopened, atomicElementBinding, true)
	got, err = reopened.PutBinding(atomicManager, atomicElementBinding, foreignExpected, operationID)
	if !errors.Is(err, contract.ErrRevisionConflict) || got != (contract.BindingSetRevision{}) {
		_ = reopened.Close()
		t.Fatalf("foreign conflict replay after later mutation: revision=%#v err=%v", got, err)
	}
	if err := reopened.Close(); err != nil {
		t.Fatalf("close Realm after later mutation: %v", err)
	}

	reopened = mustOpenAtomicRealm(t, path)
	defer func() { _ = reopened.Close() }()
	if got := reopened.BindingSetRevision(); got != committed {
		t.Fatalf("second reopen revision: got %#v, want %#v", got, committed)
	}
	assertAtomicBindingDecision(t, reopened, atomicElementBinding, true)
	got, err = reopened.PutBinding(atomicManager, atomicElementBinding, foreignExpected, operationID)
	if !errors.Is(err, contract.ErrRevisionConflict) || got != (contract.BindingSetRevision{}) {
		t.Fatalf("foreign conflict replay after second reopen: revision=%#v err=%v", got, err)
	}
}

func TestOpenDurableRejectsInconsistentRealmOperationHistoryWithoutModification(t *testing.T) {
	t.Run("missing changed generation", func(t *testing.T) {
		path := createAtomicChangedHistory(t, "missing-changed")
		mutateAtomicStore(t, path, func(tx *bbolt.Tx) error {
			return tx.Bucket(atomicOperationBucket).Delete([]byte("missing-changed"))
		})
		assertAtomicOpenRejectedUnchanged(t, path)
	})

	t.Run("duplicate changed generation", func(t *testing.T) {
		path := createAtomicChangedHistory(t, "original-changed")
		mutateAtomicStore(t, path, func(tx *bbolt.Tx) error {
			operations := tx.Bucket(atomicOperationBucket)
			return operations.Put([]byte("duplicate-changed"), bytes.Clone(operations.Get([]byte("original-changed"))))
		})
		assertAtomicOpenRejectedUnchanged(t, path)
	})

	t.Run("current binding contradicts history", func(t *testing.T) {
		path := createAtomicChangedHistory(t, "contradicted-changed")
		mutateAtomicStore(t, path, func(tx *bbolt.Tx) error {
			return tx.Bucket(atomicBindingBucket).Delete(atomicIndependentBinding(atomicElementBinding))
		})
		assertAtomicOpenRejectedUnchanged(t, path)
	})

	t.Run("no-op contradicts membership", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "realm.db")
		owner := mustCreateAtomicRealm(t, path)
		revision := owner.BindingSetRevision()
		if err := owner.Close(); err != nil {
			t.Fatalf("close Realm before mutation: %v", err)
		}
		impossibleNoop := atomicIndependentOperation(
			1, atomicManager, atomicIndependentBinding(atomicElementBinding), revision,
			1, atomicRevisionPointer(revision),
		)
		mutateAtomicStore(t, path, func(tx *bbolt.Tx) error {
			return tx.Bucket(atomicOperationBucket).Put([]byte("impossible-noop"), impossibleNoop)
		})
		assertAtomicOpenRejectedUnchanged(t, path)
	})
}
