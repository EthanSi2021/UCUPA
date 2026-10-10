package contract

import (
	"context"

	"ucupa/shared/values"
)

const (
	PermissionRealmContactRead   values.PermissionID = "ucupa.facet.realm-contact-read"
	PermissionRealmContactUpdate values.PermissionID = "ucupa.facet.realm-contact-update"
)

// RealmInitialization is trusted creation input. A Facet owner keeps this
// contact in its own Store, independently of the Core holding an Element.
type RealmInitialization struct {
	Realm   values.RealmID
	NextHop values.NextHop
}

// Valid checks the declared local or network address profile. Verification is
// performed by the configured connection port before construction or reopening.
func (initial RealmInitialization) Valid() bool {
	return initial.Realm != "" && initial.NextHop.Valid()
}

// RealmContactRevision is an opaque equality value issued by this Facet owner.
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

// RealmContactResult is the retained original decision, not necessarily the
// contact currently in force when a later Recover reads this receipt.
type RealmContactResult struct {
	OperationID OperationID
	Decision    RealmContactDecision
	Contact     RealmAttachment
}

// RealmConnections is a trusted local contact port. Its caller supplies the
// owner-persisted address and expected Realm on every use. The port cannot
// select a Realm, own the contact record, or cache authorization decisions.
// The context-bearing methods carry the original request's cancellation:
// under a network contact they are real remote waits.
type RealmConnections interface {
	Check(context.Context, values.NextHop, values.RealmID) error
	AuthorizePrincipal(values.NextHop, values.RealmID, values.PrincipalRef, ...values.Requirement) values.AuthorizationDecision
	AuthorizeView(context.Context, values.NextHop, values.RealmID, values.CallerContext, ...values.Requirement) (values.AuthorizationDecision, values.AuthorizationView)
	ValidateView(context.Context, values.NextHop, values.RealmID, values.CallerContext, values.AuthorizationView, ...values.Requirement) values.AuthorizationDecision
	CaptureDenialView(context.Context, values.NextHop, values.RealmID, values.CallerContext, values.Requirement) (values.AuthorizationDecision, values.AuthorizationDenialView)
	ValidateDenialView(context.Context, values.NextHop, values.RealmID, values.CallerContext, values.AuthorizationDenialView, values.Requirement) values.AuthorizationDecision
}
