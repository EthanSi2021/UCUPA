package fact

import (
	"slices"

	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	elementfact "ucupa/ucupa-core/internal/element/fact"
	facetfact "ucupa/ucupa-core/internal/facet/fact"
	"ucupa/ucupa-core/internal/geometry"
	mountfact "ucupa/ucupa-core/internal/mount/fact"
)

// Placement is a read-only local observation, not a reservation. Accept must
// revalidate these dependencies and durably reserve them in the sole owner.
type Placement struct {
	target    values.FactAuthorityRef
	cut       contract.CoreReadCut
	final     contract.SpatialTuple
	ancestors []elementfact.Anchor
}

func (p Placement) Target() values.FactAuthorityRef { return p.target }
func (p Placement) Cut() contract.CoreReadCut       { return p.cut }
func (p Placement) Final() contract.SpatialTuple    { return p.final }
func (p Placement) Ancestors() []elementfact.Anchor { return slices.Clone(p.ancestors) }

// Place checks target placement using validated current target facts.
// It never applies the source Revision, creates elements, or edits the supplied
// tuple. Returning claims require an explicit sole-owner continuity decision;
// a departure alone is never treated as ordinary creation.
func Place(source Slice, target values.FactAuthorityRef, cut contract.CoreReadCut,
	elements elementfact.State, mounts mountfact.View, final contract.SpatialTuple, maxRecords int, returns map[values.ElementID]elementfact.Departure) (Placement, error) {
	if source.Profile() == CarrierReplacement || !source.edgesClosedFor(target) {
		return Placement{}, contract.ErrMountCoordinationRequired
	}
	if !source.source.Valid() || len(source.members) == 0 || !target.Valid() || cut == (contract.CoreReadCut{}) || maxRecords <= 0 ||
		(source.Profile() != RootSuccessor && (!final.Parent.Valid() || !geometry.CanonicalPose(final.LocalPose) || !geometry.ValidSphere(final.Range))) {
		return Placement{}, contract.ErrInvalid
	}
	if source.source == target {
		return Placement{}, contract.ErrConflict
	}
	incoming := mounts.Incoming()
	var external elementfact.ExternalParent
	if incoming.Key.Valid() {
		if source.Profile() != SliceTransfer || incoming.Key.CarrierAuthority != target || incoming.Gate != mountfact.GateOpen ||
			incoming.Detached || incoming.Preparing != (contract.MountAttemptKey{}) ||
			!mountfact.ValidMountAdmissionKey(incoming.Admission.Key) || incoming.Admission.Key.MountKey != incoming.Key {
			return Placement{}, contract.ErrMountCoordinationRequired
		}
		external = elementfact.ExternalParent{ID: incoming.Key.MountAnchorUUID, Range: incoming.Admission.RangeValue, Present: true, Admitted: true}
	}
	if len(source.members) > maxRecords {
		return Placement{}, values.PublicResourceExhausted
	}
	if err := checkInstallClaims(source, elements, returns); err != nil {
		return Placement{}, err
	}
	var boundary Member
	for _, member := range source.members {
		if external.Present && member.ID() == external.ID {
			return Placement{}, contract.ErrConflict
		}
		// Preserve a validated former relation without activating its old key.
		if _, err := facetfact.TransferPredecessor(member.Facet, member.ID(), cut); err != nil {
			return Placement{}, err
		}
		if member.ID() == source.boundary {
			boundary = member
		}
		if member.ID() == final.Parent {
			return Placement{}, contract.ErrConflict
		}
	}
	if source.Profile() == RootSuccessor {
		if final != (contract.SpatialTuple{}) || elements.View.Counts().Anchors != 0 || elements.View.Counts().Entities != 0 {
			return Placement{}, contract.ErrConflict
		}
		// A fresh receiver or exact inactive return is admissible, never a
		// dormant fragment of an unrelated former space.
		if retired, exists := elements.RetiredRoot(); exists && (retired != source.boundary || !returns[retired].Root) {
			return Placement{}, contract.ErrConflict
		}
		if !elements.Claimed(source.boundary) && elements.Counts().Claims != 0 {
			return Placement{}, contract.ErrConflict
		}
		return Placement{target: target, cut: cut}, nil
	}
	if _, retired := elements.RetiredRoot(); retired {
		return Placement{}, contract.ErrConflict
	}
	parent, ok := elements.Anchor(final.Parent)
	if !ok && (!external.Present || final.Parent != external.ID) {
		if elements.Kind(final.Parent) != 0 {
			return Placement{}, contract.ErrConflict
		}
		return Placement{}, contract.ErrNotFound
	}
	placement := Placement{target: target, cut: cut, final: final}
	seen := make(map[values.ElementID]bool)
	for id := final.Parent; ; {
		if external.Present && id == external.ID {
			break
		}
		if id == "" {
			if external.Present {
				return Placement{}, contract.ErrStore
			}
			break
		}
		if seen[id] {
			return Placement{}, contract.ErrStore
		}
		seen[id] = true
		anchor, exists := elements.Anchor(id)
		if !exists {
			return Placement{}, contract.ErrStore
		}
		if err := checkLocalRelation(mounts, id); err != nil {
			return Placement{}, err
		}
		if len(source.members)+len(placement.ancestors) >= maxRecords {
			return Placement{}, values.PublicResourceExhausted
		}
		placement.ancestors = append(placement.ancestors, anchor)
		id = anchor.Parent
	}
	var inside bool
	var err error
	if external.Present && final.Parent == external.ID {
		inside, err = elementfact.TupleContainedByRange(external.Range, final)
	} else {
		inside, err = elementfact.TupleContainedBy(parent, final)
	}
	if err != nil {
		return Placement{}, err
	}
	if !inside {
		return Placement{}, contract.ErrConflict
	}
	if boundary.Kind == contract.ElementAnchor {
		if err := containsChildren(source, final.Range); err != nil {
			return Placement{}, err
		}
	}
	return placement, nil
}

// Source facts already prove descendant containment. Changing the boundary
// range requires rechecking its direct children using the existing exact rule;
// descendants keep their original tuple, not an automatically adjusted pose.
func containsChildren(source Slice, r contract.BoundedSphere) error {
	for _, m := range source.members {
		var child contract.SpatialTuple
		if m.Kind == contract.ElementAnchor {
			if m.Anchor.Parent != source.boundary {
				continue
			}
			rangeValue, ok := m.Anchor.Range.BoundedSphere()
			if !ok {
				return contract.ErrStore
			}
			child = contract.SpatialTuple{Parent: source.boundary, LocalPose: m.Anchor.LocalPose, Range: rangeValue}
		} else {
			if m.Entity.Parent != source.boundary {
				continue
			}
			child = contract.SpatialTuple{Parent: source.boundary, LocalPose: m.Entity.LocalPose, Range: m.Entity.Range}
		}
		inside, err := elementfact.TupleContainedByRange(r, child)
		if err != nil {
			return err
		}
		if !inside {
			return contract.ErrConflict
		}
	}
	return nil
}
