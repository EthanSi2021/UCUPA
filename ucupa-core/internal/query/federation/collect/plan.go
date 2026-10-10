package collect

import (
	"context"
	"errors"
	"sort"

	"ucupa/shared/values"
	corecontract "ucupa/ucupa-core/contract"
	federationcontract "ucupa/ucupa-core/contract/federation"
	"ucupa/ucupa-core/internal/geometry"
	queryrequest "ucupa/ucupa-core/internal/query/request"
)

func (r *Read) planQuery(ctx context.Context, request Query) error {
	start, err := r.resolve(ctx, request.Start)
	if err != nil {
		return err
	}
	basis, err := r.resolve(ctx, request.Basis)
	if err != nil {
		return err
	}
	if start.kind != corecontract.ElementAnchor || basis.kind != corecontract.ElementAnchor {
		return values.PublicInvalid
	}
	index := 0
	if request.UpDepth >= 0 && uint64(request.UpDepth) < uint64(len(start.nodes)-1) {
		index = len(start.nodes) - 1 - int(request.UpDepth)
	}
	scope := start.nodes[index]
	r.scope, r.actual = scope.node.ID, uint64(len(start.nodes)-1-index)
	if len(basis.nodes) <= index {
		return values.PublicInvalid
	}
	for i := 0; i <= index; i++ {
		if basis.nodes[i].node.ID != start.nodes[i].node.ID || basis.nodes[i].part.Part().Basis != start.nodes[i].part.Part().Basis {
			return values.PublicInvalid
		}
	}
	q := federationcontract.FederationQueryRequest{Scope: r.scope, Sphere: request.Sphere, Selector: request.Selector}
	if _, valid := queryrequest.CanonicalRegionQuery(corecontract.RegionQuery{StartAnchorID: r.scope, BasisAnchorID: r.scope, Sphere: q.Sphere, Selector: q.Selector}); !valid {
		return values.PublicInvalid
	}
	// Invert the recipe in reverse order, not a rounded accumulated pose.
	for i := len(basis.nodes) - 1; i > index; i-- {
		if basis.nodes[i].node.Pose == nil {
			return values.PublicInvalidContext
		}
		q.ToBasis = append(q.ToBasis, federationcontract.FederationPoseStep{Pose: *basis.nodes[i].node.Pose, Inverse: true})
	}
	if err := r.charge(uint64(len(q.ToBasis))*128 + 2048); err != nil {
		return err
	}
	r.plans = []projection{{part: scope.part, request: federationcontract.FederationProjectionRequest{Kind: "query", Query: &q}}}
	seen := map[values.FactAuthorityRef]bool{scope.part.Part().Basis.Owner: true}
	for i := 0; i < len(r.plans); i++ {
		plan := r.plans[i]
		page, err := plan.part.Project(ctx, plan.request, nil, 1)
		if err != nil {
			if !r.required[plan.part.Part().Basis.Value] && (errors.Is(err, values.PublicUnavailable) || errors.Is(err, values.PublicDenied) || errors.Is(err, values.PublicInvalidContext)) {
				r.gap = corecontract.QueryUnresolvedCoverage
				r.discardSource(plan.part)
				r.plans = append(r.plans[:i], r.plans[i+1:]...)
				i--
				continue
			}
			return err
		}
		if page.Gap > r.gap {
			r.gap = page.Gap
		}
		if page.Coverage == ([32]byte{}) || !validParents(page.Parents, plan.request.Query.Scope, page.Gap == corecontract.QueryNoGap) {
			return values.PublicInvalidContext
		}
		r.plans[i].coverage = page.Coverage
		if err := r.charge(uint64(len(page.Parents)) * 128); err != nil {
			return err
		}
		r.plans[i].parents = append([]federationcontract.FederationParentEvidence(nil), page.Parents...)
		sort.Slice(page.Frontiers, func(i, j int) bool {
			return page.Frontiers[i].Relation.Key.MountAnchorUUID < page.Frontiers[j].Relation.Key.MountAnchorUUID
		})
		for _, frontier := range page.Frontiers {
			scopePath, err := r.scopePath(ctx, plan.part, plan.request.Query.Scope)
			if err != nil {
				return err
			}
			prefix := append(scopePath, frontier.Path...)
			part, err := r.follow(ctx, plan.part, frontier.Relation, prefix)
			if errors.Is(err, values.PublicUnavailable) || errors.Is(err, values.PublicDenied) || errors.Is(err, values.PublicInvalidContext) {
				r.gap = corecontract.QueryUnresolvedCoverage
				continue
			}
			if err != nil {
				return err
			}
			if seen[part.Part().Basis.Owner] {
				return values.PublicInvalidContext
			}
			seen[part.Part().Basis.Owner] = true
			if part.Part().Hidden {
				continue
			}
			next := *plan.request.Query
			if err := r.charge(uint64(len(next.ToBasis)+len(frontier.ToScope))*128 + 2048); err != nil {
				return err
			}
			next.Scope = part.Part().Boundary
			next.ToBasis = append(append([]federationcontract.FederationPoseStep(nil), next.ToBasis...), frontier.ToScope...)
			if len(next.ToBasis) > geometry.MaxExactProjectionEdges {
				return values.PublicResourceExhausted
			}
			r.plans = append(r.plans, projection{part: part, request: federationcontract.FederationProjectionRequest{Kind: "query", Query: &next}})
		}
	}
	return nil
}

