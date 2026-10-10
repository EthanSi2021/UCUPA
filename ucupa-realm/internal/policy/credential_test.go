package policy

import (
	"encoding/hex"
	"errors"
	"sync"
	"testing"

	"ucupa/shared/values"
	contract "ucupa/ucupa-realm/contract"
	token "ucupa/ucupa-realm/internal/token"
)

func TestCredentialVerifierGoldenAndExactAuthentication(t *testing.T) {
	goldens := []struct {
		name        string
		audience    values.RealmID
		idStart     byte
		secretStart byte
		wantHex     string
	}{
		{
			name: "ASCII audience", audience: "realm-a", idStart: 1, secretStart: 32,
			wantHex: "9baed114d401e6681e8970675a4d9ab70ffa4dcd18cf052aca10a63f52f645ef",
		},
		{
			name: "exact multibyte audience bytes", audience: "realm-é", idStart: 240, secretStart: 160,
			wantHex: "c67af720072d644546c1cf99877cf3ade64a6981459b4b76769e936f5ba711a2",
		},
	}
	for _, test := range goldens {
		t.Run(test.name, func(t *testing.T) {
			golden := contract.CredentialEvidence{Audience: test.audience}
			for index := range golden.ID {
				golden.ID[index] = test.idStart + byte(index)
			}
			for index := range golden.Secret {
				golden.Secret[index] = test.secretStart + byte(index)
			}
			want, err := hex.DecodeString(test.wantHex)
			if err != nil {
				t.Fatalf("decode independent verifier golden: %v", err)
			}
			got := credentialVerifier(golden)
			if string(got[:]) != string(want) {
				t.Fatalf("credential verifier:\n got %x\nwant %x", got, want)
			}
		})
	}

	evidence := testCredentialEvidence("realm-a", 1, 11)
	policy := mustRealm(t, contract.Seed{
		ID:              "realm-a",
		LocalPrincipals: []values.PrincipalID{"alice"},
		Credentials: []contract.CredentialBootstrap{{
			Subject: "alice", Evidence: evidence,
		}},
	})
	wantActor := values.PrincipalRef{Realm: "realm-a", Principal: "alice"}
	if actor := mustAuthenticate(t, policy, evidence); actor != wantActor {
		t.Fatalf("Authenticate: got %#v, want %#v", actor, wantActor)
	}
	if !token.Valid(policy.CredentialSetRevision()) {
		t.Fatal("credential aggregate started with an invalid revision")
	}

	wrongSecret := evidence
	wrongSecret.Secret[0] ^= 0xff
	assertAuthenticationError(t, policy, wrongSecret, contract.ErrUnauthenticated)
	wrongAudience := evidence
	wrongAudience.Audience = "realm-b"
	assertAuthenticationError(t, policy, wrongAudience, contract.ErrUnauthenticated)
	unknown := evidence
	unknown.ID[0] ^= 0xff
	assertAuthenticationError(t, policy, unknown, contract.ErrUnauthenticated)
	assertAuthenticationError(t, policy, contract.CredentialEvidence{}, contract.ErrInvalid)
}

