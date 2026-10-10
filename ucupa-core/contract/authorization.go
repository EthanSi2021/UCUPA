package contract

import (
	"context"

	"ucupa/shared/values"
)

// Authorizer is owned by Core. Every method reports the decision of the Core's
// one currently attached Realm instead of collapsing unavailable evidence into
// a denial. The view methods carry the original request's context: they run
// outside any fact lock, and under a network Realm contact they are real
// remote waits that must honor that context's cancellation and deadline.
// Realm can satisfy this port without Core importing Realm; composition
// supplies the concrete contact-bound adapter.
type Authorizer interface {
	AuthorizePrincipal(values.PrincipalRef, ...values.Requirement) values.AuthorizationDecision
	AuthorizeView(context.Context, values.CallerContext, ...values.Requirement) (values.AuthorizationDecision, values.AuthorizationView)
	ValidateView(context.Context, values.CallerContext, values.AuthorizationView, ...values.Requirement) values.AuthorizationDecision
	CaptureDenialView(context.Context, values.CallerContext, values.Requirement) (values.AuthorizationDecision, values.AuthorizationDenialView)
	ValidateDenialView(context.Context, values.CallerContext, values.AuthorizationDenialView, values.Requirement) values.AuthorizationDecision
}
