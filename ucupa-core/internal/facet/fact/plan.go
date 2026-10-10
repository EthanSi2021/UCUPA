package fact

import (
	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	"ucupa/ucupa-core/internal/token"
)

// Plan is an ephemeral decision over the same candidate, not a retained state
// or permission. Apply must follow it without intervening candidate changes.
type Plan struct {
	binding Binding
	record  Record
	status  contract.MutationStatus
	code    Code
}

func (plan Plan) Changes() bool { return plan.status == contract.MutationCommitted }
func (plan Plan) Code() Code    { return plan.code }
func (plan Plan) Result(cut contract.CoreReadCut) contract.FacetAttachmentResult {
	if plan.status == contract.MutationRejected {
		return contract.FacetAttachmentResult{Status: contract.MutationRejected, OutcomeCut: cut}
	}
	return plan.record.Result(cut, plan.status)
}

// Plan preserves the original pre-I/O and replay decision order. The Core
// supplies only the already-selected Element existence fact and the recorded
// external evidence; no callback, authorization, Store, clock or network
// access occurs here.
func (state State) Plan(b Binding, evidence contract.FacetAcceptanceEvidence, disposition contract.FacetDispositionRead, elementExists bool) Plan {
	reject := func(code Code) Plan { return Plan{status: contract.MutationRejected, code: code} }
	if !elementExists {
		return reject(Missing)
	}
	record, ok := state.records[b.Element]
	if !ok {
		return reject(StoreFailure)
	}
	if b.Kind == Address {
		if record.Key() != b.Key {
			return reject(Changed)
		}
		if !record.NextHop.Valid() || !token.ValidFacetContactRevision(record.Contact) {
			return reject(StoreFailure)
		}
		if b.Contact != record.Contact {
			return reject(ContactConflict)
		}
		if record.NextHop == b.Address {
			return Plan{record: record, status: contract.MutationUnchanged}
		}
	} else {
		if record.Attachment.Revision != b.Expected {
			return reject(RevisionConflict)
		}
		switch b.Kind {
		case Propose:
			if record.Pending.Valid() {
				return reject(StateConflict)
			}
			// A DETACHED record kept the acceptance key of the relationship it
			// revoked — after a Core transfer with the previous Core's signed
			// comparison, after a local detach without one. Only the Facet of
			// that stored key may be (re-)proposed.
			if record.Attachment.State != contract.FacetNone &&
				!(record.Attachment.State == contract.FacetDetached && record.Attachment.Key.Facet == b.Target) {
				return reject(StateConflict)
			}
		case Cancel, Activate:
			if !record.Pending.Valid() || record.Pending.Proposal != b.Expected {
				return reject(ProposalNotCurrent)
			}
			if b.Kind == Cancel && (record.ReacceptancePending() || record.RecoveryPending()) {
				// The receiving Facet may already hold its acceptance without
				// the activation response; ordinary cancellation must not
				// erase the open transfer or recovery obligation.
				return reject(StateConflict)
			}
			if b.Kind == Activate {
				if evidence == (contract.FacetAcceptanceEvidence{}) {
					return reject(AcceptanceMissing)
				}
				if evidence.Key != record.Pending || !evidence.AcceptedAt.Valid() {
					return reject(StoreFailure)
				}
			}
		case Detach:
			if record.Attachment.State != contract.FacetActive {
				return reject(StateConflict)
			}
		case Release:
			// Only a revoked relationship's control occupancy may be released,
			// and only against the content owner's durable final disposition.
			// The disposition must name the stored lineage key or the open
			// proposal key: the retire transaction guarantees no acceptance of
			// either can ever appear at that Facet afterwards.
			if record.Attachment.State != contract.FacetDetached || b.Target != record.Attachment.Key.Facet {
				return reject(StateConflict)
			}
			if disposition == (contract.FacetDispositionRead{}) {
				return reject(DispositionMissing)
			}
			if !disposition.Found || !disposition.Key.Valid() ||
				(disposition.Key != record.Attachment.Key && disposition.Key != record.Pending) ||
				disposition.Facet != b.Target || disposition.Element != b.Element || !disposition.Cut.Valid() {
				return reject(StateConflict)
			}
		default:
			return reject(StoreFailure)
		}
	}
	return Plan{binding: b, record: record, status: contract.MutationCommitted}
}

// Apply uses the cut already issued under Core's original commit lock. It does
// not redecide, advance a counter, persist, or publish the candidate.
func (state *State) Apply(plan Plan, authority values.FactAuthorityRef, cut contract.CoreReadCut) contract.FacetAttachmentResult {
	if !plan.Changes() {
		return plan.Result(cut)
	}
	b, record := plan.binding, plan.record
	if b.Kind != Address {
		record.Attachment.Revision = AttachmentRevisionForCut(b.Element, cut)
	}
	switch b.Kind {
	case Propose:
		record.Pending = values.FacetAcceptanceKey{Core: authority, Facet: b.Target, Element: b.Element, Proposal: record.Attachment.Revision}
		record.NextHop = b.Address
		record.Contact = token.FacetContactRevisionFromParts(token.Scope(cut), record.Pending, token.Serial(cut))
	case Cancel:
		record.Pending, record.NextHop, record.Contact = values.FacetAcceptanceKey{}, "", contract.FacetContactRevision{}
	case Activate:
		record.Attachment.State, record.Attachment.Key = contract.FacetActive, record.Pending
		// Clear the predecessor comparison: its owner is the previous Core
		// and would invalidate the next transfer's proposal capture for
		// this key.
		record.Attachment.OriginalProposal = values.PublicToken{}
		record.Attachment.NextHop, record.Attachment.NextHopRevision = record.NextHop, record.Contact
		record.Pending = values.FacetAcceptanceKey{}
	case Detach:
		record.Attachment.State = contract.FacetDetached
		record.Attachment.NextHop, record.Attachment.NextHopRevision = "", contract.FacetContactRevision{}
		record.NextHop, record.Contact = "", contract.FacetContactRevision{}
	case Release:
		// The terminal release of the revoked relationship's control
		// occupancy: the former key stays as the permanent lineage record and
		// any open proposal obligation is discharged by the disposition.
		record.Attachment.State = contract.FacetRetired
		record.Pending, record.NextHop, record.Contact = values.FacetAcceptanceKey{}, "", contract.FacetContactRevision{}
	case Address:
		record.NextHop = b.Address
		record.Contact = token.FacetContactRevisionFromParts(token.Scope(cut), b.Key, token.Serial(cut))
		if record.Attachment.State == contract.FacetActive {
			record.Attachment.NextHop, record.Attachment.NextHopRevision = record.NextHop, record.Contact
		}
	}
	state.records[b.Element] = record
	return record.Result(cut, contract.MutationCommitted)
}
