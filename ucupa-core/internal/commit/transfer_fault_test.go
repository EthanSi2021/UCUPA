package commit

import (
	"encoding/json"
	"errors"
	"go.etcd.io/bbolt"
	"testing"
	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	elementfact "ucupa/ucupa-core/internal/element/fact"
	storerecord "ucupa/ucupa-core/internal/store/record"
	transferencoding "ucupa/ucupa-core/internal/transfer/encoding"
	transferfact "ucupa/ucupa-core/internal/transfer/fact"
)

func TestTransferIntentAndCancelFaultBoundaries(t *testing.T) {
	for _, step := range []string{"begin", "cancel", "fence"} {
		for _, after := range []bool{false, true} {
			t.Run(step+map[bool]string{false: "-rollback", true: "-response-lost"}[after], func(t *testing.T) {
				o, path, i := transferIntentFixture(t)
				var r transferfact.IntentRecord
				var err error
				if step != "begin" {
					r, err = beginTransfer(t, o, i)
					if err != nil {
						t.Fatal(err)
					}
				}
				before := o.state.clone()
				fail := func() error { return errors.New("lost Store response") }
				if after {
					o.store.failAfterCommit = fail
				} else {
					o.store.failBeforeCommit = fail
				}
				if step == "begin" {
					_, err = beginTransfer(t, o, i)
				} else if step == "fence" {
					_, err = fenceTransfer(o, i, r.Revision)
				} else {
					_, err = cancelTransfer(t, o, i, r.Revision)
				}
				if !errors.Is(err, contract.ErrStore) {
					t.Fatalf("uncertain result: %v", err)
				}
				if !o.AvailableLocked() || (sameCompleteCoreState(before, o.state) == after) {
					t.Fatal("did not choose the exact durable side")
				}
				durable := o.state.clone()
				closeFoundationStore(t, o)
				reopened := openFoundationStore(t, path)
				defer reopened.CloseStore()
				if !sameCompleteCoreState(durable, reopened.state) {
					t.Fatal("reopen changed chosen result")
				}
				if step == "begin" {
					r, err = beginTransfer(t, reopened, i)
				} else if step == "fence" {
					r, err = fenceTransfer(reopened, i, r.Revision)
				} else {
					r, err = cancelTransfer(t, reopened, i, r.Revision)
				}
				if err != nil || (step == "cancel" && r.Progress != transferfact.IntentCancelled) || (step == "fence" && r.Progress != transferfact.IntentFenced) {
					t.Fatalf("exact recovery: %+v %v", r, err)
				}
				want := 1
				if step != "begin" {
					want = 2
				}
				if len(reopened.state.transfers.events) != want {
					t.Fatal("retry duplicated original decision")
				}
			})
		}
	}
}

func TestTransferJournalRejectsMissingAndImpossibleHistory(t *testing.T) {
	for _, kind := range []string{"missing", "null", "duplicate", "future-cut", "backwards", "source", "unknown", "unissued", "ordinary-id", "cancel-without-begin", "cancel-wrong-view", "cancel-with-input", "extra-field"} {
		t.Run(kind, func(t *testing.T) {
			o, path, i := transferIntentFixture(t)
			r, err := beginTransfer(t, o, i)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := cancelTransfer(t, o, i, r.Revision); err != nil {
				t.Fatal(err)
			}
			closeFoundationStore(t, o)
			mutateFoundationStore(t, path, func(tx *bbolt.Tx) {
				b := tx.Bucket(elementBucketName)
				if kind == "missing" {
					if err := b.Delete(transferJournalKey); err != nil {
						t.Fatal(err)
					}
					return
				}
				if kind == "null" {
					if err := b.Put(transferJournalKey, []byte("null")); err != nil {
						t.Fatal(err)
					}
					return
				}
				var events []transferencoding.StoredTransferEvent
				if err := json.Unmarshal(b.Get(transferJournalKey), &events); err != nil {
					t.Fatal(err)
				}
				switch kind {
				case "duplicate":
					events = append(events[:1], events[0])
				case "future-cut":
					events[0].Cut++
				case "backwards":
					events[1].Cut--
				case "source":
					events[0].Input.Source = events[0].Input.Target
				case "unknown":
					events[0].Kind = 99
				case "unissued":
					events[0].Input.Expected.Serial += 100
				case "ordinary-id":
					events[0].Input.Operation = []byte("fixture-root")
				case "cancel-without-begin":
					events = events[1:]
				case "cancel-wrong-view":
					events[1].Expected++
				case "cancel-with-input":
					events[1].Input = events[0].Input
				}
				data, err := json.Marshal(events)
				if err != nil {
					t.Fatal(err)
				}
				if kind == "extra-field" {
					data = append([]byte(`[{"unexpected":true,`), data[2:]...)
				}
				if err := b.Put(transferJournalKey, data); err != nil {
					t.Fatal(err)
				}
			})
			opened, err := OpenDurable(path, nil)
			if opened != nil {
				_ = opened.CloseStore()
				t.Fatal("opened impossible transfer history")
			}
			if !errors.Is(err, contract.ErrStore) {
				t.Fatalf("invalid history did not fail as Store: %v", err)
			}
		})
	}
}

