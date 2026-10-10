package commit

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	bolt "go.etcd.io/bbolt"
	"testing"
	"ucupa/ucupa-core/contract"
	transferencoding "ucupa/ucupa-core/internal/transfer/encoding"
)

func TestCarrierSettlementRejectsCorruptJournal(t *testing.T) {
	for _, parent := range []bool{false, true} {
		for _, mode := range []string{"missing", "changed-permit", "invalid-target-cut", "foreign-field", "duplicate", "wrong-role"} {
			t.Run(fmt.Sprintf("parent=%v/%s", parent, mode), func(t *testing.T) {
				p, s, d, b := boundCarrierFixture(t, false)
				i, err := installCarrier(d, b)
				if err != nil {
					t.Fatal(err)
				}
				o := s
				if parent {
					o = p
				}
				if _, err := settleCarrier(o, i, parent); err != nil {
					t.Fatal(err)
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
					switch mode {
					case "missing":
						e.CarrierInstallation = nil
					case "changed-permit":
						e.CarrierInstallation.Binding.Acceptance.Fence.Permit.Input.TargetAddress = "local:wrong"
					case "invalid-target-cut":
						e.CarrierInstallation.At = e.CarrierInstallation.Binding.At
					case "foreign-field":
						e.CarrierBinding = &e.CarrierInstallation.Binding
					case "duplicate":
						events = append(events, *e)
					case "wrong-role":
						if parent {
							e.Kind = transferencoding.TransferCarrierSourceSettled
						} else {
							e.Kind = transferencoding.TransferCarrierParentSettled
						}
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
					t.Fatal("opened corrupt settlement")
				}
				if !errors.Is(err, contract.ErrStore) {
					t.Fatal(err)
				}
			})
		}
	}
}

func TestCarrierReturnRejectsRehashedFalseCustody(t *testing.T) {
	p, s, d, b := boundCarrierFixture(t, false)
	i, err := installCarrier(d, b)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := settleCarrier(p, i, true); err != nil {
		t.Fatal(err)
	}
	if _, err := settleCarrier(s, i, false); err != nil {
		t.Fatal(err)
	}
	f, raw := nextCarrierFence(t, p, d, s, i, "return-proof")
	for _, mode := range []string{"missing", "controller", "decision", "installation", "operation", "address"} {
		t.Run(mode, func(t *testing.T) {
			var manifest transferencoding.StoredTransferManifest
			if err := json.Unmarshal([]byte(raw), &manifest); err != nil {
				t.Fatal(err)
			}
			step := &manifest.Members[0].Lineage[0]
			switch mode {
			case "missing":
				manifest.Members[0].Lineage = nil
			case "controller":
				step.Controller = d.authority
			case "decision":
				step.Decision[0] ^= 1
			case "installation":
				step.Installation[0] ^= 1
			case "operation":
				step.Operation = []byte("unrelated")
			case "address":
				step.TargetAddress = "local:unrelated"
			}
			data, err := json.Marshal(manifest)
			if err != nil {
				t.Fatal(err)
			}
			bad := f
			bad.ManifestDigest = sha256.Sum256(data)
			before := s.state.clone()
			if _, err := acceptCarrier(s, bad, string(data)); err == nil || !sameCompleteCoreState(before, s.state) {
				t.Fatal("forged custody accepted", err)
			}
		})
	}
	if _, err := acceptCarrier(s, f, raw); err != nil {
		t.Fatal("rejection poisoned genuine return", err)
	}
}
