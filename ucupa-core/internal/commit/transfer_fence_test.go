package commit

import (
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	elementfact "ucupa/ucupa-core/internal/element/fact"
	transferencoding "ucupa/ucupa-core/internal/transfer/encoding"
	transferfact "ucupa/ucupa-core/internal/transfer/fact"
)

func fenceTransfer(o *Owner, i transferfact.Intent, expected uint64) (transferfact.IntentRecord, error) {
	o.Lock()
	defer o.Unlock()
	return o.FenceTransferSourceLocked(i.Actor, i.Operation, expected, transferfact.ParentHold{})
}

func TestTransferFenceRecapturesLastCutAndRetainsOriginalResults(t *testing.T) {
	o, path, i := transferIntentFixture(t)
	r, err := beginTransfer(t, o, i)
	if err != nil {
		t.Fatal(err)
	}
	child, _ := o.state.elements.Entity(foundationStoreID(102))
	final := foundationStoreTuple(t, i.Boundary, 2, 1)
	updated, err := applyFoundationStore(o, elementfact.OperationUpdateEntity, i.Actor, child.ID, final, child.Revision, "last-write")
	if err != nil {
		t.Fatal(err)
	}
	last := o.state.clone()
	_, manifest, err := last.captureTransferManifest(i, transferfact.ParentHold{})
	if err != nil {
		t.Fatal(err)
	}
	fenced, err := fenceTransfer(o, i, r.Revision)
	if err != nil || fenced.Progress != transferfact.IntentFenced || fenced.FinalCut != last.currentCut() {
		t.Fatalf("fence: %+v %v", fenced, err)
	}
	if o.state.cutSerial != last.cutSerial+1 || o.state.revisionSerial != last.revisionSerial || o.state.transfers.events[1].Manifest != manifest {
		t.Fatal("fence did not atomically bind last state")
	}
	if o.state.elements.Kind(i.Boundary) != 0 || o.state.elements.Kind(child.ID) != 0 || !o.state.elements.Claimed(child.ID) {
		t.Fatal("active source closure not withdrawn")
	}
	if _, ok := o.state.elements.Tombstone(child.ID); ok {
		t.Fatal("transfer turned into permanent deletion")
	}
	if _, ok := o.state.attachments.facts.Record(child.ID); ok {
		t.Fatal("current Facet contact survived departure")
	}
	var decoded transferencoding.StoredTransferManifest
	if err := json.Unmarshal([]byte(manifest), &decoded); err != nil {
		t.Fatal(err)
	}
	if len(decoded.Members) != 2 || len(decoded.Ancestors) != 1 {
		t.Fatalf("manifest closure: %+v", decoded)
	}
	for _, m := range decoded.Members {
		if m.ID == child.ID && m.Element.RevisionSerial != last.revisionSerial {
			t.Fatal("manifest captured stale child")
		}
	}
	if _, err := applyFoundationStore(o, elementfact.OperationUpdateEntity, i.Actor, child.ID, final, updated.Record.Entity.Revision, "blocked-write"); !errors.Is(err, values.PublicUnavailable) {
		t.Fatalf("write after fence: %v", err)
	}
	if _, err := applyFoundationStore(o, elementfact.OperationCreateEntity, i.Actor, foundationStoreID(110), final, contract.Revision{}, "blocked-create"); !errors.Is(err, values.PublicUnavailable) {
		t.Fatalf("create under fenced parent: %v", err)
	}
	if _, ok := o.state.operations["blocked-write"]; ok {
		t.Fatal("denial claimed new operation")
	}
	if _, err := cancelTransfer(t, o, i, fenced.Revision); err == nil {
		t.Fatal("cancelled after fence")
	}
	// Unrelated content remains writable. Original outcomes remain retrievable,
	// not silently replaced with the source's current absence.
	if _, err := applyFoundationStore(o, elementfact.OperationCreateEntity, i.Actor, foundationStoreID(111), foundationStoreTuple(t, foundationStoreID(100), 30, 1), contract.Revision{}, "unrelated"); err != nil {
		t.Fatal(err)
	}
	old, err := applyFoundationStore(o, elementfact.OperationUpdateEntity, i.Actor, child.ID, final, child.Revision, "last-write")
	if err != nil || old.OutcomeCut != updated.OutcomeCut || old.Record == nil || old.Record.Entity == nil || *old.Record.Entity != *updated.Record.Entity {
		t.Fatal("original outcome changed")
	}
	durable := o.state.clone()
	closeFoundationStore(t, o)
	reopened := openFoundationStore(t, path)
	defer reopened.CloseStore()
	if !sameCompleteCoreState(durable, reopened.state) {
		t.Fatal("restart changed fence or complete original history")
	}
	again, err := fenceTransfer(reopened, i, r.Revision)
	if err != nil || again != fenced || len(reopened.state.transfers.events) != 2 {
		t.Fatalf("fence retry: %+v %v", again, err)
	}
}

