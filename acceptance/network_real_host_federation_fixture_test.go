package acceptance_test

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"

	netpb "ucupa/shared/bindings/go/protobuf"
	"ucupa/shared/values"
	pb "ucupa/ucupa-core/bindings/go/protobuf"
	corecontract "ucupa/ucupa-core/contract"
	core "ucupa/ucupa-core/instance"
	coresdk "ucupa/ucupa-core/sdk/go"
	realm "ucupa/ucupa-realm/contract"
	realminstance "ucupa/ucupa-realm/instance"
)

// Only disposable test credentials cross the verified SSH boundary. A and C
// are separate owners on host A; B is on host B. The public signing key is
// shared, but the issuer's private key never leaves host A.
type p6FederationManifest struct {
	p6RealHostManifest
	IssuerPublicKey string `json:"issuer_public_key,omitempty"`
	GatePrefix      string `json:"gate_prefix,omitempty"`
}

const p6FederationMount = values.ElementID(networkEntity)
const p6FederationNested = nestedProductAnchor
const p6FederationLeaf = nestedProductLeaf
const p6FederationExtra = values.ElementID("00000000-0000-4000-8000-000000000017")

func p6FederationBind(t *testing.T, f *networkFixture, ip net.IP) p6FederationManifest {
	t.Helper()
	l, err := net.Listen("tcp", net.JoinHostPort(ip.String(), "0"))
	if err != nil {
		t.Fatal(err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	if err := l.Close(); err != nil {
		t.Fatal(err)
	}
	hop := values.NextHop("https://" + net.JoinHostPort(ip.String(), strconv.Itoa(port)))
	cert, roots, pin, certPEM, keyPEM := networkCertificateForIPs(t, ip)
	f.certPEM, f.keyPEM = certPEM, keyPEM
	f.config.Certificate, f.config.NextHop = cert, hop
	f.clientConfig.NextHop, f.clientConfig.SPKISHA256, f.clientConfig.Roots = hop, pin, roots
	return p6FederationManifest{p6RealHostManifest: p6RealHostManifest{
		NextHop: string(hop), Expected: string(f.c.AuthorityRef()), Realm: string(f.actor.Realm),
		CredentialID: base64.RawURLEncoding.EncodeToString(f.id[:]),
		Secret:       base64.RawURLEncoding.EncodeToString(f.secret[:]),
		CAPEM:        string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cert.Certificate[1]})),
		SPKISHA256:   base64.RawURLEncoding.EncodeToString(pin[:]),
	}}
}

func p6FederationEmptyCarrier(t *testing.T, f *networkFixture) {
	t.Helper()
	if err := f.c.Close(); err != nil {
		t.Fatal(err)
	}
	var err error
	f.c, err = core.CreateDurable(f.realmLinks, corecontract.RealmInitialization{Realm: f.actor.Realm, NextHop: f.realmAddress}, filepath.Join(f.dir, "empty-carrier.db"), corecontract.Config{
		PublicAccessPolicy: &values.PublicAccessPolicy{ContextTTL: 10 * time.Minute, ObservationTTL: 10 * time.Minute},
	})
	if err != nil {
		t.Fatal(err)
	}
	f.config.Expected, f.clientConfig.Expected = f.c.AuthorityRef(), f.c.AuthorityRef()
}

type p6FederationSeedEntry struct {
	parent, element values.ElementID
	anchor          bool
}

// Product hosts are stopped while this formal owner API seeds disposable
// content after the incoming public mount exists. H3 does not test public
// Element creation; all behavior assertions resume through product gRPC.
func p6FederationSeedContent(t *testing.T, f *networkFixture, entries ...p6FederationSeedEntry) {
	t.Helper()
	var err error
	f.r, err = realminstance.OpenDurable(filepath.Join(f.dir, "realm.db"))
	if err != nil {
		t.Fatal(err)
	}
	if err = f.realmLinks.BindEndpoint(f.realmAddress, f.r); err != nil {
		t.Fatal(err)
	}
	f.c, err = core.OpenDurable(f.realmLinks, filepath.Join(f.dir, "empty-carrier.db"), corecontract.Config{
		PublicAccessPolicy: &values.PublicAccessPolicy{ContextTTL: 10 * time.Minute, ObservationTTL: 10 * time.Minute},
	})
	if err != nil {
		t.Fatal(err)
	}
	rotation, _ := corecontract.NewRotation(1, 0, 0, 0)
	radius, _ := corecontract.NewBoundedSphere(5)
	for _, entry := range entries {
		tuple := corecontract.SpatialTuple{Parent: entry.parent, LocalPose: corecontract.Pose{Rotation: rotation}, Range: radius}
		operation := corecontract.OperationID("p6-h3-seed-" + string(entry.element))
		if entry.anchor {
			if _, err := f.c.Elements().CreateAnchor(f.actor, entry.element, tuple, operation); err != nil {
				t.Fatal(err)
			}
		} else if _, err := f.c.Elements().CreateEntity(f.actor, entry.element, tuple, operation); err != nil {
			t.Fatal(err)
		}
	}
	if err := f.c.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.r.Close(); err != nil {
		t.Fatal(err)
	}
}

func p6FederationExport(t *testing.T, path string, m p6FederationManifest) {
	t.Helper()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("stale real-host manifest at %s: %v", path, err)
	}
	raw, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Write(raw); err != nil {
		_ = f.Close()
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Remove(path) })
}

