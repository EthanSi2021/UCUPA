package policy

import (
	"bytes"
	"encoding/base64"
	"encoding/hex"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"ucupa/shared/values"
	contract "ucupa/ucupa-realm/contract"
	token "ucupa/ucupa-realm/internal/token"
)

const credentialDurableProcessEnvironment = "UCUPA_REALM_CREDENTIAL_PROCESS_STORE"

var (
	credentialDurableActor = values.PrincipalRef{Realm: "realm-credential-durable", Principal: "manager"}
	credentialDurableOld   = testCredentialEvidence("realm-credential-durable", 0x11, 0x31)
	credentialDurableSpare = testCredentialEvidence("realm-credential-durable", 0x22, 0x42)
	credentialDurableNext  = testCredentialEvidence("realm-credential-durable", 0x33, 0x53)
)

func TestCredentialDurableRoundTripAndSecretAbsence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "realm.db")
	owner := mustCreateDurableRealm(t, credentialDurableSeed(), path)
	bindingRevision := owner.BindingSetRevision()
	initial := owner.CredentialSetRevision()

	if got := mustAuthenticate(t, owner, credentialDurableOld); got != credentialDurableActor {
		t.Fatalf("bootstrap credential established %#v, want %#v", got, credentialDurableActor)
	}
	rotated := mustRotateCredential(
		t, owner, credentialDurableOld, credentialDurableOld.ID,
		credentialDurableNext, initial, "durable-rotate",
	)
	revoked := mustRevokeCredential(
		t, owner, credentialDurableSpare, credentialDurableNext.ID,
		rotated, "durable-revoke",
	)
	if got := owner.BindingSetRevision(); got != bindingRevision {
		t.Fatalf("credential lifecycle changed binding revision: got %#v, want %#v", got, bindingRevision)
	}
	assertAuthenticationError(t, owner, credentialDurableOld, contract.ErrUnauthenticated)
	assertAuthenticationError(t, owner, credentialDurableNext, contract.ErrUnauthenticated)
	if got := mustAuthenticate(t, owner, credentialDurableSpare); got != credentialDurableActor {
		t.Fatalf("spare credential established %#v, want %#v", got, credentialDurableActor)
	}

	if err := owner.Close(); err != nil {
		t.Fatalf("close credential owner: %v", err)
	}
	assertAuthenticationError(t, owner, credentialDurableSpare, contract.ErrStore)
	if owner.CredentialSetRevision() != (contract.CredentialSetRevision{}) {
		t.Fatal("closed durable owner exposed a credential revision")
	}
	assertCredentialSecretsAbsent(t, mustReadFile(t, path),
		credentialDurableOld.Secret, credentialDurableSpare.Secret, credentialDurableNext.Secret)

	reopened := mustOpenDurableRealm(t, path)
	if got := reopened.CredentialSetRevision(); got != revoked {
		t.Fatalf("reopened credential revision: got %#v, want %#v", got, revoked)
	}
	if got := reopened.BindingSetRevision(); got != bindingRevision {
		t.Fatalf("reopened binding revision: got %#v, want %#v", got, bindingRevision)
	}
	assertAuthenticationError(t, reopened, credentialDurableOld, contract.ErrUnauthenticated)
	assertAuthenticationError(t, reopened, credentialDurableNext, contract.ErrUnauthenticated)
	if got := mustAuthenticate(t, reopened, credentialDurableSpare); got != credentialDurableActor {
		t.Fatalf("reopened spare credential established %#v, want %#v", got, credentialDurableActor)
	}
	if replay := mustRotateCredential(
		t, reopened, credentialDurableSpare, credentialDurableOld.ID,
		credentialDurableNext, initial, "durable-rotate",
	); replay != rotated {
		t.Fatalf("reopened Rotate replay: got %#v, want %#v", replay, rotated)
	}
	if replay := mustRevokeCredential(
		t, reopened, credentialDurableSpare, credentialDurableNext.ID,
		rotated, "durable-revoke",
	); replay != revoked {
		t.Fatalf("reopened Revoke replay: got %#v, want %#v", replay, revoked)
	}
}

