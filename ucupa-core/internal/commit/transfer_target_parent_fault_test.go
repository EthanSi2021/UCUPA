package commit

import (
	"encoding/json"
	"errors"
	"fmt"
	bolt "go.etcd.io/bbolt"
	"sync"
	"testing"
	"ucupa/ucupa-core/contract"
	transferencoding "ucupa/ucupa-core/internal/transfer/encoding"
	transferfact "ucupa/ucupa-core/internal/transfer/fact"
)

func TestTransferMountedTargetFaultRecoveryAndConcurrentRetry(t *testing.T) {
	for _, stage := range []string{"accept", "install"} {
		for _, after := range []bool{false, true} {
			t.Run(fmt.Sprint(stage, "-", after), func(t *testing.T) {
				p, source, target, h, f, raw := targetParentCase(t, "nested-anchor")
				var decision transferfact.SourceDecision
				if stage == "install" {
					a, err := acceptWithParent(target, f, raw, h)
					if err != nil {
						t.Fatal(err)
					}
					decision, err = commitSuccessor(source, a, source.state.transfers.records[f.Input.Operation].Revision)
					if err != nil {
						t.Fatal(err)
					}
				}
				run := func() (any, error) {
					if stage == "accept" {
						return acceptWithParent(target, f, raw, h)
					}
					return installTransfer(target, decision)
				}
				before := target.state.clone()
				fault := func() error { return errors.New("target parent transaction fault") }
				if after {
					target.store.failAfterCommit = fault
				} else {
					target.store.failBeforeCommit = fault
				}
				if _, err := run(); !errors.Is(err, contract.ErrStore) || sameCompleteCoreState(before, target.state) == after {
					t.Fatal("wrong durable transaction boundary", err)
				}
				path, durable := target.store.db.Path(), target.state.clone()
				closeFoundationStore(t, target)
				target = openFoundationStore(t, path)
				defer closeFoundationStore(t, target)
				if !sameCompleteCoreState(durable, target.state) {
					t.Fatal("restart changed target outcome")
				}
				var results [8]any
				var errs [8]error
				var wg sync.WaitGroup
				for n := range results {
					wg.Add(1)
					go func() { defer wg.Done(); results[n], errs[n] = run() }()
				}
				wg.Wait()
				for n := range results {
					if errs[n] != nil || results[n] != results[0] {
						t.Fatal("retry changed original result", errs[n])
					}
				}
				if len(target.state.transfers.events) != len(before.transfers.events)+1 || p.state.mount.facts.Record(h.Input.Mount).Slot != h.Input.Slot() {
					t.Fatal("retry repeated commit or released parent without source settlement")
				}
			})
		}
	}
}

func TestTransferMountedTargetRejectsCorruptParentHistory(t *testing.T) {
	for _, mode := range []string{"missing", "role", "input", "admission", "future-source-cut", "missing-parent-cut", "unrelated-event"} {
		t.Run(mode, func(t *testing.T) {
			_, source, target, h, f, raw := targetParentCase(t, "direct-entity")
			a, err := acceptWithParent(target, f, raw, h)
			if err != nil {
				t.Fatal(err)
			}
			if mode == "unrelated-event" {
				d, err := commitSuccessor(source, a, source.state.transfers.records[f.Input.Operation].Revision)
				if err != nil {
					t.Fatal(err)
				}
				if _, err := installTransfer(target, d); err != nil {
					t.Fatal(err)
				}
			}
			path := target.store.db.Path()
			closeFoundationStore(t, target)
			mutateFoundationStore(t, path, func(tx *bolt.Tx) {
				b := tx.Bucket(elementBucketName)
				var events []transferencoding.StoredTransferEvent
				if err := json.Unmarshal(b.Get(transferJournalKey), &events); err != nil {
					t.Fatal(err)
				}
				e := &events[a.Revision-1]
				var hold transferencoding.StoredTransferParentHold
				if err := json.Unmarshal(e.TargetParent, &hold); err != nil {
					t.Fatal(err)
				}
				switch mode {
				case "role":
					hold.Input.Role = transferfact.SourceParent
				case "input":
					hold.Input.Initial.Input.Operation = []byte("other")
				case "admission":
					hold.Admission.Operation = []byte("other")
				case "future-source-cut":
					hold.Input.Initial.At.Serial = e.Fence.Cut + 1
				case "missing-parent-cut":
					hold.At = nil
				}
				e.TargetParent, err = json.Marshal(hold)
				if err != nil {
					t.Fatal(err)
				}
				if mode == "missing" {
					e.TargetParent = nil
				} else if mode == "unrelated-event" {
					events[len(events)-1].TargetParent = e.TargetParent
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
				t.Fatal("opened corrupt target parent promise")
			}
			if !errors.Is(err, contract.ErrStore) {
				t.Fatal(err)
			}
		})
	}
}
