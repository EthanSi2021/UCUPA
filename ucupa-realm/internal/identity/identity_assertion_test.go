package identity

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"ucupa/shared/values"
	contract "ucupa/ucupa-realm/contract"
	"ucupa/ucupa-realm/internal/policy"
)

// This fixture uses the real policy owner; the issuer still owns no policy state.
type Realm = policy.Realm

func viewSeed() (contract.Seed, values.PrincipalRef, contract.PermissionBinding) {
	actor := values.PrincipalRef{Realm: "views", Principal: "manager"}
	target := contract.PermissionBinding{Subject: actor, Permission: "observe", Scope: values.AnchorClosureScope("00000000-0000-4000-8000-000000000301", "00000000-0000-4000-8000-000000000101")}
	return contract.Seed{ID: actor.Realm, LocalPrincipals: []values.PrincipalID{actor.Principal}, Bindings: []contract.PermissionBinding{
		administrationBinding(actor, contract.PermissionPutBinding, actor.Realm),
		administrationBinding(actor, contract.PermissionDeleteBinding, actor.Realm), target,
	}}, actor, target
}

func administrationBinding(subject values.PrincipalRef, permission values.PermissionID, id values.RealmID) contract.PermissionBinding {
	return contract.PermissionBinding{Subject: subject, Permission: permission, Scope: values.RealmScope(id)}
}

func testCredentialEvidence(id values.RealmID, idByte, secretByte byte) contract.CredentialEvidence {
	evidence := contract.CredentialEvidence{Audience: id}
	for i := range evidence.ID {
		evidence.ID[i] = idByte
	}
	for i := range evidence.Secret {
		evidence.Secret[i] = secretByte
	}
	return evidence
}

func mustRealm(t *testing.T, seed contract.Seed) *Realm {
	t.Helper()
	r, err := policy.New(seed)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func identityFixture(t *testing.T) (*Realm, *IdentityIssuer, *IdentityVerifier, values.CallerContext, contract.CredentialEvidence) {
	t.Helper()
	seed, actor, _ := viewSeed()
	evidence := testCredentialEvidence(seed.ID, 1, 11)
	manager := testCredentialEvidence(seed.ID, 2, 12)
	seed.Credentials = []contract.CredentialBootstrap{{Subject: actor.Principal, Evidence: evidence}, {Subject: actor.Principal, Evidence: manager}}
	seed.Bindings = append(seed.Bindings, administrationBinding(actor, contract.PermissionRevokeCredential, seed.ID))
	r := mustRealm(t, seed)
	caller, err := r.EstablishCaller(evidence)
	if err != nil {
		t.Fatal(err)
	}
	key := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{7}, 32))
	s, err := NewIdentityIssuer(r, contract.IdentityIssuerConfig{Key: key, MaxTTL: time.Minute, MaxProofs: 3, MaxPerCaller: 2})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	s.now = func() time.Time { return time.Unix(2000000000, 0) }
	v, err := NewIdentityVerifier(map[values.RealmID]ed25519.PublicKey{r.ID(): key.Public().(ed25519.PublicKey)}, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	clear(key)
	return r, s, v, caller, manager
}

const identityActor values.FactAuthorityRef = "00000000-0000-4000-8000-000000000111"

