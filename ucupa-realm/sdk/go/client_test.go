package realmsdk

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"net"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	"ucupa/shared/values"
	realmpb "ucupa/ucupa-realm/bindings/go/protobuf"
)

func TestNewRejectsInvalidConfiguration(t *testing.T) {
	base := Config{
		NextHop:          "https://127.0.0.1:1",
		Expected:         "realm-a",
		Roots:            x509.NewCertPool(),
		SPKISHA256:       [32]byte{1},
		RPCDeadline:      time.Second,
		ConnectTimeout:   time.Second,
		MaxResponseBytes: 4096,
		MaxMetadataBytes: 8192,
	}
	cases := []struct {
		name   string
		mutate func(*Config)
	}{
		{"missing next hop", func(c *Config) { c.NextHop = "" }},
		{"local next hop", func(c *Config) { c.NextHop = "local:realm" }},
		{"missing realm", func(c *Config) { c.Expected = "" }},
		{"missing roots", func(c *Config) { c.Roots = nil }},
		{"zero spki", func(c *Config) { c.SPKISHA256 = [32]byte{} }},
		{"zero rpc deadline", func(c *Config) { c.RPCDeadline = 0 }},
		{"zero connect timeout", func(c *Config) { c.ConnectTimeout = 0 }},
		{"zero max response", func(c *Config) { c.MaxResponseBytes = 0 }},
		{"metadata below floor", func(c *Config) { c.MaxMetadataBytes = 255 }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			config := base
			tc.mutate(&config)
			client, err := New(config)
			if err == nil {
				client.Close()
				t.Fatal("invalid configuration accepted")
			}
		})
	}
}

