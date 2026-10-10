package acceptance_test

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	"ucupa/composition"
	networkpb "ucupa/shared/bindings/go/protobuf"
	"ucupa/shared/values"
	corepb "ucupa/ucupa-core/bindings/go/protobuf"
	corecontract "ucupa/ucupa-core/contract"
	core "ucupa/ucupa-core/instance"
	coresdk "ucupa/ucupa-core/sdk/go"
	realmpb "ucupa/ucupa-realm/bindings/go/protobuf"
	realm "ucupa/ucupa-realm/contract"
	realminstance "ucupa/ucupa-realm/instance"
	realmsdk "ucupa/ucupa-realm/sdk/go"
)

const networkContactRoot values.ElementID = "00000000-0000-4000-8000-000000000041"
const networkContactForeignUser values.FactAuthorityRef = "00000000-0000-4000-8000-000000000042"
const networkContactChild = "00000000-0000-4000-8000-000000000043"
const networkContactDeadline values.ElementID = "00000000-0000-4000-8000-000000000044"

// networkContactTuple is this fixture's anchor shape: the same bounded sphere
// as the shared networkTuple, but parented by this fixture's own root.
func networkContactTuple(radius float64) *corepb.SpatialTuple {
	return &corepb.SpatialTuple{Parent: proto.String(string(networkContactRoot)), Pose: &corepb.Pose{Translation: &networkpb.Vector3{Items: []float64{0, 0, 0}}, Rotation: &corepb.Rotation{Pivot: proto.Uint32(4), Ratios: &networkpb.Vector3{Items: []float64{0, 0, 0}}}}, Range: &corepb.Range{Kind: corepb.RangeKind_BOUNDED_SPHERE.Enum(), Radius: proto.Float64(radius)}}
}

// networkRealmContactFixture provisions one durable Realm and one durable Core
// whose persisted contact is a real network Realm address. All three TLS
// planes are real: the Realm's serving certificate, the Core's mTLS service
// identity admitted by that Realm, and the Core's user-facing certificate.
// The Store is provisioned through the trusted local Realm port — the only
// assembly that can create roots, since the network port authorizes no
// principal — and its contact then moves to the network address through the
// real maintenance update; every product launch afterwards proves that address
// online over real gRPC.
type networkRealmContactFixture struct {
	dir            string
	realmHost      string
	coreHost       string
	realm          values.RealmID
	actor          values.PrincipalRef
	id             [16]byte
	secret         [32]byte
	admin          realm.CredentialEvidence
	owner          values.FactAuthorityRef
	realmAddress   values.NextHop
	coreAddress    values.NextHop
	realmCA        *x509.Certificate
	realmRoots     *x509.CertPool
	realmSPKI      [32]byte
	realmCert      tls.Certificate
	realmCertPEM   []byte
	realmKeyPEM    []byte
	serviceSPKI    [32]byte
	serviceCert    tls.Certificate
	serviceCertPEM []byte
	serviceKeyPEM  []byte
	serviceCAPEM   []byte
	serviceRoots   *x509.CertPool
	userRoots      *x509.CertPool
	userSPKI       [32]byte
	signingKey     ed25519.PrivateKey
	startRealm     func() *serviceProductProcess
	startCore      func() *serviceProductProcess
	clientConfig   coresdk.Config
	serial         int
	materialOnce   sync.Once
}

