package commit

import (
	"encoding/json"
	"errors"
	"go.etcd.io/bbolt"
	"path/filepath"
	"sync"
	"testing"
	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	elementfact "ucupa/ucupa-core/internal/element/fact"
	storerecord "ucupa/ucupa-core/internal/store/record"
	"ucupa/ucupa-core/internal/token"
	transferencoding "ucupa/ucupa-core/internal/transfer/encoding"
	transferfact "ucupa/ucupa-core/internal/transfer/fact"
)

func successorFixture(t *testing.T) (*Owner, *Owner, string, transferfact.TargetAcceptance) {
	t.Helper()
	source, path, input := transferIntentFixture(t)
	target := createFoundationStore(t, filepath.Join(t.TempDir(), "receiver.db"))
	t.Cleanup(func() { _ = target.CloseStore() })
	if _, err := applyFoundationStore(target, elementfact.OperationCreateRoot, input.Actor, input.Final.Parent, contract.SpatialTuple{}, contract.Revision{}, "receiver-root"); err != nil {
		t.Fatal(err)
	}
	input.Target = target.authority
	f, raw := frozenSource(t, source, input)
	a, err := acceptTransfer(t, target, f, raw)
	if err != nil {
		t.Fatal(err)
	}
	return source, target, path, a
}

func commitSuccessor(o *Owner, a transferfact.TargetAcceptance, expected uint64) (transferfact.SourceDecision, error) {
	o.Lock()
	defer o.Unlock()
	return o.CommitTransferSourceLocked(a.Fence.Input.Actor, a, expected)
}

func TestTransferSuccessorExactDurableAndStillUnavailable(t *testing.T) {
	o, target, path, a := successorFixture(t)
	i := a.Fence.Input
	before := o.state.clone()
	fenced := before.transfers.records[i.Operation]
	decision, err := commitSuccessor(o, a, fenced.Revision)
	if err != nil || decision.Acceptance != a || decision.At != before.currentCut() || decision.Revision != 3 {
		t.Fatal("commit", decision, err)
	}
	if !sameCoreFacts(before, o.state) || len(before.operations) != len(o.state.operations) || len(before.commits) != len(o.state.commits) || target.state.elements.Claimed(i.Boundary) {
		t.Fatal("commit changed facts, original outcomes or target availability")
	}
	if _, err := cancelTransfer(t, o, i, decision.Revision); !errors.Is(err, contract.ErrRevisionConflict) {
		t.Fatal("commit became cancellable", err)
	}
	fenceResult, err := fenceTransfer(o, i, 1)
	if err != nil || fenceResult != fenced {
		t.Fatal("original fence result replaced by latest progress", err)
	}
	// No target installation has happened. The source gap and ancestor holds
	// must survive the new control state and ordinary unrelated source writes.
	if !o.state.transfers.records[i.Operation].SourceClosed() || !sourceVisibility(o.state).Inactive(i.Boundary) {
		t.Fatal("source reactivated on successor decision")
	}
	if err := o.state.guardSourceTransferScope(fenced.Origin.Parent); !errors.Is(err, values.PublicUnavailable) {
		t.Fatal("source parent released before install/settlement", err)
	}
	if _, err := applyFoundationStore(o, elementfact.OperationCreateEntity, i.Actor, foundationStoreID(1010), foundationStoreTuple(t, fenced.Origin.Parent, 0, 1), contract.Revision{}, "after-successor"); err != nil {
		t.Fatal("unrelated write", err)
	}
	if err := validateCoreState(o.state); err != nil {
		t.Fatal(err)
	}
	again, err := commitSuccessor(o, a, fenced.Revision)
	if err != nil || again != decision {
		t.Fatal("later source cut changed original decision", err)
	}
	closeFoundationStore(t, o)
	o = openFoundationStore(t, path)
	defer o.CloseStore()
	again, err = commitSuccessor(o, a, fenced.Revision)
	if err != nil || again != decision {
		t.Fatal("restart changed successor", err)
	}
	o.RLock()
	f, raw, err := o.TransferSourceFenceLocked(i.Actor, i.Operation)
	d, inspectErr := o.TransferSourceDecisionLocked(i.Actor, i.Operation)
	o.RUnlock()
	if err != nil || f != a.Fence || len(raw) == 0 || inspectErr != nil || d != decision {
		t.Fatal("restart evidence", err, inspectErr)
	}
	other := a
	other.Revision++
	if _, err := commitSuccessor(o, other, fenced.Revision); !errors.Is(err, contract.ErrOperationConflict) {
		t.Fatal("another acceptance replaced successor", err)
	}
	if _, err := commitSuccessor(o, a, decision.Revision); !errors.Is(err, contract.ErrRevisionConflict) {
		t.Fatal("retry silently refreshed original CAS", err)
	}
}

