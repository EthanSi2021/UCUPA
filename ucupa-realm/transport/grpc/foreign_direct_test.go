package realmgrpc

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	"ucupa/shared/values"
	realmpb "ucupa/ucupa-realm/bindings/go/protobuf"
	realm "ucupa/ucupa-realm/contract"
	realminstance "ucupa/ucupa-realm/instance"
	admission "ucupa/ucupa-realm/transport/grpc/internal/admission"
)

const issuerRealmID values.RealmID = "issuer-realm"
const issuerTTL = 30 * time.Second

// livenessAdapter presents the issuer's authenticated Check port as the
// transport's configured online liveness source.
type livenessAdapter struct {
	issuer *realminstance.IdentityIssuer
}

func (a livenessAdapter) Check(ctx context.Context, payload, signature []byte) error {
	return a.issuer.Check(ctx, realm.IdentityProof{Payload: payload, Signature: signature})
}

// foreignFixture is one independent issuer Realm with a credential-backed
// subject, mirroring the P6 local profile of a foreign identity source.
type foreignFixture struct {
	issuer   *realminstance.IdentityIssuer
	verifier *realminstance.IdentityVerifier
	Caller   values.CallerContext
}

func newForeignFixture(t *testing.T) *foreignFixture {
	t.Helper()
	_, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	var evidence realm.CredentialEvidence
	evidence.Audience = issuerRealmID
	if _, err := rand.Read(evidence.ID[:]); err != nil {
		t.Fatal(err)
	}
	if _, err := rand.Read(evidence.Secret[:]); err != nil {
		t.Fatal(err)
	}
	source, err := realminstance.New(realm.Seed{
		ID:              issuerRealmID,
		LocalPrincipals: []values.PrincipalID{"subject-x"},
		Credentials:     []realm.CredentialBootstrap{{Subject: "subject-x", Evidence: evidence}},
	})
	if err != nil {
		t.Fatal(err)
	}
	issuer, err := realminstance.NewIdentityIssuer(source, realm.IdentityIssuerConfig{Key: key, MaxTTL: issuerTTL, MaxProofs: 8, MaxPerCaller: 8})
	if err != nil {
		t.Fatal(err)
	}
	verifier, err := realminstance.NewIdentityVerifier(map[values.RealmID]ed25519.PublicKey{issuerRealmID: key.Public().(ed25519.PublicKey)}, issuerTTL)
	if err != nil {
		t.Fatal(err)
	}
	caller, err := source.EstablishCaller(evidence)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(issuer.Close)
	return &foreignFixture{issuer: issuer, verifier: verifier, Caller: caller}
}

// configure declares issuer trust and its online liveness source on a service
// configuration; it is applied before New validates the pairing.
func (x *foreignFixture) configure(c *Config) {
	c.Foreign = x.verifier
	c.ForeignLiveness = map[values.RealmID]Liveness{issuerRealmID: livenessAdapter{issuer: x.issuer}}
}