// newNetworkRealmContactFixture provisions both durable Stores with the Core's
// contact bound to a real network Realm address, then closes every in-process
// owner. The realm and Core product starters remain for the tests to launch
// actual product processes against the persisted bytes.
func newNetworkRealmContactFixture(t *testing.T) *networkRealmContactFixture {
	t.Helper()
	f := &networkRealmContactFixture{dir: t.TempDir(), realm: "contact-realm",
		actor: values.PrincipalRef{Realm: "contact-realm", Principal: "operator"}}
	f.realmHost, f.coreHost = filepath.Join(f.dir, "realm-host"), filepath.Join(f.dir, "core-host")
	if err := os.MkdirAll(f.realmHost, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(f.coreHost, 0o700); err != nil {
		t.Fatal(err)
	}
	_, _ = rand.Read(f.id[:])
	_, _ = rand.Read(f.secret[:])
	f.admin.Audience = f.actor.Realm
	_, _ = rand.Read(f.admin.ID[:])
	_, _ = rand.Read(f.admin.Secret[:])
	_, signing, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	f.signingKey = signing

	// The Realm's serving material is shared by every Realm address, so the
	// same CA answers the Core's pinned roots.
	realmCA, realmCAKey := realmForeignCA(t)
	f.realmCA = realmCA
	f.realmRoots = x509.NewCertPool()
	f.realmRoots.AddCert(realmCA)
	var realmCertPEM, realmKeyPEM string
	f.realmCert, f.realmSPKI, realmCertPEM, realmKeyPEM = realmForeignLeaf(t, realmCA, realmCAKey, "realm-contact")
	f.realmCertPEM, f.realmKeyPEM = []byte(realmCertPEM), []byte(realmKeyPEM)

	// The Core service identity: a client leaf under its own CA, admitted by
	// the Realm for exactly this Core's owner.
	serviceCA, serviceCAKey := realmForeignCA(t)
	var serviceCertPEM, serviceKeyPEM string
	_, f.serviceSPKI, serviceCertPEM, serviceKeyPEM = realmForeignLeaf(t, serviceCA, serviceCAKey, "core-service")
	f.serviceCertPEM, f.serviceKeyPEM = []byte(serviceCertPEM), []byte(serviceKeyPEM)
	f.serviceCAPEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: serviceCA.Raw})
	f.serviceRoots = x509.NewCertPool()
	f.serviceRoots.AddCert(serviceCA)
	if f.serviceCert, err = tls.X509KeyPair(f.serviceCertPEM, f.serviceKeyPEM); err != nil {
		t.Fatal(err)
	}

	policy, err := realminstance.CreateDurable(realm.Seed{ID: f.actor.Realm, LocalPrincipals: []values.PrincipalID{f.actor.Principal},
		Bindings: []realm.PermissionBinding{
			{Subject: f.actor, Permission: realm.PermissionPutBinding, Scope: values.RealmScope(f.actor.Realm)},
			{Subject: f.actor, Permission: realm.PermissionDeleteBinding, Scope: values.RealmScope(f.actor.Realm)},
			{Subject: f.actor, Permission: realm.PermissionRevokeCredential, Scope: values.RealmScope(f.actor.Realm)},
		}, Credentials: []realm.CredentialBootstrap{
			{Subject: f.actor.Principal, Evidence: realm.CredentialEvidence{Audience: f.actor.Realm, ID: realm.CredentialID(f.id), Secret: realm.CredentialSecret(f.secret)}},
			{Subject: f.actor.Principal, Evidence: f.admin}}}, filepath.Join(f.dir, "realm.db"))
	if err != nil {
		t.Fatal(err)
	}

	// Roots exist only through trusted local assembly, and the network Realm
	// port authorizes no principal: the Store is created against the local
	// Realm port and its persisted contact then moves to the pinned network
	// address through the real maintenance update. The new address is first
	// proven online when a product process opens the Store.
	var owner *core.Core
	provisioned := false
	// A failed setup must still release the durable Store handles before the
	// test's TempDir cleanup runs, or the locked files cannot be removed.
	t.Cleanup(func() {
		if provisioned {
			return
		}
		if owner != nil {
			_ = owner.Close()
		}
		_ = policy.Close()
	})
	localAddress := values.NextHop("local:contact")
	local := &composition.LocalRealmConnections{}
	if err = local.BindEndpoint(localAddress, policy); err != nil {
		t.Fatal(err)
	}
	realmListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	f.realmAddress = values.NextHop("https://" + realmListener.Addr().String())
	_ = realmListener.Close()
	owner, err = core.CreateDurable(local, corecontract.RealmInitialization{Realm: f.actor.Realm, NextHop: localAddress},
		filepath.Join(f.dir, "core.db"), corecontract.Config{ExpectedRealm: f.actor.Realm,
			PublicAccessPolicy: &values.PublicAccessPolicy{ContextTTL: time.Minute, ObservationTTL: time.Minute}})
	if err != nil {
		t.Fatal(err)
	}
	f.owner = owner.AuthorityRef()
	bind := func(permission values.PermissionID, scope values.OperationScope) {
		f.serial++
		operation := realm.OperationID(fmt.Sprintf("binding-contact-%d", f.serial))
		if _, err := policy.PutBinding(f.actor, realm.PermissionBinding{Subject: f.actor, Permission: permission, Scope: scope},
			policy.BindingSetRevision(), operation); err != nil {
			t.Fatal(err)
		}
	}
	// The maintenance guard requires the owner's authority scope; every public
	// intent authorizes through anchor closure over the root boundary.
	bind(corecontract.PermissionRealmContactRead, values.AuthorityScope(f.owner))
	bind(corecontract.PermissionRealmContactUpdate, values.AuthorityScope(f.owner))
	bind(corecontract.PermissionCreateRoot, values.CoreElementScope(f.owner, networkContactRoot))
	for _, permission := range []values.PermissionID{corecontract.PermissionReadElement, corecontract.PermissionListDirect,
		corecontract.PermissionQueryRegion, corecontract.PermissionCreateAnchor, corecontract.PermissionCreateEntity,
		corecontract.PermissionUpdateElement, corecontract.PermissionPlaceElement, corecontract.PermissionDeleteAnchor,
		corecontract.PermissionDeleteEntity, corecontract.PermissionObserveElement, corecontract.PermissionObserveAttachment} {
		bind(permission, values.AnchorClosureScope(f.owner, networkContactRoot))
	}
	if _, err = owner.Elements().CreateRoot(f.actor, networkContactRoot, "root"); err != nil {
		t.Fatal(err)
	}
	maintainer, err := local.TrustLocalCaller(localAddress, f.actor.Realm, f.actor)
	if err != nil {
		t.Fatal(err)
	}
	contact, ok := owner.CurrentRealmContact()
	if !ok {
		t.Fatal("durable Realm contact is missing")
	}
	outcome, err := owner.RealmContact().Update(context.Background(), maintainer, corecontract.RealmContactUpdate{
		OperationID: corecontract.OperationID("contact-to-network"), Owner: f.owner, Realm: f.actor.Realm,
		Expected: contact.Revision, NextHop: f.realmAddress})
	if err != nil || outcome.Decision != corecontract.RealmContactCommitted {
		t.Fatalf("contact update: %v %v", outcome, err)
	}
	if err = owner.Close(); err != nil {
		t.Fatal(err)
	}
	if err = policy.Close(); err != nil {
		t.Fatal(err)
	}
	provisioned = true

	// One user-facing certificate for the Core product and its SDK clients;
	// its CA answers the Realm's proof-verifier pinning.
	userCert, userRoots, userPin, userCertPEM, userKeyPEM := networkCertificate(t)
	f.userRoots, f.userSPKI = userRoots, userPin
	coreListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	f.coreAddress = values.NextHop("https://" + coreListener.Addr().String())
	_ = coreListener.Close()
	writeNetworkFile(t, filepath.Join(f.dir, "user-cert.pem"), userCertPEM)
	writeNetworkFile(t, filepath.Join(f.dir, "user-key.pem"), userKeyPEM)
	writeNetworkFile(t, filepath.Join(f.dir, "user-ca.pem"),
		pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: userCert.Certificate[len(userCert.Certificate)-1]}))
	f.clientConfig = coresdk.Config{Expected: f.owner, NextHop: f.coreAddress, Roots: userRoots, SPKISHA256: userPin,
		RPCDeadline: 5 * time.Second, ConnectTimeout: time.Second, MaxResponseBytes: 8 << 20, MaxMetadataBytes: 8192}

	f.startRealm = prepareServiceProductControl(t, f.realmHost, "ucupa-realm", f.realmProductConfig())
	f.startCore = prepareServiceProductControl(t, f.coreHost, "ucupa-core", f.coreProductConfig(f.realmAddress))
	return f
}