func TestTransferFaultDoesNotReconcileAnotherValidIntent(t *testing.T) {
	o, _, i := transferIntentFixture(t)
	other, err := values.NewFactAuthorityRef()
	if err != nil {
		t.Fatal(err)
	}
	o.store.failAfterCommit = func() error {
		if err := o.store.db.Update(func(tx *bbolt.Tx) error {
			b := tx.Bucket(elementBucketName)
			var events []transferencoding.StoredTransferEvent
			if err := json.Unmarshal(b.Get(transferJournalKey), &events); err != nil {
				return err
			}
			events[0].Input.Target = other
			data, err := json.Marshal(events)
			if err != nil {
				return err
			}
			return b.Put(transferJournalKey, data)
		}); err != nil {
			return err
		}
		return errors.New("response lost with a different valid state")
	}
	if _, err := beginTransfer(t, o, i); !errors.Is(err, contract.ErrStore) {
		t.Fatal(err)
	}
	if o.state != nil || o.AvailableLocked() {
		t.Fatal("owner accepted a third valid transfer state")
	}
}

func TestTransferFenceStoreRejectsDetachedOrForgedEvidence(t *testing.T) {
	for _, kind := range []string{"manifest-member", "manifest-input", "manifest-cut", "manifest-absent", "fence-before", "commit-index", "commit-kind", "departure-operation", "departure-deleted", "departure-missing", "cancel-after-fence"} {
		t.Run(kind, func(t *testing.T) {
			o, path, i := transferIntentFixture(t)
			r, err := beginTransfer(t, o, i)
			if err != nil {
				t.Fatal(err)
			}
			fenced, err := fenceTransfer(o, i, r.Revision)
			if err != nil {
				t.Fatal(err)
			}
			lastCut := o.state.cutSerial
			closeFoundationStore(t, o)
			mutateFoundationStore(t, path, func(tx *bbolt.Tx) {
				b := tx.Bucket(elementBucketName)
				var events []transferencoding.StoredTransferEvent
				if err := json.Unmarshal(b.Get(transferJournalKey), &events); err != nil {
					t.Fatal(err)
				}
				var m transferencoding.StoredTransferManifest
				if err := json.Unmarshal(events[1].Manifest, &m); err != nil {
					t.Fatal(err)
				}
				switch kind {
				case "manifest-member":
					m.Members = m.Members[:1]
					events[1].Manifest, err = json.Marshal(m)
				case "manifest-input":
					m.IntentDigest[0] ^= 1
					events[1].Manifest, err = json.Marshal(m)
				case "manifest-cut":
					m.FinalCut++
					events[1].Manifest, err = json.Marshal(m)
				case "manifest-absent":
					events[1].Manifest = nil
				case "fence-before":
					events[1].Cut--
				case "commit-index":
					putFoundationStoreJSON(t, tx.Bucket(commitBucketName), foundationCutKey(lastCut), storedCommit{Kind: "transfer", ControlIndex: 1})
				case "commit-kind":
					putFoundationStoreJSON(t, tx.Bucket(commitBucketName), foundationCutKey(lastCut), storedCommit{Kind: "attachment", ControlIndex: 2})
				case "departure-operation", "departure-deleted", "departure-missing":
					var key []byte
					// Locate by the original element codec, without relying on key layout.
					if err := b.ForEach(func(k, v []byte) error {
						var value storerecord.StoredElement
						if json.Unmarshal(v, &value) == nil && value.Kind == "departure" {
							key = append([]byte(nil), k...)
						}
						return nil
					}); err != nil {
						t.Fatal(err)
					}
					if key == nil {
						t.Fatal("fixture has no durable departure")
					}
					var value storerecord.StoredElement
					if err := json.Unmarshal(b.Get(key), &value); err != nil {
						t.Fatal(err)
					}
					if kind == "departure-missing" {
						if err := b.Delete(key); err != nil {
							t.Fatal(err)
						}
						break
					}
					if kind == "departure-operation" {
						value.Departure = []byte("different")
					} else {
						value.Kind = "tombstone"
						value.Departure = nil
					}
					putFoundationStoreJSON(t, b, key, value)
				case "cancel-after-fence":
					events = append(events, transferencoding.StoredTransferEvent{Kind: transferencoding.TransferCancelled, Cut: lastCut, Operation: []byte(i.Operation), ActorRealm: []byte(i.Actor.Realm), ActorPrincipal: []byte(i.Actor.Principal), Expected: fenced.Revision})
				}
				if err != nil {
					t.Fatal(err)
				}
				putFoundationStoreJSON(t, b, transferJournalKey, events)
			})
			opened, err := OpenDurable(path, nil)
			if opened != nil {
				_ = opened.CloseStore()
				t.Fatal("opened forged fence")
			}
			if !errors.Is(err, contract.ErrStore) {
				t.Fatalf("corruption accepted: %v", err)
			}
		})
	}
}

