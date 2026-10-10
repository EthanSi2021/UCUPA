package commit

import (
	"errors"
	"strings"
	"testing"
	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	"ucupa/ucupa-core/internal/token"
	transferencoding "ucupa/ucupa-core/internal/transfer/encoding"
	transferfact "ucupa/ucupa-core/internal/transfer/fact"
)

func TestTransferParentRetainsOpaqueInputBytes(t *testing.T) {
	p, source, _, i := transferParentFixture(t)
	in := i.Initial.Input
	in.Operation = contract.OperationID("parent-\xff\x00")
	in.Actor = values.PrincipalRef{Realm: "realm-\xff", Principal: "principal-\xfe\x00"}
	var err error
	i.Initial, err = beginTransfer(t, source, in)
	if err != nil {
		t.Fatal(err)
	}
	h, err := reserveTransferParent(p, i)
	if err != nil {
		t.Fatal(err)
	}
	cancelled, err := cancelTransfer(t, source, in, i.Initial.Revision)
	if err != nil {
		t.Fatal(err)
	}
	released, err := releaseTransferParent(p, h, transferfact.ParentRelease{Cancelled: cancelled})
	if err != nil {
		t.Fatal(err)
	}
	path, state := p.store.db.Path(), p.state.clone()
	closeFoundationStore(t, p)
	p = openFoundationStore(t, path)
	defer closeFoundationStore(t, p)
	p.Lock()
	got, err := p.TransferParentLocked(in.Actor, i.Slot())
	p.Unlock()
	if err != nil || got != released || !sameCompleteCoreState(state, p.state) {
		t.Fatal("restart changed opaque caller or operation bytes", err)
	}
}

func TestTransferParentRejectsOversizeBeforeClaim(t *testing.T) {
	p, _, _, i := transferParentFixture(t)
	before := p.state.clone()
	// The complete parent envelope, not merely the nested source input, must
	// fit admission. No durable state or private slot is claimed on rejection.
	for n := 1; ; n++ {
		i.Initial.Input.Actor.Principal = values.PrincipalID(strings.Repeat("p", n))
		if _, err := transferencoding.EncodeTransferParentIntent(i); errors.Is(err, values.PublicResourceExhausted) {
			break
		} else if err != nil || n > transferencoding.MaxTransferIntentBytes {
			t.Fatal("failed to reach finite envelope bound", n, err)
		}
	}
	if _, err := transferencoding.EncodeTransferIntent(i.Initial.Input); err != nil {
		t.Fatal("test must isolate parent envelope overhead", err)
	}
	if _, err := reserveTransferParent(p, i); !errors.Is(err, values.PublicResourceExhausted) || !sameCompleteCoreState(before, p.state) {
		t.Fatal("oversize envelope claimed parent", err)
	}
}

