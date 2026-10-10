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
)

func TestCarrierBindingAndInstallationStoreFaultAndConcurrentRetry(t *testing.T) {
	for _, stage := range []string{"binding", "installation"} {
		for _, after := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/after=%v", stage, after), func(t *testing.T) {
				p, _, d, f, raw := carrierAcceptanceFixture(t, false)
				a, err := acceptCarrier(d, f, raw)
				if err != nil {
					t.Fatal(err)
				}
				o := p
				call := func() error { _, err := bindCarrier(o, a); return err }
				if stage == "installation" {
					b, err := bindCarrier(p, a)
					if err != nil {
						t.Fatal(err)
					}
					o = d
					call = func() error { _, err := installCarrier(o, b); return err }
				}
				before := o.state.clone()
				fail := func() error { return errors.New("carrier durable failure") }
				if after {
					o.store.failAfterCommit = fail
				} else {
					o.store.failBeforeCommit = fail
				}
				if err := call(); !errors.Is(err, contract.ErrStore) || sameCompleteCoreState(before, o.state) == after || !o.AvailableLocked() {
					t.Fatal("wrong durable side", err)
				}
				path, durable := o.store.db.Path(), o.state.clone()
				closeFoundationStore(t, o)
				o = openFoundationStore(t, path)
				defer closeFoundationStore(t, o)
				if !sameCompleteCoreState(durable, o.state) {
					t.Fatal("reopen changed durable outcome")
				}
				var wg sync.WaitGroup
				var errs [8]error
				for n := range errs {
					wg.Add(1)
					go func(n int) { defer wg.Done(); errs[n] = call() }(n)
				}
				wg.Wait()
				for _, err := range errs {
					if err != nil {
						t.Fatal("concurrent retry", err)
					}
				}
				if len(o.state.transfers.events) != len(before.transfers.events)+1 || o.state.cutSerial != before.cutSerial+1 {
					t.Fatal("more than one installation/decision")
				}
				if err := validateCoreState(o.state); err != nil {
					t.Fatal(err)
				}
			})
		}
	}
}

func TestCarrierBindingAndInstallationRejectCorruptJournal(t *testing.T) {
	for _, stage := range []string{"binding", "installation"} {
		for _, mode := range []string{"missing", "changed-acceptance", "source-cut", "parent-cut", "target", "manifest", "foreign-field", "duplicate"} {
			t.Run(stage+"/"+mode, func(t *testing.T) {
				p, _, d, b := boundCarrierFixture(t, false)
				o := p
				if stage == "installation" {
					if _, err := installCarrier(d, b); err != nil {
						t.Fatal(err)
					}
					o = d
				}
				path := o.store.db.Path()
				closeFoundationStore(t, o)
				mutateFoundationStore(t, path, func(tx *bolt.Tx) {
					bucket := tx.Bucket(elementBucketName)
					var events []transferencoding.StoredTransferEvent
					if err := json.Unmarshal(bucket.Get(transferJournalKey), &events); err != nil {
						t.Fatal(err)
					}
					e := &events[len(events)-1]
					a := e.CarrierAcceptance
					if stage == "installation" {
						a = &e.CarrierBinding.Acceptance
					}
					switch mode {
					case "missing":
						e.CarrierAcceptance = nil
						e.CarrierBinding = nil
					case "changed-acceptance":
						a.Fence.Permit.Input.TargetAddress = "local:substituted"
					case "source-cut":
						a.Fence.FinalCut = nil
					case "parent-cut":
						a.Fence.Permit.PermitAt = nil
					case "target":
						a.Fence.Permit.Input.Target = p.authority
					case "manifest":
						e.Manifest = []byte("unexpected")
					case "foreign-field":
						e.CarrierFence = &a.Fence
					case "duplicate":
						events = append(events, *e)
					}
					data, err := json.Marshal(events)
					if err != nil {
						t.Fatal(err)
					}
					if err := bucket.Put(transferJournalKey, data); err != nil {
						t.Fatal(err)
					}
				})
				opened, err := OpenDurable(path, nil)
				if opened != nil {
					_ = opened.CloseStore()
					t.Fatal("opened corrupt carrier decision")
				}
				if !errors.Is(err, contract.ErrStore) {
					t.Fatal(err)
				}
			})
		}
	}
}
