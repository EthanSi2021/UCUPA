package fact

import (
	"encoding/binary"
	"errors"
	"testing"

	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	"ucupa/ucupa-core/internal/token"
)

const testElement values.ElementID = "00000000-0000-4000-8000-000000000001"

func testCut(serial uint64) contract.CoreReadCut {
	return token.FromParts([32]byte{7}, serial)
}

func testAuthority(t *testing.T) values.FactAuthorityRef {
	t.Helper()
	ref, err := values.NewFactAuthorityRef()
	if err != nil {
		t.Fatal(err)
	}
	return ref
}

func testEvidence(t *testing.T, key values.FacetAcceptanceKey) contract.FacetAcceptanceEvidence {
	t.Helper()
	var raw [40]byte
	raw[0] = 8
	binary.BigEndian.PutUint64(raw[32:], 3)
	cut, err := values.NewFacetReadCut(raw[:])
	if err != nil {
		t.Fatal(err)
	}
	return contract.FacetAcceptanceEvidence{Key: key, AcceptedAt: cut}
}

func TestLifecyclePlanAndAppliedCut(t *testing.T) {
	owner, target := testAuthority(t), testAuthority(t)
	state := New()
	state.ElementCreated(testElement, testCut(2))
	apply := func(b Binding, evidence contract.FacetAcceptanceEvidence, serial uint64) contract.FacetAttachmentResult {
		t.Helper()
		before := state.Clone()
		plan := state.Plan(b, evidence, contract.FacetDispositionRead{}, true)
		if !state.Equal(before) || !plan.Changes() || plan.Code() != OK {
			t.Fatalf("plan changed state or rejected: %+v", plan)
		}
		result := state.Apply(plan, owner, testCut(serial))
		if result.Status != contract.MutationCommitted || result.OutcomeCut != testCut(serial) {
			t.Fatalf("applied result: %+v", result)
		}
		return result
	}
	initial, _ := state.Record(testElement)
	proposed := apply(Binding{Kind: Propose, Element: testElement, Target: target, Address: "local:facet", Expected: initial.Attachment.Revision}, contract.FacetAcceptanceEvidence{}, 3)
	if !proposed.Pending.Valid() || proposed.Pending.Core != owner || proposed.Pending.Facet != target || proposed.Pending.Proposal != proposed.Attachment.Revision {
		t.Fatalf("proposal lost its owner/revision: %+v", proposed)
	}
	activate := Binding{Kind: Activate, Element: testElement, Expected: proposed.Attachment.Revision}
	missing := state.Plan(activate, contract.FacetAcceptanceEvidence{}, contract.FacetDispositionRead{}, true)
	if missing.Changes() || missing.Code() != AcceptanceMissing || missing.Result(testCut(3)).OutcomeCut != testCut(3) {
		t.Fatalf("missing evidence: %+v", missing)
	}
	beforeReject := state.Clone()
	if got := state.Apply(missing, owner, testCut(3)); got != missing.Result(testCut(3)) || !state.Equal(beforeReject) {
		t.Fatal("applying a rejected plan changed facts or its result")
	}
	active := apply(activate, testEvidence(t, proposed.Pending), 4)
	if active.Attachment.State != contract.FacetActive || active.Pending != (values.FacetAcceptanceKey{}) || active.Attachment.Key != proposed.Pending || active.NextHopRevision != proposed.NextHopRevision {
		t.Fatalf("activation changed relation/contact binding: %+v", active)
	}
	address := Binding{Kind: Address, Element: testElement, Key: active.Attachment.Key, Contact: active.NextHopRevision, Address: active.NextHop}
	unchanged := state.Plan(address, contract.FacetAcceptanceEvidence{}, contract.FacetDispositionRead{}, true)
	if unchanged.Changes() || unchanged.Code() != OK || unchanged.Result(testCut(4)).Status != contract.MutationUnchanged {
		t.Fatalf("same address not unchanged: %+v", unchanged)
	}
	beforeNoop := state.Clone()
	if got := state.Apply(unchanged, owner, testCut(4)); got != unchanged.Result(testCut(4)) || !state.Equal(beforeNoop) {
		t.Fatal("applying an unchanged plan changed facts or its result")
	}
	address.Address = "local:moved-facet"
	moved := apply(address, contract.FacetAcceptanceEvidence{}, 5)
	if moved.Attachment.Revision != active.Attachment.Revision || moved.Attachment.Key != active.Attachment.Key || moved.NextHopRevision == active.NextHopRevision || moved.Attachment.NextHopRevision != moved.NextHopRevision {
		t.Fatalf("address update changed ownership or missed active contact: %+v", moved)
	}
	detached := apply(Binding{Kind: Detach, Element: testElement, Expected: moved.Attachment.Revision}, contract.FacetAcceptanceEvidence{}, 6)
	if detached.Attachment.State != contract.FacetDetached || detached.Attachment.Key != active.Attachment.Key || detached.NextHop != "" || detached.Attachment.NextHop != "" || detached.NextHopRevision != (contract.FacetContactRevision{}) {
		t.Fatalf("detach did not preserve former key and remove contact: %+v", detached)
	}
	state.ElementDeleted(testElement)
	if _, exists := state.Record(testElement); exists {
		t.Fatal("deleted Element retained relationship")
	}
}

