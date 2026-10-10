package fact

import (
	"iter"

	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
)

// State owns historical claims/tombstones/departures and embeds View for live records.
type State struct {
	View
	claimed    map[values.ElementID]struct{}
	tombstones map[values.ElementID]contract.Revision
	departures map[values.ElementID]Departure
}

// New returns an empty, initialized fact state.
func New() State {
	return State{
		View: View{
			anchors:  make(map[values.ElementID]Anchor),
			entities: make(map[values.ElementID]Entity),
		},
		claimed:    make(map[values.ElementID]struct{}),
		tombstones: make(map[values.ElementID]contract.Revision),
		departures: make(map[values.ElementID]Departure),
	}
}

// Initialized reports whether all fact tables have been allocated.
func (s State) Initialized() bool {
	return s.claimed != nil && s.View.anchors != nil &&
		s.View.entities != nil && s.tombstones != nil && s.departures != nil
}

func (s *State) ensure() {
	if s.claimed == nil {
		s.claimed = make(map[values.ElementID]struct{})
	}
	if s.View.anchors == nil {
		s.View.anchors = make(map[values.ElementID]Anchor)
	}
	if s.View.entities == nil {
		s.View.entities = make(map[values.ElementID]Entity)
	}
	if s.tombstones == nil {
		s.tombstones = make(map[values.ElementID]contract.Revision)
	}
	if s.departures == nil {
		s.departures = make(map[values.ElementID]Departure)
	}
}

// Clone returns a state with independent table storage and value records.
func (s State) Clone() State {
	clone := State{
		View: View{
			anchors:  make(map[values.ElementID]Anchor, len(s.View.anchors)),
			entities: make(map[values.ElementID]Entity, len(s.View.entities)),
		},
		claimed:    make(map[values.ElementID]struct{}, len(s.claimed)),
		tombstones: make(map[values.ElementID]contract.Revision, len(s.tombstones)),
		departures: make(map[values.ElementID]Departure, len(s.departures)),
	}
	for id := range s.claimed {
		clone.claimed[id] = struct{}{}
	}
	for id, anchor := range s.View.anchors {
		clone.View.anchors[id] = anchor
	}
	for id, entity := range s.View.entities {
		clone.View.entities[id] = entity
	}
	for id, revision := range s.tombstones {
		clone.tombstones[id] = revision
	}
	for id, departure := range s.departures {
		clone.departures[id] = departure
	}
	return clone
}

// Equal reports whether all fact tables are equal, preserving the
// history comparison's length-and-right-index semantics for malformed states.
func (s State) Equal(other State) bool {
	if len(s.claimed) != len(other.claimed) ||
		len(s.View.anchors) != len(other.View.anchors) ||
		len(s.View.entities) != len(other.View.entities) ||
		len(s.tombstones) != len(other.tombstones) || len(s.departures) != len(other.departures) {
		return false
	}
	for id := range s.claimed {
		if _, ok := other.claimed[id]; !ok {
			return false
		}
	}
	for id, anchor := range s.View.anchors {
		if other.View.anchors[id] != anchor {
			return false
		}
	}
	for id, entity := range s.View.entities {
		if other.View.entities[id] != entity {
			return false
		}
	}
	for id, revision := range s.tombstones {
		if other.tombstones[id] != revision {
			return false
		}
	}
	for id, departure := range s.departures {
		if other.departures[id] != departure {
			return false
		}
	}
	return true
}

// Counts is the compact size projection for the fact tables.
type Counts struct {
	Claims     int
	Anchors    int
	Entities   int
	Tombstones int
	Departures int
}

// Counts returns the number of entries in the fact tables.
func (s State) Counts() Counts {
	return Counts{
		Claims:     len(s.claimed),
		Anchors:    len(s.View.anchors),
		Entities:   len(s.View.entities),
		Tombstones: len(s.tombstones),
		Departures: len(s.departures),
	}
}

// Claims iterates claimed element IDs as values, without exposing the table.
func (s State) Claims() iter.Seq[values.ElementID] {
	return func(yield func(values.ElementID) bool) {
		for id := range s.claimed {
			if !yield(id) {
				return
			}
		}
	}
}

// Tombstones iterates tombstone revisions as value copies keyed by ID.
func (s State) Tombstones() iter.Seq2[values.ElementID, contract.Revision] {
	return func(yield func(values.ElementID, contract.Revision) bool) {
		for id, revision := range s.tombstones {
			if !yield(id, revision) {
				return
			}
		}
	}
}

// Claimed reports whether an element ID has ever been claimed.
func (s State) Claimed(id values.ElementID) bool {
	_, ok := s.claimed[id]
	return ok
}

// Tombstone looks up a deleted element revision.
func (s State) Tombstone(id values.ElementID) (contract.Revision, bool) {
	revision, ok := s.tombstones[id]
	return revision, ok
}

// RestoreAnchor inserts a decoded anchor without running full graph or
// canonical validation. The enclosing recovery flow performs those checks
// after all records have been restored.
func (s *State) RestoreAnchor(anchor Anchor) error {
	if s == nil {
		return contract.ErrStore
	}
	s.ensure()
	s.claimed[anchor.ID] = struct{}{}
	s.anchors[anchor.ID] = anchor
	return nil
}

// RestoreEntity inserts a decoded entity without running full graph or
// canonical validation.
func (s *State) RestoreEntity(entity Entity) error {
	if s == nil {
		return contract.ErrStore
	}
	s.ensure()
	s.claimed[entity.ID] = struct{}{}
	s.entities[entity.ID] = entity
	return nil
}

// RestoreTombstone inserts a decoded tombstone without normalizing its
// revision or validating the complete element graph.
func (s *State) RestoreTombstone(id values.ElementID, revision contract.Revision) error {
	if s == nil {
		return contract.ErrStore
	}
	s.ensure()
	s.claimed[id] = struct{}{}
	s.tombstones[id] = revision
	return nil
}
