package contract

import (
	"ucupa/shared/values"
)

const (
	PermissionFacetReplacementBegin   values.PermissionID = "ucupa.core.element.facet-replacement-begin"
	PermissionFacetReplacementCancel  values.PermissionID = "ucupa.core.element.facet-replacement-cancel"
	PermissionFacetReplacementRecover values.PermissionID = "ucupa.core.element.facet-replacement-recover"
	PermissionFacetReplacementRead    values.PermissionID = "ucupa.core.element.facet-replacement-read"
)

// PublicFacetReplacementCommand contains only caller-visible comparisons. The
// element's current content owner is disclosed from the retained record, not
// chosen by the caller; Expected is the current attachment-revision comparison.
type PublicFacetReplacementCommand struct {
	Element       values.ElementID
	Expected      values.PublicToken
	Target        values.FactAuthorityRef
	TargetAddress values.NextHop
	SourceAddress values.NextHop
}

type FacetReplacementProgress uint8

const (
	FacetReplacementReserved FacetReplacementProgress = iota + 1
	FacetReplacementPermitted
	FacetReplacementFenced
	FacetReplacementAccepted
	FacetReplacementCommitted
	FacetReplacementInstalled
	FacetReplacementSettled
	FacetReplacementCancelled
)

// PublicFacetReplacementStatus is the separately authorized control
// projection. It discloses no manifest bytes, no private cuts and no content
// facts; Revision is the stable public comparison of the retained reservation.
type PublicFacetReplacementStatus struct {
	Authority values.FactAuthorityRef
	Operation OperationID
	Element   values.ElementID
	Source    values.FactAuthorityRef
	Target    values.FactAuthorityRef
	Progress  FacetReplacementProgress
	Refused   bool
	Revision  values.PublicToken
}

// FacetReplacementRecord is a trusted service-port result for the Facet-side
// replacement read, never a client read receipt. The caller is independently
// established as the fixed Facet service. It discloses exactly the control
// milestones and immutable fence facts a peer step needs to verify its own
// premise against the controlling Core.
type FacetReplacementRecord struct {
	Authority      values.FactAuthorityRef
	Element        values.ElementID
	Operation      OperationID
	Source         values.FactAuthorityRef
	Target         values.FactAuthorityRef
	Proposal       values.AttachmentRevision
	FinalCut       uint64
	ManifestDigest [32]byte
	Permitted      bool
	Fenced         bool
	Accepted       bool
	Committed      bool
	Settled        bool
	Cancelled      bool
}
