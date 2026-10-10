package commit

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"go.etcd.io/bbolt"
	"sync"
	"testing"
	"ucupa/ucupa-core/contract"
	elementfact "ucupa/ucupa-core/internal/element/fact"
	transferencoding "ucupa/ucupa-core/internal/transfer/encoding"
	transferfact "ucupa/ucupa-core/internal/transfer/fact"
)

func TestTransferReturnReopenRejectsRehashedContradictoryLineage(t *testing.T) {
	for _, damage := range []string{"missing", "decision", "installation", "address", "revision"} {
		t.Run(damage, func(t *testing.T) {
			_, target, f, raw := returningTransfer(t)
			accepted, err := acceptTransfer(t, target, f, raw)
			if err != nil {
				t.Fatal(err)
			}
			path := target.store.db.Path()
			closeFoundationStore(t, target)
			mutateFoundationStore(t, path, func(tx *bbolt.Tx) {
				bucket := tx.Bucket(elementBucketName)
				var events []transferencoding.StoredTransferEvent
				if err := json.Unmarshal(bucket.Get(transferJournalKey), &events); err != nil {
					t.Fatal(err)
				}
				e := &events[accepted.Revision-1]
				var manifest transferencoding.StoredTransferManifest
				if err := json.Unmarshal(e.Manifest, &manifest); err != nil {
					t.Fatal(err)
				}
				step := &manifest.Members[0].Lineage[0]
				switch damage {
				case "missing":
					manifest.Members[0].Lineage = nil
				case "decision":
					step.Decision[0] ^= 1
				case "installation":
					step.Installation[0] ^= 1
				case "address":
					step.SourceAddress = "local:substitute"
				case "revision":
					step.SourceRevision.Serial++
				}
				e.Manifest, err = json.Marshal(manifest)
				if err != nil {
					t.Fatal(err)
				}
				e.Fence.Digest = sha256.Sum256(e.Manifest)
				encoded, err := json.Marshal(events)
				if err != nil {
					t.Fatal(err)
				}
				if err := bucket.Put(transferJournalKey, encoded); err != nil {
					t.Fatal(err)
				}
			})
			opened, err := OpenDurable(path, nil)
			if opened != nil {
				_ = opened.CloseStore()
				t.Fatal("opened contradictory custody history")
			}
			if !errors.Is(err, contract.ErrStore) {
				t.Fatal("wrong rejection", err)
			}
		})
	}
}

func TestTransferReturnAtomicRestartAndConcurrentRetry(t *testing.T) {
	for _, after := range []bool{false, true} {
		t.Run(map[bool]string{false: "rollback", true: "lost-response"}[after], func(t *testing.T) {
			source, target, f, raw := returningTransfer(t)
			a, err := acceptTransfer(t, target, f, raw)
			if err != nil {
				t.Fatal(err)
			}
			d, err := commitSuccessor(source, a, source.state.transfers.records[f.Input.Operation].FenceRevision)
			if err != nil {
				t.Fatal(err)
			}
			before, path := target.state.clone(), target.store.db.Path()
			fail := func() error { return errors.New("return install failure") }
			if after {
				target.store.failAfterCommit = fail
			} else {
				target.store.failBeforeCommit = fail
			}
			if _, err := installTransfer(target, d); !errors.Is(err, contract.ErrStore) {
				t.Fatal("missing store fault", err)
			}
			if sameCompleteCoreState(before, target.state) == after {
				t.Fatal("incorrect durable outcome")
			}
			for _, id := range []uint64{101, 102} {
				_, departed := target.state.elements.Departure(foundationStoreID(id))
				if departed == after || (target.state.elements.Kind(foundationStoreID(id)) != 0) != after {
					t.Fatal("partially resumed closure")
				}
			}
			durable := target.state.clone()
			closeFoundationStore(t, target)
			target = openFoundationStore(t, path)
			defer target.CloseStore()
			if !sameCompleteCoreState(durable, target.state) {
				t.Fatal("restart changed atomic result")
			}
			var results [8]transferfact.TargetInstallation
			var errs [8]error
			var wg sync.WaitGroup
			for n := range results {
				wg.Add(1)
				go func(n int) { defer wg.Done(); results[n], errs[n] = installTransfer(target, d) }(n)
			}
			wg.Wait()
			for n := range results {
				if errs[n] != nil || results[n] != results[0] || results[n].Decision != d {
					t.Fatal("duplicate return installation", errs[n])
				}
			}
			if target.state.cutSerial != before.cutSerial+1 || target.state.revisionSerial != before.revisionSerial+2 || len(target.state.transfers.events) != len(before.transfers.events)+1 {
				t.Fatal("more than one return committed")
			}
			entity, _ := target.state.elements.Entity(foundationStoreID(102))
			out, err := applyFoundationStore(target, elementfact.OperationUpdateEntity, f.Input.Actor, entity.ID, foundationStoreTuple(t, f.Input.Boundary, 3, 1), entity.Revision, "after-return")
			if err != nil || out.Status != contract.MutationCommitted {
				t.Fatal("returned generation not writable", err)
			}
			if r, err := installTransfer(target, d); err != nil || r != results[0] {
				t.Fatal("retry lost original result", err)
			}
			current, _ := target.state.elements.Entity(entity.ID)
			if current.Revision != out.Record.Entity.Revision || current.LocalPose != out.Record.Entity.LocalPose {
				t.Fatal("retry overwrote later mutation")
			}
			if _, err := settleTransfer(source, results[0]); err != nil {
				t.Fatal(err)
			}
		})
	}
}
