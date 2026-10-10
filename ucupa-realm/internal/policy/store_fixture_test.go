package policy

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"go.etcd.io/bbolt"

	"ucupa/shared/values"
	contract "ucupa/ucupa-realm/contract"
	token "ucupa/ucupa-realm/internal/token"
)

const (
	atomicRealmID         values.RealmID      = "realm-a"
	atomicElementID       values.ElementID    = "00000000-0000-4000-8000-000000000201"
	atomicFacetElement    values.ElementID    = "00000000-0000-4000-8000-000000000202"
	atomicLaterElement    values.ElementID    = "00000000-0000-4000-8000-000000000203"
	atomicPermission      values.PermissionID = "p"
	atomicFacetPermission values.PermissionID = "q"
)

var (
	atomicMetaBucket            = []byte("ucupa.realm.meta")
	atomicPrincipalBucket       = []byte("ucupa.realm.local-principals")
	atomicBindingBucket         = []byte("ucupa.realm.permission-bindings")
	atomicOperationBucket       = []byte("ucupa.realm.binding-operations")
	atomicCredentialBucket      = []byte("ucupa.realm.credentials")
	atomicCredentialOpBucket    = []byte("ucupa.realm.credential-operations")
	atomicFormatKey             = []byte("format")
	atomicRealmIDKey            = []byte("realm-id")
	atomicRevisionScopeKey      = []byte("revision-scope")
	atomicRevisionGenerationKey = []byte("revision-generation")
	atomicCredentialScopeKey    = []byte("credential-revision-scope")
	atomicCredentialGenKey      = []byte("credential-revision-generation")
	atomicFormatID              = []byte("ucupa.realm.local@a33aa9c69815480491bca89ce0c6d653")
	atomicManager               = values.PrincipalRef{Realm: atomicRealmID, Principal: "manager"}
	atomicSubject               = values.PrincipalRef{Realm: atomicRealmID, Principal: "alice"}
	atomicElementBinding        = contract.PermissionBinding{Subject: atomicSubject, Permission: atomicPermission, Scope: values.CoreElementScope(realmCoreRef, atomicElementID)}
	atomicFacetBinding          = contract.PermissionBinding{Subject: atomicSubject, Permission: atomicFacetPermission, Scope: values.FacetKeyScope(fixtureFacetOwner, atomicFacetElement, fixtureType("mesh"))}
	atomicManagerPutBinding     = contract.PermissionBinding{Subject: atomicManager, Permission: contract.PermissionPutBinding, Scope: values.RealmScope(atomicRealmID)}
	atomicManagerDeleteBinding  = contract.PermissionBinding{Subject: atomicManager, Permission: contract.PermissionDeleteBinding, Scope: values.RealmScope(atomicRealmID)}
)

func atomicSeed(extra ...contract.PermissionBinding) contract.Seed {
	bindings := []contract.PermissionBinding{atomicManagerPutBinding, atomicManagerDeleteBinding}
	bindings = append(bindings, extra...)
	return contract.Seed{
		ID:              atomicRealmID,
		LocalPrincipals: []values.PrincipalID{atomicManager.Principal, atomicSubject.Principal},
		Bindings:        bindings,
	}
}

func mustCreateAtomicRealm(t *testing.T, path string, extra ...contract.PermissionBinding) *Realm {
	t.Helper()
	owner, err := CreateDurable(atomicSeed(extra...), path)
	if err != nil || owner == nil {
		t.Fatalf("CreateDurable: Realm=%v err=%v", owner, err)
	}
	return owner
}

func mustOpenAtomicRealm(t *testing.T, path string) *Realm {
	t.Helper()
	owner, err := OpenDurable(path)
	if err != nil || owner == nil {
		t.Fatalf("OpenDurable: Realm=%v err=%v", owner, err)
	}
	return owner
}

func atomicLaterBinding(permission string) contract.PermissionBinding {
	return contract.PermissionBinding{
		Subject:    atomicSubject,
		Permission: values.PermissionID(permission),
		Scope:      values.CoreElementScope(realmCoreRef, atomicLaterElement),
	}
}

func atomicInjectedFailure(label string) func() error {
	return func() error { return fmt.Errorf("injected %s failure", label) }
}

func assertAtomicOwnerActiveAt(t *testing.T, owner *Realm, revision contract.BindingSetRevision) {
	t.Helper()
	if got := owner.BindingSetRevision(); got != revision {
		t.Fatalf("owner is not active at unchanged revision: got %#v, want %#v", got, revision)
	}
	assertAtomicBindingDecision(t, owner, atomicManagerPutBinding, true)
}

