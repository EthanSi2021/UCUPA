package scan

import (
	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	elementfact "ucupa/ucupa-core/internal/element/fact"
)

// Stats is internal test-only evidence that a scan does not rediscover the
// same parent chain for every Element. It deliberately records work performed
// by the linear traversal/projection helpers instead of wall-clock time.
type Stats struct {
	AnchorRecordsScanned  int
	EntityRecordsScanned  int
	ClosureAnchorsVisited int
	AnchorProjectionEdges int
	EntityProjectionEdges int
}

func BoundaryClosure(
	elements elementfact.View,
	boundary values.ElementID,
	stats *Stats,
) (map[values.ElementID]struct{}, error) {
	children := make(map[values.ElementID][]values.ElementID, elements.Counts().Anchors)
	for id, anchor := range elements.Anchors() {
		if stats != nil {
			stats.AnchorRecordsScanned++
		}
		if id != boundary {
			children[anchor.Parent] = append(children[anchor.Parent], id)
		}
	}
	closure := map[values.ElementID]struct{}{boundary: {}}
	if stats != nil {
		stats.ClosureAnchorsVisited = 1
	}
	queue := []values.ElementID{boundary}
	for len(queue) != 0 {
		parent := queue[0]
		queue = queue[1:]
		for _, child := range children[parent] {
			if _, duplicate := closure[child]; duplicate {
				return nil, contract.ErrStore
			}
			closure[child] = struct{}{}
			queue = append(queue, child)
			if stats != nil {
				stats.ClosureAnchorsVisited++
			}
		}
	}
	return closure, nil
}
