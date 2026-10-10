package scan

import (
	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	elementfact "ucupa/ucupa-core/internal/element/fact"
	"ucupa/ucupa-core/internal/geometry"
)

type Projection struct {
	Pose  geometry.ExactPose
	Edges int
}

// Memo reads one caller-owned immutable fact view and caches projections only
// for the current scan. It neither copies mutable facts nor owns their lock.
type Memo struct {
	elements elementfact.View
	anchors  map[values.ElementID]Projection
	stats    *Stats
}

func NewProjectionMemo(
	elements elementfact.View,
	boundary values.ElementID,
	stats *Stats,
) *Memo {
	return &Memo{
		elements: elements, stats: stats,
		anchors: map[values.ElementID]Projection{
			boundary: {Pose: geometry.ExactFromPose(geometry.IdentityPose())},
		},
	}
}

func (memo *Memo) Anchor(id values.ElementID) (Projection, error) {
	if projection, resolved := memo.anchors[id]; resolved {
		return projection, nil
	}
	chain := make([]elementfact.Anchor, 0, 8)
	seen := make(map[values.ElementID]struct{})
	current := id
	for {
		if projection, resolved := memo.anchors[current]; resolved {
			for index := len(chain) - 1; index >= 0; index-- {
				anchor := chain[index]
				if projection.Edges >= geometry.MaxExactProjectionEdges {
					return Projection{}, contract.ErrUnrepresentable
				}
				work := &geometry.Work{}
				pose := work.Compose(projection.Pose, geometry.ExactFromPose(anchor.LocalPose))
				if work.Failed() || !geometry.PoseWithinWorkLimit(pose) {
					return Projection{}, contract.ErrUnrepresentable
				}
				projection = Projection{Pose: pose, Edges: projection.Edges + 1}
				memo.anchors[anchor.ID] = projection
				if memo.stats != nil {
					memo.stats.AnchorProjectionEdges++
				}
			}
			return memo.anchors[id], nil
		}
		if _, duplicate := seen[current]; duplicate {
			return Projection{}, contract.ErrStore
		}
		seen[current] = struct{}{}
		anchor, exists := memo.elements.Anchor(current)
		if !exists || anchor.Root() {
			return Projection{}, contract.ErrStore
		}
		chain = append(chain, anchor)
		current = anchor.Parent
	}
}

func (memo *Memo) Entity(entity elementfact.Entity) (Projection, error) {
	parent, err := memo.Anchor(entity.Parent)
	if err != nil {
		return Projection{}, err
	}
	if parent.Edges >= geometry.MaxExactProjectionEdges {
		return Projection{}, contract.ErrUnrepresentable
	}
	work := &geometry.Work{}
	pose := work.Compose(parent.Pose, geometry.ExactFromPose(entity.LocalPose))
	if work.Failed() || !geometry.PoseWithinWorkLimit(pose) {
		return Projection{}, contract.ErrUnrepresentable
	}
	if memo.stats != nil {
		memo.stats.EntityProjectionEdges++
	}
	return Projection{Pose: pose, Edges: parent.Edges + 1}, nil
}