// grant binds or removes one permission through the in-process Realm kernel
// while the product processes are stopped.
func (f *networkRealmContactFixture) grant(permission values.PermissionID, scope values.OperationScope, remove bool) error {
	policy, err := realminstance.OpenDurable(filepath.Join(f.dir, "realm.db"))
	if err != nil {
		return err
	}
	binding := realm.PermissionBinding{Subject: f.actor, Permission: permission, Scope: scope}
	f.serial++
	operation := realm.OperationID(fmt.Sprintf("binding-contact-%d", f.serial))
	if remove {
		_, err = policy.DeleteBinding(f.actor, binding, policy.BindingSetRevision(), operation)
	} else {
		_, err = policy.PutBinding(f.actor, binding, policy.BindingSetRevision(), operation)
	}
	return errors.Join(err, policy.Close())
}

// writeMaterials stores the Realm serving and Core service certificate files
// exactly once for both product configurations.
func (f *networkRealmContactFixture) writeMaterials() {
	f.materialOnce.Do(func() {
		for _, material := range []struct {
			name string
			data []byte
		}{
			{"realm-cert.pem", f.realmCertPEM},
			{"realm-key.pem", f.realmKeyPEM},
			{"realm-ca.pem", pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: f.realmCA.Raw})},
			{"service-roots.pem", f.serviceCAPEM},
			{"core-service.pem", f.serviceCertPEM},
			{"core-service.key", f.serviceKeyPEM},
		} {
			if err := os.WriteFile(filepath.Join(f.dir, material.name), material.data, 0600); err != nil {
				panic(err)
			}
		}
	})
}

// realmProductConfig returns the complete Realm product configuration for the
// provisioned Store. The Core's service identity is the only admitted caller
// and is bound to exactly the Core's owner.
func (f *networkRealmContactFixture) realmProductConfig() map[string]any {
	f.writeMaterials()
	return map[string]any{
		"realm_store": filepath.Join(f.dir, "realm.db"), "realm_id": string(f.actor.Realm),
		"listen": addressHostPort(f.realmAddress), "next_hop": string(f.realmAddress),
		"certificate_file": filepath.Join(f.dir, "realm-cert.pem"), "private_key_file": filepath.Join(f.dir, "realm-key.pem"),
		"client_roots_file": filepath.Join(f.dir, "service-roots.pem"),
		"admitted_services": []any{map[string]any{"spki_sha256": hex.EncodeToString(f.serviceSPKI[:]), "owner": string(f.owner)}},
		"handle_ttl":        "600s", "challenge_ttl": "600s", "rpc_deadline": "10s", "handshake_timeout": "5s",
		"connection_lifetime": "60s", "shutdown_timeout": "10s",
		// Caller and retained-read evidence share this finite budget. Completed
		// one-shot requests release their views; the resource-pressure test
		// exceeds this budget within one TTL without relying on expiry.
		"max_handles": 64, "max_handles_per_caller": 64, "max_challenges": 64, "max_challenges_per_owner": 16,
		"max_requirements": 64, "max_connections": 16, "max_concurrent_rpcs": 16,
		"max_response_bytes": 1048576, "max_streams": 64, "max_metadata_bytes": 8192,
	}
}

