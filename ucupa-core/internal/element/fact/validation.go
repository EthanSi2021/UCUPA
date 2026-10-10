package fact

import (
	"errors"
	"fmt"

	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	"ucupa/ucupa-core/internal/geometry"
	"ucupa/ucupa-core/internal/token"
)

// Validate proves the current Element facts against the owner-provided
// Revision scope/serial and, when present, the one external parent.
//
// State metadata and map initialization are validated by the aggregate owner;
// this method intentionally begins at the original claim/cardinality check.
func (state *State) Validate(scope [32]byte, revisionSerial uint64, external ExternalParent) error {
	return validateFacts(state.View, state.claimed, state.tombstones, state.departures, scope, revisionSerial, external)
}

// Validate checks only the live read projection. Its temporary key set is the
// same live-claim set used by retained reads before the projection split; no
// historical claims or tombstones are retained or consulted.
func (view View) Validate(scope [32]byte, revisionSerial uint64, external ExternalParent) error {
	claimed := make(map[values.ElementID]struct{}, len(view.anchors)+len(view.entities))
	for id := range view.anchors {
		claimed[id] = struct{}{}
	}
	for id := range view.entities {
		claimed[id] = struct{}{}
	}
	return validateFacts(view, claimed, nil, nil, scope, revisionSerial, external)
}

// Both full-state admission and subsequent snapshot reads use these exact
// record, revision, containment and cycle rules.
func validateFacts(view View, claimed map[values.ElementID]struct{}, tombstones map[values.ElementID]contract.Revision, departures map[values.ElementID]Departure, scope [32]byte, revisionSerial uint64, external ExternalParent) error {
	if len(claimed) != len(view.anchors)+len(view.entities)+len(tombstones)+len(departures) {
		return errors.New("claim set does not match current records and tombstones")
	}
	issuedCurrent := make(map[uint64]values.ElementID, len(claimed))
	rootCount := 0
	retiredRootCount := 0
	for id := range claimed {
		members := 0
		if anchor, exists := view.anchors[id]; exists {
			members++
			if anchor.ID != id || !ValidLocalRevision(scope, revisionSerial, id, anchor.Revision) {
				return fmt.Errorf("Anchor %q identity or Revision is invalid", id)
			}
			if anchor.Parent == "" {
				rootCount++
				if anchor.LocalPose != (contract.Pose{}) || anchor.Range != geometry.UnboundedRootRange() {
					return fmt.Errorf("root Anchor %q shape is invalid", id)
				}
			} else if !anchor.Parent.Valid() || !geometry.CanonicalPose(anchor.LocalPose) || !geometry.ValidRange(anchor.Range) || anchor.Range.Kind() != contract.RangeBoundedSphere {
				return fmt.Errorf("Anchor %q bounded record is invalid", id)
			}
			if err := rememberCurrentRevision(issuedCurrent, anchor.Revision); err != nil {
				return err
			}
		}
		if entity, exists := view.entities[id]; exists {
			members++
			if entity.ID != id || !ValidLocalRevision(scope, revisionSerial, id, entity.Revision) || !entity.Parent.Valid() || !geometry.CanonicalPose(entity.LocalPose) || !geometry.ValidSphere(entity.Range) {
				return fmt.Errorf("Entity %q record is invalid", id)
			}
			if err := rememberCurrentRevision(issuedCurrent, entity.Revision); err != nil {
				return err
			}
		}
		if revision, exists := tombstones[id]; exists {
			members++
			if !ValidLocalRevision(scope, revisionSerial, id, revision) {
				return fmt.Errorf("tombstone %q Revision is invalid", id)
			}
			if err := rememberCurrentRevision(issuedCurrent, revision); err != nil {
				return err
			}
		}
		if d, exists := departures[id]; exists {
			if d.Root {
				retiredRootCount++
			}
			members++
			if !d.Controller.Valid() || !ValidLocalRevision(scope, revisionSerial, id, d.Revision) || len(d.Operation) == 0 || len(d.Operation) > 128 {
				return errors.New("departure controller, Revision or operation is invalid")
			}
			if err := rememberCurrentRevision(issuedCurrent, d.Revision); err != nil {
				return err
			}
		}
		if members != 1 {
			return fmt.Errorf("claim %q has %d current representations", id, members)
		}
	}
	for id := range view.anchors {
		if _, exists := claimed[id]; !exists {
			return fmt.Errorf("Anchor %q is not permanently claimed", id)
		}
	}
	for id := range view.entities {
		if _, exists := claimed[id]; !exists {
			return fmt.Errorf("Entity %q is not permanently claimed", id)
		}
	}
	for id := range tombstones {
		if _, exists := claimed[id]; !exists {
			return fmt.Errorf("tombstone %q is not permanently claimed", id)
		}
	}
	for id := range departures {
		if _, exists := claimed[id]; !exists {
			return errors.New("departure is not permanently claimed")
		}
	}
	if retiredRootCount > 1 || retiredRootCount > 0 && (len(view.anchors) != 0 || len(view.entities) != 0 || external.Present) {
		return errors.New("retired root collides with live or external space")
	}
	if external.Present {
		externalClaimed := false
		if _, exists := claimed[external.ID]; exists {
			externalClaimed = true
		}
		if rootCount != 0 || externalClaimed || !geometry.ValidSphere(external.Range) {
			return errors.New("external mount parent is claimed, rooted, or has invalid admission")
		}
		if !external.Admitted && len(claimed) != 0 {
			return errors.New("external mount content exists before initial admission")
		}
	} else if len(view.anchors) == 0 {
		if len(view.entities) != 0 || len(claimed) != 0 && retiredRootCount != 1 {
			return errors.New("claims exist without a root Anchor")
		}
		return nil
	} else if rootCount != 1 {
		return fmt.Errorf("expected one root Anchor, found %d", rootCount)
	}
	for id, anchor := range view.anchors {
		if anchor.Parent == "" {
			continue
		}
		sphere, ok := anchor.Range.BoundedSphere()
		if !ok {
			return fmt.Errorf("Anchor %q has a non-bounded Range", id)
		}
		inside, err := view.Contains(contract.SpatialTuple{Parent: anchor.Parent, LocalPose: anchor.LocalPose, Range: sphere}, external)
		if err != nil || !inside {
			return fmt.Errorf("Anchor %q is not contained by its parent", id)
		}
	}
	for id, entity := range view.entities {
		inside, err := view.Contains(contract.SpatialTuple{Parent: entity.Parent, LocalPose: entity.LocalPose, Range: entity.Range}, external)
		if err != nil || !inside {
			return fmt.Errorf("Entity %q is not contained by its parent", id)
		}
	}
	return view.validateNoAnchorCycles(external)
}