func (r *Read) discardSource(part federationcontract.FederationReadConnection) {
	basis := part.Part().Basis
	for i, candidate := range r.sources.parts {
		if candidate.Part().Basis == basis {
			r.sources.parts = append(r.sources.parts[:i], r.sources.parts[i+1:]...)
			break
		}
	}
	delete(r.sources.owners, basis.Owner)
	delete(r.sources.graph, basis.Value)
	delete(r.prefix, basis.Value)
	_ = retirePart(part)
}

func (r *Read) scopePath(ctx context.Context, part federationcontract.FederationReadConnection, scope values.ElementID) ([]values.ElementID, error) {
	if scope == part.Part().Boundary {
		return append([]values.ElementID(nil), r.prefix[part.Part().Basis.Value]...), nil
	}
	p, err := part.Project(ctx, federationcontract.FederationProjectionRequest{Kind: "path", Path: &federationcontract.FederationPathRequest{Origin: scope}}, nil, 1)
	if err != nil {
		return nil, err
	}
	if p.Path == nil {
		return nil, values.PublicInvalidContext
	}
	ids := make([]values.ElementID, 0, len(p.Path.Spine))
	for _, n := range p.Path.Spine {
		if n.ID != part.Part().Boundary {
			ids = append(ids, n.ID)
		}
	}
	return r.fullPath(part, ids)
}

func (r *Read) planList(ctx context.Context) error {
	if r.target.kind != corecontract.ElementAnchor {
		return values.PublicInvalid
	}
	part, element := r.target.part(), r.target.id()
	for {
		request := federationcontract.FederationProjectionRequest{Kind: "list", Element: element}
		page, err := part.Project(ctx, request, nil, 1)
		if err != nil {
			return err
		}
		if len(page.Frontiers) == 0 {
			r.plans = []projection{{part: part, request: request}}
			return nil
		}
		if len(page.Frontiers) != 1 || len(page.Records) != 0 {
			return values.PublicInvalidContext
		}
		part, err = r.follow(ctx, part, page.Frontiers[0].Relation, r.target.path())
		if err != nil {
			return err
		}
		element = part.Part().Boundary
		if part.Part().Hidden {
			r.plans = nil
			return nil
		}
	}
}

func validParents(parents []federationcontract.FederationParentEvidence, scope values.ElementID, closed bool) bool {
	if !scope.Valid() || len(parents) > 4096 || closed && len(parents) == 0 {
		return false
	}
	found := false
	for i, parent := range parents {
		if !parent.Parent.Valid() || parent.Adjacency == ([32]byte{}) || i > 0 && parents[i-1].Parent >= parent.Parent {
			return false
		}
		found = found || parent.Parent == scope
	}
	return len(parents) == 0 || found
}
