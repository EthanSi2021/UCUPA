package commit

import (
	"errors"
	"strings"
	"testing"
	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	transferencoding "ucupa/ucupa-core/internal/transfer/encoding"
)

func TestTransferTargetParentRetainedCapacityAndCodec(t *testing.T) {
	_, _, target, h, f, raw := targetParentCase(t, "direct-entity")
	proof, err := transferencoding.EncodeTargetParentHold(h)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := transferencoding.DecodeTargetParentHold(proof)
	if err != nil || decoded != h {
		t.Fatal("parent proof did not roundtrip", err)
	}
	// Exact capacity arithmetic on an unpublished disposable state; invalid
	// filler bytes are never installed as a real owner's history.
	s := &coreState{transfers: newTransferState()}
	s.transfers.events = []transferencoding.TransferEvent{{TargetParent: proof, Manifest: strings.Repeat("x", transferencoding.MaxTransferRetainedBytes-len(proof)-len(raw))}}
	if err := s.checkTransferRetainedBytes(len(raw)); err != nil {
		t.Fatal("exact shared retained limit", err)
	}
	if err := s.checkTransferRetainedBytes(len(raw) + 1); !errors.Is(err, values.PublicResourceExhausted) {
		t.Fatal("parent proof escaped retained accounting", err)
	}
	s.transfers.events[0].TargetParent = ""
	if err := s.checkTransferTargetCapacity(raw, "", proof, 0); err != nil {
		t.Fatal("acceptance double-charged parent proof", err)
	}
	if err := s.checkTransferTargetCapacity(raw+"x", "", proof, 0); !errors.Is(err, values.PublicResourceExhausted) {
		t.Fatal("acceptance failed to reserve parent evidence", err)
	}
	if _, err := transferencoding.DecodeTargetParentHold(strings.Repeat("x", transferencoding.MaxTransferParentProofBytes+1)); !errors.Is(err, values.PublicResourceExhausted) {
		t.Fatal("unbounded parent proof", err)
	}
	if _, err := acceptWithParent(target, f, raw, h); err != nil {
		t.Fatal(err)
	}
	for _, kind := range []transferencoding.TransferEventKind{transferencoding.TransferPending, transferencoding.TransferTargetInstalled, transferencoding.TransferParentReleased} {
		bad := transferencoding.TransferEvent{Kind: kind, At: target.state.currentCut(), TargetParent: proof}
		if _, err := transferencoding.EncodeTransferJournal([]transferencoding.TransferEvent{bad}); !errors.Is(err, contract.ErrInvalid) {
			t.Fatal("proof admitted on unrelated event", err)
		}
		if err := target.state.clone().applyTransferEvent(bad); err == nil {
			t.Fatal("runtime accepted proof on unrelated event")
		}
	}
}

func TestTransferTargetParentLargestInputHasBoundedEnvelope(t *testing.T) {
	_, _, target, h, f, _ := targetParentCase(t, "direct-entity")
	// Synthetic boundary bytes test codec arithmetic, not authenticated proof.
	h.Input.Initial.Input.Operation = contract.OperationID(string([]byte{0xff, 't'}))
	for n := 1; n <= transferencoding.MaxTransferIntentBytes; n++ {
		trial := h.Input
		trial.Initial.Input.Actor.Principal = values.PrincipalID(strings.Repeat("p", n))
		if _, err := transferencoding.EncodeTransferParentIntent(trial); err != nil {
			break
		}
		h.Input = trial
	}
	proof, err := transferencoding.EncodeTargetParentHold(h)
	if err != nil || len(proof) > transferencoding.MaxTransferParentProofBytes {
		t.Fatal("accepted input cannot retain its parent proof", len(proof), err)
	}
	if decoded, err := transferencoding.DecodeTargetParentHold(proof); err != nil || decoded != h {
		t.Fatal("largest input loses exact bytes", err)
	}
	f.Input = h.Input.Initial.Input
	e := transferencoding.TransferEvent{Kind: transferencoding.TransferTargetAccepted, At: target.state.currentCut(), Fence: f, TargetParent: proof}
	data, err := transferencoding.EncodeTransferJournal([]transferencoding.TransferEvent{e})
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := transferencoding.LoadTransferJournal(data, target.state.scope)
	if err != nil || len(loaded) != 1 || loaded[0] != e {
		t.Fatal("acceptance envelope lost proof", err)
	}
	// The proof is charged to retained evidence; fixed control still fits its
	// original 8 KiB allowance, without another copy at installation/settlement.
	e.TargetParent = ""
	control, err := transferencoding.EncodeTransferJournal([]transferencoding.TransferEvent{e})
	if err != nil || len(control) > 2*transferencoding.MaxTransferIntentBytes {
		t.Fatal("control envelope outgrew terminal reserve", len(control), err)
	}
}
