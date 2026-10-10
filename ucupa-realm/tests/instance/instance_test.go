package instance_test

import (
	"errors"
	"path/filepath"
	"reflect"
	"testing"

	"ucupa/shared/values"
	"ucupa/ucupa-realm/contract"
	"ucupa/ucupa-realm/instance"
)

func TestPublicConstructionAndOriginalStoreReopen(t *testing.T) {
	subject := values.PrincipalRef{Realm: "realm-public", Principal: "admin"}
	seed := contract.Seed{ID: subject.Realm, LocalPrincipals: []values.PrincipalID{subject.Principal}, Bindings: []contract.PermissionBinding{
		{Subject: subject, Permission: contract.PermissionPutBinding, Scope: values.RealmScope(subject.Realm)},
	}}
	path := filepath.Join(t.TempDir(), "realm.db")
	r, err := instance.CreateDurable(seed, path)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	before := r.BindingSetRevision()
	binding := contract.PermissionBinding{Subject: subject, Permission: "read", Scope: values.CoreElementScope("00000000-0000-4000-8000-000000000001", "00000000-0000-4000-8000-000000000002")}
	after, err := r.PutBinding(subject, binding, before, "public-put")
	if err != nil || before == after {
		t.Fatalf("put: %v", err)
	}
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}
	r, err = instance.OpenDurable(path)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	if r.ID() != seed.ID || r.BindingSetRevision() != after {
		t.Fatal("original identity/revision changed")
	}
	if replay, err := r.PutBinding(subject, binding, before, "public-put"); err != nil || replay != after {
		t.Fatalf("retained result: %v", err)
	}
	if _, err := instance.OpenDurable(filepath.Join(t.TempDir(), "absent.db")); !errors.Is(err, contract.ErrStore) {
		t.Fatalf("missing Store: %v", err)
	}
	memory, err := instance.New(seed)
	if err != nil {
		t.Fatal(err)
	}
	if err := memory.Close(); err != nil {
		t.Fatal(err)
	}
	if id, live := memory.ActiveID(); !live || id != seed.ID {
		t.Fatal("memory Close changed its original no-op semantics")
	}
}

func TestPublicRevisionsRemainOpaqueEqualityValues(t *testing.T) {
	for _, value := range []any{contract.BindingSetRevision{}, contract.CredentialSetRevision{}} {
		typ := reflect.TypeOf(value)
		if !typ.Comparable() || typ.NumMethod() != 0 {
			t.Fatalf("revision lost equality-only surface: %v", typ)
		}
		for i := 0; i < typ.NumField(); i++ {
			if typ.Field(i).IsExported() {
				t.Fatalf("revision exposed %s", typ.Field(i).Name)
			}
		}
	}
}