func TestCredentialSeedAndLifecycleAuthorityAreExact(t *testing.T) {
	valid := testCredentialEvidence("realm-a", 1, 11)
	invalidSeeds := []struct {
		name        string
		principals  []values.PrincipalID
		credentials []contract.CredentialBootstrap
	}{
		{
			name:        "unregistered subject",
			principals:  []values.PrincipalID{"alice"},
			credentials: []contract.CredentialBootstrap{{Subject: "missing", Evidence: valid}},
		},
		{
			name:        "zero ID",
			principals:  []values.PrincipalID{"alice"},
			credentials: []contract.CredentialBootstrap{{Subject: "alice", Evidence: contract.CredentialEvidence{Audience: "realm-a", Secret: valid.Secret}}},
		},
		{
			name:        "zero secret",
			principals:  []values.PrincipalID{"alice"},
			credentials: []contract.CredentialBootstrap{{Subject: "alice", Evidence: contract.CredentialEvidence{Audience: "realm-a", ID: valid.ID}}},
		},
		{
			name:       "foreign audience",
			principals: []values.PrincipalID{"alice"},
			credentials: []contract.CredentialBootstrap{{
				Subject: "alice", Evidence: testCredentialEvidence("realm-b", 1, 11),
			}},
		},
		{
			name:       "duplicate ID",
			principals: []values.PrincipalID{"alice", "bob"},
			credentials: []contract.CredentialBootstrap{
				{Subject: "alice", Evidence: valid},
				{Subject: "bob", Evidence: testCredentialEvidence("realm-a", 1, 22)},
			},
		},
	}
	for _, test := range invalidSeeds {
		t.Run(test.name, func(t *testing.T) {
			got, err := New(contract.Seed{
				ID:              "realm-a",
				LocalPrincipals: test.principals,
				Credentials:     test.credentials,
			})
			if got != nil || !errors.Is(err, contract.ErrInvalidSeed) {
				t.Fatalf("New invalid credential Seed: Realm=%v err=%v, want nil/ErrInvalidSeed", got, err)
			}
		})
	}

	alice := values.PrincipalRef{Realm: "realm-a", Principal: "alice"}
	policy := mustRealm(t, contract.Seed{
		ID:              "realm-a",
		LocalPrincipals: []values.PrincipalID{"alice"},
		Bindings: []contract.PermissionBinding{
			administrationBinding(alice, contract.PermissionRotateOwnCredential, "realm-a"),
			administrationBinding(alice, contract.PermissionRevokeCredential, "realm-a"),
		},
		Credentials: []contract.CredentialBootstrap{{Subject: "alice", Evidence: valid}},
	})
	if !policy.Authorize(
		alice,
		values.Requirement{Permission: contract.PermissionRotateOwnCredential, Scope: values.RealmScope("realm-a")},
		values.Requirement{Permission: contract.PermissionRevokeCredential, Scope: values.RealmScope("realm-a")},
	) {
		t.Fatal("exact immutable credential lifecycle bindings did not authorize")
	}

	target := contract.PermissionBinding{
		Subject: alice, Permission: contract.PermissionRotateOwnCredential, Scope: values.RealmScope("realm-a"),
	}
	got, err := policy.PutBinding(alice, target, policy.BindingSetRevision(), "online-lifecycle-target")
	assertBindingMutationError(t, got, err, contract.ErrInvalid)
}

func TestCredentialSelfRotationReplayConflictAndIndependentOperationNamespace(t *testing.T) {
	alice := values.PrincipalRef{Realm: "realm-a", Principal: "alice"}
	old := testCredentialEvidence("realm-a", 1, 11)
	claimed := testCredentialEvidence("realm-a", 3, 33)
	replacement := testCredentialEvidence("realm-a", 2, 22)
	later := testCredentialEvidence("realm-a", 4, 44)
	targetBinding := contract.PermissionBinding{
		Subject: alice, Permission: permissionReadElement, Scope: values.CoreElementScope(realmCoreRef, realmSiteID),
	}
	policy := mustRealm(t, contract.Seed{
		ID:              "realm-a",
		LocalPrincipals: []values.PrincipalID{"alice", "other"},
		Bindings: []contract.PermissionBinding{
			administrationBinding(alice, contract.PermissionPutBinding, "realm-a"),
			administrationBinding(alice, contract.PermissionRotateOwnCredential, "realm-a"),
		},
		Credentials: []contract.CredentialBootstrap{
			{Subject: "alice", Evidence: old},
			{Subject: "other", Evidence: claimed},
		},
	})

	credentialInitial := policy.CredentialSetRevision()
	bindingInitial := policy.BindingSetRevision()
	bindingCommitted := mustPutBinding(t, policy, alice, targetBinding, bindingInitial, contract.OperationID("shared-id"))
	if policy.CredentialSetRevision() != credentialInitial {
		t.Fatal("binding mutation changed credential revision")
	}

	got, err := policy.RotateCredential(old, old.ID, claimed, credentialInitial, "claimed")
	assertCredentialMutationError(t, got, err, contract.ErrCredentialConflict)

	rotated := mustRotateCredential(t, policy, old, old.ID, replacement, credentialInitial, "shared-id")
	if rotated == credentialInitial {
		t.Fatal("changed rotation retained the initial credential revision")
	}
	if policy.BindingSetRevision() != bindingCommitted {
		t.Fatal("credential mutation changed binding revision")
	}
	assertAuthenticationError(t, policy, old, contract.ErrUnauthenticated)
	wantActor := values.PrincipalRef{Realm: "realm-a", Principal: "alice"}
	if actor := mustAuthenticate(t, policy, replacement); actor != wantActor {
		t.Fatalf("replacement proof established %#v, want %#v", actor, wantActor)
	}

	if replay := mustRotateCredential(t, policy, replacement, old.ID, replacement, credentialInitial, "shared-id"); replay != rotated {
		t.Fatalf("replacement-proof replay: got %#v, want %#v", replay, rotated)
	}
	got, err = policy.RotateCredential(replacement, old.ID, later, credentialInitial, "shared-id")
	assertCredentialMutationError(t, got, err, contract.ErrOperationConflict)

	got, err = policy.RotateCredential(replacement, replacement.ID, claimed, rotated, "claimed-again")
	assertCredentialMutationError(t, got, err, contract.ErrCredentialConflict)
	laterRevision := mustRotateCredential(t, policy, replacement, replacement.ID, later, rotated, "later")
	got, err = policy.RotateCredential(later, replacement.ID, claimed, rotated, "claimed-again")
	assertCredentialMutationError(t, got, err, contract.ErrCredentialConflict)
	if policy.CredentialSetRevision() != laterRevision {
		t.Fatal("historical credential-conflict replay changed current revision")
	}

	unknownProof := testCredentialEvidence("realm-a", 9, 99)
	got, err = policy.RotateCredential(unknownProof, unknownProof.ID, testCredentialEvidence("realm-a", 10, 100), laterRevision, "unknown-proof")
	assertCredentialMutationError(t, got, err, contract.ErrUnauthenticated)
	got, err = policy.RotateCredential(later, old.ID, testCredentialEvidence("realm-a", 10, 100), laterRevision, "different-old")
	assertCredentialMutationError(t, got, err, contract.ErrDenied)

	if replay := mustPutBinding(t, policy, alice, targetBinding, bindingInitial, contract.OperationID("shared-id")); replay != bindingCommitted {
		t.Fatal("binding OperationID did not replay independently from CredentialOperationID")
	}
}

