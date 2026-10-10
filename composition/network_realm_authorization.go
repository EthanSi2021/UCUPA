package composition

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"ucupa/shared/values"
	realmpb "ucupa/ucupa-realm/bindings/go/protobuf"
	realmsdk "ucupa/ucupa-realm/sdk/go"
)

// ReleaseRealmEvidence retires one unused reference at its exact bound endpoint.
// This is cleanup, not a retry of the canceled business request: one separate,
// bounded attempt is allowed after cancellation; failure falls back to TTL.
func (connections *NetworkRealmConnections) ReleaseRealmEvidence(ctx context.Context, address values.NextHop, expected values.RealmID, evidence [32]byte) {
	if ctx == nil || evidence == ([32]byte{}) {
		return
	}
	endpoint, err := connections.selectEndpoint(address, expected)
	if err != nil {
		return
	}
	cleanup, cancel := context.WithTimeout(ctx, endpoint.config.RPCDeadline)
	defer cancel()
	_, _ = endpoint.client.IdentityService().ReleaseHandle(cleanup, &realmpb.ReleaseHandleRequest{Handle: evidence[:]})
}

// AuthorizePrincipal has no network counterpart: the wire protocol carries
// only established caller handles, and a Core never sends raw principal names
// as authorization requests. The network profile fails closed.
func (connections *NetworkRealmConnections) AuthorizePrincipal(values.NextHop, values.RealmID, values.PrincipalRef, ...values.Requirement) values.AuthorizationDecision {
	return values.AuthorizationUnavailable
}

func (connections *NetworkRealmConnections) AuthorizeView(ctx context.Context, address values.NextHop, expected values.RealmID, caller values.CallerContext, requirements ...values.Requirement) (values.AuthorizationDecision, values.AuthorizationView) {
	endpoint, err := connections.selectEndpoint(address, expected)
	if err != nil {
		return values.AuthorizationUnavailable, values.AuthorizationView{}
	}
	wire, ok := requirementsToWire(requirements)
	if !ok || caller.Establishment == ([32]byte{}) {
		return values.AuthorizationDenied, values.AuthorizationView{}
	}
	var response *realmpb.AuthorizeViewResponse
	invokeErr := connections.invoke(ctx, endpoint, func(ctx context.Context, client *realmsdk.Client) error {
		result, callErr := client.AuthorizationService().AuthorizeView(ctx, &realmpb.AuthorizeViewRequest{
			Caller: caller.Establishment[:], Requirements: wire,
		})
		response = result
		return callErr
	})
	if !connections.stillBound(address, expected, endpoint) {
		return values.AuthorizationUnavailable, values.AuthorizationView{}
	}
	if invokeErr != nil {
		// An unauthenticated handle is the kernel's invalid-caller denial; a
		// malformed requirement set keeps the same denial, never a pass.
		if code := status.Code(invokeErr); code == codes.Unauthenticated || code == codes.InvalidArgument {
			return values.AuthorizationDenied, values.AuthorizationView{}
		}
		return values.AuthorizationUnavailable, values.AuthorizationView{}
	}
	decision := wireDecision(response.GetDecision())
	if decision != values.AuthorizationValid {
		return decision, values.AuthorizationView{}
	}
	view, ok := networkView(response.GetView())
	if !ok {
		return values.AuthorizationUnavailable, values.AuthorizationView{}
	}
	return values.AuthorizationValid, view
}

func (connections *NetworkRealmConnections) ValidateView(ctx context.Context, address values.NextHop, expected values.RealmID, caller values.CallerContext, view values.AuthorizationView, requirements ...values.Requirement) values.AuthorizationDecision {
	endpoint, err := connections.selectEndpoint(address, expected)
	if err != nil {
		return values.AuthorizationUnavailable
	}
	wire, ok := requirementsToWire(requirements)
	if !ok || caller.Establishment == ([32]byte{}) || view == (values.AuthorizationView{}) {
		return values.AuthorizationChanged
	}
	var decision realmpb.Decision
	invokeErr := connections.invoke(ctx, endpoint, func(ctx context.Context, client *realmsdk.Client) error {
		result, callErr := client.AuthorizationService().ValidateView(ctx, &realmpb.ValidateViewRequest{
			Caller: caller.Establishment[:], View: view[:], Requirements: wire,
		})
		if callErr == nil {
			decision = result.GetDecision()
		}
		return callErr
	})
	if !connections.stillBound(address, expected, endpoint) {
		return values.AuthorizationUnavailable
	}
	if invokeErr != nil {
		// A handle or view the Realm does not recognize is changed evidence,
		// exactly like a kernel view mismatch; it can never be re-upgraded.
		if code := status.Code(invokeErr); code == codes.Unauthenticated {
			return values.AuthorizationDenied
		}
		if code := status.Code(invokeErr); code == codes.InvalidArgument {
			return values.AuthorizationChanged
		}
		return values.AuthorizationUnavailable
	}
	return wireDecision(decision)
}

