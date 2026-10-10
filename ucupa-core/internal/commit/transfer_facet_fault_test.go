package commit

import (
	"errors"
	"sync"
	"testing"
	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	transferencoding "ucupa/ucupa-core/internal/transfer/encoding"
	transferfact "ucupa/ucupa-core/internal/transfer/fact"
)

func TestTransferFacetRejectsIncoherentAndReturningForgedPredecessor(t *testing.T) {
	a, _, initial := transferIntentFixture(t)
	b := transferReceiver(t, foundationStoreID(999))
	root, _ := a.state.elements.ParentOf(initial.Boundary)
	transferFacetFixture(t, a, foundationStoreID(102), contract.FacetActive)
	f, raw := outgoingTransfer(t, a, b, initial.Boundary, foundationStoreID(999), "out")
	completeFencedTransfer(t, a, b, f, raw)
	f, raw = outgoingTransfer(t, b, a, initial.Boundary, root, "back")
	for name, mutate := range map[string]func(*transferencoding.StoredTransferMember){
		"missing-original":           func(m *transferencoding.StoredTransferMember) { m.Facet.OriginalProposal = nil },
		"wrong-kind":                 func(m *transferencoding.StoredTransferMember) { m.Facet.OriginalProposal.Kind = "contact-revision" },
		"wrong-public-owner":         func(m *transferencoding.StoredTransferMember) { m.Facet.OriginalProposal.Owner = b.authority },
		"foreign-active":             func(m *transferencoding.StoredTransferMember) { m.Facet.State = contract.FacetActive },
		"detached-contact":           func(m *transferencoding.StoredTransferMember) { m.Facet.NextHop = "local:should-not-connect" },
		"wrong-key-element":          func(m *transferencoding.StoredTransferMember) { m.Facet.Key.Element = initial.Boundary },
		"raw-proposal-wrong-element": func(m *transferencoding.StoredTransferMember) { copy(m.Facet.Key.Proposal[32:68], initial.Boundary) },
		"unknown-original-owner": func(m *transferencoding.StoredTransferMember) {
			m.Facet.Key.Core = values.FactAuthorityRef(foundationStoreID(789))
			m.Facet.OriginalProposal.Owner = m.Facet.Key.Core
		},
		"forged-original-mac": func(m *transferencoding.StoredTransferMember) {
			m.Facet.OriginalProposal.Value = "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"
		},
		"erase-member-custody": func(m *transferencoding.StoredTransferMember) { m.Lineage = nil },
	} {
		t.Run(name, func(t *testing.T) {
			bad, altered := changedLineage(t, f, raw, func(m *transferencoding.StoredTransferManifest) {
				for n := range m.Members {
					if m.Members[n].ID == foundationStoreID(102) {
						mutate(&m.Members[n])
						return
					}
				}
				t.Fatal("missing facet member")
			})
			before := a.state.clone()
			if _, err := acceptTransfer(t, a, bad, altered); err == nil {
				t.Fatal("accepted altered precursor with recomputed outer digest")
			}
			if !sameCompleteCoreState(before, a.state) {
				t.Fatal("rejected evidence reserved target")
			}
		})
	}
	completeFencedTransfer(t, b, a, f, raw)
}

