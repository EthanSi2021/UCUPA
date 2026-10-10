package commit

import (
	"errors"
	"sync"
	"testing"

	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	elementfact "ucupa/ucupa-core/internal/element/fact"
	"ucupa/ucupa-core/internal/token"
	transferfact "ucupa/ucupa-core/internal/transfer/fact"
)

func settleTransfer(o *Owner, installed transferfact.TargetInstallation) (transferfact.SourceSettlement, error) {
	o.Lock()
	defer o.Unlock()
	return o.SettleTransferSourceLocked(transferfact.CoreCompletion{Installed: transferfact.CoreInstallation{Source: installed}, At: installed.At, Revision: installed.Revision})
}

func installedTransfer(t *testing.T) (*Owner, *Owner, transferfact.TargetInstallation) {
	t.Helper()
	source, target, d := decidedTransfer(t)
	r, err := installTransfer(target, d)
	if err != nil {
		t.Fatal(err)
	}
	return source, target, r
}

func TestTransferSettlementExactCutCoverageAndRecovery(t *testing.T) {
	source, target, installed := installedTransfer(t)
	i := installed.Decision.Acceptance.Fence.Input
	before, targetBefore := source.state.clone(), target.state.clone()
	original := before.transfers.records[i.Operation]
	f, raw, err := source.TransferSourceFenceLocked(i.Actor, i.Operation)
	if err != nil {
		t.Fatal(err)
	}
	_, beforeGaps, _ := sourceVisibility(before).Counts()
	r, err := settleTransfer(source, installed)
	if err != nil || r.Installation != installed || r.Revision != 4 || token.Serial(r.At) != before.cutSerial+1 {
		t.Fatal("settle", r, err)
	}
	if _, gaps, _ := sourceVisibility(source.state).Counts(); beforeGaps != 1 || gaps != 0 {
		t.Fatal("source coverage not settled", beforeGaps, gaps)
	}
	if source.state.revisionSerial != before.revisionSerial || len(source.state.operations) != len(before.operations) || len(source.state.commits) != len(before.commits)+1 || !sameCompleteCoreState(targetBefore, target.state) {
		t.Fatal("settlement changed elements, outcomes, target, or commit count")
	}
	if !sourceVisibility(source.state).Inactive(i.Boundary) || !source.state.transfers.records[i.Operation].SourceClosed() || source.state.transfers.records[i.Operation].SourcePending() {
		t.Fatal("departure or pending semantics")
	}
	if err := source.state.guardSourceTransferScope(original.Origin.Parent); err != nil {
		t.Fatal("ancestor still held", err)
	}
	if err := source.state.guardTransferElement(i.Boundary); !errors.Is(err, values.PublicUnavailable) {
		t.Fatal("old address active", err)
	}
	if _, err := cancelTransfer(t, source, i, r.Revision); err == nil {
		t.Fatal("settled transfer cancelled")
	}
	// All three original milestones remain stable, even after a later write.
	parent, _ := source.state.elements.Anchor(original.Origin.Parent)
	if _, err := applyFoundationStore(source, elementfact.OperationCreateEntity, i.Actor, foundationStoreID(3001), foundationStoreTuple(t, parent.ID, 0, 0), contract.Revision{}, "after-settlement"); err != nil {
		t.Fatal(err)
	}
	decision, err := commitSuccessor(source, installed.Decision.Acceptance, original.FenceRevision)
	if err != nil || decision != installed.Decision {
		t.Fatal("decision replaced by settlement", err)
	}
	gotFence, gotRaw, err := source.TransferSourceFenceLocked(i.Actor, i.Operation)
	if err != nil || gotFence != f || gotRaw != raw {
		t.Fatal("original fence lost", err)
	}
	again, err := settleTransfer(source, installed)
	if err != nil || again != r {
		t.Fatal("original settlement lost", err)
	}
	if err := validateCoreState(source.state); err != nil {
		t.Fatal("replay", err)
	}
	path, durable := source.store.db.Path(), source.state.clone()
	closeFoundationStore(t, source)
	source = openFoundationStore(t, path)
	defer source.CloseStore()
	if !sameCompleteCoreState(durable, source.state) {
		t.Fatal("reopen differs")
	}
	again, err = settleTransfer(source, installed)
	if err != nil || again != r {
		t.Fatal("reopened result", err)
	}
	// A completed departure remains distinct from a permanent delete. Neither
	// ordinary creation nor an old source CAS can reclaim its UUID.
	if _, err := applyFoundationStore(source, elementfact.OperationCreateAnchor, i.Actor, i.Boundary, i.Final, contract.Revision{}, "ordinary-reuse"); err == nil {
		t.Fatal("ordinary UUID reuse")
	}
}

