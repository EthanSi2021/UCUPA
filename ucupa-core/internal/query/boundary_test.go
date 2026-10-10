package query

import (
	"fmt"
	"testing"

	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
)

// A stale authorization approval must never reach the final read boundary: when
// the durable Realm contact moves between authorize and the owner read lock, the
// captured generation no longer matches and the whole page chain becomes
// unavailable instead of disclosing element summaries.
func TestQueryBoundaryStopsOnContactMoveAfterAuthorization(t *testing.T) {
	f := newQueryFixture(t, contract.Config{}, false)
	var armed func(values.PrincipalRef)
	// QueryRegion re-captures the contact for its final boundary check, so the
	// armed move must fire on every authorize call, not only the first one.
	f.queries.authorizer = queryAuthorize(func(actor values.PrincipalRef, _ ...values.Requirement) bool {
		if armed != nil {
			armed(actor)
		}
		return true
	})
	moves := 0
	movesContact := func(values.PrincipalRef) {
		moves++
		current, ok := f.owner.CurrentContact()
		if !ok {
			t.Fatal("missing durable contact")
		}
		request := contract.RealmContactUpdate{OperationID: contract.OperationID(fmt.Sprintf("boundary-move-%d", moves)),
			Owner: f.owner.AuthorityRef(), Realm: "fixture-realm", Expected: current.Revision,
			NextHop: values.NextHop(fmt.Sprintf("local:fixture-moved-%d", moves))}
		f.owner.Lock()
		defer f.owner.Unlock()
		if _, err := f.owner.ApplyRealmContactLocked(queryActor, request); err != nil {
			t.Fatalf("durable contact move = %v", err)
		}
	}
	armed = movesContact
	page, err := f.queries.QueryRegion(queryActor, f.region, nil, 1, nil)
	requireQueryError(t, page, err, contract.ErrUnavailable)
	requireNoQueryResources(t, f.queries)
	// Contrast: with the moved contact now current, the same authorized actor
	// receives the complete current-verified page without any special handling.
	armed = nil
	full, err := f.queries.QueryRegion(queryActor, f.region, nil, 256, nil)
	if err != nil || full.Status != contract.QueryComplete || len(full.Matches) != len(f.ids) {
		t.Fatalf("query on current contact = %#v %v", full, err)
	}
}
