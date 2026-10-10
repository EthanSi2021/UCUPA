package projection

import (
	federationvalue "ucupa/ucupa-core/internal/query/federation/value"

	"context"

	"ucupa/shared/values"
	corecontract "ucupa/ucupa-core/contract"
	elementfact "ucupa/ucupa-core/internal/element/fact"
	"ucupa/ucupa-core/internal/geometry"
	mountfact "ucupa/ucupa-core/internal/mount/fact"
)

func (s *Projection) Project(ctx context.Context, boundary values.ElementID, r federationvalue.FederationProjectionRequest, after values.ElementID, limit uint16) (federationvalue.FederationProjectionPage, values.ElementID, map[values.ElementID]mountfact.Record, error) {
	deps := make(map[values.ElementID]mountfact.Record)
	if ctx.Err() != nil || s.transfers.Inactive(boundary) || s.inactiveWithin(r.Element, boundary) {
		return federationvalue.FederationProjectionPage{}, "", nil, values.PublicUnavailable
	}
	switch r.Kind {
	case "path":
		if after != "" {
			return federationvalue.FederationProjectionPage{}, "", nil, values.PublicInvalidContext
		}
		path, err := s.path(boundary, *r.Path, deps)
		return federationvalue.FederationProjectionPage{Path: path, Complete: true}, "", deps, err
	case "read", "read-at":
		if after != "" || !elementfact.Member(s.elements, r.Element, boundary) {
			return federationvalue.FederationProjectionPage{}, "", nil, values.PublicInvalid
		}
		row, _, err := s.record(r.Element, boundary, true, 1<<20)
		return federationvalue.FederationProjectionPage{Records: []federationvalue.FederationRecord{row}, Complete: true}, "", deps, err
	case "list":
		return s.list(ctx, boundary, r.Element, after, limit, deps)
	case "query":
		return s.query(ctx, boundary, *r.Query, after, limit, deps)
	}
	return federationvalue.FederationProjectionPage{}, "", nil, values.PublicInvalid
}

func (s *Projection) path(boundary values.ElementID, r federationvalue.FederationPathRequest, deps map[values.ElementID]mountfact.Record) (*federationvalue.FederationPathResult, error) {
	if s.inactiveWithin(r.Origin, boundary) {
		return nil, values.PublicUnavailable
	}
	if !s.elements.DescendsFrom(r.Origin, boundary) || (s.elements.Kind(r.Origin) != corecontract.ElementAnchor && r.Origin != boundary) {
		return nil, values.PublicInvalid
	}
	result := &federationvalue.FederationPathResult{Element: r.Origin, Kind: corecontract.ElementAnchor, RemainingUp: r.Up, RemainingDown: append([]values.ElementID(nil), r.Down...)}
	for result.RemainingUp > 0 && result.Element != boundary {
		a, ok := s.elements.Anchor(result.Element)
		if !ok || !a.Parent.Valid() {
			return nil, values.PublicUnavailable
		}
		result.Element = a.Parent
		result.RemainingUp--
		result.ActualUp++
	}
	if result.RemainingUp != 0 {
		if mountfact.IsExternalParent(s.mount, boundary) {
			in := s.incoming()
			result.Frontier = &federationvalue.FederationFrontier{Relation: in}
		}
	} else {
		for len(result.RemainingDown) > 0 {
			if mountfact.HasContent(s.mount, result.Element) {
				frontier, err := s.frontier(result.Element, boundary, deps)
				if err != nil {
					return nil, err
				}
				result.Frontier = &frontier
				break
			}
			next := result.RemainingDown[0]
			if a, ok := s.elements.Anchor(next); ok && a.Parent == result.Element {
				result.Kind = corecontract.ElementAnchor
			} else if e, ok := s.elements.Entity(next); ok && e.Parent == result.Element {
				result.Kind = corecontract.ElementEntity
			} else {
				for fence := range s.transfers.Fences() {
					if fence.Boundary == next && fence.Origin.Parent == result.Element {
						return nil, values.PublicUnavailable
					}
				}
				return nil, values.PublicNotFound
			}
			result.Element = next
			if s.transfers.Inactive(next) {
				return nil, values.PublicUnavailable
			}
			result.RemainingDown = result.RemainingDown[1:]
		}
	}
	spine, err := s.spine(boundary, result.Element)
	if err != nil {
		return nil, err
	}
	result.Spine = spine
	return result, nil
}

