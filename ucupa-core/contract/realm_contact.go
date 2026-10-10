package contract

import (
	"context"

	"ucupa/shared/values"
)

const (
	PermissionRealmContactRead   values.PermissionID = "ucupa.core.realm-contact-read"
	PermissionRealmContactUpdate values.PermissionID = "ucupa.core.realm-contact-update"
)

// RealmInitialization is trusted creation input, never a caller-selected
// authorization source. The new Core identity is generated independently and
// bound to this one Realm contact in its original owner Store.
type RealmInitialization struct {
	Realm   values.RealmID
	NextHop values.NextHop
}

// Valid accepts either declared address profile. The assembled Realm
// connections port, not this shape check, decides which profile one running
// Core can actually verify and use.
func (initial RealmInitialization) Valid() bool {
	return initial.Realm != "" && initial.NextHop.Valid()
}

// RealmContactRevision is an opaque equality value issued by the Core owner.
// It is unrelated to Element, mount, Facet or writer revisions.
type RealmContactRevision [32]byte

type RealmAttachment struct {
	Owner    values.FactAuthorityRef
	Realm    values.RealmID
	NextHop  values.NextHop
	Revision RealmContactRevision
}

// RealmContactUpdate is one exact address-maintenance request. The caller
// identity is supplied separately by the trusted admission boundary.
type RealmContactUpdate struct {
	OperationID OperationID
	Owner       values.FactAuthorityRef
	Realm       values.RealmID
	Expected    RealmContactRevision
	NextHop     values.NextHop
}

type RealmContactDecision uint8

const (
	RealmContactCommitted RealmContactDecision = iota + 1
	RealmContactUnchanged
	RealmContactRejected
)

// RealmContactResult is the retained decision from that operation, not a
// promise that the address remains current when the receipt is later read.
type RealmContactResult struct {
	OperationID OperationID
	Decision    RealmContactDecision
	Contact     RealmAttachment
}

// RealmConnections is a trusted local contact port. Every call supplies the
// owner's persisted address and expected Realm; the implementation must not
// resolve a Realm from the caller's identity or silently choose a fallback.
// It does not own the contact record or any authorization facts. The
// context-bearing methods carry the original request's cancellation: under a
// network contact they are real remote waits.
type RealmConnections interface {
	Check(context.Context, values.NextHop, values.RealmID) error
	AuthorizePrincipal(values.NextHop, values.RealmID, values.PrincipalRef, ...values.Requirement) values.AuthorizationDecision
	AuthorizeView(context.Context, values.NextHop, values.RealmID, values.CallerContext, ...values.Requirement) (values.AuthorizationDecision, values.AuthorizationView)
	ValidateView(context.Context, values.NextHop, values.RealmID, values.CallerContext, values.AuthorizationView, ...values.Requirement) values.AuthorizationDecision
	CaptureDenialView(context.Context, values.NextHop, values.RealmID, values.CallerContext, values.Requirement) (values.AuthorizationDecision, values.AuthorizationDenialView)
	ValidateDenialView(context.Context, values.NextHop, values.RealmID, values.CallerContext, values.AuthorizationDenialView, values.Requirement) values.AuthorizationDecision
}

// RealmEvidenceReleaser is the resource-only face of an allocating network
// profile. Local kernel views allocate no remote references and need no face.
// Cleanup is bounded, outside fact locks, and never changes an authorization
// decision. A failed release leaves only the existing finite TTL fallback.
type RealmEvidenceReleaser interface {
	ReleaseRealmEvidence(context.Context, values.NextHop, values.RealmID, [32]byte)
}