func TestCredentialRevocationRetainsHistoricalResultBeforeSelfSafety(t *testing.T) {
	alice := values.PrincipalRef{Realm: "realm-a", Principal: "alice"}
	old := testCredentialEvidence("realm-a", 1, 11)
	spare := testCredentialEvidence("realm-a", 2, 22)
	future := testCredentialEvidence("realm-a", 3, 33)
	unknown := testCredentialEvidence("realm-a", 4, 44)
	policy := mustRealm(t, contract.Seed{
		ID:              "realm-a",
		LocalPrincipals: []values.PrincipalID{"alice"},
		Bindings: []contract.PermissionBinding{
			administrationBinding(alice, contract.PermissionRotateOwnCredential, "realm-a"),
			administrationBinding(alice, contract.PermissionRevokeCredential, "realm-a"),
		},
		Credentials: []contract.CredentialBootstrap{
			{Subject: "alice", Evidence: old},
			{Subject: "alice", Evidence: spare},
		},
	})

	initial := policy.CredentialSetRevision()
	got, err := policy.RevokeCredential(old, future.ID, initial, "historical-not-found")
	assertCredentialMutationError(t, got, err, contract.ErrNotFound)
	rotated := mustRotateCredential(t, policy, old, old.ID, future, initial, "claim-future")

	got, err = policy.RevokeCredential(future, future.ID, initial, "historical-not-found")
	assertCredentialMutationError(t, got, err, contract.ErrNotFound)
	got, err = policy.RevokeCredential(future, future.ID, rotated, "new-self-revoke")
	assertCredentialMutationError(t, got, err, contract.ErrInvalid)

	revoked := mustRevokeCredential(t, policy, spare, future.ID, rotated, "revoke-active")
	assertAuthenticationError(t, policy, future, contract.ErrUnauthenticated)
	if replay := mustRevokeCredential(t, policy, spare, future.ID, rotated, "revoke-active"); replay != revoked {
		t.Fatalf("changed Revoke replay: got %#v, want %#v", replay, revoked)
	}
	if noop := mustRevokeCredential(t, policy, spare, future.ID, revoked, "revoke-noop"); noop != revoked {
		t.Fatalf("revoked-target no-op advanced revision: got %#v, want %#v", noop, revoked)
	}
	got, err = policy.RevokeCredential(spare, unknown.ID, revoked, "revoke-unknown")
	assertCredentialMutationError(t, got, err, contract.ErrNotFound)

	staleReplacement := testCredentialEvidence("realm-a", 5, 55)
	got, err = policy.RotateCredential(spare, spare.ID, future, rotated, "stale-before-claimed")
	assertCredentialMutationError(t, got, err, contract.ErrRevisionConflict)
	got, err = policy.RotateCredential(spare, spare.ID, staleReplacement, contract.CredentialSetRevision{}, "invalid-before-owner")
	assertCredentialMutationError(t, got, err, contract.ErrInvalid)
}

