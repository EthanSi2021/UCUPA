package commit

import (
	"encoding/json"
	"errors"
	bolt "go.etcd.io/bbolt"
	"strings"
	"sync"
	"testing"
	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	storebytes "ucupa/ucupa-core/internal/store"
	transferencoding "ucupa/ucupa-core/internal/transfer/encoding"
	transferfact "ucupa/ucupa-core/internal/transfer/fact"
)

func TestTransferClosedEdgesAcceptanceFaultAndConcurrentRetry(t *testing.T) {
	for _, after := range []bool{false, true} {
		t.Run(map[bool]string{false: "rollback", true: "response-lost"}[after], func(t *testing.T) {
			_, c, d, f, raw := edgeClosureFixture(t, 104, 30)
			closed, err := closeEdge(c, foundationStoreActor, f, raw)
			if err != nil {
				t.Fatal(err)
			}
			proofs := []transferfact.EdgeClosure{closed}
			before := d.state.clone()
			fail := func() error { return errors.New("acceptance fault") }
			if after {
				d.store.failAfterCommit = fail
			} else {
				d.store.failBeforeCommit = fail
			}
			if err := acceptClosedEdges(d, f, raw, proofs); !errors.Is(err, contract.ErrStore) {
				t.Fatal(err)
			}
			if sameCompleteCoreState(before, d.state) == after {
				t.Fatal("partial proof persistence")
			}
			path := d.store.db.Path()
			closeFoundationStore(t, d)
			d = openFoundationStore(t, path)
			defer closeFoundationStore(t, d)
			if (len(d.state.transfers.targets) == 1) != after {
				t.Fatal("restart changed durable boundary")
			}
			var wg sync.WaitGroup
			errs := make(chan error, 8)
			for n := 0; n < 8; n++ {
				wg.Add(1)
				go func() { defer wg.Done(); errs <- acceptClosedEdges(d, f, raw, proofs) }()
			}
			wg.Wait()
			close(errs)
			for err := range errs {
				if err != nil {
					t.Fatal(err)
				}
			}
			if len(d.state.transfers.targets) != 1 || len(d.state.transfers.events) != 1 || d.state.currentCut() != before.currentCut() || d.state.transfers.targets[f.Key().Transfer].closures[0] != closed {
				t.Fatal("retry duplicated or changed obligation")
			}
			if err := validateCoreState(d.state); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestTransferClosedEdgesRejectCorruptAcceptanceOnReopen(t *testing.T) {
	for _, mode := range []string{"missing", "empty", "duplicate", "wrong-mount", "wrong-range", "zero-revision", "missing-cut", "noncanonical", "foreign-event"} {
		t.Run(mode, func(t *testing.T) {
			_, c, d, f, raw := edgeClosureFixture(t, 104, 30)
			closed, err := closeEdge(c, foundationStoreActor, f, raw)
			if err != nil {
				t.Fatal(err)
			}
			if err := acceptClosedEdges(d, f, raw, []transferfact.EdgeClosure{closed}); err != nil {
				t.Fatal(err)
			}
			path := d.store.db.Path()
			closeFoundationStore(t, d)
			mutateFoundationStore(t, path, func(tx *bolt.Tx) {
				b := tx.Bucket(elementBucketName)
				var events []transferencoding.StoredTransferEvent
				if err := json.Unmarshal(b.Get(transferJournalKey), &events); err != nil {
					t.Fatal(err)
				}
				e := &events[0]
				var proofs []transferencoding.StoredEdgeClosure
				if err := json.Unmarshal(e.Closures, &proofs); err != nil {
					t.Fatal(err)
				}
				switch mode {
				case "duplicate":
					proofs = append(proofs, proofs[0])
				case "wrong-mount":
					proofs[0].Mount = foundationStoreID(101)
				case "wrong-range":
					proofs[0].Range = storebytes.EncodeFloatBits(29)
				case "zero-revision":
					proofs[0].Revision = 0
				case "missing-cut":
					proofs[0].At = nil
				}
				e.Closures, err = json.Marshal(proofs)
				if err != nil {
					t.Fatal(err)
				}
				switch mode {
				case "missing":
					e.Closures = nil
				case "empty":
					e.Closures = []byte("[]")
				case "noncanonical":
					e.Closures = append(e.Closures, ' ')
				case "foreign-event":
					e.Kind = transferencoding.TransferPending
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
				t.Fatal("opened corrupted boundary obligation")
			}
			if !errors.Is(err, contract.ErrStore) {
				t.Fatal(err)
			}
		})
	}
}

func TestTransferClosedEdgesShareOriginalRetainedCapacity(t *testing.T) {
	s := newTransferState()
	s.events = []transferencoding.TransferEvent{{Manifest: strings.Repeat("m", 3<<20), Closures: strings.Repeat("c", 5<<20)}}
	state := &coreState{transfers: s}
	if err := state.checkTransferRetainedCapacity("", ""); err != nil {
		t.Fatal("exact original limit", err)
	}
	for _, pair := range [][2]string{{"x", ""}, {"", "x"}} {
		if err := state.checkTransferRetainedCapacity(pair[0], pair[1]); !errors.Is(err, values.PublicResourceExhausted) {
			t.Fatal("closure bytes escaped capacity", err)
		}
	}
	_, c, _, f, raw := edgeClosureFixture(t, 104, 30)
	closed, err := closeEdge(c, foundationStoreActor, f, raw)
	if err != nil {
		t.Fatal(err)
	}
	closed.Previous.Key.OperationID = contract.OperationID(strings.Repeat("x", transferencoding.MaxTransferClosureBytes+1))
	if _, err := transferencoding.EncodeEdgeClosures(f, []transferfact.EdgeClosure{closed}); !errors.Is(err, values.PublicResourceExhausted) {
		t.Fatal("unbounded proof input", err)
	}
}
