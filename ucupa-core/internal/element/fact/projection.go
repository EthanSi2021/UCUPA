package fact

import (
	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
)

// AnchorsOnly returns a partial read state that aliases only the anchor
// table. The other tables are deliberately absent; this shape is for parent
// scope resolution and is not a complete fact state.
func (s State) AnchorsOnly() State {
	return State{View: View{anchors: s.View.anchors}}
}

// WithoutHistory returns a read state that shares live records while
// replacing historical claims with the current live claim set and an empty
// tombstone table. Full validation must happen before this method is used.
func (s State) WithoutHistory() State {
	claimed := make(map[values.ElementID]struct{}, len(s.View.anchors)+len(s.View.entities))
	for id := range s.View.anchors {
		claimed[id] = struct{}{}
	}
	for id := range s.View.entities {
		claimed[id] = struct{}{}
	}
	return State{
		View: View{
			anchors:  s.View.anchors,
			entities: s.View.entities,
		},
		claimed:    claimed,
		tombstones: make(map[values.ElementID]contract.Revision),
		departures: make(map[values.ElementID]Departure),
	}
}

// ProjectionBuilder assembles a filtered/combined live read projection. Its
// maps are private, it performs no validation or duplicate policy, and Build
// transfers the two map references without copying. It is not Store restore.
type ProjectionBuilder struct {
	anchors  map[values.ElementID]Anchor
	entities map[values.ElementID]Entity
}

// NewProjection returns an empty live-only projection builder with the same
// zero-capacity map behavior as the existing connected read paths.
func NewProjection() *ProjectionBuilder {
	return &ProjectionBuilder{
		anchors:  make(map[values.ElementID]Anchor),
		entities: make(map[values.ElementID]Entity),
	}
}

// AddAnchor adds or replaces the supplied ID in the builder. Duplicate
// handling, if any, remains the caller's responsibility.
func (b *ProjectionBuilder) AddAnchor(id values.ElementID, record Anchor) {
	b.anchors[id] = record
}

// AddEntity adds or replaces the supplied ID in the builder. Duplicate
// handling, if any, remains the caller's responsibility.
func (b *ProjectionBuilder) AddEntity(id values.ElementID, record Entity) {
	b.entities[id] = record
}

// Build returns a live-only state backed by the builder's two maps. Claims and
// tombstones remain nil. It consumes the builder; the resulting read records
// must remain immutable and are not a replacement for a complete owner state.
func (b *ProjectionBuilder) Build() State {
	state := State{View: View{anchors: b.anchors, entities: b.entities}}
	b.anchors, b.entities = nil, nil
	return state
}