func TestCredentialStoreFailureBoundaries(t *testing.T) {
	t.Run("definite precommit leaves changed operation reusable", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "realm.db")
		owner := mustCreateDurableRealm(t, credentialDurableSeed(), path)
		initial := owner.CredentialSetRevision()
		owner.state.store.failBeforeCommit = atomicInjectedFailure("credential changed precommit")

		got, err := owner.RotateCredential(
			credentialDurableOld, credentialDurableOld.ID, credentialDurableNext,
			initial, "credential-changed-precommit",
		)
		assertCredentialMutationError(t, got, err, contract.ErrStore)
		if owner.CredentialSetRevision() != initial {
			t.Fatal("definite precommit changed credential revision")
		}
		mustAuthenticate(t, owner, credentialDurableOld)
		assertAuthenticationError(t, owner, credentialDurableNext, contract.ErrUnauthenticated)
		if len(owner.state.credentialOperations) != 0 {
			t.Fatal("definite precommit consumed credential operation")
		}

		mustRotateCredential(
			t, owner, credentialDurableOld, credentialDurableOld.ID,
			credentialDurableNext, initial, "credential-changed-precommit",
		)
	})

	t.Run("definite precommit leaves outcome-only operation reusable", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "realm.db")
		owner := mustCreateDurableRealm(t, credentialDurableSeed(), path)
		initial := owner.CredentialSetRevision()
		unknown := testCredentialEvidence(credentialDurableActor.Realm, 0x44, 0x64)
		owner.state.store.failBeforeCommit = atomicInjectedFailure("credential outcome precommit")

		got, err := owner.RevokeCredential(
			credentialDurableOld, unknown.ID, initial, "credential-outcome-precommit",
		)
		assertCredentialMutationError(t, got, err, contract.ErrStore)
		if owner.CredentialSetRevision() != initial || len(owner.state.credentialOperations) != 0 {
			t.Fatal("definite precommit retained an outcome-only credential fact")
		}

		got, err = owner.RevokeCredential(
			credentialDurableOld, unknown.ID, initial, "credential-outcome-precommit",
		)
		assertCredentialMutationError(t, got, err, contract.ErrNotFound)
		if len(owner.state.credentialOperations) != 1 {
			t.Fatal("successful outcome-only retry was not retained")
		}
	})

	t.Run("indeterminate changed result retires and reopens", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "realm.db")
		owner := mustCreateDurableRealm(t, credentialDurableSeed(), path)
		initial := owner.CredentialSetRevision()
		owner.state.store.failAfterCommit = atomicInjectedFailure("credential changed lost response")

		got, err := owner.RotateCredential(
			credentialDurableOld, credentialDurableOld.ID, credentialDurableNext,
			initial, "credential-changed-lost-response",
		)
		assertCredentialMutationError(t, got, err, contract.ErrStore)
		assertAuthenticationError(t, owner, credentialDurableOld, contract.ErrStore)
		assertAuthenticationError(t, owner, credentialDurableNext, contract.ErrStore)
		if owner.CredentialSetRevision() != (contract.CredentialSetRevision{}) {
			t.Fatal("retired owner exposed a credential revision")
		}

		reopened := mustOpenDurableRealm(t, path)
		committed := reopened.CredentialSetRevision()
		if token.Scope(committed) != token.Scope(initial) || token.Generation(committed) != token.Generation(initial)+1 {
			t.Fatalf("reopened changed revision: got %#v after %#v", committed, initial)
		}
		assertAuthenticationError(t, reopened, credentialDurableOld, contract.ErrUnauthenticated)
		mustAuthenticate(t, reopened, credentialDurableNext)
		if replay := mustRotateCredential(
			t, reopened, credentialDurableNext, credentialDurableOld.ID,
			credentialDurableNext, initial, "credential-changed-lost-response",
		); replay != committed {
			t.Fatalf("lost-response Rotate replay: got %#v, want %#v", replay, committed)
		}
	})

	t.Run("indeterminate outcome retires and reopens", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "realm.db")
		owner := mustCreateDurableRealm(t, credentialDurableSeed(), path)
		initial := owner.CredentialSetRevision()
		unknown := testCredentialEvidence(credentialDurableActor.Realm, 0x55, 0x75)
		owner.state.store.failAfterCommit = atomicInjectedFailure("credential outcome lost response")

		got, err := owner.RevokeCredential(
			credentialDurableOld, unknown.ID, initial, "credential-outcome-lost-response",
		)
		assertCredentialMutationError(t, got, err, contract.ErrStore)
		assertAuthenticationError(t, owner, credentialDurableOld, contract.ErrStore)

		reopened := mustOpenDurableRealm(t, path)
		if reopened.CredentialSetRevision() != initial {
			t.Fatal("outcome-only credential transaction advanced revision")
		}
		got, err = reopened.RevokeCredential(
			credentialDurableOld, unknown.ID, initial, "credential-outcome-lost-response",
		)
		assertCredentialMutationError(t, got, err, contract.ErrNotFound)
	})
}

