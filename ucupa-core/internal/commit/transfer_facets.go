package commit

import (
	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	"ucupa/ucupa-core/internal/token"
	transferfact "ucupa/ucupa-core/internal/transfer/fact"
)

func (s *coreState) captureTransferEvidence(slice transferfact.Slice) (transferfact.Slice, error) {
	slice, err := s.captureTransferLineage(slice)
	if err != nil {
		return transferfact.Slice{}, err
	}
	// A slice fence or carrier manifest departs every member durably; an open
	// facet namespace replacement on any member must settle or cancel first.
	// An open re-acceptance proposal is refused earlier, when the slice members
	// are selected with a pending attachment.
	for _, member := range slice.Members() {
		if s.facetReplacementOpen(member.ID()) {
			return transferfact.Slice{}, contract.ErrConflict
		}
	}
	proposals := make(map[values.ElementID]values.PublicToken)
	for _, member := range slice.Members() {
		a := member.Facet.Attachment
		if a.State == contract.FacetNone {
			continue
		}
		p := token.NewPublicSigner(s.publicKey, s.authority).FacetProposal(a.Key, a.OriginalProposal)
		if !p.Valid() {
			return transferfact.Slice{}, contract.ErrStore
		}
		// A returning comparison owned by this Core must still be its exact
		// original signature, not a self-reported public token from a peer.
		if a.Key.Core == s.authority && p != token.PublicAttachmentRevision(s.publicKey, s.authority, a.Key.Proposal) {
			return transferfact.Slice{}, contract.ErrStore
		}
		proposals[member.ID()] = p
	}
	return slice.WithFacetProposals(proposals)
}

// Returning keys owned by this receiver can be checked against its original
// durable signer. This is an additional exact check, not remote authentication.
func (s *coreState) validateReturningFacetProposals(slice transferfact.Slice) error {
	for _, member := range slice.Members() {
		a := member.Facet.Attachment
		if a.Key.Valid() && a.Key.Core == s.authority && a.OriginalProposal != token.PublicAttachmentRevision(s.publicKey, s.authority, a.Key.Proposal) {
			return contract.ErrConflict
		}
	}
	return nil
}
