package values

// Product identities remain distinct even when their underlying text happens
// to be equal. Paths and deployment locations are deliberately not identities.
type ElementID string
type ClusterID string
type RealmID string
type PrincipalID string
type PermissionID string

type PrincipalRef struct {
	Realm     RealmID
	Principal PrincipalID
}

type FacetKey struct {
	Element ElementID
	Type    FacetTypeRef
}

// OperationScope supports owner-qualified Core/Facet resources and exact Realm
// administration. It cannot carry a path, endpoint, descendant rule, or
// deployment location.
type OperationScope struct {
	kind    scopeKind
	owner   FactAuthorityRef
	element ElementID
	facet   FacetTypeRef
	realm   RealmID
}

type scopeKind uint8

const (
	coreElementScopeKind scopeKind = iota + 1
	facetElementScopeKind
	facetKeyScopeKind
	realmScopeKind
	anchorClosureScopeKind
	authorityScopeKind
)

func CoreElementScope(core FactAuthorityRef, element ElementID) OperationScope {
	return OperationScope{kind: coreElementScopeKind, owner: core, element: element}
}

func FacetElementScope(owner FactAuthorityRef, element ElementID) OperationScope {
	return OperationScope{kind: facetElementScopeKind, owner: owner, element: element}
}

func FacetKeyScope(owner FactAuthorityRef, element ElementID, facet FacetTypeRef) OperationScope {
	return OperationScope{kind: facetKeyScopeKind, owner: owner, element: element, facet: facet}
}

func RealmScope(realm RealmID) OperationScope {
	return OperationScope{kind: realmScopeKind, realm: realm}
}

// AuthorityScope names owner lifecycle control, not access to its elements.
func AuthorityScope(owner FactAuthorityRef) OperationScope {
	return OperationScope{kind: authorityScopeKind, owner: owner}
}

func (scope OperationScope) AuthorityTarget() (FactAuthorityRef, bool) {
	return scope.owner, scope.kind == authorityScopeKind && scope.Valid()
}

// AnchorClosureScope names a grant on one explicit spatial boundary. Realm
// compares this exact target; only the fact owner verifies actual membership.
func AnchorClosureScope(core FactAuthorityRef, anchor ElementID) OperationScope {
	return OperationScope{kind: anchorClosureScopeKind, owner: core, element: anchor}
}

func (scope OperationScope) AnchorClosureTarget() (FactAuthorityRef, ElementID, bool) {
	if scope.kind != anchorClosureScopeKind || !scope.Valid() {
		return "", "", false
	}
	return scope.owner, scope.element, true
}

func (scope OperationScope) Valid() bool {
	switch scope.kind {
	case authorityScopeKind:
		return scope.owner.Valid() && scope.element == "" && scope.facet == (FacetTypeRef{}) && scope.realm == ""
	case coreElementScopeKind, anchorClosureScopeKind:
		return scope.owner.Valid() && scope.element.Valid() && scope.facet == (FacetTypeRef{}) && scope.realm == ""
	case facetElementScopeKind:
		return scope.owner.Valid() && scope.element.Valid() && scope.facet == (FacetTypeRef{}) && scope.realm == ""
	case facetKeyScopeKind:
		return scope.owner.Valid() && scope.element.Valid() && scope.facet.Valid() && scope.realm == ""
	case realmScopeKind:
		return scope.owner == "" && scope.element == "" && scope.facet == (FacetTypeRef{}) && scope.realm != ""
	default:
		return false
	}
}

// CoreElementTarget returns the exact valid Core and Element target, or false otherwise.
func (scope OperationScope) CoreElementTarget() (FactAuthorityRef, ElementID, bool) {
	if scope.kind != coreElementScopeKind || !scope.Valid() {
		return "", "", false
	}
	return scope.owner, scope.element, true
}

// FacetElementTarget returns an exact namespace target, never a descendant rule.
func (scope OperationScope) FacetElementTarget() (FactAuthorityRef, ElementID, bool) {
	if scope.kind != facetElementScopeKind || !scope.Valid() {
		return "", "", false
	}
	return scope.owner, scope.element, true
}

// FacetKeyTarget returns one exact owner-qualified typed record target.
func (scope OperationScope) FacetKeyTarget() (FactAuthorityRef, FacetKey, bool) {
	if scope.kind != facetKeyScopeKind || !scope.Valid() {
		return "", FacetKey{}, false
	}
	return scope.owner, FacetKey{Element: scope.element, Type: scope.facet}, true
}

// RealmTarget returns the exact valid Realm target, or false otherwise.
func (scope OperationScope) RealmTarget() (RealmID, bool) {
	if scope.kind != realmScopeKind || !scope.Valid() {
		return "", false
	}
	return scope.realm, true
}

type Requirement struct {
	Permission PermissionID
	Scope      OperationScope
}

func (value PrincipalRef) Valid() bool {
	return value.Realm != "" && value.Principal != ""
}

func (value Requirement) Valid() bool {
	return value.Permission != "" && value.Scope.Valid()
}
