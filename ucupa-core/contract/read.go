package contract

import "ucupa/shared/values"

type ExistenceState uint8

const (
	ExistencePresent ExistenceState = iota + 1
	ExistenceNotFound
)

type ElementRead struct {
	Authority  values.FactAuthorityRef
	Element    values.ElementID
	Attachment FacetAttachment
	State      ExistenceState
	Record     *ElementRecord
	Cut        CoreReadCut
}

type DirectPage struct {
	Records      []ElementRecord
	Cut          CoreReadCut
	Continuation *ListContinuation
}

type Projection struct {
	Pose            Pose
	SubjectRevision Revision
	Cut             CoreReadCut
}
