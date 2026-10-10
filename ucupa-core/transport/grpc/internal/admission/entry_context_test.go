package admission

import (
	"context"
	"testing"
	"time"

	netpb "ucupa/shared/bindings/go/protobuf"
)

func TestEntryContextEndsWithOriginalSession(t *testing.T) {
	for _, mode := range []string{"release", "stop", "expiry", "request cancelled"} {
		t.Run(mode, func(t *testing.T) {
			s := New(nil, &entryTestAuthenticator{caller: entryTestCaller}, Config{SessionTTL: time.Minute, MaxSessions: 2, MaxSessionsPerCaller: 2})
			r, err := s.EstablishSession(context.Background(), entryEstablishRequest(1))
			if err != nil {
				t.Fatal(err)
			}
			bearer := entryBearerContext(r.Session)
			if mode == "expiry" {
				key, entry := entryStoredSession(t, s, r.Session)
				entry.expires = time.Now().Add(40 * time.Millisecond)
				s.sessions[key] = entry
			}
			entry, err := s.BindEntrySession(bearer, entryTestCaller)
			if err != nil {
				t.Fatal(err)
			}
			parent, stop := context.WithCancel(bearer)
			defer stop()
			ctx, cancel, err := s.EntryContext(parent, entry)
			if err != nil {
				t.Fatal(err)
			}
			defer cancel()
			if ctx.Err() != nil {
				t.Fatal("new context cancelled")
			}
			switch mode {
			case "release":
				if _, err := s.ReleaseSession(bearer, &netpb.ReleaseSessionRequest{}); err != nil {
					t.Fatal(err)
				}
			case "stop":
				s.BeginStop()
			case "request cancelled":
				stop()
			case "expiry":
				<-ctx.Done()
			}
			// Do not wait for the cancellation callback: Err is the owner's final
			// synchronous guard after an external wait.
			if ctx.Err() == nil {
				t.Fatal("ended session still authorizes work")
			}
			select {
			case <-ctx.Done():
			default:
				t.Fatal("Err became non-nil before Done closed")
			}
			select {
			case <-ctx.Done():
			case <-time.After(time.Second):
				t.Fatal("network wait not cancelled")
			}
		})
	}
}

func TestEntryContextCancellationIsNotAnotherSession(t *testing.T) {
	s := New(nil, &entryTestAuthenticator{caller: entryTestCaller}, Config{SessionTTL: time.Minute, MaxSessions: 2, MaxSessionsPerCaller: 2})
	r, err := s.EstablishSession(context.Background(), entryEstablishRequest(2))
	if err != nil {
		t.Fatal(err)
	}
	bearer := entryBearerContext(r.Session)
	entry, err := s.BindEntrySession(bearer, entryTestCaller)
	if err != nil {
		t.Fatal(err)
	}
	a, cancelA, err := s.EntryContext(bearer, entry)
	if err != nil {
		t.Fatal(err)
	}
	b, cancelB, err := s.EntryContext(bearer, entry)
	if err != nil {
		t.Fatal(err)
	}
	defer cancelB()
	cancelA()
	if a.Err() == nil || b.Err() != nil || s.CheckEntrySession(entry) != nil {
		t.Fatal("one RPC cancelled session or another RPC")
	}
	if _, _, err := s.EntryContext(bearer, EntrySession{}); err == nil {
		t.Fatal("invented session accepted")
	}
	s.BeginStop()
}

func TestSessionContextErrClosesDoneWithoutNotificationCallback(t *testing.T) {
	for _, expiry := range []bool{false, true} {
		base, cancel := context.WithCancel(context.Background())
		lifetime := newSessionLifetime()
		expires := time.Now().Add(time.Minute)
		if expiry {
			expires = time.Now().Add(-time.Second)
		} else {
			lifetime.stop()
		}
		// Deliberately install no AfterFunc: test the final owner guard when
		// notification has not run, without relying on scheduler timing.
		ctx := sessionContext{Context: base, lifetime: lifetime.context, expires: expires, cancel: cancel}
		if ctx.Err() == nil {
			t.Fatal("ended lifetime ignored")
		}
		select {
		case <-ctx.Done():
		default:
			t.Fatal("Err and Done disagree")
		}
		cancel()
		lifetime.stop()
	}
}
