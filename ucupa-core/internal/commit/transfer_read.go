package commit

import (
	"slices"
	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	elementfact "ucupa/ucupa-core/internal/element/fact"
	mountfact "ucupa/ucupa-core/internal/mount/fact"
	"ucupa/ucupa-core/internal/token"
	transferfact "ucupa/ucupa-core/internal/transfer/fact"
)

// SourceVisibilityLocked copies finite inactive identities and pending spatial
// envelopes and exact held parent relations only. It never decodes manifests
// or retains the control journal.
func (o *Owner) SourceVisibilityLocked() transferfact.Visibility {
	return sourceVisibility(o.state)
}

// ParentReadVisibilityLocked checks only currently occupied mount slots. The
// final disclosure check does not scan departures or retain transfer history.
func (o *Owner) ParentReadVisibilityLocked() transferfact.Visibility {
	return transferfact.NewVisibility(nil, nil, heldParentRelations(o.state), nil, nil)
}

func sourceVisibility(s *coreState) transferfact.Visibility {
	if s == nil {
		return transferfact.Visibility{}
	}
	fences := make([]transferfact.SourceFootprint, 0)
	for _, record := range s.transfers.records {
		if record.SourcePending() && record.Input.Profile != transferfact.RootSuccessor {
			fences = append(fences, transferfact.SourceFootprint{Boundary: record.Input.Boundary, Origin: record.Origin, FinalCut: record.FinalCut})
		}
	}
	inactive := make(map[values.ElementID]values.ElementID)
	for _, r := range s.transfers.targets {
		if !r.installed() || r.complete() {
			continue
		}
		boundary := r.receipt.Fence.Input.Boundary
		if r.carrier.Revision != 0 {
			boundary = r.carrier.Fence.Permit.Reservation.Input.Mount
		}
		for _, id := range r.members {
			inactive[id] = boundary
		}
		if r.receipt.Fence.Input.Profile == transferfact.SliceTransfer {
			fences = append(fences, transferfact.SourceFootprint{Boundary: boundary, Origin: r.receipt.Fence.Input.Final, FinalCut: r.installation.At})
		}
	}
	successors := make(map[values.ElementID]contract.TransitionTerminal)
	pending := make(map[values.ElementID]bool)
	for id := range s.elements.DepartureIDs() {
		departure, _ := s.elements.Departure(id)
		if r, ok := s.transfers.records[departure.Operation]; ok && r.Input.Key().Controller == departure.Controller {
			inactive[id] = r.Input.Boundary
			// Only a settled decision this owner finalized ends the old scope
			// provably; earlier own decisions still under way keep the queue
			// pending until their publication proves the terminal successor.
			if departure.Controller == s.authority && r.Input.Source == s.authority {
				if r.Progress == transferfact.IntentSettled {
					successors[id] = contract.TransitionTerminal{
						Operation: departure.Operation,
						Source:    r.Input.Source,
						Target:    r.Input.Target,
						FinalCut:  token.Serial(r.FinalCut),
						Settled:   true,
						NextHop:   r.Input.TargetAddress,
					}
				} else if r.Progress == transferfact.IntentFenced || r.Progress == transferfact.IntentCommitted {
					pending[id] = true
				}
			}
		} else if r, ok := s.transfers.carrierSources[transferfact.TransferKey{Controller: departure.Controller, Operation: departure.Operation}]; ok && r.Fence.Valid() {
			inactive[id] = r.Permit.Reservation.Input.Mount
		}
	}
	return transferfact.NewVisibility(inactive, fences, heldParentRelations(s), successors, pending)
}

func heldParentRelations(s *coreState) map[values.ElementID]mountfact.Record {
	if s == nil {
		return nil
	}
	parents := make(map[values.ElementID]mountfact.Record)
	current := mountSnapshotFactsFromState(s)
	for id, r := range s.mount.facts.Outgoings() {
		op, exists := s.mount.operations[r.Slot]
		c := op.Parent
		if r.Slot != "" && exists && c.ReleasedAt == (contract.CoreReadCut{}) && c.Hold.Input.Mount == id &&
			validateTransferParentFacts(current, r.Slot, op) == nil {
			parents[id] = r
		}
	}
	return parents
}

