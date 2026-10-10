package coresdk

import (
	"ucupa/shared/values"
	pb "ucupa/ucupa-core/bindings/go/protobuf"
)

// The source attests adjacency, but the consumer must still prove that the
// disclosed match path is covered by those statements. Prefixes above the
// source query scope belong to other independently captured source views.
func validateQueryParentPath(scope, origin, element string, kind pb.ElementKind, path []string, parents []*pb.ParentAdjacencyEvidence) error {
	if !values.ElementID(scope).Valid() || !values.ElementID(origin).Valid() || !values.ElementID(element).Valid() || (kind != pb.ElementKind_ANCHOR && kind != pb.ElementKind_ENTITY) {
		return badFederation()
	}
	if element == scope && kind == pb.ElementKind_ENTITY {
		return badFederation()
	}
	if element == origin {
		if len(path) != 0 || kind != pb.ElementKind_ANCHOR || scope != origin {
			return badFederation()
		}
	} else if len(path) == 0 || path[len(path)-1] != element {
		return badFederation()
	}
	seen := map[string]bool{origin: true}
	start := -1
	if scope == origin {
		start = 0
	}
	for i, id := range path {
		if !values.ElementID(id).Valid() || seen[id] {
			return badFederation()
		}
		seen[id] = true
		if id == scope {
			start = i
		}
	}
	if start < 0 {
		return badFederation()
	}
	covered := make(map[string]bool, len(parents))
	for _, p := range parents {
		if p != nil {
			covered[p.GetParent()] = true
		}
	}
	if !covered[scope] {
		return badFederation()
	}
	end := len(path)
	if kind == pb.ElementKind_ENTITY {
		end--
	}
	for _, id := range path[start:end] {
		if !covered[id] {
			return badFederation()
		}
	}
	return nil
}

func validatePublicQueryEvidence(result *pb.QueryRegionResult) error {
	if result.SourceGraph == nil {
		return nil
	}
	sources := make(map[string]*pb.PublicReadSource, len(result.SourceGraph.Sources))
	for _, s := range result.SourceGraph.Sources {
		sources[s.GetOwner()] = s
		if s.Coverage != nil && validateParentEvidence(s.Parents, s.GetQueryScope(), result.GetStatus() == pb.Coverage_COMPLETE) != nil {
			return badFederation()
		}
	}
	for _, m := range result.Matches {
		if m == nil || m.Revision == nil || m.Address == nil || m.Address.Down == nil || m.Address.GetUp() != 0 {
			return badFederation()
		}
		s := sources[m.Revision.GetOwner()]
		if s == nil || s.Coverage == nil || validateQueryParentPath(s.GetQueryScope(), m.Address.GetOrigin(), m.GetElement(), m.GetKind(), m.Address.Down.Items, s.Parents) != nil {
			return badFederation()
		}
	}
	return nil
}
