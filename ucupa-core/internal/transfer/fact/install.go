package fact

import (
	"math"

	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	elementfact "ucupa/ucupa-core/internal/element/fact"
	"ucupa/ucupa-core/internal/geometry"
	"ucupa/ucupa-core/internal/token"
)

// Install installs a validated owner-local closure into the caller's
// unpublished candidate. The sole commit owner proves placement and the durable
// decision first, supplies its own issuance sequence, validates the whole graph
// and publishes once. No per-member operation/outcome or intermediate cut exists.
func Install(slice Slice, elements *elementfact.State, final contract.SpatialTuple, scope [32]byte, serial uint64, returns map[values.ElementID]elementfact.Departure) (uint64, error) {
	if elements == nil || scope == ([32]byte{}) || len(slice.members) == 0 && slice.Profile() != CarrierReplacement {
		return serial, contract.ErrInvalid
	}
	if slice.HasMounts() && !slice.edgeTarget.Valid() {
		return serial, contract.ErrMountCoordinationRequired
	}
	if uint64(len(slice.members)) > math.MaxUint64-serial {
		return serial, values.PublicResourceExhausted
	}
	if err := checkInstallClaims(slice, *elements, returns); err != nil {
		return serial, err
	}
	for _, m := range slice.members {
		if departure, returning := returns[m.ID()]; returning {
			if err := elements.ResumeDeparture(m.ID(), departure); err != nil {
				return serial, err
			}
		}
		serial++
		revision := token.RevisionFromParts(scope, m.ID(), serial)
		if m.Kind == contract.ElementAnchor {
			a := m.Anchor
			a.Revision = revision
			if m.ID() == slice.boundary {
				if slice.Profile() == RootSuccessor {
					a.Parent, a.LocalPose, a.Range = "", contract.Pose{}, geometry.UnboundedRootRange()
				} else {
					a.Parent, a.LocalPose, a.Range = final.Parent, final.LocalPose, geometry.BoundedRange(final.Range)
				}
			}
			if err := elements.RestoreAnchor(a); err != nil {
				return serial, err
			}
		} else {
			v := m.Entity
			v.Revision = revision
			if m.ID() == slice.boundary {
				v.Parent, v.LocalPose, v.Range = final.Parent, final.LocalPose, final.Range
			}
			if err := elements.RestoreEntity(v); err != nil {
				return serial, err
			}
		}
	}
	return serial, nil
}

// Returns is a sole-owner checked allowance, never inferred from Claim alone.
// Pure placement/assignment still reject stale departures and permanent deletes.
func checkInstallClaims(slice Slice, elements elementfact.State, returns map[values.ElementID]elementfact.Departure) error {
	matched := 0
	for _, member := range slice.members {
		id := member.ID()
		if member.Kind != contract.ElementAnchor && member.Kind != contract.ElementEntity {
			return contract.ErrConflict
		}
		if expected, returning := returns[id]; returning {
			departure, ok := elements.Departure(id)
			_, dead := elements.Tombstone(id)
			if !ok || departure != expected || dead || elements.Kind(id) != 0 || !elements.Claimed(id) {
				return contract.ErrConflict
			}
			matched++
		} else if elements.Claimed(id) {
			return contract.ErrConflict
		}
	}
	if matched != len(returns) {
		return contract.ErrConflict
	}
	return nil
}
