package fact

import (
	"testing"

	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
)

// recoveryFixture builds the outcome of a local detach: a DETACHED record
// that retains the same Core's acceptance key for the same Facet, with no
// Core-transfer predecessor evidence. The stored key is the revoked
// relationship itself, so a recovery proposal can only target that Facet.
func recoveryFixture(t *testing.T) (State, values.FacetAcceptanceKey, values.FactAuthorityRef) {
	t.Helper()
	state := New()
	state.ElementCreated(testElement, testCut(2))
	owner, facet := testAuthority(t), testAuthority(t)
	initial, _ := state.Record(testElement)
	proposed := state.Apply(
		state.Plan(Binding{Kind: Propose, Element: testElement, Target: facet, Address: "local:facet", Expected: initial.Attachment.Revision}, contract.FacetAcceptanceEvidence{}, contract.FacetDispositionRead{}, true),
		owner, testCut(3))
	active := state.Apply(
		state.Plan(Binding{Kind: Activate, Element: testElement, Expected: proposed.Attachment.Revision}, testEvidence(t, proposed.Pending), contract.FacetDispositionRead{}, true),
		owner, testCut(4))
	detached := state.Apply(
		state.Plan(Binding{Kind: Detach, Element: testElement, Expected: active.Attachment.Revision}, contract.FacetAcceptanceEvidence{}, contract.FacetDispositionRead{}, true),
		owner, testCut(5))
	r, ok := state.Record(testElement)
	if !ok || detached.Attachment.State != contract.FacetDetached || r.Attachment.Key != active.Attachment.Key {
		t.Fatal("local detach lost the stored key", r)
	}
	if r.predecessorEvidence() || r.ReacceptancePending() || r.RecoveryPending() {
		t.Fatalf("local detach gained a transfer obligation: %+v", r)
	}
	return state, active.Attachment.Key, owner
}

func TestRecoveryProposeReopensOnlyTheStoredFacet(t *testing.T) {
	state, predecessor, owner := recoveryFixture(t)
	r, _ := state.Record(testElement)
	if r.Attachment.Key.Facet != predecessor.Facet || r.Attachment.Key.Core != owner {
		t.Fatal("stored key is not this Core's acceptance", r)
	}
	other := Binding{Kind: Propose, Element: testElement, Target: testAuthority(t), Address: "local:other-facet", Expected: r.Attachment.Revision}
	if plan := state.Plan(other, contract.FacetAcceptanceEvidence{}, contract.FacetDispositionRead{}, true); plan.Code() != StateConflict {
		t.Fatalf("recovery proposal to another Facet admitted: %+v", plan)
	}
	b := Binding{Kind: Propose, Element: testElement, Target: predecessor.Facet, Address: "local:facet-again", Expected: r.Attachment.Revision}
	plan := state.Plan(b, contract.FacetAcceptanceEvidence{}, contract.FacetDispositionRead{}, true)
	if !plan.Changes() || plan.Code() != OK {
		t.Fatalf("recovery proposal to the stored Facet rejected: %+v", plan)
	}
	reproposed := state.Apply(plan, owner, testCut(6))
	if !reproposed.Pending.Valid() || reproposed.Pending.Core != owner || reproposed.Pending.Facet != predecessor.Facet ||
		reproposed.Pending.Proposal != reproposed.Attachment.Revision {
		t.Fatalf("recovery proposal lost its key: %+v", reproposed)
	}
	current, _ := state.Record(testElement)
	if !current.RecoveryPending() || current.ReacceptancePending() {
		t.Fatal("open recovery obligation not detected as recovery shape")
	}
	duplicate := Binding{Kind: Propose, Element: testElement, Target: predecessor.Facet, Address: "local:facet-again", Expected: reproposed.Attachment.Revision}
	if plan := state.Plan(duplicate, contract.FacetAcceptanceEvidence{}, contract.FacetDispositionRead{}, true); plan.Code() != StateConflict {
		t.Fatalf("duplicate recovery proposal admitted: %+v", plan)
	}
}

func TestRecoveryCancelProtectsObligation(t *testing.T) {
	state, predecessor, owner := recoveryFixture(t)
	r, _ := state.Record(testElement)
	proposed := state.Apply(
		state.Plan(Binding{Kind: Propose, Element: testElement, Target: predecessor.Facet, Address: "local:facet-again", Expected: r.Attachment.Revision}, contract.FacetAcceptanceEvidence{}, contract.FacetDispositionRead{}, true),
		owner, testCut(6))
	cancel := Binding{Kind: Cancel, Element: testElement, Expected: proposed.Attachment.Revision}
	if plan := state.Plan(cancel, contract.FacetAcceptanceEvidence{}, contract.FacetDispositionRead{}, true); plan.Code() != StateConflict {
		t.Fatalf("recovery proposal cancelled like an ordinary one: %+v", plan)
	}
}

func TestRecoveryActivateRenewsLineageAndRepeats(t *testing.T) {
	state, predecessor, owner := recoveryFixture(t)
	r, _ := state.Record(testElement)
	proposed := state.Apply(
		state.Plan(Binding{Kind: Propose, Element: testElement, Target: predecessor.Facet, Address: "local:facet-again", Expected: r.Attachment.Revision}, contract.FacetAcceptanceEvidence{}, contract.FacetDispositionRead{}, true),
		owner, testCut(6))
	activated := state.Apply(
		state.Plan(Binding{Kind: Activate, Element: testElement, Expected: proposed.Attachment.Revision}, testEvidence(t, proposed.Pending), contract.FacetDispositionRead{}, true),
		owner, testCut(7))
	if activated.Attachment.State != contract.FacetActive || activated.Attachment.Key != proposed.Pending ||
		activated.Pending != (values.FacetAcceptanceKey{}) || activated.Attachment.OriginalProposal != (values.PublicToken{}) {
		t.Fatalf("recovery activation kept predecessor state: %+v", activated)
	}
	current, _ := state.Record(testElement)
	if current.RecoveryPending() || current.ReacceptancePending() || current.predecessorEvidence() {
		t.Fatal("activated record still reports an open obligation")
	}
	// The renewed record supports another local detach and recovery round.
	detached := state.Apply(
		state.Plan(Binding{Kind: Detach, Element: testElement, Expected: activated.Attachment.Revision}, contract.FacetAcceptanceEvidence{}, contract.FacetDispositionRead{}, true),
		owner, testCut(8))
	if detached.Attachment.State != contract.FacetDetached || detached.Attachment.Key != proposed.Pending {
		t.Fatalf("renewed detach lost its key: %+v", detached)
	}
	again := state.Apply(
		state.Plan(Binding{Kind: Propose, Element: testElement, Target: predecessor.Facet, Address: "local:facet-again", Expected: detached.Attachment.Revision}, contract.FacetAcceptanceEvidence{}, contract.FacetDispositionRead{}, true),
		owner, testCut(9))
	if !again.Pending.Valid() || again.Pending.Facet != predecessor.Facet {
		t.Fatalf("second recovery proposal lost its key: %+v", again)
	}
	if current, _ := state.Record(testElement); !current.RecoveryPending() {
		t.Fatal("second recovery obligation not detected")
	}
}
