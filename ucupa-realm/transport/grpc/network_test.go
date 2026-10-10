package realmgrpc

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"math/big"
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
	realm "ucupa/ucupa-realm/contract"
	realminstance "ucupa/ucupa-realm/instance"
)

// networkCA issues short-lived test certificates. Server leaves carry the
// localhost and loopback names the pinned dial verifies; client leaves are the
// admitted service identities.
type networkCA struct {
	certificate *x509.Certificate
	key         ed25519.PrivateKey
}

func newNetworkCA(t *testing.T, commonName string) *networkCA {
	t.Helper()
	pub, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	ca := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: commonName}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour), IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature}
	der, err := x509.CreateCertificate(rand.Reader, ca, ca, pub, key)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	return &networkCA{certificate: parsed, key: key}
}

func (c *networkCA) issue(t *testing.T, commonName string, server bool) (tls.Certificate, [32]byte) {
	t.Helper()
	pub, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	leaf := &x509.Certificate{SerialNumber: big.NewInt(2), Subject: pkix.Name{CommonName: commonName}, NotBefore: c.certificate.NotBefore, NotAfter: c.certificate.NotAfter, KeyUsage: x509.KeyUsageDigitalSignature}
	if server {
		leaf.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}
		leaf.DNSNames = []string{"localhost"}
		leaf.IPAddresses = []net.IP{net.ParseIP("127.0.0.1")}
	} else {
		leaf.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}
	}
	der, err := x509.CreateCertificate(rand.Reader, leaf, c.certificate, pub, c.key)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	cert, err := tls.X509KeyPair(
		pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}),
		pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER}),
	)
	if err != nil {
		t.Fatal(err)
	}
	return cert, sha256.Sum256(parsed.RawSubjectPublicKeyInfo)
}

func (c *networkCA) pool() *x509.CertPool {
	pool := x509.NewCertPool()
	pool.AddCert(c.certificate)
	return pool
}

// networkFixture serves one real Realm service over TLS on the loopback
// interface with two admitted service owners and one unbound client key.
type networkFixture struct {
	s                       *Server
	kernel                  *realminstance.Realm
	admin                   values.PrincipalRef
	userCred                realm.CredentialEvidence
	ca, clientCA            *networkCA
	serverSpki              [32]byte
	ownerA, ownerB, unbound tls.Certificate
	ownerASpki, ownerBSpki  [32]byte
	addr                    string
}

func newNetworkFixture(t *testing.T, mutate func(*Config)) *networkFixture {
	t.Helper()
	f := &networkFixture{
		kernel:   nil,
		admin:    values.PrincipalRef{Realm: "realm-a", Principal: "admin"},
		ca:       newNetworkCA(t, "UCUPA realm test CA"),
		clientCA: newNetworkCA(t, "UCUPA realm client CA"),
	}
	f.userCred = randomCredential(t)
	kernel, err := realminstance.New(newDirectSeed(f.userCred, randomCredential(t)))
	if err != nil {
		t.Fatal(err)
	}
	f.kernel = kernel
	serverCert, serverSpki := f.ca.issue(t, "realm-a", true)
	f.serverSpki = serverSpki
	f.ownerA, f.ownerASpki = f.clientCA.issue(t, "owner-a", false)
	f.ownerB, f.ownerBSpki = f.clientCA.issue(t, "owner-b", false)
	f.unbound, _ = f.clientCA.issue(t, "unbound", false)
	config := baseTestConfig()
	config.Certificate = serverCert
	config.ClientRoots = f.clientCA.pool()
	config.AdmittedServices = map[[32]byte]values.FactAuthorityRef{
		f.ownerASpki: handleTestOwner,
		f.ownerBSpki: handleOtherOwner,
	}
	if mutate != nil {
		mutate(&config)
	}
	server, err := New(kernel, config)
	if err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	f.addr = listener.Addr().String()
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
	f.s = server
	return f
}

