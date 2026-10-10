package entry

import (
	"context"
	"crypto/sha256"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"ucupa/shared/values"
	realmpb "ucupa/ucupa-realm/bindings/go/protobuf"
	admission "ucupa/ucupa-realm/transport/grpc/internal/admission"
	wire "ucupa/ucupa-realm/transport/grpc/internal/wire"
)

// derivedExpiry caps a derived handle's lease to its parent caller handle's
// remaining lifetime; derivation never extends the original lease.
func (s *Service) derivedExpiry(parent time.Time) time.Time {
	expires := s.now().Add(s.config.HandleTTL)
	if parent.Before(expires) {
		return parent
	}
	return expires
}

// AuthorizeView asks the kernel for the current decision and, when valid,
// issues one run-local view handle covering exactly this requirement set.
func (s *Service) AuthorizeView(ctx context.Context, in *realmpb.AuthorizeViewRequest) (*realmpb.AuthorizeViewResponse, error) {
	token, entry, err := s.callerEntry(ctx, in.GetCaller())
	if err != nil {
		return nil, err
	}
	requirements, err := wire.Requirements(in.GetRequirements(), s.config.MaxRequirements)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid requirement set")
	}
	decision, view := s.realm.AuthorizeView(entry.Caller, requirements...)
	response := &realmpb.AuthorizeViewResponse{Decision: wire.DecisionResponse(decision)}
	if decision == values.AuthorizationValid {
		viewToken, err := s.registry.Issue(admission.Entry{
			Kind:         admission.View,
			Owner:        entry.Owner,
			Caller:       entry.Caller,
			Requirements: requirements,
			View:         view,
			Parent:       sha256.Sum256(token[:]),
			Expires:      s.derivedExpiry(entry.Expires),
		}, s.config.MaxHandles, s.config.MaxHandlesPerCaller)
		if err != nil {
			return nil, err
		}
		response.View = viewToken[:]
	}
	return response, nil
}

// ValidateView revalidates one view handle online: the caller handle, the
// binding and the exact requirement set must all match, then the kernel
// re-decides with no cached Allow.
func (s *Service) ValidateView(ctx context.Context, in *realmpb.ValidateViewRequest) (*realmpb.ValidateViewResponse, error) {
	token, entry, err := s.callerEntry(ctx, in.GetCaller())
	if err != nil {
		return nil, err
	}
	viewToken, err := wire.Exact32(in.GetView())
	if err != nil {
		return nil, err
	}
	viewEntry, err := s.registry.Lookup(viewToken)
	if err != nil {
		return nil, err
	}
	if viewEntry.Kind != admission.View || viewEntry.Owner != entry.Owner || viewEntry.Parent != sha256.Sum256(token[:]) {
		return nil, status.Error(codes.InvalidArgument, "handle mismatch")
	}
	requirements, err := wire.Requirements(in.GetRequirements(), s.config.MaxRequirements)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid requirement set")
	}
	if !wire.SameRequirements(requirements, viewEntry.Requirements) {
		return nil, status.Error(codes.InvalidArgument, "handle mismatch")
	}
	decision := s.realm.ValidateView(entry.Caller, viewEntry.View, viewEntry.Requirements...)
	return &realmpb.ValidateViewResponse{Decision: wire.DecisionResponse(decision)}, nil
}

// CaptureDenialView proves a closed branch. Only a real DENIED with a nonzero
// kernel view yields a handle; VALID is still reported as valid, never as a
// denial, and grants nothing.
func (s *Service) CaptureDenialView(ctx context.Context, in *realmpb.CaptureDenialViewRequest) (*realmpb.CaptureDenialViewResponse, error) {
	token, entry, err := s.callerEntry(ctx, in.GetCaller())
	if err != nil {
		return nil, err
	}
	requirement, err := wire.Requirement(in.GetRequirement())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid requirement")
	}
	decision, denial := s.realm.CaptureDenialView(entry.Caller, requirement)
	response := &realmpb.CaptureDenialViewResponse{Decision: wire.DecisionResponse(decision)}
	if decision == values.AuthorizationDenied && denial != (values.AuthorizationDenialView{}) {
		denialToken, err := s.registry.Issue(admission.Entry{
			Kind:         admission.Denial,
			Owner:        entry.Owner,
			Caller:       entry.Caller,
			Requirements: []values.Requirement{requirement},
			Denial:       denial,
			Parent:       sha256.Sum256(token[:]),
			Expires:      s.derivedExpiry(entry.Expires),
		}, s.config.MaxHandles, s.config.MaxHandlesPerCaller)
		if err != nil {
			return nil, err
		}
		response.Denial = denialToken[:]
	}
	return response, nil
}

// ValidateDenialView revalidates one captured denial online. A valid denial
// proves the branch stayed denied; it never proves an Allow.
func (s *Service) ValidateDenialView(ctx context.Context, in *realmpb.ValidateDenialViewRequest) (*realmpb.ValidateDenialViewResponse, error) {
	token, entry, err := s.callerEntry(ctx, in.GetCaller())
	if err != nil {
		return nil, err
	}
	denialToken, err := wire.Exact32(in.GetDenial())
	if err != nil {
		return nil, err
	}
	denialEntry, err := s.registry.Lookup(denialToken)
	if err != nil {
		return nil, err
	}
	if denialEntry.Kind != admission.Denial || denialEntry.Owner != entry.Owner || denialEntry.Parent != sha256.Sum256(token[:]) {
		return nil, status.Error(codes.InvalidArgument, "handle mismatch")
	}
	requirement, err := wire.Requirement(in.GetRequirement())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid requirement")
	}
	if len(denialEntry.Requirements) != 1 || denialEntry.Requirements[0] != requirement {
		return nil, status.Error(codes.InvalidArgument, "handle mismatch")
	}
	decision := s.realm.ValidateDenialView(entry.Caller, denialEntry.Denial, requirement)
	return &realmpb.ValidateDenialViewResponse{Decision: wire.DecisionResponse(decision)}, nil
}
