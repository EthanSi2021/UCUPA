package read

import (
	"context"
	"sort"

	"ucupa/shared/values"
	elementfact "ucupa/ucupa-core/internal/element/fact"
)

// DirectAdjacency is a temporary projection of one immutable owner cut, not a
// maintained index. List and discovery use the same direct-parent membership
// and UUID order. Callers retain responsibility for authorization/disclosure.

func DirectAdjacency(ctx context.Context, elements elementfact.View, parents map[values.ElementID]struct{}) (map[values.ElementID][]values.ElementID, error) {
	children := make(map[values.ElementID][]values.ElementID)
	add := func(id, parent values.ElementID) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if _, selected := parents[parent]; selected {
			children[parent] = append(children[parent], id)
		}
		return nil
	}
	for id, a := range elements.Anchors() {
		if err := add(id, a.Parent); err != nil {
			return nil, err
		}
	}
	for id, e := range elements.Entities() {
		if err := add(id, e.Parent); err != nil {
			return nil, err
		}
	}
	for _, ids := range children {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		elementfact.SortIDs(ids)
	}
	return children, ctx.Err()
}

// Canonical UUID text has the same order as UUID bytes. The caller validates
// the cursor's parent/cut and limit before invoking this private operation.
func PageBounds(ids []values.ElementID, after values.ElementID, limit uint16) (int, int) {
	start := 0
	if after != "" {
		start = sort.Search(len(ids), func(i int) bool { return ids[i] > after })
	}
	return start, min(start+int(limit), len(ids))
}