func (x *foreignFixture) challenge(t *testing.T, server *Server, ctx context.Context) []byte {
	t.Helper()
	out, err := server.service.BeginChallenge(ctx, &realmpb.BeginChallengeRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if len(out.GetChallenge()) != 32 {
		t.Fatalf("challenge length = %d", len(out.GetChallenge()))
	}
	return out.GetChallenge()
}

func (x *foreignFixture) issue(t *testing.T, actor values.FactAuthorityRef, challenge []byte, ttl time.Duration) realm.IdentityProof {
	t.Helper()
	proof, err := x.issuer.Issue(context.Background(), x.Caller, actor, [32]byte(challenge), ttl)
	if err != nil {
		t.Fatal(err)
	}
	return proof
}

func (x *foreignFixture) present(t *testing.T, server *Server, ctx context.Context, challenge []byte, proof realm.IdentityProof) []byte {
	t.Helper()
	out, err := server.service.EstablishForeignCaller(ctx, &realmpb.EstablishForeignCallerRequest{
		Actor:     proto.String(string(handleTestOwner)),
		Payload:   proof.Payload,
		Signature: proof.Signature,
		Challenge: challenge,
	})
	if err != nil {
		t.Fatal(err)
	}
	return out.GetCaller()
}

// establish runs the full challenge, issue and presentation chain.
func (x *foreignFixture) establish(t *testing.T, server *Server, ctx context.Context, ttl time.Duration) []byte {
	t.Helper()
	challenge := x.challenge(t, server, ctx)
	owner, err := admission.OwnerFromContext(ctx)
	if err != nil {
		t.Fatal(err)
	}
	proof := x.issue(t, owner, challenge, ttl)
	return x.present(t, server, ctx, challenge, proof)
}

// foreignVerifierForTest builds issuer trust without any liveness source; it
// is only used to exercise configuration validation.
func foreignVerifierForTest(t *testing.T) *realminstance.IdentityVerifier {
	t.Helper()
	_, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	verifier, err := realminstance.NewIdentityVerifier(map[values.RealmID]ed25519.PublicKey{issuerRealmID: key.Public().(ed25519.PublicKey)}, issuerTTL)
	if err != nil {
		t.Fatal(err)
	}
	return verifier
}

func foreignRequest(challenge []byte, proof realm.IdentityProof) *realmpb.EstablishForeignCallerRequest {
	return &realmpb.EstablishForeignCallerRequest{
		Actor:     proto.String(string(handleTestOwner)),
		Payload:   proof.Payload,
		Signature: proof.Signature,
		Challenge: challenge,
	}
}

func TestEstablishForeignCallerDirectFullChain(t *testing.T) {
	foreign := newForeignFixture(t)
	f := newDirectFixture(t, foreign.configure)
	ctx := ownerCtx(handleTestOwner)
	handle := foreign.establish(t, f.s, ctx, 10*time.Second)
	if len(handle) != 32 {
		t.Fatalf("handle length = %d", len(handle))
	}
	out, err := f.s.service.AuthorizeView(ctx, &realmpb.AuthorizeViewRequest{
		Caller:       handle,
		Requirements: []*realmpb.Requirement{f.userRequirement()},
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.GetDecision() != realmpb.Decision_VALID || len(out.GetView()) != 32 {
		t.Fatalf("foreign authorization = %v", out)
	}
	// A requirement the foreign subject lacks is DENIED, not unavailable.
	missing := coreElementRequirement("ucupa.test.write")
	denied, err := f.s.service.AuthorizeView(ctx, &realmpb.AuthorizeViewRequest{
		Caller:       handle,
		Requirements: []*realmpb.Requirement{missing},
	})
	if err != nil {
		t.Fatal(err)
	}
	if denied.GetDecision() != realmpb.Decision_DENIED || denied.GetView() != nil {
		t.Fatalf("foreign denial = %v", denied)
	}
}

// P6 signs the relay actor, not the receiving service owner. The admitted
// receiver still owns the challenge and every resulting handle.
func TestForeignRelayActorDiffersFromReceivingOwner(t *testing.T) {
	foreign := newForeignFixture(t)
	f := newDirectFixture(t, foreign.configure)
	ctx := ownerCtx(handleTestOwner)
	challenge := foreign.challenge(t, f.s, ctx)
	proof := foreign.issue(t, handleOtherOwner, challenge, 10*time.Second)
	request := &realmpb.EstablishForeignCallerRequest{Actor: proto.String(string(handleOtherOwner)), Payload: proof.Payload, Signature: proof.Signature, Challenge: challenge}
	wrong := proto.Clone(request).(*realmpb.EstablishForeignCallerRequest)
	wrong.Actor = proto.String(string(handleTestOwner))
	if _, err := f.s.service.EstablishForeignCaller(ctx, wrong); status.Code(err) != codes.Unauthenticated {
		t.Fatalf("actor substitution accepted: %v", err)
	}
	wrong.Actor = nil
	if _, err := f.s.service.EstablishForeignCaller(ctx, wrong); status.Code(err) != codes.Unauthenticated {
		t.Fatalf("missing actor accepted: %v", err)
	}
	out, err := f.s.service.EstablishForeignCaller(ctx, request)
	if err != nil || out.GetSubjectRealm() != "issuer-realm" || out.GetSubjectPrincipal() != "subject-x" {
		t.Fatalf("independent relay actor: %v %v", out, err)
	}
	if _, err := f.s.service.AuthorizeView(ownerCtx(handleOtherOwner), authorizeRequest(out.GetCaller(), f.userRequirement())); status.Code(err) != codes.Unauthenticated {
		t.Fatalf("relay actor reused receiver handle: %v", err)
	}
	if _, err := f.s.service.EstablishForeignCaller(ctx, request); status.Code(err) != codes.Unauthenticated {
		t.Fatalf("consumed challenge reused: %v", err)
	}
}

func TestEstablishForeignCallerDirectRejectsMisboundAndUntrustedProofs(t *testing.T) {
	foreign := newForeignFixture(t)
	// An independent issuer with the same realm identity but a different key:
	// its proofs are well-formed yet not trusted by the receiving verifier.
	untrusted := newForeignFixture(t)
	f := newDirectFixture(t, foreign.configure)
	ctx := ownerCtx(handleTestOwner)
	challenge := foreign.challenge(t, f.s, ctx)

	// A proof bound to another receiving owner is rejected.
	wrongActor := foreign.issue(t, handleOtherOwner, challenge, 10*time.Second)
	if _, err := f.s.service.EstablishForeignCaller(ctx, foreignRequest(challenge, wrongActor)); status.Code(err) != codes.Unauthenticated {
		t.Fatalf("wrong actor = %v", err)
	}

	// A proof signed by a key the receiver does not trust is rejected.
	alien := untrusted.issue(t, handleTestOwner, challenge, 10*time.Second)
	if _, err := f.s.service.EstablishForeignCaller(ctx, foreignRequest(challenge, alien)); status.Code(err) != codes.Unauthenticated {
		t.Fatalf("untrusted key = %v", err)
	}

	// Neither refusal spent the still-pending challenge.
	proof := foreign.issue(t, handleTestOwner, challenge, 10*time.Second)
	foreign.present(t, f.s, ctx, challenge, proof)

	// The proof binds exactly one challenge; presenting it again is refused.
	if _, err := f.s.service.EstablishForeignCaller(ctx, foreignRequest(challenge, proof)); status.Code(err) != codes.Unauthenticated {
		t.Fatalf("replayed challenge = %v", err)
	}
	if f.s.admission.Size() != 1 {
		t.Fatalf("handles = %d", f.s.admission.Size())
	}
}

func TestEstablishForeignCallerDirectRejectsForeignOwnerChallenge(t *testing.T) {
	foreign := newForeignFixture(t)
	f := newDirectFixture(t, foreign.configure)
	// A challenge issued to another admitted service cannot be spent here.
	other := foreign.challenge(t, f.s, ownerCtx(handleOtherOwner))
	proof := foreign.issue(t, handleTestOwner, other, 10*time.Second)
	if _, err := f.s.service.EstablishForeignCaller(ownerCtx(handleTestOwner), foreignRequest(other, proof)); status.Code(err) != codes.Unauthenticated {
		t.Fatalf("cross-owner challenge = %v", err)
	}
}

func TestEstablishForeignCallerDirectRejectsStaleChallenge(t *testing.T) {
	foreign := newForeignFixture(t)
	f := newDirectFixture(t, func(c *Config) {
		c.ChallengeTTL = 10 * time.Second
		foreign.configure(c)
	})
	ctx := ownerCtx(handleTestOwner)
	challenge := foreign.challenge(t, f.s, ctx)
	f.clock.advance(10 * time.Second)
	// The expired challenge alone is refused before any proof is examined.
	if _, err := f.s.service.EstablishForeignCaller(ctx, &realmpb.EstablishForeignCallerRequest{
		Actor:     proto.String(string(handleTestOwner)),
		Payload:   make([]byte, 8),
		Signature: make([]byte, 64),
		Challenge: challenge,
	}); status.Code(err) != codes.Unauthenticated {
		t.Fatalf("stale challenge = %v", err)
	}
}

func TestEstablishForeignCallerDirectRejectsExpiredProof(t *testing.T) {
	foreign := newForeignFixture(t)
	f := newDirectFixture(t, foreign.configure)
	ctx := ownerCtx(handleTestOwner)
	challenge := foreign.challenge(t, f.s, ctx)
	proof := foreign.issue(t, handleTestOwner, challenge, 2*time.Second)
	// Whole-second expiry is rounded down; one extra second is definitely past.
	f.clock.advance(3 * time.Second)
	if _, err := f.s.service.EstablishForeignCaller(ctx, foreignRequest(challenge, proof)); status.Code(err) != codes.Unauthenticated {
		t.Fatalf("expired proof = %v", err)
	}
}

func TestEstablishForeignCallerDirectRefusesSubSecondLeaseFloor(t *testing.T) {
	foreign := newForeignFixture(t)
	f := newDirectFixture(t, foreign.configure)
	ctx := ownerCtx(handleTestOwner)
	challenge := foreign.challenge(t, f.s, ctx)
	proof := foreign.issue(t, handleTestOwner, challenge, 2*time.Second)
	var claim struct {
		Expires int64 `json:"expires"`
	}
	if err := json.Unmarshal(proof.Payload, &claim); err != nil {
		t.Fatal(err)
	}
	// Move the service clock inside the proof's final second: the remaining
	// lease rounds down to zero whole seconds and must be refused.
	untilExpiry := time.Until(time.Unix(claim.Expires, 0))
	if untilExpiry <= 500*time.Millisecond || untilExpiry > 2*time.Second {
		t.Fatalf("unexpected remaining lease %v", untilExpiry)
	}
	f.clock.advance(untilExpiry - 500*time.Millisecond)
	if _, err := f.s.service.EstablishForeignCaller(ctx, foreignRequest(challenge, proof)); status.Code(err) != codes.Unauthenticated {
		t.Fatalf("sub-second lease floor = %v", err)
	}
}

func TestEstablishForeignCallerDirectReportsOfflineIssuer(t *testing.T) {
	foreign := newForeignFixture(t)
	f := newDirectFixture(t, foreign.configure)
	ctx := ownerCtx(handleTestOwner)
	challenge := foreign.challenge(t, f.s, ctx)
	proof := foreign.issue(t, handleTestOwner, challenge, 10*time.Second)
	foreign.issuer.Close()
	if _, err := f.s.service.EstablishForeignCaller(ctx, foreignRequest(challenge, proof)); status.Code(err) != codes.Unavailable {
		t.Fatalf("offline issuer = %v", err)
	}
}

func TestChallengeCapacityDirectIsBoundedAndExpiring(t *testing.T) {
	f := newDirectFixture(t, func(c *Config) {
		c.MaxChallenges = 2
		c.MaxChallengesPerOwner = 1
		c.ChallengeTTL = 10 * time.Second
	})
	ctxA := ownerCtx(handleTestOwner)
	ctxB := ownerCtx(handleOtherOwner)
	first, err := f.s.service.BeginChallenge(ctxA, &realmpb.BeginChallengeRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.service.BeginChallenge(ctxA, &realmpb.BeginChallengeRequest{}); status.Code(err) != codes.ResourceExhausted {
		t.Fatalf("per-owner challenge capacity = %v", err)
	}
	if _, err := f.s.service.BeginChallenge(ctxB, &realmpb.BeginChallengeRequest{}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.service.BeginChallenge(ctxB, &realmpb.BeginChallengeRequest{}); status.Code(err) != codes.ResourceExhausted {
		t.Fatalf("total challenge capacity = %v", err)
	}
	f.clock.advance(10 * time.Second)
	// Expired challenges free the bounded space without any grace period.
	after, err := f.s.service.BeginChallenge(ctxA, &realmpb.BeginChallengeRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if after.GetChallenge() == nil || string(after.GetChallenge()) == string(first.GetChallenge()) {
		t.Fatalf("challenge after expiry = %v", after)
	}
}

// barrierLiveness holds every concurrent online check at a barrier so tests
// can observe what happens while the network wait is in flight; it reports
// each arrival before blocking.
type barrierLiveness struct {
	inner   Liveness
	entered chan struct{}
	gate    chan struct{}
}

func (b barrierLiveness) Check(ctx context.Context, payload, signature []byte) error {
	b.entered <- struct{}{}
	<-b.gate
	return b.inner.Check(ctx, payload, signature)
}

// Two requests carrying the same challenge and a valid proof reach the online
// liveness wait at the same time: exactly one identity may be established,
// and the loser must be refused, not silently granted a second handle.
func TestEstablishForeignCallerDirectConsumesChallengeExactlyOnce(t *testing.T) {
	foreign := newForeignFixture(t)
	liveness := barrierLiveness{
		inner:   livenessAdapter{issuer: foreign.issuer},
		entered: make(chan struct{}),
		gate:    make(chan struct{}),
	}
	f := newDirectFixture(t, func(c *Config) {
		foreign.configure(c)
		c.ForeignLiveness[issuerRealmID] = liveness
	})
	ctx := ownerCtx(handleTestOwner)
	challenge := foreign.challenge(t, f.s, ctx)
	proof := foreign.issue(t, handleTestOwner, challenge, 10*time.Second)

	results := make(chan error, 2)
	for i := 0; i < 2; i++ {
		go func() {
			_, err := f.s.service.EstablishForeignCaller(ctx, foreignRequest(challenge, proof))
			results <- err
		}()
	}
	// Both requests are inside the online issuer wait at the same time.
	<-liveness.entered
	<-liveness.entered
	close(liveness.gate)
	successes, refusals := 0, 0
	for i := 0; i < 2; i++ {
		if err := <-results; err == nil {
			successes++
		} else if status.Code(err) == codes.Unauthenticated {
			refusals++
		} else {
			t.Fatalf("unexpected concurrent result = %v", err)
		}
	}
	if successes != 1 || refusals != 1 {
		t.Fatalf("concurrent identical challenges: %d established, %d refused", successes, refusals)
	}
	if f.s.admission.Size() != 1 {
		t.Fatalf("handles after concurrent use = %d", f.s.admission.Size())
	}
}

// A challenge valid at arrival may expire while the request waits for the
// issuer's online check: the wait must re-check the challenge's own expiry
// with a fresh clock, not only the proof's.
func TestEstablishForeignCallerDirectRechecksChallengeExpiryAfterLivenessWait(t *testing.T) {
	foreign := newForeignFixture(t)
	liveness := barrierLiveness{
		inner:   livenessAdapter{issuer: foreign.issuer},
		entered: make(chan struct{}),
		gate:    make(chan struct{}),
	}
	f := newDirectFixture(t, func(c *Config) {
		c.ChallengeTTL = 2 * time.Second
		foreign.configure(c)
		c.ForeignLiveness[issuerRealmID] = liveness
	})
	ctx := ownerCtx(handleTestOwner)
	challenge := foreign.challenge(t, f.s, ctx)
	proof := foreign.issue(t, handleTestOwner, challenge, 10*time.Second)

	results := make(chan error, 1)
	go func() {
		_, err := f.s.service.EstablishForeignCaller(ctx, foreignRequest(challenge, proof))
		results <- err
	}()
	<-liveness.entered
	// The challenge expires during the wait while the proof stays valid.
	f.clock.advance(3 * time.Second)
	close(liveness.gate)
	if err := <-results; status.Code(err) != codes.Unauthenticated {
		t.Fatalf("challenge spent after expiring during liveness wait: %v", err)
	}
	if f.s.admission.Size() != 0 {
		t.Fatalf("handles after expired challenge = %d", f.s.admission.Size())
	}
}

// livenessFunc adapts one function to the Liveness port for tests.
type livenessFunc func(context.Context, []byte, []byte) error

func (f livenessFunc) Check(ctx context.Context, payload, signature []byte) error {
	return f(ctx, payload, signature)
}

// A liveness failure is a transport outcome, not a challenge consumption:
// the same still-valid challenge and proof may be retried within their
// validity once the issuer is reachable again.
func TestEstablishForeignCallerDirectAllowsRetryAfterLivenessFailure(t *testing.T) {
	foreign := newForeignFixture(t)
	var calls atomic.Int32
	flaky := livenessFunc(func(ctx context.Context, payload, signature []byte) error {
		if calls.Add(1) == 1 {
			return errors.New("issuer temporarily unreachable")
		}
		return livenessAdapter{issuer: foreign.issuer}.Check(ctx, payload, signature)
	})
	f := newDirectFixture(t, func(c *Config) {
		foreign.configure(c)
		c.ForeignLiveness[issuerRealmID] = flaky
	})
	ctx := ownerCtx(handleTestOwner)
	challenge := foreign.challenge(t, f.s, ctx)
	proof := foreign.issue(t, handleTestOwner, challenge, 10*time.Second)
	if _, err := f.s.service.EstablishForeignCaller(ctx, foreignRequest(challenge, proof)); status.Code(err) != codes.Unavailable {
		t.Fatalf("unreachable issuer = %v", err)
	}
	foreign.present(t, f.s, ctx, challenge, proof)
	if f.s.admission.Size() != 1 {
		t.Fatalf("handles after retry = %d", f.s.admission.Size())
	}
	// The spent challenge is refused on a third attempt.
	if _, err := f.s.service.EstablishForeignCaller(ctx, foreignRequest(challenge, proof)); status.Code(err) != codes.Unauthenticated {
		t.Fatalf("replayed challenge = %v", err)
	}
}
