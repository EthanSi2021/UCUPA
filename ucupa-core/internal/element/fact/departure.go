package fact

import (
	"iter"

	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
)

// Departure is a non-active historical claim, not permanent deletion. The
// aggregate's exact transfer journal owns successor and admission evidence.
// Merely presenting this value never permits importing an old UUID again.
type Departure struct {
	Controller values.FactAuthorityRef
	Revision   contract.Revision
	Operation  contract.OperationID
	Root       bool // retired root role, proved by the sole owner's transfer journal
}

// RetiredRoot is not a new root and cannot authorize CreateRoot or import.
// Full validation rejects more than one retired root or a live-root collision.
func (s State) RetiredRoot() (values.ElementID, bool) {
	for id, d := range s.departures {
		if d.Root {
			return id, true
		}
	}
	return "", false
}

func (s State) Departure(id values.ElementID) (Departure, bool) {
	d, ok := s.departures[id]
	return d, ok
}

// DepartureIDs projects only current non-active identities, not their historical
// revisions, operation IDs or permanent claims. The live Element View stays clean.
func (s State) DepartureIDs() iter.Seq[values.ElementID] {
	return func(yield func(values.ElementID) bool) {
		for id := range s.departures {
			if !yield(id) {
				return
			}
		}
	}
}

// Depart validates a complete non-root owner-local closure before changing any
// table. The sole aggregate owner supplies the already-fenced transfer ID;
// no new Revision is minted and old operation results are not removed.
func (s *State) Depart(ids []values.ElementID, controller values.FactAuthorityRef, operation contract.OperationID) error {
	return s.depart(ids, controller, operation, "")
}

// DepartRoot closes the complete active local space in the same candidate.
// Claimed identities and original results remain; this is not a fresh Store.
func (s *State) DepartRoot(ids []values.ElementID, controller values.FactAuthorityRef, operation contract.OperationID, root values.ElementID) error {
	if !root.Valid() {
		return contract.ErrInvalid
	}
	return s.depart(ids, controller, operation, root)
}

func (s *State) depart(ids []values.ElementID, controller values.FactAuthorityRef, operation contract.OperationID, root values.ElementID) error {
	if s == nil || !s.Initialized() || len(ids) == 0 || !controller.Valid() || len(operation) == 0 || len(operation) > 128 {
		return contract.ErrInvalid
	}
	selected := make(map[values.ElementID]contract.Revision, len(ids))
	if root != "" {
		a, ok := s.Anchor(root)
		if !ok || !a.Root() || len(ids) != len(s.anchors)+len(s.entities) {
			return contract.ErrConflict
		}
	}
	for _, id := range ids {
		if _, duplicate := selected[id]; duplicate {
			return contract.ErrConflict
		}
		if a, ok := s.Anchor(id); ok {
			if a.Root() && id != root {
				return contract.ErrConflict
			}
			selected[id] = a.Revision
		} else if e, ok := s.Entity(id); ok {
			selected[id] = e.Revision
		} else {
			return contract.ErrNotFound
		}
		if _, dead := s.tombstones[id]; dead {
			return contract.ErrStore
		}
		if _, gone := s.departures[id]; gone {
			return contract.ErrStore
		}
	}
	for id, a := range s.anchors {
		if _, parent := selected[a.Parent]; parent {
			if _, member := selected[id]; !member {
				return contract.ErrConflict
			}
		}
	}
	for id, e := range s.entities {
		if _, parent := selected[e.Parent]; parent {
			if _, member := selected[id]; !member {
				return contract.ErrConflict
			}
		}
	}
	for id, revision := range selected {
		delete(s.anchors, id)
		delete(s.entities, id)
		s.departures[id] = Departure{Controller: controller, Revision: revision, Operation: operation, Root: id == root}
	}
	return nil
}

// RestoreDeparture restores only decoded bytes, like the other Restore
// methods. Full claim, Revision and transfer-history validation remains with
// the existing aggregate load path; this is not an adoption entry point.
func (s *State) RestoreDeparture(id values.ElementID, departure Departure) error {
	if s == nil {
		return contract.ErrStore
	}
	s.ensure()
	s.claimed[id] = struct{}{}
	s.departures[id] = departure
	return nil
}

// ResumeDeparture removes only the matched inactive representation in an
// unpublished candidate. The aggregate must prove successor continuity first,
// install the fresh record and validate/publish once. Claimed identity and
// permanent deletion are never cleared by this operation.
func (s *State) ResumeDeparture(id values.ElementID, expected Departure) error {
	if s == nil || !s.Initialized() || !id.Valid() {
		return contract.ErrInvalid
	}
	if d, ok := s.departures[id]; !ok || d != expected || s.Kind(id) != 0 {
		return contract.ErrConflict
	}
	if _, dead := s.tombstones[id]; dead {
		return contract.ErrConflict
	}
	if _, claimed := s.claimed[id]; !claimed {
		return contract.ErrStore
	}
	delete(s.departures, id)
	return nil
}
