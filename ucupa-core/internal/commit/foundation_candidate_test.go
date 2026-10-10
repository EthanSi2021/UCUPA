package commit

import (
	"testing"

	"ucupa/ucupa-core/contract"
	commitbinding "ucupa/ucupa-core/internal/commit/binding"
	elementfact "ucupa/ucupa-core/internal/element/fact"
	facetfact "ucupa/ucupa-core/internal/facet/fact"
)

func TestFoundationCandidatesDoNotPublishOrMutateTheirSource(t *testing.T) {
	t.Run("element and attachment", func(t *testing.T) {
		f := newFoundationCandidateElementFixture(t)
		before := f.owner.state.clone()
		entity, _ := before.elements.Entity(f.entity)
		input := commitbinding.Binding{Actor: foundationCandidateActor, Kind: elementfact.OperationCreateEntity,
			Element: foundationCandidateID(811099), Final: foundationCandidateTuple(t, entity.Parent, 0, 0, 0, 1)}
		next, outcome, err := f.owner.state.prepareMutation("candidate-element", input)
		if err != nil || next == nil || next == f.owner.state || outcome.Status != contract.MutationCommitted ||
			!sameCompleteCoreState(before, f.owner.state) || len(next.operations) != len(before.operations)+1 ||
			len(next.commits) != len(before.commits)+1 {
			t.Fatalf("Element candidate changed the publishing state: %#v %v", outcome, err)
		}
		if err := validateCoreState(next); err != nil {
			t.Fatal(err)
		}
		replayed, retained, err := next.prepareMutation("candidate-element", input)
		if err != nil || replayed != nil || retained != outcome {
			t.Fatal("exact retry constructed a new publication")
		}
		input.Element = foundationCandidateID(811098)
		if candidate, _, err := next.prepareMutation("candidate-element", input); candidate != nil || err != contract.ErrOperationConflict {
			t.Fatal("different retry binding did not retain conflict precedence")
		}
		current, _ := before.attachments.facts.Record(f.entity)
		binding := facetfact.Binding{Actor: foundationCandidateActor, Kind: facetfact.Propose, Element: f.entity,
			Target: foundationCandidateOtherAuthority(t, before.authority), Address: "local:facet", Expected: current.Attachment.Revision}
		next, result, code := f.owner.state.prepareAttachment("candidate-attachment", binding, contract.FacetAcceptanceEvidence{}, contract.FacetDispositionRead{})
		if code != facetfact.OK || next == nil || result.Status != contract.MutationCommitted ||
			!sameCompleteCoreState(before, f.owner.state) || len(next.attachments.events) != len(before.attachments.events)+1 ||
			len(next.attachments.operations) != len(before.attachments.operations)+1 || len(next.commits) != len(before.commits)+1 {
			t.Fatalf("Attachment candidate split its receipt/journal: %#v %v", result, code)
		}
		if err := validateCoreState(next); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("mount", func(t *testing.T) {
		f := newFoundationCandidateMountFixture(t)
		before := f.owner.state.clone()
		event := MountEvent{Kind: MountEventReserve, Operation: "candidate-mount", Authorized: true,
			Binding: MountBinding{Actor: foundationCandidateMountActor, PeerActor: foundationCandidateMountActor, Command: f.command}}
		next, err := f.owner.state.prepareMountEvent(event)
		if err != nil || next == nil || next == f.owner.state || !sameCompleteCoreState(before, f.owner.state) ||
			len(next.mount.events) != len(before.mount.events)+1 || len(next.mount.operations) != len(before.mount.operations)+1 {
			t.Fatalf("Mount candidate changed the source: %v", err)
		}
		if err := validateCoreState(next); err != nil {
			t.Fatal(err)
		}
		event.Kind = MountEventKind(255)
		if rejected, err := f.owner.state.prepareMountEvent(event); rejected != nil || err != contract.ErrStore || !sameCompleteCoreState(before, f.owner.state) {
			t.Fatal("invalid Mount candidate was published or changed source state")
		}
	})
}

func TestFoundationControlPreparationRetainsExactRetryPrecedence(t *testing.T) {
	f := newFoundationCandidateMountFixture(t)
	before := f.owner.state.clone()
	binding := MountBinding{Actor: foundationCandidateMountActor, Command: f.command}
	if next, err := before.prepareMountIntent(nil, "missing-caller", binding); next != nil || err != contract.ErrDenied {
		t.Fatal("missing peer reserved an operation")
	}
	next, err := before.prepareMountIntent(f.callers, "candidate-intent", binding)
	if err != nil || next == nil || !sameCompleteCoreState(before, f.owner.state) {
		t.Fatalf("reservation preparation changed its source: %v", err)
	}
	if again, err := next.prepareMountIntent(f.callers, "candidate-intent", binding); again != nil || err != nil {
		t.Fatal("exact reservation retry constructed another candidate")
	}
	changed := binding
	changed.Command.ParentNextHop = "local:other"
	if again, err := next.prepareMountIntent(nil, "candidate-intent", changed); again != nil || err != contract.ErrDenied {
		t.Fatal("retry conflict took precedence over missing retained peer identity")
	}
	if again, err := next.prepareMountIntent(f.callers, "candidate-intent", changed); again != nil || err != contract.ErrOperationConflict {
		t.Fatal("different authenticated retry was accepted")
	}
	record := next.mount.facts.Record(f.mount)
	address := NextHopBinding{Actor: foundationCandidateMountActor, Element: f.mount, Key: record.Key,
		Expected: record.Contact.Revision, NextHop: "local:b-new"}
	addressState, outcome, err := next.prepareMountAddress("candidate-address", address)
	if err != nil || addressState == nil || !outcome.result.Changed || next.mount.facts.Record(f.mount) != record {
		t.Fatalf("address preparation mutated its source: %#v %v", outcome, err)
	}
	if again, got, err := addressState.prepareMountAddress("candidate-address", address); again != nil || got != outcome || err != nil {
		t.Fatal("exact address retry did not retain its original receipt")
	}
	// The old expected revision is a new retained rejection. Its no-cut journal
	// entry must be part of the candidate before the decision is returned.
	rejected, receipt, err := addressState.prepareMountAddress("stale-address", address)
	if err != nil || rejected == nil || len(rejected.mount.events) != len(addressState.mount.events)+1 || len(rejected.commits) != len(addressState.commits) {
		t.Fatalf("address rejection lost its no-cut history: %v", err)
	}
	if _, err := receipt.output(); err != contract.ErrRevisionConflict {
		t.Fatalf("wrong retained rejection: %v", err)
	}
	for _, state := range []*coreState{next, addressState, rejected} {
		if err := validateCoreState(state); err != nil {
			t.Fatal(err)
		}
	}
	if again, _, err := next.prepareMountAddress("candidate-intent", address); again != nil || err != contract.ErrOperationConflict {
		t.Fatal("address operation reused a mount operation ID")
	}
}

func TestFoundationAttachmentLookupDoesNotConstructOrChangeState(t *testing.T) {
	f := newFoundationCandidateElementFixture(t)
	before := f.owner.state.clone()
	record, _ := before.attachments.facts.Record(f.entity)
	binding := facetfact.Binding{Actor: foundationCandidateActor, Kind: facetfact.Propose, Element: f.entity,
		Target: foundationCandidateOtherAuthority(t, before.authority), Address: "local:facet", Expected: record.Attachment.Revision}
	next, result, code := before.prepareAttachment("lookup-attachment", binding, contract.FacetAcceptanceEvidence{}, contract.FacetDispositionRead{})
	if code != facetfact.OK {
		t.Fatal(code)
	}
	if got, err, found := next.lookupAttachment("lookup-attachment", binding); !found || err != nil || got != result {
		t.Fatalf("exact attachment retry: %#v %v %v", got, err, found)
	}
	binding.Address = "local:changed"
	if _, err, found := next.lookupAttachment("lookup-attachment", binding); !found || err != contract.ErrOperationConflict {
		t.Fatal("mismatched attachment retry was accepted")
	}
	for operation := range next.operations {
		if _, err, found := next.lookupAttachment(operation, binding); !found || err != contract.ErrOperationConflict {
			t.Fatal("attachment reused an Element operation ID")
		}
	}
	if _, err, found := next.lookupAttachment("unclaimed", binding); found || err != nil || !sameCompleteCoreState(before, f.owner.state) {
		t.Fatal("unclaimed lookup reserved or published state")
	}
}