func (f *networkFixture) clientTLSConfig(t *testing.T, cert *tls.Certificate, pin *[32]byte) *tls.Config {
	t.Helper()
	config := &tls.Config{
		MinVersion: tls.VersionTLS13,
		NextProtos: []string{"h2"},
		RootCAs:    f.ca.pool(),
		ServerName: "127.0.0.1",
	}
	if cert != nil {
		config.GetClientCertificate = func(*tls.CertificateRequestInfo) (*tls.Certificate, error) { return cert, nil }
	}
	if pin != nil {
		pinned := *pin
		config.VerifyConnection = func(state tls.ConnectionState) error {
			if len(state.PeerCertificates) == 0 || sha256.Sum256(state.PeerCertificates[0].RawSubjectPublicKeyInfo) != pinned {
				return errors.New("server key not pinned")
			}
			return nil
		}
	}
	return config
}

func (f *networkFixture) dial(t *testing.T, cert *tls.Certificate, pin *[32]byte) *grpc.ClientConn {
	t.Helper()
	conn, err := grpc.NewClient("dns:///"+f.addr, grpc.WithTransportCredentials(credentials.NewTLS(f.clientTLSConfig(t, cert, pin))))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

func networkTargetCtx() context.Context {
	return metadata.AppendToOutgoingContext(context.Background(), "ucupa-realm-target", "realm-a")
}

func (f *networkFixture) establish(t *testing.T, conn *grpc.ClientConn) []byte {
	t.Helper()
	out, err := realmpb.NewRealmIdentityServiceClient(conn).EstablishCaller(networkTargetCtx(), &realmpb.EstablishCallerRequest{
		Audience:     proto.String("realm-a"),
		CredentialId: f.userCred.ID[:],
		Secret:       f.userCred.Secret[:],
	})
	if err != nil {
		t.Fatal(err)
	}
	return out.GetCaller()
}

func (f *networkFixture) requirement() *realmpb.Requirement {
	return coreElementRequirement(string(directTestPermission))
}

// gatedLiveness blocks the online issuer check until released, signalling its
// arrival first: concurrency and deadline behavior stay deterministic.
type gatedLiveness struct {
	issuer  *realminstance.IdentityIssuer
	gate    chan struct{}
	entered chan struct{}
}

func (g gatedLiveness) Check(ctx context.Context, payload, signature []byte) error {
	select {
	case g.entered <- struct{}{}:
	case <-ctx.Done():
		return ctx.Err()
	}
	select {
	case <-g.gate:
	case <-ctx.Done():
		return ctx.Err()
	}
	return g.issuer.Check(ctx, realm.IdentityProof{Payload: payload, Signature: signature})
}

func TestNetworkFullChainOverTLS(t *testing.T) {
	f := newNetworkFixture(t, nil)
	conn := f.dial(t, &f.ownerA, &f.serverSpki)

	// The read-only target check confirms the actual Realm identity.
	target, err := realmpb.NewRealmTargetServiceClient(conn).CheckTarget(networkTargetCtx(), &realmpb.CheckTargetRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if target.GetRealmId() != "realm-a" {
		t.Fatalf("target = %v", target)
	}

	identity := realmpb.NewRealmIdentityServiceClient(conn)
	authorization := realmpb.NewRealmAuthorizationServiceClient(conn)
	caller := f.establish(t, conn)

	authorized, err := authorization.AuthorizeView(networkTargetCtx(), authorizeRequest(caller, f.requirement()))
	if err != nil {
		t.Fatal(err)
	}
	if authorized.GetDecision() != realmpb.Decision_VALID || len(authorized.GetView()) != 32 {
		t.Fatalf("authorize = %v", authorized)
	}

	validated, err := authorization.ValidateView(networkTargetCtx(), &realmpb.ValidateViewRequest{
		Caller: caller, View: authorized.GetView(), Requirements: []*realmpb.Requirement{f.requirement()},
	})
	if err != nil {
		t.Fatal(err)
	}
	if validated.GetDecision() != realmpb.Decision_VALID {
		t.Fatalf("validate = %v", validated)
	}

	// A closed branch is proven with a denial view, never reported as Allow.
	captured, err := authorization.CaptureDenialView(networkTargetCtx(), &realmpb.CaptureDenialViewRequest{
		Caller: caller, Requirement: coreElementRequirement("ucupa.test.write"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if captured.GetDecision() != realmpb.Decision_DENIED || len(captured.GetDenial()) != 32 {
		t.Fatalf("denial capture = %v", captured)
	}
	denied, err := authorization.ValidateDenialView(networkTargetCtx(), &realmpb.ValidateDenialViewRequest{
		Caller: caller, Denial: captured.GetDenial(), Requirement: coreElementRequirement("ucupa.test.write"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if denied.GetDecision() != realmpb.Decision_VALID {
		t.Fatalf("denial validation = %v", denied)
	}

	// Release ends the run-local reference and every later use fails closed.
	if _, err := identity.ReleaseHandle(networkTargetCtx(), &realmpb.ReleaseHandleRequest{Handle: caller}); err != nil {
		t.Fatal(err)
	}
	if _, err := authorization.ValidateView(networkTargetCtx(), &realmpb.ValidateViewRequest{
		Caller: caller, View: authorized.GetView(), Requirements: []*realmpb.Requirement{f.requirement()},
	}); status.Code(err) != codes.Unauthenticated {
		t.Fatalf("released view = %v", err)
	}
}

func TestNetworkWrongTargetOrAudienceRefusesEstablishment(t *testing.T) {
	f := newNetworkFixture(t, nil)
	conn := f.dial(t, &f.ownerA, &f.serverSpki)
	identity := realmpb.NewRealmIdentityServiceClient(conn)
	request := &realmpb.EstablishCallerRequest{
		Audience:     proto.String("realm-a"),
		CredentialId: f.userCred.ID[:],
		Secret:       f.userCred.Secret[:],
	}

	// A request aimed at another Realm is refused before any handler runs.
	wrongTarget := metadata.AppendToOutgoingContext(context.Background(), "ucupa-realm-target", "other-realm")
	if _, err := identity.EstablishCaller(wrongTarget, request); status.Code(err) != codes.Unavailable {
		t.Fatalf("wrong target = %v", err)
	}
	// A request without the target metadata is refused before any handler.
	if _, err := identity.EstablishCaller(context.Background(), request); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("missing target = %v", err)
	}
	// An audience mismatch is refused before the kernel examines the evidence.
	foreignAudience := &realmpb.EstablishCallerRequest{
		Audience:     proto.String("other-realm"),
		CredentialId: f.userCred.ID[:],
		Secret:       f.userCred.Secret[:],
	}
	if _, err := identity.EstablishCaller(networkTargetCtx(), foreignAudience); status.Code(err) != codes.Unauthenticated {
		t.Fatalf("wrong audience = %v", err)
	}
	// Refused requests never issue a caller handle. Whether a raw connection
	// carried request bytes is a transport property; the SDK-level proof that
	// credentials are not delivered to a wrong target lives in the SDK tests.
	if f.s.admission.Size() != 0 {
		t.Fatalf("refused calls issued handles: %d handles", f.s.admission.Size())
	}
}

func TestNetworkAdmissionRequiresBoundService(t *testing.T) {
	f := newNetworkFixture(t, nil)
	// A verified client key without a service binding cannot act for any owner.
	unbound := f.dial(t, &f.unbound, &f.serverSpki)
	_, err := realmpb.NewRealmIdentityServiceClient(unbound).EstablishCaller(networkTargetCtx(), &realmpb.EstablishCallerRequest{
		Audience: proto.String("realm-a"), CredentialId: f.userCred.ID[:], Secret: f.userCred.Secret[:],
	})
	if status.Code(err) != codes.PermissionDenied {
		t.Fatalf("unbound service = %v", err)
	}
	// A connection without any client certificate is refused the same way.
	anonymous := f.dial(t, nil, &f.serverSpki)
	_, err = realmpb.NewRealmIdentityServiceClient(anonymous).EstablishCaller(networkTargetCtx(), &realmpb.EstablishCallerRequest{
		Audience: proto.String("realm-a"), CredentialId: f.userCred.ID[:], Secret: f.userCred.Secret[:],
	})
	if status.Code(err) != codes.PermissionDenied {
		t.Fatalf("anonymous connection = %v", err)
	}
	if f.s.admission.Size() != 0 {
		t.Fatalf("unadmitted calls issued handles: %d", f.s.admission.Size())
	}
	// The read-only target check stays reachable without service admission.
	target, err := realmpb.NewRealmTargetServiceClient(anonymous).CheckTarget(networkTargetCtx(), &realmpb.CheckTargetRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if target.GetRealmId() != "realm-a" {
		t.Fatalf("target = %v", target)
	}
}

func TestNetworkPinnedSPKIAndHostnameFailClosed(t *testing.T) {
	f := newNetworkFixture(t, nil)
	request := &realmpb.EstablishCallerRequest{
		Audience: proto.String("realm-a"), CredentialId: f.userCred.ID[:], Secret: f.userCred.Secret[:],
	}
	// A pin on any other key rejects the handshake before credentials leave.
	wrongPin := f.ownerBSpki
	pinned := f.dial(t, &f.ownerA, &wrongPin)
	if _, err := realmpb.NewRealmIdentityServiceClient(pinned).EstablishCaller(networkTargetCtx(), request); err == nil {
		t.Fatal("wrong server key accepted")
	}
	// An untrusted certification authority is refused by the standard chain.
	untrusted := f.clientTLSConfig(t, &f.ownerA, nil)
	untrusted.RootCAs = f.clientCA.pool()
	conn, err := grpc.NewClient("dns:///"+f.addr, grpc.WithTransportCredentials(credentials.NewTLS(untrusted)))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	if _, err := realmpb.NewRealmIdentityServiceClient(conn).EstablishCaller(networkTargetCtx(), request); err == nil {
		t.Fatal("untrusted server chain accepted")
	}
	// A wrong hostname fails the same standard verification.
	hostname := f.clientTLSConfig(t, &f.ownerA, nil)
	hostname.ServerName = "wrong.invalid"
	named, err := grpc.NewClient("dns:///"+f.addr, grpc.WithTransportCredentials(credentials.NewTLS(hostname)))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = named.Close() })
	if _, err := realmpb.NewRealmIdentityServiceClient(named).EstablishCaller(networkTargetCtx(), request); err == nil {
		t.Fatal("wrong hostname accepted")
	}
	if f.s.admission.Size() != 0 {
		t.Fatalf("failed handshakes issued handles: %d", f.s.admission.Size())
	}
}

func TestNetworkHandlesAreRunLocal(t *testing.T) {
	f := newNetworkFixture(t, nil)
	conn := f.dial(t, &f.ownerA, &f.serverSpki)
	caller := f.establish(t, conn)
	authorized, err := realmpb.NewRealmAuthorizationServiceClient(conn).AuthorizeView(networkTargetCtx(), authorizeRequest(caller, f.requirement()))
	if err != nil {
		t.Fatal(err)
	}
	view := authorized.GetView()

	// A transient disconnect does not revoke still-valid run-local evidence:
	// the same handles validate on a fresh connection of the same owner.
	_ = conn.Close()
	again := f.dial(t, &f.ownerA, &f.serverSpki)
	validated, err := realmpb.NewRealmAuthorizationServiceClient(again).ValidateView(networkTargetCtx(), &realmpb.ValidateViewRequest{
		Caller: caller, View: view, Requirements: []*realmpb.Requirement{f.requirement()},
	})
	if err != nil {
		t.Fatal(err)
	}
	if validated.GetDecision() != realmpb.Decision_VALID {
		t.Fatalf("reconnected validation = %v", validated)
	}

	// Another admitted service cannot present the first service's references.
	otherOwner := f.dial(t, &f.ownerB, &f.serverSpki)
	if _, err := realmpb.NewRealmAuthorizationServiceClient(otherOwner).AuthorizeView(networkTargetCtx(), authorizeRequest(caller, f.requirement())); status.Code(err) != codes.Unauthenticated {
		t.Fatalf("cross-owner handle = %v", err)
	}

	// Restarting the service drops every run-local reference: the kernel keeps
	// its identity, but nothing issued by the previous run survives.
	f.s.Stop()
	restarted, err := New(f.kernel, f.s.config)
	if err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	served := make(chan error, 1)
	go func() { served <- restarted.Serve(listener) }()
	t.Cleanup(func() {
		restarted.Stop()
		select {
		case err := <-served:
			if err != nil {
				t.Errorf("restarted Serve returned %v", err)
			}
		case <-time.After(5 * time.Second):
			t.Error("restarted Serve did not return after Stop")
		}
	})
	fresh := &networkFixture{s: restarted, kernel: f.kernel, userCred: f.userCred, addr: listener.Addr().String(), ca: f.ca, clientCA: f.clientCA, ownerA: f.ownerA, ownerB: f.ownerB, ownerASpki: f.ownerASpki}
	postRestart := fresh.dial(t, &f.ownerA, &f.serverSpki)
	if _, err := realmpb.NewRealmAuthorizationServiceClient(postRestart).AuthorizeView(networkTargetCtx(), authorizeRequest(caller, f.requirement())); status.Code(err) != codes.Unauthenticated {
		t.Fatalf("pre-restart handle = %v", err)
	}
	// A fresh establishment on the restarted run works normally.
	renewed := fresh.establish(t, postRestart)
	reauthorized, err := realmpb.NewRealmAuthorizationServiceClient(postRestart).AuthorizeView(networkTargetCtx(), authorizeRequest(renewed, f.requirement()))
	if err != nil {
		t.Fatal(err)
	}
	if reauthorized.GetDecision() != realmpb.Decision_VALID {
		t.Fatalf("post-restart authorization = %v", reauthorized)
	}
}

func TestNetworkDecisionsStaySeparateOverWire(t *testing.T) {
	f := newNetworkFixture(t, nil)
	conn := f.dial(t, &f.ownerA, &f.serverSpki)
	authorization := realmpb.NewRealmAuthorizationServiceClient(conn)
	caller := f.establish(t, conn)

	authorized, err := authorization.AuthorizeView(networkTargetCtx(), authorizeRequest(caller, f.requirement()))
	if err != nil {
		t.Fatal(err)
	}
	if authorized.GetDecision() != realmpb.Decision_VALID || authorized.GetView() == nil {
		t.Fatalf("valid decision = %v", authorized)
	}
	// A missing permission is DENIED with no view, not an error.
	denied, err := authorization.AuthorizeView(networkTargetCtx(), authorizeRequest(caller, coreElementRequirement("ucupa.test.write")))
	if err != nil {
		t.Fatal(err)
	}
	if denied.GetDecision() != realmpb.Decision_DENIED || denied.GetView() != nil {
		t.Fatalf("denied decision = %v", denied)
	}

	// Revocation turns the next online validation into DENIED.
	binding := realm.PermissionBinding{
		Subject:    values.PrincipalRef{Realm: "realm-a", Principal: "user"},
		Permission: directTestPermission,
		Scope:      values.CoreElementScope(requirementsTestOwner, requirementsTestElement),
	}
	if _, err := f.kernel.DeleteBinding(f.admin, binding, f.kernel.BindingSetRevision(), "op-revoke"); err != nil {
		t.Fatal(err)
	}
	revoked, err := authorization.ValidateView(networkTargetCtx(), &realmpb.ValidateViewRequest{
		Caller: caller, View: authorized.GetView(), Requirements: []*realmpb.Requirement{f.requirement()},
	})
	if err != nil {
		t.Fatal(err)
	}
	if revoked.GetDecision() != realmpb.Decision_DENIED {
		t.Fatalf("revoked validation = %v", revoked)
	}

	// A revoke/regrant never revives the old view: it reports CHANGED.
	if _, err := f.kernel.PutBinding(f.admin, binding, f.kernel.BindingSetRevision(), "op-regrant"); err != nil {
		t.Fatal(err)
	}
	after, err := authorization.ValidateView(networkTargetCtx(), &realmpb.ValidateViewRequest{
		Caller: caller, View: authorized.GetView(), Requirements: []*realmpb.Requirement{f.requirement()},
	})
	if err != nil {
		t.Fatal(err)
	}
	if after.GetDecision() != realmpb.Decision_CHANGED {
		t.Fatalf("regranted validation = %v", after)
	}
}

func TestNetworkForeignIdentityChainOverWire(t *testing.T) {
	foreign := newForeignFixture(t)
	f := newNetworkFixture(t, foreign.configure)
	conn := f.dial(t, &f.ownerA, &f.serverSpki)
	identity := realmpb.NewRealmIdentityServiceClient(conn)

	challenge, err := identity.BeginChallenge(networkTargetCtx(), &realmpb.BeginChallengeRequest{})
	if err != nil {
		t.Fatal(err)
	}
	proof := foreign.issue(t, handleTestOwner, challenge.GetChallenge(), 10*time.Second)
	established, err := identity.EstablishForeignCaller(networkTargetCtx(), &realmpb.EstablishForeignCallerRequest{
		Actor: proto.String(string(handleTestOwner)), Payload: proof.Payload, Signature: proof.Signature, Challenge: challenge.GetChallenge(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if established.GetSubjectRealm() != "issuer-realm" || established.GetSubjectPrincipal() != "subject-x" {
		t.Fatalf("foreign subject = %v", established)
	}
	authorized, err := realmpb.NewRealmAuthorizationServiceClient(conn).AuthorizeView(networkTargetCtx(), authorizeRequest(established.GetCaller(), f.requirement()))
	if err != nil {
		t.Fatal(err)
	}
	if authorized.GetDecision() != realmpb.Decision_VALID {
		t.Fatalf("foreign authorization = %v", authorized)
	}
}

func TestNetworkDeadlineBoundsBlockedCalls(t *testing.T) {
	foreign := newForeignFixture(t)
	gate := make(chan struct{})
	t.Cleanup(func() { close(gate) })
	f := newNetworkFixture(t, func(c *Config) {
		foreign.configure(c)
		c.ForeignLiveness[issuerRealmID] = gatedLiveness{issuer: foreign.issuer, gate: gate, entered: make(chan struct{}, 1)}
	})
	conn := f.dial(t, &f.ownerA, &f.serverSpki)
	identity := realmpb.NewRealmIdentityServiceClient(conn)
	challenge, err := identity.BeginChallenge(networkTargetCtx(), &realmpb.BeginChallengeRequest{})
	if err != nil {
		t.Fatal(err)
	}
	proof := foreign.issue(t, handleTestOwner, challenge.GetChallenge(), 10*time.Second)
	ctx, cancel := context.WithTimeout(networkTargetCtx(), 100*time.Millisecond)
	defer cancel()
	if _, err := identity.EstablishForeignCaller(ctx, &realmpb.EstablishForeignCallerRequest{
		Actor: proto.String(string(handleTestOwner)), Payload: proof.Payload, Signature: proof.Signature, Challenge: challenge.GetChallenge(),
	}); status.Code(err) != codes.DeadlineExceeded {
		t.Fatalf("deadline = %v", err)
	}
	// The aborted establishment left no caller handle behind.
	if f.s.admission.Size() != 0 {
		t.Fatalf("handles after deadline = %d", f.s.admission.Size())
	}
}

func TestNetworkConcurrentCapacityIsBounded(t *testing.T) {
	foreign := newForeignFixture(t)
	gate := make(chan struct{})
	entered := make(chan struct{}, 1)
	t.Cleanup(func() {
		select {
		case <-gate:
		default:
			close(gate)
		}
	})
	f := newNetworkFixture(t, func(c *Config) {
		c.MaxConcurrentRPCs = 1
		foreign.configure(c)
		c.ForeignLiveness[issuerRealmID] = gatedLiveness{issuer: foreign.issuer, gate: gate, entered: entered}
	})
	conn := f.dial(t, &f.ownerA, &f.serverSpki)
	identity := realmpb.NewRealmIdentityServiceClient(conn)
	challenge, err := identity.BeginChallenge(networkTargetCtx(), &realmpb.BeginChallengeRequest{})
	if err != nil {
		t.Fatal(err)
	}
	proof := foreign.issue(t, handleTestOwner, challenge.GetChallenge(), 10*time.Second)
	request := &realmpb.EstablishForeignCallerRequest{
		Actor: proto.String(string(handleTestOwner)), Payload: proof.Payload, Signature: proof.Signature, Challenge: challenge.GetChallenge(),
	}
	done := make(chan error, 1)
	go func() {
		_, err := identity.EstablishForeignCaller(networkTargetCtx(), request)
		done <- err
	}()
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("first call never reached the issuer check")
	}
	// The one configured concurrent slot is taken: the second call is refused
	// without waiting and without reaching the issuer.
	if _, err := identity.EstablishForeignCaller(networkTargetCtx(), request); status.Code(err) != codes.ResourceExhausted {
		t.Fatalf("second concurrent call = %v", err)
	}
	close(gate)
	if err := <-done; err != nil {
		t.Fatalf("first call = %v", err)
	}
	if f.s.admission.Size() != 1 {
		t.Fatalf("handles after bounded concurrency = %d", f.s.admission.Size())
	}
}
