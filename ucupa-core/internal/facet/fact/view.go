package fact

import (
	"iter"
	"maps"
	corecontract "ucupa/ucupa-core/contract"
	coretoken "ucupa/ucupa-core/internal/token"

	"ucupa/shared/values"
)

// View reads the current Facet relationship records. It has no Element
// lifecycle, Plan, Apply, clone, or equality operation.
type View struct {
	records map[values.ElementID]Record
}

// Record looks up one current relationship record and returns a value copy.
func (view View) Record(id values.ElementID) (Record, bool) {
	record, ok := view.records[id]
	return record, ok
}

// Records iterates current relationship records as value copies.
func (view View) Records() iter.Seq2[values.ElementID, Record] {
	return maps.All(view.records)
}

// PublicInspection projects the original bounded relationship with the supplied
// owner signer. It does not authorize, load newer facts or mutate the record.
func PublicInspection(signer coretoken.PublicSigner, id values.ElementID, r corecontract.FacetAttachmentResult) corecontract.PublicFacetInspection {
	x := corecontract.PublicFacetInspection{Element: id, State: r.Attachment.State, NextHop: r.NextHop, ContactRevision: signer.FacetContact(r.NextHopRevision)}
	if r.Attachment.Revision.Valid() {
		x.Revision = signer.AttachmentRevision(r.Attachment.Revision)
	}
	if r.Attachment.Key.Valid() {
		x.Target = r.Attachment.Key.Facet
		x.KeyCore = r.Attachment.Key.Core
		x.Proposal = signer.FacetProposal(r.Attachment.Key, r.Attachment.OriginalProposal)
	}
	if r.Pending.Valid() {
		x.PendingTarget = r.Pending.Facet
		x.PendingProposal = signer.AttachmentRevision(r.Pending.Proposal)
	}
	return x
}
