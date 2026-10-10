package query

import (
	"errors"
	"os"
	"reflect"
	"testing"
	"time"

	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	"ucupa/ucupa-core/internal/token"
)

func TestSnapshotSealedSharedViewStillValidatesFacts(t *testing.T) {
	ttl := time.Hour
	f := newQueryFixture(t, contract.Config{SnapshotRetention: &ttl}, false)
	first, err := f.queries.QueryRegion(queryActor, f.region, nil, 1, nil)
	if err != nil {
		t.Fatal(err)
	}
	second, err := f.queries.QueryRegion(queryActor, f.region, nil, 1, nil)
	if err != nil || second.Snapshot == nil {
		t.Fatal(err)
	}
	r := f.queries.reads
	r.mu.Lock()
	view := r.views[first.Cut]
	if len(r.views) != 1 || len(r.entries) != 2 || view == nil || view.projection == nil || view.candidate != nil {
		r.mu.Unlock()
		t.Fatal("same cut did not share sealed history-free view with separate handles")
	}
	serial := view.projection.RevisionSerial
	view.projection.RevisionSerial = 0
	if err := validateReadView(view); !errors.Is(err, contract.ErrStore) {
		r.mu.Unlock()
		t.Fatalf("retained view skipped live fact validation: %v", err)
	}
	view.projection.RevisionSerial = serial
	r.mu.Unlock()
}

func TestSnapshotExpiryAndBorrowAccounting(t *testing.T) {
	ttl := time.Hour
	f := newQueryFixture(t, contract.Config{SnapshotRetention: &ttl, SnapshotCapacity: 1, ConcurrentReads: 1}, false)
	page, err := f.queries.QueryRegion(queryActor, f.region, nil, 1, nil)
	if err != nil {
		t.Fatal(err)
	}
	limited, err := f.queries.QueryRegion(queryActor, f.region, nil, 1, nil)
	requireQueryError(t, limited, err, contract.ErrSnapshotCapacity)
	borrow, err := f.queries.Borrow(queryActor, *page.Snapshot)
	if err != nil {
		t.Fatal(err)
	}
	limited, err = f.queries.QueryRegion(queryActor, f.region, nil, 1, page.Continuation)
	requireQueryError(t, limited, err, contract.ErrSnapshotCapacity)
	r := f.queries.reads
	r.mu.Lock()
	entry := r.entries[token.SnapshotID(*page.Snapshot)]
	r.now = func() time.Time { return entry.expires }
	r.expireLocked()
	if len(r.entries) != 0 || len(r.views) != 1 || r.bytes == 0 {
		r.mu.Unlock()
		t.Fatal("expiry lost active borrower accounting")
	}
	r.mu.Unlock()
	if err := f.queries.Check(borrow); !errors.Is(err, contract.ErrReadCutExpired) {
		t.Fatalf("expired borrower disclosed: %v", err)
	}
	borrow.Finish()
	requireNoQueryResources(t, f.queries)
}

func TestSnapshotAutomaticExpiryWithoutFurtherCalls(t *testing.T) {
	ttl := 30 * time.Millisecond
	f := newQueryFixture(t, contract.Config{SnapshotRetention: &ttl}, false)
	if _, err := f.queries.QueryRegion(queryActor, f.region, nil, 1, nil); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for {
		r := f.queries.reads
		r.mu.Lock()
		done := len(r.entries) == 0 && len(r.views) == 0 && r.bytes == 0
		r.mu.Unlock()
		if done {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("timer did not reclaim data without another API call")
		}
		time.Sleep(time.Millisecond)
	}
}

func TestSnapshotCurrentModeRetainsNoCrossRequestState(t *testing.T) {
	f := newQueryFixture(t, contract.Config{}, false)
	page, err := f.queries.QueryRegion(queryActor, f.region, nil, 1, nil)
	if err != nil || page.Snapshot != nil || page.ReadMode != contract.QueryCurrentVerified {
		t.Fatalf("off policy changed query mode: %#v %v", page, err)
	}
	requireNoQueryResources(t, f.queries)
	if _, err := f.queries.QueryRegion(queryActor, f.region, nil, 256, page.Continuation); err != nil {
		t.Fatal(err)
	}
	requireNoQueryResources(t, f.queries)
}

func TestSnapshotMemoryCapacityPreservesFactsAndAuthorizationOrder(t *testing.T) {
	f := newQueryFixture(t, contract.Config{SnapshotBytes: 1}, true)
	// Query cannot access the private aggregate. Preserve the old unchanged-
	// facts check through its finite fact view/cut and the actual durable bytes.
	f.owner.RLock()
	before := f.owner.FactsLocked()
	f.owner.RUnlock()
	bytesBefore, err := os.ReadFile(f.path)
	if err != nil {
		t.Fatal(err)
	}
	page, err := f.queries.QueryRegion(queryActor, f.region, nil, 1, nil)
	requireQueryError(t, page, err, contract.ErrSnapshotCapacity)
	f.owner.RLock()
	after := f.owner.FactsLocked()
	f.owner.RUnlock()
	bytesAfter, err := os.ReadFile(f.path)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, after) || !reflect.DeepEqual(bytesBefore, bytesAfter) {
		t.Fatal("capacity error changed current facts, cut, or durable state/history")
	}
	f.queries.authorizer = queryAuthorize(func(values.PrincipalRef, ...values.Requirement) bool { return false })
	page, err = f.queries.QueryRegion(queryActor, f.region, nil, 1, nil)
	requireQueryError(t, page, err, contract.ErrDenied)
	requireNoQueryResources(t, f.queries)
}
