package value

import (
	"ucupa/shared/values"
	corecontract "ucupa/ucupa-core/contract"
	coretoken "ucupa/ucupa-core/internal/token"
)

func PathJSON(origin values.ElementID, path []values.ElementID) map[string]any {
	chain := make([]any, len(path))
	for i, id := range path {
		chain[i] = string(id)
	}
	return map[string]any{"origin": string(origin), "up": "0", "down": chain}
}

func ElementRecordJSON(signer coretoken.PublicSigner, record corecontract.ElementRecord) map[string]any {
	if record.Kind == corecontract.ElementAnchor {
		a := record.Anchor
		result := map[string]any{"kind": "anchor", "element": string(a.ID), "revision": TokenJSON(signer.ElementRevision(a.Revision))}
		if a.ParentAnchorID == nil {
			result["range"] = map[string]any{"kind": "unbounded-root"}
		} else {
			sphere, _ := a.Range.BoundedSphere()
			result["parent"] = string(*a.ParentAnchorID)
			result["pose"] = PoseJSON(*a.LocalPose)
			result["range"] = map[string]any{"kind": "bounded-sphere", "radius": sphere.Radius()}
		}
		return result
	}
	e := record.Entity
	return map[string]any{"kind": "entity", "element": string(e.ID), "parent": string(e.ContainingAnchorID), "pose": PoseJSON(e.LocalPose), "range": map[string]any{"kind": "bounded-sphere", "radius": e.Range.Radius()}, "revision": TokenJSON(signer.ElementRevision(e.Revision))}
}

func ElementReadJSON(signer coretoken.PublicSigner, read corecontract.ElementRead) map[string]any {
	if read.State == corecontract.ExistenceNotFound {
		return map[string]any{"state": "NOT_FOUND"}
	}
	return map[string]any{"state": "PRESENT", "record": ElementRecordJSON(signer, *read.Record), "attachment": AttachmentJSON(signer, read.Attachment)}
}

func AttachmentJSON(signer coretoken.PublicSigner, attachment corecontract.FacetAttachment) map[string]any {
	status := "NONE"
	if attachment.State == corecontract.FacetActive {
		status = "ACTIVE"
	}
	if attachment.State == corecontract.FacetDetached {
		status = "DETACHED"
	}
	if attachment.State == corecontract.FacetRetired {
		status = "RETIRED"
	}
	result := map[string]any{"state": status, "revision": TokenJSON(signer.AttachmentRevision(attachment.Revision))}
	if attachment.Key.Valid() {
		key := attachment.Key
		result["key"] = map[string]any{"core": string(key.Core), "facet": string(key.Facet), "element": string(key.Element), "proposal": TokenJSON(signer.FacetProposal(key, attachment.OriginalProposal))}
		if attachment.State == corecontract.FacetActive {
			result["nextHop"] = string(attachment.NextHop)
			result["contactRevision"] = TokenJSON(signer.FacetContact(attachment.NextHopRevision))
		}
	}
	return result
}
