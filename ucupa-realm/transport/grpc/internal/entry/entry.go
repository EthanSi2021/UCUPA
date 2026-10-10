// Package entry implements the typed Realm services against the actual
// consumer port. It owns neither Realm facts nor a second handle registry.
package entry

import (
	"context"
	"time"

	"ucupa/shared/values"
	realmpb "ucupa/ucupa-realm/bindings/go/protobuf"
	"ucupa/ucupa-realm/contract"
	"ucupa/ucupa-realm/transport/grpc/internal/admission"
)

type Kernel interface {
	ID() values.RealmID
	ActiveID() (values.RealmID, bool)
	EstablishCaller(contract.CredentialEvidence) (values.CallerContext, error)
	TrustLocalCaller(values.PrincipalRef) (values.CallerContext, error)
	ForkCaller(context.Context, values.CallerContext) (values.CallerContext, error)
	AuthorizeView(values.CallerContext, ...values.Requirement) (values.AuthorizationDecision, values.AuthorizationView)
	ValidateView(values.CallerContext, values.AuthorizationView, ...values.Requirement) values.AuthorizationDecision
	CaptureDenialView(values.CallerContext, values.Requirement) (values.AuthorizationDecision, values.AuthorizationDenialView)
	ValidateDenialView(values.CallerContext, values.AuthorizationDenialView, values.Requirement) values.AuthorizationDecision
}

type Verifier interface {
	Verify(contract.IdentityProof, values.FactAuthorityRef, [32]byte, time.Time) (contract.IdentityClaim, error)
}

// Liveness is an authenticated check at the explicitly configured issuer.
type Liveness interface {
	Check(context.Context, []byte, []byte) error
}

type Config struct {
	Expected                                         values.RealmID
	Foreign                                          Verifier
	ForeignLiveness                                  map[values.RealmID]Liveness
	HandleTTL, ChallengeTTL                          time.Duration
	MaxHandles, MaxHandlesPerCaller, MaxRequirements int
}

type Service struct {
	realmpb.UnimplementedRealmTargetServiceServer
	realmpb.UnimplementedRealmIdentityServiceServer
	realmpb.UnimplementedRealmAuthorizationServiceServer
	config   Config
	realm    Kernel
	registry *admission.Registry
	now      func() time.Time
}

func New(kernel Kernel, config Config, registry *admission.Registry, now func() time.Time) *Service {
	return &Service{realm: kernel, config: config, registry: registry, now: now}
}