func TestIdentityProofBindingAndLiveness(t *testing.T) {
	r, s, v, caller, evidence := identityFixture(t)
	challenge := [32]byte{4}
	p, err := s.Issue(context.Background(), caller, identityActor, challenge, 20*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	c, err := v.Verify(p, identityActor, challenge, s.now())
	if err != nil || c.Issuer != caller.Subject.Realm || c.Subject != caller.Subject.Principal {
		t.Fatalf("claim: %+v %v", c, err)
	}
	if err := s.Check(context.Background(), p); err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(p.Payload, []byte("credential")) || bytes.Contains(p.Payload, []byte("nextHop")) {
		t.Fatal("private evidence in proof")
	}
	if _, err := v.Verify(p, identityActor, [32]byte{5}, s.now()); err != contract.ErrUnauthenticated {
		t.Fatal("wrong challenge", err)
	}
	if _, err := v.Verify(p, "00000000-0000-4000-8000-000000000112", challenge, s.now()); err != contract.ErrUnauthenticated {
		t.Fatal("wrong actor", err)
	}
	if _, err := v.Verify(p, identityActor, challenge, s.now().Add(20*time.Second)); err != contract.ErrUnauthenticated {
		t.Fatal("expiry", err)
	}
	if _, err := v.Verify(p, identityActor, challenge, s.now().Add(-time.Second)); err != contract.ErrUnauthenticated {
		t.Fatal("future", err)
	}
	if _, err := r.RevokeCredential(evidence, contract.CredentialID(caller.Credential), r.CredentialSetRevision(), "identity-revoke"); err != nil {
		t.Fatal(err)
	}
	if err := s.Check(context.Background(), p); err != contract.ErrUnauthenticated {
		t.Fatal("revoked", err)
	}
	// A signature remains mathematically valid after revoke. This is why
	// signature-only acceptance is forbidden and online liveness is required.
	if _, err := v.Verify(p, identityActor, challenge, s.now()); err != nil {
		t.Fatal(err)
	}
}

func TestIdentityProofStrictEncodingAndIssuerTrust(t *testing.T) {
	_, s, v, caller, _ := identityFixture(t)
	challenge := [32]byte{4}
	p, err := s.Issue(context.Background(), caller, identityActor, challenge, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(*contract.IdentityProof){
		func(p *contract.IdentityProof) { p.Signature[0] ^= 1 },
		func(p *contract.IdentityProof) { p.Payload = append(p.Payload, '\n') },
		func(p *contract.IdentityProof) { p.Payload = bytes.Repeat([]byte{'x'}, 4097) },
		func(p *contract.IdentityProof) { p.Signature = p.Signature[:63] },
	} {
		copy := contract.IdentityProof{Payload: append([]byte(nil), p.Payload...), Signature: append([]byte(nil), p.Signature...)}
		mutate(&copy)
		if _, err := v.Verify(copy, identityActor, challenge, s.now()); err != contract.ErrUnauthenticated {
			t.Fatal("malformed proof accepted", err)
		}
		if err := s.Check(context.Background(), copy); err != contract.ErrUnauthenticated {
			t.Fatal("malformed liveness", err)
		}
	}
	var fields map[string]any
	if err := json.Unmarshal(p.Payload, &fields); err != nil {
		t.Fatal(err)
	}
	fields["permission"] = "admin"
	payload, _ := values.CanonicalJSONValue(fields)
	bad := contract.IdentityProof{Payload: payload, Signature: ed25519.Sign(s.config.Key, append([]byte(identityDomain), payload...))}
	if _, err := v.Verify(bad, identityActor, challenge, s.now()); err != contract.ErrUnauthenticated {
		t.Fatal("unknown field", err)
	}
	key := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{8}, 32))
	wrong, _ := NewIdentityVerifier(map[values.RealmID]ed25519.PublicKey{caller.Subject.Realm: key.Public().(ed25519.PublicKey)}, time.Minute)
	if _, err := wrong.Verify(p, identityActor, challenge, s.now()); err != contract.ErrUnauthenticated {
		t.Fatal("wrong signing key", err)
	}
	unknown, _ := NewIdentityVerifier(map[values.RealmID]ed25519.PublicKey{"other": s.config.Key.Public().(ed25519.PublicKey)}, time.Minute)
	if _, err := unknown.Verify(p, identityActor, challenge, s.now()); err != contract.ErrUnauthenticated {
		t.Fatal("issuer namespace", err)
	}
}

