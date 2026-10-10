package coordinator

import (
	"ucupa/shared/values"
	corecontract "ucupa/ucupa-core/contract"
	federationcollect "ucupa/ucupa-core/internal/query/federation/collect"
)

// federatedSourceGraphJSON encodes the collector's finite disclosure value.
// It never receives the private source graph, source connections or mutex.
func federatedSourceGraphJSON(graph federationcollect.SourceGraph) map[string]any {
	nodes := make([]any, 0, len(graph.Sources))
	for _, source := range graph.Sources {
		mode := "CURRENT_VERIFIED"
		if source.ReadMode == corecontract.QueryRetained {
			mode = "RETAINED"
		}
		out := map[string]any{
			"owner":    string(source.Owner),
			"boundary": string(source.Boundary),
			"principal": map[string]any{
				"realm":     string(source.Subject.Realm),
				"principal": string(source.Subject.Principal),
			},
			"cut":           values.EncodePublicBytes(source.CutStamp[:]),
			"authorization": values.EncodePublicBytes(source.AuthorizationStamp[:]),
			"readMode":      mode,
		}
		if source.Parent != nil {
			out["parent"] = map[string]any{
				"owner":      string(source.Parent.Owner),
				"anchor":     string(source.Parent.Anchor),
				"delegation": values.EncodePublicBytes(source.Parent.Delegation[:]),
				"acceptance": values.EncodePublicBytes(source.Parent.Acceptance[:]),
			}
		}
		if source.Query != nil {
			out["queryScope"] = string(source.Query.Scope)
			out["coverage"] = values.EncodePublicBytes(source.Query.Coverage[:])
			parents := make([]any, 0, len(source.Query.Parents))
			for _, parent := range source.Query.Parents {
				parents = append(parents, map[string]any{
					"parent":    string(parent.Parent),
					"adjacency": values.EncodePublicBytes(parent.Adjacency[:]),
				})
			}
			out["parents"] = parents
		}
		nodes = append(nodes, out)
	}
	return map[string]any{
		"purpose":   values.EncodePublicBytes(graph.Purpose[:]),
		"operation": graph.Operation,
		"sources":   nodes,
	}
}
