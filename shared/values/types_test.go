package values

import (
	"crypto/sha256"
	"testing"
)

func TestElementIDProfile(t *testing.T) {
	valid := ElementID("01234567-89ab-4cde-8f01-23456789abcd")
	if !valid.Valid() {
		t.Fatalf("canonical UUIDv4 rejected: %q", valid)
	}

	invalid := []struct {
		name string
		id   ElementID
	}{
		{name: "wrong length", id: "01234567-89ab-4cde-8f01-23456789abc"},
		{name: "uppercase", id: "01234567-89AB-4cde-8f01-23456789abcd"},
		{name: "dash", id: "0123456-789ab-4cde-8f01-23456789abcd"},
		{name: "hex", id: "01234567-89ab-4cde-8f01-23456789abcg"},
		{name: "version", id: "01234567-89ab-7cde-8f01-23456789abcd"},
		{name: "variant", id: "01234567-89ab-4cde-7f01-23456789abcd"},
		{name: "prefix", id: "urn:uuid:01234567-89ab-4cde-8f01-23456789abcd"},
		{name: "suffix", id: "01234567-89ab-4cde-8f01-23456789abcd!"},
	}
	for _, test := range invalid {
		t.Run(test.name, func(t *testing.T) {
			if test.id.Valid() {
				t.Fatalf("invalid ElementID accepted: %q", test.id)
			}
		})
	}
}

func TestProductIdentityTypesDoNotCollapse(t *testing.T) {
	text := "00000000-0000-4000-8000-000000000001"
	element := ElementID(text)
	cluster := ClusterID(text)
	realm := RealmID(text)

	if string(element) != string(cluster) || string(cluster) != string(realm) {
		t.Fatal("test setup must use identical text")
	}
}

const testCoreRef FactAuthorityRef = "00000000-0000-4000-8000-000000000301"

func testType(text string) FacetTypeRef {
	digest := sha256.Sum256([]byte(text))
	ref, err := NewFacetTypeRef(digest[:])
	if err != nil {
		panic(err)
	}
	return ref
}

func TestFacetKeyIsExactlyElementAndType(t *testing.T) {
	ref := testType("demo.profile")
	key := FacetKey{Element: "00000000-0000-4000-8000-000000000002", Type: ref}
	if key.Element != "00000000-0000-4000-8000-000000000002" || !key.Type.Equal(ref) {
		t.Fatal(key)
	}
}

func TestOperationScopeKindsRemainDistinct(t *testing.T) {
	const id ElementID = "00000000-0000-4000-8000-000000000003"
	scopes := []OperationScope{
		CoreElementScope(testCoreRef, id), FacetElementScope(testCoreRef, id),
		FacetKeyScope(testCoreRef, id, testType("demo.profile")), RealmScope("realm-a"),
	}
	for i, scope := range scopes {
		if !scope.Valid() {
			t.Fatalf("invalid scope: %#v", scope)
		}
		for j, other := range scopes {
			if i != j && scope == other {
				t.Fatal("scope kinds collapsed")
			}
		}
	}
	other := FactAuthorityRef("00000000-0000-4000-8000-000000000302")
	if scopes[0] == CoreElementScope(other, id) || scopes[1] == FacetElementScope(other, id) || scopes[2] == FacetKeyScope(other, id, testType("demo.profile")) {
		t.Fatal("authority isolation lost")
	}
}

