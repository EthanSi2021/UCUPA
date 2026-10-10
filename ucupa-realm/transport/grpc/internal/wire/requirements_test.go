package wire

import (
	"testing"

	"google.golang.org/protobuf/proto"

	"ucupa/shared/values"
	realmpb "ucupa/ucupa-realm/bindings/go/protobuf"
)

const requirementsTestOwner values.FactAuthorityRef = "00000000-0000-4000-8000-000000000001"
const requirementsTestElement values.ElementID = "00000000-0000-4000-8000-000000000010"

func coreElementRequirement(permission string) *realmpb.Requirement {
	return &realmpb.Requirement{
		Permission: proto.String(permission),
		Scope: &realmpb.Scope{
			Kind:      realmpb.ScopeKind_CORE_ELEMENT.Enum(),
			Authority: proto.String(string(requirementsTestOwner)),
			Element:   proto.String(string(requirementsTestElement)),
		},
	}
}

func TestRequirementFromWireCoversEveryDeclaredKind(t *testing.T) {
	facet, err := values.NewFacetTypeRef(make([]byte, 32))
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name string
		in   *realmpb.Requirement
	}{
		{"core element", coreElementRequirement("ucupa.test.read")},
		{"facet element", &realmpb.Requirement{Permission: proto.String("p"), Scope: &realmpb.Scope{Kind: realmpb.ScopeKind_FACET_ELEMENT.Enum(), Authority: proto.String(string(requirementsTestOwner)), Element: proto.String(string(requirementsTestElement))}}},
		{"facet key", &realmpb.Requirement{Permission: proto.String("p"), Scope: &realmpb.Scope{Kind: realmpb.ScopeKind_FACET_KEY.Enum(), Authority: proto.String(string(requirementsTestOwner)), Element: proto.String(string(requirementsTestElement)), Facet: facet.Bytes()}}},
		{"realm", &realmpb.Requirement{Permission: proto.String("p"), Scope: &realmpb.Scope{Kind: realmpb.ScopeKind_REALM.Enum(), Realm: proto.String("realm-a")}}},
		{"authority", &realmpb.Requirement{Permission: proto.String("p"), Scope: &realmpb.Scope{Kind: realmpb.ScopeKind_AUTHORITY.Enum(), Authority: proto.String(string(requirementsTestOwner))}}},
		{"anchor closure", &realmpb.Requirement{Permission: proto.String("p"), Scope: &realmpb.Scope{Kind: realmpb.ScopeKind_ANCHOR_CLOSURE.Enum(), Authority: proto.String(string(requirementsTestOwner)), Element: proto.String(string(requirementsTestElement))}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			requirement, err := Requirement(tc.in)
			if err != nil {
				t.Fatal(err)
			}
			if !requirement.Valid() || requirement.Permission == "" {
				t.Fatalf("requirement = %+v", requirement)
			}
		})
	}
}

