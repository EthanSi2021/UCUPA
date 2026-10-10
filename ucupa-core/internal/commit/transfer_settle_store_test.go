package commit

import (
	"encoding/json"
	"errors"
	"go.etcd.io/bbolt"
	"strings"
	"testing"
	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	"ucupa/ucupa-core/internal/token"
	transferencoding "ucupa/ucupa-core/internal/transfer/encoding"
	transferfact "ucupa/ucupa-core/internal/transfer/fact"
)

func TestTransferSettlementCorruptHistoryRejected(t *testing.T) {
	for _, kind := range []string{"missing", "decision", "target-scope", "target-cut", "target-revision", "duplicate", "before-decision", "extra-manifest", "extra-actor", "on-begin", "missing-commit"} {
		t.Run(kind, func(t *testing.T) {
			source, _, installed := installedTransfer(t)
			if _, err := settleTransfer(source, installed); err != nil {
				t.Fatal(err)
			}
			path := source.store.db.Path()
			closeFoundationStore(t, source)
			mutateFoundationStore(t, path, func(tx *bbolt.Tx) {
				b := tx.Bucket(elementBucketName)
				var events []transferencoding.StoredTransferEvent
				if err := json.Unmarshal(b.Get(transferJournalKey), &events); err != nil {
					t.Fatal(err)
				}
				e := &events[3]
				switch kind {
				case "missing":
					e.Installation = nil
				case "decision":
					e.Installation.Decision.Revision++
				case "target-scope":
					e.Installation.Scope = e.Installation.Decision.Scope
				case "target-cut":
					e.Installation.Cut = e.Installation.Decision.Acceptance.Cut
				case "target-revision":
					e.Installation.Revision = e.Installation.Decision.Acceptance.Revision
				case "duplicate":
					events = append(events, *e)
				case "before-decision":
					events[2], events[3] = events[3], events[2]
				case "extra-manifest":
					e.Manifest = []byte("not a fence")
				case "extra-actor":
					e.ActorPrincipal = []byte("not allowed")
				case "on-begin":
					events[0].Installation = e.Installation
				case "missing-commit":
					// A data-visible settlement cannot be replayed as a no-cut
					// control update, nor dropped while leaving current metadata.
					events = events[:3]
				}
				data, err := json.Marshal(events)
				if err != nil {
					t.Fatal(err)
				}
				if err := b.Put(transferJournalKey, data); err != nil {
					t.Fatal(err)
				}
			})
			opened, err := OpenDurable(path, nil)
			if opened != nil {
				_ = opened.CloseStore()
				t.Fatal("opened corrupt settlement")
			}
			if !errors.Is(err, contract.ErrStore) {
				t.Fatal("failure classification", err)
			}
		})
	}
}

func TestTransferJournalReservesSettlementEnvelopeHeadroom(t *testing.T) {
	_, _, installed := installedTransfer(t)
	i := installed.Decision.Acceptance.Fence.Input
	var scope [32]byte
	for index := range scope {
		scope[index] = 255
	}
	cut := token.FromParts(scope, ^uint64(0))
	maximum := ^uint64(0)
	i.Expected = token.RevisionFromParts(scope, i.Boundary, maximum-3)
	// Fix the largest token encoding first, then fill the remaining input room.
	// Enlarging the token after filling the principal constructs an input that
	// admission would reject, not a valid task whose completion must fit.
	for n := 1; n <= transferencoding.MaxTransferIntentBytes; n++ {
		trial := i
		trial.Actor.Principal = values.PrincipalID(strings.Repeat("p", n))
		if _, err := transferencoding.EncodeTransferIntent(trial); err != nil {
			break
		}
		i = trial
	}
	if _, err := transferencoding.EncodeTransferIntent(i); err != nil {
		t.Fatal("maximum admitted input", err)
	}
	installed.Decision.Acceptance.Fence.Input = i
	installed.Decision.Acceptance.Fence.FinalCut = token.FromParts(scope, maximum-2)
	installed.Decision.Acceptance.Fence.ManifestDigest = scope
	targetScope := scope
	targetScope[0] = 254
	installed.Decision.Acceptance.At = token.FromParts(targetScope, maximum-1)
	installed.Decision.At, installed.At = token.FromParts(scope, maximum-1), token.FromParts(targetScope, maximum)
	installed.Decision.Acceptance.Revision, installed.Decision.Revision, installed.Revision = maximum-1, maximum, maximum
	events := []transferencoding.TransferEvent{
		{Kind: transferencoding.TransferPending, At: cut, Input: i},
		{Kind: transferencoding.TransferCancelled, At: cut, Operation: i.Operation, Actor: i.Actor, Expected: ^uint64(0)},
		{Kind: transferencoding.TransferFenced, At: cut, Operation: i.Operation, Actor: i.Actor, Expected: ^uint64(0)},
		{Kind: transferencoding.TransferTargetAccepted, At: cut, Fence: installed.Decision.Acceptance.Fence},
		{Kind: transferencoding.TransferSuccessorDecided, At: cut, Operation: i.Operation, Actor: i.Actor, Expected: ^uint64(0), Acceptance: installed.Decision.Acceptance},
		{Kind: transferencoding.TransferTargetInstalled, At: cut, Decision: installed.Decision},
		{Kind: transferencoding.TransferEdgeInstalled, At: cut, EdgeMount: i.Boundary, EdgeParent: transferfact.CoreInstallation{Source: installed}},
		{Kind: transferencoding.TransferSourceSettled, At: cut, Installation: installed, Completion: transferfact.CoreCompletion{Installed: transferfact.CoreInstallation{Source: installed}, At: installed.At, Revision: installed.Revision}},
	}
	for _, event := range events {
		encoded, err := transferencoding.EncodeTransferJournal([]transferencoding.TransferEvent{event})
		// Source commitment includes original actor separately, hence the
		// envelope allowance also covers its repeated, input-bounded identity.
		if err != nil || len(encoded) > 2*transferencoding.MaxTransferIntentBytes {
			t.Fatal("control envelope exceeds reserved completion room", event.Kind, len(encoded), err)
		}
	}
	maxBase64Manifests := (transferencoding.MaxTransferRetainedBytes + 2) / 3 * 4
	// Completion adds one compact local reference event per accepted target,
	// not another repeated transfer input. Receipt arrays share retained bytes.
	compact, err := transferencoding.EncodeTransferJournal([]transferencoding.TransferEvent{{Kind: transferencoding.TransferTargetCompleted, At: cut, Expected: maximum}})
	if err != nil || len(compact) > 256 {
		t.Fatal("compact completion envelope", len(compact), err)
	}
	if (4*transferencoding.MaxTransferIntents+2*transferencoding.MaxTransferTargets)*(2*transferencoding.MaxTransferIntentBytes+1)+transferencoding.MaxTransferTargets*256+maxBase64Manifests+2 > transferencoding.MaxTransferJournalBytes {
		t.Fatal("task admission can exhaust terminal byte headroom")
	}
}
