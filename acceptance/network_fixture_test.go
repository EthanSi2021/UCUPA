package acceptance_test

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"

	"ucupa/composition"
	networkpb "ucupa/shared/bindings/go/protobuf"
	"ucupa/shared/values"
	corepb "ucupa/ucupa-core/bindings/go/protobuf"
	corecontract "ucupa/ucupa-core/contract"
	core "ucupa/ucupa-core/instance"
	coresdk "ucupa/ucupa-core/sdk/go"
	coregrpc "ucupa/ucupa-core/transport/grpc"
	realm "ucupa/ucupa-realm/contract"
	realminstance "ucupa/ucupa-realm/instance"
)

const networkRoot values.ElementID = "00000000-0000-4000-8000-000000000001"
const networkEntity = "00000000-0000-4000-8000-000000000002"

type networkFixture struct {
	admin              realm.CredentialEvidence
	dir                string
	c                  *core.Core
	r                  *realminstance.Realm
	realmLinks         *composition.LocalRealmConnections
	realmAddress       values.NextHop
	actor              values.PrincipalRef
	id                 [16]byte
	secret             [32]byte
	facetService       values.PrincipalRef
	facetServiceID     [16]byte
	facetServiceSecret [32]byte
	server             *coregrpc.Server
	client             *coresdk.Client
	config             coregrpc.Config
	clientConfig       coresdk.Config
	certPEM, keyPEM    []byte
	serial             int
}

func newNetworkFixture(t *testing.T, start bool) *networkFixture {
	return newNetworkFixtureRealm(t, start, "network-realm")
}

func newNetworkFixtureRealm(t *testing.T, start bool, id values.RealmID) *networkFixture {
	return newNetworkFixtureWriter(t, start, id, nil)
}

func newNetworkFixtureWriter(t *testing.T, start bool, id values.RealmID, writer *values.WriterStartup) *networkFixture {
	t.Helper()
	f := &networkFixture{dir: t.TempDir(), actor: values.PrincipalRef{Realm: id, Principal: "operator"},
		facetService: values.PrincipalRef{Realm: id, Principal: "facet-service"}}
	_, _ = rand.Read(f.id[:])
	_, _ = rand.Read(f.secret[:])
	_, _ = rand.Read(f.facetServiceID[:])
	_, _ = rand.Read(f.facetServiceSecret[:])
	f.admin.Audience = f.actor.Realm
	_, _ = rand.Read(f.admin.ID[:])
	_, _ = rand.Read(f.admin.Secret[:])
	var err error
	f.r, err = realminstance.CreateDurable(realm.Seed{ID: f.actor.Realm, LocalPrincipals: []values.PrincipalID{f.actor.Principal, f.facetService.Principal}, Bindings: []realm.PermissionBinding{
		{Subject: f.actor, Permission: realm.PermissionPutBinding, Scope: values.RealmScope(f.actor.Realm)},
		{Subject: f.actor, Permission: realm.PermissionDeleteBinding, Scope: values.RealmScope(f.actor.Realm)},
		{Subject: f.actor, Permission: realm.PermissionRevokeCredential, Scope: values.RealmScope(f.actor.Realm)},
	}, Credentials: []realm.CredentialBootstrap{{Subject: f.actor.Principal, Evidence: realm.CredentialEvidence{Audience: f.actor.Realm, ID: realm.CredentialID(f.id), Secret: realm.CredentialSecret(f.secret)}}, {Subject: f.actor.Principal, Evidence: f.admin}, {Subject: f.facetService.Principal, Evidence: realm.CredentialEvidence{Audience: f.actor.Realm, ID: realm.CredentialID(f.facetServiceID), Secret: realm.CredentialSecret(f.facetServiceSecret)}}}}, filepath.Join(f.dir, "realm.db"))
	if err != nil {
		t.Fatal(err)
	}
	f.realmAddress = values.NextHop("local:" + string(id))
	f.realmLinks = &composition.LocalRealmConnections{}
	if err = f.realmLinks.BindEndpoint(f.realmAddress, f.r); err != nil {
		t.Fatal(err)
	}
	f.c, err = core.CreateDurable(f.realmLinks, corecontract.RealmInitialization{Realm: f.actor.Realm, NextHop: f.realmAddress}, filepath.Join(f.dir, "core.db"), corecontract.Config{Writer: writer, PublicAccessPolicy: &values.PublicAccessPolicy{ContextTTL: time.Minute, ObservationTTL: time.Minute}})
	if err != nil {
		t.Fatal(err)
	}
	f.grant(t, corecontract.PermissionCreateRoot, values.CoreElementScope(f.c.AuthorityRef(), networkRoot), false)
	if _, err = f.c.Elements().CreateRoot(f.actor, networkRoot, "root"); err != nil {
		t.Fatal(err)
	}
	for _, p := range []values.PermissionID{corecontract.PermissionReadElement, corecontract.PermissionListDirect, corecontract.PermissionQueryRegion, corecontract.PermissionCreateAnchor, corecontract.PermissionCreateEntity, corecontract.PermissionUpdateElement, corecontract.PermissionPlaceElement, corecontract.PermissionDeleteAnchor, corecontract.PermissionDeleteEntity, corecontract.PermissionObserveElement, corecontract.PermissionObserveAttachment} {
		f.grant(t, p, values.AnchorClosureScope(f.c.AuthorityRef(), networkRoot), false)
	}
	cert, roots, pin, certPEM, keyPEM := networkCertificate(t)
	f.certPEM, f.keyPEM = certPEM, keyPEM
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := values.NextHop("https://" + l.Addr().String())
	f.config = coregrpc.Config{Expected: f.c.AuthorityRef(), NextHop: address, Certificate: cert, SessionTTL: 30 * time.Second, RPCDeadline: 5 * time.Second, HandshakeTimeout: time.Second, ConnectionLifetime: time.Minute, ShutdownTimeout: time.Second, MaxSessions: 16, MaxSessionsPerCaller: 8, MaxConnections: 16, MaxConcurrentRPCs: 8, MaxStreams: 8, MaxMetadataBytes: 8192, MaxResponseBytes: 8 << 20, Federation: f.c.Federation()}
	f.clientConfig = coresdk.Config{Expected: f.c.AuthorityRef(), NextHop: address, Roots: roots, SPKISHA256: pin, RPCDeadline: 5 * time.Second, ConnectTimeout: time.Second, MaxResponseBytes: 8 << 20, MaxMetadataBytes: 8192}
	t.Cleanup(func() {
		if f.client != nil {
			_ = f.client.Close()
		}
		if f.server != nil {
			f.server.Stop()
		}
		_ = f.c.Close()
		_ = f.r.Close()
	})
	if start {
		f.config.WriterControl = f.c.Writer()
		f.config.TransitionControl = f.c.Transfers()
		f.config.FacetControl = f.c.Facets()
		f.config.AttachmentControl = f.c.Facets()
		f.config.MountControl = f.c.Mounts()
		f.config.Federation = f.c.Federation()
		f.server, err = coregrpc.New(f.c.Access(), composition.RealmNetworkAuthentication{Connections: f.realmLinks, Owner: f.c.Access()}, f.config)
		if err != nil {
			t.Fatal(err)
		}
		go func() { _ = f.server.Serve(l) }()
		f.connect(t)
	} else {
		_ = l.Close()
	}
	return f
}

