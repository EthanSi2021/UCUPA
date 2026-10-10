package commit

import (
	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	facetfact "ucupa/ucupa-core/internal/facet/fact"
	"ucupa/ucupa-core/internal/token"
)

func FacetCommandKind(s string) facetfact.Kind {
	switch s {
	case "propose":
		return facetfact.Propose
	case "cancel":
		return facetfact.Cancel
	case "activate":
		return facetfact.Activate
	case "detach":
		return facetfact.Detach
	case "release":
		return facetfact.Release
	case "contact":
		return facetfact.Address
	default:
		return 0
	}
}

func (state *coreState) matchesPublicFacet(c contract.PublicFacetCommand, b facetfact.Binding) bool {
	if FacetCommandKind(c.Kind) != b.Kind || c.Element != b.Element || c.Target != b.Target || c.NextHop != b.Address {
		return false
	}
	if b.Kind == facetfact.Address {
		return c.Expected == token.PublicFacetContact(state.publicKey, state.authority, b.Contact)
	}
	return c.Expected == token.PublicAttachmentRevision(state.publicKey, state.authority, b.Expected)
}

// Resolve exactly one operation's original comparison, or the current record
// for a new operation. No retained journal or mutable fact escapes this choice.
func (state *coreState) resolvePublicFacet(id contract.OperationID, input contract.PublicFacetCommand, b facetfact.Binding) (facetfact.Binding, error) {
	if retained, ok := state.attachments.operations[id]; ok {
		if retained.binding.Actor != b.Actor || !state.matchesPublicFacet(input, retained.binding) {
			return facetfact.Binding{}, values.PublicConflict
		}
		return retained.binding, nil
	}
	r, ok := state.attachments.facts.Record(input.Element)
	if !ok {
		return facetfact.Binding{}, values.PublicNotFound
	}
	if b.Kind == facetfact.Address {
		b.Key, b.Contact = r.Key(), r.Contact
	} else {
		b.Expected = r.Attachment.Revision
	}
	if !b.Valid() || !state.matchesPublicFacet(input, b) {
		return facetfact.Binding{}, values.PublicConflict
	}
	return b, nil
}

func (owner *Owner) ResolvePublicFacetLocked(id contract.OperationID, input contract.PublicFacetCommand, binding facetfact.Binding) (facetfact.Binding, error) {
	return owner.state.resolvePublicFacet(id, input, binding)
}