func TestTransferWriteRacingFenceIsIncludedOrRefused(t *testing.T) {
	for n := 0; n < 10; n++ {
		o, path, i := transferIntentFixture(t)
		r, err := beginTransfer(t, o, i)
		if err != nil {
			t.Fatal(err)
		}
		child, _ := o.state.elements.Entity(foundationStoreID(102))
		start := make(chan struct{})
		type writeResult struct {
			result contract.MutationResult
			err    error
		}
		written := make(chan writeResult, 1)
		frozen := make(chan error, 1)
		go func() {
			<-start
			result, err := applyFoundationStore(o, elementfact.OperationUpdateEntity, i.Actor, child.ID, foundationStoreTuple(t, i.Boundary, 3, 1), child.Revision, "racing-write")
			written <- writeResult{result, err}
		}()
		go func() { <-start; _, err := fenceTransfer(o, i, r.Revision); frozen <- err }()
		close(start)
		w, err := <-written, <-frozen
		if err != nil {
			t.Fatal(err)
		}
		var m transferencoding.StoredTransferManifest
		if err := json.Unmarshal([]byte(o.state.transfers.events[1].Manifest), &m); err != nil {
			t.Fatal(err)
		}
		var captured uint64
		for _, member := range m.Members {
			if member.ID == child.ID {
				captured = member.Element.RevisionSerial
			}
		}
		if w.err == nil {
			if w.result.Status != contract.MutationCommitted || captured != o.state.revisionSerial {
				t.Fatal("acknowledged racing write missing from final manifest")
			}
		} else if !errors.Is(w.err, values.PublicUnavailable) || captured != 3 {
			t.Fatalf("race was neither included nor refused: %v", w.err)
		}
		closeFoundationStore(t, o)
		reopened := openFoundationStore(t, path)
		if !sameCompleteCoreState(o.state, reopened.state) {
			t.Fatal("restart race result differs")
		}
		closeFoundationStore(t, reopened)
	}
}

func TestTransferFenceManifestValidationIsAtomic(t *testing.T) {
	o, _, i := transferIntentFixture(t)
	r, err := beginTransfer(t, o, i)
	if err != nil {
		t.Fatal(err)
	}
	_, manifest, err := o.state.captureTransferManifest(i, transferfact.ParentHold{})
	if err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"missing-member", "missing-parent", "extra-member", "changed-value", "different-input", "different-cut", "different-scope"} {
		var m transferencoding.StoredTransferManifest
		if err := json.Unmarshal([]byte(manifest), &m); err != nil {
			t.Fatal(err)
		}
		switch kind {
		case "missing-member":
			m.Members = m.Members[:1]
		case "missing-parent":
			m.Ancestors = nil
		case "extra-member":
			m.Members = append(m.Members, m.Members[0])
		case "changed-value":
			m.Members[0].Element.RevisionSerial++
		case "different-input":
			m.IntentDigest[0] ^= 1
		case "different-cut":
			m.FinalCut++
		case "different-scope":
			m.Scope[0] ^= 1
		}
		data, err := json.Marshal(m)
		if err != nil {
			t.Fatal(err)
		}
		candidate := o.state.clone()
		e := transferencoding.TransferEvent{Kind: transferencoding.TransferFenced, At: candidate.currentCut(), Operation: i.Operation, Actor: i.Actor, Expected: r.Revision, Manifest: string(data)}
		if err := candidate.applyTransferEvent(e); err == nil {
			t.Fatalf("accepted %s", kind)
		}
		if !sameCompleteCoreState(candidate, o.state) {
			t.Fatalf("partial fence for %s", kind)
		}
	}
}

func TestTransferFenceRefusesWrongActorStaleCASAndChangedBoundary(t *testing.T) {
	o, _, i := transferIntentFixture(t)
	r, err := beginTransfer(t, o, i)
	if err != nil {
		t.Fatal(err)
	}
	before := o.state.clone()
	wrong := i
	wrong.Actor.Principal = "other"
	if _, err := fenceTransfer(o, wrong, r.Revision); !errors.Is(err, values.PublicDenied) {
		t.Fatal(err)
	}
	if _, err := fenceTransfer(o, i, r.Revision+1); !errors.Is(err, contract.ErrRevisionConflict) {
		t.Fatal(err)
	}
	if !sameCompleteCoreState(before, o.state) {
		t.Fatal("rejected fence changed state")
	}
	if _, err := applyFoundationStore(o, elementfact.OperationUpdateAnchor, i.Actor, i.Boundary, foundationStoreTuple(t, foundationStoreID(100), 1, 20), i.Expected, "moved-boundary"); err != nil {
		t.Fatal(err)
	}
	before = o.state.clone()
	if _, err := fenceTransfer(o, i, r.Revision); !errors.Is(err, contract.ErrRevisionConflict) {
		t.Fatal(err)
	}
	if !sameCompleteCoreState(before, o.state) {
		t.Fatal("stale preflight changed state")
	}
}

func TestConcurrentTransferFencesHaveOneDurableWinner(t *testing.T) {
	o, _, i := transferIntentFixture(t)
	r, err := beginTransfer(t, o, i)
	if err != nil {
		t.Fatal(err)
	}
	const count = 12
	results := make(chan transferfact.IntentRecord, count)
	errors := make(chan error, count)
	var wg sync.WaitGroup
	for n := 0; n < count; n++ {
		wg.Add(1)
		go func() { defer wg.Done(); r, e := fenceTransfer(o, i, r.Revision); results <- r; errors <- e }()
	}
	wg.Wait()
	close(results)
	close(errors)
	for e := range errors {
		if e != nil {
			t.Fatal(e)
		}
	}
	var want transferfact.IntentRecord
	for result := range results {
		if want == (transferfact.IntentRecord{}) {
			want = result
		}
		if result != want {
			t.Fatal("concurrent retry changed outcome")
		}
	}
	if len(o.state.transfers.events) != 2 || want.Progress != transferfact.IntentFenced {
		t.Fatal("multiple durable fences")
	}
}
