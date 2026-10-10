package realmsdk

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
	"google.golang.org/protobuf/proto"

	"ucupa/shared/values"
	realmpb "ucupa/ucupa-realm/bindings/go/protobuf"
)

const sdkTestOwner values.FactAuthorityRef = "00000000-0000-4000-8000-000000000001"
const sdkTestElement values.ElementID = "00000000-0000-4000-8000-000000000020"

// sdkCA issues short-lived test certificates for the fake service and the
// presented client identity.
type sdkCA struct {
	certificate *x509.Certificate
	key         ed25519.PrivateKey
}

func newSDKCA(t *testing.T, commonName string) *sdkCA {
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
	return &sdkCA{certificate: parsed, key: key}
}

func (c *sdkCA) issue(t *testing.T, commonName string, server bool) (tls.Certificate, [32]byte) {
	t.Helper()
	pub, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	leaf := &x509.Certificate{SerialNumber: big.NewInt(2), Subject: pkix.Name{CommonName: commonName}, NotBefore: c.certificate.NotBefore, NotAfter: c.certificate.NotAfter, KeyUsage: x509.KeyUsageDigitalSignature}
	if server {
		leaf.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}
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

func (c *sdkCA) pool() *x509.CertPool {
	pool := x509.NewCertPool()
	pool.AddCert(c.certificate)
	return pool
}

// fakeRealmService answers the three Realm services with fixed protocol-valid
// responses and records exactly what the SDK sent. It owns no Realm state: SDK
// behavior is provable against wire shape alone, never against a second
// permission implementation.
type fakeRealmService struct {
	realmpb.UnimplementedRealmTargetServiceServer
	realmpb.UnimplementedRealmIdentityServiceServer
	realmpb.UnimplementedRealmAuthorizationServiceServer

	realmID        values.RealmID
	targetMetadata []string
	clientIdentity string
	audience       string
	credentialID   []byte
	secret         []byte
	authorized     []byte
	requirements   []*realmpb.Requirement
	validatedView  []byte
	released       []byte
}

func (f *fakeRealmService) CheckTarget(ctx context.Context, _ *realmpb.CheckTargetRequest) (*realmpb.CheckTargetResponse, error) {
	md, _ := metadata.FromIncomingContext(ctx)
	f.targetMetadata = md.Get("ucupa-realm-target")
	if p, ok := peer.FromContext(ctx); ok {
		if info, ok := p.AuthInfo.(credentials.TLSInfo); ok && len(info.State.PeerCertificates) != 0 {
			f.clientIdentity = info.State.PeerCertificates[0].Subject.CommonName
		}
	}
	return &realmpb.CheckTargetResponse{RealmId: proto.String(string(f.realmID))}, nil
}

func (f *fakeRealmService) EstablishCaller(_ context.Context, req *realmpb.EstablishCallerRequest) (*realmpb.EstablishCallerResponse, error) {
	f.audience = req.GetAudience()
	f.credentialID = req.GetCredentialId()
	f.secret = req.GetSecret()
	caller := make([]byte, 32)
	for i := range caller {
		caller[i] = byte(i + 1)
	}
	return &realmpb.EstablishCallerResponse{
		Caller:           caller,
		SubjectRealm:     proto.String(string(f.realmID)),
		SubjectPrincipal: proto.String("user"),
		ExpiresInSeconds: proto.Uint64(60),
	}, nil
}

func (f *fakeRealmService) AuthorizeView(_ context.Context, req *realmpb.AuthorizeViewRequest) (*realmpb.AuthorizeViewResponse, error) {
	f.authorized = req.GetCaller()
	f.requirements = req.GetRequirements()
	view := make([]byte, 32)
	for i := range view {
		view[i] = byte(32 - i)
	}
	return &realmpb.AuthorizeViewResponse{Decision: realmpb.Decision_VALID.Enum(), View: view}, nil
}

func (f *fakeRealmService) ValidateView(_ context.Context, req *realmpb.ValidateViewRequest) (*realmpb.ValidateViewResponse, error) {
	f.validatedView = req.GetView()
	return &realmpb.ValidateViewResponse{Decision: realmpb.Decision_VALID.Enum()}, nil
}

func (f *fakeRealmService) ReleaseHandle(_ context.Context, req *realmpb.ReleaseHandleRequest) (*realmpb.ReleaseHandleResponse, error) {
	f.released = req.GetHandle()
	return &realmpb.ReleaseHandleResponse{}, nil
}

type fakeRealm struct {
	addr        string
	spki        [32]byte
	roots       *x509.CertPool
	clientCert  tls.Certificate
	serverCert  tls.Certificate
	clientRoots *x509.CertPool
	server      *grpc.Server
	served      chan error
	fake        *fakeRealmService
}

// startFakeRealm serves the recording fake behind real TLS with the same
// optional client-certificate shape the Realm service uses.
func startFakeRealm(t *testing.T) *fakeRealm {
	t.Helper()
	serverCA := newSDKCA(t, "UCUPA realm SDK test CA")
	clientCA := newSDKCA(t, "UCUPA realm SDK client CA")
	serverCert, spki := serverCA.issue(t, "realm-a", true)
	clientCert, _ := clientCA.issue(t, "admitted", false)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	realm := &fakeRealm{
		addr:        listener.Addr().String(),
		spki:        spki,
		roots:       serverCA.pool(),
		clientCert:  clientCert,
		serverCert:  serverCert,
		clientRoots: clientCA.pool(),
		fake:        &fakeRealmService{realmID: "realm-a"},
	}
	realm.serve(t, listener)
	t.Cleanup(realm.stop)
	return realm
}

func (s *fakeRealm) serve(t *testing.T, listener net.Listener) {
	t.Helper()
	tlsConfig := &tls.Config{
		MinVersion:   tls.VersionTLS13,
		MaxVersion:   tls.VersionTLS13,
		Certificates: []tls.Certificate{s.serverCert},
		ClientCAs:    s.clientRoots,
		ClientAuth:   tls.VerifyClientCertIfGiven,
		NextProtos:   []string{"h2"},
	}
	server := grpc.NewServer(grpc.Creds(credentials.NewTLS(tlsConfig)))
	realmpb.RegisterRealmTargetServiceServer(server, s.fake)
	realmpb.RegisterRealmIdentityServiceServer(server, s.fake)
	realmpb.RegisterRealmAuthorizationServiceServer(server, s.fake)
	s.server = server
	s.served = make(chan error, 1)
	go func() { s.served <- server.Serve(listener) }()
}

// stop tears the listener down and waits for Serve to return.
func (s *fakeRealm) stop() {
	if s.server == nil {
		return
	}
	s.server.Stop()
	select {
	case <-s.served:
	case <-time.After(5 * time.Second):
	}
	s.server = nil
}

// restart replaces the serving Realm with another identity behind the same
// address and the same pinned certificate, so the client must complete a new
// TLS handshake. It returns the new recording fake.
func (s *fakeRealm) restart(t *testing.T, realmID values.RealmID) *fakeRealmService {
	t.Helper()
	s.stop()
	s.fake = &fakeRealmService{realmID: realmID}
	listener, err := net.Listen("tcp", s.addr)
	for attempt := 0; err != nil && attempt < 50; attempt++ {
		time.Sleep(10 * time.Millisecond)
		listener, err = net.Listen("tcp", s.addr)
	}
	if err != nil {
		t.Fatalf("rebind %s: %v", s.addr, err)
	}
	s.serve(t, listener)
	return s.fake
}

func (s *fakeRealm) dialConfig() Config {
	return Config{
		NextHop:          values.NextHop("https://" + s.addr),
		Expected:         "realm-a",
		Roots:            s.roots,
		SPKISHA256:       s.spki,
		Certificate:      &s.clientCert,
		RPCDeadline:      5 * time.Second,
		ConnectTimeout:   5 * time.Second,
		MaxResponseBytes: 1 << 20,
		MaxMetadataBytes: 8192,
	}
}

func sdkCredential() (id [16]byte, secret [32]byte) {
	for i := range id {
		id[i] = byte(i)
	}
	for i := range secret {
		secret[i] = byte(200 - i)
	}
	return id, secret
}

func sdkRequirement() *realmpb.Requirement {
	return &realmpb.Requirement{
		Permission: proto.String("ucupa.test.read"),
		Scope: &realmpb.Scope{
			Kind:      realmpb.ScopeKind_CORE_ELEMENT.Enum(),
			Authority: proto.String(string(sdkTestOwner)),
			Element:   proto.String(string(sdkTestElement)),
		},
	}
}

// rebindAddress takes the same listening address back after the fake Realm
// released it, mirroring restart's retry pattern without starting a server.
func rebindAddress(t *testing.T, addr string) net.Listener {
	t.Helper()
	listener, err := net.Listen("tcp", addr)
	for attempt := 0; err != nil && attempt < 50; attempt++ {
		time.Sleep(10 * time.Millisecond)
		listener, err = net.Listen("tcp", addr)
	}
	if err != nil {
		t.Fatalf("rebind %s: %v", addr, err)
	}
	t.Cleanup(func() { listener.Close() })
	return listener
}

// readApplicationBytes drains the decrypted side of a completed server-side
// TLS connection until the peer stops sending or the bounded deadline passes.
func readApplicationBytes(t *testing.T, conn *tls.Conn) []byte {
	t.Helper()
	if err := conn.SetReadDeadline(time.Now().Add(5 * time.Second)); err != nil {
		t.Fatal(err)
	}
	var seen []byte
	buffer := make([]byte, 4096)
	for {
		n, err := conn.Read(buffer)
		seen = append(seen, buffer[:n]...)
		if err != nil {
			return seen
		}
		if len(seen) > 1<<20 {
			return seen
		}
	}
}