func TestTransferSettlementMismatchDoesNotClearSource(t *testing.T) {
	source, _, installed := installedTransfer(t)
	changes := map[string]func(*transferfact.TargetInstallation){
		"no-install-cut":  func(r *transferfact.TargetInstallation) { r.At = contract.CoreReadCut{} },
		"accept-cut":      func(r *transferfact.TargetInstallation) { r.At = r.Decision.Acceptance.At },
		"source-cut":      func(r *transferfact.TargetInstallation) { r.At = r.Decision.At },
		"no-revision":     func(r *transferfact.TargetInstallation) { r.Revision = 0 },
		"accept-revision": func(r *transferfact.TargetInstallation) { r.Revision = r.Decision.Acceptance.Revision },
		"decision":        func(r *transferfact.TargetInstallation) { r.Decision.Revision++ },
		"acceptance":      func(r *transferfact.TargetInstallation) { r.Decision.Acceptance.Revision++ },
		"digest":          func(r *transferfact.TargetInstallation) { r.Decision.Acceptance.Fence.ManifestDigest[0] ^= 1 },
		"address": func(r *transferfact.TargetInstallation) {
			r.Decision.Acceptance.Fence.Input.TargetAddress = "local:wrong"
		},
	}
	for name, change := range changes {
		t.Run(name, func(t *testing.T) {
			bad, before := installed, source.state.clone()
			change(&bad)
			if _, err := settleTransfer(source, bad); err == nil {
				t.Fatal("mismatch accepted")
			}
			if !sameCompleteCoreState(before, source.state) {
				t.Fatal("refusal changed state")
			}
		})
	}
	if _, err := settleTransfer(source, installed); err != nil {
		t.Fatal(err)
	}
	bad := installed
	bad.Revision++
	if _, err := settleTransfer(source, bad); !errors.Is(err, contract.ErrOperationConflict) {
		t.Fatal("different replay result accepted", err)
	}
}

func TestTransferSettlementFaultAndConcurrentRetries(t *testing.T) {
	for _, after := range []bool{false, true} {
		t.Run(map[bool]string{false: "rollback", true: "response-lost"}[after], func(t *testing.T) {
			source, _, installed := installedTransfer(t)
			before, path := source.state.clone(), source.store.db.Path()
			fault := func() error { return errors.New("settlement fault") }
			if after {
				source.store.failAfterCommit = fault
			} else {
				source.store.failBeforeCommit = fault
			}
			if _, err := settleTransfer(source, installed); !errors.Is(err, contract.ErrStore) {
				t.Fatal(err)
			}
			if sameCompleteCoreState(before, source.state) == after {
				t.Fatal("wrong durable reconciliation")
			}
			closeFoundationStore(t, source)
			source = openFoundationStore(t, path)
			defer source.CloseStore()
			var results [8]transferfact.SourceSettlement
			var errs [8]error
			var wg sync.WaitGroup
			for index := range results {
				wg.Add(1)
				go func(index int) { defer wg.Done(); results[index], errs[index] = settleTransfer(source, installed) }(index)
			}
			wg.Wait()
			for index := range results {
				if errs[index] != nil || results[index] != results[0] {
					t.Fatal("retry", errs[index])
				}
			}
			if source.state.cutSerial != before.cutSerial+1 || len(source.state.transfers.events) != 4 {
				t.Fatal("duplicate settlement")
			}
		})
	}
}