func TestRequirementFromWireFailsClosed(t *testing.T) {
	cases := []struct {
		name string
		in   *realmpb.Requirement
	}{
		{"nil requirement", nil},
		{"nil scope", &realmpb.Requirement{Permission: proto.String("p")}},
		{"empty permission", &realmpb.Requirement{Permission: proto.String(""), Scope: &realmpb.Scope{Kind: realmpb.ScopeKind_CORE_ELEMENT.Enum(), Authority: proto.String(string(requirementsTestOwner)), Element: proto.String(string(requirementsTestElement))}}},
		{"oversized permission", &realmpb.Requirement{Permission: proto.String(tooLongPermission()), Scope: &realmpb.Scope{Kind: realmpb.ScopeKind_CORE_ELEMENT.Enum(), Authority: proto.String(string(requirementsTestOwner)), Element: proto.String(string(requirementsTestElement))}}},
		{"undeclared kind", &realmpb.Requirement{Permission: proto.String("p"), Scope: &realmpb.Scope{Kind: realmpb.ScopeKind(0).Enum(), Authority: proto.String(string(requirementsTestOwner)), Element: proto.String(string(requirementsTestElement))}}},
		{"unknown kind", &realmpb.Requirement{Permission: proto.String("p"), Scope: &realmpb.Scope{Kind: realmpb.ScopeKind(99).Enum(), Authority: proto.String(string(requirementsTestOwner)), Element: proto.String(string(requirementsTestElement))}}},
		{"missing authority", &realmpb.Requirement{Permission: proto.String("p"), Scope: &realmpb.Scope{Kind: realmpb.ScopeKind_CORE_ELEMENT.Enum(), Element: proto.String(string(requirementsTestElement))}}},
		{"missing element", &realmpb.Requirement{Permission: proto.String("p"), Scope: &realmpb.Scope{Kind: realmpb.ScopeKind_CORE_ELEMENT.Enum(), Authority: proto.String(string(requirementsTestOwner))}}},
		{"invalid authority", &realmpb.Requirement{Permission: proto.String("p"), Scope: &realmpb.Scope{Kind: realmpb.ScopeKind_CORE_ELEMENT.Enum(), Authority: proto.String("not-an-uuid"), Element: proto.String(string(requirementsTestElement))}}},
		{"core element with realm set", &realmpb.Requirement{Permission: proto.String("p"), Scope: &realmpb.Scope{Kind: realmpb.ScopeKind_CORE_ELEMENT.Enum(), Authority: proto.String(string(requirementsTestOwner)), Element: proto.String(string(requirementsTestElement)), Realm: proto.String("realm-a")}}},
		{"core element with facet set", &realmpb.Requirement{Permission: proto.String("p"), Scope: &realmpb.Scope{Kind: realmpb.ScopeKind_CORE_ELEMENT.Enum(), Authority: proto.String(string(requirementsTestOwner)), Element: proto.String(string(requirementsTestElement)), Facet: make([]byte, 32)}}},
		{"realm with authority set", &realmpb.Requirement{Permission: proto.String("p"), Scope: &realmpb.Scope{Kind: realmpb.ScopeKind_REALM.Enum(), Authority: proto.String(string(requirementsTestOwner)), Realm: proto.String("realm-a")}}},
		{"realm with element set", &realmpb.Requirement{Permission: proto.String("p"), Scope: &realmpb.Scope{Kind: realmpb.ScopeKind_REALM.Enum(), Element: proto.String(string(requirementsTestElement)), Realm: proto.String("realm-a")}}},
		{"authority with element set", &realmpb.Requirement{Permission: proto.String("p"), Scope: &realmpb.Scope{Kind: realmpb.ScopeKind_AUTHORITY.Enum(), Authority: proto.String(string(requirementsTestOwner)), Element: proto.String(string(requirementsTestElement))}}},
		{"realm without realm", &realmpb.Requirement{Permission: proto.String("p"), Scope: &realmpb.Scope{Kind: realmpb.ScopeKind_REALM.Enum()}}},
		{"invalid facet bytes", &realmpb.Requirement{Permission: proto.String("p"), Scope: &realmpb.Scope{Kind: realmpb.ScopeKind_FACET_KEY.Enum(), Authority: proto.String(string(requirementsTestOwner)), Element: proto.String(string(requirementsTestElement)), Facet: []byte{1, 2, 3}}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if requirement, err := Requirement(tc.in); err == nil {
				t.Fatalf("accepted %+v", requirement)
			}
		})
	}
}

func tooLongPermission() string {
	out := make([]byte, 1025)
	for i := range out {
		out[i] = 'p'
	}
	return string(out)
}

func TestRequirementsFromWireNormalizesAndBounds(t *testing.T) {
	if _, err := Requirements(nil, 8); err == nil {
		t.Fatal("empty set accepted")
	}
	list := make([]*realmpb.Requirement, 0, 9)
	for i := 0; i < 9; i++ {
		list = append(list, coreElementRequirement("p"))
	}
	if _, err := Requirements(list, 8); err == nil {
		t.Fatal("oversized set accepted")
	}
	// The fixed protocol ceiling is 64 even when a configuration allows more.
	wide := make([]*realmpb.Requirement, 0, MaxRequirements+1)
	for i := 0; i <= MaxRequirements; i++ {
		wide = append(wide, coreElementRequirement("p"))
	}
	if _, err := Requirements(wide, 128); err == nil {
		t.Fatal("protocol ceiling ignored")
	}
	mixed := []*realmpb.Requirement{
		coreElementRequirement("first"),
		coreElementRequirement("second"),
		coreElementRequirement("first"),
	}
	normalized, err := Requirements(mixed, 8)
	if err != nil {
		t.Fatal(err)
	}
	if len(normalized) != 2 || normalized[0].Permission != "first" || normalized[1].Permission != "second" {
		t.Fatalf("deduplicated order = %+v", normalized)
	}
}

func TestSameRequirementsIgnoresOrder(t *testing.T) {
	first, err := Requirements([]*realmpb.Requirement{coreElementRequirement("a"), coreElementRequirement("b")}, 8)
	if err != nil {
		t.Fatal(err)
	}
	second, err := Requirements([]*realmpb.Requirement{coreElementRequirement("b"), coreElementRequirement("a")}, 8)
	if err != nil {
		t.Fatal(err)
	}
	if !SameRequirements(first, second) {
		t.Fatal("same set in different order reported unequal")
	}
	second, err = Requirements([]*realmpb.Requirement{coreElementRequirement("a"), coreElementRequirement("c")}, 8)
	if err != nil {
		t.Fatal(err)
	}
	if SameRequirements(first, second) {
		t.Fatal("different sets reported equal")
	}
	if SameRequirements(first, nil) {
		t.Fatal("different lengths reported equal")
	}
}