// coreProductConfig returns the complete Core product configuration with the
// explicit network Realm profile: pinned endpoints only, no local Realm Store.
func (f *networkRealmContactFixture) coreProductConfig(addresses ...values.NextHop) map[string]any {
	endpoints := make([]any, 0, len(addresses))
	for _, address := range addresses {
		endpoints = append(endpoints, map[string]any{
			"next_hop": string(address), "ca_file": filepath.Join(f.dir, "realm-ca.pem"),
			"spki_sha256":              values.EncodePublicBytes(f.realmSPKI[:]),
			"service_certificate_file": filepath.Join(f.dir, "core-service.pem"), "service_private_key_file": filepath.Join(f.dir, "core-service.key"),
		})
	}
	return map[string]any{
		"listen": addressHostPort(f.coreAddress), "next_hop": string(f.coreAddress), "expected_core": string(f.owner),
		"realm_id": string(f.actor.Realm), "realm_profile": "network", "realm_endpoints": endpoints,
		"core_store":       filepath.Join(f.dir, "core.db"),
		"certificate_file": filepath.Join(f.dir, "user-cert.pem"), "private_key_file": filepath.Join(f.dir, "user-key.pem"),
		"session_ttl": "30s", "rpc_deadline": "5s", "handshake_timeout": "1s", "connection_lifetime": "1m", "shutdown_timeout": "1s",
		"max_sessions": 16, "max_sessions_per_caller": 8, "max_connections": 16, "max_concurrent_rpcs": 8,
		"max_streams": 8, "max_metadata_bytes": 8192, "max_response_bytes": 8 << 20,
		"context_ttl": "1m", "observation_ttl": "1m",
	}
}

// relaunchRealm stops any running Realm process, rewrites its configuration
// for the given address and starts a fresh product process.
func (f *networkRealmContactFixture) relaunchRealm(t *testing.T, previous *serviceProductProcess, address values.NextHop) *serviceProductProcess {
	t.Helper()
	if previous != nil {
		previous.gracefulStop()
	}
	configuration := f.realmProductConfig()
	configuration["listen"] = addressHostPort(address)
	configuration["next_hop"] = string(address)
	writeServiceConfig(t, f.realmHost, "ucupa-realm", configuration)
	return f.startRealm()
}

// relaunchCore stops any running Core process, rewrites its configuration
// for the given pinned Realm endpoints and starts a fresh product process.
func (f *networkRealmContactFixture) relaunchCore(t *testing.T, previous *serviceProductProcess, addresses ...values.NextHop) *serviceProductProcess {
	t.Helper()
	if previous != nil {
		previous.gracefulStop()
	}
	writeServiceConfig(t, f.coreHost, "ucupa-core", f.coreProductConfig(addresses...))
	return f.startCore()
}

func (f *networkRealmContactFixture) connect(t *testing.T) *coresdk.Client {
	t.Helper()
	client, err := coresdk.New(f.clientConfig)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	if err = client.Establish(context.Background(), f.actor.Realm, f.id, f.secret); err != nil {
		t.Fatal(err)
	}
	return client
}