func TestTransferFacetRejectsIncoherentRetiredPredecessor(t *testing.T) {
	a, _, initial := transferIntentFixture(t)
	b := transferReceiver(t, foundationStoreID(999))
	root, _ := a.state.elements.ParentOf(initial.Boundary)
	transferFacetFixture(t, a, foundationStoreID(102), contract.FacetRetired)
	f, raw := outgoingTransfer(t, a, b, initial.Boundary, foundationStoreID(999), "retired-out")
	completeFencedTransfer(t, a, b, f, raw)
	// The return leg carries the retired lineage as a foreign-key member: the
	// terminal state, its disposed key and its lineage proof must all survive
	// decoding exactly, or the whole manifest is refused.
	f, raw = outgoingTransfer(t, b, a, initial.Boundary, root, "retired-back")
	for name, mutate := range map[string]func(*transferencoding.StoredTransferMember){
		"retired-active":           func(m *transferencoding.StoredTransferMember) { m.Facet.State = contract.FacetActive },
		"retired-none":             func(m *transferencoding.StoredTransferMember) { m.Facet.State = contract.FacetNone },
		"retired-contact":          func(m *transferencoding.StoredTransferMember) { m.Facet.NextHop = "local:should-not-connect" },
		"retired-missing-original": func(m *transferencoding.StoredTransferMember) { m.Facet.OriginalProposal = nil },
		"retired-unknown-key-core": func(m *transferencoding.StoredTransferMember) {
			m.Facet.Key.Core = foundationStoreOtherAuthority(t, m.Facet.Key.Core)
			m.Facet.OriginalProposal.Owner = m.Facet.Key.Core
		},
		"retired-erase-lineage": func(m *transferencoding.StoredTransferMember) { m.Lineage = nil },
	} {
		t.Run(name, func(t *testing.T) {
			bad, altered := changedLineage(t, f, raw, func(m *transferencoding.StoredTransferManifest) {
				for n := range m.Members {
					if m.Members[n].ID == foundationStoreID(102) {
						mutate(&m.Members[n])
						return
					}
				}
				t.Fatal("missing facet member")
			})
			before := a.state.clone()
			if _, err := acceptTransfer(t, a, bad, altered); err == nil {
				t.Fatal("accepted altered retired predecessor with recomputed outer digest")
			}
			if !sameCompleteCoreState(before, a.state) {
				t.Fatal("rejected evidence reserved target")
			}
		})
	}
	completeFencedTransfer(t, b, a, f, raw)
}

func TestTransferFacetInstallationFailureRestartAndConcurrentRetry(t *testing.T) {
	for _, after := range []bool{false, true} {
		t.Run(map[bool]string{false: "rollback", true: "lost-response"}[after], func(t *testing.T) {
			source, _, initial := transferIntentFixture(t)
			target := transferReceiver(t, foundationStoreID(999))
			original := transferFacetFixture(t, source, foundationStoreID(102), contract.FacetActive)
			f, raw := outgoingTransfer(t, source, target, initial.Boundary, foundationStoreID(999), "faulted-facet")
			a, err := acceptTransfer(t, target, f, raw)
			if err != nil {
				t.Fatal(err)
			}
			d, err := commitSuccessor(source, a, source.state.transfers.records[f.Input.Operation].FenceRevision)
			if err != nil {
				t.Fatal(err)
			}
			before, path := target.state.clone(), target.store.db.Path()
			fail := func() error { return errors.New("facet predecessor installation fault") }
			if after {
				target.store.failAfterCommit = fail
			} else {
				target.store.failBeforeCommit = fail
			}
			if _, err := installTransfer(target, d); !errors.Is(err, contract.ErrStore) {
				t.Fatal(err)
			}
			if sameCompleteCoreState(before, target.state) == after {
				t.Fatal("wrong publish outcome")
			}
			_, present := target.state.attachments.facts.Record(foundationStoreID(102))
			if present != after || (target.state.elements.Kind(foundationStoreID(102)) != 0) != after {
				t.Fatal("space and attachment installed separately")
			}
			durable := target.state.clone()
			closeFoundationStore(t, target)
			target = openFoundationStore(t, path)
			defer target.CloseStore()
			if !sameCompleteCoreState(durable, target.state) {
				t.Fatal("restart changed durable outcome")
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
				if errs[n] != nil || results[n] != results[0] {
					t.Fatal("retry result", errs[n])
				}
			}
			r, _ := target.state.attachments.facts.Record(foundationStoreID(102))
			if r.Attachment.Key != original.Attachment.Key || r.Attachment.State != contract.FacetDetached || r.NextHop != "" || target.state.cutSerial != before.cutSerial+1 {
				t.Fatal("retry reinstated active relationship or advanced twice")
			}
			if err := validateCoreState(target.state); err != nil {
				t.Fatal(err)
			}
		})
	}
}
