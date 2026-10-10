package commit

import (
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	elementfact "ucupa/ucupa-core/internal/element/fact"
	transferencoding "ucupa/ucupa-core/internal/transfer/encoding"
	transferfact "ucupa/ucupa-core/internal/transfer/fact"
)

func transferIntentFixture(t *testing.T) (*Owner, string, transferfact.Intent) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "source.db")
	ids := seedFoundationStore(t, path)
	o := openFoundationStore(t, path)
	t.Cleanup(func() { _ = o.CloseStore() })
	target, err := values.NewFactAuthorityRef()
	if err != nil {
		t.Fatal(err)
	}
	a, _ := o.state.elements.Anchor(ids.anchorID)
	i := transferfact.Intent{Profile: transferfact.SliceTransfer, Operation: "transfer", Actor: foundationStoreActor, Source: o.authority,
		Target: target, Boundary: a.ID, Expected: a.Revision, Final: foundationStoreTuple(t, foundationStoreID(999), 7, 20),
		SourceAddress: "local:source", TargetAddress: "local:target"}
	return o, path, i
}

func beginTransfer(t *testing.T, o *Owner, i transferfact.Intent) (transferfact.IntentRecord, error) {
	t.Helper()
	o.Lock()
	defer o.Unlock()
	return o.BeginTransferIntentLocked(i, 100)
}

func cancelTransfer(t *testing.T, o *Owner, i transferfact.Intent, expected uint64) (transferfact.IntentRecord, error) {
	t.Helper()
	o.Lock()
	defer o.Unlock()
	return o.CancelTransferIntentLocked(i.Actor, i.Operation, expected)
}

func TestTransferIntentDurableExactInputAndNoPrematureFence(t *testing.T) {
	o, path, i := transferIntentFixture(t)
	i.Operation = contract.OperationID(string([]byte{0xff, 0, 1}))
	i.Actor = values.PrincipalRef{Realm: "r\xff", Principal: "p\xfe\x00"}
	before := o.state.clone()
	r, err := beginTransfer(t, o, i)
	if err != nil || r.Progress != transferfact.IntentPending || r.Input != i || r.Admission != before.currentCut() || r.Revision != 1 {
		t.Fatalf("begin: %+v %v", r, err)
	}
	if !sameCoreFacts(before, o.state) || len(o.state.operations) != len(before.operations) || len(o.state.commits) != len(before.commits) {
		t.Fatal("intent modified spatial truth or old outcomes")
	}
	if sameCompleteCoreState(before, o.state) {
		t.Fatal("complete state omitted transfer history")
	}
	for _, intent := range []transferfact.Intent{i, i} {
		again, err := beginTransfer(t, o, intent)
		if err != nil || again != r || len(o.state.transfers.events) != 1 {
			t.Fatalf("repeat: %+v %v", again, err)
		}
	}
	// Children can change without changing the boundary Revision. INTENT does
	// not freeze their contents or turn its observation into a final cut.
	var child elementfact.Entity
	for _, e := range o.state.elements.Entities() {
		child = e
		break
	}
	result, err := applyFoundationStore(o, elementfact.OperationUpdateEntity, foundationStoreActor, child.ID,
		foundationStoreTuple(t, i.Boundary, 2, 1), child.Revision, "after-intent")
	if err != nil || result.Status != contract.MutationCommitted {
		t.Fatalf("intent fenced write: %+v %v", result, err)
	}
	updated := o.state.clone()
	closeFoundationStore(t, o)
	reopened := openFoundationStore(t, path)
	defer reopened.CloseStore()
	if !sameCompleteCoreState(updated, reopened.state) {
		t.Fatal("restart changed complete truth/input/lineage")
	}
	again, err := beginTransfer(t, reopened, i)
	if err != nil || again != r {
		t.Fatalf("restart exact input: %+v %v", again, err)
	}
	old, err := applyFoundationStore(reopened, elementfact.OperationUpdateEntity, foundationStoreActor, child.ID,
		foundationStoreTuple(t, i.Boundary, 2, 1), child.Revision, "after-intent")
	if err != nil || old.Status != result.Status || old.OutcomeCut != result.OutcomeCut {
		t.Fatalf("old outcome: %+v %v", old, err)
	}
	if _, err := applyFoundationStore(reopened, elementfact.OperationUpdateEntity, i.Actor, child.ID,
		foundationStoreTuple(t, i.Boundary, 3, 1), result.Record.Entity.Revision, i.Operation); !errors.Is(err, contract.ErrOperationConflict) {
		t.Fatalf("ordinary write reused control ID: %v", err)
	}
}

