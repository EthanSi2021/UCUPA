package fact

import (
	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	elementfact "ucupa/ucupa-core/internal/element/fact"
	facetfact "ucupa/ucupa-core/internal/facet/fact"
	mountfact "ucupa/ucupa-core/internal/mount/fact"
	"ucupa/ucupa-core/internal/token"
)

// CarrierFence is source withdrawal, not the parent's binding decision. It
// retains the exact permit and final content digest without inventing a local
// Element for the parent's mount. Authentication belongs to the operation.
type CarrierFence struct {
	Permit         CarrierControl
	FinalCut       contract.CoreReadCut
	ManifestDigest [32]byte
	Revision       uint64
}

// Acceptance retains a complete private candidate at the designated new
// carrier. It is not the parent's binding decision or permission to serve.
type CarrierAcceptance struct {
	Fence    CarrierFence
	At       contract.CoreReadCut
	Revision uint64
}

func (a CarrierAcceptance) Valid() bool {
	return a.Fence.Valid() && token.Valid(a.At) && a.Revision != 0 &&
		token.Scope(a.At) != token.Scope(a.Fence.FinalCut) &&
		token.Scope(a.At) != token.Scope(a.Fence.Permit.PermitAt)
}

func (f CarrierFence) Valid() bool {
	return f.Permit.Valid() && f.Permit.PermitAt != (contract.CoreReadCut{}) &&
		f.Permit.CancelledAt == (contract.CoreReadCut{}) && token.Valid(f.FinalCut) &&
		token.Scope(f.FinalCut) != token.Scope(f.Permit.PermitAt) && f.Revision > 0 && f.ManifestDigest != ([32]byte{})
}

// SelectCarrier captures the entire owner-local content below one external
// parent, including an actually empty closure. Graph validation proves every
// live member belongs there; no root or parent-side facts are fabricated.
// The commit owner separately checks the current matching incoming relation.
func SelectCarrier(owner values.FactAuthorityRef, cut contract.CoreReadCut, revision uint64,
	elements elementfact.View, facets facetfact.View, mounts mountfact.View, external elementfact.ExternalParent, maxRecords int) (Slice, error) {
	if !owner.Valid() || !token.Valid(cut) || !external.Present || !external.Admitted || !external.ID.Valid() || maxRecords <= 0 {
		return Slice{}, contract.ErrInvalid
	}
	counts := elements.Counts()
	if counts.Anchors+counts.Entities > maxRecords {
		return Slice{}, values.PublicResourceExhausted
	}
	if err := elements.Validate(token.Scope(cut), revision, external); err != nil {
		return Slice{}, contract.ErrConflict
	}
	ids := make([]values.ElementID, 0, counts.Anchors+counts.Entities)
	for id := range elements.Anchors() {
		ids = append(ids, id)
	}
	for id := range elements.Entities() {
		ids = append(ids, id)
	}
	elementfact.SortIDs(ids)
	s := Slice{source: owner, cut: cut, boundary: external.ID, external: external}
	if err := s.selectRecords(ids, elements, facets, mounts); err != nil {
		return Slice{}, err
	}
	return s, nil
}