// CheckTransferReadLocked runs under the existing owner lock at disclosure.
// exact selects a point read; otherwise the read depends on the whole scope.
// Unrelated point reads do not expire just because a shared snapshot contains
// another transferred branch. No old context is silently redirected or widened.
func (o *Owner) CheckTransferReadLocked(elements elementfact.View, cut contract.CoreReadCut, scope, exact values.ElementID) error {
	if o == nil || o.ClosedLocked() || !o.AvailableLocked() {
		return contract.ErrUnavailable
	}
	s := o.state
	if !token.Valid(cut) || token.Scope(cut) != s.scope || token.Serial(cut) > s.cutSerial || !scope.Valid() {
		return contract.ErrReadCutExpired
	}
	// A parent move does not move this content, but invalidates its old entry
	// context. Checking this finite receipt never decodes the retained manifest.
	if c := s.currentEdgeClosure(); c.Revision != 0 {
		if token.Serial(cut) < token.Serial(c.At) {
			return contract.ErrReadCutExpired
		}
		return contract.ErrUnavailable
	}
	for _, r := range s.transfers.edges {
		c := r.closure
		if token.Serial(cut) < token.Serial(c.At) || r.installation.Revision != 0 && token.Serial(cut) < token.Serial(r.installation.At) {
			return contract.ErrReadCutExpired
		}
	}
	if f := s.currentCarrierFence(); f.Valid() {
		if token.Serial(cut) <= token.Serial(f.FinalCut) {
			return contract.ErrReadCutExpired
		}
		return contract.ErrUnavailable
	}
	// Returning later does not revive a pre-departure read context. A carrier
	// fence covers this owner's complete space, including an empty one.
	for _, r := range s.transfers.carrierSources {
		if r.Fence.Valid() && token.Serial(cut) <= token.Serial(r.Fence.FinalCut) {
			return contract.ErrReadCutExpired
		}
	}
	// Target reservations are private write obligations, not source departures
	// or active target facts. They must not leak through current/retained reads.
	for _, r := range s.transfers.targets {
		if !r.installed() || r.complete() || token.Serial(cut) < token.Serial(r.installedProof().Cut()) {
			continue
		}
		if r.reservesWholeSpace() || exact != "" && slices.Contains(r.members, exact) || exact == "" && slices.Contains(r.members, scope) {
			return contract.ErrUnavailable
		}
	}
	if _, departed := s.elements.Departure(scope); departed {
		return contract.ErrUnavailable
	}
	if exact != "" {
		if _, departed := s.elements.Departure(exact); departed {
			return contract.ErrUnavailable
		}
	}
	for _, record := range s.transfers.records {
		if !record.SourceClosed() || token.Serial(cut) > token.Serial(record.FinalCut) {
			continue
		}
		if exact != "" {
			if elementfact.Member(elements, exact, record.Input.Boundary) || elements.DescendsFrom(scope, record.Input.Boundary) {
				return contract.ErrReadCutExpired
			}
			continue
		}
		if elementfact.Member(elements, record.Input.Boundary, scope) || elements.DescendsFrom(scope, record.Input.Boundary) {
			return contract.ErrReadCutExpired
		}
	}
	return nil
}

// FacetSuccessorStatementLocked reads one element's durable successor
// statement under the existing owner lock. A false result is a proven exact
// absence: the element never departed under this owner's control, or its
// departure has no unique committed successor decision yet. It never decodes
// the retained manifest and discloses no content beyond the binding itself.
func (o *Owner) FacetSuccessorStatementLocked(element values.ElementID) (transferfact.SuccessorStatement, bool, error) {
	if o == nil || o.ClosedLocked() || !o.AvailableLocked() {
		return transferfact.SuccessorStatement{}, false, values.PublicUnavailable
	}
	if !element.Valid() {
		return transferfact.SuccessorStatement{}, false, contract.ErrInvalid
	}
	s := o.state
	departure, departed := s.elements.Departure(element)
	if !departed || departure.Controller != s.authority {
		return transferfact.SuccessorStatement{}, false, nil
	}
	record, ok := s.transfers.records[departure.Operation]
	if !ok || record.Input.Source != s.authority || record.Input.Key().Controller != departure.Controller {
		return transferfact.SuccessorStatement{}, false, nil
	}
	if record.Progress != transferfact.IntentCommitted && record.Progress != transferfact.IntentSettled {
		return transferfact.SuccessorStatement{}, false, nil
	}
	return transferfact.SuccessorStatement{
		Operation: departure.Operation,
		Source:    record.Input.Source,
		Target:    record.Input.Target,
		FinalCut:  record.FinalCut,
		Settled:   record.Progress == transferfact.IntentSettled,
	}, true, nil
}