func TestTransferIntentInputCannotBeRebound(t *testing.T) {
	o, _, i := transferIntentFixture(t)
	if _, err := beginTransfer(t, o, i); err != nil {
		t.Fatal(err)
	}
	before := o.state.clone()
	for _, change := range []func(*transferfact.Intent){
		func(v *transferfact.Intent) { v.Actor.Principal = "other" },
		func(v *transferfact.Intent) { v.Target, _ = values.NewFactAuthorityRef() },
		func(v *transferfact.Intent) { v.Final.Parent = foundationStoreID(998) },
		func(v *transferfact.Intent) { v.Final = foundationStoreTuple(t, v.Final.Parent, 8, 20) },
		func(v *transferfact.Intent) { v.SourceAddress = "local:new-source" },
		func(v *transferfact.Intent) { v.TargetAddress = "local:new-target" },
	} {
		changed := i
		change(&changed)
		if _, err := beginTransfer(t, o, changed); !errors.Is(err, contract.ErrOperationConflict) {
			t.Fatalf("input rebound: %v", err)
		}
		if !sameCompleteCoreState(before, o.state) {
			t.Fatal("conflict changed durable state")
		}
	}
}

func TestTransferCancellationExactRetryAndPermanentID(t *testing.T) {
	o, path, i := transferIntentFixture(t)
	r, err := beginTransfer(t, o, i)
	if err != nil {
		t.Fatal(err)
	}
	wrong := i
	wrong.Actor.Principal = "different"
	if _, err := cancelTransfer(t, o, wrong, r.Revision); !errors.Is(err, values.PublicDenied) {
		t.Fatal(err)
	}
	if _, err := cancelTransfer(t, o, i, r.Revision+1); !errors.Is(err, contract.ErrRevisionConflict) {
		t.Fatal(err)
	}
	done, err := cancelTransfer(t, o, i, r.Revision)
	if err != nil || done.Progress != transferfact.IntentCancelled || done.Revision != 2 || done.Admission != r.Admission {
		t.Fatalf("cancel: %+v %v", done, err)
	}
	again, err := beginTransfer(t, o, i)
	if err != nil || again != done {
		t.Fatalf("begin resurrected cancelled intent: %+v %v", again, err)
	}
	closeFoundationStore(t, o)
	reopened := openFoundationStore(t, path)
	defer reopened.CloseStore()
	again, err = cancelTransfer(t, reopened, i, r.Revision)
	if err != nil || again != done || len(reopened.state.transfers.events) != 2 {
		t.Fatalf("cancel retry: %+v %v", again, err)
	}
	if _, err := cancelTransfer(t, reopened, i, done.Revision); !errors.Is(err, contract.ErrRevisionConflict) {
		t.Fatalf("cancel changed original CAS: %v", err)
	}
}

func TestTransferIntentConcurrentSameIDCommitsOnce(t *testing.T) {
	o, _, i := transferIntentFixture(t)
	start := make(chan struct{})
	var wg sync.WaitGroup
	results := make(chan transferfact.IntentRecord, 16)
	for range 16 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			r, err := beginTransfer(t, o, i)
			if err != nil {
				t.Error(err)
				return
			}
			results <- r
		}()
	}
	close(start)
	wg.Wait()
	close(results)
	for r := range results {
		if r.Revision != 1 || r.Progress != transferfact.IntentPending {
			t.Fatalf("duplicate commit: %+v", r)
		}
	}
	if len(o.state.transfers.events) != 1 {
		t.Fatal("concurrent retry created multiple durable intents")
	}
}

func TestTransferIntentCapacityAndInputBounds(t *testing.T) {
	o, _, i := transferIntentFixture(t)
	huge := i
	huge.Actor.Principal = values.PrincipalID(strings.Repeat("x", transferencoding.MaxTransferIntentBytes+1))
	if _, err := beginTransfer(t, o, huge); !errors.Is(err, values.PublicResourceExhausted) {
		t.Fatalf("unbounded caller: %v", err)
	}
	// Exercise the exact production capacity using the same deterministic
	// state rule, then persist/validate the complete candidate once.
	next := o.state.clone()
	for n := range transferencoding.MaxTransferIntents {
		input := i
		input.Operation = contract.OperationID(foundationStoreID(uint64(n + 10000)))
		if err := next.applyTransferEvent(transferencoding.TransferEvent{Kind: transferencoding.TransferPending, At: next.currentCut(), Input: input}); err != nil {
			t.Fatal(err)
		}
	}
	o.Lock()
	err := o.publishStateLocked(next)
	o.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := beginTransfer(t, o, i); !errors.Is(err, values.PublicResourceExhausted) {
		t.Fatalf("capacity exceeded: %v", err)
	}
	first := next.transfers.records[contract.OperationID(foundationStoreID(10000))]
	if _, err := cancelTransfer(t, o, first.Input, first.Revision); err != nil {
		t.Fatalf("no space reserved for cancellation: %v", err)
	}
	if _, err := beginTransfer(t, o, i); !errors.Is(err, values.PublicResourceExhausted) {
		t.Fatalf("cancelled ID was evicted: %v", err)
	}
}
