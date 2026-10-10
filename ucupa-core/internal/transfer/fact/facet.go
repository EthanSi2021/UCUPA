package fact

import (
	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
)

// WithFacetProposals enriches immutable source copies with original public
// comparison values. These must be issued or retained by the actual source;
// this value method alone cannot authenticate them or adopt Facet content.
func (s Slice) WithFacetProposals(proposals map[values.ElementID]values.PublicToken) (Slice, error) {
	members := s.Members()
	matched := 0
	for index := range members {
		a := &members[index].Facet.Attachment
		p, provided := proposals[members[index].ID()]
		if a.State == contract.FacetNone {
			if provided || a.OriginalProposal != (values.PublicToken{}) {
				return Slice{}, contract.ErrAttachmentStateConflict
			}
			continue
		}
		if (a.State != contract.FacetActive && a.State != contract.FacetDetached && a.State != contract.FacetRetired) || !provided || !a.Key.Valid() || a.Key.Element != members[index].ID() || !p.Valid() || p.Owner != a.Key.Core || p.Kind != "attachment-revision" ||
			(a.OriginalProposal != (values.PublicToken{}) && a.OriginalProposal != p) {
			return Slice{}, contract.ErrAttachmentStateConflict
		}
		a.OriginalProposal = p
		matched++
	}
	if matched != len(proposals) {
		return Slice{}, contract.ErrAttachmentStateConflict
	}
	s.members = members
	return s, nil
}
