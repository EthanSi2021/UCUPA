package collect

import (
	"context"

	"ucupa/shared/values"
	corecontract "ucupa/ucupa-core/contract"
	federationcontract "ucupa/ucupa-core/contract/federation"
	"ucupa/ucupa-core/internal/geometry"
)

type node struct {
	part federationcontract.FederationReadConnection
	node federationcontract.FederationSpineNode
}

type location struct {
	nodes []node
	kind  corecontract.ElementKind
}

func (l location) id() values.ElementID { return l.nodes[len(l.nodes)-1].node.ID }

func (l location) part() federationcontract.FederationReadConnection {
	return l.nodes[len(l.nodes)-1].part
}

func (l location) path() []values.ElementID {
	p := make([]values.ElementID, 0, len(l.nodes)-1)
	for _, n := range l.nodes[1:] {
		p = append(p, n.node.ID)
	}
	return p
}

func (r *Read) resolve(ctx context.Context, address Address) (location, error) {
	if !address.Origin.Valid() || len(address.Down) > geometry.MaxExactProjectionEdges {
		return location{}, values.PublicInvalid
	}
	for _, id := range address.Down {
		if !id.Valid() {
			return location{}, values.PublicInvalid
		}
	}
	part := r.sources.parts[0]
	path := federationcontract.FederationPathRequest{Origin: address.Origin, Up: address.Up, Down: append([]values.ElementID(nil), address.Down...)}
	result := location{}
	for {
		r.required[part.Part().Basis.Value] = true
		page, err := part.Project(ctx, federationcontract.FederationProjectionRequest{Kind: "path", Path: &path}, nil, 1)
		if err != nil {
			return location{}, err
		}
		p := page.Path
		if p == nil || p.RemainingUp != 0 {
			return location{}, values.PublicInvalid
		}
		for _, n := range p.Spine {
			if len(result.nodes) > 0 && n.Parent != result.id() {
				return location{}, values.PublicInvalidContext
			}
			if len(result.nodes) >= geometry.MaxExactProjectionEdges+1 {
				return location{}, values.PublicResourceExhausted
			}
			result.nodes = append(result.nodes, node{part: part, node: n})
		}
		if len(result.nodes) == 0 || result.id() != p.Element {
			return location{}, values.PublicInvalidContext
		}
		result.kind = p.Kind
		if len(p.RemainingDown) == 0 {
			return result, nil
		}
		if p.Frontier == nil || p.Frontier.Relation.Incoming {
			return location{}, values.PublicUnavailable
		}
		part, err = r.follow(ctx, part, p.Frontier.Relation, result.path())
		if err != nil {
			return location{}, err
		}
		path = federationcontract.FederationPathRequest{Origin: part.Part().Boundary, Down: append([]values.ElementID(nil), p.RemainingDown...)}
	}
}

func (r *Read) follow(ctx context.Context, parent federationcontract.FederationReadConnection, edge federationcontract.FederationMountRelation, prefix []values.ElementID) (federationcontract.FederationReadConnection, error) {
	if len(prefix) > geometry.MaxExactProjectionEdges {
		return nil, values.PublicResourceExhausted
	}
	for _, part := range r.sources.parts {
		if part.Part().Basis.Owner != edge.Key.CarrierAuthority {
			continue
		}
		if !edgeMatches(edge, part.Part().Incoming) || edge.Key.ParentAuthority != parent.Part().Basis.Owner {
			return nil, values.PublicInvalidContext
		}
		old := r.prefix[part.Part().Basis.Value]
		if len(old) != len(prefix) {
			return nil, values.PublicInvalidContext
		}
		for i := range old {
			if old[i] != prefix[i] {
				return nil, values.PublicInvalidContext
			}
		}
		return part, nil
	}
	part, err := r.sources.follow(ctx, parent, edge)
	if err != nil {
		return nil, err
	}
	if err := r.charge(uint64(len(prefix))*64 + 4096); err != nil {
		return nil, err
	}
	r.prefix[part.Part().Basis.Value] = append([]values.ElementID(nil), prefix...)
	return part, nil
}

func (r *Read) fullPath(part federationcontract.FederationReadConnection, path []values.ElementID) ([]values.ElementID, error) {
	prefix := r.prefix[part.Part().Basis.Value]
	if len(prefix)+len(path) > geometry.MaxExactProjectionEdges {
		return nil, values.PublicResourceExhausted
	}
	return append(append([]values.ElementID(nil), prefix...), path...), nil
}
