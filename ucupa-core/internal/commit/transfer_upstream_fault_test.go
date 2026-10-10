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

func TestTransferMountedSourceFenceFaultRecovery(t *testing.T) {
	for _, after := range []bool{false, true} {
		t.Run(fmt.Sprint(after), func(t *testing.T) {
			p, source, _, pi := sourceParentFixture(t, "anchor")
			h, err := reserveTransferParent(p, pi)
			if err != nil {
				t.Fatal(err)
			}
			before := source.state.clone()
			fault := func() error { return errors.New("source fence fault") }
			if after {
				source.store.failAfterCommit = fault
			} else {
				source.store.failBeforeCommit = fault
			}
			if _, err := fenceWithParent(source, h); !errors.Is(err, contract.ErrStore) || sameCompleteCoreState(before, source.state) == after {
				t.Fatal("wrong source commit boundary", err)
			}
			path, durable := source.store.db.Path(), source.state.clone()
			closeFoundationStore(t, source)
			source = openFoundationStore(t, path)
			defer closeFoundationStore(t, source)
			if !sameCompleteCoreState(durable, source.state) {
				t.Fatal("restart changed source fence")
			}
			var results [8]transferfact.IntentRecord
			var errs [8]error
			var wg sync.WaitGroup
			for n := range results {
				wg.Add(1)
				go func() { defer wg.Done(); results[n], errs[n] = fenceWithParent(source, h) }()
			}
			wg.Wait()
			for n := range results {
				if errs[n] != nil || results[n] != results[0] || results[n].Progress != transferfact.IntentFenced {
					t.Fatal("retry changed source result", errs[n])
				}
			}
			if source.state.cutSerial != before.cutSerial+1 || len(source.state.transfers.events) != len(before.transfers.events)+1 || p.state.mount.facts.Record(pi.Mount).Slot != pi.Slot() {
				t.Fatal("source retries changed parent hold or repeated departure")
			}
		})
	}
}

func TestTransferMountedSourceCorruptUpstreamRejected(t *testing.T) {
	for _, mode := range []string{"missing", "different-operation", "different-admission", "future-source-cut", "role"} {
		t.Run(mode, func(t *testing.T) {
			p, source, _, pi := sourceParentFixture(t, "anchor")
			h, err := reserveTransferParent(p, pi)
			if err != nil {
				t.Fatal(err)
			}
			fenced, err := fenceWithParent(source, h)
			if err != nil {
				t.Fatal(err)
			}
			path := source.store.db.Path()
			closeFoundationStore(t, source)
			mutateFoundationStore(t, path, func(tx *bolt.Tx) {
				b := tx.Bucket(elementBucketName)
				var events []transferencoding.StoredTransferEvent
				if err := json.Unmarshal(b.Get(transferJournalKey), &events); err != nil {
					t.Fatal(err)
				}
				e := &events[fenced.FenceRevision-1]
				var m transferencoding.StoredTransferManifest
				if err := json.Unmarshal(e.Manifest, &m); err != nil {
					t.Fatal(err)
				}
				switch mode {
				case "missing":
					m.SourceParent = nil
				case "different-operation":
					m.SourceParent.Input.Initial.Input.Operation = []byte("different")
				case "different-admission":
					m.SourceParent.Input.Initial.Revision++
				case "future-source-cut":
					m.SourceParent.Input.Initial.At.Serial = m.FinalCut + 1
				case "role":
					m.SourceParent.Input.Role = transferfact.TargetParent
				}
				e.Manifest, err = json.Marshal(m)
				if err != nil {
					t.Fatal(err)
				}
				raw, err := json.Marshal(events)
				if err != nil {
					t.Fatal(err)
				}
				if err := b.Put(transferJournalKey, raw); err != nil {
					t.Fatal(err)
				}
			})
			opened, err := OpenDurable(path, nil)
			if opened != nil {
				_ = opened.CloseStore()
				t.Fatal("opened unreachable upstream fence")
			}
			if !errors.Is(err, contract.ErrStore) {
				t.Fatal("wrong corruption error", err)
			}
		})
	}
}