func TestCredentialAuthenticationAuthorizationAndAudiencePrecedeReplay(t *testing.T) {
	alice := values.PrincipalRef{Realm: "realm-a", Principal: "alice"}
	bob := values.PrincipalRef{Realm: "realm-a", Principal: "bob"}
	aliceOld := testCredentialEvidence("realm-a", 1, 11)
	aliceNext := testCredentialEvidence("realm-a", 2, 22)
	bobProof := testCredentialEvidence("realm-a", 3, 33)
	unknown := testCredentialEvidence("realm-a", 4, 44)
	policy := mustRealm(t, contract.Seed{
		ID:              "realm-a",
		LocalPrincipals: []values.PrincipalID{"alice", "bob"},
		Bindings: []contract.PermissionBinding{
			administrationBinding(alice, contract.PermissionRotateOwnCredential, "realm-a"),
			administrationBinding(alice, contract.PermissionRevokeCredential, "realm-a"),
			administrationBinding(bob, contract.PermissionRotateOwnCredential, "realm-a"),
		},
		Credentials: []contract.CredentialBootstrap{
			{Subject: "alice", Evidence: aliceOld},
			{Subject: "bob", Evidence: bobProof},
		},
	})
	initial := policy.CredentialSetRevision()

	got, err := policy.RevokeCredential(aliceOld, unknown.ID, initial, "known-revoke")
	assertCredentialMutationError(t, got, err, contract.ErrNotFound)
	badProof := aliceOld
	badProof.Secret[0] ^= 0xff
	got, err = policy.RevokeCredential(badProof, unknown.ID, initial, "known-revoke")
	assertCredentialMutationError(t, got, err, contract.ErrUnauthenticated)
	got, err = policy.RevokeCredential(bobProof, unknown.ID, initial, "known-revoke")
	assertCredentialMutationError(t, got, err, contract.ErrDenied)
	got, err = policy.RevokeCredential(aliceOld, unknown.ID, initial, "known-revoke")
	assertCredentialMutationError(t, got, err, contract.ErrNotFound)

	rotated := mustRotateCredential(
		t, policy, aliceOld, aliceOld.ID, aliceNext, initial, "known-rotate",
	)
	foreignReplacement := aliceNext
	foreignReplacement.Audience = "realm-b"
	got, err = policy.RotateCredential(
		aliceNext, aliceOld.ID, foreignReplacement, initial, "known-rotate",
	)
	assertCredentialMutationError(t, got, err, contract.ErrInvalid)
	if replay := mustRotateCredential(
		t, policy, aliceNext, aliceOld.ID, aliceNext, initial, "known-rotate",
	); replay != rotated {
		t.Fatalf("exact Rotate replay: got %#v, want %#v", replay, rotated)
	}

	changedSecret := aliceNext
	changedSecret.Secret[0] ^= 0xff
	conflicts := []struct {
		name string
		call func() (contract.CredentialSetRevision, error)
	}{
		{
			name: "actor",
			call: func() (contract.CredentialSetRevision, error) {
				return policy.RotateCredential(bobProof, aliceOld.ID, aliceNext, initial, "known-rotate")
			},
		},
		{
			name: "kind",
			call: func() (contract.CredentialSetRevision, error) {
				return policy.RevokeCredential(aliceNext, aliceOld.ID, initial, "known-rotate")
			},
		},
		{
			name: "target",
			call: func() (contract.CredentialSetRevision, error) {
				return policy.RotateCredential(aliceNext, bobProof.ID, aliceNext, initial, "known-rotate")
			},
		},
		{
			name: "replacement ID",
			call: func() (contract.CredentialSetRevision, error) {
				return policy.RotateCredential(
					aliceNext, aliceOld.ID, testCredentialEvidence("realm-a", 5, 55),
					initial, "known-rotate",
				)
			},
		},
		{
			name: "replacement verifier",
			call: func() (contract.CredentialSetRevision, error) {
				return policy.RotateCredential(
					aliceNext, aliceOld.ID, changedSecret, initial, "known-rotate",
				)
			},
		},
		{
			name: "expected revision",
			call: func() (contract.CredentialSetRevision, error) {
				return policy.RotateCredential(
					aliceNext, aliceOld.ID, aliceNext, rotated, "known-rotate",
				)
			},
		},
	}
	for _, test := range conflicts {
		t.Run(test.name, func(t *testing.T) {
			got, err := test.call()
			assertCredentialMutationError(t, got, err, contract.ErrOperationConflict)
		})
	}
}

