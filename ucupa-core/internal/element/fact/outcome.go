package fact

import (
	"ucupa/ucupa-core/contract"
	"ucupa/ucupa-core/internal/token"
)

// Result projects the original deterministic decision at the cut supplied by
// the committing owner. It neither issues that cut nor publishes any state.
// The validation and error precedence match the retained receipt boundary.
func (outcome Outcome) Result(cut contract.CoreReadCut) (contract.MutationResult, error) {
	result := contract.MutationResult{Status: outcome.Status, OutcomeCut: cut}
	switch outcome.Status {
	case contract.MutationCommitted, contract.MutationUnchanged:
		if outcome.Rejection != NoError {
			return contract.MutationResult{}, contract.ErrStore
		}
		switch outcome.Kind {
		case contract.ElementAnchor:
			record := outcome.Anchor.Record()
			result.Record = &record
		case contract.ElementEntity:
			record := outcome.Entity.Record()
			result.Record = &record
		case 0:
			if outcome.Status != contract.MutationCommitted || !token.ValidRevision(outcome.Tombstone) {
				return contract.MutationResult{}, contract.ErrStore
			}
			result.TombstoneRevision = outcome.Tombstone
		default:
			return contract.MutationResult{}, contract.ErrStore
		}
	case contract.MutationRejected:
		if outcome.Rejection == NoError || outcome.Kind != 0 || token.ValidRevision(outcome.Tombstone) {
			return contract.MutationResult{}, contract.ErrStore
		}
	default:
		return contract.MutationResult{}, contract.ErrStore
	}
	if !token.Valid(cut) {
		return contract.MutationResult{}, contract.ErrStore
	}
	switch outcome.Rejection {
	case NoError:
		return result, nil
	case NotFound:
		return result, contract.ErrNotFound
	case Conflict:
		return result, contract.ErrConflict
	case RevisionConflict:
		return result, contract.ErrRevisionConflict
	default:
		return contract.MutationResult{}, contract.ErrStore
	}
}
