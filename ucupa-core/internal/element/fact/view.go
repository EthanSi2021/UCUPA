package fact

import (
	"iter"

	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
)

// View reads the existing live Anchor and Entity tables. It has no claims,
// tombstones, mutation methods, or restoration entry points.
type View struct {
	anchors  map[values.ElementID]Anchor
	entities map[values.ElementID]Entity
}

// LiveCounts is the size projection for the two live fact tables.
type LiveCounts struct {
	Anchors  int
	Entities int
}

// Counts reports only live Anchor and Entity entries. State.Counts remains the
// four-table projection used by history and Store validation.
func (view View) Counts() LiveCounts {
	return LiveCounts{
		Anchors:  len(view.anchors),
		Entities: len(view.entities),
	}
}

// Anchors iterates Anchor values as copies keyed by their IDs.
func (view View) Anchors() iter.Seq2[values.ElementID, Anchor] {
	return func(yield func(values.ElementID, Anchor) bool) {
		for id, anchor := range view.anchors {
			if !yield(id, anchor) {
				return
			}
		}
	}
}

// Entities iterates Entity values as copies keyed by their IDs.
func (view View) Entities() iter.Seq2[values.ElementID, Entity] {
	return func(yield func(values.ElementID, Entity) bool) {
		for id, entity := range view.entities {
			if !yield(id, entity) {
				return
			}
		}
	}
}

// Anchor looks up a live Anchor.
func (view View) Anchor(id values.ElementID) (Anchor, bool) {
	anchor, ok := view.anchors[id]
	return anchor, ok
}

// Entity looks up a live Entity.
func (view View) Entity(id values.ElementID) (Entity, bool) {
	entity, ok := view.entities[id]
	return entity, ok
}

// Kind reports the live kind for an ID. Anchors retain the historical
// anchor-first precedence used by Core.
func (view View) Kind(id values.ElementID) contract.ElementKind {
	if _, ok := view.anchors[id]; ok {
		return contract.ElementAnchor
	}
	if _, ok := view.entities[id]; ok {
		return contract.ElementEntity
	}
	return 0
}

// ParentOf returns the direct parent of a live element.
func (view View) ParentOf(id values.ElementID) (values.ElementID, bool) {
	if anchor, ok := view.anchors[id]; ok {
		return anchor.Parent, true
	}
	if entity, ok := view.entities[id]; ok {
		return entity.Parent, true
	}
	return "", false
}

// RootExists reports whether any live anchor has no parent.
func (view View) RootExists() bool {
	for _, anchor := range view.anchors {
		if anchor.Root() {
			return true
		}
	}
	return false
}

// DescendsFrom reports whether id reaches ancestor through live anchors.
// Entity records are leaves for this relationship, matching the existing
// anchor-only descendant rule.
func (view View) DescendsFrom(id, ancestor values.ElementID) bool {
	for id != "" {
		if id == ancestor {
			return true
		}
		anchor, ok := view.anchors[id]
		if !ok {
			return false
		}
		id = anchor.Parent
	}
	return false
}

// Member retains Core's closure-membership rule for an Anchor or an Entity.
// Unlike DescendsFrom's anchor-only walk, an Entity starts from its parent.
// This is a structural predicate, not an authorization decision.
func Member(view View, id, boundary values.ElementID) bool {
	if entity, exists := view.Entity(id); exists {
		id = entity.Parent
	}
	return view.DescendsFrom(id, boundary)
}

// HasDirectContent reports whether a live anchor or entity directly names id
// as its parent.
func (view View) HasDirectContent(id values.ElementID) bool {
	for _, anchor := range view.anchors {
		if anchor.Parent == id {
			return true
		}
	}
	for _, entity := range view.entities {
		if entity.Parent == id {
			return true
		}
	}
	return false
}

// RangeContainsDirectContent reports whether every direct child of parent is
// contained by r. It does not inspect grandchildren.
func (view View) RangeContainsDirectContent(parent values.ElementID, r contract.BoundedSphere) (bool, error) {
	for _, anchor := range view.anchors {
		if anchor.Parent != parent {
			continue
		}
		child, bounded := anchor.Range.BoundedSphere()
		if !bounded {
			return false, contract.ErrStore
		}
		inside, err := TupleContainedByRange(r, contract.SpatialTuple{
			Parent:    parent,
			LocalPose: anchor.LocalPose,
			Range:     child,
		})
		if err != nil || !inside {
			return false, err
		}
	}
	for _, entity := range view.entities {
		if entity.Parent != parent {
			continue
		}
		inside, err := TupleContainedByRange(r, contract.SpatialTuple{
			Parent:    parent,
			LocalPose: entity.LocalPose,
			Range:     entity.Range,
		})
		if err != nil || !inside {
			return false, err
		}
	}
	return true, nil
}

// Contains reports whether child is contained by its local anchor parent or
// by the supplied admitted external parent.
func (view View) Contains(child contract.SpatialTuple, external ExternalParent) (bool, error) {
	if parent, ok := view.anchors[child.Parent]; ok {
		return TupleContainedBy(parent, child)
	}
	if external.Present && child.Parent == external.ID {
		return TupleContainedByRange(external.Range, child)
	}
	return false, contract.ErrStore
}
