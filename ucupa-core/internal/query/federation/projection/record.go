package projection

import (
	federationvalue "ucupa/ucupa-core/internal/query/federation/value"

	"ucupa/shared/values"
	corecontract "ucupa/ucupa-core/contract"
	facetfact "ucupa/ucupa-core/internal/facet/fact"
	"ucupa/ucupa-core/internal/geometry"
	queryscan "ucupa/ucupa-core/internal/query/scan"
)

func (state *Projection) record(id, boundary values.ElementID, attachment bool, remaining uint64) (federationvalue.FederationRecord, uint64, error) {
	depth, cursor := uint64(0), id
	for cursor != boundary {
		depth++
		if depth > uint64(state.elements.Counts().Anchors)+1 {
			return federationvalue.FederationRecord{}, 0, values.PublicUnavailable
		}
		if depth > remaining/64 {
			return federationvalue.FederationRecord{}, 0, values.PublicResourceExhausted
		}
		if a, ok := state.elements.Anchor(cursor); ok {
			cursor = a.Parent
		} else if e, ok := state.elements.Entity(cursor); ok {
			cursor = e.Parent
		} else {
			return federationvalue.FederationRecord{}, 0, values.PublicUnavailable
		}
	}
	path, err := queryscan.Path(state.elements, id, boundary)
	if err != nil {
		return federationvalue.FederationRecord{}, 0, ReadError(err)
	}
	row := federationvalue.FederationRecord{ID: id, Path: path}
	if a, ok := state.elements.Anchor(id); ok {
		row.Kind, row.Parent, row.Range, row.Revision = corecontract.ElementAnchor, a.Parent, a.Range, state.signer.ElementRevision(a.Revision)
		if !a.Root() {
			pose := a.LocalPose
			row.Pose = &pose
		}
	} else if e, ok := state.elements.Entity(id); ok {
		row.Kind, row.Parent, row.Range, row.Revision = corecontract.ElementEntity, e.Parent, geometry.BoundedRange(e.Range), state.signer.ElementRevision(e.Revision)
		pose := e.LocalPose
		row.Pose = &pose
	} else {
		return federationvalue.FederationRecord{}, 0, values.PublicNotFound
	}
	if attachment {
		stored, ok := state.attachments.Record(id)
		if !ok || !stored.Attachment.Revision.Valid() {
			return federationvalue.FederationRecord{}, 0, values.PublicUnavailable
		}
		a := stored.Attachment
		value := facetfact.PublicInspection(state.signer, id, corecontract.FacetAttachmentResult{Attachment: a, NextHop: a.NextHop, NextHopRevision: a.NextHopRevision})
		row.Attachment = &value
	}
	// Spatial List never exports an unrelated mount service address. Only a
	// projection's necessary Frontier contains authenticated contact evidence.
	return row, depth * 64, nil
}
