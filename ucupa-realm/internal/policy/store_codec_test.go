package policy

import (
	"bytes"
	"errors"
	"path/filepath"
	"testing"

	"go.etcd.io/bbolt"

	"ucupa/shared/values"
	contract "ucupa/ucupa-realm/contract"
)

func TestRealmStoreFormatIDAndExactSixBucketLayout(t *testing.T) {
	if got, want := realmFormatID, atomicFormatID; !bytes.Equal(got, want) {
		t.Fatalf("Realm Store FormatID: got %q, want %q", got, want)
	}

	path := filepath.Join(t.TempDir(), "realm.db")
	owner := mustCreateAtomicRealm(t, path, atomicElementBinding, atomicFacetBinding)
	revision := owner.BindingSetRevision()
	credentialRevision := owner.CredentialSetRevision()
	if err := owner.Close(); err != nil {
		t.Fatalf("close created Realm: %v", err)
	}

	database, err := bbolt.Open(path, 0o600, &bbolt.Options{ReadOnly: true})
	if err != nil {
		t.Fatalf("open Realm Store for inspection: %v", err)
	}
	viewErr := database.View(func(tx *bbolt.Tx) error {
		return assertAtomicLayout(tx, revision, credentialRevision)
	})
	closeErr := database.Close()
	if viewErr != nil || closeErr != nil {
		t.Fatalf("inspect Realm Store: view=%v close=%v", viewErr, closeErr)
	}

	fixturePath := filepath.Join(t.TempDir(), "independent.db")
	writeAtomicIndependentFixture(t, fixturePath, nil)
	opened := mustOpenAtomicRealm(t, fixturePath)
	defer func() { _ = opened.Close() }()
	if got, want := opened.BindingSetRevision(), atomicFixtureRevision(); got != want {
		t.Fatalf("independent fixture revision: got %#v, want %#v", got, want)
	}
	assertAtomicBindingDecision(t, opened, atomicElementBinding, true)
	assertAtomicBindingDecision(t, opened, atomicFacetBinding, true)
	assertAtomicBindingDecision(t, opened, atomicManagerPutBinding, true)
}

func TestRealmPermissionBindingCodecMatchesIndependentGoldens(t *testing.T) {
	tests := []struct {
		name    string
		binding contract.PermissionBinding
		golden  []byte
	}{
		{
			name:    "Core Element",
			binding: atomicElementBinding,
			golden: []byte("\x00\x00\x00\x07realm-a" +
				"\x00\x00\x00\x05alice" +
				"\x00\x00\x00\x01p" +
				"\x01" +
				"\x00\x00\x00\x24" + string(realmCoreRef) +
				"\x00\x00\x00\x24" + string(atomicElementID)),
		},
		{
			name:    "Facet key",
			binding: atomicFacetBinding,
			golden: []byte("\x00\x00\x00\x07realm-a" +
				"\x00\x00\x00\x05alice" +
				"\x00\x00\x00\x01q" +
				"\x03" +
				"\x00\x00\x00\x24" + string(fixtureFacetOwner) +
				"\x00\x00\x00\x24" + string(atomicFacetElement) +
				"\x00\x00\x00\x20\xd3\x0c\xa7\xa7\xa3\x2b\xf5\x77\x2d\xc5\xeb\x2a\x2e\x7b\xd3\x57\x37\xef\xf7\x95\xad\x74\xf2\x47\x9b\x35\x97\x16\xb5\x9a\xbd\xfa"),
		},
		{
			name: "Facet namespace",
			binding: contract.PermissionBinding{Subject: atomicSubject, Permission: atomicFacetPermission,
				Scope: values.FacetElementScope(fixtureFacetOwner, atomicFacetElement)},
			golden: []byte("\x00\x00\x00\x07realm-a" +
				"\x00\x00\x00\x05alice" + "\x00\x00\x00\x01q" + "\x02" +
				"\x00\x00\x00\x24" + string(fixtureFacetOwner) +
				"\x00\x00\x00\x24" + string(atomicFacetElement)),
		},
		{
			name:    "Realm",
			binding: atomicManagerPutBinding,
			golden: []byte("\x00\x00\x00\x07realm-a" +
				"\x00\x00\x00\x07manager" +
				"\x00\x00\x00\x17ucupa.realm.binding.put" +
				"\x04" +
				"\x00\x00\x00\x07realm-a"),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			encoded, err := encodePermissionBinding(test.binding)
			if err != nil {
				t.Fatalf("encodePermissionBinding: %v", err)
			}
			if !bytes.Equal(encoded, test.golden) {
				t.Fatalf("encoded binding:\n got %x\nwant %x", encoded, test.golden)
			}
			decoded, err := decodePermissionBinding(test.golden)
			if err != nil {
				t.Fatalf("decode independent golden: %v", err)
			}
			if decoded != test.binding {
				t.Fatalf("decoded binding: got %#v, want %#v", decoded, test.binding)
			}
		})
	}
}

