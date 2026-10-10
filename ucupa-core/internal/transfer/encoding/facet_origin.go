package encoding

import (
	"ucupa/ucupa-core/contract"
	facetfact "ucupa/ucupa-core/internal/facet/fact"
	transferfact "ucupa/ucupa-core/internal/transfer/fact"
)

func validateTransferFacetOrigin(slice transferfact.Slice) error {
	for _, member := range slice.Members() {
		a := member.Facet.Attachment
		if _, err := facetfact.TransferPredecessor(member.Facet, member.ID(), slice.Cut()); err != nil {
			return err
		}
		if !a.Key.Valid() || a.Key.Core == slice.Source() {
			continue
		}
		if a.State != contract.FacetDetached && a.State != contract.FacetRetired {
			return contract.ErrAttachmentStateConflict
		}
		known := false
		for _, step := range slice.Lineage(member.ID()) {
			known = known || (step.Source == a.Key.Core && transferAttachmentToken(a.Key.Proposal, member.ID(), step.FinalCut))
		}
		if !known {
			return contract.ErrConflict
		}
	}
	return nil
}
