package contract

import "ucupa/shared/values"

const (
	PermissionPut               values.PermissionID = "ucupa.facet.put"
	PermissionGet               values.PermissionID = "ucupa.facet.get"
	PermissionDelete            values.PermissionID = "ucupa.facet.delete"
	PermissionAccept            values.PermissionID = "ucupa.facet.element.accept"
	PermissionReadAcceptance    values.PermissionID = "ucupa.facet.element.acceptance-read"
	PermissionListTypes         values.PermissionID = "ucupa.facet.element.list-types"
	PermissionCoreContactUpdate values.PermissionID = "ucupa.facet.element.core-contact-update"
	PermissionNamespaceExport   values.PermissionID = "ucupa.facet.element.namespace-export"
	PermissionNamespaceImport   values.PermissionID = "ucupa.facet.element.namespace-import"
	PermissionRetireContent     values.PermissionID = "ucupa.facet.element.retire"
)

type OperationID string

type MutationStatus uint8

const (
	Committed MutationStatus = iota + 1
	Unchanged
	Rejected
)

type Acceptance struct {
	Key        values.FacetAcceptanceKey
	AcceptedAt values.FacetReadCut
}
type AcceptanceRead struct {
	Found           bool
	Acceptance      Acceptance
	CoreNextHop     values.NextHop
	NextHopRevision ContactRevision
	Cut             values.FacetReadCut
}
type MutationResult struct {
	Status          MutationStatus
	Cut             values.FacetReadCut
	Acceptance      Acceptance
	CoreNextHop     values.NextHop
	NextHopRevision ContactRevision
	Revision        Revision
	Core            CoreEvidence
}

// DispositionRead is the durable final disposition of one element's retired
// namespace in this Facet: the retired acceptance key and the cut that
// recorded it. Found=false is a proven exact absence at this Facet: the
// element was never retired here.
type DispositionRead struct {
	Found bool
	Key   values.FacetAcceptanceKey
	Cut   values.FacetReadCut
}
type ContentRead struct {
	Payload    []byte
	Revision   Revision
	Types      []values.FacetTypeRef
	Cut        values.FacetReadCut
	Acceptance Acceptance
	Core       CoreEvidence
}

const PermissionObserveElement values.PermissionID = "ucupa.facet.element.observe"
const PermissionObserveKey values.PermissionID = "ucupa.facet.observe"
