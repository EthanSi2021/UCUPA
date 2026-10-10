package admission

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	networkpb "ucupa/shared/bindings/go/protobuf"
	"ucupa/shared/values"
)

var entryTestCaller = values.CallerContext{
	Subject:       values.PrincipalRef{Realm: "entry-realm", Principal: "entry-caller"},
	Credential:    [16]byte{1},
	Establishment: [32]byte{2},
	Proof:         [32]byte{3},
}

type entryTestAuthenticator struct {
	caller values.CallerContext
	calls  int
}

func (auth *entryTestAuthenticator) Establish(context.Context, values.RealmID, [16]byte, [32]byte) (values.CallerContext, error) {
	auth.calls++
	return auth.caller, nil
}

func entryEstablishRequest(seed byte) *networkpb.EstablishSessionRequest {
	return &networkpb.EstablishSessionRequest{
		Audience:     proto.String("entry-realm"),
		CredentialId: bytes.Repeat([]byte{seed}, 16),
		Secret:       bytes.Repeat([]byte{seed + 1}, 32),
	}
}

func entryBearerContext(token []byte) context.Context {
	return metadata.NewIncomingContext(context.Background(), metadata.Pairs("authorization", "Bearer "+values.EncodePublicBytes(token)))
}

func entryStoredSession(t *testing.T, service *Service, token []byte) ([32]byte, session) {
	t.Helper()
	if len(token) != 32 {
		t.Fatalf("session token length = %d, want 32", len(token))
	}
	key := sha256.Sum256(token)
	service.mu.Lock()
	entry, found := service.sessions[key]
	service.mu.Unlock()
	if !found {
		t.Fatal("established session is absent from the registry")
	}
	return key, entry
}

func requireEntryDenied(t *testing.T, err error) {
	t.Helper()
	if !errors.Is(err, values.PublicDenied) {
		t.Fatalf("entry error = %v, want %v", err, values.PublicDenied)
	}
}

func TestEntrySessionBindsCallerExpiryReleaseAndStop(t *testing.T) {
	auth := &entryTestAuthenticator{caller: entryTestCaller}
	service := New(nil, auth, Config{
		SessionTTL:           2 * time.Minute,
		MaxSessions:          4,
		MaxSessionsPerCaller: 4,
		MaxConcurrentRPCs:    1,
	})

	responses := make([]*networkpb.EstablishSessionResponse, 3)
	contexts := make([]context.Context, 3)
	keys := make([][32]byte, 3)
	records := make([]session, 3)
	entries := make([]EntrySession, 3)
	for index := range responses {
		out, err := service.EstablishSession(context.Background(), entryEstablishRequest(byte(index+1)))
		if err != nil || out == nil || out.GetExpiresInSeconds() != 120 {
			t.Fatalf("establish %d failed: %v %v", index, out, err)
		}
		responses[index] = out
		contexts[index] = entryBearerContext(out.Session)
		keys[index], records[index] = entryStoredSession(t, service, out.Session)
		entries[index], err = service.BindEntrySession(contexts[index], entryTestCaller)
		if err != nil {
			t.Fatalf("bind %d failed: %v", index, err)
		}
		if !entries[index].Expires().Equal(records[index].expires) {
			t.Fatalf("entry %d expiry = %v, want original %v", index, entries[index].Expires(), records[index].expires)
		}
	}
	if bytes.Equal(responses[0].Session, responses[1].Session) || entries[0] == entries[1] {
		t.Fatal("two establishments for the same caller collapsed into one session")
	}
	if auth.calls != 3 {
		t.Fatalf("authenticator calls = %d, want 3", auth.calls)
	}

	wrongCaller := entryTestCaller
	wrongCaller.Proof[0]++
	if _, err := service.BindEntrySession(contexts[0], wrongCaller); err == nil {
		t.Fatal("session bound to a different caller witness")
	} else {
		requireEntryDenied(t, err)
	}
	for index := range entries {
		if err := service.CheckEntrySession(entries[index]); err != nil {
			t.Fatalf("live entry %d failed its exact check: %v", index, err)
		}
	}

	released, err := service.ReleaseSession(contexts[0], &networkpb.ReleaseSessionRequest{})
	if err != nil || released == nil {
		t.Fatalf("release failed: %v %v", released, err)
	}
	requireEntryDenied(t, service.CheckEntrySession(entries[0]))
	if _, err := service.BindEntrySession(contexts[0], entryTestCaller); err == nil {
		t.Fatal("released session rebound")
	} else {
		requireEntryDenied(t, err)
	}
	if err := service.CheckEntrySession(entries[1]); err != nil {
		t.Fatalf("releasing one same-caller session invalidated another: %v", err)
	}

	service.mu.Lock()
	expired := service.sessions[keys[2]]
	expired.expires = time.Now().Add(-time.Second)
	service.sessions[keys[2]] = expired
	service.mu.Unlock()
	if _, err := service.BindEntrySession(contexts[2], entryTestCaller); err == nil {
		t.Fatal("expired session rebound")
	} else {
		requireEntryDenied(t, err)
	}
	requireEntryDenied(t, service.CheckEntrySession(entries[2]))
	service.mu.Lock()
	_, retainedExpired := service.sessions[keys[2]]
	service.mu.Unlock()
	if retainedExpired {
		t.Fatal("expired session remained in the registry")
	}

	if listener := service.BeginStop(); listener != nil {
		t.Fatal("entry-only service returned a listener at stop")
	}
	requireEntryDenied(t, service.CheckEntrySession(entries[1]))
	if _, err := service.BindEntrySession(contexts[1], entryTestCaller); err == nil {
		t.Fatal("stopped session rebound")
	} else {
		requireEntryDenied(t, err)
	}
	if out, err := service.EstablishSession(context.Background(), entryEstablishRequest(9)); out != nil || status.Code(err) != codes.Unavailable {
		t.Fatalf("stopped service established a replacement session: %v %v", out, err)
	}
	service.mu.Lock()
	remaining := len(service.sessions)
	service.mu.Unlock()
	if remaining != 0 || auth.calls != 4 {
		t.Fatalf("stopped registry revived: sessions=%d authenticatorCalls=%d", remaining, auth.calls)
	}
}