func TestClientTypedChainOverPinnedTLS(t *testing.T) {
	service := startFakeRealm(t)
	client, err := New(service.dialConfig())
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	if hop, id := client.Target(); hop != values.NextHop("https://"+service.addr) || id != "realm-a" {
		t.Fatalf("target = %v/%v", hop, id)
	}

	// The read-only target check confirms the Realm before credentials flow,
	// presents the service identity and pins the expected Realm metadata.
	realmID, err := client.CheckTarget(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if realmID != "realm-a" {
		t.Fatalf("realm = %v", realmID)
	}
	fake := service.fake
	if len(fake.targetMetadata) != 1 || fake.targetMetadata[0] != "realm-a" {
		t.Fatalf("target metadata = %v", fake.targetMetadata)
	}
	if fake.clientIdentity != "admitted" {
		t.Fatalf("presented client identity = %q", fake.clientIdentity)
	}

	// The credential bytes cross the wire exactly once, unmodified.
	credentialID, secret := sdkCredential()
	established, err := client.IdentityService().EstablishCaller(context.Background(), &realmpb.EstablishCallerRequest{
		Audience:     proto.String("realm-a"),
		CredentialId: credentialID[:],
		Secret:       secret[:],
	})
	if err != nil {
		t.Fatal(err)
	}
	if established.GetSubjectPrincipal() != "user" || established.GetExpiresInSeconds() != 60 {
		t.Fatalf("established = %v", established)
	}
	if fake.audience != "realm-a" || !bytes.Equal(fake.credentialID, credentialID[:]) || !bytes.Equal(fake.secret, secret[:]) {
		t.Fatal("credential evidence was not delivered exactly as issued")
	}

	// The view authorization carries the same caller and requirement set.
	authorized, err := client.AuthorizationService().AuthorizeView(context.Background(), &realmpb.AuthorizeViewRequest{
		Caller:       established.GetCaller(),
		Requirements: []*realmpb.Requirement{sdkRequirement()},
	})
	if err != nil {
		t.Fatal(err)
	}
	if authorized.GetDecision() != realmpb.Decision_VALID || len(authorized.GetView()) != 32 {
		t.Fatalf("authorized = %v", authorized)
	}
	if !bytes.Equal(fake.authorized, established.GetCaller()) {
		t.Fatal("authorized caller differs from the established caller")
	}
	if len(fake.requirements) != 1 ||
		fake.requirements[0].GetPermission() != "ucupa.test.read" ||
		fake.requirements[0].GetScope().GetKind() != realmpb.ScopeKind_CORE_ELEMENT ||
		fake.requirements[0].GetScope().GetAuthority() != string(sdkTestOwner) ||
		fake.requirements[0].GetScope().GetElement() != string(sdkTestElement) {
		t.Fatalf("authorized requirements = %v", fake.requirements)
	}

	// Validation reuses the exact view, and release returns the exact handle.
	validated, err := client.AuthorizationService().ValidateView(context.Background(), &realmpb.ValidateViewRequest{
		Caller: established.GetCaller(), View: authorized.GetView(), Requirements: []*realmpb.Requirement{sdkRequirement()},
	})
	if err != nil {
		t.Fatal(err)
	}
	if validated.GetDecision() != realmpb.Decision_VALID {
		t.Fatalf("validated = %v", validated)
	}
	if !bytes.Equal(fake.validatedView, authorized.GetView()) {
		t.Fatal("validated view differs from the authorized view")
	}
	if _, err := client.IdentityService().ReleaseHandle(context.Background(), &realmpb.ReleaseHandleRequest{Handle: established.GetCaller()}); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(fake.released, established.GetCaller()) {
		t.Fatal("released handle differs from the established caller")
	}
}

func TestClientPinsTheServerKey(t *testing.T) {
	service := startFakeRealm(t)
	config := service.dialConfig()
	config.SPKISHA256 = [32]byte{1}
	client, err := New(config)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	if _, err := client.CheckTarget(context.Background()); err == nil {
		t.Fatal("unpinned server key accepted")
	}
}

func TestClientBoundsDeadlinesAndCallerMetadata(t *testing.T) {
	service := startFakeRealm(t)
	client, err := New(service.dialConfig())
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	// An expired deadline is reported as such, not as a Realm decision.
	ctx, cancel := context.WithTimeout(context.Background(), time.Nanosecond)
	defer cancel()
	time.Sleep(time.Millisecond)
	if _, err := client.CheckTarget(ctx); status.Code(err) != codes.DeadlineExceeded {
		t.Fatalf("deadline = %v", err)
	}
	// Cancellation propagates to the typed call the same way.
	canceled, stop := context.WithCancel(context.Background())
	stop()
	if _, err := client.CheckTarget(canceled); status.Code(err) != codes.Canceled {
		t.Fatalf("canceled = %v", err)
	}
	// Caller-supplied metadata is refused before anything is sent.
	decorated := metadata.AppendToOutgoingContext(context.Background(), "x-trace", "1")
	if _, err := client.CheckTarget(decorated); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("caller metadata = %v", err)
	}
}

// Protected methods may not carry credential or authorization payloads until
// the actual Realm behind the pinned address has been verified, and a failed
// verification attempt is not a pass. Credential safety is proven by the
// receiving fake's request recording, not by handle outcomes.
func TestClientRequiresVerifiedTargetBeforeProtectedCalls(t *testing.T) {
	service := startFakeRealm(t)
	client, err := New(service.dialConfig())
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	id, secret := sdkCredential()
	establish := func() error {
		_, err := client.IdentityService().EstablishCaller(context.Background(), &realmpb.EstablishCallerRequest{
			Audience:     proto.String("realm-a"),
			CredentialId: id[:],
			Secret:       secret[:],
		})
		return err
	}
	if code := status.Code(establish()); code != codes.FailedPrecondition {
		t.Fatalf("unverified target accepted credentials: %v", code)
	}
	if service.fake.secret != nil {
		t.Fatal("credential secret delivered without target verification")
	}
	if _, err := client.AuthorizationService().AuthorizeView(context.Background(), &realmpb.AuthorizeViewRequest{
		Caller:       make([]byte, 32),
		Requirements: []*realmpb.Requirement{sdkRequirement()},
	}); status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("unverified target accepted authorization: %v", err)
	}
	// A canceled verification attempt is not a verification either.
	canceled, stop := context.WithCancel(context.Background())
	stop()
	if _, err := client.CheckTarget(canceled); err == nil {
		t.Fatal("canceled target check succeeded")
	}
	if code := status.Code(establish()); code != codes.FailedPrecondition {
		t.Fatalf("canceled verification accepted credentials: %v", code)
	}
	// Once the target is verified on this connection the same calls succeed
	// and the credential is delivered exactly as issued.
	if _, err := client.CheckTarget(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := establish(); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(service.fake.secret, secret[:]) {
		t.Fatal("credential secret was not delivered after verification")
	}
}

