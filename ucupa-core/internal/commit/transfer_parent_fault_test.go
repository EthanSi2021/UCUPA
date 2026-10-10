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

func TestTransferParentFaultsAndConcurrentRecovery(t *testing.T) {
	for _, release := range []bool{false, true} {
		for _, after := range []bool{false, true} {
			t.Run(fmt.Sprintf("release=%v/after=%v", release, after), func(t *testing.T) {
				p, s, _, i := transferParentFixture(t)
				var h transferfact.ParentHold
				var terminal transferfact.ParentRelease
				if release {
					var err error
					h, err = reserveTransferParent(p, i)
					if err != nil {
						t.Fatal(err)
					}
					terminal.Cancelled, err = cancelTransfer(t, s, i.Initial.Input, i.Initial.Revision)
					if err != nil {
						t.Fatal(err)
					}
				}
				invoke := func() (transferfact.ParentControl, error) {
					if release {
						return releaseTransferParent(p, h, terminal)
					}
					hold, err := reserveTransferParent(p, i)
					return transferfact.ParentControl{Hold: hold, Revision: hold.Revision}, err
				}
				before := p.state.clone()
				fault := func() error { return errors.New("lost parent control reply") }
				if after {
					p.store.failAfterCommit = fault
				} else {
					p.store.failBeforeCommit = fault
				}
				if _, err := invoke(); !errors.Is(err, contract.ErrStore) || sameCompleteCoreState(before, p.state) == after {
					t.Fatal("wrong durable failure side", err)
				}
				path, durable := p.store.db.Path(), p.state.clone()
				closeFoundationStore(t, p)
				p = openFoundationStore(t, path)
				defer closeFoundationStore(t, p)
				if !sameCompleteCoreState(durable, p.state) {
					t.Fatal("restart changed held relation")
				}
				candidate, valid := p.CaptureReadLocked()
				if !valid || candidate.Validate() != nil {
					t.Fatal("restart lost finite parent read proof")
				}
				_, _, held := candidate.Projection.Transfers.Counts()
				wantHeld := 0
				if release != after {
					wantHeld = 1
				}
				if held != wantHeld {
					t.Fatal("read proof disagreed with durable fault side", held, wantHeld)
				}
				var wg sync.WaitGroup
				var results [8]transferfact.ParentControl
				var errs [8]error
				for n := range errs {
					wg.Add(1)
					go func(n int) { defer wg.Done(); results[n], errs[n] = invoke() }(n)
				}
				wg.Wait()
				for n, err := range errs {
					if err != nil || !results[n].Valid() || results[n] != results[0] {
						t.Fatal("concurrent retry changed result", err)
					}
				}
				if p.state.cutSerial != before.cutSerial+1 || p.state.revisionSerial != before.revisionSerial || len(p.state.transfers.events) != len(before.transfers.events)+1 {
					t.Fatal("retry duplicated cut, event or element change")
				}
				if err := validateCoreState(p.state); err != nil {
					t.Fatal(err)
				}
			})
		}
	}
}

func TestTransferParentJournalRejectsUnreachableControl(t *testing.T) {
	for _, mode := range []string{"missing-intent", "wrong-parent", "wrong-role", "wrong-expected", "missing-release", "wrong-transfer", "not-cancelled", "wrong-sequence", "duplicate", "foreign-field"} {
		t.Run(mode, func(t *testing.T) {
			p, s, _, i := transferParentFixture(t)
			h, err := reserveTransferParent(p, i)
			if err != nil {
				t.Fatal(err)
			}
			cancelled, err := cancelTransfer(t, s, i.Initial.Input, i.Initial.Revision)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := releaseTransferParent(p, h, transferfact.ParentRelease{Cancelled: cancelled}); err != nil {
				t.Fatal(err)
			}
			path := p.store.db.Path()
			closeFoundationStore(t, p)
			mutateFoundationStore(t, path, func(tx *bolt.Tx) {
				b := tx.Bucket(elementBucketName)
				var events []transferencoding.StoredTransferEvent
				if err := json.Unmarshal(b.Get(transferJournalKey), &events); err != nil {
					t.Fatal(err)
				}
				switch mode {
				case "missing-intent":
					events[0].Parent = nil
				case "wrong-parent":
					events[0].Parent.Parent = i.Initial.Input.Source
				case "wrong-role":
					events[0].Parent.Role = transferfact.SourceParent
				case "wrong-expected":
					events[1].Expected++
				case "missing-release":
					events[1].ParentRelease = nil
				case "wrong-transfer":
					events[1].ParentRelease.Cancelled.Input.Operation = []byte("not-this-transfer")
				case "not-cancelled":
					events[1].ParentRelease.Settled = &transferencoding.StoredParentSettlement{}
				case "wrong-sequence":
					events[1].ParentRelease.Cancelled.Revision = events[0].Parent.Initial.Revision
				case "duplicate":
					events = append(events, events[1])
				case "foreign-field":
					events[1].Manifest = []byte("unrelated content")
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
				t.Fatal("opened invalid parent control")
			}
			if !errors.Is(err, contract.ErrStore) {
				t.Fatal("corruption classification", err)
			}
		})
	}
}
