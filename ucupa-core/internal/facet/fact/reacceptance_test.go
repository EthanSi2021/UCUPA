package fact

import (
	"testing"

	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	"ucupa/ucupa-core/internal/token"
)

// reacceptanceFixture builds the outcome of a completed Core transfer through
// the real ElementTransferred path: a DETACHED record carrying the previous
// Core's acceptance key and its signed proposal comparison.
func reacceptanceFixture(t *testing.T) (State, values.FacetAcceptanceKey, values.PublicToken) {
	t.Helper()
	source := New()
	source.ElementCreated(testElement, testCut(2))
	owner, facet := testAuthority(t), testAuthority(t)
	initial, _ := source.Record(testElement)
	proposed := source.Apply(
		source.Plan(Binding{Kind: Propose, Element: testElement, Target: facet, Address: "local:facet", Expected: initial.Attachment.Revision}, contract.FacetAcceptanceEvidence{}, contract.FacetDispositionRead{}, true),
		owner, testCut(3))
	active := source.Apply(
		source.Plan(Binding{Kind: Activate, Element: testElement, Expected: proposed.Attachment.Revision}, testEvidence(t, proposed.Pending), contract.FacetDispositionRead{}, true),
		owner, testCut(4))
	r, ok := source.Record(testElement)
	if !ok || r.Attachment.State != contract.FacetActive || r.Attachment.Key != active.Attachment.Key {
		t.Fatal("source activation", r)
	}
	// The transfer capture enriches the signed proposal comparison for the
	// active key; the local record keeps none.
	comparison := token.NewPublicSigner([32]byte{9}, owner).FacetProposal(active.Attachment.Key, values.PublicToken{})
	if !comparison.Valid() {
		t.Fatal("signed proposal comparison is invalid")
	}
	r.Attachment.OriginalProposal = comparison
	transferred := New()
	if err := transferred.ElementTransferred(testElement, testCut(5), r); err != nil {
		t.Fatal(err)
	}
	return transferred, active.Attachment.Key, comparison
}

func TestReacceptanceProposeSameFacetOnly(t *testing.T) {
	state, predecessor, _ := reacceptanceFixture(t)
	r, _ := state.Record(testElement)
	if r.Attachment.State != contract.FacetDetached || r.Attachment.Key != predecessor || !r.predecessorEvidence() || r.ReacceptancePending() {
		t.Fatal("transferred record lost its predecessor shape", r)
	}
	other := Binding{Kind: Propose, Element: testElement, Target: testAuthority(t), Address: "local:other-facet", Expected: r.Attachment.Revision}
	if plan := state.Plan(other, contract.FacetAcceptanceEvidence{}, contract.FacetDispositionRead{}, true); plan.Code() != StateConflict {
		t.Fatalf("proposal to a different Facet admitted: %+v", plan)
	}
	b := Binding{Kind: Propose, Element: testElement, Target: predecessor.Facet, Address: "local:facet-again", Expected: r.Attachment.Revision}
	plan := state.Plan(b, contract.FacetAcceptanceEvidence{}, contract.FacetDispositionRead{}, true)
	if !plan.Changes() || plan.Code() != OK {
		t.Fatalf("same-Facet re-proposal rejected: %+v", plan)
	}
	next := testAuthority(t)
	reproposed := state.Apply(plan, next, testCut(6))
	if !reproposed.Pending.Valid() || reproposed.Pending.Core != next || reproposed.Pending.Facet != predecessor.Facet ||
		reproposed.Pending.Proposal != reproposed.Attachment.Revision {
		t.Fatalf("re-proposal lost its key: %+v", reproposed)
	}
	if current, _ := state.Record(testElement); !current.ReacceptancePending() {
		t.Fatal("open re-acceptance proposal not detected")
	}
	duplicate := Binding{Kind: Propose, Element: testElement, Target: predecessor.Facet, Address: "local:facet-again", Expected: reproposed.Attachment.Revision}
	if plan := state.Plan(duplicate, contract.FacetAcceptanceEvidence{}, contract.FacetDispositionRead{}, true); plan.Code() != StateConflict {
		t.Fatalf("duplicate re-acceptance proposal admitted: %+v", plan)
	}
}

