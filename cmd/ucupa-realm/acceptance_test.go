package main

import (
	"bufio"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	"ucupa/shared/values"
	realmpb "ucupa/ucupa-realm/bindings/go/protobuf"
	realm "ucupa/ucupa-realm/contract"
	realmsdk "ucupa/ucupa-realm/sdk/go"
)

// productBootstrap declares the complete initial state of one Realm: two
// local principals, one administration binding, one user grant and one user
// credential. Nothing beyond this document is seeded.
const productBootstrap = `{
	"realm_id": "realm-a",
	"principals": ["admin", "user"],
	"bindings": [
		{"subject": {"realm": "realm-a", "principal": "admin"}, "permission": "ucupa.realm.binding.put", "scope": {"kind": "realm", "realm": "realm-a"}},
		{"subject": {"realm": "realm-a", "principal": "user"}, "permission": "ucupa.test.read", "scope": {"kind": "core_element", "authority": "00000000-0000-4000-8000-000000000001", "element": "00000000-0000-4000-8000-000000000020"}}
	],
	"credentials": [
		{"subject": "user", "audience": "realm-a", "id": "00112233445566778899aabbccddeeff", "secret": "ffeeddccbbaa99887766554433221100ffeeddccbbaa99887766554433221100"}
	]
}`

var productCredential realm.CredentialEvidence

func init() {
	rawID, _ := hex.DecodeString("00112233445566778899aabbccddeeff")
	rawSecret, _ := hex.DecodeString("ffeeddccbbaa99887766554433221100ffeeddccbbaa99887766554433221100")
	copy(productCredential.ID[:], rawID)
	copy(productCredential.Secret[:], rawSecret)
	productCredential.Audience = "realm-a"
}

func productRequirement() *realmpb.Requirement {
	return &realmpb.Requirement{
		Permission: proto.String("ucupa.test.read"),
		Scope: &realmpb.Scope{
			Kind:      realmpb.ScopeKind_CORE_ELEMENT.Enum(),
			Authority: proto.String(string(testOwner)),
			Element:   proto.String(string(testElement)),
		},
	}
}

// productBinary builds the real product once for the whole test run.
var (
	productBinaryOnce sync.Once
	productBinaryPath string
	productBinaryErr  error
)

func productBinary(t *testing.T) string {
	t.Helper()
	productBinaryOnce.Do(func() {
		dir, err := os.MkdirTemp("", "ucupa-realm-product-*")
		if err != nil {
			productBinaryErr = err
			return
		}
		name := "ucupa-realm"
		if os.PathSeparator == '\\' {
			name += ".exe"
		}
		path := filepath.Join(dir, name)
		command := exec.Command("go", "build", "-o", path, ".")
		if output, err := command.CombinedOutput(); err != nil {
			productBinaryErr = errors.New(string(output))
			return
		}
		productBinaryPath = path
	})
	if productBinaryErr != nil {
		t.Fatalf("build product: %v", productBinaryErr)
	}
	return productBinaryPath
}

func runProduct(t *testing.T, args ...string) (string, error) {
	t.Helper()
	command := exec.Command(productBinary(t), args...)
	output, err := command.CombinedOutput()
	return string(output), err
}

// productProcess is one running product serving one persistent Store.
type productProcess struct {
	command *exec.Cmd
	addr    string
	stop    sync.Once
}

// startProduct runs the real product and waits for its ready line, which
// reports the actually bound listener address.
func startProduct(t *testing.T, configPath string) *productProcess {
	t.Helper()
	command := exec.Command(productBinary(t), "-config", configPath)
	command.Stderr = os.Stderr
	stdout, err := command.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	p := &productProcess{command: command}
	ready := make(chan string, 1)
	go func() {
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			fields := strings.Fields(scanner.Text())
			if len(fields) >= 3 && fields[0] == "ucupa-realm:" && fields[1] == "ready" {
				select {
				case ready <- fields[2]:
				default:
				}
			}
		}
	}()
	select {
	case p.addr = <-ready:
	case <-time.After(60 * time.Second):
		p.Kill()
		t.Fatal("product did not become ready")
	}
	t.Cleanup(p.Kill)
	return p
}

// Kill terminates the process hard. Windows cannot deliver a termination
// signal to another process, so the hard kill is also the restart boundary:
// the next start must recover the same persistent Store on its own.
func (p *productProcess) Kill() {
	p.stop.Do(func() {
		if p.command.Process != nil {
			_ = p.command.Process.Kill()
		}
		_ = p.command.Wait()
	})
}

