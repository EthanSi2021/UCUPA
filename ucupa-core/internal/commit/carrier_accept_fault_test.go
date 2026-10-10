package commit

import (
	"encoding/json"
	"errors"
	bolt "go.etcd.io/bbolt"
	"sync"
	"testing"
	"ucupa/ucupa-core/contract"
	elementfact "ucupa/ucupa-core/internal/element/fact"
	transferencoding "ucupa/ucupa-core/internal/transfer/encoding"
	transferfact "ucupa/ucupa-core/internal/transfer/fact"
)

func TestCarrierAcceptanceStoreFailureAndConcurrentRetryKeepOneObligation(t *testing.T) {
	for _, after := range []bool{false, true} {
		t.Run(map[bool]string{false: "rollback", true: "response-lost"}[after], func(t *testing.T) {
			_, _, d, f, raw := carrierAcceptanceFixture(t, false)
			before := d.state.clone()
			fail := func() error { return errors.New("carrier target Store failure") }
			if after {
				d.store.failAfterCommit = fail
			} else {
				d.store.failBeforeCommit = fail
			}
			if _, err := acceptCarrier(d, f, raw); !errors.Is(err, contract.ErrStore) {
				t.Fatal(err)
			}
			if sameCompleteCoreState(before, d.state) == after || !d.AvailableLocked() {
				t.Fatal("wrong durable outcome")
			}
			path := d.store.db.Path()
			closeFoundationStore(t, d)
			d = openFoundationStore(t, path)
			defer closeFoundationStore(t, d)
			if len(d.state.transfers.targets) != map[bool]int{false: 0, true: 1}[after] {
				t.Fatal("reopen lost transaction boundary")
			}
			var wg sync.WaitGroup
			results := make(chan transferfact.CarrierAcceptance, 8)
			errs := make(chan error, 8)
			for n := 0; n < 8; n++ {
				wg.Add(1)
				go func() { defer wg.Done(); a, err := acceptCarrier(d, f, raw); results <- a; errs <- err }()
			}
			wg.Wait()
			close(results)
			close(errs)
			for err := range errs {
				if err != nil {
					t.Fatal(err)
				}
			}
			want := d.state.transfers.targets[f.Permit.Reservation.Input.Key()].carrier
			for a := range results {
				if a != want {
					t.Fatal("different retries", a, want)
				}
			}
			if len(d.state.transfers.events) != 1 || len(d.state.transfers.targets) != 1 || d.state.cutSerial != before.cutSerial {
				t.Fatal("retry duplicated receipt or changed space")
			}
			if err := validateCoreState(d.state); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestCarrierAcceptanceSerializesRootMountAndOtherTransfer(t *testing.T) {
	for _, mode := range []string{"root", "mount", "transfer"} {
		t.Run(mode, func(t *testing.T) {
			for n := 0; n < 6; n++ {
				_, _, d, f, raw := carrierAcceptanceFixture(t, n%2 == 0)
				var competing func() bool
				switch mode {
				case "root":
					competing = func() bool {
						_, err := applyFoundationStore(d, elementfact.OperationCreateRoot, foundationStoreActor, foundationStoreID(9500), contract.SpatialTuple{}, contract.Revision{}, "race-root")
						return err == nil
					}
				case "mount":
					p, request := competingRootMount(t, d)
					competing = func() bool {
						d.Lock()
						defer d.Unlock()
						r, err := d.ApplyMountPrepareLocked(p.authority, request, true)
						return err == nil && r.Prepared
					}
				case "transfer":
					s, _, _ := transferIntentFixture(t)
					i := rootTransferIntent(t, s, d, "race-transfer")
					f, raw := frozenSource(t, s, i)
					competing = func() bool { _, err := acceptTransfer(t, d, f, raw); return err == nil }
				}
				var accepted, other bool
				start := make(chan struct{})
				var wg sync.WaitGroup
				wg.Add(2)
				go func() { defer wg.Done(); <-start; _, err := acceptCarrier(d, f, raw); accepted = err == nil }()
				go func() { defer wg.Done(); <-start; other = competing() }()
				close(start)
				wg.Wait()
				if accepted == other {
					t.Fatal("both or neither acquired receiver", accepted, other)
				}
				if err := validateCoreState(d.state); err != nil {
					t.Fatal("invalid racing history", err)
				}
				path, durable := d.store.db.Path(), d.state.clone()
				closeFoundationStore(t, d)
				d = openFoundationStore(t, path)
				if !sameCompleteCoreState(durable, d.state) {
					t.Fatal("replay lost winning receiver")
				}
				if accepted {
					if _, err := acceptCarrier(d, f, raw); err != nil {
						t.Fatal(err)
					}
				}
				closeFoundationStore(t, d)
			}
		})
	}
}

func TestCarrierAcceptanceStoreRejectsMissingOrChangedEvidence(t *testing.T) {
	for _, mode := range []string{"fence", "permit", "source-cut", "digest", "manifest", "target", "kind", "duplicate", "foreign-field"} {
		t.Run(mode, func(t *testing.T) {
			_, _, d, f, raw := carrierAcceptanceFixture(t, false)
			if _, err := acceptCarrier(d, f, raw); err != nil {
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
				switch mode {
				case "fence":
					e.CarrierFence = nil
				case "permit":
					e.CarrierFence.Permit.PermitAt = nil
				case "source-cut":
					e.CarrierFence.FinalCut = nil
				case "digest":
					e.CarrierFence.Digest[0] ^= 1
				case "manifest":
					e.Manifest = append(e.Manifest, ' ')
				case "target":
					e.CarrierFence.Permit.Input.Target = f.Permit.Reservation.Input.Source
				case "kind":
					e.Kind = transferencoding.TransferTargetAccepted
				case "duplicate":
					events = append(events, events[0])
				case "foreign-field":
					e.CarrierPermit = &e.CarrierFence.Permit
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
				t.Fatal("opened corrupt acceptance")
			}
			if !errors.Is(err, contract.ErrStore) {
				t.Fatal(err)
			}
		})
	}
}