func TestCredentialDurableValueCopySharesRetirement(t *testing.T) {
	path := filepath.Join(t.TempDir(), "realm.db")
	owner := mustCreateDurableRealm(t, credentialDurableSeed(), path)
	alias := *owner
	if err := (&alias).Close(); err != nil {
		t.Fatalf("close copied credential owner: %v", err)
	}
	for _, policy := range []*Realm{owner, &alias} {
		assertAuthenticationError(t, policy, credentialDurableOld, contract.ErrStore)
		if policy.CredentialSetRevision() != (contract.CredentialSetRevision{}) {
			t.Fatal("retired copied owner exposed credential revision")
		}
	}
	assertRealmStoreUnlocked(t, path)
}

func TestCredentialDurableCommittedRotationProcessExitHelper(t *testing.T) {
	path := os.Getenv(credentialDurableProcessEnvironment)
	if path == "" {
		return
	}
	owner, err := CreateDurable(credentialDurableSeed(), path)
	if err != nil {
		os.Exit(2)
	}
	initial := owner.CredentialSetRevision()
	committed, err := owner.RotateCredential(
		credentialDurableOld, credentialDurableOld.ID, credentialDurableNext,
		initial, "credential-process-rotate",
	)
	if err != nil || !token.Valid(committed) || committed == initial {
		os.Exit(3)
	}
	if actor, err := owner.Authenticate(credentialDurableNext); err != nil || actor != credentialDurableActor {
		os.Exit(4)
	}
	os.Exit(0)
}

func TestCredentialDurableReopensCommittedRotationAfterProcessExit(t *testing.T) {
	path := filepath.Join(t.TempDir(), "realm.db")
	command := exec.Command(os.Args[0], "-test.run=^TestCredentialDurableCommittedRotationProcessExitHelper$")
	command.Env = append(os.Environ(), credentialDurableProcessEnvironment+"="+path)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("credential durable helper: %v\n%s", err, output)
	}

	reopened := mustOpenDurableRealm(t, path)
	committed := reopened.CredentialSetRevision()
	if token.Generation(committed) != 2 {
		t.Fatalf("process-exit credential generation: got %d, want 2", token.Generation(committed))
	}
	initial := token.Credential(token.Scope(committed), 1)
	assertAuthenticationError(t, reopened, credentialDurableOld, contract.ErrUnauthenticated)
	mustAuthenticate(t, reopened, credentialDurableNext)
	if replay := mustRotateCredential(
		t, reopened, credentialDurableNext, credentialDurableOld.ID,
		credentialDurableNext, initial, "credential-process-rotate",
	); replay != committed {
		t.Fatalf("process-exit credential replay: got %#v, want %#v", replay, committed)
	}
}

func credentialDurableSeed() contract.Seed {
	return contract.Seed{
		ID:              credentialDurableActor.Realm,
		LocalPrincipals: []values.PrincipalID{credentialDurableActor.Principal},
		Bindings: []contract.PermissionBinding{
			administrationBinding(
				credentialDurableActor,
				contract.PermissionRotateOwnCredential,
				credentialDurableActor.Realm,
			),
			administrationBinding(
				credentialDurableActor,
				contract.PermissionRevokeCredential,
				credentialDurableActor.Realm,
			),
		},
		Credentials: []contract.CredentialBootstrap{
			{Subject: credentialDurableActor.Principal, Evidence: credentialDurableOld},
			{Subject: credentialDurableActor.Principal, Evidence: credentialDurableSpare},
		},
	}
}

func assertCredentialSecretsAbsent(t *testing.T, stored []byte, secrets ...contract.CredentialSecret) {
	t.Helper()
	for _, secret := range secrets {
		encodings := [][]byte{
			secret[:],
			[]byte(hex.EncodeToString(secret[:])),
			[]byte(base64.StdEncoding.EncodeToString(secret[:])),
		}
		for _, encoded := range encodings {
			if bytes.Contains(stored, encoded) {
				t.Fatalf("Realm Store retained credential secret encoding %q", encoded)
			}
		}
	}
}
