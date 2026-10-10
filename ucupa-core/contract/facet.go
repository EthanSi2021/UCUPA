package contract

import (
	"context"
	"errors"

	"ucupa/shared/values"
)

const (
	PermissionFacetPropose        values.PermissionID = "ucupa.core.element.facet-propose"
	PermissionFacetCancel         values.PermissionID = "ucupa.core.element.facet-cancel"
	PermissionFacetActivate       values.PermissionID = "ucupa.core.element.facet-activate"
	PermissionFacetDetach         values.PermissionID = "ucupa.core.element.facet-detach"
	PermissionFacetContactUpdate  values.PermissionID = "ucupa.core.element.facet-contact-update"
	PermissionFacetProposalRead   values.PermissionID = "ucupa.core.element.facet-proposal-read"
	PermissionFacetAttachmentRead values.PermissionID = "ucupa.core.element.facet-attachment-read"
	PermissionFacetRelease        values.PermissionID = "ucupa.core.element.facet-release"
	PermissionObserveAttachment   values.PermissionID = "ucupa.core.element.facet-attachment-observe"
)

var (
	ErrAttachmentRevisionConflict   = errors.New("Core attachment revision conflict")
	ErrAttachmentStateConflict      = errors.New("Core attachment state conflict")
	ErrProposalNotCurrent           = errors.New("Core attachment proposal is not current")
	ErrAcceptanceNotFound           = errors.New("Facet acceptance not found")
	ErrDispositionNotFound          = errors.New("Facet final disposition not found")
	ErrAttachmentChanged            = errors.New("Core attachment relationship changed")
	ErrFacetContactRevisionConflict = errors.New("Core Facet contact revision conflict")
	ErrAttachmentIndeterminate      = errors.New("Core attachment commit outcome indeterminate")
)

type FacetAttachmentState uint8

const (
	FacetNone FacetAttachmentState = iota + 1
	FacetActive
	FacetDetached
	FacetRetired
)

// FacetAttachment discloses no pending proposal and no Facet payload existence.
// Key is current for ACTIVE, former for DETACHED and RETIRED, and zero for NONE.
type FacetAttachment struct {
	State           FacetAttachmentState
	Revision        values.AttachmentRevision
	Key             values.FacetAcceptanceKey
	NextHop         values.NextHop
	NextHopRevision FacetContactRevision
	// OriginalProposal retains the original Core-issued public comparison for
	// a transferred key. It grants no access and is not the new relationship CAS.
	// Local, never-transferred keys can derive it from their own Core signer.
	OriginalProposal values.PublicToken
}

type FacetProposalRead struct {
	Authority       values.FactAuthorityRef
	Element         values.ElementID
	State           ExistenceState
	Attachment      FacetAttachment
	Pending         values.FacetAcceptanceKey
	NextHop         values.NextHop
	NextHopRevision FacetContactRevision
	Cut             CoreReadCut
}

// FacetAttachmentRead is the separately authorized minimum disclosure for
// Facet's current-Core check. It never includes a spatial ElementRecord.
type FacetAttachmentRead struct {
	Authority  values.FactAuthorityRef
	Element    values.ElementID
	State      ExistenceState
	Attachment FacetAttachment
	Cut        CoreReadCut
}

// FacetSuccessorRead is the source Core's durable successor disclosure for
// one departed element. State NOT_FOUND is a proven exact absence: the element
// never departed under this owner, or no unique successor decision exists yet.
// It discloses no manifest content and grants no access to the successor.
type FacetSuccessorRead struct {
	Authority values.FactAuthorityRef
	Element   values.ElementID
	State     ExistenceState
	Operation OperationID
	Source    values.FactAuthorityRef
	Target    values.FactAuthorityRef
	FinalCut  uint64
	Settled   bool
}

type FacetAcceptanceEvidence struct {
	Key        values.FacetAcceptanceKey
	AcceptedAt values.FacetReadCut
}

// FacetAcceptanceSource is consumer-owned. The adapter must connect only at
// nextHop, verify the expected Facet identity and independently authorize its
// service caller. A nil error and zero evidence mean proven exact absence.
type FacetAcceptanceSource interface {
	ReadFacetAcceptance(context.Context, values.NextHop, values.FacetAcceptanceKey) (FacetAcceptanceEvidence, error)
}

// FacetDispositionRead is the narrow projection of one content owner's durable
// final disposition for a retired element namespace. Found=false is a proven
// exact absence at the reachable Facet: the element was never retired there.
type FacetDispositionRead struct {
	Found   bool
	Facet   values.FactAuthorityRef
	Element values.ElementID
	Key     values.FacetAcceptanceKey
	Cut     values.FacetReadCut
}

// FacetDispositionSource is consumer-owned. The adapter must connect only at
// nextHop, verify the addressed service is exactly the expected Facet authority
// and independently authorize its service caller. A nil error and zero read
// mean proven exact absence, never an unknown disposition.
type FacetDispositionSource interface {
	ReadFacetDisposition(context.Context, values.NextHop, values.FactAuthorityRef, values.ElementID) (FacetDispositionRead, error)
}

type FacetAttachmentResult struct {
	Status          MutationStatus
	Attachment      FacetAttachment
	Pending         values.FacetAcceptanceKey
	NextHop         values.NextHop
	NextHopRevision FacetContactRevision
	OutcomeCut      CoreReadCut
}

// PublicFacetCommand contains only caller-visible comparisons. Kind is one of
// propose, cancel, activate, detach, release, contact; irrelevant fields are
// forbidden.
type PublicFacetCommand struct {
	Kind     string
	Element  values.ElementID
	Expected values.PublicToken
	Target   values.FactAuthorityRef
	NextHop  values.NextHop
}

// PublicFacetInspection is the separately authorized relationship-control
// projection. It never exports private revisions, cuts or service evidence.
type PublicFacetInspection struct {
	Element         values.ElementID
	State           FacetAttachmentState
	Revision        values.PublicToken
	Target          values.FactAuthorityRef
	KeyCore         values.FactAuthorityRef
	Proposal        values.PublicToken
	PendingTarget   values.FactAuthorityRef
	PendingProposal values.PublicToken
	NextHop         values.NextHop
	ContactRevision values.PublicToken
}

// AttachmentControlRead is a trusted service-port result, never a client read
// receipt. The caller is independently established as the fixed Facet service.
// Preparation is private to that service; it uses the existing bounded stream.
type AttachmentControlRead struct {
	Read        FacetAttachmentRead
	Proposal    values.PublicToken
	Preparation values.PublicToken
}