// connectReady establishes a session right after the serving Realm restarted:
// the Core product's long-lived service connection may still sit in its
// reconnect backoff window, so authentication retries until the Realm actually
// answers. Only the transient unavailability is retried; a real denial stands.
func (f *networkRealmContactFixture) connectReady(t *testing.T) *coresdk.Client {
	t.Helper()
	client, err := coresdk.New(f.clientConfig)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	deadline := time.Now().Add(10 * time.Second)
	for {
		err = client.Establish(context.Background(), f.actor.Realm, f.id, f.secret)
		if err == nil || status.Code(err) != codes.Unavailable || !time.Now().Before(deadline) {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if err != nil {
		t.Fatal(err)
	}
	return client
}

func (f *networkRealmContactFixture) reference() *networkpb.Reference {
	return &networkpb.Reference{Boundary: &networkpb.Boundary{Core: proto.String(string(f.owner)), Anchor: proto.String(string(networkContactRoot))}, NextHop: proto.String(string(f.coreAddress))}
}

func (f *networkRealmContactFixture) enter(t *testing.T, client *coresdk.Client, intent corepb.Intent) *networkpb.Token {
	t.Helper()
	out, err := client.Core().Enter(context.Background(), &corepb.EnterRequest{Reference: f.reference(), Intent: intent.Enum()})
	if err != nil || out.GetResult() == nil {
		t.Fatalf("Enter: %v %v", out, err)
	}
	return out.GetResult().Handle
}

// spareAddress reserves one more local port for a Realm relaunch.
func (f *networkRealmContactFixture) spareAddress(t *testing.T) values.NextHop {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := values.NextHop("https://" + l.Addr().String())
	_ = l.Close()
	return address
}

// writeServiceConfig rewrites one product's service.json in place; the
// prepared starter reads the latest file on every launch.
func writeServiceConfig(t *testing.T, dir, program string, configuration map[string]any) {
	t.Helper()
	raw, err := json.Marshal(configuration)
	if err != nil {
		t.Fatal(err)
	}
	writeNetworkFile(t, filepath.Join(dir, "service.json"), raw)
}

// rejectServiceProductStartup proves one product configuration refuses to
// serve: the process exits instead of announcing readiness.
func rejectServiceProductStartup(t *testing.T, dir, program string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		program += ".exe"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, filepath.Join(dir, program), "-config", filepath.Join(dir, "service.json")).CombinedOutput()
	if err == nil || ctx.Err() != nil {
		t.Fatalf("misbound startup did not fail closed: %v %s", err, out)
	}
}

func addressHostPort(address values.NextHop) string {
	return string(address[len("https://"):])
}

// invertPin derives one wrong but well-formed SPKI pin.
func invertPin(pin [32]byte) []byte {
	wrong := pin
	wrong[0] ^= 0xff
	return wrong[:]
}

// TestNetworkRealmContactProductChain proves the explicit network profile end
// to end with only real product processes: a Realm process and a Core process
// serve independent durable Stores over real gRPC and TLS, every authorization
// and session derivation reaches the Realm through the Core's mTLS service
// identity, results survive Core restarts through the recovered contact, a
// Realm restart invalidates its run-local handles without opening the
// authorization to a stale handle, an offline Realm fails closed with no local
// pass, a revoked permission denies online, and a client deadline bounds the
// wait without a commit.
func TestNetworkRealmContactProductChain(t *testing.T) {
	f := newNetworkRealmContactFixture(t)
	realmProcess := f.startRealm()
	coreProcess := f.startCore()
	client := f.connect(t)
	ctx := context.Background()

	// The full authorization chain runs through the network Realm: Enter, a
	// write, reads, a query and one observation.
	write, err := client.Core().CreateAnchor(ctx, &corepb.CreateAnchorRequest{Handle: f.enter(t, client, corepb.Intent_CREATE_ANCHOR),
		Element: proto.String(networkContactChild), OperationId: []byte("chain-write"), Final: networkContactTuple(10)})
	if err != nil || write.GetResult() == nil {
		t.Fatalf("create anchor: %v %v", write, err)
	}
	list, err := client.Core().ListDirect(ctx, &corepb.ListDirectRequest{Handle: f.enter(t, client, corepb.Intent_LIST), Address: &networkpb.Address{}})
	if err != nil || len(list.GetResult().GetRecords()) == 0 {
		t.Fatalf("list direct: %v %v", list, err)
	}
	read, err := client.Core().ReadElement(ctx, &corepb.ReadElementRequest{Handle: f.enter(t, client, corepb.Intent_READ),
		Address: list.GetResult().GetRecords()[0].GetAddress(), PrepareObserve: proto.Bool(true)})
	if err != nil || read.GetResult().GetRecord() == nil {
		t.Fatalf("read element: %v %v", read, err)
	}
	if outcome, queryErr := client.Core().QueryRegion(ctx, &corepb.QueryRegionRequest{Handle: f.enter(t, client, corepb.Intent_QUERY), Start: &networkpb.Address{}, Basis: &networkpb.Address{},
		Sphere: &corepb.Sphere{Center: &networkpb.Vector3{Items: []float64{0, 0, 0}}, Radius: proto.Float64(20)}, UpDepth: proto.Int64(-1)}); queryErr != nil || outcome.GetResult() == nil {
		t.Fatalf("query region: %v %v", outcome, queryErr)
	}
	if outcome, observeErr := client.Core().BeginObserve(ctx, &corepb.BeginObserveRequest{Preparation: read.GetResult().Preparation}); observeErr != nil || outcome.GetResult() == nil {
		t.Fatalf("begin observe: %v %v", outcome, observeErr)
	}

	// A Core product restart reopens the same Store, verifies the persisted
	// contact online and discloses the original result.
	coreProcess = f.relaunchCore(t, coreProcess, f.realmAddress)
	client = f.connect(t)
	recovered, err := client.Core().RecoverOutcome(ctx, &corepb.RecoverOutcomeRequest{Reference: f.reference(), OperationId: []byte("chain-write"),
		Request: &corepb.MutationCommand{Command: &corepb.MutationCommand_CreateAnchor{CreateAnchor: &corepb.CreateAnchorCommand{
			Element: proto.String(networkContactChild), OperationId: []byte("chain-write"), Final: networkContactTuple(10)}}}})
	if err != nil || !proto.Equal(write.GetResult(), recovered.GetResult()) {
		t.Fatalf("recovered outcome: %v %v", recovered, err)
	}

	// A Realm restart clears its run-local handles: derivation over the old
	// Core session fails closed, never silently passes, and a fresh session
	// through the live Core re-establishes normally.
	stale := f.connect(t)
	realmProcess = f.relaunchRealm(t, realmProcess, f.realmAddress)
	// Denials arrive as response failures, not RPC errors: crossing the
	// restart means a real entry result came back.
	if outcome, enterErr := stale.Core().Enter(ctx, &corepb.EnterRequest{Reference: f.reference(), Intent: corepb.Intent_READ.Enum()}); enterErr == nil && outcome.GetResult() != nil {
		t.Fatal("stale session crossed a Realm restart")
	}
	if outcome, enterErr := client.Core().Enter(ctx, &corepb.EnterRequest{Reference: f.reference(), Intent: corepb.Intent_READ.Enum()}); enterErr == nil && outcome.GetResult() != nil {
		t.Fatal("second stale session crossed a Realm restart")
	}
	fresh := f.connectReady(t)
	handle := f.enter(t, fresh, corepb.Intent_READ)
	if outcome, readErr := fresh.Core().ReadElement(ctx, &corepb.ReadElementRequest{Handle: handle, Address: &networkpb.Address{}, PrepareObserve: proto.Bool(false)}); readErr != nil || outcome.GetResult().GetRecord() == nil {
		t.Fatalf("fresh read after Realm restart: %v %v", outcome, readErr)
	}

	// While the Realm process is down nothing authorizes offline, no fallback
	// source appears and no new session can be derived; once it returns, the
	// old session stays dead while a fresh session serves the same read.
	realmProcess.gracefulStop()
	offline, err := fresh.Core().ReadElement(ctx, &corepb.ReadElementRequest{Handle: handle, Address: &networkpb.Address{}, PrepareObserve: proto.Bool(false)})
	if offline.GetResult() != nil {
		t.Fatalf("offline Realm passed: %v %v", offline, err)
	}
	offlineClient, err := coresdk.New(f.clientConfig)
	if err != nil {
		t.Fatal(err)
	}
	if err = offlineClient.Establish(ctx, f.actor.Realm, f.id, f.secret); err == nil {
		t.Fatal("session derived while the Realm was offline")
	}
	_ = offlineClient.Close()
	realmProcess = f.relaunchRealm(t, realmProcess, f.realmAddress)
	if outcome, readErr := fresh.Core().ReadElement(ctx, &corepb.ReadElementRequest{Handle: handle, Address: &networkpb.Address{}, PrepareObserve: proto.Bool(false)}); readErr == nil && outcome.GetResult() != nil {
		t.Fatal("stale session crossed the second Realm restart")
	}
	resumed := f.connectReady(t)
	resumedHandle := f.enter(t, resumed, corepb.Intent_READ)
	if outcome, readErr := resumed.Core().ReadElement(ctx, &corepb.ReadElementRequest{Handle: resumedHandle, Address: &networkpb.Address{}, PrepareObserve: proto.Bool(false)}); readErr != nil || outcome.GetResult().GetRecord() == nil {
		t.Fatalf("online read did not resume: %v %v", outcome, readErr)
	}

	// A revoked permission denies through the live Realm: after the restart
	// only a fresh session can reach it, and its READ entry is denied online;
	// restoring the permission lets the next fresh session read again, proving
	// the denial was an online authorization decision.
	realmProcess.gracefulStop()
	if err = f.grant(corecontract.PermissionReadElement, values.AnchorClosureScope(f.owner, networkContactRoot), true); err != nil {
		t.Fatal(err)
	}
	realmProcess = f.relaunchRealm(t, realmProcess, f.realmAddress)
	denied := f.connectReady(t)
	if outcome, enterErr := denied.Core().Enter(ctx, &corepb.EnterRequest{Reference: f.reference(), Intent: corepb.Intent_READ.Enum()}); enterErr == nil && outcome.GetResult() != nil {
		t.Fatal("revoked permission passed")
	}
	realmProcess.gracefulStop()
	if err = f.grant(corecontract.PermissionReadElement, values.AnchorClosureScope(f.owner, networkContactRoot), false); err != nil {
		t.Fatal(err)
	}
	realmProcess = f.relaunchRealm(t, realmProcess, f.realmAddress)
	restored := f.connectReady(t)
	restoredHandle := f.enter(t, restored, corepb.Intent_READ)
	if outcome, readErr := restored.Core().ReadElement(ctx, &corepb.ReadElementRequest{Handle: restoredHandle, Address: &networkpb.Address{}, PrepareObserve: proto.Bool(false)}); readErr != nil || outcome.GetResult().GetRecord() == nil {
		t.Fatalf("restored permission denied: %v %v", outcome, readErr)
	}

	// A client deadline bounds the waiting without a commit: an already
	// expired deadline fails the write deterministically before it reaches
	// the Core, the operation id discloses no fabricated outcome, and the
	// same id then commits on retry.
	deadlined, cancelDeadlined := context.WithDeadline(ctx, time.Now().Add(-time.Second))
	defer cancelDeadlined()
	final := f.connect(t)
	if _, err = final.Core().CreateAnchor(deadlined, &corepb.CreateAnchorRequest{Handle: f.enter(t, final, corepb.Intent_CREATE_ANCHOR),
		Element: proto.String(string(networkContactDeadline)), OperationId: []byte("chain-deadline"), Final: networkContactTuple(10)}); status.Code(err) != codes.DeadlineExceeded {
		t.Fatalf("client deadline = %v", err)
	}
	if outcome, recoverErr := final.Core().RecoverOutcome(ctx, &corepb.RecoverOutcomeRequest{Reference: f.reference(), OperationId: []byte("chain-deadline"),
		Request: &corepb.MutationCommand{Command: &corepb.MutationCommand_CreateAnchor{CreateAnchor: &corepb.CreateAnchorCommand{
			Element: proto.String(string(networkContactDeadline)), OperationId: []byte("chain-deadline"), Final: networkContactTuple(10)}}}}); recoverErr == nil && outcome.GetResult() != nil {
		t.Fatal("deadline abort disclosed a fabricated outcome")
	}
	retried, err := final.Core().CreateAnchor(ctx, &corepb.CreateAnchorRequest{Handle: f.enter(t, final, corepb.Intent_CREATE_ANCHOR),
		Element: proto.String(string(networkContactDeadline)), OperationId: []byte("chain-deadline"), Final: networkContactTuple(10)})
	if err != nil || retried.GetResult() == nil {
		t.Fatalf("retry after deadline: %v %v", retried, err)
	}
	_ = coreProcess
}

// TestNetworkRealmContactProductMisbinding proves the assembly fail closed on
// wrong pins: a wrong endpoint SPKI or a wrong expected Realm refuse to start
// the Core product, and a Realm that does not admit the Core's service
// identity serves nothing to this Core's sessions.
func TestNetworkRealmContactProductMisbinding(t *testing.T) {
	f := newNetworkRealmContactFixture(t)
	// The correctly configured Realm product must be live: every refusal
	// below must come from a wrong pin or identity, never a dead endpoint.
	realmProcess := f.startRealm()

	// A tampered endpoint SPKI never reaches authorization: the pinned target
	// check fails during open and the product refuses to serve.
	wrongPin := f.coreProductConfig(f.realmAddress)
	wrongPin["realm_endpoints"].([]any)[0].(map[string]any)["spki_sha256"] = values.EncodePublicBytes(invertPin(f.realmSPKI))
	writeServiceConfig(t, f.coreHost, "ucupa-core", wrongPin)
	rejectServiceProductStartup(t, f.coreHost, "ucupa-core")

	// A wrong expected Realm rejects the persisted contact at open.
	wrongRealm := f.coreProductConfig(f.realmAddress)
	wrongRealm["realm_id"] = "other-realm"
	writeServiceConfig(t, f.coreHost, "ucupa-core", wrongRealm)
	rejectServiceProductStartup(t, f.coreHost, "ucupa-core")

	// A Realm that does not admit the Core's service identity still starts,
	// and the Core product opens its Store, but session derivation fails
	// closed: nothing falls back open.
	unadmitted := f.realmProductConfig()
	unadmitted["admitted_services"] = []any{map[string]any{"spki_sha256": hex.EncodeToString(invertPin(f.serviceSPKI)), "owner": string(f.owner)}}
	writeServiceConfig(t, f.realmHost, "ucupa-realm", unadmitted)
	realmProcess.gracefulStop()
	realmProcess = f.startRealm()
	writeServiceConfig(t, f.coreHost, "ucupa-core", f.coreProductConfig(f.realmAddress))
	coreProcess := f.startCore()
	client, err := coresdk.New(f.clientConfig)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	if err = client.Establish(context.Background(), f.actor.Realm, f.id, f.secret); err == nil {
		t.Fatal("session established through an unadmitted service identity")
	}
	realmProcess.gracefulStop()
	coreProcess.gracefulStop()
}

// TestNetworkRealmContactProductIdentityChain proves the P6 source-side
// identity over the network profile: the Core product's identity issuer
// signs proofs with its caller source on the pinned Realm contact, the Realm
// product verifies the proof and checks its liveness online at that Core, the
// established subject receives only the seeded authorization, and a proof
// naming another owner stays rejected.
func TestNetworkRealmContactProductIdentityChain(t *testing.T) {
	f := newNetworkRealmContactFixture(t)

	// The issuer Core product: its signing key and the delegation admission
	// for the Realm's serving identity as verifier.
	signingDER, err := x509.MarshalPKCS8PrivateKey(f.signingKey)
	if err != nil {
		t.Fatal(err)
	}
	writeNetworkFile(t, filepath.Join(f.dir, "identity-signing.key"), pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: signingDER}))
	signingPub, err := x509.MarshalPKIXPublicKey(f.signingKey.Public())
	if err != nil {
		t.Fatal(err)
	}
	writeNetworkFile(t, filepath.Join(f.dir, "identity-signing.pub"), pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: signingPub}))
	coreConfiguration := f.coreProductConfig(f.realmAddress)
	coreConfiguration["identity_issuer"] = map[string]any{
		"signing_key_file": filepath.Join(f.dir, "identity-signing.key"), "max_ttl": "30s", "max_proofs": 8, "max_per_caller": 6,
	}
	coreConfiguration["read_delegation"] = map[string]any{
		"client_ca_file":     filepath.Join(f.dir, "realm-ca.pem"),
		"client_spki_sha256": map[string]string{string(f.owner): values.EncodePublicBytes(f.realmSPKI[:])},
		"max_ttl":            "10s", "max_grants": 4, "max_per_caller": 3,
	}

	// The Realm product trusts exactly this issuer and verifier.
	realmConfiguration := f.realmProductConfig()
	realmConfiguration["foreign"] = map[string]any{
		"max_proof_ttl": "30s",
		"issuers": map[string]any{string(f.actor.Realm): map[string]any{
			"signing_public_key_file": filepath.Join(f.dir, "identity-signing.pub"),
			"next_hop":                string(f.coreAddress), "expected_core": string(f.owner),
			"ca_file": filepath.Join(f.dir, "user-ca.pem"), "spki_sha256": hex.EncodeToString(f.userSPKI[:]),
			"verifier": string(f.owner),
		}},
	}
	writeServiceConfig(t, f.realmHost, "ucupa-realm", realmConfiguration)
	writeServiceConfig(t, f.coreHost, "ucupa-core", coreConfiguration)
	realmProcess := f.startRealm()
	coreProcess := f.startCore()
	issuerClient := f.connect(t)

	// The test presents the Core's admitted service identity to the Realm.
	realmConn, err := realmsdk.New(realmsdk.Config{NextHop: f.realmAddress, Expected: f.actor.Realm,
		Roots: f.realmRoots, SPKISHA256: f.realmSPKI, Certificate: &f.serviceCert,
		RPCDeadline: 10 * time.Second, ConnectTimeout: 10 * time.Second, MaxResponseBytes: 1 << 20, MaxMetadataBytes: 8192})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = realmConn.Close() })
	ctx := context.Background()
	if realmID, err := realmConn.CheckTarget(ctx); err != nil || realmID != f.actor.Realm {
		t.Fatalf("target check = %v %v", realmID, err)
	}

	establish := func(actor values.FactAuthorityRef) (*realmpb.EstablishForeignCallerResponse, error) {
		challenge, err := realmConn.IdentityService().BeginChallenge(ctx, &realmpb.BeginChallengeRequest{})
		if err != nil {
			t.Fatal(err)
		}
		proof, err := issuerClient.IdentityProofs().IssueIdentityProof(ctx, &networkpb.IssueIdentityProofRequest{
			Actor: proto.String(string(actor)), Challenge: challenge.GetChallenge(), TtlSeconds: proto.Uint64(25),
		})
		if err != nil {
			return nil, err
		}
		return realmConn.IdentityService().EstablishForeignCaller(ctx, &realmpb.EstablishForeignCallerRequest{
			Actor: proto.String(string(f.owner)), Challenge: challenge.GetChallenge(), Payload: proof.GetPayload(), Signature: proof.GetSignature(),
		})
	}

	// A correctly bound proof establishes the issuer's session subject and the
	// caller receives only the seeded authorization.
	established, err := establish(f.owner)
	if err != nil {
		t.Fatal(err)
	}
	if established.GetSubjectRealm() != string(f.actor.Realm) || established.GetSubjectPrincipal() != string(f.actor.Principal) {
		t.Fatalf("foreign subject = %v/%v", established.GetSubjectRealm(), established.GetSubjectPrincipal())
	}
	requirement := &realmpb.Requirement{Permission: proto.String(string(corecontract.PermissionReadElement)), Scope: &realmpb.Scope{
		Kind: realmpb.ScopeKind_ANCHOR_CLOSURE.Enum(), Authority: proto.String(string(f.owner)), Element: proto.String(string(networkContactRoot))}}
	authorized, err := realmConn.AuthorizationService().AuthorizeView(ctx, &realmpb.AuthorizeViewRequest{Caller: established.GetCaller(), Requirements: []*realmpb.Requirement{requirement}})
	if err != nil || authorized.GetDecision() != realmpb.Decision_VALID {
		t.Fatalf("foreign authorization = %v %v", authorized.GetDecision(), err)
	}

	// A proof naming another owner is rejected before any identity exists.
	if _, err = establish(networkContactForeignUser); status.Code(err) != codes.Unauthenticated {
		t.Fatalf("misbound proof accepted: %v", err)
	}

	// While the issuer Core is down the foreign port fails closed; once the
	// issuer returns it no longer knows the old proof.
	challenge, err := realmConn.IdentityService().BeginChallenge(ctx, &realmpb.BeginChallengeRequest{})
	if err != nil {
		t.Fatal(err)
	}
	proof, err := issuerClient.IdentityProofs().IssueIdentityProof(ctx, &networkpb.IssueIdentityProofRequest{
		Actor: proto.String(string(f.owner)), Challenge: challenge.GetChallenge(), TtlSeconds: proto.Uint64(25)})
	if err != nil {
		t.Fatal(err)
	}
	coreProcess.gracefulStop()
	if _, err = realmConn.IdentityService().EstablishForeignCaller(ctx, &realmpb.EstablishForeignCallerRequest{
		Actor: proto.String(string(f.owner)), Challenge: challenge.GetChallenge(), Payload: proof.GetPayload(), Signature: proof.GetSignature()}); status.Code(err) != codes.Unavailable {
		t.Fatalf("offline issuer accepted: %v", err)
	}
	coreProcess = f.startCore()
	// The realm's pinned issuer liveness client may still sit in its reconnect
	// backoff window when the issuer returns; keep presenting the pre-restart
	// proof until the check reaches the revived issuer. The revived issuer no
	// longer knows that proof, so the reached check must deny it, never accept.
	var revived error
	reviveDeadline := time.Now().Add(10 * time.Second)
	for {
		_, revived = realmConn.IdentityService().EstablishForeignCaller(ctx, &realmpb.EstablishForeignCallerRequest{
			Actor: proto.String(string(f.owner)), Challenge: challenge.GetChallenge(), Payload: proof.GetPayload(), Signature: proof.GetSignature()})
		if status.Code(revived) != codes.Unavailable || !time.Now().Before(reviveDeadline) {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if status.Code(revived) != codes.Unauthenticated {
		t.Fatalf("revived proof accepted across issuer restart: %v", revived)
	}
	_ = realmProcess
}
