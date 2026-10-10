// Package denial owns the opaque evidence retained for one authenticated
// Federation discovery denial. It is runtime authorization evidence, not a
// serializable query value or a second access decision.
package denial

import (
	"time"

	"ucupa/shared/values"
	mountfact "ucupa/ucupa-core/internal/mount/fact"
	federationvalue "ucupa/ucupa-core/internal/query/federation/value"
)

// Evidence is the single opaque representation of a captured Federation
// denial. External consumers can retain or return a value received from the
// Core, but cannot construct a nonzero value or inspect its authorization
// evidence. The owning Core uses only the bounded functions below.
type Evidence struct {
	owner       values.FactAuthorityRef
	source      values.FactAuthorityRef
	caller      values.CallerContext
	request     federationvalue.FederationReadRequest
	requirement values.Requirement
	view        values.AuthorizationDenialView
	incoming    mountfact.Incoming
	expires     time.Time
}

// New captures an already-adjudicated denial. Validation and error ordering
// remain with the source owner; this package only retains the exact evidence.
func New(
	owner values.FactAuthorityRef,
	source values.FactAuthorityRef,
	caller values.CallerContext,
	request federationvalue.FederationReadRequest,
	requirement values.Requirement,
	view values.AuthorizationDenialView,
	incoming mountfact.Incoming,
	expires time.Time,
) Evidence {
	return Evidence{
		owner:       owner,
		source:      source,
		caller:      caller,
		request:     request,
		requirement: requirement,
		view:        view,
		incoming:    incoming,
		expires:     expires,
	}
}

// MatchesBinding checks the complete authenticated owner/coordinator/caller
// and original-request binding. A zero authorization view is never evidence.
func MatchesBinding(evidence Evidence, owner, source values.FactAuthorityRef, caller values.CallerContext, request federationvalue.FederationReadRequest) bool {
	return evidence.owner == owner && evidence.source == source && evidence.caller == caller &&
		evidence.request == request && evidence.view != (values.AuthorizationDenialView{})
}

// Authorization returns only the denial view and requirement needed for the
// owning authorizer's continuity check.
func Authorization(evidence Evidence) (values.AuthorizationDenialView, values.Requirement) {
	return evidence.view, evidence.requirement
}

// Incoming returns the exact admitted incoming relation retained by the
// evidence. It is used only to seed the source-local relationship dependency.
func Incoming(evidence Evidence) mountfact.Incoming {
	return evidence.incoming
}

// Expired applies the original absolute lease boundary: equality is expired.
func Expired(evidence Evidence, now time.Time) bool {
	return !now.Before(evidence.expires)
}

// MatchesIncoming checks that the exact admitted incoming relation has not
// changed since capture.
func MatchesIncoming(evidence Evidence, incoming mountfact.Incoming) bool {
	return evidence.incoming == incoming
}