func (s *Projection) spine(boundary, id values.ElementID) ([]federationvalue.FederationSpineNode, error) {
	path, err := s.PathIDs(id, boundary, 4096)
	if err != nil || len(path) > 4096 {
		return nil, values.PublicResourceExhausted
	}
	ids := append([]values.ElementID{boundary}, path...)
	spine := make([]federationvalue.FederationSpineNode, 0, len(ids))
	for _, current := range ids {
		n := federationvalue.FederationSpineNode{ID: current}
		if a, ok := s.elements.Anchor(current); ok {
			n.Parent = a.Parent
			if !a.Root() {
				p := a.LocalPose
				n.Pose = &p
			}
		} else if e, ok := s.elements.Entity(current); ok {
			n.Parent = e.Parent
			p := e.LocalPose
			n.Pose = &p
		} else if current == boundary && mountfact.IsExternalParent(s.mount, boundary) {
			// Do not manufacture a second copy of the parent's mount Element.
			continue
		} else {
			return nil, values.PublicUnavailable
		}
		spine = append(spine, n)
	}
	return spine, nil
}

func (s *Projection) frontier(id, scope values.ElementID, deps map[values.ElementID]mountfact.Record) (federationvalue.FederationFrontier, error) {
	m, ok := s.mount.Outgoing(id)
	if !ok || !s.transfers.OutgoingReadable(id, m) {
		return federationvalue.FederationFrontier{}, values.PublicUnavailable
	}
	path, err := s.PathIDs(id, scope, geometry.MaxExactProjectionEdges)
	if err != nil || len(path) > geometry.MaxExactProjectionEdges || len(deps) >= 256 {
		return federationvalue.FederationFrontier{}, values.PublicResourceExhausted
	}
	steps := make([]federationvalue.FederationPoseStep, 0, len(path))
	for _, element := range path {
		a, ok := s.elements.Anchor(element)
		if !ok {
			return federationvalue.FederationFrontier{}, values.PublicUnavailable
		}
		steps = append(steps, federationvalue.FederationPoseStep{Pose: a.LocalPose})
	}
	deps[id] = m
	return federationvalue.FederationFrontier{Relation: s.outgoing(id), Path: path, ToScope: steps}, nil
}

func (s *Projection) list(ctx context.Context, boundary, parent, after values.ElementID, limit uint16, deps map[values.ElementID]mountfact.Record) (federationvalue.FederationProjectionPage, values.ElementID, map[values.ElementID]mountfact.Record, error) {
	if !s.elements.DescendsFrom(parent, boundary) || s.elements.Kind(parent) != corecontract.ElementAnchor && !mountfact.IsExternalParent(s.mount, parent) {
		return federationvalue.FederationProjectionPage{}, "", nil, values.PublicInvalid
	}
	if mountfact.HasContent(s.mount, parent) {
		f, err := s.frontier(parent, boundary, deps)
		return federationvalue.FederationProjectionPage{Frontiers: []federationvalue.FederationFrontier{f}, Complete: true}, "", deps, err
	}
	ids := make([]values.ElementID, 0)
	for id, a := range s.elements.Anchors() {
		if a.Parent == parent && id > after && !s.transfers.Inactive(id) {
			ids = append(ids, id)
		}
	}
	for id, e := range s.elements.Entities() {
		if e.Parent == parent && id > after && !s.transfers.Inactive(id) {
			ids = append(ids, id)
		}
	}
	elementfact.SortIDs(ids)
	page := federationvalue.FederationProjectionPage{Complete: len(ids) <= int(limit), Records: make([]federationvalue.FederationRecord, 0, min(len(ids), int(limit)))}
	remaining := uint64(1 << 20)
	for _, id := range ids[:min(len(ids), int(limit))] {
		if ctx.Err() != nil {
			return federationvalue.FederationProjectionPage{}, "", nil, values.PublicUnavailable
		}
		r, cost, err := s.record(id, boundary, false, remaining)
		if err != nil {
			return federationvalue.FederationProjectionPage{}, "", nil, err
		}
		remaining -= cost
		page.Records = append(page.Records, r)
	}
	var last values.ElementID
	if !page.Complete {
		last = ids[limit-1]
	}
	for fence := range s.transfers.Fences() {
		if fence.Origin.Parent == parent {
			page.Complete, page.Gap = false, corecontract.QueryUnresolvedCoverage
		}
	}
	return page, last, deps, nil
}
