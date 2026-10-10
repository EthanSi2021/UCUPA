package collect_test

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"testing"

	"ucupa/shared/values"
	corecontract "ucupa/ucupa-core/contract"
	federationcontract "ucupa/ucupa-core/contract/federation"
)

type actualObservationHook struct {
	federationcontract.FederationReadConnection
	mu        sync.Mutex
	afterPoll func()
}

func (part *actualObservationHook) Observe(ctx context.Context, request federationcontract.FederationObserveRequest) (federationcontract.FederationObservationPage, error) {
	page, err := part.FederationReadConnection.Observe(ctx, request)
	if request.Operation != "poll" {
		return page, err
	}
	part.mu.Lock()
	after := part.afterPoll
	part.afterPoll = nil
	part.mu.Unlock()
	if after != nil {
		after()
	}
	return page, err
}

func (part *actualObservationHook) setAfterPoll(after func()) {
	part.mu.Lock()
	part.afterPoll = after
	part.mu.Unlock()
}

func openActualObservation(t *testing.T, fixture *actualFixture) (*actualReadAttempt, *actualObservationHook) {
	t.Helper()
	var hook *actualObservationHook
	request := federationcontract.FederationReadRequest{Boundary: fixture.root, Intent: "query", PrepareObserve: true, Purpose: [32]byte{7}}
	attempt, err := fixture.openQuery(t, request, 3, actualReadOptions{wrapLocal: func(part federationcontract.FederationReadConnection) federationcontract.FederationReadConnection {
		hook = &actualObservationHook{FederationReadConnection: part}
		return hook
	}})
	if err != nil {
		t.Fatal(err)
	}
	page, err := collectActualPage(attempt.read, true)
	if err != nil || page.Closure == ([32]byte{}) {
		t.Fatal(err)
	}
	return attempt, hook
}

func writeObservationVector(t *testing.T, fixture *actualFixture, index int, serial uint64) {
	t.Helper()
	parents := []values.ElementID{fixture.root, actualID(9710), actualID(9720)}
	fixture.write(t, index, serial, parents[index], corecontract.OperationID(actualID(serial)))
}

// Migrated from the root TestFederationObservationVectorOrderingAcknowledgementAndRelease.
func TestFederationObservationVectorOrderingAcknowledgementAndRelease(t *testing.T) {
	fixture := newActualFixture(t, false, nil)
	attempt, _ := openActualObservation(t, fixture)
	for index := 0; index < 3; index++ {
		writeObservationVector(t, fixture, index, uint64(9790+index))
	}
	var position uint64
	for index := 0; index < 3; index++ {
		batch, err := observeActualPage(attempt.read, position, 1)
		if err != nil || len(batch.Groups) != 1 || batch.Groups[0].ID.Owner != fixture.owners[index].AuthorityRef() || batch.Position != position+1 {
			t.Fatal("lost per-source delivery", batch, err)
		}
		repeat, err := observeActualPage(attempt.read, position, 1)
		if err != nil || !reflect.DeepEqual(batch, repeat) {
			t.Fatal("retry changed batch", repeat, err)
		}
		batch.Groups[0].Categories[0] = "changed-by-consumer"
		repeat, err = observeActualPage(attempt.read, position, 1)
		if err != nil || repeat.Groups[0].Categories[0] != "spatial" {
			t.Fatal("retained batch aliased", err)
		}
		position = batch.Position
	}
	empty, err := observeActualPage(attempt.read, position, 1)
	if err != nil || len(empty.Groups) != 0 || empty.Position != position {
		t.Fatal("unexpected phantom delivery", empty, err)
	}
	if _, err := observeActualPage(attempt.read, 0, 1); !errors.Is(err, values.PublicGap) {
		t.Fatal("acknowledged cursor reused", err)
	}
	if err := attempt.read.Retire(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := observeActualPage(attempt.read, position, 1); !errors.Is(err, values.PublicGap) {
		t.Fatal("released sources observed", err)
	}
	attempt.tracker.requireReleased(t, actualOwnerRefs(fixture, 3)...)
}

// Migrated from the root
// TestFederationObservationVectorAuthorizationRecheckedForCachedAndNewBatch.
func TestFederationObservationVectorAuthorizationRecheckedForCachedAndNewBatch(t *testing.T) {
	for _, cached := range []bool{false, true} {
		t.Run(map[bool]string{false: "during-collection", true: "cached-retry"}[cached], func(t *testing.T) {
			policy := &actualPolicy{epoch: 1}
			fixture := newActualFixture(t, false, policy)
			attempt, hook := openActualObservation(t, fixture)
			writeObservationVector(t, fixture, 0, 9797)
			baseline := policy.authorizeViewCalls()
			if cached {
				if batch, err := observeActualPage(attempt.read, 0, 1); err != nil || len(batch.Groups) != 1 {
					t.Fatal(batch, err)
				}
				policy.advance()
			} else {
				hook.setAfterPoll(policy.advance)
			}
			batch, err := observeActualPage(attempt.read, 0, 1)
			if !errors.Is(err, values.PublicGap) || len(batch.Groups) != 0 || policy.authorizeViewCalls() != baseline {
				t.Fatal("revoked observation disclosed or renewed", batch, err)
			}
			batch, err = observeActualPage(attempt.read, 0, 1)
			if !errors.Is(err, values.PublicGap) || len(batch.Groups) != 0 || policy.authorizeViewCalls() != baseline {
				t.Fatal("revoked observation gap was renewed", batch, err)
			}
			if err := attempt.read.Retire(context.Background()); err != nil {
				t.Fatal(err)
			}
			attempt.tracker.requireReleased(t, actualOwnerRefs(fixture, 3)...)
		})
	}
}
