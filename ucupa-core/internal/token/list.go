package token

import "ucupa/shared/values"

// ListContinuation binds a ListDirect page position to its original Core,
// parent, and fact cut. Its fields deliberately remain private.
type ListContinuation struct {
	scope  [32]byte
	parent values.ElementID
	cut    CoreReadCut
	last   values.ElementID
}

func ValidListContinuation(v ListContinuation) bool {
	return v.scope != ([32]byte{}) && v.parent.Valid() && v.last.Valid() &&
		Valid(v.cut) && Scope(v.cut) == v.scope
}
func ListContinuationFromParts(scope [32]byte, parent values.ElementID, cut CoreReadCut, last values.ElementID) ListContinuation {
	return ListContinuation{scope: scope, parent: parent, cut: cut, last: last}
}
func ListScope(v ListContinuation) [32]byte          { return v.scope }
func ListParent(v ListContinuation) values.ElementID { return v.parent }
func ListCut(v ListContinuation) CoreReadCut         { return v.cut }
func ListLast(v ListContinuation) values.ElementID   { return v.last }