// TestRealmProductProcessServesAndRestoresIdentity proves the independent
// product process lifecycle: trusted bootstrap, exclusive creation, service
// over the real pinned TLS dial, restart with the same Realm identity, and
// run-local handles that do not survive the restart.
func TestRealmProductProcessServesAndRestoresIdentity(t *testing.T) {
	dir := t.TempDir()

	bootstrapPath := writeFile(t, dir, "bootstrap.json", []byte(productBootstrap))
	storePath := filepath.Join(dir, "realm.db")
	output, err := runProduct(t, "-init", bootstrapPath, "-store", storePath)
	if err != nil {
		t.Fatalf("bootstrap: %v\n%s", err, output)
	}
	if !strings.Contains(output, "realm-a") {
		t.Fatalf("bootstrap output hides the created identity: %q", output)
	}
	if _, err := os.Stat(storePath); err != nil {
		t.Fatalf("created Store: %v", err)
	}
	// The trusted bootstrap is exclusive: an existing Store is never re-seeded.
	if output, err := runProduct(t, "-init", bootstrapPath, "-store", storePath); err == nil {
		t.Fatalf("duplicate bootstrap accepted: %q", output)
	}

	serverCA := newTestCA(t, "UCUPA realm product server CA")
	clientCA := newTestCA(t, "UCUPA realm product client CA")
	serverCert, serverSPKI := serverCA.issue(t, "realm-a", true)
	clientCert, clientSPKI := clientCA.issue(t, "admitted", false)
	configPath := writeProductServiceConfig(t, dir, storePath, serverCert, clientCA, clientSPKI)

	first := startProduct(t, configPath)
	client := dialProduct(t, first.addr, serverCA.pool(), serverSPKI, clientCert)
	if realmID, err := client.CheckTarget(context.Background()); err != nil || realmID != "realm-a" {
		t.Fatalf("target check = %v %v", realmID, err)
	}
	caller, view := establishProductCaller(t, client)
	validated, err := client.AuthorizationService().ValidateView(context.Background(), &realmpb.ValidateViewRequest{
		Caller: caller, View: view, Requirements: []*realmpb.Requirement{productRequirement()},
	})
	if err != nil || validated.GetDecision() != realmpb.Decision_VALID {
		t.Fatalf("first run validation = %v %v", validated.GetDecision(), err)
	}
	if err := client.Close(); err != nil {
		t.Fatal(err)
	}
	first.Kill()

	// Restart against the same persistent Store: the identity is kept, every
	// handle from the dead run is invalid, and a fresh caller works again.
	second := startProduct(t, configPath)
	restarted := dialProduct(t, second.addr, serverCA.pool(), serverSPKI, clientCert)
	if realmID, err := restarted.CheckTarget(context.Background()); err != nil || realmID != "realm-a" {
		t.Fatalf("restarted target = %v %v", realmID, err)
	}
	if _, err := restarted.AuthorizationService().ValidateView(context.Background(), &realmpb.ValidateViewRequest{
		Caller: caller, View: view, Requirements: []*realmpb.Requirement{productRequirement()},
	}); err == nil {
		t.Fatal("handle from the previous run survived the restart")
	}
	freshCaller, _ := establishProductCaller(t, restarted)
	if len(freshCaller) != 32 {
		t.Fatalf("fresh caller handle = %d bytes", len(freshCaller))
	}
	if err := restarted.Close(); err != nil {
		t.Fatal(err)
	}
	second.Kill()
}

// TestRealmProductRequiresExistingStore proves a missing Store is an error,
// never an implicit fresh start of an empty Realm.
func TestRealmProductRequiresExistingStore(t *testing.T) {
	dir := t.TempDir()
	serverCA := newTestCA(t, "UCUPA realm product server CA")
	clientCA := newTestCA(t, "UCUPA realm product client CA")
	serverCert, _ := serverCA.issue(t, "realm-a", true)
	_, clientSPKI := clientCA.issue(t, "admitted", false)
	missing := filepath.Join(dir, "absent-realm.db")
	configPath := writeProductServiceConfig(t, dir, missing, serverCert, clientCA, clientSPKI)
	output, err := runProduct(t, "-config", configPath)
	if err == nil {
		t.Fatalf("missing Store served: %q", output)
	}
	if !strings.Contains(output, "cannot open Realm Store") {
		t.Fatalf("missing Store error = %q", output)
	}
}

