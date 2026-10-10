package projection

import (
	"context"
	elementread "ucupa/ucupa-core/internal/element/read"
	mountfact "ucupa/ucupa-core/internal/mount/fact"
	"ucupa/ucupa-core/internal/query/federation/transcript"

	"ucupa/shared/values"
	elementfact "ucupa/ucupa-core/internal/element/fact"
)

// Commit the actual owner-local adjacency inspected by the oracle. This is an
// internal transcript: it must be salted with the finite read basis before it
// leaves Core. It neither copies a subtree nor stores a second spatial index.
func (s *Projection) traversal(ctx context.Context, scope values.ElementID, closure map[values.ElementID]struct{}) (transcript.Writer, error) {
	return s.parentTraversal(ctx, scope, closure, nil)
}

func (s *Projection) parentTraversal(ctx context.Context, scope values.ElementID, closure map[values.ElementID]struct{}, parents map[values.ElementID][32]byte) (transcript.Writer, error) {
	w := transcript.SHA256()
	w.Text("ucupa.core.local-query-traversal", string(scope))
	if _, exists := closure[scope]; !exists {
		return w, values.PublicUnavailable
	}
	adjacency, err := elementread.DirectAdjacency(ctx, s.elements, closure)
	if err != nil {
		return w, values.PublicUnavailable
	}
	ids := make([]values.ElementID, 0, len(closure))
	for id := range closure {
		ids = append(ids, id)
	}
	elementfact.SortIDs(ids)
	for _, parent := range ids {
		if ctx.Err() != nil {
			return w, values.PublicUnavailable
		}
		if _, exists := s.elements.Anchor(parent); !exists && !mountfact.IsExternalParent(s.mount, parent) {
			return w, values.PublicUnavailable
		}
		p := transcript.SHA256()
		p.Text("parent", string(parent))
		children := adjacency[parent]
		var after values.ElementID
		for {
			if ctx.Err() != nil {
				return w, values.PublicUnavailable
			}
			start, end := elementread.PageBounds(children, after, 256)
			p.Text("direct-page", string(after))
			p.Number(uint64(end - start))
			for _, id := range children[start:end] {
				if s.transfers.Inactive(id) {
					p.Text("transfer-pending", string(id))
					continue
				}
				if _, anchor := s.elements.Anchor(id); anchor {
					if _, visited := closure[id]; !visited {
						return w, values.PublicUnavailable
					}
					p.Text("anchor", string(id))
				} else {
					p.Text("entity", string(id))
				}
			}
			if end == len(children) {
				// Empty parents also have one complete page, not missing evidence.
				p.Text("complete")
				break
			}
			after = children[end-1]
			p.Text("continue", string(after))
		}
		var digest [32]byte
		copy(digest[:], p.Sum())
		w.Text(string(parent))
		w.Bytes(digest[:])
		if parents != nil {
			parents[parent] = digest
		}
	}
	return w, nil
}
