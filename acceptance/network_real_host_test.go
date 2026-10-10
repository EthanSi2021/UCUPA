package acceptance_test

import (
	"context"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	netpb "ucupa/shared/bindings/go/protobuf"
	"ucupa/shared/values"
	corepb "ucupa/ucupa-core/bindings/go/protobuf"
	coresdk "ucupa/ucupa-core/sdk/go"
)

// This opt-in fixture puts a real Core product on one host and a public SDK
// caller on another. Its manifest contains a disposable credential: transfer
// it only over an authenticated channel and remove the remote copy afterward.
type p6RealHostManifest struct {
	NextHop      string `json:"next_hop"`
	Expected     string `json:"expected_core"`
	Realm        string `json:"realm"`
	CredentialID string `json:"credential_id"`
	Secret       string `json:"secret"`
	CAPEM        string `json:"ca_pem"`
	SPKISHA256   string `json:"spki_sha256"`
}

func TestP6RealHostCoreProduct(t *testing.T) {
	bindIP, export, gate := os.Getenv("UCUPA_P6_HOST_IP"), os.Getenv("UCUPA_P6_MANIFEST_OUT"), os.Getenv("UCUPA_P6_HOST_GATE")
	if bindIP == "" && export == "" && gate == "" {
		t.Skip("opt-in real-host Core product")
	}
	ip := net.ParseIP(bindIP)
	if ip == nil || !ip.IsPrivate() || export == "" || gate == "" {
		t.Fatal("real-host product requires a private bind IP, manifest path, and gate prefix")
	}
	for _, path := range []string{export, gate + ".restart", gate + ".restarted", gate + ".done"} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("real-host control path must not exist: %s (%v)", path, err)
		}
	}
	f := newNetworkFixture(t, false)
	l, err := net.Listen("tcp", net.JoinHostPort(ip.String(), "0"))
	if err != nil {
		t.Fatal(err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	if err := l.Close(); err != nil {
		t.Fatal(err)
	}
	hop := values.NextHop("https://" + net.JoinHostPort(ip.String(), strconv.Itoa(port)))
	cert, _, pin, certPEM, keyPEM := networkCertificateForIPs(t, ip)
	f.certPEM, f.keyPEM = certPEM, keyPEM
	f.config.Certificate, f.config.NextHop = cert, hop
	f.clientConfig.NextHop, f.clientConfig.SPKISHA256 = hop, pin
	start := prepareNetworkProductControl(t, f, map[string]any{"session_ttl": "3m", "connection_lifetime": "4m"})
	product := start()
	defer product.stop()
	manifest := p6RealHostManifest{
		NextHop: string(hop), Expected: string(f.config.Expected), Realm: string(f.actor.Realm),
		CredentialID: base64.RawURLEncoding.EncodeToString(f.id[:]),
		Secret:       base64.RawURLEncoding.EncodeToString(f.secret[:]),
		CAPEM:        string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cert.Certificate[1]})),
		SPKISHA256:   base64.RawURLEncoding.EncodeToString(pin[:]),
	}
	raw, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	file, err := os.OpenFile(export, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.Write(raw); err != nil {
		_ = file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = os.Remove(export)
		_ = os.Remove(gate + ".restart")
		_ = os.Remove(gate + ".restarted")
		_ = os.Remove(gate + ".done")
	})
	t.Logf("Core product ready at %s; copy disposable manifest %s to the other host", hop, export)
	deadline := time.Now().Add(12 * time.Minute)
	restarted := false
	for time.Now().Before(deadline) {
		if _, err := os.Stat(gate + ".restart"); err == nil && !restarted {
			product.gracefulStop()
			product = start() // same original Core and Realm Stores
			restarted = true
			if err := os.WriteFile(gate+".restarted", []byte("ready"), 0600); err != nil {
				t.Fatal(err)
			}
			t.Log("Core product restarted on its original Stores")
		}
		if _, err := os.Stat(gate + ".done"); err == nil {
			if !restarted {
				t.Fatal("real-host exercise ended without original-Store restart")
			}
			product.gracefulStop()
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatal("real-host exercise timed out")
}

func TestP6RealHostCoreProbe(t *testing.T) {
	path, state, phase := os.Getenv("UCUPA_P6_REMOTE_MANIFEST"), os.Getenv("UCUPA_P6_REMOTE_STATE"), os.Getenv("UCUPA_P6_REMOTE_PHASE")
	if path == "" && state == "" && phase == "" {
		t.Skip("opt-in real-host SDK probe")
	}
	if path == "" || state == "" || phase != "write" && phase != "recover" {
		t.Fatal("real-host probe requires a manifest, state path, and write/recover phase")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var manifest p6RealHostManifest
	if err := json.Unmarshal(raw, &manifest); err != nil {
		t.Fatal(err)
	}
	endpoint, err := url.Parse(manifest.NextHop)
	if err != nil {
		t.Fatal(err)
	}
	targetIP := net.ParseIP(endpoint.Hostname())
	if targetIP == nil || !targetIP.IsPrivate() {
		t.Fatal("real-host probe requires a private IP endpoint")
	}
	addresses, err := net.InterfaceAddrs()
	if err != nil {
		t.Fatal(err)
	}
	for _, address := range addresses {
		if network, ok := address.(*net.IPNet); ok && network.IP.Equal(targetIP) {
			t.Fatal("target Core is on the probe host, not a second machine")
		}
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM([]byte(manifest.CAPEM)) {
		t.Fatal("invalid manifest CA")
	}
	pinBytes, err := base64.RawURLEncoding.DecodeString(manifest.SPKISHA256)
	if err != nil || len(pinBytes) != 32 {
		t.Fatal("invalid manifest SPKI pin")
	}
	idBytes, idErr := base64.RawURLEncoding.DecodeString(manifest.CredentialID)
	secretBytes, secretErr := base64.RawURLEncoding.DecodeString(manifest.Secret)
	if idErr != nil || secretErr != nil || len(idBytes) != 16 || len(secretBytes) != 32 {
		t.Fatal("invalid manifest credential")
	}
	var pin [32]byte
	copy(pin[:], pinBytes)
	config := coresdk.Config{NextHop: values.NextHop(manifest.NextHop), Expected: values.FactAuthorityRef(manifest.Expected),
		Roots: roots, SPKISHA256: pin, RPCDeadline: 5 * time.Second, ConnectTimeout: 3 * time.Second,
		MaxResponseBytes: 8 << 20, MaxMetadataBytes: 8192}
	open := func(c coresdk.Config) *coresdk.Client {
		t.Helper()
		client, err := coresdk.New(c)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = client.Close() })
		return client
	}
	ctx := context.Background()
	var id [16]byte
	var secret [32]byte
	copy(id[:], idBytes)
	copy(secret[:], secretBytes)
	if phase == "write" {
		badConfig := config
		badConfig.SPKISHA256[0] ^= 0xff
		if err := open(badConfig).Establish(ctx, values.RealmID(manifest.Realm), id, secret); err == nil {
			t.Fatal("wrong TLS key accepted")
		}
		badOwner := config
		badOwner.Expected = "00000000-0000-4000-8000-00000000ffff"
		if err := open(badOwner).Establish(ctx, values.RealmID(manifest.Realm), id, secret); err == nil {
			t.Fatal("wrong expected Core accepted")
		}
		badSecret := secret
		badSecret[0] ^= 0xff
		if err := open(config).Establish(ctx, values.RealmID(manifest.Realm), id, badSecret); status.Code(err) != codes.Unauthenticated {
			t.Fatalf("wrong credential was not rejected: %v", err)
		}
	}
	client := open(config)
	if err := client.Establish(ctx, values.RealmID(manifest.Realm), id, secret); err != nil {
		t.Fatalf("real-host Establish: %v", err)
	}
	ref := &netpb.Reference{Boundary: &netpb.Boundary{Core: proto.String(manifest.Expected), Anchor: proto.String(string(networkRoot))}, NextHop: proto.String(manifest.NextHop)}
	const element = "00000000-0000-4000-8000-00000000a006"
	operation := []byte("p6-real-host-create-entity")
	final := networkTuple(1)
	command := &corepb.CreateEntityCommand{Element: proto.String(element), OperationId: operation, Final: final}
	var receipt *corepb.MutationResult
	if phase == "write" {
		entered, err := client.Core().Enter(ctx, &corepb.EnterRequest{Reference: ref, Intent: corepb.Intent_CREATE_ENTITY.Enum()})
		if err != nil || entered.GetResult() == nil {
			t.Fatalf("remote Enter: %v %v", entered, err)
		}
		created, err := client.Core().CreateEntity(ctx, &corepb.CreateEntityRequest{Handle: entered.GetResult().Handle, Element: command.Element, OperationId: operation, Final: final})
		if err != nil || created.GetResult() == nil {
			t.Fatalf("remote CreateEntity: %v %v", created, err)
		}
		receipt = created.GetResult()
	} else {
		recovered, err := client.Core().RecoverOutcome(ctx, &corepb.RecoverOutcomeRequest{Reference: ref, OperationId: operation,
			Request: &corepb.MutationCommand{Command: &corepb.MutationCommand_CreateEntity{CreateEntity: command}}})
		if err != nil || recovered.GetResult() == nil {
			t.Fatalf("cross-host original OperationID recovery: %v %v", recovered, err)
		}
		receipt = recovered.GetResult()
	}
	encoded, err := proto.Marshal(receipt)
	if err != nil {
		t.Fatal(err)
	}
	digest := fmt.Sprintf("%x", sha256.Sum256(encoded))
	if phase == "write" {
		file, err := os.OpenFile(state, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := file.WriteString(digest); err != nil {
			_ = file.Close()
			t.Fatal(err)
		}
		if err := file.Close(); err != nil {
			t.Fatal(err)
		}
	} else {
		previous, err := os.ReadFile(state)
		if err != nil || strings.TrimSpace(string(previous)) != digest {
			t.Fatalf("original receipt changed after cross-host restart: %v", err)
		}
	}
	entered, err := client.Core().Enter(ctx, &corepb.EnterRequest{Reference: ref, Intent: corepb.Intent_READ.Enum()})
	if err != nil || entered.GetResult() == nil {
		t.Fatalf("remote read Enter: %v %v", entered, err)
	}
	read, err := client.Core().ReadElement(ctx, &corepb.ReadElementRequest{Handle: entered.GetResult().Handle,
		Address: &netpb.Address{Down: &netpb.ElementPath{Items: []string{element}}}})
	if err != nil || read.GetResult().GetRecord() == nil || !proto.Equal(receipt.GetRecord().Revision, read.GetResult().GetRecord().Revision) {
		t.Fatalf("cross-host exact read: %v %v", read, err)
	}
	t.Logf("P6 real-host %s: TLS target, caller auth, Core write/read and original receipt verified at %s", phase, manifest.NextHop)
}