// A target check that identified a different Realm must prevent credential
// delivery on every following call of the same client: the mismatch is a
// hard failure, not a soft warning the caller may ignore.
func TestClientFailedTargetCheckBlocksCredentialDelivery(t *testing.T) {
	service := startFakeRealm(t)
	config := service.dialConfig()
	config.Expected = "realm-b"
	client, err := New(config)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	if _, err := client.CheckTarget(context.Background()); status.Code(err) != codes.Unavailable {
		t.Fatalf("expected actual Realm mismatch, got %v", err)
	}
	id, secret := sdkCredential()
	_, err = client.IdentityService().EstablishCaller(context.Background(), &realmpb.EstablishCallerRequest{
		Audience:     proto.String("realm-b"),
		CredentialId: id[:],
		Secret:       secret[:],
	})
	if status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("credentials accepted after Realm mismatch: %v", err)
	}
	if service.fake.secret != nil {
		t.Fatal("secret delivered to actual realm-a after expected realm-b target mismatch")
	}
}

// Verification is bound to the connection that answered it: a rebuilt
// connection to a different Realm must be re-verified before protected calls,
// even when the replacement presents the same pinned certificate.
func TestClientTargetVerificationBindsToTheCurrentConnection(t *testing.T) {
	service := startFakeRealm(t)
	client, err := New(service.dialConfig())
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	if _, err := client.CheckTarget(context.Background()); err != nil {
		t.Fatal(err)
	}
	id, secret := sdkCredential()
	establish := func() error {
		_, err := client.IdentityService().EstablishCaller(context.Background(), &realmpb.EstablishCallerRequest{
			Audience:     proto.String("realm-a"),
			CredentialId: id[:],
			Secret:       secret[:],
		})
		return err
	}
	// A poll is bounded and deterministic in outcome: each round either fails
	// on the broken transport or is answered by the currently serving Realm.
	reach := func(done func(error) bool) {
		deadline := time.Now().Add(15 * time.Second)
		for {
			_, err := client.CheckTarget(context.Background())
			if done(err) {
				return
			}
			if time.Now().After(deadline) {
				t.Fatalf("restarted Realm not reached: %v", err)
			}
			time.Sleep(10 * time.Millisecond)
		}
	}

	// Replace the Realm behind the same address and certificate. The rebuilt
	// connection must answer with the mismatch before the test proceeds.
	replacement := service.restart(t, "realm-b")
	reach(func(err error) bool {
		return err != nil && status.Convert(err).Message() == "Realm identity mismatch"
	})
	if code := status.Code(establish()); code != codes.FailedPrecondition {
		t.Fatalf("stale verification accepted credentials after rebuild: %v", code)
	}
	if replacement.secret != nil {
		t.Fatal("credential secret delivered to the replacement Realm")
	}

	// Restoring the expected Realm and re-verifying re-enables the calls.
	restored := service.restart(t, "realm-a")
	reach(func(err error) bool { return err == nil })
	if err := establish(); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(restored.secret, secret[:]) {
		t.Fatal("credential secret not delivered after re-verification")
	}
}