func TestCredentialLifecyclePermissionsAreIndependent(t *testing.T) {
	tests := []struct {
		name   string
		rotate bool
		revoke bool
	}{
		{name: "none"},
		{name: "rotate only", rotate: true},
		{name: "revoke only", revoke: true},
		{name: "both", rotate: true, revoke: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			actor := values.PrincipalRef{Realm: "realm-a", Principal: "alice"}
			old := testCredentialEvidence("realm-a", 1, 11)
			spare := testCredentialEvidence("realm-a", 2, 22)
			next := testCredentialEvidence("realm-a", 3, 33)
			seed := contract.Seed{
				ID:              "realm-a",
				LocalPrincipals: []values.PrincipalID{"alice"},
				Credentials: []contract.CredentialBootstrap{
					{Subject: "alice", Evidence: old},
					{Subject: "alice", Evidence: spare},
				},
			}
			if test.rotate {
				seed.Bindings = append(seed.Bindings,
					administrationBinding(actor, contract.PermissionRotateOwnCredential, "realm-a"))
			}
			if test.revoke {
				seed.Bindings = append(seed.Bindings,
					administrationBinding(actor, contract.PermissionRevokeCredential, "realm-a"))
			}
			policy := mustRealm(t, seed)
			initial := policy.CredentialSetRevision()

			rotated, rotateErr := policy.RotateCredential(old, old.ID, next, initial, "rotate")
			if test.rotate {
				if rotateErr != nil || !token.Valid(rotated) {
					t.Fatalf("authorized Rotate: revision=%#v err=%v", rotated, rotateErr)
				}
			} else {
				assertCredentialMutationError(t, rotated, rotateErr, contract.ErrDenied)
				rotated = initial
			}

			target := old.ID
			if test.rotate {
				target = next.ID
			}
			revoked, revokeErr := policy.RevokeCredential(spare, target, rotated, "revoke")
			if test.revoke {
				if revokeErr != nil || !token.Valid(revoked) {
					t.Fatalf("authorized Revoke: revision=%#v err=%v", revoked, revokeErr)
				}
			} else {
				assertCredentialMutationError(t, revoked, revokeErr, contract.ErrDenied)
			}
		})
	}
}