func assertAtomicOwnerRetired(t *testing.T, owner *Realm) {
	t.Helper()
	if got := owner.BindingSetRevision(); got != (contract.BindingSetRevision{}) {
		t.Fatalf("retired owner exposed revision %#v", got)
	}
	if owner.Authorize(atomicManager, values.Requirement{
		Permission: contract.PermissionPutBinding, Scope: values.RealmScope(atomicRealmID),
	}) {
		t.Fatal("retired owner authorized manager")
	}
	if owner.state.store == nil || owner.state.store.db != nil || !owner.state.retired {
		t.Fatalf("indeterminate owner did not retire Store: store=%#v retired=%v", owner.state.store, owner.state.retired)
	}
}

func assertAtomicBindingDecision(t *testing.T, owner *Realm, binding contract.PermissionBinding, want bool) {
	t.Helper()
	got := owner.Authorize(binding.Subject, values.Requirement{
		Permission: binding.Permission, Scope: binding.Scope,
	})
	if got != want {
		t.Fatalf("Authorize(%#v): got %v, want %v", binding, got, want)
	}
}

func assertAtomicOperationCount(t *testing.T, owner *Realm, want int) {
	t.Helper()
	if owner.state.store == nil || owner.state.store.db == nil {
		t.Fatal("cannot inspect closed Realm Store")
	}
	err := owner.state.store.db.View(func(tx *bbolt.Tx) error {
		bucket := tx.Bucket(atomicOperationBucket)
		if bucket == nil {
			return errors.New("operation bucket is missing")
		}
		count := 0
		if err := bucket.ForEach(func(_, value []byte) error {
			if value == nil {
				return errors.New("nested operation bucket")
			}
			count++
			return nil
		}); err != nil {
			return err
		}
		if count != want {
			return fmt.Errorf("operation count: got %d, want %d", count, want)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("inspect retained operations: %v", err)
	}
}

func assertAtomicLayout(
	tx *bbolt.Tx,
	revision contract.BindingSetRevision,
	credentialRevision contract.CredentialSetRevision,
) error {
	wantTop := map[string]struct{}{
		string(atomicMetaBucket): {}, string(atomicPrincipalBucket): {},
		string(atomicBindingBucket): {}, string(atomicOperationBucket): {},
		string(atomicCredentialBucket): {}, string(atomicCredentialOpBucket): {},
	}
	gotTop := make(map[string]struct{})
	if err := tx.ForEach(func(name []byte, _ *bbolt.Bucket) error {
		gotTop[string(name)] = struct{}{}
		return nil
	}); err != nil {
		return err
	}
	if !reflect.DeepEqual(gotTop, wantTop) {
		return fmt.Errorf("top-level buckets: got %#v, want %#v", gotTop, wantTop)
	}

	buckets := map[string]*bbolt.Bucket{
		"meta":                  tx.Bucket(atomicMetaBucket),
		"principals":            tx.Bucket(atomicPrincipalBucket),
		"bindings":              tx.Bucket(atomicBindingBucket),
		"operations":            tx.Bucket(atomicOperationBucket),
		"credentials":           tx.Bucket(atomicCredentialBucket),
		"credential-operations": tx.Bucket(atomicCredentialOpBucket),
	}
	for name, bucket := range buckets {
		if bucket == nil {
			return fmt.Errorf("missing %s bucket", name)
		}
		if bucket.Sequence() != 0 {
			return fmt.Errorf("%s bucket sequence: got %d, want 0", name, bucket.Sequence())
		}
		if err := bucket.ForEach(func(key, value []byte) error {
			if value == nil {
				return fmt.Errorf("%s contains nested bucket %q", name, key)
			}
			return nil
		}); err != nil {
			return err
		}
	}

	wantGeneration := make([]byte, 8)
	binary.BigEndian.PutUint64(wantGeneration, token.Generation(revision))
	wantCredentialGeneration := make([]byte, 8)
	binary.BigEndian.PutUint64(wantCredentialGeneration, token.Generation(credentialRevision))
	wantMeta := map[string][]byte{
		string(atomicFormatKey):             atomicFormatID,
		string(atomicRealmIDKey):            []byte(atomicRealmID),
		string(atomicRevisionScopeKey):      token.ScopeBytes(revision),
		string(atomicRevisionGenerationKey): wantGeneration,
		string(atomicCredentialScopeKey):    token.ScopeBytes(credentialRevision),
		string(atomicCredentialGenKey):      wantCredentialGeneration,
	}
	if got, err := atomicBucketFacts(buckets["meta"]); err != nil {
		return err
	} else if !reflect.DeepEqual(got, wantMeta) {
		return fmt.Errorf("metadata: got %#v, want %#v", got, wantMeta)
	}

	wantPrincipals := map[string][]byte{
		string(atomicManager.Principal): {}, string(atomicSubject.Principal): {},
	}
	if got, err := atomicBucketFacts(buckets["principals"]); err != nil {
		return err
	} else if !reflect.DeepEqual(got, wantPrincipals) {
		return fmt.Errorf("local Principals: got %#v, want %#v", got, wantPrincipals)
	}

	wantBindings := map[string][]byte{}
	for _, binding := range []contract.PermissionBinding{
		atomicManagerPutBinding, atomicManagerDeleteBinding, atomicElementBinding, atomicFacetBinding,
	} {
		wantBindings[string(atomicIndependentBinding(binding))] = []byte{}
	}
	if got, err := atomicBucketFacts(buckets["bindings"]); err != nil {
		return err
	} else if !reflect.DeepEqual(got, wantBindings) {
		return fmt.Errorf("bindings: got %#v, want %#v", got, wantBindings)
	}
	if got, err := atomicBucketFacts(buckets["operations"]); err != nil {
		return err
	} else if len(got) != 0 {
		return fmt.Errorf("bootstrap operations: got %#v, want empty", got)
	}
	if got, err := atomicBucketFacts(buckets["credentials"]); err != nil {
		return err
	} else if len(got) != 0 {
		return fmt.Errorf("bootstrap credentials: got %#v, want empty", got)
	}
	if got, err := atomicBucketFacts(buckets["credential-operations"]); err != nil {
		return err
	} else if len(got) != 0 {
		return fmt.Errorf("bootstrap credential operations: got %#v, want empty", got)
	}
	return nil
}

func atomicBucketFacts(bucket *bbolt.Bucket) (map[string][]byte, error) {
	facts := make(map[string][]byte)
	err := bucket.ForEach(func(key, value []byte) error {
		if value == nil {
			return fmt.Errorf("nested bucket %q", key)
		}
		facts[string(key)] = bytes.Clone(value)
		return nil
	})
	return facts, err
}

func atomicFixtureRevision() contract.BindingSetRevision {
	return atomicRevision(0x33, 1)
}

func atomicFixtureCredentialRevision() contract.CredentialSetRevision {
	var scope [32]byte
	for index := range scope {
		scope[index] = 0x55
	}
	return token.Credential(scope, 1)
}

func atomicRevision(fill byte, generation uint64) contract.BindingSetRevision {
	var scope [32]byte
	for index := range scope {
		scope[index] = fill
	}
	return token.Binding(scope, generation)
}

func atomicRevisionPointer(revision contract.BindingSetRevision) *contract.BindingSetRevision {
	return &revision
}

func atomicIndependentBinding(binding contract.PermissionBinding) []byte {
	encoded := make([]byte, 0, 128)
	encoded = atomicIndependentField(encoded, []byte(binding.Subject.Realm))
	encoded = atomicIndependentField(encoded, []byte(binding.Subject.Principal))
	encoded = atomicIndependentField(encoded, []byte(binding.Permission))
	if targetCore, element, ok := binding.Scope.CoreElementTarget(); ok {
		encoded = append(encoded, 1)
		encoded = atomicIndependentField(encoded, []byte(targetCore))
		return atomicIndependentField(encoded, []byte(element))
	}
	if owner, element, ok := binding.Scope.FacetElementTarget(); ok {
		encoded = append(encoded, 2)
		encoded = atomicIndependentField(encoded, []byte(owner))
		return atomicIndependentField(encoded, []byte(element))
	}
	if owner, facet, ok := binding.Scope.FacetKeyTarget(); ok {
		encoded = append(encoded, 3)
		encoded = atomicIndependentField(encoded, []byte(owner))
		encoded = atomicIndependentField(encoded, []byte(facet.Element))
		return atomicIndependentField(encoded, facet.Type.Bytes())
	}
	if realmID, ok := binding.Scope.RealmTarget(); ok {
		encoded = append(encoded, 4)
		return atomicIndependentField(encoded, []byte(realmID))
	}
	panic("atomicIndependentBinding called with invalid scope")
}

func atomicIndependentOperation(
	kind byte,
	actor values.PrincipalRef,
	target []byte,
	expected contract.BindingSetRevision,
	outcome byte,
	result *contract.BindingSetRevision,
) []byte {
	encoded := []byte{kind}
	encoded = atomicIndependentField(encoded, []byte(actor.Realm))
	encoded = atomicIndependentField(encoded, []byte(actor.Principal))
	encoded = atomicIndependentField(encoded, target)
	encoded = append(encoded, token.ScopeBytes(expected)...)
	encoded = binary.BigEndian.AppendUint64(encoded, token.Generation(expected))
	encoded = append(encoded, outcome)
	if result != nil {
		encoded = append(encoded, token.ScopeBytes(*result)...)
		encoded = binary.BigEndian.AppendUint64(encoded, token.Generation(*result))
	}
	return encoded
}

func atomicIndependentField(encoded, field []byte) []byte {
	encoded = binary.BigEndian.AppendUint32(encoded, uint32(len(field)))
	return append(encoded, field...)
}

func writeAtomicIndependentFixture(t *testing.T, path string, mutate func(*bbolt.Tx) error) {
	t.Helper()
	database, err := bbolt.Open(path, 0o600, nil)
	if err != nil {
		t.Fatalf("open independent Realm fixture: %v", err)
	}
	err = database.Update(func(tx *bbolt.Tx) error {
		meta, err := tx.CreateBucket(atomicMetaBucket)
		if err != nil {
			return err
		}
		principals, err := tx.CreateBucket(atomicPrincipalBucket)
		if err != nil {
			return err
		}
		bindings, err := tx.CreateBucket(atomicBindingBucket)
		if err != nil {
			return err
		}
		if _, err := tx.CreateBucket(atomicOperationBucket); err != nil {
			return err
		}
		if _, err := tx.CreateBucket(atomicCredentialBucket); err != nil {
			return err
		}
		if _, err := tx.CreateBucket(atomicCredentialOpBucket); err != nil {
			return err
		}
		revision := atomicFixtureRevision()
		generation := make([]byte, 8)
		binary.BigEndian.PutUint64(generation, token.Generation(revision))
		credentialRevision := atomicFixtureCredentialRevision()
		credentialGeneration := make([]byte, 8)
		binary.BigEndian.PutUint64(credentialGeneration, token.Generation(credentialRevision))
		for key, value := range map[string][]byte{
			string(atomicFormatKey):             atomicFormatID,
			string(atomicRealmIDKey):            []byte(atomicRealmID),
			string(atomicRevisionScopeKey):      token.ScopeBytes(revision),
			string(atomicRevisionGenerationKey): generation,
			string(atomicCredentialScopeKey):    token.ScopeBytes(credentialRevision),
			string(atomicCredentialGenKey):      credentialGeneration,
		} {
			if err := meta.Put([]byte(key), value); err != nil {
				return err
			}
		}
		for _, principal := range []values.PrincipalID{atomicManager.Principal, atomicSubject.Principal} {
			if err := principals.Put([]byte(principal), nil); err != nil {
				return err
			}
		}
		for _, binding := range []contract.PermissionBinding{
			atomicManagerPutBinding, atomicManagerDeleteBinding, atomicElementBinding, atomicFacetBinding,
		} {
			if err := bindings.Put(atomicIndependentBinding(binding), nil); err != nil {
				return err
			}
		}
		if mutate != nil {
			return mutate(tx)
		}
		return nil
	})
	closeErr := database.Close()
	if err != nil || closeErr != nil {
		t.Fatalf("write independent Realm fixture: update=%v close=%v", err, closeErr)
	}
}

func mutateAtomicStore(t *testing.T, path string, mutate func(*bbolt.Tx) error) {
	t.Helper()
	database, err := bbolt.Open(path, 0o600, nil)
	if err != nil {
		t.Fatalf("open Realm Store for mutation: %v", err)
	}
	updateErr := database.Update(mutate)
	closeErr := database.Close()
	if updateErr != nil || closeErr != nil {
		t.Fatalf("mutate Realm Store: update=%v close=%v", updateErr, closeErr)
	}
}

func assertAtomicOpenRejectedUnchanged(t *testing.T, path string) {
	t.Helper()
	before, err := readAtomicStore(path)
	if err != nil {
		t.Fatalf("read rejected Store before open: %v", err)
	}
	owner, openErr := OpenDurable(path)
	if owner != nil || !errors.Is(openErr, contract.ErrStore) {
		if owner != nil {
			_ = owner.Close()
		}
		t.Fatalf("OpenDurable rejected Store: Realm=%v err=%v, want nil/ErrStore", owner, openErr)
	}
	after, err := readAtomicStore(path)
	if err != nil {
		t.Fatalf("read rejected Store after open: %v", err)
	}
	if !bytes.Equal(after, before) {
		t.Fatal("rejected Realm Store changed bytes")
	}
}

func readAtomicStore(path string) ([]byte, error) {
	return os.ReadFile(path)
}

func createAtomicChangedHistory(t *testing.T, operationID contract.OperationID) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "realm.db")
	owner := mustCreateAtomicRealm(t, path)
	revision := owner.BindingSetRevision()
	got, err := owner.PutBinding(atomicManager, atomicElementBinding, revision, operationID)
	if err != nil || token.Generation(got) != token.Generation(revision)+1 {
		_ = owner.Close()
		t.Fatalf("create changed history: revision=%#v err=%v", got, err)
	}
	if err := owner.Close(); err != nil {
		t.Fatalf("close changed history: %v", err)
	}
	return path
}
