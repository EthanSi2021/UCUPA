package coresdk

import (
	"ucupa/shared/values"
	pb "ucupa/ucupa-core/bindings/go/protobuf"
)

// Validate disclosure against the requested operation, independently of the
// server implementation. A Query reply cannot smuggle full records or content.
func validateFederationProjection(q *pb.FederationProjectionRequest, p *pb.FederationProjectionPage, part *pb.FederationReadPart, owner values.FactAuthorityRef) error {
	if q == nil {
		return badFederation()
	}
	if q.GetKind() == "query" {
		if len(p.Coverage) != 32 || [32]byte(p.Coverage) == ([32]byte{}) {
			return badFederation()
		}
		if q.Query == nil || validateParentEvidence(p.Parents, q.Query.GetScope(), p.GetGap() == 0) != nil {
			return badFederation()
		}
	} else if p.Coverage != nil || len(p.Parents) != 0 {
		return badFederation()
	}
	switch q.GetKind() {
	case "path":
		if q.Path == nil || p.Path == nil || len(p.Records) != 0 || len(p.Matches) != 0 || len(p.Frontiers) != 0 || !p.GetComplete() || p.GetGap() != 0 || p.Continuation != nil {
			return badFederation()
		}
		return validateFederationPathProjection(q.Path, p.Path, part, owner)
	case "read", "read-at":
		if p.Path != nil || len(p.Matches) != 0 || len(p.Frontiers) != 0 || len(p.Records) != 1 || p.Records[0].GetElement() != q.GetElement() || p.Records[0].Attachment == nil || p.Records[0].Mount != nil || !p.GetComplete() || p.GetGap() != 0 {
			return badFederation()
		}
	case "list":
		if p.Path != nil || len(p.Matches) != 0 || p.GetGap() != 0 || len(p.Frontiers) > 1 {
			return badFederation()
		}
		for _, r := range p.Records {
			if r.GetParent() != q.GetElement() || r.Attachment != nil || r.Mount != nil {
				return badFederation()
			}
		}
		if len(p.Frontiers) == 1 && (len(p.Records) != 0 || !p.GetComplete() || p.Frontiers[0].GetRelation().GetKey().GetElement() != q.GetElement()) {
			return badFederation()
		}
	case "query":
		if q.Query == nil || p.Path != nil || len(p.Records) != 0 {
			return badFederation()
		}
		seen := map[string]bool{}
		for _, m := range p.Matches {
			if m == nil || !values.ElementID(m.GetElement()).Valid() || seen[m.GetElement()] || m.GetKind() < pb.ElementKind_ANCHOR || m.GetKind() > pb.ElementKind_ENTITY || m.Path == nil || len(m.Path.Items) > 4096 {
				return badFederation()
			}
			seen[m.GetElement()] = true
			if q.Query.GetSelector() == pb.Selector_ANCHORS && m.GetKind() != pb.ElementKind_ANCHOR || q.Query.GetSelector() == pb.Selector_ENTITIES && m.GetKind() != pb.ElementKind_ENTITY {
				return badFederation()
			}
			path := m.Path.Items
			if validateQueryParentPath(q.Query.GetScope(), part.GetBoundary(), m.GetElement(), m.GetKind(), path, p.Parents) != nil {
				return badFederation()
			}
			if m.GetElement() == part.GetBoundary() {
				if len(path) != 0 {
					return badFederation()
				}
			} else if len(path) == 0 || path[len(path)-1] != m.GetElement() {
				return badFederation()
			}
			for _, id := range path {
				if !values.ElementID(id).Valid() {
					return badFederation()
				}
			}
		}
	default:
		return badFederation()
	}
	for _, f := range p.Frontiers {
		if err := validateFederationFrontier(f, owner); err != nil {
			return err
		}
		if f.Relation.GetIncoming() {
			return badFederation()
		}
	}
	return nil
}

func validateFederationFrontier(f *pb.FederationFrontier, owner values.FactAuthorityRef) error {
	if f == nil || f.Path == nil || len(f.Path.Items) > 4096 || len(f.ToScope) > 1024 {
		return badFederation()
	}
	if err := validateFederationRelation(f.Relation, owner); err != nil {
		return err
	}
	if f.Relation.GetIncoming() && (len(f.Path.Items) != 0 || len(f.ToScope) != 0) {
		return badFederation()
	}
	for _, id := range f.Path.Items {
		if !values.ElementID(id).Valid() {
			return badFederation()
		}
	}
	for _, s := range f.ToScope {
		if s == nil || s.Pose == nil || !validFederationPose(s.Pose) {
			return badFederation()
		}
	}
	return nil
}

func validateFederationPathProjection(q *pb.FederationPathRequest, p *pb.FederationPathResult, part *pb.FederationReadPart, owner values.FactAuthorityRef) error {
	if !values.ElementID(p.GetElement()).Valid() || p.GetKind() < pb.ElementKind_ANCHOR || p.GetKind() > pb.ElementKind_ENTITY || p.GetActualUp() > q.GetUp() || p.GetRemainingUp() != q.GetUp()-p.GetActualUp() || p.RemainingDown == nil || q.Down == nil || len(p.RemainingDown.Items) > len(q.Down.Items) || len(p.Spine) > 4097 {
		return badFederation()
	}
	start := len(q.Down.Items) - len(p.RemainingDown.Items)
	for i, id := range p.RemainingDown.Items {
		if id != q.Down.Items[start+i] {
			return badFederation()
		}
	}
	if p.GetRemainingUp() > 0 && (p.GetElement() != part.GetBoundary() || start != 0) {
		return badFederation()
	}
	if p.Frontier != nil {
		if err := validateFederationFrontier(p.Frontier, owner); err != nil {
			return err
		}
		if p.Frontier.Relation.Key.GetElement() != p.GetElement() || p.Frontier.Relation.GetIncoming() && p.GetRemainingUp() == 0 || !p.Frontier.Relation.GetIncoming() && len(p.RemainingDown.Items) == 0 {
			return badFederation()
		}
	} else if len(p.RemainingDown.Items) > 0 && p.GetRemainingUp() == 0 {
		return badFederation()
	}
	seen := map[string]bool{}
	previous := part.GetBoundary()
	for i, n := range p.Spine {
		if n == nil || !values.ElementID(n.GetElement()).Valid() || seen[n.GetElement()] {
			return badFederation()
		}
		seen[n.GetElement()] = true
		if i > 0 || n.GetElement() != part.GetBoundary() {
			if n.GetParent() != previous || n.Pose == nil {
				return badFederation()
			}
		}
		if n.Pose != nil && !validFederationPose(n.Pose) {
			return badFederation()
		}
		previous = n.GetElement()
	}
	if previous != p.GetElement() {
		return badFederation()
	}
	return nil
}
