package contract

import (
	"context"
	"ucupa/shared/values"
)

// Authorizer is owned by the Facet owner. Every method reports the decision of
// the Facet's one currently attached Realm instead of collapsing unavailable
// evidence into a denial. The view methods carry the original request's
// context: under a network Realm contact they are real remote waits, so they
// must run outside any Facet lock and honor that context's cancellation.
type Authorizer interface {
	AuthorizePrincipal(values.PrincipalRef, ...values.Requirement) values.AuthorizationDecision
	AuthorizeView(context.Context, values.CallerContext, ...values.Requirement) (values.AuthorizationDecision, values.AuthorizationView)
	ValidateView(context.Context, values.CallerContext, values.AuthorizationView, ...values.Requirement) values.AuthorizationDecision
	CaptureDenialView(context.Context, values.CallerContext, values.Requirement) (values.AuthorizationDecision, values.AuthorizationDenialView)
	ValidateDenialView(context.Context, values.CallerContext, values.AuthorizationDenialView, values.Requirement) values.AuthorizationDecision
}

// CoreState is a narrow projection of a current, authorized Core read.
type CoreState uint8

const (
	CoreMissing CoreState = iota + 1
	CoreNone
	CoreActive
	CoreDetached
)

type CoreEvidence struct {
	Authority          values.FactAuthorityRef
	Element            values.ElementID
	State              CoreState
	AttachmentRevision values.AttachmentRevision
	Active             values.FacetAcceptanceKey
	Pending            values.FacetAcceptanceKey
	Cut                values.CoreCutEvidence
}

// CoreSource must use only the supplied address, establish the expected Core,
// and use a fixed service identity independent of the requesting caller.
type CoreSource interface {
	ReadProposal(context.Context, values.NextHop, values.FacetAcceptanceKey) (CoreEvidence, error)
	ReadAttachment(context.Context, values.NextHop, values.FacetAcceptanceKey) (CoreEvidence, error)
	ReadSuccessor(context.Context, values.NextHop, values.FactAuthorityRef, values.ElementID) (CoreSuccessorRead, error)
}

// CoreSuccessorRead is the narrow projection of one previous Core's durable
// successor statement for a departed element. Found=false is a proven exact
// absence at the reachable Core: the element never departed under its control,
// or no unique successor decision exists. It never distinguishes an ordinary
// deletion from an undecided transfer, and both fail the lineage check.
type CoreSuccessorRead struct {
	Found     bool
	Authority values.FactAuthorityRef
	Element   values.ElementID
	Operation OperationID
	Source    values.FactAuthorityRef
	Target    values.FactAuthorityRef
	FinalCut  uint64
	Settled   bool
}

// CoreReplacementRead is a narrow projection of one durable facet-replacement
// control record kept by the controlling Core of an element. The FinalCut and
// ManifestDigest are fixed by the source fence and stay stable afterwards.
type CoreReplacementRead struct {
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

// ReplacementSource is the CoreSource capability required by namespace
// replacement steps. It must use only the supplied address, establish the
// expected Core and disclose only the exact named control record.
type ReplacementSource interface {
	ReadReplacement(context.Context, values.NextHop, values.FactAuthorityRef, values.ElementID, OperationID) (CoreReplacementRead, error)
}

type CoreControlRead struct {
	Evidence    CoreEvidence
	Proposal    values.PublicToken
	Preparation values.PublicToken
}

type CoreProposalRead struct {
	Key      values.FacetAcceptanceKey
	Evidence CoreEvidence
	Proposal values.PublicToken
}