// TestRealmProductForeignPortIsFailClosedWithoutConfiguration proves the
// independent product refuses foreign identity establishment when no issuer
// trust is configured: challenges are still issued, but no proof is examined
// and no foreign handle exists.
func TestRealmProductForeignPortIsFailClosedWithoutConfiguration(t *testing.T) {
	dir := t.TempDir()
	serverCA := newTestCA(t, "UCUPA realm product server CA")
	clientCA := newTestCA(t, "UCUPA realm product client CA")
	serverCert, serverSPKI := serverCA.issue(t, "realm-a", true)
	clientCert, clientSPKI := clientCA.issue(t, "admitted", false)
	bootstrapPath := writeFile(t, dir, "bootstrap.json", []byte(productBootstrap))
	storePath := filepath.Join(dir, "realm.db")
	if output, err := runProduct(t, "-init", bootstrapPath, "-store", storePath); err != nil {
		t.Fatalf("bootstrap: %v\n%s", err, output)
	}
	configPath := writeProductServiceConfig(t, dir, storePath, serverCert, clientCA, clientSPKI)
	product := startProduct(t, configPath)
	client := dialProduct(t, product.addr, serverCA.pool(), serverSPKI, clientCert)
	defer client.Close()
	if _, err := client.CheckTarget(context.Background()); err != nil {
		t.Fatal(err)
	}
	challenge, err := client.IdentityService().BeginChallenge(context.Background(), &realmpb.BeginChallengeRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.IdentityService().EstablishForeignCaller(context.Background(), &realmpb.EstablishForeignCallerRequest{
		Actor:     proto.String(string(testOwner)),
		Challenge: challenge.GetChallenge(),
		Payload:   make([]byte, 8),
		Signature: make([]byte, 64),
	}); status.Code(err) != codes.Unavailable {
		t.Fatalf("unconfigured foreign identity = %v", err)
	}
}

func writeProductServiceConfig(t *testing.T, dir, storePath string, serverCert tls.Certificate, clientCA *testCA, clientSPKI [32]byte) string {
	t.Helper()
	keyDER, err := x509.MarshalPKCS8PrivateKey(serverCert.PrivateKey)
	if err != nil {
		t.Fatal(err)
	}
	certPath := writeFile(t, dir, "server.pem", pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: serverCert.Certificate[0]}))
	keyPath := writeFile(t, dir, "server.key", pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER}))
	rootsPath := writeFile(t, dir, "roots.pem", pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: clientCA.certificate.Raw}))
	document := map[string]any{
		"realm_store":              storePath,
		"realm_id":                 "realm-a",
		"listen":                   "127.0.0.1:0",
		"next_hop":                 "https://realm-a.invalid:8443",
		"certificate_file":         certPath,
		"private_key_file":         keyPath,
		"client_roots_file":        rootsPath,
		"admitted_services":        []any{map[string]any{"spki_sha256": hex.EncodeToString(clientSPKI[:]), "owner": string(testOwner)}},
		"handle_ttl":               "600s",
		"challenge_ttl":            "600s",
		"rpc_deadline":             "10s",
		"handshake_timeout":        "5s",
		"connection_lifetime":      "60s",
		"shutdown_timeout":         "10s",
		"max_handles":              64,
		"max_handles_per_caller":   8,
		"max_challenges":           16,
		"max_challenges_per_owner": 8,
		"max_requirements":         16,
		"max_connections":          16,
		"max_concurrent_rpcs":      8,
		"max_response_bytes":       1048576,
		"max_streams":              64,
		"max_metadata_bytes":       8192,
	}
	b, err := json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	return writeFile(t, dir, "service.json", b)
}

func dialProduct(t *testing.T, addr string, roots *x509.CertPool, spki [32]byte, clientCert tls.Certificate) *realmsdk.Client {
	t.Helper()
	client, err := realmsdk.New(realmsdk.Config{
		NextHop:          values.NextHop("https://" + addr),
		Expected:         "realm-a",
		Roots:            roots,
		SPKISHA256:       spki,
		Certificate:      &clientCert,
		RPCDeadline:      10 * time.Second,
		ConnectTimeout:   10 * time.Second,
		MaxResponseBytes: 1 << 20,
		MaxMetadataBytes: 8192,
	})
	if err != nil {
		t.Fatal(err)
	}
	return client
}

func establishProductCaller(t *testing.T, client *realmsdk.Client) (caller, view []byte) {
	t.Helper()
	established, err := client.IdentityService().EstablishCaller(context.Background(), &realmpb.EstablishCallerRequest{
		Audience:     proto.String("realm-a"),
		CredentialId: productCredential.ID[:],
		Secret:       productCredential.Secret[:],
	})
	if err != nil {
		t.Fatal(err)
	}
	if established.GetSubjectPrincipal() != "user" {
		t.Fatalf("established principal = %q", established.GetSubjectPrincipal())
	}
	authorized, err := client.AuthorizationService().AuthorizeView(context.Background(), &realmpb.AuthorizeViewRequest{
		Caller:       established.GetCaller(),
		Requirements: []*realmpb.Requirement{productRequirement()},
	})
	if err != nil {
		t.Fatal(err)
	}
	if authorized.GetDecision() != realmpb.Decision_VALID || len(authorized.GetView()) != 32 {
		t.Fatalf("authorization = %v %v", authorized.GetDecision(), authorized.GetView())
	}
	return established.GetCaller(), authorized.GetView()
}
