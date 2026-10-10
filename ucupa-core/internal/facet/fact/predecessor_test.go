package fact

import (
	"testing"

	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	"ucupa/ucupa-core/internal/token"
)

func TestPredecessorClearsContactWithoutMutatingSource(t *testing.T) {
	owner, target := testAuthority(t), testAuthority(t)
	key := values.FacetAcceptanceKey{Core: owner, Facet: target, Element: testElement, Proposal: AttachmentRevisionForCut(testElement, testCut(3))}
	signer := token.NewPublicSigner([32]byte{4}, owner)
	proof := signer.AttachmentRevision(key.Proposal)
	contact := token.FacetContactRevisionFromParts(token.Scope(testCut(3)), key, 3)
	a := contract.FacetAttachment{State: contract.FacetActive, Revision: AttachmentRevisionForCut(testElement, testCut(4)), Key: key, OriginalProposal: proof, NextHop: "local:facet", NextHopRevision: contact}
	source := Record{Attachment: a, NextHop: a.NextHop, Contact: contact}
	cut := token.FromParts([32]byte{9}, 8)
	r, err := TransferPredecessor(source, testElement, cut)
	if err != nil || r.Attachment.State != contract.FacetDetached || r.Attachment.Key != key || r.Attachment.OriginalProposal != proof || r.Key().Valid() || r.NextHop != "" || r.Contact != (contract.FacetContactRevision{}) {
		t.Fatal(r, err)
	}
	if source.Attachment != a || source.NextHop != "local:facet" {
		t.Fatal("mutated source")
	}
	if token.NewPublicSigner([32]byte{5}, target).FacetProposal(r.Attachment.Key, r.Attachment.OriginalProposal) != proof {
		t.Fatal("reissued old proposal under new owner")
	}
	r.Attachment.OriginalProposal = values.PublicToken{}
	if token.NewPublicSigner([32]byte{5}, target).FacetProposal(r.Attachment.Key, r.Attachment.OriginalProposal).Valid() {
		t.Fatal("missing original silently reminted")
	}
	for name, mutate := range map[string]func(*Record){
		"pending":                func(r *Record) { r.Pending = key },
		"missing-proof":          func(r *Record) { r.Attachment.OriginalProposal = values.PublicToken{} },
		"wrong-proof-owner":      func(r *Record) { r.Attachment.OriginalProposal.Owner = target },
		"missing-active-contact": func(r *Record) { r.NextHop = "" },
		"detached-contact":       func(r *Record) { r.Attachment.State = contract.FacetDetached },
		"none-with-key":          func(r *Record) { r.Attachment.State = contract.FacetNone },
		"wrong-member":           func(r *Record) { r.Attachment.Key.Element = values.ElementID(target) },
	} {
		t.Run(name, func(t *testing.T) {
			bad := source
			mutate(&bad)
			s := New()
			s.ElementCreated(testElement, cut)
			before := s.Clone()
			if err := s.ElementTransferred(testElement, cut, bad); err == nil || !s.Equal(before) {
				t.Fatal("invalid precursor mutated candidate", err)
			}
		})
	}
}