func TestReacceptanceCancelProtectsObligation(t *testing.T) {
	state, predecessor, _ := reacceptanceFixture(t)
	r, _ := state.Record(testElement)
	proposed := state.Apply(
		state.Plan(Binding{Kind: Propose, Element: testElement, Target: predecessor.Facet, Address: "local:facet-again", Expected: r.Attachment.Revision}, contract.FacetAcceptanceEvidence{}, contract.FacetDispositionRead{}, true),
		testAuthority(t), testCut(6))
	cancel := Binding{Kind: Cancel, Element: testElement, Expected: proposed.Attachment.Revision}
	if plan := state.Plan(cancel, contract.FacetAcceptanceEvidence{}, contract.FacetDispositionRead{}, true); plan.Code() != StateConflict {
		t.Fatalf("re-acceptance proposal cancelled like an ordinary one: %+v", plan)
	}
	// Ordinary proposals keep their existing cancellation path.
	ordinary := New()
	ordinary.ElementCreated(testElement, testCut(2))
	owner, facet := testAuthority(t), testAuthority(t)
	initial, _ := ordinary.Record(testElement)
	opened := ordinary.Apply(
		ordinary.Plan(Binding{Kind: Propose, Element: testElement, Target: facet, Address: "local:facet", Expected: initial.Attachment.Revision}, contract.FacetAcceptanceEvidence{}, contract.FacetDispositionRead{}, true),
		owner, testCut(3))
	closed := ordinary.Apply(
		ordinary.Plan(Binding{Kind: Cancel, Element: testElement, Expected: opened.Attachment.Revision}, contract.FacetAcceptanceEvidence{}, contract.FacetDispositionRead{}, true),
		owner, testCut(4))
	if closed.Pending.Valid() || closed.Attachment.State != contract.FacetNone {
		t.Fatalf("ordinary cancellation regressed: %+v", closed)
	}
}

func TestReacceptanceActivateRenewsLineage(t *testing.T) {
	state, predecessor, comparison := reacceptanceFixture(t)
	next := testAuthority(t)
	r, _ := state.Record(testElement)
	proposed := state.Apply(
		state.Plan(Binding{Kind: Propose, Element: testElement, Target: predecessor.Facet, Address: "local:facet-again", Expected: r.Attachment.Revision}, contract.FacetAcceptanceEvidence{}, contract.FacetDispositionRead{}, true),
		next, testCut(6))
	activated := state.Apply(
		state.Plan(Binding{Kind: Activate, Element: testElement, Expected: proposed.Attachment.Revision}, testEvidence(t, proposed.Pending), contract.FacetDispositionRead{}, true),
		next, testCut(7))
	if activated.Attachment.State != contract.FacetActive || activated.Attachment.Key != proposed.Pending ||
		activated.Pending != (values.FacetAcceptanceKey{}) || activated.Attachment.OriginalProposal != (values.PublicToken{}) {
		t.Fatalf("re-acceptance activation kept predecessor state: %+v", activated)
	}
	current, _ := state.Record(testElement)
	if current.ReacceptancePending() || current.predecessorEvidence() {
		t.Fatal("activated record still reports a pending re-acceptance")
	}
	// The previous Core's comparison cannot serve the new key: keeping it
	// would break the next transfer's proposal capture.
	if stale := token.NewPublicSigner([32]byte{11}, next).FacetProposal(activated.Attachment.Key, comparison); stale != (values.PublicToken{}) {
		t.Fatal("stale predecessor comparison would survive the next capture")
	}
	// With the cleared record the capture renews: the new owner signs the
	// new key's proposal and the transfer chain continues.
	renewed := current
	renewed.Attachment.OriginalProposal = token.NewPublicSigner([32]byte{11}, next).FacetProposal(renewed.Attachment.Key, values.PublicToken{})
	chain := New()
	if err := chain.ElementTransferred(testElement, testCut(8), renewed); err != nil {
		t.Fatal(err)
	}
	if transferred, _ := chain.Record(testElement); transferred.Attachment.State != contract.FacetDetached || transferred.Attachment.Key != activated.Attachment.Key {
		t.Fatal("continued transfer lost the renewed key", transferred)
	}
}