func p6FederationImport(t *testing.T, path string) p6FederationManifest {
	t.Helper()
	var m p6FederationManifest
	if err := json.Unmarshal(p6RealHostAwaitManifest(t, path, 15*time.Minute), &m); err != nil {
		t.Fatal(err)
	}
	if !values.FactAuthorityRef(m.Expected).Valid() || m.Realm == "" || !values.NextHop(m.NextHop).Valid() {
		t.Fatal("invalid real-host Core manifest identity")
	}
	_, _ = p6RealHostCredential(t, m.CredentialID, m.Secret)
	_ = p6RealHostPin(t, m.SPKISHA256)
	_ = p6RealHostRoots(t, m.CAPEM)
	t.Cleanup(func() { _ = os.Remove(path) })
	return m
}

func p6FederationPeer(t *testing.T, dir string, m p6FederationManifest) map[string]any {
	t.Helper()
	path := filepath.Join(dir, "peer-"+m.Expected+".pem")
	writeNetworkFile(t, path, []byte(m.CAPEM))
	return map[string]any{"next_hop": values.NextHop(m.NextHop), "expected_core": values.FactAuthorityRef(m.Expected),
		"ca_file": path, "spki_sha256": m.SPKISHA256}
}

func p6FederationTrust(t *testing.T, dir string, peers ...p6FederationManifest) map[string]any {
	t.Helper()
	var roots []byte
	pins := make(map[string]string)
	for _, peer := range peers {
		roots = append(roots, peer.CAPEM...)
		pins[peer.Expected] = peer.SPKISHA256
	}
	path := filepath.Join(dir, "trusted-peer-cas.pem")
	writeNetworkFile(t, path, roots)
	return map[string]any{"client_ca_file": path, "client_spki_sha256": pins,
		"max_ttl": "10m", "max_grants": 32, "max_per_caller": 24}
}

func p6FederationExternalIdentity(t *testing.T, dir string, issuer p6FederationManifest) map[string]any {
	t.Helper()
	if issuer.IssuerPublicKey == "" {
		t.Fatal("missing trusted issuer public key")
	}
	path := filepath.Join(dir, "issuer.pub.pem")
	writeNetworkFile(t, path, []byte(issuer.IssuerPublicKey))
	peer := p6FederationPeer(t, dir, issuer)
	peer["signing_public_key_file"] = path
	return map[string]any{"max_ttl": "10m", "issuers": map[string]any{issuer.Realm: peer}}
}

func p6FederationClient(t *testing.T, m p6FederationManifest) *coresdk.Client {
	t.Helper()
	client, err := coresdk.New(coresdk.Config{NextHop: values.NextHop(m.NextHop), Expected: values.FactAuthorityRef(m.Expected),
		Roots: p6RealHostRoots(t, m.CAPEM), SPKISHA256: p6RealHostPin(t, m.SPKISHA256),
		RPCDeadline: 5 * time.Second, ConnectTimeout: 3 * time.Second, MaxResponseBytes: 8 << 20, MaxMetadataBytes: 8192})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	return client
}

func p6FederationLogin(t *testing.T, client *coresdk.Client, m p6FederationManifest) {
	t.Helper()
	id, secret := p6RealHostCredential(t, m.CredentialID, m.Secret)
	if err := client.Establish(context.Background(), values.RealmID(m.Realm), id, secret); err != nil {
		t.Fatal(err)
	}
}

func p6FederationWaitLogin(t *testing.T, client *coresdk.Client, m p6FederationManifest) {
	t.Helper()
	id, secret := p6RealHostCredential(t, m.CredentialID, m.Secret)
	deadline := time.Now().Add(10 * time.Minute)
	for time.Now().Before(deadline) {
		if err := client.Establish(context.Background(), values.RealmID(m.Realm), id, secret); err == nil {
			return
		}
		time.Sleep(500 * time.Millisecond)
	}
	t.Fatal("peer Core product did not become reachable")
}

func p6FederationGrantOriginal(t *testing.T, host *networkFixture, original p6FederationManifest, scope values.OperationScope) {
	t.Helper()
	subject := values.PrincipalRef{Realm: values.RealmID(original.Realm), Principal: "operator"}
	for i, permission := range []values.PermissionID{corecontract.PermissionReadElement, corecontract.PermissionListDirect, corecontract.PermissionQueryRegion} {
		binding := realm.PermissionBinding{Subject: subject, Permission: permission, Scope: scope}
		if _, err := host.r.PutBinding(host.actor, binding, host.r.BindingSetRevision(), realm.OperationID("h3-original-"+strconv.Itoa(i))); err != nil {
			t.Fatal(err)
		}
	}
}

func p6FederationIssuer(t *testing.T, dir string) (string, string) {
	t.Helper()
	pub, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	keyDER, err := x509.MarshalPKCS8PrivateKey(private)
	if err != nil {
		t.Fatal(err)
	}
	pubDER, err := x509.MarshalPKIXPublicKey(pub)
	if err != nil {
		t.Fatal(err)
	}
	privatePath := filepath.Join(dir, "issuer.pem")
	writeNetworkFile(t, privatePath, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER}))
	return privatePath, string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pubDER}))
}

func p6FederationReference(m p6FederationManifest, anchor values.ElementID) *netpb.Reference {
	return &netpb.Reference{Boundary: &netpb.Boundary{Core: proto.String(m.Expected), Anchor: proto.String(string(anchor))},
		NextHop: proto.String(m.NextHop)}
}

func p6FederationEnter(t *testing.T, client *coresdk.Client, m p6FederationManifest, anchor values.ElementID, intent pb.Intent) *netpb.Token {
	t.Helper()
	out, err := client.Core().Enter(context.Background(), &pb.EnterRequest{Reference: p6FederationReference(m, anchor), Intent: intent.Enum()})
	if err != nil || out.GetResult() == nil {
		t.Fatalf("public Enter %v at %s: %v %v", intent, m.Expected, out, err)
	}
	return out.GetResult().Handle
}
