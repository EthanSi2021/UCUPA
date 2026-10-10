// Package projection computes bounded Federation read results from one borrowed
// immutable fact view. It owns no facts, authorization, registry, Store or locks.
// Its caller retains and revalidates the original borrow before disclosure.
package projection

import (
	"ucupa/shared/values"
	elementfact "ucupa/ucupa-core/internal/element/fact"
	facetfact "ucupa/ucupa-core/internal/facet/fact"
	mountfact "ucupa/ucupa-core/internal/mount/fact"
	coretoken "ucupa/ucupa-core/internal/token"
	transferfact "ucupa/ucupa-core/internal/transfer/fact"
)

type Projection struct {
	elements    elementfact.View
	mount       mountfact.View
	attachments facetfact.View
	transfers   transferfact.Visibility
	signer      coretoken.PublicSigner
}

// New adapts only the finite views of the same owner-local borrow. It neither
// copies a subtree nor extends the borrow's lifetime. The signer is call-local.
func New(elements elementfact.View, mounts mountfact.View, attachments facetfact.View, transfers transferfact.Visibility, signer coretoken.PublicSigner) *Projection {
	return &Projection{elements: elements, mount: mounts, attachments: attachments, transfers: transfers, signer: signer}
}

// The original closure grant cannot reveal whether an arbitrary outside UUID
// departed. Only a known member of this boundary gets the unavailable outcome.
func (s *Projection) inactiveWithin(id, boundary values.ElementID) bool {
	departed := s.transfers.Boundary(id)
	if departed == "" {
		return false
	}
	if id == boundary {
		return true
	}
	for fence := range s.transfers.Fences() {
		if fence.Boundary == departed && s.elements.DescendsFrom(fence.Origin.Parent, boundary) {
			return true
		}
	}
	return false
}
