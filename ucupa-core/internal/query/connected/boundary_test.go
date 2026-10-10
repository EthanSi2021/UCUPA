package connected

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
)

// A stale authorization approval must never reach the connected planning or
// disclosure boundary: when the durable Realm contact moves between authorize and
// the owner read lock, the captured generation no longer matches and the whole
// connected operation becomes unavailable instead of merging remote sources.
func TestConnectedBoundaryStopsOnContactMoveAfterAuthorization(t *testing.T) {
	f := newReviewFixture(t, false, 128)
	var armed func()
	// Connected planning authorizes the coordinator several times per operation,
	// so the armed move must fire on every authorize call: whichever approval a
	// settled boundary consumes is then already stale.
	f.parent.policy.set(func(_ values.PrincipalRef, _ ...values.Requirement) bool {
		if armed != nil {
			armed()
		}
		return true
	})
	moves := 0
	armed = func() {
		moves++
		current, ok := f.parent.owner.CurrentContact()
		if !ok {
			t.Fatal("missing durable contact")
		}
		request := contract.RealmContactUpdate{OperationID: contract.OperationID(fmt.Sprintf("boundary-move-%d", moves)),
			Owner: f.parent.owner.AuthorityRef(), Realm: "fixture-realm", Expected: current.Revision,
			NextHop: values.NextHop(fmt.Sprintf("local:fixture-moved-%d", moves))}
		f.parent.owner.Lock()
		defer f.parent.owner.Unlock()
		if _, err := f.parent.owner.ApplyRealmContactLocked(reviewActor, request); err != nil {
			t.Fatalf("durable contact move = %v", err)
		}
	}
	_, err := f.parent.connected.QueryRegionPath(context.Background(), f.callers, reviewRootQuery(f.root), nil, 256, nil)
	if !errors.Is(err, contract.ErrUnavailable) {
		t.Fatalf("connected query after contact move = %v", err)
	}
	// Contrast: with the moved contact now current, the same authorized callers
	// reconnect both sources and complete the same query without any special
	// handling.
	armed = nil
	page, err := f.parent.connected.QueryRegionPath(context.Background(), f.callers, reviewRootQuery(f.root), nil, 256, nil)
	if err != nil || page.Status != contract.QueryComplete || len(page.Matches) == 0 {
		t.Fatalf("connected query on current contact = %#v %v", page, err)
	}
}
