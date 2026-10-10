package entry

import (
	"context"
	"errors"

	"ucupa/shared/values"
	federationcontract "ucupa/ucupa-core/contract/federation"
	"ucupa/ucupa-core/transport/grpc/internal/admission"
	grpcsession "ucupa/ucupa-core/transport/grpc/internal/session"
)

type publicFederationOwner interface {
	PublicCallWithFederation(context.Context, values.CallerContext, []byte, federationcontract.FederationConnections) ([]byte, error)
}

func ValidateFederation(owner admission.Owner, expected values.FactAuthorityRef, c Config) error {
	if c.FederationConnections == nil {
		return nil
	}
	if !c.FederationConnections.MatchesSource(expected) || c.IdentityProofs == nil {
		return errors.New("federation requires this Core's original-caller identity issuer")
	}
	if _, ok := owner.(publicFederationOwner); !ok {
		return errors.New("federated public owner required")
	}
	return nil
}

// Only the original authenticated entry may coordinate. A source's restricted
// read grant does not gain permission to redelegate or issue a new identity.
// All durable and read decisions remain inside the owner, not this adapter.
func (s *Service) publicOwnerCall(ctx context.Context, caller values.CallerContext, request []byte) ([]byte, error) {
	if s.config.FederationConnections == nil {
		return s.owner.PublicCall(ctx, caller, request)
	}
	if _, delegated := grpcsession.Grant(ctx); delegated {
		return s.owner.PublicCall(ctx, caller, request)
	}
	entry, err := s.admission.BindEntrySession(ctx, caller)
	if err != nil {
		return nil, err
	}
	connections, err := s.config.FederationConnections.WithCaller(caller, s.config.IdentityProofs, entry.Expires())
	if err != nil {
		return nil, err
	}
	result, err := s.owner.(publicFederationOwner).PublicCallWithFederation(ctx, caller, request, connections)
	// A slow remote source cannot extend the entry session, nor can a concurrent
	// ReleaseSession or a newly established session revive this invocation.
	if err := s.admission.CheckEntrySession(entry); err != nil {
		return nil, err
	}
	if ctx.Err() != nil {
		return nil, values.PublicUnavailable
	}
	return result, err
}
