package commit

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"testing"
	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	elementfact "ucupa/ucupa-core/internal/element/fact"
	transferencoding "ucupa/ucupa-core/internal/transfer/encoding"
	transferfact "ucupa/ucupa-core/internal/transfer/fact"
)

func returningTransfer(t *testing.T) (*Owner, *Owner, transferfact.SourceFence, string) {
	t.Helper()
	a, _, i := transferIntentFixture(t)
	b := transferReceiver(t, foundationStoreID(999))
	parent, _ := a.state.elements.ParentOf(i.Boundary)
	f, raw := outgoingTransfer(t, a, b, i.Boundary, foundationStoreID(999), "out")
	completeFencedTransfer(t, a, b, f, raw)
	f, raw = outgoingTransfer(t, b, a, i.Boundary, parent, "return")
	return b, a, f, raw
}

func changedLineage(t *testing.T, f transferfact.SourceFence, raw string, change func(*transferencoding.StoredTransferManifest)) (transferfact.SourceFence, string) {
	t.Helper()
	var manifest transferencoding.StoredTransferManifest
	if err := json.Unmarshal([]byte(raw), &manifest); err != nil {
		t.Fatal(err)
	}
	change(&manifest)
	bytes, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	f.ManifestDigest = sha256.Sum256(bytes)
	return f, string(bytes)
}

func TestTransferReturnRejectsMissingOrAlteredKnownDeparture(t *testing.T) {
	_, target, f, raw := returningTransfer(t)
	for name, change := range map[string]func(*transferencoding.StoredTransferManifest){
		"missing-controller":   func(m *transferencoding.StoredTransferManifest) { m.Members[0].Lineage[0].Controller = "" },
		"wrong-controller":     func(m *transferencoding.StoredTransferManifest) { m.Members[0].Lineage[0].Controller = f.Input.Source },
		"missing-member-chain": func(m *transferencoding.StoredTransferManifest) { m.Members[1].Lineage = nil },
		"wrong-operation": func(m *transferencoding.StoredTransferManifest) {
			m.Members[0].Lineage[0].Operation = []byte("not-out")
		},
		"wrong-decision":     func(m *transferencoding.StoredTransferManifest) { m.Members[0].Lineage[0].Decision[0] ^= 1 },
		"wrong-installation": func(m *transferencoding.StoredTransferManifest) { m.Members[0].Lineage[0].Installation[0] ^= 1 },
		"wrong-address": func(m *transferencoding.StoredTransferManifest) {
			m.Members[0].Lineage[0].TargetAddress = "local:other"
		},
		"foreign-member": func(m *transferencoding.StoredTransferManifest) { m.Members[0].Lineage = m.Members[1].Lineage },
		"duplicate-step": func(m *transferencoding.StoredTransferManifest) {
			m.Members[0].Lineage = append(m.Members[0].Lineage, m.Members[0].Lineage[0])
		},
		"disconnected-owner": func(m *transferencoding.StoredTransferManifest) { m.Members[0].Lineage[0].Target = f.Input.Target },
	} {
		t.Run(name, func(t *testing.T) {
			bad, altered := changedLineage(t, f, raw, change)
			before := target.state.clone()
			// Rehashing a syntactically valid envelope does not establish
			// continuity with this receiver's actual durable departure.
			if _, err := acceptTransfer(t, target, bad, altered); err == nil || !sameCompleteCoreState(before, target.state) {
				t.Fatal("changed proof adopted UUID or left a reservation", err)
			}
		})
	}
	if _, err := acceptTransfer(t, target, f, raw); err != nil {
		t.Fatal("negative trials poisoned genuine admission", err)
	}
}

func TestTransferReturnRejectsStaleRoundAndErasedPrefix(t *testing.T) {
	b, a, f, raw := returningTransfer(t)
	var first transferencoding.StoredTransferManifest
	if err := json.Unmarshal([]byte(raw), &first); err != nil {
		t.Fatal(err)
	}
	completeFencedTransfer(t, b, a, f, raw)
	f, raw = outgoingTransfer(t, a, b, f.Input.Boundary, foundationStoreID(999), "out-again")
	completeFencedTransfer(t, a, b, f, raw)
	f, raw = outgoingTransfer(t, b, a, f.Input.Boundary, foundationStoreID(100), "return-again")
	for _, erase := range []bool{false, true} {
		bad, altered := changedLineage(t, f, raw, func(m *transferencoding.StoredTransferManifest) {
			for index := range m.Members {
				if erase {
					// The latest real local successor by itself still cannot
					// erase the immutable prefix of a prior return cycle.
					chain := m.Members[index].Lineage
					m.Members[index].Lineage = chain[len(chain)-1:]
				} else {
					m.Members[index].Lineage = first.Members[index].Lineage
				}
			}
		})
		if _, err := transferencoding.DecodeTransferManifest(bad, altered); err != nil {
			t.Fatal("counterexample must be structurally continuous", err)
		}
		before := a.state.clone()
		if _, err := acceptTransfer(t, a, bad, altered); !errors.Is(err, contract.ErrConflict) || !sameCompleteCoreState(before, a.state) {
			t.Fatal("old generation accepted", err)
		}
	}
	completeFencedTransfer(t, b, a, f, raw)
}

func TestTransferReturnWaitsForEarlierSourceSettlement(t *testing.T) {
	a, b, installed := installedTransfer(t)
	i := installed.Decision.Acceptance.Fence.Input
	f, raw := outgoingTransfer(t, b, a, i.Boundary, foundationStoreID(100), "early-return")
	before := a.state.clone()
	if _, err := acceptTransfer(t, a, f, raw); !errors.Is(err, values.PublicUnavailable) || !sameCompleteCoreState(before, a.state) {
		t.Fatal("unsettled departure bypassed", err)
	}
	if _, err := settleTransfer(a, installed); err != nil {
		t.Fatal("earlier duty cannot finish", err)
	}
	completeFencedTransfer(t, b, a, f, raw)
}

func TestTransferReturnCannotAdoptActiveOrPermanentlyDeletedUUID(t *testing.T) {
	a, _, _ := transferIntentFixture(t)
	b := transferReceiver(t, foundationStoreID(999))
	id := foundationStoreID(102)
	if _, err := applyFoundationStore(b, elementfact.OperationCreateEntity, foundationStoreActor, id, foundationStoreTuple(t, foundationStoreID(999), 0, 1), contract.Revision{}, "independent-origin"); err != nil {
		t.Fatal(err)
	}
	f, raw := outgoingTransfer(t, b, a, id, foundationStoreID(100), "collision")
	for _, deleted := range []bool{false, true} {
		if deleted {
			entity, _ := a.state.elements.Entity(id)
			out, err := applyFoundationStore(a, elementfact.OperationDeleteEntity, foundationStoreActor, id, contract.SpatialTuple{}, entity.Revision, "permanent-delete")
			if err != nil || out.Status != contract.MutationCommitted {
				t.Fatal("delete fixture", err)
			}
		}
		before := a.state.clone()
		if _, err := acceptTransfer(t, a, f, raw); !errors.Is(err, contract.ErrConflict) || !sameCompleteCoreState(before, a.state) {
			t.Fatal("active/dead UUID adopted", deleted, err)
		}
	}
}
