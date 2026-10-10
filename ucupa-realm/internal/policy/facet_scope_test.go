package policy

import (
	"bytes"
	"errors"
	"path/filepath"
	"testing"

	"ucupa/shared/values"
	contract "ucupa/ucupa-realm/contract"
)

func TestFacetQualifiedScopesRetainIsolationThroughAdministrationAndReopen(t *testing.T) {
	for _, namespace := range []bool{false, true} {
		name := "key"
		if namespace {
			name = "namespace"
		}
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "realm.db")
			policy := mustCreateAtomicRealm(t, path)
			defer func() { _ = policy.Close() }()
			target := atomicFacetBinding
			if namespace {
				target.Scope = values.FacetElementScope(fixtureFacetOwner, atomicFacetElement)
			}
			before := policy.BindingSetRevision()
			written, err := policy.PutBinding(atomicManager, target, before, "facet-qualified-put")
			if err != nil {
				t.Fatal(err)
			}
			assertAtomicBindingDecision(t, policy, target, true)
			otherOwner := values.FactAuthorityRef("00000000-0000-4000-8000-000000009902")
			other := target
			if namespace {
				other.Scope = values.FacetElementScope(otherOwner, atomicFacetElement)
			} else {
				other.Scope = values.FacetKeyScope(otherOwner, atomicFacetElement, fixtureType("mesh"))
			}
			assertAtomicBindingDecision(t, policy, other, false)
			otherKind := target
			if namespace {
				otherKind.Scope = values.FacetKeyScope(fixtureFacetOwner, atomicFacetElement, fixtureType("mesh"))
			} else {
				otherKind.Scope = values.FacetElementScope(fixtureFacetOwner, atomicFacetElement)
			}
			assertAtomicBindingDecision(t, policy, otherKind, false)
			otherElement := target
			if namespace {
				otherElement.Scope = values.FacetElementScope(fixtureFacetOwner, atomicElementID)
			} else {
				otherElement.Scope = values.FacetKeyScope(fixtureFacetOwner, atomicElementID, fixtureType("mesh"))
			}
			assertAtomicBindingDecision(t, policy, otherElement, false)
			if err := policy.Close(); err != nil {
				t.Fatal(err)
			}
			policy = mustOpenAtomicRealm(t, path)
			assertAtomicBindingDecision(t, policy, target, true)
			assertAtomicBindingDecision(t, policy, other, false)
			assertAtomicBindingDecision(t, policy, otherKind, false)
			if replay, err := policy.PutBinding(atomicManager, target, before, "facet-qualified-put"); err != nil || replay != written {
				t.Fatal("exact retry lost", replay, err)
			}
			if _, err := policy.PutBinding(atomicManager, other, before, "facet-qualified-put"); !errors.Is(err, contract.ErrOperationConflict) {
				t.Fatal("owner omitted from intent", err)
			}
			if _, err := policy.DeleteBinding(atomicManager, target, written, "facet-qualified-delete"); err != nil {
				t.Fatal(err)
			}
			assertAtomicBindingDecision(t, policy, target, false)
			if replay, err := policy.PutBinding(atomicManager, target, before, "facet-qualified-put"); err != nil || replay != written {
				t.Fatal("historical retry", err)
			}
			assertAtomicBindingDecision(t, policy, target, false)
			if err := policy.Close(); err != nil {
				t.Fatal(err)
			}
			policy = mustOpenAtomicRealm(t, path)
			assertAtomicBindingDecision(t, policy, target, false)
		})
	}
}

func TestFacetScopeCodecPreservesOpaqueTypeAndRejectsMalformedProfiles(t *testing.T) {
	for _, raw := range [][]byte{append([]byte{0, 255, 128, '/'}, make([]byte, 28)...), bytes.Repeat([]byte{255}, 32)} {
		ref, err := values.NewFacetTypeRef(raw)
		if err != nil {
			t.Fatal(err)
		}
		target := atomicFacetBinding
		target.Scope = values.FacetKeyScope(fixtureFacetOwner, atomicFacetElement, ref)
		encoded, err := encodePermissionBinding(target)
		if err != nil {
			t.Fatal(err)
		}
		decoded, err := decodePermissionBinding(encoded)
		if err != nil || decoded != target {
			t.Fatal("opaque round trip", decoded, err)
		}
		badOwner := bytes.Replace(encoded, []byte(fixtureFacetOwner), bytes.Repeat([]byte{'x'}, 36), 1)
		if _, err := decodePermissionBinding(badOwner); err == nil {
			t.Fatal("malformed owner accepted")
		}
		if _, err := decodePermissionBinding(append(bytes.Clone(encoded), 0)); err == nil {
			t.Fatal("trailing bytes accepted")
		}
	}
	// Construct independently: element-only profile with missing owner, empty
	// type or oversized type is never a second accepted scope representation.
	for _, raw := range [][]byte{nil, bytes.Repeat([]byte{'x'}, 257)} {
		encoded := atomicIndependentField(nil, []byte(atomicSubject.Realm))
		encoded = atomicIndependentField(encoded, []byte(atomicSubject.Principal))
		encoded = atomicIndependentField(encoded, []byte(atomicFacetPermission))
		encoded = append(encoded, 3)
		encoded = atomicIndependentField(encoded, []byte(fixtureFacetOwner))
		encoded = atomicIndependentField(encoded, []byte(atomicFacetElement))
		encoded = atomicIndependentField(encoded, raw)
		if _, err := decodePermissionBinding(encoded); err == nil {
			t.Fatal("invalid type accepted")
		}
	}
	invalid := atomicFacetBinding
	invalid.Scope = values.FacetKeyScope("", atomicFacetElement, fixtureType("mesh"))
	if _, err := encodePermissionBinding(invalid); err == nil {
		t.Fatal("unqualified scope encoded")
	}
	if _, err := New(contract.Seed{ID: "realm-a", Bindings: []contract.PermissionBinding{invalid}}); !errors.Is(err, contract.ErrInvalidSeed) {
		t.Fatal("unqualified seed accepted", err)
	}
}