func TestCoreElementBindingKeepsOwnerInAuthorizationHistoryAndReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "realm.db")
	owner := mustCreateAtomicRealm(t, path, atomicElementBinding, atomicFacetBinding)
	defer func() { _ = owner.Close() }()
	other := atomicElementBinding
	other.Scope = values.CoreElementScope("00000000-0000-4000-8000-000000000302", atomicElementID)
	assertAtomicBindingDecision(t, owner, atomicElementBinding, true)
	assertAtomicBindingDecision(t, owner, other, false)
	expected := owner.BindingSetRevision()
	added, err := owner.PutBinding(atomicManager, other, expected, "grant-other-core")
	if err != nil {
		t.Fatalf("grant other Core: %v", err)
	}
	if _, err := owner.PutBinding(atomicManager, atomicElementBinding, expected, "grant-other-core"); !errors.Is(err, contract.ErrOperationConflict) {
		t.Fatalf("same operation with different Core: got %v, want ErrOperationConflict", err)
	}
	deleted, err := owner.DeleteBinding(atomicManager, atomicElementBinding, added, "delete-first-core")
	if err != nil {
		t.Fatalf("delete first Core grant: %v", err)
	}
	assertAtomicBindingDecision(t, owner, atomicElementBinding, false)
	assertAtomicBindingDecision(t, owner, other, true)
	assertAtomicBindingDecision(t, owner, atomicFacetBinding, true)
	if err := owner.Close(); err != nil {
		t.Fatal(err)
	}
	owner = mustOpenAtomicRealm(t, path)
	if got := owner.BindingSetRevision(); got != deleted {
		t.Fatalf("reopened revision: got %#v, want %#v", got, deleted)
	}
	assertAtomicBindingDecision(t, owner, atomicElementBinding, false)
	assertAtomicBindingDecision(t, owner, other, true)
	assertAtomicBindingDecision(t, owner, atomicFacetBinding, true)
	if got, err := owner.PutBinding(atomicManager, other, expected, "grant-other-core"); err != nil || got != added {
		t.Fatalf("reopened exact replay: got %#v, %v; want %#v", got, err, added)
	}
	if _, err := owner.PutBinding(atomicManager, atomicElementBinding, expected, "grant-other-core"); !errors.Is(err, contract.ErrOperationConflict) {
		t.Fatalf("reopened operation with different Core: got %v, want ErrOperationConflict", err)
	}
}

func TestCoreElementBindingCodecRequiresBothExactTargets(t *testing.T) {
	prefix := []byte("\x00\x00\x00\x07realm-a" + "\x00\x00\x00\x05alice" + "\x00\x00\x00\x01p" + "\x01")
	for name, fields := range map[string][][]byte{
		"missing element": {[]byte(realmCoreRef)},
		"empty Core":      {nil, []byte(atomicElementID)},
		"invalid Core":    {[]byte("invalid"), []byte(atomicElementID)},
		"empty Element":   {[]byte(realmCoreRef), nil},
		"invalid Element": {[]byte(realmCoreRef), []byte("invalid")},
		"trailing target": {[]byte(realmCoreRef), []byte(atomicElementID), []byte(atomicLaterElement)},
	} {
		t.Run(name, func(t *testing.T) {
			encoded := bytes.Clone(prefix)
			for _, field := range fields {
				encoded = atomicIndependentField(encoded, field)
			}
			if got, err := decodePermissionBinding(encoded); err == nil || got != (contract.PermissionBinding{}) {
				t.Fatalf("malformed Core Element scope decoded: %#v, %v", got, err)
			}
		})
	}
}

func TestRealmOperationCodecMatchesIndependentGoldens(t *testing.T) {
	localExpected := atomicRevision(0x11, 7)
	changedRevision := atomicRevision(0x11, 8)
	foreignExpected := atomicRevision(0x22, 19)
	missingTarget := atomicElementBinding
	missingTarget.Subject.Principal = "missing"

	tests := []struct {
		name    string
		binding operationBinding
		outcome retainedOutcome
		golden  []byte
	}{
		{
			name: "changed success",
			binding: operationBinding{
				actor: atomicManager, kind: operationPutBinding,
				target: atomicElementBinding, expected: localExpected,
			},
			outcome: retainedOutcome{kind: outcomeSuccess, revision: changedRevision},
			golden: atomicIndependentOperation(
				1, atomicManager, atomicIndependentBinding(atomicElementBinding), localExpected,
				1, &changedRevision,
			),
		},
		{
			name: "no-op success",
			binding: operationBinding{
				actor: atomicManager, kind: operationDeleteBinding,
				target: atomicElementBinding, expected: localExpected,
			},
			outcome: retainedOutcome{kind: outcomeSuccess, revision: localExpected},
			golden: atomicIndependentOperation(
				2, atomicManager, atomicIndependentBinding(atomicElementBinding), localExpected,
				1, &localExpected,
			),
		},
		{
			name: "not found",
			binding: operationBinding{
				actor: atomicManager, kind: operationPutBinding,
				target: missingTarget, expected: localExpected,
			},
			outcome: retainedOutcome{kind: outcomeNotFound},
			golden: atomicIndependentOperation(
				1, atomicManager, atomicIndependentBinding(missingTarget), localExpected, 2, nil,
			),
		},
		{
			name: "revision conflict",
			binding: operationBinding{
				actor: atomicManager, kind: operationDeleteBinding,
				target: atomicElementBinding, expected: foreignExpected,
			},
			outcome: retainedOutcome{kind: outcomeRevisionConflict},
			golden: atomicIndependentOperation(
				2, atomicManager, atomicIndependentBinding(atomicElementBinding), foreignExpected, 3, nil,
			),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			encoded, err := encodeOperation(test.binding, test.outcome)
			if err != nil {
				t.Fatalf("encodeOperation: %v", err)
			}
			if !bytes.Equal(encoded, test.golden) {
				t.Fatalf("encoded operation:\n got %x\nwant %x", encoded, test.golden)
			}
			binding, outcome, err := decodeOperation(test.golden)
			if err != nil {
				t.Fatalf("decode independent golden: %v", err)
			}
			if binding != test.binding || outcome != test.outcome {
				t.Fatalf("decoded operation: got %#v/%#v, want %#v/%#v",
					binding, outcome, test.binding, test.outcome)
			}
		})
	}
}