// malformedIdentityService answers a success status with a response that lacks
// every required field: the SDK must reject it before a caller sees it. The
// target check itself is well-formed so the test reaches the malformed
// protected response instead of the verification gate.
type malformedIdentityService struct {
	realmpb.UnimplementedRealmTargetServiceServer
	realmpb.UnimplementedRealmIdentityServiceServer
}

func (malformedIdentityService) CheckTarget(context.Context, *realmpb.CheckTargetRequest) (*realmpb.CheckTargetResponse, error) {
	return &realmpb.CheckTargetResponse{RealmId: proto.String("realm-a")}, nil
}

func (malformedIdentityService) EstablishCaller(context.Context, *realmpb.EstablishCallerRequest) (*realmpb.EstablishCallerResponse, error) {
	return &realmpb.EstablishCallerResponse{}, nil
}

func TestClientRejectsMalformedResponses(t *testing.T) {
	serverCA := newSDKCA(t, "UCUPA realm SDK test CA")
	serverCert, spki := serverCA.issue(t, "realm-a", true)
	tlsConfig := &tls.Config{MinVersion: tls.VersionTLS13, MaxVersion: tls.VersionTLS13, Certificates: []tls.Certificate{serverCert}, NextProtos: []string{"h2"}}
	server := grpc.NewServer(grpc.Creds(credentials.NewTLS(tlsConfig)))
	realmpb.RegisterRealmTargetServiceServer(server, malformedIdentityService{})
	realmpb.RegisterRealmIdentityServiceServer(server, malformedIdentityService{})
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	served := make(chan error, 1)
	go func() { served <- server.Serve(listener) }()
	t.Cleanup(func() {
		server.Stop()
		select {
		case err := <-served:
			if err != nil {
				t.Errorf("Serve returned %v", err)
			}
		case <-time.After(5 * time.Second):
			t.Error("Serve did not return after Stop")
		}
	})
	c, err := New(Config{
		NextHop:          values.NextHop("https://" + listener.Addr().String()),
		Expected:         "realm-a",
		Roots:            serverCA.pool(),
		SPKISHA256:       spki,
		RPCDeadline:      5 * time.Second,
		ConnectTimeout:   5 * time.Second,
		MaxResponseBytes: 1 << 20,
		MaxMetadataBytes: 8192,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	// The target check passes so the protected call reaches the malformed
	// response, which must still be rejected before a caller sees it.
	if _, err := c.CheckTarget(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := c.IdentityService().EstablishCaller(context.Background(), &realmpb.EstablishCallerRequest{
		Audience:     proto.String("realm-a"),
		CredentialId: make([]byte, 16),
		Secret:       make([]byte, 32),
	}); status.Code(err) != codes.DataLoss {
		t.Fatalf("malformed response = %v", err)
	}
}

// TestClientProtectedCallRejectsHandshakeRebuiltInsideInvoke proves the exact
// ordering the real transport exposes: the preflight verification passes, and
// only then does gRPC select (and possibly rebuild) the actual connection. A
// replacement handshake completing inside that window must neither carry the
// credentials nor surface its response as verified.
func TestClientProtectedCallRejectsHandshakeRebuiltInsideInvoke(t *testing.T) {
	c := &Client{config: Config{Expected: "realm-a", RPCDeadline: time.Second, MaxResponseBytes: 1 << 20}, handshakeEpoch: 1, verifiedEpoch: 1}
	id, secret := sdkCredential()
	request := &realmpb.EstablishCallerRequest{Audience: proto.String("realm-a"), CredentialId: id[:], Secret: secret[:]}
	response := &realmpb.EstablishCallerResponse{}
	sent := false
	invoke := func(_ context.Context, _ string, in, out any, _ *grpc.ClientConn, opts ...grpc.CallOption) error {
		// A replacement handshake completing inside this window must be
		// refused by the in-flight gate.
		if _, err := c.acceptHandshake(); err == nil {
			t.Error("replacement handshake accepted while a protected call was in flight")
		}
		// If a handshake completed anyway, its response must still never
		// surface as verified.
		c.stateMu.Lock()
		c.handshakeEpoch++
		c.stateMu.Unlock()
		recordReplyTransport(t, opts, 2)
		sent = bytes.Equal(in.(*realmpb.EstablishCallerRequest).GetSecret(), secret[:])
		*out.(*realmpb.EstablishCallerResponse) = realmpb.EstablishCallerResponse{Caller: make([]byte, 32), SubjectRealm: proto.String("realm-b"), SubjectPrincipal: proto.String("user"), ExpiresInSeconds: proto.Uint64(60)}
		return nil
	}
	err := c.intercept(context.Background(), realmpb.RealmIdentityService_EstablishCaller_FullMethodName, request, response, nil, invoke, grpc.StaticMethod())
	if !sent {
		t.Fatal("protected request never reached the sending transport")
	}
	if status.Code(err) != codes.DataLoss {
		t.Fatalf("response from a replacement handshake surfaced as %v", err)
	}
}

// TestClientQueuedProtectedCallNeverLeavesOnReplacementConnection exercises
// the real gRPC/TLS path: the target check passes on connection C1, C1 breaks,
// the same pinned certificate serves a replacement endpoint, and the protected
// request has already passed its preflight when the replacement handshake
// completes. The credential must never travel on the replacement connection.
func TestClientQueuedProtectedCallNeverLeavesOnReplacementConnection(t *testing.T) {
	service := startFakeRealm(t)
	c, err := New(service.dialConfig())
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if _, err := c.CheckTarget(context.Background()); err != nil {
		t.Fatal(err)
	}
	// The verified connection breaks; the client holds no ready transport
	// until the replacement endpoint completes its handshake.
	service.stop()
	listener := rebindAddress(t, service.addr)
	c.conn.ResetConnectBackoff()
	c.conn.Connect()
	raw, err := listener.Accept()
	if err != nil {
		t.Fatal(err)
	}
	replacement := tls.Server(raw, &tls.Config{MinVersion: tls.VersionTLS13, MaxVersion: tls.VersionTLS13, Certificates: []tls.Certificate{service.serverCert}, NextProtos: []string{"h2"}})
	id, secret := sdkCredential()
	result := make(chan error, 1)
	go func() {
		_, err := c.IdentityService().EstablishCaller(context.Background(), &realmpb.EstablishCallerRequest{Audience: proto.String("realm-a"), CredentialId: id[:], Secret: secret[:]})
		result <- err
	}()
	// Wait until the queued protected call is between its preflight and its
	// response before completing the replacement handshake: the reviewer's
	// exact interleave, made deterministic.
	inFlight := func() int {
		c.stateMu.Lock()
		defer c.stateMu.Unlock()
		return c.protectedInFlight
	}
	deadline := time.Now().Add(5 * time.Second)
	for inFlight() != 1 {
		select {
		case err := <-result:
			t.Fatalf("protected call settled before the replacement handshake: %v", err)
		default:
		}
		if time.Now().After(deadline) {
			t.Fatal("protected call never passed its preflight")
		}
		time.Sleep(time.Millisecond)
	}
	handshakeErr := replacement.Handshake()
	var seen []byte
	if handshakeErr == nil {
		// An empty server SETTINGS frame completes the HTTP/2 preface
		// exchange, making the replacement transport ready for the queued
		// call to leave on it.
		if _, err := replacement.Write([]byte{0, 0, 0, 0x4, 0, 0, 0, 0, 0}); err != nil {
			t.Fatal(err)
		}
		seen = readApplicationBytes(t, replacement)
	}
	rpcErr := <-result
	_ = replacement.Close()
	if bytes.Contains(seen, secret[:]) {
		t.Fatal("credential secret left on the unverified replacement connection")
	}
	if rpcErr == nil {
		t.Fatal("protected call succeeded over an unverified replacement connection")
	}
}
