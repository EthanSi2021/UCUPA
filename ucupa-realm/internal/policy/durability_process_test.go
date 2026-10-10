package policy

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	contract "ucupa/ucupa-realm/contract"
	token "ucupa/ucupa-realm/internal/token"
)

const (
	realmDurableHelperEnvironment         = "UCUPA_REALM_DURABLE_HELPER_STORE"
	realmDurableMutationHelperEnvironment = "UCUPA_REALM_DURABLE_MUTATION_HELPER_STORE"
	realmDurableProcessExitOperationID    = contract.OperationID("durability-process-exit-put")
)

func TestRealmDurableProcessExitHelper(t *testing.T) {
	path := os.Getenv(realmDurableHelperEnvironment)
	if path == "" {
		return
	}
	if _, err := CreateDurable(durableRealmSeed(), path); err != nil {
		os.Exit(2)
	}
	// Deliberately leave the long-lived owner open. Process exit is the recovery
	// boundary; the parent can observe state only through an independent reopen.
	os.Exit(0)
}

func TestRealmDurableRecoversAfterProcessExit(t *testing.T) {
	path := filepath.Join(t.TempDir(), "realm.db")
	command := exec.Command(os.Args[0], "-test.run=^TestRealmDurableProcessExitHelper$")
	command.Env = append(os.Environ(), realmDurableHelperEnvironment+"="+path)
	if err := command.Run(); err != nil {
		t.Fatalf("durable Realm helper: %v", err)
	}

	reopened := mustOpenDurableRealm(t, path)
	assertRealmStoreLocked(t, path)
	assertActiveRealmSemantics(t, reopened)
	if err := reopened.Close(); err != nil {
		t.Fatalf("Close process-exit recovery owner: %v", err)
	}
	assertRealmStoreUnlocked(t, path)
}

func TestRealmDurableCommittedMutationProcessExitHelper(t *testing.T) {
	path := os.Getenv(realmDurableMutationHelperEnvironment)
	if path == "" {
		return
	}
	owner, err := CreateDurable(durableRealmSeed(), path)
	if err != nil {
		os.Exit(2)
	}
	initial := owner.BindingSetRevision()
	committed, err := owner.PutBinding(
		durableManagerActor,
		durableDynamicBinding(),
		initial,
		realmDurableProcessExitOperationID,
	)
	if err != nil || committed == (contract.BindingSetRevision{}) || committed == initial {
		os.Exit(3)
	}
	if !owner.Authorize(durableLocalActor, durableDynamicRequirement) {
		os.Exit(4)
	}
	// The mutation returned after its durable commit. Do not Close: process exit
	// is the only boundary before the parent independently reopens and replays.
	os.Exit(0)
}

func TestRealmDurableReplaysCommittedMutationAfterProcessExit(t *testing.T) {
	path := filepath.Join(t.TempDir(), "realm.db")
	command := exec.Command(os.Args[0], "-test.run=^TestRealmDurableCommittedMutationProcessExitHelper$")
	command.Env = append(os.Environ(), realmDurableMutationHelperEnvironment+"="+path)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("durable Realm mutation helper: %v\n%s", err, output)
	}

	reopened := mustOpenDurableRealm(t, path)
	committed := reopened.BindingSetRevision()
	if token.Generation(committed) != 2 {
		t.Fatalf("process-exit committed generation: got %d, want 2", token.Generation(committed))
	}
	initial := token.Binding(token.Scope(committed), 1)
	if !reopened.Authorize(durableLocalActor, durableDynamicRequirement) {
		t.Fatal("process-exit reopen lost the committed binding")
	}
	replayed, err := reopened.PutBinding(
		durableManagerActor,
		durableDynamicBinding(),
		initial,
		realmDurableProcessExitOperationID,
	)
	if err != nil || replayed != committed {
		t.Fatalf("process-exit exact replay: revision=%#v err=%v, want %#v/nil", replayed, err, committed)
	}
	if got := reopened.BindingSetRevision(); got != committed {
		t.Fatalf("process-exit replay changed revision: got %#v, want %#v", got, committed)
	}
	if err := reopened.Close(); err != nil {
		t.Fatalf("close process-exit mutation owner: %v", err)
	}
	assertRealmStoreUnlocked(t, path)
}
