package scan

import (
	"bytes"
	"errors"

	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	elementfact "ucupa/ucupa-core/internal/element/fact"
	"ucupa/ucupa-core/internal/geometry"
	mountfact "ucupa/ucupa-core/internal/mount/fact"
	queryrequest "ucupa/ucupa-core/internal/query/request"
	transferfact "ucupa/ucupa-core/internal/transfer/fact"
)

// Coverage scans a validated immutable read set; it does not authorize or retain it.
func Coverage(elements elementfact.View, mounts mountfact.View, transfers transferfact.Visibility, scope [32]byte, query contract.RegionQuery, cut contract.CoreReadCut, limit uint16, continuation *contract.QueryContinuation, public bool) (contract.QueryPage, error) {
	if transfers.Inactive(query.StartAnchorID) || transfers.Inactive(query.BasisAnchorID) {
		return contract.QueryPage{}, contract.ErrUnavailable
	}
	closure, err := BoundaryClosure(elements, query.StartAnchorID, nil)
	if err != nil {
		return contract.QueryPage{}, contract.ErrStore
	}
	if _, inside := closure[query.BasisAnchorID]; !inside {
		// No distinction between missing, non-Anchor and outside-Scope Basis.
		return contract.QueryPage{}, contract.ErrInvalid
	}
	// Installed-but-unsettled members remain in the owner's facts for exact
	// relation matching. They are not yet query results or traversable mounts.
	for id := range closure {
		if transfers.Inactive(id) {
			delete(closure, id)
		}
	}
	if !public {
		if err := GuardLocalClosure(mounts, closure); err != nil {
			return contract.QueryPage{}, err
		}
	}
	projections := NewProjectionMemo(elements, query.StartAnchorID, nil)
	basis, err := projections.Anchor(query.BasisAnchorID)
	if err != nil {
		return contract.QueryPage{}, queryGeometryError(err)
	}
	work := &geometry.Work{}
	inverse := work.Inverse(basis.Pose)
	if work.Failed() || !geometry.PoseWithinWorkLimit(inverse) {
		return contract.QueryPage{}, contract.ErrUnrepresentable
	}
	queryRange, _ := contract.NewBoundedSphere(query.Sphere.Radius) // validated above
	ids := make([]values.ElementID, 0, len(closure)+elements.Counts().Entities)
	if query.Selector != contract.QueryEntities {
		for id := range closure {
			ids = append(ids, id)
		}
	}
	if query.Selector != contract.QueryAnchors {
		for id, entity := range elements.Entities() {
			if _, inside := closure[entity.Parent]; inside && !transfers.Inactive(id) {
				ids = append(ids, id)
			}
		}
	}
	elementfact.SortIDs(ids)
	page := contract.QueryPage{Cut: cut, Status: contract.QueryComplete, Matches: make([]contract.QueryMatch, 0)}
	if public {
		for id := range mounts.Outgoings() {
			if _, inside := closure[id]; !inside || !mountfact.HasContent(mounts, id) {
				continue
			}
			_, intersects, err := projections.Candidate(id, query, queryRange, inverse)
			if err != nil || intersects {
				page.Gap = contract.QueryUnresolvedCoverage
			}
		}
	}
	more := false
	for _, id := range ids {
		match, intersects, err := projections.Candidate(id, query, queryRange, inverse)
		if errors.Is(err, contract.ErrUnrepresentable) {
			page.Gap = contract.QueryExactWorkLimit
			continue
		}
		if err != nil {
			return contract.QueryPage{}, contract.ErrStore
		}
		if !intersects {
			continue
		}
		// The complete scan above/below this position still determines Gap.
		if continuation != nil && bytes.Compare(elementfact.UUIDBytes(id), elementfact.UUIDBytes(queryrequest.QueryLast(*(continuation)))) <= 0 {
			continue
		}
		if len(page.Matches) == int(limit) {
			more = true
			continue
		}
		match.Path, err = Path(elements, id, query.StartAnchorID)
		if err != nil {
			return contract.QueryPage{}, contract.ErrStore
		}
		page.Matches = append(page.Matches, match)
	}
	if more {
		page.Continuation = queryrequest.QueryContinuationFromParts("", scope, [32]byte{}, query, values.PrincipalRef{}, cut, page.Matches[len(page.Matches)-1].ElementID, 0, "", 0, nil)
	}
	if more || page.Gap != contract.QueryNoGap {
		page.Status = contract.QueryPartial
	}
	return page, nil
}

func queryGeometryError(err error) error {
	if errors.Is(err, contract.ErrUnrepresentable) {
		return contract.ErrUnrepresentable
	}
	return contract.ErrStore
}

func GuardLocalClosure(mounts mountfact.View, closure map[values.ElementID]struct{}) error {
	for id := range closure {
		if mountfact.HasContent(mounts, id) {
			return contract.ErrMountContextRequired
		}
	}
	return nil
}