func (view View) validateNoAnchorCycles(external ExternalParent) error {
	const (
		visiting byte = iota + 1
		validated
	)
	marks := make(map[values.ElementID]byte, len(view.anchors))
	for id := range view.anchors {
		if marks[id] == validated {
			continue
		}
		chain := make([]values.ElementID, 0, 4)
		current := id
		for current != "" {
			if external.Present && current == external.ID {
				break
			}
			switch marks[current] {
			case visiting:
				return fmt.Errorf("Anchor cycle includes %q", current)
			case validated:
				current = ""
				continue
			}
			marks[current] = visiting
			chain = append(chain, current)
			anchor, exists := view.anchors[current]
			if !exists {
				return fmt.Errorf("Anchor chain has a missing parent %q", current)
			}
			current = anchor.Parent
		}
		for _, current = range chain {
			marks[current] = validated
		}
	}
	return nil
}

// ValidLocalRevision preserves the old validLocalRevision rule while making
// the aggregate scope and current revision serial explicit inputs.
func ValidLocalRevision(scope [32]byte, revisionSerial uint64, id values.ElementID, revision contract.Revision) bool {
	return token.ValidRevision(revision) && token.RevisionScope(revision) == scope && token.RevisionElement(revision) == id && token.RevisionSerial(revision) <= revisionSerial
}

func rememberCurrentRevision(seen map[uint64]values.ElementID, revision contract.Revision) error {
	if previous, duplicate := seen[token.RevisionSerial(revision)]; duplicate {
		return fmt.Errorf("current records %q and %q share Revision serial %d", previous, token.RevisionElement(revision), token.RevisionSerial(revision))
	}
	seen[token.RevisionSerial(revision)] = token.RevisionElement(revision)
	return nil
}