func TestTransferParentTerminalFitsExistingJournalReserve(t *testing.T) {
	p, _, _, i := transferParentFixture(t)
	h, err := reserveTransferParent(p, i)
	if err != nil {
		t.Fatal(err)
	}
	// Codec upper bounds, not fabricated live transfer evidence. Both the
	// largest tokens and source input are fixed before filling the envelope.
	maximum := ^uint64(0)
	var sourceScope, parentScope, targetScope [32]byte
	for n := range sourceScope {
		sourceScope[n], parentScope[n], targetScope[n] = 255, 254, 253
	}
	i.Initial.Input.Operation = contract.OperationID(strings.Repeat("o", 128))
	i.Initial.Input.Expected = token.RevisionFromParts(sourceScope, i.Initial.Input.Boundary, maximum-7)
	i.Initial.Admission, i.Initial.Revision = token.FromParts(sourceScope, maximum-6), maximum-6
	i.ExpectedElement = token.RevisionFromParts(parentScope, i.Mount, maximum-4)
	i.ExpectedMount = token.MountRevisionFromParts(parentScope, i.Mount, maximum-3)
	for n := 1; n <= transferencoding.MaxTransferIntentBytes; n++ {
		trial := i
		trial.Initial.Input.Actor.Principal = values.PrincipalID(strings.Repeat("p", n))
		if _, err := transferencoding.EncodeTransferParentIntent(trial); err != nil {
			break
		}
		i = trial
	}
	if _, err := transferencoding.EncodeTransferParentIntent(i); err != nil {
		t.Fatal(err)
	}
	h.Input = i
	h.Admission.MountKey.ProposalRevision = token.MountRevisionFromParts(parentScope, i.Mount, maximum-5)
	h.Contact = token.NextHopRevisionFromParts(parentScope, h.Admission.MountKey, false, maximum-4)
	h.At, h.Revision = token.FromParts(parentScope, maximum-2), maximum-1
	f := transferfact.SourceFence{Input: i.Initial.Input, FinalCut: token.FromParts(sourceScope, maximum-5), ManifestDigest: sourceScope}
	a := transferfact.TargetAcceptance{Fence: f, At: token.FromParts(targetScope, maximum-3), Revision: maximum - 4}
	d := transferfact.SourceDecision{Acceptance: a, At: token.FromParts(sourceScope, maximum-4), Revision: maximum - 3}
	installed := transferfact.TargetInstallation{Decision: d, At: token.FromParts(targetScope, maximum-2), Revision: maximum - 2}
	settled := transferfact.ParentRelease{Settled: transferfact.SourceSettlement{Installation: installed, At: token.FromParts(sourceScope, maximum-3), Revision: maximum - 2}}
	c := i.Initial
	c.Progress, c.Revision = transferfact.IntentCancelled, maximum-1
	cancelled := transferfact.ParentRelease{Cancelled: c}
	if !h.Valid() || !settled.ValidFor(h) || !cancelled.ValidFor(h) {
		t.Fatal("invalid maximum codec fixture")
	}
	events := []transferencoding.TransferEvent{
		{Kind: transferencoding.TransferParentReserved, At: token.FromParts(parentScope, maximum-3), Parent: i},
		{Kind: transferencoding.TransferParentReleased, At: h.At, Operation: i.Slot(), Expected: h.Revision, ParentRelease: cancelled},
		{Kind: transferencoding.TransferParentReleased, At: h.At, Operation: i.Slot(), Expected: h.Revision, ParentRelease: settled},
	}
	for _, e := range events {
		raw, err := transferencoding.EncodeTransferJournal([]transferencoding.TransferEvent{e})
		if err != nil || len(raw) > 2*transferencoding.MaxTransferIntentBytes {
			t.Fatal("terminal exceeds existing per-event room", e.Kind, len(raw), err)
		}
		decoded, err := transferencoding.LoadTransferJournal(raw, parentScope)
		if err != nil || len(decoded) != 1 || decoded[0] != e {
			t.Fatal("parent envelope roundtrip", e.Kind, err)
		}
	}
}

func TestTransferParentUsesSharedIntentCapacity(t *testing.T) {
	p, source, _, i := transferParentFixture(t)
	h, err := reserveTransferParent(p, i)
	if err != nil || p.state.transferIntentCount() != 1 {
		t.Fatal("hold not charged to existing capacity", err)
	}
	c, err := cancelTransfer(t, source, i.Initial.Input, i.Initial.Revision)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := releaseTransferParent(p, h, transferfact.ParentRelease{Cancelled: c}); err != nil || p.state.transferIntentCount() != 1 {
		t.Fatal("terminal freed a retained identity", err)
	}
	// Capacity arithmetic on a disposable candidate, never a published Store.
	candidate := p.state.clone()
	for n := range transferencoding.MaxTransferIntents - 1 {
		candidate.transfers.records[contract.OperationID(strings.Repeat("i", n+1))] = transferfact.IntentRecord{}
	}
	i.Initial.Input.Operation = "another-parent-hold"
	i.ExpectedMount = candidate.mount.facts.Record(i.Mount).Revision
	if err := candidate.reserveTransferParent(transferencoding.TransferEvent{Kind: transferencoding.TransferParentReserved, Parent: i}); !errors.Is(err, values.PublicResourceExhausted) {
		t.Fatal("parent hold bypassed existing capacity", err)
	}
}