func TestCredentialCASConcurrencyDistinguishesActiveAndInvalidatedProofs(t *testing.T) {
	alice := values.PrincipalRef{Realm: "realm-a", Principal: "alice"}
	seedWith := func(credentials ...contract.CredentialEvidence) *Realm {
		seed := contract.Seed{
			ID:              "realm-a",
			LocalPrincipals: []values.PrincipalID{"alice"},
			Bindings: []contract.PermissionBinding{
				administrationBinding(alice, contract.PermissionRotateOwnCredential, "realm-a"),
			},
		}
		for _, evidence := range credentials {
			seed.Credentials = append(seed.Credentials, contract.CredentialBootstrap{Subject: "alice", Evidence: evidence})
		}
		return mustRealm(t, seed)
	}

	t.Run("distinct active proofs reach CAS", func(t *testing.T) {
		first := testCredentialEvidence("realm-a", 1, 11)
		second := testCredentialEvidence("realm-a", 2, 22)
		policy := seedWith(first, second)
		initial := policy.CredentialSetRevision()
		calls := []credentialRotationCall{
			{proof: first, replacement: testCredentialEvidence("realm-a", 3, 33), id: "first"},
			{proof: second, replacement: testCredentialEvidence("realm-a", 4, 44), id: "second"},
		}
		results := runConcurrentRotations(t, policy, initial, calls)
		successes, revisions, unauthenticated := countCredentialResults(results)
		if successes != 1 || revisions != 1 || unauthenticated != 0 {
			t.Fatalf("distinct-proof race: success=%d revision-conflict=%d unauthenticated=%d", successes, revisions, unauthenticated)
		}
	})

	t.Run("same proof is invalidated before CAS", func(t *testing.T) {
		proof := testCredentialEvidence("realm-a", 1, 11)
		policy := seedWith(proof)
		initial := policy.CredentialSetRevision()
		calls := []credentialRotationCall{
			{proof: proof, replacement: testCredentialEvidence("realm-a", 2, 22), id: "first"},
			{proof: proof, replacement: testCredentialEvidence("realm-a", 3, 33), id: "second"},
		}
		results := runConcurrentRotations(t, policy, initial, calls)
		successes, revisions, unauthenticated := countCredentialResults(results)
		if successes != 1 || revisions != 0 || unauthenticated != 1 {
			t.Fatalf("same-proof race: success=%d revision-conflict=%d unauthenticated=%d", successes, revisions, unauthenticated)
		}
		if got := len(policy.state.credentialOperations); got != 1 {
			t.Fatalf("same-proof loser consumed an operation ID: got %d retained operations, want 1", got)
		}
	})
}

func TestCredentialInMemoryCloseAndValueCopyShareOneOwnerState(t *testing.T) {
	actor := values.PrincipalRef{Realm: "realm-a", Principal: "alice"}
	old := testCredentialEvidence("realm-a", 1, 11)
	next := testCredentialEvidence("realm-a", 2, 22)
	policy := mustRealm(t, contract.Seed{
		ID:              "realm-a",
		LocalPrincipals: []values.PrincipalID{"alice"},
		Bindings: []contract.PermissionBinding{
			administrationBinding(actor, contract.PermissionRotateOwnCredential, "realm-a"),
		},
		Credentials: []contract.CredentialBootstrap{{Subject: "alice", Evidence: old}},
	})
	alias := *policy
	initial := policy.CredentialSetRevision()
	if err := (&alias).Close(); err != nil {
		t.Fatalf("in-memory copied Close: %v", err)
	}
	if policy.CredentialSetRevision() != initial {
		t.Fatal("in-memory Close changed credential revision")
	}
	mustAuthenticate(t, policy, old)
	rotated := mustRotateCredential(t, &alias, old, old.ID, next, initial, "copy-rotate")
	if policy.CredentialSetRevision() != rotated {
		t.Fatal("original handle retained stale credential state after copied Rotate")
	}
	assertAuthenticationError(t, policy, old, contract.ErrUnauthenticated)
	mustAuthenticate(t, policy, next)
}

func TestCredentialZeroRealmFailsClosedAfterShapeValidation(t *testing.T) {
	var nilPolicy *Realm
	var policy Realm
	valid := testCredentialEvidence("realm-a", 1, 11)
	revision := mustRealm(t, contract.Seed{ID: "realm-a"}).CredentialSetRevision()
	assertAuthenticationError(t, nilPolicy, contract.CredentialEvidence{}, contract.ErrInvalid)
	assertAuthenticationError(t, nilPolicy, valid, contract.ErrStore)
	if policy.CredentialSetRevision() != (contract.CredentialSetRevision{}) {
		t.Fatal("zero Realm exposed a credential revision")
	}
	assertAuthenticationError(t, &policy, contract.CredentialEvidence{}, contract.ErrInvalid)
	assertAuthenticationError(t, &policy, valid, contract.ErrStore)
	got, err := policy.RotateCredential(valid, valid.ID, testCredentialEvidence("realm-a", 2, 22), revision, "zero-rotate")
	assertCredentialMutationError(t, got, err, contract.ErrStore)
	got, err = policy.RevokeCredential(valid, valid.ID, revision, "zero-revoke")
	assertCredentialMutationError(t, got, err, contract.ErrStore)
}

type credentialRotationCall struct {
	proof       contract.CredentialEvidence
	replacement contract.CredentialEvidence
	id          contract.CredentialOperationID
}

type credentialMutationResult struct {
	revision contract.CredentialSetRevision
	err      error
}

