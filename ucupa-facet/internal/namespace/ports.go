package namespace

import (
	"context"
	"ucupa/shared/values"
	"ucupa/ucupa-facet/contract"
)

// CoreControlSource is a fixed-service-identity connection, not a client
// capability. Implementations must never manufacture continuity from two bool
// checks. Its tokens stay inside Facet and its assembly adapter.
type coreControlSource interface {
	ReadControl(context.Context, values.NextHop, values.FacetAcceptanceKey, bool) (contract.CoreControlRead, error)
	CheckControl(context.Context, values.NextHop, values.FacetAcceptanceKey, values.PublicToken) error
	ControlCall(context.Context, values.NextHop, values.FacetAcceptanceKey, []byte) ([]byte, error)
}

// CoreProposalSource resolves one caller-visible proposal through the supplied
// Core address using the Facet's fixed service identity. The returned key and
// evidence are service-only values, never request credentials.
type coreProposalSource interface {
	ResolveProposal(context.Context, values.NextHop, contract.PublicAcceptanceRef) (contract.CoreProposalRead, error)
}