func TestDecisionPrecedenceAndCancel(t *testing.T) {
	state := New()
	b := Binding{Kind: Propose, Element: testElement}
	for _, tc := range []struct {
		exists bool
		want   Code
	}{{false, Missing}, {true, StoreFailure}} {
		if got := state.Plan(b, contract.FacetAcceptanceEvidence{}, contract.FacetDispositionRead{}, tc.exists); got.Code() != tc.want {
			t.Fatalf("existence precedes relationship: %+v", got)
		}
	}
	state.ElementCreated(testElement, testCut(2))
	if got := state.Plan(b, contract.FacetAcceptanceEvidence{}, contract.FacetDispositionRead{}, true); got.Code() != RevisionConflict {
		t.Fatalf("revision precedes state: %+v", got)
	}
	r, _ := state.Record(testElement)
	b.Expected, b.Target, b.Address = r.Attachment.Revision, testAuthority(t), "local:facet"
	proposal := state.Apply(state.Plan(b, contract.FacetAcceptanceEvidence{}, contract.FacetDispositionRead{}, true), testAuthority(t), testCut(3))
	cancel := Binding{Kind: Cancel, Element: testElement, Expected: proposal.Attachment.Revision}
	result := state.Apply(state.Plan(cancel, contract.FacetAcceptanceEvidence{}, contract.FacetDispositionRead{}, true), proposal.Pending.Core, testCut(4))
	if result.Attachment.State != contract.FacetNone || result.Pending != (values.FacetAcceptanceKey{}) || result.NextHop != "" || result.NextHopRevision != (contract.FacetContactRevision{}) {
		t.Fatalf("cancel left pending contact: %+v", result)
	}
	for _, tc := range []struct {
		code Code
		err  error
	}{{Missing, contract.ErrNotFound}, {RevisionConflict, contract.ErrAttachmentRevisionConflict}, {StateConflict, contract.ErrAttachmentStateConflict}, {ProposalNotCurrent, contract.ErrProposalNotCurrent}, {AcceptanceMissing, contract.ErrAcceptanceNotFound}, {Changed, contract.ErrAttachmentChanged}, {ContactConflict, contract.ErrFacetContactRevisionConflict}, {StoreFailure, contract.ErrStore}, {Code(99), contract.ErrStore}} {
		if !errors.Is(tc.code.Err(), tc.err) {
			t.Fatalf("error identity changed for %d", tc.code)
		}
	}
}

func TestProjectionCloneAndRevisionBytes(t *testing.T) {
	state := New()
	state.ElementCreated(testElement, testCut(2))
	before := state.Clone()
	builder := NewProjectionBuilder()
	for id, record := range state.Records() {
		builder.Add(id, Record{Attachment: record.Attachment})
		record.Attachment.State = contract.FacetDetached
	}
	projection := builder.Build()
	if !projection.Equal(state) || builder.state.records != nil || !state.Equal(before) {
		t.Fatal("projection/iteration altered facts or retained mutable builder")
	}
	projection.ElementDeleted(testElement)
	if !state.Equal(before) || projection.Equal(state) {
		t.Fatal("projected map shares writable storage with source")
	}
	clone := state.Clone()
	clone.ElementDeleted(testElement)
	if !state.Equal(before) || clone.Equal(state) {
		t.Fatal("clone shares writable storage")
	}
	var expected [76]byte
	expected[0] = 7
	copy(expected[32:68], []byte(testElement))
	binary.BigEndian.PutUint64(expected[68:], 2)
	got := AttachmentRevisionForCut(testElement, testCut(2)).Bytes()
	if string(got) != string(expected[:]) {
		t.Fatal("attachment revision representation changed")
	}
	var zero State
	if zero.Clone().records != nil || !zero.Equal(New()) {
		t.Fatal("nil clone/equality semantics changed")
	}
}
