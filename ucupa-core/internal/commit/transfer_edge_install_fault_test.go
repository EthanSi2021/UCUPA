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

// Actual source fence, carrier closure, target promise, source decision and
// target installation; no synthetic installation proof or writable fixture map.
func pendingEdgeInstallation(t *testing.T) (*Owner, *Owner, transferfact.CoreInstallation, transferfact.EdgeClosure) {
	t.Helper()
	p, c, d, f, raw := edgeClosureFixture(t, 104, 30)
	closed, err := closeEdge(c, foundationStoreActor, f, raw)
	if err != nil {
		t.Fatal(err)
	}
	if err := acceptClosedEdges(d, f, raw, []transferfact.EdgeClosure{closed}); err != nil {
		t.Fatal(err)
	}
	a := d.state.transfers.targets[f.Key().Transfer].receipt
	decision, err := commitSuccessor(p, a, p.state.transfers.records[f.Source.Input.Operation].Revision)
	if err != nil {
		t.Fatal(err)
	}
	i, err := installTransfer(d, decision)
	if err != nil {
		t.Fatal(err)
	}
	return c, d, transferfact.CoreInstallation{Source: i}, closed
}

func TestTransferredEdgeInstallationAndCompletionFaultRetry(t *testing.T) {
	for _, target := range []bool{false, true} {
		for _, after := range []bool{false, true} {
			t.Run(fmt.Sprintf("target=%v/after=%v", target, after), func(t *testing.T) {
				c, d, parent, closed := pendingEdgeInstallation(t)
				o := c
				var child transferfact.EdgeInstallation
				var err error
				if target {
					child, err = installClosedEdge(c, parent, closed)
					if err != nil {
						t.Fatal(err)
					}
					o = d
				}
				invoke := func() (transferfact.EdgeInstallation, transferfact.CoreCompletion, error) {
					if target {
						r, err := completeEdges(o, parent, []transferfact.EdgeInstallation{child})
						return transferfact.EdgeInstallation{}, r, err
					}
					r, err := installClosedEdge(o, parent, closed)
					return r, transferfact.CoreCompletion{}, err
				}
				before := o.state.clone()
				fail := func() error { return errors.New("edge terminal commit fault") }
				if after {
					o.store.failAfterCommit = fail
				} else {
					o.store.failBeforeCommit = fail
				}
				if _, _, err := invoke(); !errors.Is(err, contract.ErrStore) || sameCompleteCoreState(before, o.state) == after {
					t.Fatal("wrong commit failure side", err)
				}
				path, durable := o.store.db.Path(), o.state.clone()
				closeFoundationStore(t, o)
				o = openFoundationStore(t, path)
				defer closeFoundationStore(t, o)
				if !sameCompleteCoreState(durable, o.state) {
					t.Fatal("restart changed durable result")
				}
				var wg sync.WaitGroup
				var edges [8]transferfact.EdgeInstallation
				var completions [8]transferfact.CoreCompletion
				var errs [8]error
				for n := range errs {
					wg.Add(1)
					go func(n int) { defer wg.Done(); edges[n], completions[n], errs[n] = invoke() }(n)
				}
				wg.Wait()
				for n, err := range errs {
					if err != nil || edges[n] != edges[0] || completions[n] != completions[0] {
						t.Fatal("concurrent exact retry", err)
					}
				}
				if o.state.cutSerial != before.cutSerial+1 || len(o.state.transfers.events) != len(before.transfers.events)+1 || o.state.revisionSerial != before.revisionSerial {
					t.Fatal("terminal retry repeated cut or changed content revision")
				}
				if err := validateCoreState(o.state); err != nil {
					t.Fatal(err)
				}
			})
		}
	}
}

func TestTransferredEdgeTerminalJournalRejectsCorruption(t *testing.T) {
	for _, target := range []bool{false, true} {
		for _, mode := range []string{"missing", "wrong-cut", "wrong-revision", "wrong-member", "foreign-field", "duplicate"} {
			t.Run(fmt.Sprintf("target=%v/%s", target, mode), func(t *testing.T) {
				c, d, parent, closed := pendingEdgeInstallation(t)
				child, err := installClosedEdge(c, parent, closed)
				if err != nil {
					t.Fatal(err)
				}
				o := c
				if target {
					if _, err := completeEdges(d, parent, []transferfact.EdgeInstallation{child}); err != nil {
						t.Fatal(err)
					}
					o = d
				}
				path := o.store.db.Path()
				closeFoundationStore(t, o)
				mutateFoundationStore(t, path, func(tx *bolt.Tx) {
					b := tx.Bucket(elementBucketName)
					var events []transferencoding.StoredTransferEvent
					if err := json.Unmarshal(b.Get(transferJournalKey), &events); err != nil {
						t.Fatal(err)
					}
					e := &events[len(events)-1]
					if target {
						var results []transferencoding.StoredEdgeResult
						if err := json.Unmarshal(e.EdgeResults, &results); err != nil {
							t.Fatal(err)
						}
						switch mode {
						case "missing":
							results = nil
						case "wrong-cut":
							results[0].At.Serial = 0
						case "wrong-revision":
							results[0].Revision = 0
						case "wrong-member":
							results[0].Mount = foundationStoreID(999)
						case "foreign-field":
							e.Manifest = []byte("not allowed")
						case "duplicate":
							events = append(events, *e)
						}
						if mode != "duplicate" {
							e.EdgeResults, err = json.Marshal(results)
							if err != nil {
								t.Fatal(err)
							}
						}
					} else {
						switch mode {
						case "missing":
							e.EdgeInstallation = nil
						case "wrong-cut":
							e.EdgeInstallation.Parent.Source.Cut = 0
						case "wrong-revision":
							e.EdgeInstallation.Parent.Source.Revision = 0
						case "wrong-member":
							e.EdgeInstallation.Mount = foundationStoreID(999)
						case "foreign-field":
							e.EdgeResults = []byte("not allowed")
						case "duplicate":
							events = append(events, *e)
						}
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
					t.Fatal("opened corrupt terminal journal")
				}
				if !errors.Is(err, contract.ErrStore) {
					t.Fatal("corruption classification", err)
				}
			})
		}
	}
}