func TestOperationScopeTypedTargets(t *testing.T) {
	const id ElementID = "00000000-0000-4000-8000-000000000003"
	ref := testType("demo.profile")
	scopes := []OperationScope{CoreElementScope(testCoreRef, id), FacetElementScope(testCoreRef, id), FacetKeyScope(testCoreRef, id, ref), RealmScope("realm-a")}
	for i, scope := range scopes {
		owner, element, ok := scope.CoreElementTarget()
		if ok != (i == 0) || (ok && (owner != testCoreRef || element != id)) || (!ok && (owner != "" || element != "")) {
			t.Fatal("Core target", scope)
		}
		owner, element, ok = scope.FacetElementTarget()
		if ok != (i == 1) || (ok && (owner != testCoreRef || element != id)) || (!ok && (owner != "" || element != "")) {
			t.Fatal("Facet namespace target", scope)
		}
		owner, key, ok := scope.FacetKeyTarget()
		if ok != (i == 2) || (ok && (owner != testCoreRef || key != (FacetKey{Element: id, Type: ref}))) || (!ok && (owner != "" || key != (FacetKey{}))) {
			t.Fatal("Facet key target", scope)
		}
		realm, ok := scope.RealmTarget()
		if ok != (i == 3) || (ok && realm != "realm-a") || (!ok && realm != "") {
			t.Fatal("Realm target", scope)
		}
	}
}

func TestOperationScopeTypedTargetsRejectInvalidScopes(t *testing.T) {
	const id ElementID = "00000000-0000-4000-8000-000000000003"
	invalid := []OperationScope{{}, {kind: scopeKind(255)}, RealmScope("")}
	for _, kind := range []scopeKind{coreElementScopeKind, facetElementScopeKind, facetKeyScopeKind} {
		valid := OperationScope{kind: kind, owner: testCoreRef, element: id}
		if kind == facetKeyScopeKind {
			valid.facet = testType("x")
		}
		bad := valid
		bad.owner = ""
		invalid = append(invalid, bad)
		bad = valid
		bad.owner = "not-a-ref"
		invalid = append(invalid, bad)
		bad = valid
		bad.element = ""
		invalid = append(invalid, bad)
		bad = valid
		bad.element = "00000000-0000-7000-8000-000000000003"
		invalid = append(invalid, bad)
		bad = valid
		bad.realm = "realm-a"
		invalid = append(invalid, bad)
		bad = valid
		if kind == facetKeyScopeKind {
			bad.facet = FacetTypeRef{}
		} else {
			bad.facet = testType("x")
		}
		invalid = append(invalid, bad)
	}
	invalid = append(invalid, OperationScope{kind: realmScopeKind, realm: "realm-a", owner: testCoreRef}, OperationScope{kind: realmScopeKind, realm: "realm-a", element: id}, OperationScope{kind: realmScopeKind, realm: "realm-a", facet: testType("x")})
	for _, scope := range invalid {
		if scope.Valid() {
			t.Fatal("invalid accepted", scope)
		}
		if o, e, ok := scope.CoreElementTarget(); ok || o != "" || e != "" {
			t.Fatal("invalid Core target")
		}
		if o, e, ok := scope.FacetElementTarget(); ok || o != "" || e != "" {
			t.Fatal("invalid namespace target")
		}
		if o, k, ok := scope.FacetKeyTarget(); ok || o != "" || k != (FacetKey{}) {
			t.Fatal("invalid key target")
		}
		if r, ok := scope.RealmTarget(); ok || r != "" {
			t.Fatal("invalid Realm target")
		}
	}
}

func TestRequirementNeedsExactPermissionAndScope(t *testing.T) {
	const id ElementID = "00000000-0000-4000-8000-000000000004"
	valid := Requirement{
		Permission: PermissionID("ucupa.core.element.read"),
		Scope:      CoreElementScope(testCoreRef, id),
	}
	if !valid.Valid() {
		t.Fatal("exact permission and scope must form a valid requirement")
	}
	if (Requirement{Scope: valid.Scope}).Valid() {
		t.Fatal("zero PermissionID must be invalid")
	}
	if (Requirement{Permission: valid.Permission}).Valid() {
		t.Fatal("zero OperationScope must be invalid")
	}

	coreDelete := PermissionID("ucupa.core.entity.delete")
	facetDelete := PermissionID("ucupa.facet.delete")
	if coreDelete == facetDelete {
		t.Fatal("owner-qualified permission identities must remain distinct")
	}
}
