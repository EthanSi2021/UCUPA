package fact

import (
	"slices"

	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	"ucupa/ucupa-core/internal/token"
)

// WithClosedEdges binds all and only the external boundaries to authenticated
// carrier holds. The caller supplies proofs fetched from those exact carriers;
// structural validity alone is not authentication. A hold permits private target
// admission, never active delegation or reopening a carrier's gate.
func (s Slice) WithClosedEdges(f EdgeFence, closures []EdgeClosure) (Slice, error) {
	source, target, _ := f.Owners()
	if !f.Valid() || source != s.source || f.Mount != s.boundary {
		return Slice{}, contract.ErrConflict
	}
	if s.Profile() == CarrierReplacement {
		if f.Carrier == (CarrierFence{}) || f.Carrier.FinalCut != s.cut || f.Carrier.Permit.Reservation.Input.Mount != s.boundary {
			return Slice{}, contract.ErrConflict
		}
	} else if f.Source == (SourceFence{}) || f.Source.FinalCut != s.cut || f.Source.Input.Boundary != s.boundary || f.Source.Input.Profile != s.Profile() {
		return Slice{}, contract.ErrConflict
	}
	if len(closures) > len(s.members) {
		return Slice{}, contract.ErrConflict
	}
	byMount := make(map[values.ElementID]EdgeClosure, len(closures))
	for _, c := range closures {
		if _, duplicate := byMount[c.Fence.Mount]; duplicate {
			return Slice{}, contract.ErrConflict
		}
		byMount[c.Fence.Mount] = c
	}
	closed := make([]EdgeClosure, 0, len(closures))
	for _, member := range s.members {
		if member.Mount.State == contract.MountNone {
			continue
		}
		c, present := byMount[member.ID()]
		if !present {
			return Slice{}, contract.ErrMountCoordinationRequired
		}
		exact := f
		exact.Mount = member.ID()
		rangeValue, err := exact.EdgeRange(member)
		previousRange, bounded := member.Anchor.Range.BoundedSphere()
		if err != nil || !bounded || c.Fence != exact || !c.Valid(member.Mount.Key.CarrierAuthority) ||
			c.Previous.Key != member.Mount.Admission || c.Previous.RangeValue != previousRange || c.Range != rangeValue ||
			token.Serial(c.Previous.DecisionCut) > token.MountRevisionSerial(member.Mount.Revision) {
			return Slice{}, contract.ErrConflict
		}
		closed = append(closed, c)
		delete(byMount, member.ID())
	}
	if len(byMount) != 0 {
		return Slice{}, contract.ErrConflict
	}
	s.edgeTarget, s.closedEdges = target, closed
	return s, nil
}

func (s Slice) ClosedEdges() []EdgeClosure { return slices.Clone(s.closedEdges) }

func (s Slice) edgesClosedFor(target values.FactAuthorityRef) bool {
	return !s.HasMounts() || s.edgeTarget == target
}