func TestTransferIntentCannotBypassWriterFence(t *testing.T) {
	o, path, cfg, installation := writerFixture(t)
	root, leaf := foundationStoreID(901), foundationStoreID(902)
	if _, err := applyFoundationStore(o, elementfact.OperationCreateRoot, foundationStoreActor, root, contract.SpatialTuple{}, contract.Revision{}, "writer-root"); err != nil {
		t.Fatal(err)
	}
	created, err := applyFoundationStore(o, elementfact.OperationCreateEntity, foundationStoreActor, leaf, foundationStoreTuple(t, root, 0, 1), contract.Revision{}, "writer-leaf")
	if err != nil {
		t.Fatal(err)
	}
	target, err := values.NewFactAuthorityRef()
	if err != nil {
		t.Fatal(err)
	}
	i := transferfact.Intent{Profile: transferfact.SliceTransfer, Operation: "transfer-before-handoff", Actor: foundationStoreActor,
		Source: o.authority, Target: target, Boundary: leaf, Expected: created.Record.Entity.Revision,
		Final: foundationStoreTuple(t, foundationStoreID(903), 0, 1), SourceAddress: "local:source", TargetAddress: "local:target"}
	r, err := beginTransfer(t, o, i)
	if err != nil {
		t.Fatal(err)
	}
	o.Lock()
	_, err = o.HandoffWriterLocked(foundationStoreActor, writerRequest(installation, 1))
	o.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := beginTransfer(t, o, i); !errors.Is(err, values.PublicUnavailable) {
		t.Fatalf("retired begin: %v", err)
	}
	if _, err := cancelTransfer(t, o, i, r.Revision); !errors.Is(err, values.PublicUnavailable) {
		t.Fatalf("retired cancel: %v", err)
	}
	if _, err := fenceTransfer(o, i, r.Revision); !errors.Is(err, values.PublicUnavailable) {
		t.Fatalf("retired source fence: %v", err)
	}
	o.RLock()
	_, err = o.TransferIntentLocked(i.Operation)
	o.RUnlock()
	if !errors.Is(err, values.PublicUnavailable) {
		t.Fatalf("retired disclosure: %v", err)
	}
	closeFoundationStore(t, o)
	nextConfig := *cfg
	nextConfig.Install = &installation
	next, err := OpenDurable(path, &nextConfig)
	if err != nil {
		t.Fatal(err)
	}
	defer next.CloseStore()
	again, err := beginTransfer(t, next, i)
	if err != nil || again != r {
		t.Fatalf("designated writer lost transfer lineage: %+v %v", again, err)
	}
	fenced, err := fenceTransfer(next, i, r.Revision)
	if err != nil || fenced.Progress != transferfact.IntentFenced {
		t.Fatalf("designated source fence: %+v %v", fenced, err)
	}
}
