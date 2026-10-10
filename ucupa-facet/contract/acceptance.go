package contract

import "ucupa/shared/values"

// PublicAcceptanceRef selects a proposal at its original Core. The Facet
// authority is the receiving owner, not a field that can redirect the request.
type PublicAcceptanceRef struct {
	Core     values.FactAuthorityRef
	Element  values.ElementID
	Proposal values.PublicToken
}

// PublicAcceptanceInspection does not expose private proposal revisions or
// either owner's read cut. Found means retained acceptance, not current ACTIVE.
type PublicAcceptanceInspection struct {
	Ref             PublicAcceptanceRef
	Found           bool
	CoreNextHop     values.NextHop
	ContactRevision values.PublicToken
}

type PublicAcceptanceOutcome struct {
	Status     MutationStatus
	Inspection PublicAcceptanceInspection
}
