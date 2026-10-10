package contract

import "ucupa/shared/values"

// CarrierCommand addresses the parent's existing mount, not an Element owned
// by Source. The receiving Core supplies Parent; the authenticated request
// supplies the actor. The mount's spatial tuple is retained, not replaced.
type CarrierCommand struct {
	Source, Target                              values.FactAuthorityRef
	Mount                                       values.ElementID
	ExpectedElement, ExpectedMount              values.PublicToken
	ParentAddress, SourceAddress, TargetAddress values.NextHop
}

// CarrierApprovalBinding fixes the public parent operation at each peer.
// Each peer authenticates its own caller; this value contains no identity proof.
type CarrierApprovalBinding struct {
	Parent    values.FactAuthorityRef
	Operation OperationID
	Command   CarrierCommand
}

type CarrierProgress uint8

const (
	CarrierReserved CarrierProgress = iota + 1
	CarrierPermitted
	CarrierCancelled
	CarrierCommitted
	CarrierSettled
)

// CarrierStatus is the parent's durable control projection. PERMITTED means
// dispatch may have occurred, not that the source is fenced. No peer receipt,
// manifest, private cut or permission is conveyed by this status or revision.
type CarrierStatus struct {
	Parent, Source, Target values.FactAuthorityRef
	Operation              OperationID
	Mount                  values.ElementID
	Progress               CarrierProgress
	Revision               values.PublicToken
}