func (connections *NetworkRealmConnections) CaptureDenialView(ctx context.Context, address values.NextHop, expected values.RealmID, caller values.CallerContext, requirement values.Requirement) (values.AuthorizationDecision, values.AuthorizationDenialView) {
	endpoint, err := connections.selectEndpoint(address, expected)
	if err != nil {
		return values.AuthorizationUnavailable, values.AuthorizationDenialView{}
	}
	wire, ok := requirementToWire(requirement)
	if !ok || caller.Establishment == ([32]byte{}) {
		return values.AuthorizationDenied, values.AuthorizationDenialView{}
	}
	var response *realmpb.CaptureDenialViewResponse
	invokeErr := connections.invoke(ctx, endpoint, func(ctx context.Context, client *realmsdk.Client) error {
		result, callErr := client.AuthorizationService().CaptureDenialView(ctx, &realmpb.CaptureDenialViewRequest{
			Caller: caller.Establishment[:], Requirement: wire,
		})
		response = result
		return callErr
	})
	if !connections.stillBound(address, expected, endpoint) {
		return values.AuthorizationUnavailable, values.AuthorizationDenialView{}
	}
	if invokeErr != nil {
		if code := status.Code(invokeErr); code == codes.Unauthenticated || code == codes.InvalidArgument {
			return values.AuthorizationDenied, values.AuthorizationDenialView{}
		}
		return values.AuthorizationUnavailable, values.AuthorizationDenialView{}
	}
	decision := wireDecision(response.GetDecision())
	if decision != values.AuthorizationDenied {
		return decision, values.AuthorizationDenialView{}
	}
	denial, ok := networkDenial(response.GetDenial())
	if !ok {
		return values.AuthorizationDenied, values.AuthorizationDenialView{}
	}
	return values.AuthorizationDenied, denial
}

func (connections *NetworkRealmConnections) ValidateDenialView(ctx context.Context, address values.NextHop, expected values.RealmID, caller values.CallerContext, denial values.AuthorizationDenialView, requirement values.Requirement) values.AuthorizationDecision {
	endpoint, err := connections.selectEndpoint(address, expected)
	if err != nil {
		return values.AuthorizationUnavailable
	}
	wire, ok := requirementToWire(requirement)
	if !ok || caller.Establishment == ([32]byte{}) || denial == (values.AuthorizationDenialView{}) {
		return values.AuthorizationChanged
	}
	var decision realmpb.Decision
	invokeErr := connections.invoke(ctx, endpoint, func(ctx context.Context, client *realmsdk.Client) error {
		result, callErr := client.AuthorizationService().ValidateDenialView(ctx, &realmpb.ValidateDenialViewRequest{
			Caller: caller.Establishment[:], Denial: denial[:], Requirement: wire,
		})
		if callErr == nil {
			decision = result.GetDecision()
		}
		return callErr
	})
	if !connections.stillBound(address, expected, endpoint) {
		return values.AuthorizationUnavailable
	}
	if invokeErr != nil {
		if code := status.Code(invokeErr); code == codes.Unauthenticated {
			return values.AuthorizationDenied
		}
		if code := status.Code(invokeErr); code == codes.InvalidArgument {
			return values.AuthorizationChanged
		}
		return values.AuthorizationUnavailable
	}
	return wireDecision(decision)
}

func wireDecision(decision realmpb.Decision) values.AuthorizationDecision {
	switch decision {
	case realmpb.Decision_VALID:
		return values.AuthorizationValid
	case realmpb.Decision_CHANGED:
		return values.AuthorizationChanged
	case realmpb.Decision_DENIED:
		return values.AuthorizationDenied
	default:
		return values.AuthorizationUnavailable
	}
}
