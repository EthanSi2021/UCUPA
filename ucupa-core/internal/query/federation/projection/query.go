package projection

import (
	federationvalue "ucupa/ucupa-core/internal/query/federation/value"

	"context"
	"errors"

	"ucupa/shared/values"
	corecontract "ucupa/ucupa-core/contract"
	elementfact "ucupa/ucupa-core/internal/element/fact"
	"ucupa/ucupa-core/internal/geometry"
	mountfact "ucupa/ucupa-core/internal/mount/fact"
	queryscan "ucupa/ucupa-core/internal/query/scan"
)

func (s *Projection) query(ctx context.Context, boundary values.ElementID, request federationvalue.FederationQueryRequest, after values.ElementID, limit uint16, deps map[values.ElementID]mountfact.Record) (federationvalue.FederationProjectionPage, values.ElementID, map[values.ElementID]mountfact.Record, error) {
	if s.inactiveWithin(request.Scope, boundary) {
		return federationvalue.FederationProjectionPage{}, "", nil, values.PublicUnavailable
	}
	if !s.elements.DescendsFrom(request.Scope, boundary) || s.elements.Kind(request.Scope) != corecontract.ElementAnchor && !mountfact.IsExternalParent(s.mount, request.Scope) {
		return federationvalue.FederationProjectionPage{}, "", nil, values.PublicInvalid
	}
	closure, err := queryscan.BoundaryClosure(s.elements, request.Scope, nil)
	if err != nil {
		return federationvalue.FederationProjectionPage{}, "", nil, values.PublicUnavailable
	}
	for id := range closure {
		if s.transfers.Inactive(id) {
			delete(closure, id)
		}
	}
	parents := make(map[values.ElementID][32]byte, len(closure))
	transcript, err := s.parentTraversal(ctx, request.Scope, closure, parents)
	if err != nil {
		return federationvalue.FederationProjectionPage{}, "", nil, err
	}
	projections := queryscan.NewProjectionMemo(s.elements, request.Scope, nil)
	toBasis := geometry.ExactFromPose(geometry.IdentityPose())
	for _, step := range request.ToBasis {
		w := &geometry.Work{}
		p := geometry.ExactFromPose(step.Pose)
		if step.Inverse {
			p = w.Inverse(p)
		}
		toBasis = w.Compose(toBasis, p)
		if w.Failed() || !geometry.PoseWithinWorkLimit(toBasis) {
			return federationvalue.FederationProjectionPage{Complete: false, Gap: corecontract.QueryExactWorkLimit}, "", deps, nil
		}
	}
	query := corecontract.RegionQuery{StartAnchorID: request.Scope, BasisAnchorID: request.Scope, Sphere: request.Sphere, Selector: request.Selector}
	radius, _ := corecontract.NewBoundedSphere(request.Sphere.Radius)
	page := federationvalue.FederationProjectionPage{Complete: true, Matches: make([]federationvalue.FederationMatch, 0)}
	gap, err := queryscan.TransferGapProjected(s.elements, s.transfers, query, toBasis)
	if err != nil {
		return federationvalue.FederationProjectionPage{}, "", nil, ReadError(err)
	}
	if gap {
		page.Gap = corecontract.QueryUnresolvedCoverage
		transcript.Text("source-transfer-unresolved")
	}
	visibleParents := map[values.ElementID]bool{request.Scope: true}
	remaining := uint64(1 << 20)
	// The declared enclosing range decides whether a remote branch contributes.
	// An off-query unavailable mount is neither contacted nor a live dependency.
	mountIDs := make([]values.ElementID, 0, s.mount.OutgoingCount())
	for id := range s.mount.Outgoings() {
		mountIDs = append(mountIDs, id)
	}
	elementfact.SortIDs(mountIDs)
	for _, id := range mountIDs {
		if _, ok := closure[id]; !ok || !mountfact.HasContent(s.mount, id) {
			continue
		}
		if ctx.Err() != nil {
			return federationvalue.FederationProjectionPage{}, "", nil, values.PublicUnavailable
		}
		_, intersects, err := projections.Candidate(id, query, radius, toBasis)
		if errors.Is(err, corecontract.ErrUnrepresentable) {
			transcript.Text("mount-inexact", string(id))
			page.Gap = corecontract.QueryExactWorkLimit
			continue
		}
		if err != nil {
			return federationvalue.FederationProjectionPage{}, "", nil, values.PublicUnavailable
		}
		if !intersects {
			transcript.Text("mount-pruned", string(id))
			continue
		}
		f, err := s.frontier(id, request.Scope, deps)
		if err == values.PublicUnavailable {
			transcript.Text("mount-unresolved", string(id))
			if page.Gap == corecontract.QueryNoGap {
				page.Gap = corecontract.QueryUnresolvedCoverage
			}
			continue
		}
		if err != nil {
			return federationvalue.FederationProjectionPage{}, "", nil, err
		}
		transcript.Text("mount-follow", string(id))
		transcript.Relation(&f.Relation)
		cost := uint64(len(f.Path))*256 + 1024
		if cost > remaining {
			return federationvalue.FederationProjectionPage{}, "", nil, values.PublicResourceExhausted
		}
		remaining -= cost
		page.Frontiers = append(page.Frontiers, f)
	}
	ids := make([]values.ElementID, 0, len(closure)+s.elements.Counts().Entities)
	if request.Selector != corecontract.QueryEntities {
		for id := range closure {
			if _, real := s.elements.Anchor(id); real { // No synthetic carrier-boundary Element.
				ids = append(ids, id)
			}
		}
	}
	if request.Selector != corecontract.QueryAnchors {
		for id, e := range s.elements.Entities() {
			if _, ok := closure[e.Parent]; ok && !s.transfers.Inactive(id) {
				ids = append(ids, id)
			}
		}
	}
	elementfact.SortIDs(ids)
	more := false
	for _, id := range ids {
		if ctx.Err() != nil {
			return federationvalue.FederationProjectionPage{}, "", nil, values.PublicUnavailable
		}
		match, intersects, err := projections.Candidate(id, query, radius, toBasis)
		if errors.Is(err, corecontract.ErrUnrepresentable) {
			transcript.Text("candidate-inexact", string(id))
			page.Gap = corecontract.QueryExactWorkLimit
			continue
		}
		if err != nil {
			return federationvalue.FederationProjectionPage{}, "", nil, values.PublicUnavailable
		}
		transcript.Text("candidate", string(id))
		if intersects {
			transcript.Number(1)
		} else {
			transcript.Number(0)
		}
		if !intersects {
			continue
		}
		if err := s.collectQueryParents(id, request.Scope, visibleParents); err != nil {
			return federationvalue.FederationProjectionPage{}, "", nil, err
		}
		if id <= after {
			continue
		}
		if len(page.Matches) == int(limit) {
			more = true
			continue
		}
		path, err := s.PathIDs(id, boundary, min(4096, int(remaining/64)))
		if err != nil {
			return federationvalue.FederationProjectionPage{}, "", nil, err
		}
		remaining -= uint64(len(path)) * 64
		page.Matches = append(page.Matches, federationvalue.FederationMatch{ID: id, Kind: match.Kind, Revision: s.signer.ElementRevision(match.Revision), Path: path})
	}
	var last values.ElementID
	if more {
		last = page.Matches[len(page.Matches)-1].ID
	}
	page.Complete = !more && page.Gap == corecontract.QueryNoGap
	parentIDs := make([]values.ElementID, 0, len(visibleParents))
	for id := range visibleParents {
		parentIDs = append(parentIDs, id)
	}
	elementfact.SortIDs(parentIDs)
	for _, id := range parentIDs {
		digest, ok := parents[id]
		if !ok {
			return federationvalue.FederationProjectionPage{}, "", nil, values.PublicUnavailable
		}
		page.Parents = append(page.Parents, federationvalue.FederationParentEvidence{Parent: id, Adjacency: digest})
	}
	var traversal [32]byte
	copy(traversal[:], transcript.Sum())
	federationvalue.SetProjectionTraversal(&page, traversal)
	return page, last, deps, nil
}

func (s *Projection) collectQueryParents(id, scope values.ElementID, visible map[values.ElementID]bool) error {
	if e, ok := s.elements.Entity(id); ok {
		id = e.Parent
	}
	for id != scope {
		if visible[id] {
			return nil
		}
		if len(visible) >= 4096 {
			return values.PublicResourceExhausted
		}
		visible[id] = true
		a, ok := s.elements.Anchor(id)
		if !ok {
			return values.PublicUnavailable
		}
		id = a.Parent
	}
	return nil
}

// Count before allocating so a deeply nested origin cannot bypass admission.
func (s *Projection) PathIDs(id, boundary values.ElementID, maximum int) ([]values.ElementID, error) {
	cursor, depth := id, 0
	for cursor != boundary {
		depth++
		if depth > maximum {
			return nil, values.PublicResourceExhausted
		}
		if a, ok := s.elements.Anchor(cursor); ok {
			cursor = a.Parent
		} else if e, ok := s.elements.Entity(cursor); ok {
			cursor = e.Parent
		} else {
			return nil, values.PublicUnavailable
		}
	}
	return queryscan.Path(s.elements, id, boundary)
}
