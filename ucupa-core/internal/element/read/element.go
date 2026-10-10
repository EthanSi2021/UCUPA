// Package read projects finite, already-selected Element facts for local and
// query consumers. It owns no state, locks, authorization, or read lifetime.
package read

import (
	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	elementfact "ucupa/ucupa-core/internal/element/fact"
	facetfact "ucupa/ucupa-core/internal/facet/fact"
	transferfact "ucupa/ucupa-core/internal/transfer/fact"
)

func Element(authority values.FactAuthorityRef, elements elementfact.View, attachments facetfact.View, transfers transferfact.Visibility, id values.ElementID, cut contract.CoreReadCut) (contract.ElementRead, error) {
	if transfers.Inactive(id) {
		return contract.ElementRead{}, contract.ErrUnavailable
	}
	result := contract.ElementRead{Authority: authority, Element: id, State: contract.ExistenceNotFound, Cut: cut}
	var record contract.ElementRecord
	if anchor, ok := elements.Anchor(id); ok {
		record = anchor.Record()
	} else if entity, ok := elements.Entity(id); ok {
		record = entity.Record()
	} else {
		return result, nil
	}
	attachment, ok := attachments.Record(id)
	if !ok || !attachment.Attachment.Revision.Valid() {
		return contract.ElementRead{}, contract.ErrStore
	}
	result.State, result.Record, result.Attachment = contract.ExistencePresent, &record, attachment.Attachment
	return result, nil
}
