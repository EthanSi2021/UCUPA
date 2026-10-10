package request

import "ucupa/shared/values"

// Access is the existing public selector result. It carries only the Core,
// Element, and proposal values used by Facet callers; it is not an
// authorization grant and contains no Facet state or resolved acceptance key.
type Access struct {
	Core     values.FactAuthorityRef
	Element  values.ElementID
	Proposal values.PublicToken
}

// ParseAccess validates a public access selector for the receiving owner. The
// owner check intentionally returns PublicUnavailable, matching the existing
// Facet boundary. Token.Valid supplies syntax only; it never authenticates the
// selector or proves an accepted relation.
func ParseAccess(owner values.FactAuthorityRef, raw any) (Access, error) {
	o, err := Object(raw, "key", "nextHop")
	if err != nil {
		return Access{}, err
	}
	hop, ok := o["nextHop"].(string)
	if !ok || !values.NextHop(hop).Valid() {
		return Access{}, values.PublicInvalid
	}
	k, err := Object(o["key"], "core", "facet", "element", "proposal")
	if err != nil {
		return Access{}, err
	}
	c, a := k["core"].(string)
	facetOwner, b := k["facet"].(string)
	element, d := k["element"].(string)
	proposal, err := Token(k["proposal"], "attachment-revision")
	if !a || !b || !d || err != nil || !values.FactAuthorityRef(c).Valid() || !values.ElementID(element).Valid() || proposal.Owner != values.FactAuthorityRef(c) {
		return Access{}, values.PublicInvalid
	}
	if values.FactAuthorityRef(facetOwner) != owner {
		return Access{}, values.PublicUnavailable
	}
	return Access{Core: values.FactAuthorityRef(c), Element: values.ElementID(element), Proposal: proposal}, nil
}
