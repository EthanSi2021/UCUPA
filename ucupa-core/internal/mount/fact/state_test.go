package fact

import (
	"testing"

	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
)

func TestStateCloneReplaceAndIteration(t *testing.T) {
	id := values.ElementID("anchor")
	original := New()
	original.ReplaceOutgoing(id, Record{Slot: contract.OperationID("original")})
	original.ReplaceIncoming(Incoming{Gate: GatePreparing})

	clone := original.Clone()
	if !clone.Initialized() || !clone.Equal(original) {
		t.Fatal("clone should be initialized and equal before mutation")
	}

	record, ok := clone.Outgoing(id)
	if !ok {
		t.Fatal("clone lost outgoing relation")
	}
	record.Slot = contract.OperationID("clone")
	clone.ReplaceOutgoing(id, record)

	incoming := clone.Incoming()
	incoming.Gate = GateOpen
	clone.ReplaceIncoming(incoming)

	if got, _ := original.Outgoing(id); got.Slot != contract.OperationID("original") {
		t.Fatal("outgoing replacement mutated original state")
	}
	if original.Incoming().Gate != GatePreparing {
		t.Fatal("incoming replacement mutated original state")
	}

	seen := false
	for gotID, got := range clone.Outgoings() {
		if gotID != id || got.Slot != contract.OperationID("clone") {
			t.Fatalf("unexpected outgoing iteration: %q %#v", gotID, got)
		}
		seen = true
	}
	if !seen {
		t.Fatal("outgoing iterator yielded no relation")
	}
}

func TestStateZeroCloneAndEqualNilEmptyMapSemantics(t *testing.T) {
	var zero State
	if zero.Initialized() {
		t.Fatal("zero State must retain nil-table state")
	}
	clone := zero.Clone()
	if !clone.Initialized() {
		t.Fatal("Clone must initialize the outgoing table")
	}
	empty := State{View: View{outgoing: make(map[values.ElementID]Record)}}
	if !zero.Equal(empty) || !empty.Equal(zero) {
		t.Fatal("Equal must preserve nil/empty map equality")
	}
}

func TestStateIncomingReplacementIsWholeValue(t *testing.T) {
	state := New()
	want := Incoming{Gate: GatePreparing, Detached: true}
	state.ReplaceIncoming(want)
	got := state.Incoming()
	if got != want {
		t.Fatalf("incoming replacement mismatch: got %#v want %#v", got, want)
	}
	got.Gate = GateOpen
	if state.Incoming().Gate != GatePreparing {
		t.Fatal("Incoming must return a value copy")
	}
}

func TestStateBoundedIncomingAndMissingRecord(t *testing.T) {
	state := New()
	id := values.ElementID("mount")
	state.ReplaceOutgoing(id, Record{State: contract.MountActive})
	state.ReplaceIncoming(Incoming{Gate: GateOpen})
	projection := IncomingOnly(state.Incoming())
	if projection.Initialized() || projection.OutgoingCount() != 0 || projection.Incoming() != state.Incoming() {
		t.Fatal("incoming projection retained outgoing table or changed incoming")
	}
	if _, exists := projection.Outgoing(id); exists || projection.Record(id) != (Record{}) {
		t.Fatal("incoming-only projection disclosed outgoing relation")
	}
	if state.OutgoingCount() != 1 || state.Record(id).State != contract.MountActive {
		t.Fatal("bounded projection mutated source")
	}
}

func TestStateOutgoingIteratorStopsAndReturnsCopies(t *testing.T) {
	state := New()
	for _, id := range []values.ElementID{"first", "second"} {
		state.ReplaceOutgoing(id, Record{Slot: "unchanged"})
	}
	count := 0
	for _, record := range state.Outgoings() {
		count++
		record.Slot = "local-only"
		break
	}
	if count != 1 || state.Record("first").Slot != "unchanged" || state.Record("second").Slot != "unchanged" {
		t.Fatal("iterator did not stop or exposed mutable records")
	}
}
