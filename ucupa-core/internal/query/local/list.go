package local

import (
	"ucupa/shared/values"
	accessvalue "ucupa/ucupa-core/internal/access/value"
	commitpkg "ucupa/ucupa-core/internal/commit"
	elementfact "ucupa/ucupa-core/internal/element/fact"
	elementread "ucupa/ucupa-core/internal/element/read"
	mountfact "ucupa/ucupa-core/internal/mount/fact"
	queryscan "ucupa/ucupa-core/internal/query/scan"
	coretoken "ucupa/ucupa-core/internal/token"
)

func publicListJSON(projection commitpkg.ReadView, signer coretoken.PublicSigner, boundary values.ElementID, address any, after values.ElementID, limit int) (map[string]any, values.ElementID, error) {
	parent, err := publicAddressForViews(projection.Elements, projection.Mount, projection.Transfers, boundary, address)
	if err != nil {
		return nil, "", err
	}
	if _, ok := projection.Elements.Anchor(parent); !ok {
		return nil, "", values.PublicInvalid
	}
	if err := mountfact.GuardLocalList(projection.Mount, parent); err != nil {
		return nil, "", values.PublicUnavailable
	}
	ids := make([]values.ElementID, 0)
	for id, anchor := range projection.Elements.Anchors() {
		if anchor.Parent == parent && id > after && !projection.Transfers.Inactive(id) {
			ids = append(ids, id)
		}
	}
	for id, entity := range projection.Elements.Entities() {
		if entity.Parent == parent && id > after && !projection.Transfers.Inactive(id) {
			ids = append(ids, id)
		}
	}
	elementfact.SortIDs(ids)
	records := make([]any, 0, min(limit, len(ids)))
	for _, id := range ids[:min(limit, len(ids))] {
		read, err := elementread.Element(projection.Authority, projection.Elements, projection.Attachments, projection.Transfers, id, projection.CurrentCut())
		if err != nil {
			return nil, "", err
		}
		record := accessvalue.ElementRecordJSON(signer, *read.Record)
		path, err := queryscan.Path(projection.Elements, id, boundary)
		if err != nil {
			return nil, "", err
		}
		record["address"] = accessvalue.PathJSON(boundary, path)
		records = append(records, record)
	}
	last := values.ElementID("")
	status := "COMPLETE"
	if len(ids) > limit {
		last, status = ids[limit-1], "PARTIAL"
	}
	for fence := range projection.Transfers.Fences() {
		if fence.Origin.Parent == parent {
			status = "PARTIAL"
		}
	}
	return map[string]any{"records": records, "status": status}, last, nil
}