func runConcurrentRotations(
	t *testing.T,
	policy *Realm,
	expected contract.CredentialSetRevision,
	calls []credentialRotationCall,
) []credentialMutationResult {
	t.Helper()
	start := make(chan struct{})
	results := make(chan credentialMutationResult, len(calls))
	var writers sync.WaitGroup
	for _, call := range calls {
		call := call
		writers.Add(1)
		go func() {
			defer writers.Done()
			<-start
			revision, err := policy.RotateCredential(
				call.proof,
				call.proof.ID,
				call.replacement,
				expected,
				call.id,
			)
			results <- credentialMutationResult{revision: revision, err: err}
		}()
	}
	close(start)
	writers.Wait()
	close(results)
	got := make([]credentialMutationResult, 0, len(calls))
	for result := range results {
		got = append(got, result)
	}
	return got
}

func countCredentialResults(results []credentialMutationResult) (successes, revisions, unauthenticated int) {
	for _, result := range results {
		switch {
		case result.err == nil && token.Valid(result.revision):
			successes++
		case errors.Is(result.err, contract.ErrRevisionConflict) && result.revision == (contract.CredentialSetRevision{}):
			revisions++
		case errors.Is(result.err, contract.ErrUnauthenticated) && result.revision == (contract.CredentialSetRevision{}):
			unauthenticated++
		}
	}
	return successes, revisions, unauthenticated
}

func testCredentialEvidence(realmID values.RealmID, idByte, secretByte byte) contract.CredentialEvidence {
	var evidence contract.CredentialEvidence
	evidence.Audience = realmID
	for index := range evidence.ID {
		evidence.ID[index] = idByte
	}
	for index := range evidence.Secret {
		evidence.Secret[index] = secretByte
	}
	return evidence
}

func mustAuthenticate(t *testing.T, policy *Realm, evidence contract.CredentialEvidence) values.PrincipalRef {
	t.Helper()
	actor, err := policy.Authenticate(evidence)
	if err != nil {
		t.Fatalf("Authenticate: %v", err)
	}
	if !actor.Valid() {
		t.Fatalf("Authenticate returned invalid PrincipalRef %#v", actor)
	}
	return actor
}

func assertAuthenticationError(t *testing.T, policy *Realm, evidence contract.CredentialEvidence, want error) {
	t.Helper()
	actor, err := policy.Authenticate(evidence)
	if !errors.Is(err, want) || actor != (values.PrincipalRef{}) {
		t.Fatalf("Authenticate: actor=%#v err=%v, want zero/%v", actor, err, want)
	}
}

func mustRotateCredential(
	t *testing.T,
	policy *Realm,
	proof contract.CredentialEvidence,
	oldID contract.CredentialID,
	replacement contract.CredentialEvidence,
	expected contract.CredentialSetRevision,
	operationID contract.CredentialOperationID,
) contract.CredentialSetRevision {
	t.Helper()
	revision, err := policy.RotateCredential(proof, oldID, replacement, expected, operationID)
	if err != nil {
		t.Fatalf("RotateCredential(%q): %v", operationID, err)
	}
	if !token.Valid(revision) {
		t.Fatalf("RotateCredential(%q) returned invalid revision %#v", operationID, revision)
	}
	return revision
}

func mustRevokeCredential(
	t *testing.T,
	policy *Realm,
	proof contract.CredentialEvidence,
	target contract.CredentialID,
	expected contract.CredentialSetRevision,
	operationID contract.CredentialOperationID,
) contract.CredentialSetRevision {
	t.Helper()
	revision, err := policy.RevokeCredential(proof, target, expected, operationID)
	if err != nil {
		t.Fatalf("RevokeCredential(%q): %v", operationID, err)
	}
	if !token.Valid(revision) {
		t.Fatalf("RevokeCredential(%q) returned invalid revision %#v", operationID, revision)
	}
	return revision
}

func assertCredentialMutationError(
	t *testing.T,
	revision contract.CredentialSetRevision,
	err error,
	want error,
) {
	t.Helper()
	if !errors.Is(err, want) {
		t.Fatalf("credential mutation error: got %v, want %v", err, want)
	}
	if revision != (contract.CredentialSetRevision{}) {
		t.Fatalf("failed credential mutation returned revision %#v", revision)
	}
}