func TestIdentityProofAdmissionExpirationAndRestart(t *testing.T) {
	r, s, _, caller, _ := identityFixture(t)
	local, _ := r.TrustLocalCaller(caller.Subject)
	if _, err := s.Issue(context.Background(), local, identityActor, [32]byte{1}, time.Second); err != contract.ErrUnauthenticated {
		t.Fatal("local caller signed", err)
	}
	for _, ttl := range []time.Duration{0, -time.Second, time.Millisecond, 61 * time.Second} {
		if _, err := s.Issue(context.Background(), caller, identityActor, [32]byte{1}, ttl); err != contract.ErrInvalid {
			t.Fatal("bad TTL", err)
		}
	}
	var mu sync.Mutex
	var proofs []contract.IdentityProof
	var wg sync.WaitGroup
	for range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			p, err := s.Issue(context.Background(), caller, identityActor, [32]byte{1}, time.Second)
			if err != nil && !errors.Is(err, values.PublicResourceExhausted) {
				t.Error(err)
			}
			if err == nil {
				mu.Lock()
				proofs = append(proofs, p)
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if len(proofs) != 2 {
		t.Fatal("per-caller bound", len(proofs))
	}
	key := append(ed25519.PrivateKey(nil), s.config.Key...)
	restarted, err := NewIdentityIssuer(r, contract.IdentityIssuerConfig{Key: key, MaxTTL: time.Minute, MaxProofs: 3, MaxPerCaller: 2})
	if err != nil {
		t.Fatal(err)
	}
	defer restarted.Close()
	if err := restarted.Check(context.Background(), proofs[0]); err != contract.ErrUnauthenticated {
		t.Fatal("proof revived after issuer restart", err)
	}
	s.now = func() time.Time { return time.Unix(2000000001, 0) }
	if err := s.Check(context.Background(), proofs[0]); err != contract.ErrUnauthenticated {
		t.Fatal("expired proof", err)
	}
	if _, err := s.Issue(context.Background(), caller, identityActor, [32]byte{1}, time.Second); err != nil {
		t.Fatal("capacity not reclaimed", err)
	}
	s.Close()
	if err := s.Check(context.Background(), proofs[1]); err != contract.ErrStore {
		t.Fatal("closed issuer", err)
	}
	if _, err := s.Issue(context.Background(), caller, identityActor, [32]byte{1}, time.Second); err != contract.ErrStore {
		t.Fatal("closed issue", err)
	}
}

// gatedCallerSource turns the issuer's caller re-validation into an explicit
// barrier: the first fork signals its arrival and waits for the release,
// every later fork passes straight through, so concurrent activity during a
// gated wait stays deterministic without any sleeps.
type gatedCallerSource struct {
	base    *Realm
	entered atomic.Bool
	seen    chan struct{}
	release chan struct{}
}

func (s *gatedCallerSource) ForkCaller(ctx context.Context, c values.CallerContext) (values.CallerContext, error) {
	if s.entered.CompareAndSwap(false, true) {
		close(s.seen)
		<-s.release
	}
	return s.base.ForkCaller(ctx, c)
}

func gateIssuerFork(issuer *IdentityIssuer, r *Realm) *gatedCallerSource {
	source := &gatedCallerSource{base: r, seen: make(chan struct{}), release: make(chan struct{})}
	issuer.source = source
	return source
}

func awaitIssuerFork(t *testing.T, source *gatedCallerSource) {
	t.Helper()
	select {
	case <-source.seen:
	case <-time.After(5 * time.Second):
		t.Fatal("the caller re-validation never consulted the caller source")
	}
}

// TestIdentityIssuerForkRunsOutsideRegistryLock proves the caller
// re-validation is a real wait that never holds the issuer's registry lock:
// while the online check is gated, the registry stays lockable, and after
// the release the same in-date proof passes through the identical normal
// chain, so the wait never poisons a live statement.
func TestIdentityIssuerForkRunsOutsideRegistryLock(t *testing.T) {
	r, issuer, _, caller, _ := identityFixture(t)
	proof, err := issuer.Issue(context.Background(), caller, identityActor, [32]byte{4}, 20*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	source := gateIssuerFork(issuer, r)
	done := make(chan error, 1)
	go func() { done <- issuer.Check(context.Background(), proof) }()
	awaitIssuerFork(t, source)
	if !issuer.mu.TryLock() {
		t.Fatal("the online caller fork held the issuer registry lock")
	}
	issuer.mu.Unlock()
	close(source.release)
	if err := <-done; err != nil {
		t.Fatalf("the in-date proof failed through the same chain after the wait: %v", err)
	}
}

// TestIdentityIssuerCheckRejectsExpiryDuringForkWait proves the online wait
// never extends a proof's life: the statement is valid when the gated check
// starts, the controlled clock passes its expiry while the fork is still
// waiting, and the finished wait rejects the statement.
func TestIdentityIssuerCheckRejectsExpiryDuringForkWait(t *testing.T) {
	r, issuer, _, caller, _ := identityFixture(t)
	var seconds atomic.Int64
	seconds.Store(2000000000)
	issuer.now = func() time.Time { return time.Unix(seconds.Load(), 0) }
	proof, err := issuer.Issue(context.Background(), caller, identityActor, [32]byte{4}, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	source := gateIssuerFork(issuer, r)
	done := make(chan error, 1)
	go func() { done <- issuer.Check(context.Background(), proof) }()
	awaitIssuerFork(t, source)
	seconds.Add(2)
	close(source.release)
	if err := <-done; err != contract.ErrUnauthenticated {
		t.Fatalf("a proof expired during the online wait passed: %v", err)
	}
}

// TestIdentityIssuerRejectsCloseDuringForkWait proves a concurrent close
// during the online wait decides the in-flight check when it returns, and
// the closed registry never issues or checks anything again.
func TestIdentityIssuerRejectsCloseDuringForkWait(t *testing.T) {
	r, issuer, _, caller, _ := identityFixture(t)
	proof, err := issuer.Issue(context.Background(), caller, identityActor, [32]byte{4}, 20*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	source := gateIssuerFork(issuer, r)
	done := make(chan error, 1)
	go func() { done <- issuer.Check(context.Background(), proof) }()
	awaitIssuerFork(t, source)
	issuer.Close()
	close(source.release)
	if err := <-done; err != contract.ErrStore {
		t.Fatalf("a check that crossed a close during its wait = %v", err)
	}
	if _, err := issuer.Issue(context.Background(), caller, identityActor, [32]byte{10}, time.Second); err != contract.ErrStore {
		t.Fatalf("issue after close = %v", err)
	}
	if err := issuer.Check(context.Background(), proof); err != contract.ErrStore {
		t.Fatalf("check after close = %v", err)
	}
}

// TestIdentityIssuerIssueRejectsCloseDuringForkWait proves the symmetric
// issuance boundary: an issue whose caller fork is still gated when the
// registry closes never registers its statement afterwards.
func TestIdentityIssuerIssueRejectsCloseDuringForkWait(t *testing.T) {
	r, issuer, _, caller, _ := identityFixture(t)
	source := gateIssuerFork(issuer, r)
	done := make(chan error, 1)
	go func() {
		_, err := issuer.Issue(context.Background(), caller, identityActor, [32]byte{8}, 20*time.Second)
		done <- err
	}()
	awaitIssuerFork(t, source)
	issuer.Close()
	close(source.release)
	if err := <-done; err != contract.ErrStore {
		t.Fatalf("an issue that crossed a close during its wait = %v", err)
	}
}

// TestIdentityIssuerIssueRespectsCapacityFilledDuringForkWait proves the
// issuance decision re-reads the registry after the fork: while the gated
// issue waits, two sibling proofs pass through and fill the caller's
// per-caller capacity, and the returning issue must not register a third.
func TestIdentityIssuerIssueRespectsCapacityFilledDuringForkWait(t *testing.T) {
	r, issuer, _, caller, _ := identityFixture(t)
	source := gateIssuerFork(issuer, r)
	done := make(chan error, 1)
	go func() {
		_, err := issuer.Issue(context.Background(), caller, identityActor, [32]byte{8}, 20*time.Second)
		done <- err
	}()
	awaitIssuerFork(t, source)
	if _, err := issuer.Issue(context.Background(), caller, identityActor, [32]byte{1}, 20*time.Second); err != nil {
		t.Fatal(err)
	}
	if _, err := issuer.Issue(context.Background(), caller, identityActor, [32]byte{2}, 20*time.Second); err != nil {
		t.Fatal(err)
	}
	close(source.release)
	if err := <-done; err != values.PublicResourceExhausted {
		t.Fatalf("an issue after its caller's capacity filled during the wait = %v", err)
	}
}

// A remote fork may have succeeded before cancellation becomes visible to
// its caller. The issuer must still reject the canceled final decision and
// release only the temporary fork, never the basis of a retained proof.
type releasingCallerSource struct {
	*gatedCallerSource
	issuer *IdentityIssuer
	calls  int
	t      *testing.T
}

func (s *releasingCallerSource) ForkCaller(_ context.Context, caller values.CallerContext) (values.CallerContext, error) {
	return s.gatedCallerSource.ForkCaller(context.Background(), caller)
}

func (s *releasingCallerSource) ReleaseCaller(_ context.Context, _ values.CallerContext) {
	if !s.issuer.mu.TryLock() {
		s.t.Error("temporary caller cleanup held the issuer registry lock")
		return
	}
	s.issuer.mu.Unlock()
	s.calls++
}

func TestIdentityIssuerCanceledWaitReleasesOnlyTemporaryCaller(t *testing.T) {
	for _, operation := range []string{"issue", "check"} {
		t.Run(operation, func(t *testing.T) {
			r, issuer, _, caller, _ := identityFixture(t)
			proof, err := issuer.Issue(context.Background(), caller, identityActor, [32]byte{4}, 20*time.Second)
			if err != nil {
				t.Fatal(err)
			}
			gate := gateIssuerFork(issuer, r)
			source := &releasingCallerSource{gatedCallerSource: gate, issuer: issuer, t: t}
			issuer.source = source
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			done := make(chan error, 1)
			go func() {
				if operation == "check" {
					done <- issuer.Check(ctx, proof)
					return
				}
				_, err := issuer.Issue(ctx, caller, identityActor, [32]byte{5}, 20*time.Second)
				done <- err
			}()
			awaitIssuerFork(t, gate)
			cancel()
			close(gate.release)
			if err := <-done; !errors.Is(err, context.Canceled) {
				t.Fatalf("canceled final %s decision = %v", operation, err)
			}
			if source.calls != 1 || len(issuer.proofs) != 1 {
				t.Fatalf("cleanup=%d retained proofs=%d", source.calls, len(issuer.proofs))
			}
			if err := issuer.Check(context.Background(), proof); err != nil {
				t.Fatalf("original proof was prematurely released: %v", err)
			}
			if _, err := issuer.Issue(context.Background(), caller, identityActor, [32]byte{6}, 20*time.Second); err != nil {
				t.Fatalf("fresh issue after cancellation = %v", err)
			}
			if source.calls != 2 {
				t.Fatalf("successful issue released retained caller: cleanup=%d", source.calls)
			}
		})
	}
}