func TestTransferSuccessorRejectsWrongEvidenceWithoutChange(t *testing.T) {
	o, _, _, a := successorFixture(t)
	for name, mutate := range map[string]func(*transferfact.TargetAcceptance){
		"digest": func(a *transferfact.TargetAcceptance) { a.Fence.ManifestDigest[0] ^= 1 },
		"final-cut": func(a *transferfact.TargetAcceptance) {
			a.Fence.FinalCut = token.FromParts(token.Scope(a.Fence.FinalCut), token.Serial(a.Fence.FinalCut)+1)
		},
		"target":               func(a *transferfact.TargetAcceptance) { a.Fence.Input.Target, _ = values.NewFactAuthorityRef() },
		"placement":            func(a *transferfact.TargetAcceptance) { a.Fence.Input.Final.LocalPose.Translation.X++ },
		"address":              func(a *transferfact.TargetAcceptance) { a.Fence.Input.TargetAddress = "local:another" },
		"acceptance-cut":       func(a *transferfact.TargetAcceptance) { a.At = contract.CoreReadCut{} },
		"source-as-target-cut": func(a *transferfact.TargetAcceptance) { a.At = a.Fence.FinalCut },
		"acceptance-revision":  func(a *transferfact.TargetAcceptance) { a.Revision = 0 },
	} {
		t.Run(name, func(t *testing.T) {
			bad := a
			mutate(&bad)
			before := o.state.clone()
			if _, err := commitSuccessor(o, bad, 2); err == nil || !sameCompleteCoreState(before, o.state) {
				t.Fatal("accepted bad successor or changed state", err)
			}
		})
	}
	o.Lock()
	_, err := o.CommitTransferSourceLocked(values.PrincipalRef{Realm: "realm", Principal: "imposter"}, a, 2)
	o.Unlock()
	if !errors.Is(err, values.PublicDenied) {
		t.Fatal("wrong actor", err)
	}
	if _, err := commitSuccessor(o, a, 1); !errors.Is(err, contract.ErrRevisionConflict) {
		t.Fatal("wrong comparison revision", err)
	}
}

func TestTransferSuccessorConcurrentExactRetryAndFaultRecovery(t *testing.T) {
	for _, after := range []bool{false, true} {
		t.Run(map[bool]string{false: "rollback", true: "response-lost"}[after], func(t *testing.T) {
			o, _, path, a := successorFixture(t)
			before := o.state.clone()
			fail := func() error { return errors.New("successor write uncertain") }
			if after {
				o.store.failAfterCommit = fail
			} else {
				o.store.failBeforeCommit = fail
			}
			if _, err := commitSuccessor(o, a, 2); !errors.Is(err, contract.ErrStore) {
				t.Fatal("fault", err)
			}
			if !o.AvailableLocked() || sameCompleteCoreState(before, o.state) == after {
				t.Fatal("failed to reconcile exact durable side")
			}
			durable := o.state.clone()
			closeFoundationStore(t, o)
			o = openFoundationStore(t, path)
			defer o.CloseStore()
			if !sameCompleteCoreState(durable, o.state) {
				t.Fatal("reopen differs")
			}
			const workers = 8
			var results [workers]transferfact.SourceDecision
			var errs [workers]error
			var wg sync.WaitGroup
			for index := range results {
				wg.Add(1)
				go func(index int) { defer wg.Done(); results[index], errs[index] = commitSuccessor(o, a, 2) }(index)
			}
			wg.Wait()
			for index, result := range results {
				if errs[index] != nil || result != results[0] || result.Acceptance != a {
					t.Fatal("concurrent retry differs", errs[index])
				}
			}
			if len(o.state.transfers.events) != 3 {
				t.Fatal("duplicated successor")
			}
		})
	}
}

func TestTransferSuccessorCorruptHistoryRejected(t *testing.T) {
	for _, kind := range []string{"missing-acceptance", "duplicate", "not-fenced", "wrong-expected", "actor", "target", "digest", "placement", "zero-acceptance-cut", "source-acceptance-scope", "zero-acceptance-revision", "manifest", "acceptance-on-begin"} {
		t.Run(kind, func(t *testing.T) {
			o, _, path, a := successorFixture(t)
			if _, err := commitSuccessor(o, a, 2); err != nil {
				t.Fatal(err)
			}
			closeFoundationStore(t, o)
			mutateFoundationStore(t, path, func(tx *bbolt.Tx) {
				b := tx.Bucket(elementBucketName)
				var events []transferencoding.StoredTransferEvent
				if err := json.Unmarshal(b.Get(transferJournalKey), &events); err != nil {
					t.Fatal(err)
				}
				e := &events[2]
				switch kind {
				case "missing-acceptance":
					e.Acceptance = nil
				case "duplicate":
					events = append(events, *e)
				case "not-fenced":
					events[1], events[2] = events[2], events[1]
				case "wrong-expected":
					e.Expected++
				case "actor":
					e.ActorPrincipal = []byte("different")
				case "target":
					e.Acceptance.Fence.Input.Target, _ = values.NewFactAuthorityRef()
				case "digest":
					e.Acceptance.Fence.Digest[0] ^= 1
				case "placement":
					final := storerecord.StoreTuple(foundationStoreTuple(t, a.Fence.Input.Final.Parent, 99, 20))
					e.Acceptance.Fence.Input.Final = &final
				case "zero-acceptance-cut":
					e.Acceptance.Cut = 0
				case "source-acceptance-scope":
					e.Acceptance.Scope = e.Acceptance.Fence.Scope
				case "zero-acceptance-revision":
					e.Acceptance.Revision = 0
				case "manifest":
					e.Manifest = []byte("not-a-fence")
				case "acceptance-on-begin":
					events[0].Acceptance = e.Acceptance
				}
				data, err := json.Marshal(events)
				if err != nil {
					t.Fatal(err)
				}
				if err := b.Put(transferJournalKey, data); err != nil {
					t.Fatal(err)
				}
			})
			if opened, err := OpenDurable(path, nil); err == nil {
				_ = opened.CloseStore()
				t.Fatal("accepted impossible successor history")
			}
		})
	}
}
