package commit

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	bolt "go.etcd.io/bbolt"
	"sync"
	"testing"
	"ucupa/ucupa-core/contract"
	elementfact "ucupa/ucupa-core/internal/element/fact"
	mountfact "ucupa/ucupa-core/internal/mount/fact"
	transferencoding "ucupa/ucupa-core/internal/transfer/encoding"
	transferfact "ucupa/ucupa-core/internal/transfer/fact"
)

func TestTransferExternalEdgeFaultAndConcurrentRetry(t *testing.T) {
	for _, after := range []bool{false, true} {
		t.Run(fmt.Sprint(after), func(t *testing.T) {
			_, c, _, f, raw := edgeClosureFixture(t, 104, 30)
			before := c.state.clone()
			fail := func() error { return errors.New("edge commit fault") }
			if after {
				c.store.failAfterCommit = fail
			} else {
				c.store.failBeforeCommit = fail
			}
			if _, err := closeEdge(c, foundationStoreActor, f, raw); !errors.Is(err, contract.ErrStore) || sameCompleteCoreState(before, c.state) == after {
				t.Fatal("wrong commit side", err)
			}
			path, saved := c.store.db.Path(), c.state.clone()
			closeFoundationStore(t, c)
			c = openFoundationStore(t, path)
			defer closeFoundationStore(t, c)
			if !sameCompleteCoreState(saved, c.state) {
				t.Fatal("restart lost durable closure")
			}
			var wg sync.WaitGroup
			var results [8]transferfact.EdgeClosure
			var errs [8]error
			for n := range errs {
				wg.Add(1)
				go func(n int) { defer wg.Done(); results[n], errs[n] = closeEdge(c, foundationStoreActor, f, raw) }(n)
			}
			wg.Wait()
			for n, err := range errs {
				if err != nil || results[n] != results[0] {
					t.Fatal("concurrent retry", err)
				}
			}
			if c.state.cutSerial != before.cutSerial+1 || len(c.state.transfers.events) != len(before.transfers.events)+1 {
				t.Fatal("duplicate closure")
			}
			beforeRetry := c.state.clone()
			changed := f
			changed.Source.ManifestDigest[0] ^= 1
			if _, err := closeEdge(c, foundationStoreActor, changed, raw); !errors.Is(err, contract.ErrOperationConflict) || !sameCompleteCoreState(beforeRetry, c.state) {
				t.Fatal("changed closure overwrote original", err)
			}
		})
	}
}

func TestTransferExternalEdgeClosureSerializesWithContentWrite(t *testing.T) {
	for n := 0; n < 8; n++ {
		_, c, _, f, raw := edgeClosureFixture(t, 104, 3)
		entity, _ := c.state.elements.Entity(foundationStoreID(105))
		final := foundationStoreTuple(t, f.Mount, 4, 1)
		start := make(chan struct{})
		var wg sync.WaitGroup
		var writeErr, closeErr error
		wg.Add(2)
		go func() {
			defer wg.Done()
			<-start
			_, writeErr = applyFoundationStore(c, elementfact.OperationUpdateEntity, foundationStoreActor, entity.ID, final, entity.Revision, "race-edge-range")
		}()
		go func() {
			defer wg.Done()
			<-start
			_, closeErr = closeEdge(c, foundationStoreActor, f, raw)
		}()
		close(start)
		wg.Wait()
		current, _ := c.state.elements.Entity(entity.ID)
		if closeErr == nil {
			if !errors.Is(writeErr, contract.ErrMountBusy) || current != entity || c.state.mount.facts.Incoming().Gate != mountfact.GateReparenting {
				t.Fatal("write escaped durable hold", writeErr)
			}
		} else if writeErr != nil || !errors.Is(closeErr, contract.ErrConflict) || current.LocalPose.Translation.X != 4 || c.state.mount.facts.Incoming().Gate != mountfact.GateOpen {
			t.Fatal("closure used stale range validation", writeErr, closeErr)
		}
		if err := validateCoreState(c.state); err != nil {
			t.Fatal(err)
		}
	}
}

func TestTransferExternalEdgeRejectsCorruptJournal(t *testing.T) {
	for _, mode := range []string{"missing", "wrong-member", "wrong-source", "dual-profile", "no-manifest", "foreign-field", "duplicate"} {
		t.Run(mode, func(t *testing.T) {
			_, c, d, f, raw := edgeClosureFixture(t, 104, 30)
			if _, err := closeEdge(c, foundationStoreActor, f, raw); err != nil {
				t.Fatal(err)
			}
			path := c.store.db.Path()
			closeFoundationStore(t, c)
			mutateFoundationStore(t, path, func(tx *bolt.Tx) {
				b := tx.Bucket(elementBucketName)
				var events []transferencoding.StoredTransferEvent
				if err := json.Unmarshal(b.Get(transferJournalKey), &events); err != nil {
					t.Fatal(err)
				}
				e := &events[len(events)-1]
				switch mode {
				case "missing":
					e.EdgeFence = nil
				case "wrong-member":
					e.EdgeFence.Mount = foundationStoreID(101)
				case "wrong-source":
					e.EdgeFence.Source.Input.Source = d.authority
				case "dual-profile":
					e.EdgeFence.Carrier = &transferencoding.StoredCarrierFence{}
				case "no-manifest":
					e.Manifest = nil
				case "foreign-field":
					e.Input = &e.EdgeFence.Source.Input
				case "duplicate":
					events = append(events, *e)
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
				t.Fatal("opened corrupt edge journal")
			}
			if !errors.Is(err, contract.ErrStore) {
				t.Fatal(err)
			}
		})
	}
}

func TestTransferExternalEdgeRejectsRehashedInvalidBoundary(t *testing.T) {
	_, c, _, f, raw := edgeClosureFixture(t, 104, 30)
	for _, mode := range []string{"missing", "bad-revision", "bad-contact", "bad-admission", "nil-contact"} {
		t.Run(mode, func(t *testing.T) {
			var manifest transferencoding.StoredTransferManifest
			if err := json.Unmarshal([]byte(raw), &manifest); err != nil {
				t.Fatal(err)
			}
			member := &manifest.Members[0]
			switch mode {
			case "missing":
				member.Mount = nil
			case "bad-revision":
				member.Mount.Revision = nil
			case "bad-contact":
				member.Mount.NextHop = ""
			case "bad-admission":
				member.Mount.Admission = nil
			case "nil-contact":
				member.Mount.Contact = nil
			}
			data, err := json.Marshal(manifest)
			if err != nil {
				t.Fatal(err)
			}
			bad := f
			bad.Source.ManifestDigest = sha256.Sum256(data)
			before := c.state.clone()
			if _, err := closeEdge(c, foundationStoreActor, bad, string(data)); err == nil || !sameCompleteCoreState(before, c.state) {
				t.Fatal("forged boundary accepted", err)
			}
		})
	}
}
