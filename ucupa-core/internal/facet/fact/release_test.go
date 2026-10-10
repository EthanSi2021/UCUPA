package fact

import (
	"encoding/binary"
	"testing"

	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
)

func testDisposition(t *testing.T, facet values.FactAuthorityRef, key values.FacetAcceptanceKey) contract.FacetDispositionRead {
	t.Helper()
	var raw [40]byte
	raw[0] = 8
	binary.BigEndian.PutUint64(raw[32:], 5)
	cut, err := values.NewFacetReadCut(raw[:])
	if err != nil {
		t.Fatal(err)
	}
	return contract.FacetDispositionRead{Found: true, Facet: facet, Element: testElement, Key: key, Cut: cut}
}

// detachedFixture returns a locally detached state: the revoked relationship's
// acceptance key stays on the record without Core-transfer predecessor
// evidence.
func detachedFixture(t *testing.T) (State, values.FacetAcceptanceKey, values.FactAuthorityRef) {
	t.Helper()
	state, key, owner := recoveryFixture(t)
	return state, key, owner
}

func TestReleaseRequiresDetachedRecordAndMatchingDisposition(t *testing.T) {
	state, key, owner := detachedFixture(t)
	facet := key.Facet
	r, _ := state.Record(testElement)
	release := Binding{Kind: Release, Element: testElement, Target: facet, Address: "local:facet", Expected: r.Attachment.Revision}

	if plan := state.Plan(release, contract.FacetAcceptanceEvidence{}, contract.FacetDispositionRead{}, true); plan.Code() != DispositionMissing {
		t.Fatalf("missing disposition admitted: %+v", plan)
	}
	absent := testDisposition(t, facet, key)
	absent.Found = false
	if plan := state.Plan(release, contract.FacetAcceptanceEvidence{}, absent, true); plan.Code() != StateConflict {
		t.Fatalf("absent disposition admitted: %+v", plan)
	}
	foreign := testDisposition(t, facet, key)
	foreign.Key = values.FacetAcceptanceKey{}
	if plan := state.Plan(release, contract.FacetAcceptanceEvidence{}, foreign, true); plan.Code() != StateConflict {
		t.Fatalf("disposition without key admitted: %+v", plan)
	}
	wrongFacet := Binding{Kind: Release, Element: testElement, Target: testAuthority(t), Address: "local:other", Expected: r.Attachment.Revision}
	if plan := state.Plan(wrongFacet, contract.FacetAcceptanceEvidence{}, testDisposition(t, facet, key), true); plan.Code() != StateConflict {
		t.Fatalf("release to a different Facet admitted: %+v", plan)
	}

	// An active relationship is not a releasable control occupancy.
	active := New()
	active.ElementCreated(testElement, testCut(2))
	facetRef := testAuthority(t)
	initial, _ := active.Record(testElement)
	proposed := active.Apply(
		active.Plan(Binding{Kind: Propose, Element: testElement, Target: facetRef, Address: "local:facet", Expected: initial.Attachment.Revision}, contract.FacetAcceptanceEvidence{}, contract.FacetDispositionRead{}, true),
		testAuthority(t), testCut(3))
	if plan := active.Plan(Binding{Kind: Release, Element: testElement, Target: facetRef, Address: "local:facet", Expected: proposed.Attachment.Revision}, contract.FacetAcceptanceEvidence{}, testDisposition(t, facetRef, proposed.Pending), true); plan.Code() != StateConflict {
		t.Fatalf("release of an active relationship admitted: %+v", plan)
	}

	plan := state.Plan(release, contract.FacetAcceptanceEvidence{}, testDisposition(t, facet, key), true)
	if !plan.Changes() || plan.Code() != OK {
		t.Fatalf("matching disposition rejected: %+v", plan)
	}
	released := state.Apply(plan, owner, testCut(6))
	if released.Attachment.State != contract.FacetRetired || released.Attachment.Key != key || released.Pending != (values.FacetAcceptanceKey{}) ||
		released.NextHop != "" || released.Attachment.NextHop != "" || released.NextHopRevision != (contract.FacetContactRevision{}) {
		t.Fatalf("release did not retire the record: %+v", released)
	}
}

func TestReleaseDischargesPendingObligationAndStaysTerminal(t *testing.T) {
	state, key, owner := detachedFixture(t)
	facet := key.Facet
	r, _ := state.Record(testElement)
	// The open recovery proposal is the protected obligation only the final
	// disposition protocol may discharge.
	proposed := state.Apply(
		state.Plan(Binding{Kind: Propose, Element: testElement, Target: facet, Address: "local:facet-again", Expected: r.Attachment.Revision}, contract.FacetAcceptanceEvidence{}, contract.FacetDispositionRead{}, true),
		owner, testCut(6))
	obligated, _ := state.Record(testElement)
	if !proposed.Pending.Valid() || !obligated.RecoveryPending() {
		t.Fatal("recovery obligation missing", proposed)
	}
	// A disposition naming the stored key also discharges the obligation: the
	// retire transaction removed the predecessor acceptance that recovery
	// would need.
	lineage := Binding{Kind: Release, Element: testElement, Target: facet, Address: "local:facet", Expected: proposed.Attachment.Revision}
	plan := state.Plan(lineage, contract.FacetAcceptanceEvidence{}, testDisposition(t, facet, key), true)
	if !plan.Changes() || plan.Code() != OK {
		t.Fatalf("lineage disposition rejected under an open proposal: %+v", plan)
	}
	released := state.Apply(plan, owner, testCut(7))
	if released.Attachment.State != contract.FacetRetired || released.Pending != (values.FacetAcceptanceKey{}) {
		t.Fatalf("release did not discharge the obligation: %+v", released)
	}
	current, _ := state.Record(testElement)
	if current.RecoveryPending() || current.ReacceptancePending() || current.predecessorEvidence() {
		t.Fatal("retired record still reports an obligation")
	}
	terminal := current.Attachment.Revision
	for _, tc := range []struct {
		name string
		b    Binding
		want Code
	}{
		{"propose", Binding{Kind: Propose, Element: testElement, Target: facet, Address: "local:facet", Expected: terminal}, StateConflict},
		{"cancel", Binding{Kind: Cancel, Element: testElement, Expected: terminal}, ProposalNotCurrent},
		{"activate", Binding{Kind: Activate, Element: testElement, Expected: terminal}, ProposalNotCurrent},
		{"detach", Binding{Kind: Detach, Element: testElement, Expected: terminal}, StateConflict},
		{"release", Binding{Kind: Release, Element: testElement, Target: facet, Address: "local:facet", Expected: terminal}, StateConflict},
		{"contact", Binding{Kind: Address, Element: testElement, Key: key, Contact: current.Contact, Address: "local:facet"}, Changed},
	} {
		if plan := state.Plan(tc.b, contract.FacetAcceptanceEvidence{}, testDisposition(t, facet, key), true); plan.Code() != tc.want {
			t.Fatalf("retired record admitted %s: %+v", tc.name, plan)
		}
	}
}
