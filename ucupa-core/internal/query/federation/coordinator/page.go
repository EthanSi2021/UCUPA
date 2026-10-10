package coordinator

import (
	"strconv"

	"ucupa/shared/values"
	corecontract "ucupa/ucupa-core/contract"
	federationcontract "ucupa/ucupa-core/contract/federation"
	accessvalue "ucupa/ucupa-core/internal/access/value"
	federationcollect "ucupa/ucupa-core/internal/query/federation/collect"
)

// federatedPageJSON is the public representation adapter for one detached
// collector page. It cannot access the source graph or retained recipe state.
func federatedPageJSON(operation string, boundary values.ElementID, page federationcollect.Page) map[string]any {
	if operation == "read" {
		return map[string]any{
			"state":      "PRESENT",
			"record":     federatedRecordJSON(*page.Record),
			"attachment": federatedAttachmentJSON(*page.Record.Attachment),
		}
	}
	status := "COMPLETE"
	if page.Partial {
		status = "PARTIAL"
	}
	result := map[string]any{"status": status}
	if operation == "query" {
		matches := make([]any, 0, len(page.Matches))
		for _, match := range page.Matches {
			kind := "anchor"
			if match.Kind == corecontract.ElementEntity {
				kind = "entity"
			}
			matches = append(matches, map[string]any{
				"element":  string(match.ID),
				"kind":     kind,
				"revision": accessvalue.TokenJSON(match.Revision),
				"address":  accessvalue.PathJSON(boundary, match.Path),
			})
		}
		result["matches"] = matches
		result["scope"] = string(page.Scope)
		result["actualUpDepth"] = strconv.FormatUint(page.ActualUpDepth, 10)
	} else {
		records := make([]any, 0, len(page.Records))
		for _, record := range page.Records {
			encoded := federatedRecordJSON(record)
			encoded["address"] = accessvalue.PathJSON(boundary, record.Path)
			records = append(records, encoded)
		}
		result["records"] = records
	}
	if page.Gap != corecontract.QueryNoGap {
		result["gap"] = "UNRESOLVED_COVERAGE"
		if page.Gap == corecontract.QueryExactWorkLimit {
			result["gap"] = "EXACT_WORK_LIMIT"
		}
	}
	return result
}

func federatedRecordJSON(row federationcontract.FederationRecord) map[string]any {
	kind := "anchor"
	if row.Kind == corecontract.ElementEntity {
		kind = "entity"
	}
	result := map[string]any{"element": string(row.ID), "kind": kind, "revision": accessvalue.TokenJSON(row.Revision)}
	if sphere, bounded := row.Range.BoundedSphere(); bounded {
		result["range"] = map[string]any{"kind": "bounded-sphere", "radius": sphere.Radius()}
	} else {
		result["range"] = map[string]any{"kind": "unbounded-root"}
	}
	if row.Parent.Valid() {
		result["parent"] = string(row.Parent)
	}
	if row.Pose != nil {
		result["pose"] = accessvalue.PoseJSON(*row.Pose)
	}
	return result
}

func federatedAttachmentJSON(attachment corecontract.PublicFacetInspection) map[string]any {
	state := "NONE"
	if attachment.State == corecontract.FacetActive {
		state = "ACTIVE"
	} else if attachment.State == corecontract.FacetDetached {
		state = "DETACHED"
	} else if attachment.State == corecontract.FacetRetired {
		state = "RETIRED"
	}
	result := map[string]any{"state": state, "revision": accessvalue.TokenJSON(attachment.Revision)}
	if attachment.Target.Valid() {
		result["key"] = map[string]any{
			"core":     string(attachment.KeyCore),
			"facet":    string(attachment.Target),
			"element":  string(attachment.Element),
			"proposal": accessvalue.TokenJSON(attachment.Proposal),
		}
		if attachment.State == corecontract.FacetActive {
			result["nextHop"] = string(attachment.NextHop)
			result["contactRevision"] = accessvalue.TokenJSON(attachment.ContactRevision)
		}
	}
	return result
}
