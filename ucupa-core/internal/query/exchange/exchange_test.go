package exchange

import (
	"reflect"
	"sync"
	"sync/atomic"
	"testing"

	"ucupa/shared/values"
)

func TestOpaqueEnvelopeRoundTrip(t *testing.T) {
	requestData := RequestData{Action: Borrow, Intent: Intent{Kind: Query}, Grant: Grant{Prefix: []values.ElementID{"a"}}, Run: [32]byte{1}, Lease: LeaseID{Run: [32]byte{2}, ID: 3}}
	request := NewRequest(requestData)
	if got := ReadRequest(request); !reflect.DeepEqual(got, requestData) {
		t.Fatalf("request round trip: %#v", got)
	}
	replyData := ReplyData{Grant: Grant{Adjacency: []values.ElementID{"b"}}, Authority: "owner"}
	reply := NewReply(replyData)
	if got := ReadReply(reply); !reflect.DeepEqual(got, replyData) {
		t.Fatalf("reply round trip: %#v", got)
	}
}

func TestCleanupDisposesOnceInRegistrationOrder(t *testing.T) {
	var order []string
	cleanup := &Cleanup{}
	for _, name := range []string{"one", "two"} {
		name := name
		if !cleanup.Add(CleanupItem{Finish: func() { order = append(order, "finish-"+name) }, Discard: func() { order = append(order, "discard-"+name) }}) {
			t.Fatal("open cleanup rejected item")
		}
	}
	cleanup.Close()
	cleanup.Close()
	want := []string{"finish-one", "discard-one", "finish-two", "discard-two"}
	if !reflect.DeepEqual(order, want) {
		t.Fatalf("dispose order: %v", order)
	}
	if cleanup.Add(CleanupItem{Finish: func() { order = append(order, "finish-late") }, Discard: func() { order = append(order, "discard-late") }}) {
		t.Fatal("closed cleanup retained late item")
	}
	want = append(want, "finish-late", "discard-late")
	if !reflect.DeepEqual(order, want) {
		t.Fatalf("late dispose order: %v", order)
	}
}

func TestCleanupPreservesOnlyPins(t *testing.T) {
	var finished, discarded int
	cleanup := &Cleanup{}
	cleanup.Add(CleanupItem{Finish: func() { finished++ }, Discard: func() { discarded++ }})
	cleanup.PreservePins()
	cleanup.Close()
	if finished != 1 || discarded != 0 {
		t.Fatalf("finish=%d discard=%d", finished, discarded)
	}
}

func TestConcurrentAddAndCloseDisposesEveryItemOnce(t *testing.T) {
	cleanup := &Cleanup{}
	var finished, discarded atomic.Int64
	var workers sync.WaitGroup
	start := make(chan struct{})
	for i := 0; i < 64; i++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			<-start
			cleanup.Add(CleanupItem{Finish: func() { finished.Add(1) }, Discard: func() { discarded.Add(1) }})
		}()
	}
	workers.Add(1)
	go func() { defer workers.Done(); <-start; cleanup.Close() }()
	close(start)
	workers.Wait()
	cleanup.Close()
	if finished.Load() != 64 || discarded.Load() != 64 {
		t.Fatalf("lost or double disposal: finished=%d discarded=%d", finished.Load(), discarded.Load())
	}
}
