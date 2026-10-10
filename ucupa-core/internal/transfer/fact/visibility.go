package fact

import (
	"iter"
	"maps"
	"slices"

	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	mountfact "ucupa/ucupa-core/internal/mount/fact"
)

// SourceFootprint is finite read evidence for an unavailable transfer branch
// (a pending departure or an installed target awaiting relation completion). It
// contains no manifest, actor, target address, authorization, receipt or journal.
type SourceFootprint struct {
	Boundary values.ElementID
	Origin   contract.SpatialTuple
	FinalCut contract.CoreReadCut
}

// Visibility is an immutable read-only projection of current transfer effect.
// It is not a writable owner or permission to follow a successor.
type Visibility struct {
	inactive   map[values.ElementID]values.ElementID // member -> unavailable boundary, not operation ID
	fences     []SourceFootprint
	parents    map[values.ElementID]mountfact.Record // exact currently held, unchanged relationships only
	successors map[values.ElementID]contract.TransitionTerminal
	pending    map[values.ElementID]bool // departures under this owner's own fence-pending transfer decision
}

func NewVisibility(inactive map[values.ElementID]values.ElementID, fences []SourceFootprint, parents map[values.ElementID]mountfact.Record, successors map[values.ElementID]contract.TransitionTerminal, pending map[values.ElementID]bool) Visibility {
	return Visibility{inactive: maps.Clone(inactive), fences: slices.Clone(fences), parents: maps.Clone(parents), successors: maps.Clone(successors), pending: maps.Clone(pending)}
}

func (v Visibility) Inactive(id values.ElementID) bool             { _, ok := v.inactive[id]; return ok }
func (v Visibility) Boundary(id values.ElementID) values.ElementID { return v.inactive[id] }
func (v Visibility) Counts() (int, int, int)                       { return len(v.inactive), len(v.fences), len(v.parents) }

// Successor returns the settled terminal successor evidence bound to one newly
// unavailable element. It is proven by the decision that finalized the old
// scope and never invents a successor for an absent departure.
func (v Visibility) Successor(id values.ElementID) (contract.TransitionTerminal, bool) {
	terminal, ok := v.successors[id]
	return terminal, ok
}

// PendingDecision reports whether one newly unavailable element departed under
// this owner's own transfer decision that has fenced or committed but not yet
// settled. The old scope is not proven ended, but its loss is not final GAP
// evidence either; the next publication proves or loses it.
func (v Visibility) PendingDecision(id values.ElementID) bool { return v.pending[id] }

// OutgoingReadable distinguishes a promise to keep the current relationship
// from a control operation changing it. Exact matching includes the slot,
// admission, revision and contact. This is read evidence, not authorization or
// permission to mutate the mount; no actor, manifest or control journal escapes.
func (v Visibility) OutgoingReadable(id values.ElementID, r mountfact.Record) bool {
	if r.State != contract.MountActive || r.Key.MountAnchorUUID != id || r.Admission.MountKey != r.Key || !mountfact.ValidMountAdmissionKey(r.Admission) {
		return false
	}
	if r.Slot == "" {
		return true
	}
	held, ok := v.parents[id]
	return ok && held == r
}

func (v Visibility) InactiveIDs() iter.Seq2[values.ElementID, values.ElementID] {
	return maps.All(v.inactive)
}
func (v Visibility) Fences() iter.Seq[SourceFootprint] {
	return func(yield func(SourceFootprint) bool) {
		for _, fence := range v.fences {
			if !yield(fence) {
				return
			}
		}
	}
}
