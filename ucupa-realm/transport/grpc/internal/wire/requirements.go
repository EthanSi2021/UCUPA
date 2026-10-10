// Package wire converts the declared Realm protocol and validates its shape.
// It owns no grants, credentials, admission state or authorization decisions.
package wire

import (
	"errors"

	"ucupa/shared/values"
	realmpb "ucupa/ucupa-realm/bindings/go/protobuf"
)

// maxWireRequirements is the fixed protocol ceiling for one requirement set;
// a service configuration may tighten it but never exceed it.
const MaxRequirements = 64

// Decision preserves the four-state kernel decision, never a bool projection.
func Decision(decision values.AuthorizationDecision) realmpb.Decision {
	switch decision {
	case values.AuthorizationValid:
		return realmpb.Decision_VALID
	case values.AuthorizationChanged:
		return realmpb.Decision_CHANGED
	case values.AuthorizationDenied:
		return realmpb.Decision_DENIED
	default:
		return realmpb.Decision_UNAVAILABLE
	}
}

func DecisionResponse(decision values.AuthorizationDecision) *realmpb.Decision {
	mapped := Decision(decision)
	return &mapped
}

// requirementFromWire converts one declared wire requirement to the exact
// kernel value. Undeclared kinds or inconsistent scope fields fail closed.
func Requirement(in *realmpb.Requirement) (values.Requirement, error) {
	if in == nil || in.GetScope() == nil || in.GetPermission() == "" || len(in.GetPermission()) > 1024 {
		return values.Requirement{}, errors.New("invalid requirement")
	}
	scope := in.GetScope()
	authority := values.FactAuthorityRef(scope.GetAuthority())
	element := values.ElementID(scope.GetElement())
	var target values.OperationScope
	// Each kind requires exactly its own scope fields; any field set outside
	// the kind's shape is a mismatch and fails closed before kernel work.
	exact := func(authoritySet, elementSet, facetSet, realmSet bool) bool {
		return authoritySet == (scope.GetAuthority() != "") &&
			elementSet == (scope.GetElement() != "") &&
			facetSet == (len(scope.GetFacet()) != 0) &&
			realmSet == (scope.GetRealm() != "")
	}
	switch scope.GetKind() {
	case realmpb.ScopeKind_CORE_ELEMENT:
		if !exact(true, true, false, false) {
			return values.Requirement{}, errors.New("invalid requirement")
		}
		target = values.CoreElementScope(authority, element)
	case realmpb.ScopeKind_FACET_ELEMENT:
		if !exact(true, true, false, false) {
			return values.Requirement{}, errors.New("invalid requirement")
		}
		target = values.FacetElementScope(authority, element)
	case realmpb.ScopeKind_FACET_KEY:
		if !exact(true, true, true, false) {
			return values.Requirement{}, errors.New("invalid requirement")
		}
		facet, err := values.NewFacetTypeRef(scope.GetFacet())
		if err != nil {
			return values.Requirement{}, errors.New("invalid requirement")
		}
		target = values.FacetKeyScope(authority, element, facet)
	case realmpb.ScopeKind_REALM:
		if !exact(false, false, false, true) {
			return values.Requirement{}, errors.New("invalid requirement")
		}
		target = values.RealmScope(values.RealmID(scope.GetRealm()))
	case realmpb.ScopeKind_AUTHORITY:
		if !exact(true, false, false, false) {
			return values.Requirement{}, errors.New("invalid requirement")
		}
		target = values.AuthorityScope(authority)
	case realmpb.ScopeKind_ANCHOR_CLOSURE:
		if !exact(true, true, false, false) {
			return values.Requirement{}, errors.New("invalid requirement")
		}
		target = values.AnchorClosureScope(authority, element)
	default:
		return values.Requirement{}, errors.New("invalid requirement")
	}
	requirement := values.Requirement{Permission: values.PermissionID(in.GetPermission()), Scope: target}
	if !requirement.Valid() {
		return values.Requirement{}, errors.New("invalid requirement")
	}
	return requirement, nil
}

// requirementsFromWire validates the bounded wire set and normalizes it to
// the deduplicated first-occurrence order used for exact-set comparisons.
func Requirements(list []*realmpb.Requirement, maximum int) ([]values.Requirement, error) {
	if len(list) == 0 || len(list) > maximum || len(list) > MaxRequirements {
		return nil, errors.New("invalid requirement set")
	}
	seen := make(map[values.Requirement]bool, len(list))
	out := make([]values.Requirement, 0, len(list))
	for _, item := range list {
		requirement, err := Requirement(item)
		if err != nil {
			return nil, err
		}
		if seen[requirement] {
			continue
		}
		seen[requirement] = true
		out = append(out, requirement)
	}
	return out, nil
}

// sameRequirements reports exact set equality without order sensitivity.
func SameRequirements(a, b []values.Requirement) bool {
	if len(a) != len(b) {
		return false
	}
	counts := make(map[values.Requirement]int, len(a))
	for _, requirement := range a {
		counts[requirement]++
	}
	for _, requirement := range b {
		if counts[requirement] == 0 {
			return false
		}
		counts[requirement]--
	}
	return true
}
