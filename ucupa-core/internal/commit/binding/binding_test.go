package binding

import (
	"math"
	"strings"
	"testing"

	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	elementfact "ucupa/ucupa-core/internal/element/fact"
	"ucupa/ucupa-core/internal/geometry"
	"ucupa/ucupa-core/internal/token"
)

const (
	bindingTestElement values.ElementID = "00000000-0000-4000-8000-000000000002"
	bindingTestParent  values.ElementID = "00000000-0000-4000-8000-000000000001"
)

func bindingTestValue(t *testing.T) Binding {
	t.Helper()
	rangeValue, err := contract.NewBoundedSphere(2)
	if err != nil {
		t.Fatal(err)
	}
	return Binding{
		Actor:   values.PrincipalRef{Realm: "realm", Principal: "principal"},
		Kind:    elementfact.OperationCreateAnchor,
		Element: bindingTestElement,
		Final: contract.SpatialTuple{
			Parent:    bindingTestParent,
			LocalPose: geometry.IdentityPose(),
			Range:     rangeValue,
		},
	}
}

func bindingTestRevision() contract.Revision {
	var scope [32]byte
	scope[0] = 1
	return token.RevisionFromParts(scope, bindingTestElement, 1)
}

func TestValidOperationIDUsesByteBoundaries(t *testing.T) {
	for _, test := range []struct {
		length int
		valid  bool
	}{
		{length: 0, valid: false},
		{length: 1, valid: true},
		{length: 128, valid: true},
		{length: 129, valid: false},
	} {
		if got := ValidOperationID(contract.OperationID(strings.Repeat("a", test.length))); got != test.valid {
			t.Errorf("ValidOperationID(%d bytes) = %v, want %v", test.length, got, test.valid)
		}
	}
	if !ValidOperationID(contract.OperationID(strings.Repeat("é", 64))) {
		t.Error("ValidOperationID should count 64 two-byte runes as 128 bytes")
	}
	if ValidOperationID(contract.OperationID(strings.Repeat("é", 65))) {
		t.Error("ValidOperationID should reject 130 bytes")
	}
}

func TestBindingValidityEqualityAndCanonicalityRemainDistinct(t *testing.T) {
	base := bindingTestValue(t)
	if !ValidBinding(base) || !CanonicalBindingForStore(base) {
		t.Fatalf("valid canonical binding rejected: %#v", base)
	}

	nonCanonical := base
	nonCanonical.Final.LocalPose.Translation.X = math.Copysign(0, -1)
	if !ValidBinding(nonCanonical) {
		t.Fatal("ValidBinding should retain the existing permissive valid-pose check")
	}
	if CanonicalBindingForStore(nonCanonical) {
		t.Fatal("CanonicalBindingForStore accepted a negative-zero pose")
	}
	if EqualBindings(base, nonCanonical) {
		t.Fatal("EqualBindings normalized distinct floating-point bits")
	}

	update := base
	update.Kind = elementfact.OperationUpdateAnchor
	update.Expected = bindingTestRevision()
	if !ValidBinding(update) || !CanonicalBindingForStore(update) {
		t.Fatal("valid update binding rejected")
	}
	withoutExpected := update
	withoutExpected.Expected = contract.Revision{}
	if ValidBinding(withoutExpected) {
		t.Fatal("update binding with unresolved local revision was accepted")
	}
}

func TestKindNamesRoundTripAndRejectUnknown(t *testing.T) {
	for _, kind := range []elementfact.OperationKind{
		elementfact.OperationInvalid,
		elementfact.OperationCreateRoot,
		elementfact.OperationCreateAnchor,
		elementfact.OperationCreateEntity,
		elementfact.OperationUpdateAnchor,
		elementfact.OperationUpdateEntity,
		elementfact.OperationDeleteAnchor,
		elementfact.OperationDeleteEntity,
	} {
		name := KindName(kind)
		if kind == elementfact.OperationInvalid {
			if name != "" {
				t.Fatalf("KindName(Invalid) = %q, want empty", name)
			}
			continue
		}
		if got := KindFromName(name); got != kind {
			t.Errorf("KindFromName(KindName(%d)) = %d, want %d", kind, got, kind)
		}
	}
	for _, unknown := range []string{"", "create-rootx", "update", "delete-entity ", "CREATE-ANCHOR"} {
		if got := KindFromName(unknown); got != elementfact.OperationInvalid {
			t.Errorf("KindFromName(%q) = %d, want Invalid", unknown, got)
		}
	}
	if KindName(elementfact.OperationKind(255)) != "" {
		t.Fatal("unknown operation kind received a persisted name")
	}
}
