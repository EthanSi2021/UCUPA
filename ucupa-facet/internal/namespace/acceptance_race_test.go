package namespace

import (
	"context"
	"sync/atomic"
	"testing"

	"ucupa/ucupa-facet/contract"
)

func TestConcurrentAcceptancePreservesWinnerAndRepairedContact(t *testing.T) {
	x := newFixture(t, false)
	ctx := context.Background()
	k := testKey(x.key.Facet, x.key.Core, x.key.Element, 5)
	x.source.current = testProof(k, contract.CoreNone, 5)
	x.source.current.Pending = k
	entered, release := make(chan struct{}), make(chan struct{})
	var calls atomic.Int64
	x.source.hook = func() {
		if calls.Add(1) == 1 {
			close(entered)
			<-release
		}
	}
	results := make(chan contract.MutationResult, 1)
	failed := make(chan error, 1)
	go func() {
		r, err := x.f.AcceptAttachment(ctx, testActor, k, "local:core", "late")
		results <- r
		failed <- err
	}()
	<-entered
	winner, err := x.f.AcceptAttachment(ctx, testActor, k, "local:core", "winner")
	if err != nil {
		t.Fatal(err)
	}
	changed, err := x.f.UpdateCoreNextHop(testActor, "repair", k, winner.NextHopRevision, "local:repaired")
	if err != nil {
		t.Fatal(err)
	}
	close(release)
	late, err := <-results, <-failed
	if err != nil || late.Status != contract.Unchanged || late.Acceptance != winner.Acceptance || late.NextHopRevision != changed.NextHopRevision || late.CoreNextHop != "local:repaired" {
		t.Fatalf("late acceptance overwrote winner: %+v %v", late, err)
	}
}
func TestConcurrentPutPinsOneRevision(t *testing.T) {
	x := newFixture(t, false)
	entered, release := make(chan struct{}), make(chan struct{})
	var calls atomic.Int64
	x.source.hook = func() {
		if calls.Add(1) == 1 {
			close(entered)
			<-release
		}
	}
	done := make(chan error, 1)
	go func() {
		_, err := x.f.Put(context.Background(), testActor, x.key, x.typ, fixturePayload("first"), contract.ExpectAbsent(), "late")
		done <- err
	}()
	<-entered
	x.put(t, "winner", "second", contract.ExpectAbsent())
	close(release)
	mustError(t, <-done, contract.ErrRevisionConflict)
}
