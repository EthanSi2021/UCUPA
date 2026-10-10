package namespace

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"

	"ucupa/shared/values"
	"ucupa/ucupa-facet/contract"
)

func TestConcurrentCASAndExactRetry(t *testing.T) {
	profiles(t, func(t *testing.T, x *fixture) {
		var wg sync.WaitGroup
		var winners atomic.Int64
		first := x.put(t, "initial", "zero", contract.ExpectAbsent())
		for i := 0; i < 16; i++ {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				_, err := x.f.Put(context.Background(), testActor, x.key, x.typ, fixturePayload(fmt.Sprint(i)), contract.ExpectRevision(first.Revision), contract.OperationID(fmt.Sprint("cas", i)))
				if err == nil {
					winners.Add(1)
				} else if !errors.Is(err, contract.ErrRevisionConflict) {
					t.Errorf("CAS: %v", err)
				}
			}(i)
		}
		wg.Wait()
		if winners.Load() != 1 {
			t.Fatal("multiple CAS winners")
		}
		for i := 0; i < 16; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				result, err := x.f.Put(context.Background(), testActor, x.key, x.typ, fixturePayload("zero"), contract.ExpectAbsent(), "initial")
				if err != nil || result != first {
					t.Errorf("retry: %+v %v", result, err)
				}
			}()
		}
		wg.Wait()
	})
}
func TestReadF0RejectsConcurrentFactOrContactChange(t *testing.T) {
	profiles(t, func(t *testing.T, x *fixture) {
		ctx := context.Background()
		x.put(t, "initial", "zero", contract.ExpectAbsent())
		for _, list := range []bool{false, true} {
			entered, release := make(chan struct{}), make(chan struct{})
			x.source.mu.Lock()
			x.source.hook = func() { close(entered); <-release }
			x.source.mu.Unlock()
			done := make(chan error, 1)
			go func() {
				var err error
				if list {
					_, err = x.f.ListTypes(ctx, testActor, x.key)
				} else {
					_, err = x.f.Get(ctx, testActor, x.key, x.typ)
				}
				done <- err
			}()
			<-entered
			x.source.mu.Lock()
			x.source.hook = nil
			x.source.mu.Unlock()
			contact, err := x.f.ReadAcceptance(testActor, x.key)
			if err != nil {
				t.Fatal(err)
			}
			address := "local:changed"
			if list {
				address = "local:changed-again"
			}
			if _, err = x.f.UpdateCoreNextHop(testActor, contract.OperationID(address), x.key, contact.NextHopRevision, values.NextHop(address)); err != nil {
				t.Fatal(err)
			}
			close(release)
			mustError(t, <-done, contract.ErrSnapshotChanged)
			x.source.mu.Lock()
			x.source.address = values.NextHop(address)
			x.source.mu.Unlock()
		}
	})
}
func TestLatePutCannotBeSplicedIntoEarlierRead(t *testing.T) {
	profiles(t, func(t *testing.T, x *fixture) {
		entered, release := make(chan struct{}), make(chan struct{})
		var calls atomic.Int64
		x.source.hook = func() {
			if calls.Add(1) == 1 {
				close(entered)
				<-release
			}
		}
		done := make(chan error, 1)
		go func() { _, err := x.f.Get(context.Background(), testActor, x.key, x.typ); done <- err }()
		<-entered
		x.put(t, "late", "new", contract.ExpectAbsent())
		close(release)
		mustError(t, <-done, contract.ErrSnapshotChanged)
	})
}
func TestPutMayCommitAfterCoreDetachWithOriginalEvidence(t *testing.T) {
	profiles(t, func(t *testing.T, x *fixture) {
		entered, release := make(chan struct{}), make(chan struct{})
		x.source.hook = func() { close(entered); <-release }
		done := make(chan contract.MutationResult, 1)
		failed := make(chan error, 1)
		go func() {
			r, err := x.f.Put(context.Background(), testActor, x.key, x.typ, fixturePayload("late"), contract.ExpectAbsent(), "late")
			done <- r
			failed <- err
		}()
		<-entered
		x.source.mu.Lock()
		x.source.hook = nil
		x.source.current = testProof(x.key, contract.CoreDetached, 4)
		x.source.mu.Unlock()
		close(release)
		r, err := <-done, <-failed
		if err != nil || r.Status != contract.Committed || r.Core.State != contract.CoreActive {
			t.Fatalf("late write: %+v %v", r, err)
		}
		_, err = x.f.Get(context.Background(), testActor, x.key, x.typ)
		mustError(t, err, contract.ErrNamespaceDetached)
	})
}
func TestAcceptMayFinishAfterCancelAndConcurrentAcceptCannotRewriteContact(t *testing.T) {
	x := newFixture(t, false)
	key := testKey(x.key.Facet, x.key.Core, x.key.Element, 5)
	x.source.current = testProof(key, contract.CoreNone, 5)
	x.source.current.Pending = key
	entered, release := make(chan struct{}), make(chan struct{})
	x.source.hook = func() { close(entered); <-release }
	done := make(chan error, 1)
	go func() {
		_, err := x.f.AcceptAttachment(context.Background(), testActor, key, "local:core", "late")
		done <- err
	}()
	<-entered
	x.source.mu.Lock()
	x.source.current.Pending = values.FacetAcceptanceKey{}
	x.source.hook = nil
	x.source.mu.Unlock()
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	read, err := x.f.ReadAcceptance(testActor, key)
	if err != nil || !read.Found {
		t.Fatal("historical acceptance lost")
	}
}
func TestCallbacksAndCloseBoundary(t *testing.T) {
	x := newFixture(t, false)
	x.auth.hook = func() { _ = Close(x.f) }
	x.source.hook = func() { _ = Close(x.f) }
	x.put(t, "callbacks", "data", contract.ExpectAbsent())
	y := newFixture(t, true)
	entered, release := make(chan struct{}), make(chan struct{})
	y.source.hook = func() { close(entered); <-release }
	done := make(chan error, 1)
	go func() {
		_, err := y.f.Put(context.Background(), testActor, y.key, y.typ, []byte("null"), contract.ExpectAbsent(), "after-close")
		done <- err
	}()
	<-entered
	if err := Close(y.f); err != nil {
		t.Fatal(err)
	}
	close(release)
	mustError(t, <-done, contract.ErrStore)
}