func (f *networkFixture) grant(t *testing.T, p values.PermissionID, scope values.OperationScope, remove bool) {
	t.Helper()
	f.serial++
	binding := realm.PermissionBinding{Subject: f.actor, Permission: p, Scope: scope}
	var err error
	if remove {
		_, err = f.r.DeleteBinding(f.actor, binding, f.r.BindingSetRevision(), realm.OperationID(fmt.Sprintf("binding-%d", f.serial)))
	} else {
		_, err = f.r.PutBinding(f.actor, binding, f.r.BindingSetRevision(), realm.OperationID(fmt.Sprintf("binding-%d", f.serial)))
	}
	if err != nil {
		t.Fatal(err)
	}
}
func (f *networkFixture) connect(t *testing.T) {
	t.Helper()
	var err error
	f.client, err = coresdk.New(f.clientConfig)
	if err != nil {
		t.Fatal(err)
	}
	if err = f.client.Establish(context.Background(), f.actor.Realm, f.id, f.secret); err != nil {
		t.Fatal(err)
	}
}
func (f *networkFixture) reference() *networkpb.Reference {
	return &networkpb.Reference{Boundary: &networkpb.Boundary{Core: proto.String(string(f.config.Expected)), Anchor: proto.String(string(networkRoot))}, NextHop: proto.String(string(f.config.NextHop))}
}
func (f *networkFixture) enter(t *testing.T, intent corepb.Intent) *networkpb.Token {
	t.Helper()
	out, err := f.client.Core().Enter(context.Background(), &corepb.EnterRequest{Reference: f.reference(), Intent: intent.Enum()})
	if err != nil || out.GetResult() == nil {
		t.Fatalf("Enter: %v %v", out, err)
	}
	return out.GetResult().Handle
}
func networkTuple(radius float64) *corepb.SpatialTuple {
	return &corepb.SpatialTuple{Parent: proto.String(string(networkRoot)), Pose: &corepb.Pose{Translation: &networkpb.Vector3{Items: []float64{0, 0, 0}}, Rotation: &corepb.Rotation{Pivot: proto.Uint32(4), Ratios: &networkpb.Vector3{Items: []float64{0, 0, 0}}}}, Range: &corepb.Range{Kind: corepb.RangeKind_BOUNDED_SPHERE.Enum(), Radius: proto.Float64(radius)}}
}

func networkCertificate(t *testing.T) (tls.Certificate, *x509.CertPool, [32]byte, []byte, []byte) {
	return networkCertificateForIPs(t)
}

func networkCertificateForIPs(t *testing.T, extra ...net.IP) (tls.Certificate, *x509.CertPool, [32]byte, []byte, []byte) {
	t.Helper()
	pub, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	ca := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "UCUPA test CA"}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour), IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature}
	caDER, err := x509.CreateCertificate(rand.Reader, ca, ca, pub, key)
	if err != nil {
		t.Fatal(err)
	}
	ca, err = x509.ParseCertificate(caDER)
	if err != nil {
		t.Fatal(err)
	}
	leafPub, leafKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	leaf := &x509.Certificate{SerialNumber: big.NewInt(2), Subject: pkix.Name{CommonName: "Core test"}, NotBefore: ca.NotBefore, NotAfter: ca.NotAfter, KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth}, DNSNames: []string{"localhost"}, IPAddresses: append([]net.IP{net.ParseIP("127.0.0.1")}, extra...)}
	der, err := x509.CreateCertificate(rand.Reader, leaf, ca, leafPub, key)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	keyDER, err := x509.MarshalPKCS8PrivateKey(leafKey)
	if err != nil {
		t.Fatal(err)
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER})
	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		t.Fatal(err)
	}
	cert.Certificate = append(cert.Certificate, caDER)
	roots := x509.NewCertPool()
	roots.AddCert(ca)
	return cert, roots, sha256.Sum256(parsed.RawSubjectPublicKeyInfo), certPEM, keyPEM
}

func writeNetworkFile(t *testing.T, name string, data []byte) {
	t.Helper()
	if err := os.WriteFile(name, data, 0600); err != nil {
		t.Fatal(err)
	}
}
