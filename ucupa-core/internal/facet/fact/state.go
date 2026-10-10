package fact

import (
	"encoding/binary"
	"maps"

	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	"ucupa/ucupa-core/internal/token"
)

// Record is a value copy of the Core-side relationship, never Facet content.
type Record struct {
	Attachment contract.FacetAttachment
	Pending    values.FacetAcceptanceKey
	NextHop    values.NextHop
	Contact    contract.FacetContactRevision
}

func (record Record) Result(cut contract.CoreReadCut, status contract.MutationStatus) contract.FacetAttachmentResult {
	return contract.FacetAttachmentResult{Status: status, Attachment: record.Attachment, Pending: record.Pending, NextHop: record.NextHop, NextHopRevision: record.Contact, OutcomeCut: cut}
}
func (record Record) Key() values.FacetAcceptanceKey {
	if record.Pending.Valid() {
		return record.Pending
	}
	if record.Attachment.State == contract.FacetActive {
		return record.Attachment.Key
	}
	return values.FacetAcceptanceKey{}
}

// predecessorEvidence reports whether the record originated from a completed
// Core transfer: the previous Core's acceptance key with its original signed
// proposal comparison. Local propose, activate and detach never set one.
func (record Record) predecessorEvidence() bool {
	a := record.Attachment
	return a.State == contract.FacetDetached && a.Key.Valid() &&
		a.OriginalProposal.Valid() && a.OriginalProposal.Owner == a.Key.Core && a.OriginalProposal.Kind == "attachment-revision"
}

// ReacceptancePending reports the open obligation of a completed Core
// transfer: one proposal to the same Facet that stored the content. The Facet
// may already hold its acceptance without the activation response, so
// ordinary cancellation, Element deletion and repeated Core transfer must
// not erase it; only an explicit final disposition releases it.
func (record Record) ReacceptancePending() bool {
	return record.Pending.Valid() && record.Pending.Facet == record.Attachment.Key.Facet && record.predecessorEvidence()
}

// RecoveryPending reports the open obligation of a local detach: one proposal
// to the same Facet of the same Core's revoked relationship. The Facet may
// already hold its acceptance without the activation response, so ordinary
// cancellation and Element deletion must not erase it either.
func (record Record) RecoveryPending() bool {
	return record.Pending.Valid() && record.Pending.Facet == record.Attachment.Key.Facet &&
		record.Attachment.State == contract.FacetDetached && record.Attachment.Key.Valid() && !record.predecessorEvidence()
}

// State owns only current relationship records. Operation receipts, journal,
// cut issuance, persistence and publication remain with the Core commit owner.
type State struct{ View }

func New() State {
	return State{View: View{records: make(map[values.ElementID]Record)}}
}
func (state State) Clone() State {
	return State{View: View{records: maps.Clone(state.View.records)}}
}
func (state State) Equal(other State) bool {
	return maps.Equal(state.View.records, other.View.records)
}

// ElementCreated/ElementDeleted run only on the owner's existing candidate,
// after the one Element decision. They issue no revision and claim no operation.
func (state *State) ElementCreated(id values.ElementID, cut contract.CoreReadCut) {
	state.View.records[id] = Record{Attachment: contract.FacetAttachment{State: contract.FacetNone, Revision: AttachmentRevisionForCut(id, cut)}}
}
func (state *State) ElementDeleted(id values.ElementID) { delete(state.View.records, id) }

// AttachmentRevisionForCut preserves the existing byte representation. The
// supplied cut is issued by the Core owner, not by this constructor.
func AttachmentRevisionForCut(id values.ElementID, cut contract.CoreReadCut) values.AttachmentRevision {
	var raw [76]byte
	scope := token.Scope(cut)
	copy(raw[:32], scope[:])
	copy(raw[32:68], []byte(id))
	binary.BigEndian.PutUint64(raw[68:], token.Serial(cut))
	revision, _ := values.NewAttachmentRevision(raw[:])
	return revision
}

// ProjectionBuilder assembles a bounded, read-only record set at the original
// query selection sites. It carries no operation or control history.
type ProjectionBuilder struct{ state State }

func NewProjectionBuilder() ProjectionBuilder { return ProjectionBuilder{state: New()} }
func (builder *ProjectionBuilder) Add(id values.ElementID, record Record) {
	builder.state.View.records[id] = record
}
func (builder *ProjectionBuilder) Build() State {
	state := builder.state
	builder.state = State{}
	return state
}
