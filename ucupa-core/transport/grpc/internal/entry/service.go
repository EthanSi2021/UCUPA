// Package entry adapts the CoreService protocol to the existing public owner.
// It shares the host's admission lifetime and owns no facts or session tables.
package entry

import (
	"ucupa/shared/values"
	corepb "ucupa/ucupa-core/bindings/go/protobuf"
	"ucupa/ucupa-core/transport/grpc/federationconn"
	"ucupa/ucupa-core/transport/grpc/internal/admission"
)

// Config contains only the existing protocol bounds and fixed federation ports.
type Config struct {
	NextHop               values.NextHop
	MaxResponseBytes      int
	FederationConnections *federationconn.FederationConnections
	IdentityProofs        admission.IdentityProofIssuer
}

type Service struct {
	corepb.UnimplementedCoreServiceServer
	owner     admission.Owner
	admission *admission.Service
	config    Config
}

// New receives the host's validated fixed configuration and single admission.
func New(owner admission.Owner, gate *admission.Service, config Config) *Service {
	return &Service{owner: owner, admission: gate, config: config}
}
