package local

import (
	"strconv"
	"ucupa/shared/values"
	corecontract "ucupa/ucupa-core/contract"
	accessvalue "ucupa/ucupa-core/internal/access/value"
	commitpkg "ucupa/ucupa-core/internal/commit"
	queryrequest "ucupa/ucupa-core/internal/query/request"
	queryscan "ucupa/ucupa-core/internal/query/scan"
	coretoken "ucupa/ucupa-core/internal/token"
)

func publicQueryJSON(projection commitpkg.ReadView, signer coretoken.PublicSigner, boundary values.ElementID, request map[string]any, after values.ElementID, limit int) (map[string]any, values.ElementID, error) {
	start, err := publicAddressForViews(projection.Elements, projection.Mount, projection.Transfers, boundary, request["start"])
	if err != nil {
		return nil, "", err
	}
	basis, err := publicAddressForViews(projection.Elements, projection.Mount, projection.Transfers, boundary, request["basis"])
	if err != nil {
		return nil, "", err
	}
	depth := int64(0)
	if raw, ok := request["upDepth"]; ok {
		text, ok := raw.(string)
		if !ok {
			return nil, "", values.PublicInvalid
		}
		depth, err = values.ParsePublicUpDepth(text)
		if err != nil {
			return nil, "", err
		}
	}
	scope, actual := start, uint64(0)
	for {
		anchor, ok := projection.Elements.Anchor(scope)
		if !ok {
			return nil, "", values.PublicInvalid
		}
		if scope == boundary || depth >= 0 && actual == uint64(depth) {
			break
		}
		if !anchor.Parent.Valid() {
			return nil, "", values.PublicUnavailable
		}
		scope, actual = anchor.Parent, actual+1
	}
	if !projection.Elements.DescendsFrom(basis, scope) {
		return nil, "", values.PublicInvalid
	}
	sphere, err := accessvalue.Object(request["sphere"], "center", "radius")
	if err != nil {
		return nil, "", err
	}
	center, err := accessvalue.Vec3(sphere["center"])
	if err != nil {
		return nil, "", err
	}
	radius, ok := sphere["radius"].(float64)
	if !ok {
		return nil, "", values.PublicInvalid
	}
	selector := corecontract.QueryBoth
	switch request["selector"] {
	case "anchors":
		selector = corecontract.QueryAnchors
	case "entities":
		selector = corecontract.QueryEntities
	case "both", nil:
	default:
		return nil, "", values.PublicInvalid
	}
	query, valid := queryrequest.CanonicalRegionQuery(corecontract.RegionQuery{StartAnchorID: scope, BasisAnchorID: basis, Sphere: corecontract.QuerySphere{Center: center, Radius: radius}, Selector: selector})
	if !valid {
		return nil, "", values.PublicInvalid
	}
	var continuation *corecontract.QueryContinuation
	if after != "" {
		continuation = queryrequest.QueryContinuationFromParts("", [32]byte{}, [32]byte{}, corecontract.RegionQuery{}, values.PrincipalRef{}, corecontract.CoreReadCut{}, after, 0, "", 0, nil)
	}
	page, err := queryscan.Coverage(projection.Elements, projection.Mount, projection.Transfers, projection.Scope, query, projection.CurrentCut(), uint16(limit), continuation, true)
	if err != nil {
		return nil, "", err
	}
	matches := make([]any, 0, len(page.Matches))
	gap, err := queryscan.TransferGap(projection.Elements, projection.Transfers, query)
	if err != nil {
		return nil, "", err
	}
	if gap {
		page.Status, page.Gap = corecontract.QueryPartial, corecontract.QueryUnresolvedCoverage
	}
	for _, match := range page.Matches {
		path, err := queryscan.Path(projection.Elements, match.ElementID, boundary)
		if err != nil {
			return nil, "", err
		}
		kind := "anchor"
		if match.Kind == corecontract.ElementEntity {
			kind = "entity"
		}
		matches = append(matches, map[string]any{"element": string(match.ElementID), "kind": kind, "revision": accessvalue.TokenJSON(signer.ElementRevision(match.Revision)), "address": accessvalue.PathJSON(boundary, path)})
	}
	status := "COMPLETE"
	if page.Status == corecontract.QueryPartial {
		status = "PARTIAL"
	}
	result := map[string]any{"matches": matches, "scope": string(scope), "actualUpDepth": strconv.FormatUint(actual, 10), "status": status}
	if page.Gap != corecontract.QueryNoGap {
		gap := "UNRESOLVED_COVERAGE"
		if page.Gap == corecontract.QueryExactWorkLimit {
			gap = "EXACT_WORK_LIMIT"
		}
		result["gap"] = gap
	}
	last := values.ElementID("")
	if page.Continuation != nil {
		last = queryrequest.QueryLast(*(page.Continuation))
	}
	return result, last, nil
}
