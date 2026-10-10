package acceptance_test

import (
	"context"
	"crypto/rand"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	"ucupa/composition"
	netpb "ucupa/shared/bindings/go/protobuf"
	"ucupa/shared/values"
	corepb "ucupa/ucupa-core/bindings/go/protobuf"
	corecontract "ucupa/ucupa-core/contract"
	coresdk "ucupa/ucupa-core/sdk/go"
	facetpb "ucupa/ucupa-facet/bindings/go/protobuf"
	facetcontract "ucupa/ucupa-facet/contract"
	facet "ucupa/ucupa-facet/instance"
	facetsdk "ucupa/ucupa-facet/sdk/go"
	realm "ucupa/ucupa-realm/contract"
	realminstance "ucupa/ucupa-realm/instance"
)

// This opt-in pair puts the Core product on one real host and the Facet
// product on a second real host. Both manifests carry disposable credentials:
// transfer them only over an authenticated channel and remove the copies
// afterward. Neither test starts unless its explicit environment is present.
type p6RealHostFacetManifest struct {
	NextHop        string `json:"next_hop"`
	Expected       string `json:"expected_facet"`
	Realm          string `json:"realm"`
	CredentialID   string `json:"credential_id"`
	Secret         string `json:"secret"`
	ObserverID     string `json:"observer_id"`
	ObserverSecret string `json:"observer_secret"`
	CAPEM          string `json:"ca_pem"`
	SPKISHA256     string `json:"spki_sha256"`
}

// p6RealHostRequireOtherMachine rejects loopback, non-private and locally
// assigned endpoints, so a single host cannot impersonate the second machine.
func p6RealHostRequireOtherMachine(t *testing.T, hop string) {
	t.Helper()
	endpoint, err := url.Parse(hop)
	if err != nil {
		t.Fatal(err)
	}
	target := net.ParseIP(endpoint.Hostname())
	if target == nil || !target.IsPrivate() {
		t.Fatalf("real-host endpoint must be a private LAN address: %s", hop)
	}
	addresses, err := net.InterfaceAddrs()
	if err != nil {
		t.Fatal(err)
	}
	for _, address := range addresses {
		if network, ok := address.(*net.IPNet); ok && network.IP.Equal(target) {
			t.Fatalf("endpoint %s is assigned to this host, not a second machine", hop)
		}
	}
}

func p6RealHostAwaitManifest(t *testing.T, path string, limit time.Duration) []byte {
	t.Helper()
	deadline := time.Now().Add(limit)
	for time.Now().Before(deadline) {
		if raw, err := os.ReadFile(path); err == nil && json.Valid(raw) {
			return raw
		}
		time.Sleep(250 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for the peer manifest at %s", path)
	return nil
}

func p6RealHostRoots(t *testing.T, caPEM string) *x509.CertPool {
	t.Helper()
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM([]byte(caPEM)) {
		t.Fatal("invalid peer CA")
	}
	return roots
}

func p6RealHostPin(t *testing.T, text string) [32]byte {
	t.Helper()
	raw, err := base64.RawURLEncoding.DecodeString(text)
	if err != nil || len(raw) != 32 {
		t.Fatal("invalid peer SPKI pin")
	}
	var pin [32]byte
	copy(pin[:], raw)
	return pin
}

func p6RealHostCredential(t *testing.T, idText, secretText string) ([16]byte, [32]byte) {
	t.Helper()
	id, err := base64.RawURLEncoding.DecodeString(idText)
	secret, secretErr := base64.RawURLEncoding.DecodeString(secretText)
	if err != nil || secretErr != nil || len(id) != 16 || len(secret) != 32 {
		t.Fatal("invalid peer credential")
	}
	var outID [16]byte
	var outSecret [32]byte
	copy(outID[:], id)
	copy(outSecret[:], secret)
	return outID, outSecret
}

// TestP6RealHostFacetChainCoreSide runs on the Core host: it provisions the
// temporary Core and Realm Stores, exports the Core manifest, imports the
// Facet host's manifest, starts the Core product bound to a real private
// address and then drives the whole public chain across the two machines.
func TestP6RealHostFacetChainCoreSide(t *testing.T) {
	bindIP, export, peer := os.Getenv("UCUPA_P6_CHAIN_CORE_IP"), os.Getenv("UCUPA_P6_CHAIN_CORE_MANIFEST_OUT"), os.Getenv("UCUPA_P6_CHAIN_FACET_MANIFEST")
	if bindIP == "" && export == "" && peer == "" {
		t.Skip("opt-in real-host Core side of the cross-host Facet chain")
	}
	ip := net.ParseIP(bindIP)
	if ip == nil || !ip.IsPrivate() || export == "" || peer == "" {
		t.Fatal("real-host Core side requires a private bind IP, a manifest export path and a peer manifest path")
	}
	if _, err := os.Stat(peer); !os.IsNotExist(err) {
		t.Fatalf("stale Facet host manifest must be removed before starting: %s (%v)", peer, err)
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
	cert, roots, pin, certPEM, keyPEM := networkCertificateForIPs(t, ip)
	f.certPEM, f.keyPEM = certPEM, keyPEM
	f.config.Certificate, f.config.NextHop = cert, hop
	f.clientConfig.NextHop, f.clientConfig.SPKISHA256, f.clientConfig.Roots = hop, pin, roots
	for _, p := range []values.PermissionID{corecontract.PermissionFacetProposalRead, corecontract.PermissionFacetAttachmentRead, corecontract.PermissionFacetPropose, corecontract.PermissionFacetActivate, corecontract.PermissionFacetContactUpdate} {
		f.grant(t, p, values.CoreElementScope(f.c.AuthorityRef(), networkRoot), false)
	}
	manifest := p6RealHostManifest{NextHop: string(hop), Expected: string(f.config.Expected), Realm: string(f.actor.Realm),
		CredentialID: base64.RawURLEncoding.EncodeToString(f.id[:]), Secret: base64.RawURLEncoding.EncodeToString(f.secret[:]),
		CAPEM:      string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cert.Certificate[1]})),
		SPKISHA256: base64.RawURLEncoding.EncodeToString(pin[:])}
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
		_ = os.Remove(peer)
	})
	t.Logf("Core manifest exported to %s; waiting for the Facet host manifest at %s", export, peer)
	var facet p6RealHostFacetManifest
	if err := json.Unmarshal(p6RealHostAwaitManifest(t, peer, 15*time.Minute), &facet); err != nil {
		t.Fatal(err)
	}
	p6RealHostRequireOtherMachine(t, facet.NextHop)
	if !values.FactAuthorityRef(facet.Expected).Valid() {
		t.Fatal("invalid Facet authority in the peer manifest")
	}
	facetRoots, facetPin := p6RealHostRoots(t, facet.CAPEM), p6RealHostPin(t, facet.SPKISHA256)
	facetID, facetSecret := p6RealHostCredential(t, facet.CredentialID, facet.Secret)
	observerID, observerSecret := p6RealHostCredential(t, facet.ObserverID, facet.ObserverSecret)
	facetOwner := values.FactAuthorityRef(facet.Expected)
	facetCA := filepath.Join(f.dir, "facet-ca.pem")
	writeNetworkFile(t, facetCA, []byte(facet.CAPEM))
	facetCredential, err := json.Marshal(map[string]any{"audience": facet.Realm, "credential_id": facet.CredentialID, "secret": facet.Secret})
	if err != nil {
		t.Fatal(err)
	}
	writeNetworkFile(t, filepath.Join(f.dir, "facet-credential.json"), facetCredential)
	start := prepareNetworkProductControl(t, f, map[string]any{
		"facet_services":    []any{map[string]any{"realm": f.actor.Realm, "principal": f.actor.Principal, "facet": facetOwner}},
		"facet_connections": []any{map[string]any{"next_hop": values.NextHop(facet.NextHop), "expected_facet": facetOwner, "ca_file": facetCA, "spki_sha256": values.EncodePublicBytes(facetPin[:]), "credential_file": filepath.Join(f.dir, "facet-credential.json")}},
		"session_ttl":       "3m", "connection_lifetime": "4m"})
	process := start()
	defer process.stop()
	ctx := context.Background()
	base := facetsdk.Config{NextHop: values.NextHop(facet.NextHop), Expected: facetOwner, Roots: facetRoots, SPKISHA256: facetPin,
		RPCDeadline: 5 * time.Second, ConnectTimeout: 3 * time.Second, MaxResponseBytes: 4 << 20, MaxMetadataBytes: 8192}
	// The Facet product on the other host must be listening before any probe.
	var client *facetsdk.Client
	reached, deadline := false, time.Now().Add(10*time.Minute)
	for !reached && time.Now().Before(deadline) {
		candidate, err := facetsdk.New(base)
		if err != nil {
			t.Fatal(err)
		}
		if err = candidate.Establish(ctx, values.RealmID(facet.Realm), facetID, facetSecret); err == nil {
			client, reached = candidate, true
			break
		}
		_ = candidate.Close()
		if status.Code(err) != codes.Unavailable {
			t.Fatalf("unexpected Facet establishment while waiting for the product: %v", err)
		}
		time.Sleep(500 * time.Millisecond)
	}
	if !reached {
		t.Fatal("Facet product on the other host did not become reachable")
	}
	defer client.Close()
	// The Facet owner rejects the wrong TLS target, wrong expected authority
	// and wrong caller credential on its public plane, all over the LAN.
	bad := base
	bad.SPKISHA256[0] ^= 0xff
	wrongTarget, err := facetsdk.New(bad)
	if err != nil {
		t.Fatal(err)
	}
	if err = wrongTarget.Establish(ctx, values.RealmID(facet.Realm), facetID, facetSecret); err == nil {
		t.Fatal("wrong Facet TLS target accepted")
	}
	_ = wrongTarget.Close()
	bad = base
	bad.Expected = "00000000-0000-4000-8000-00000000ffff"
	wrongAuthority, err := facetsdk.New(bad)
	if err != nil {
		t.Fatal(err)
	}
	if err = wrongAuthority.Establish(ctx, values.RealmID(facet.Realm), facetID, facetSecret); err == nil {
		t.Fatal("wrong expected Facet authority accepted")
	}
	_ = wrongAuthority.Close()
	wrongSecret := facetSecret
	wrongSecret[0] ^= 0xff
	if err := client.Establish(ctx, values.RealmID(facet.Realm), facetID, wrongSecret); status.Code(err) != codes.Unauthenticated {
		t.Fatalf("wrong Facet caller credential accepted: %v", err)
	}
	if err := client.Establish(ctx, values.RealmID(facet.Realm), facetID, facetSecret); err != nil {
		t.Fatal(err)
	}
	// The Core owner verifies caller permissions: the fixture's inert
	// facet-service principal authenticates but holds no Core permission.
	f.connect(t)
	inspect, err := f.client.FacetControl().InspectFacet(ctx, &corepb.InspectFacetRequest{Element: proto.String(string(networkRoot))})
	if err != nil || inspect.GetResult() == nil {
		t.Fatalf("initial Facet inspection: %v %v", inspect, err)
	}
	unauthorized, err := coresdk.New(f.clientConfig)
	if err != nil {
		t.Fatal(err)
	}
	defer unauthorized.Close()
	if err = unauthorized.Establish(ctx, f.actor.Realm, f.facetServiceID, f.facetServiceSecret); err != nil {
		t.Fatalf("inert principal could not authenticate to the Core product: %v", err)
	}
	denied, err := unauthorized.FacetControl().ApplyFacet(ctx, &corepb.ApplyFacetRequest{OperationId: []byte("p6-real-host-facet-service-propose"),
		Command: &corepb.PublicFacetCommand{Kind: proto.String("propose"), Element: proto.String(string(networkRoot)), Expected: inspect.GetResult().Revision,
			Target: proto.String(string(facetOwner)), NextHop: proto.String(facet.NextHop)}})
	if err != nil || denied.GetFailure() != netpb.Failure_DENIED {
		t.Fatalf("Core owner did not deny an unauthorized caller: %v %v", denied, err)
	}
	// Build the attachment itself: propose on the Core, accept on the Facet
	// (whose product verifies the proposal against this host's Core product),
	// activate on the Core (whose product verifies the acceptance against the
	// Facet product), then write and read opaque Payload through the key.
	proposal, err := f.client.FacetControl().ApplyFacet(ctx, &corepb.ApplyFacetRequest{OperationId: []byte("p6-real-host-facet-propose"),
		Command: &corepb.PublicFacetCommand{Kind: proto.String("propose"), Element: proto.String(string(networkRoot)), Expected: inspect.GetResult().Revision,
			Target: proto.String(string(facetOwner)), NextHop: proto.String(facet.NextHop)}})
	if err != nil || proposal.GetResult() == nil {
		t.Fatalf("cross-host proposal: %v %v", proposal, err)
	}
	ref := &facetpb.PublicAcceptanceRef{Core: proto.String(string(f.config.Expected)), Element: proto.String(string(networkRoot)), Proposal: proposal.GetResult().GetState().PendingProposal}
	accept := &facetpb.AcceptAttachmentRequest{Ref: ref, CoreNextHop: proto.String(string(hop)), OperationId: []byte("p6-real-host-facet-accept")}
	accepted, err := client.Relationships().AcceptAttachment(ctx, accept)
	if err != nil || accepted.GetResult() == nil {
		t.Fatalf("cross-host acceptance: %v %v", accepted, err)
	}
	if accepted.GetResult().GetInspection().GetCoreNextHop() != string(hop) {
		t.Fatalf("Facet host recorded the wrong reverse contact: %v", accepted.GetResult().GetInspection().GetCoreNextHop())
	}
	activated, err := f.client.FacetControl().ApplyFacet(ctx, &corepb.ApplyFacetRequest{OperationId: []byte("p6-real-host-facet-activate"),
		Command: &corepb.PublicFacetCommand{Kind: proto.String("activate"), Element: proto.String(string(networkRoot)), Expected: proposal.GetResult().GetState().Revision}})
	if err != nil || activated.GetResult().GetState().GetState() != corepb.AttachmentState_ACTIVE {
		t.Fatalf("cross-host activation: %v %v", activated, err)
	}
	if activated.GetResult().GetState().GetNextHop() != facet.NextHop {
		t.Fatalf("Core recorded the wrong Facet contact: %v", activated.GetResult().GetState().GetNextHop())
	}
	typ := fixtureType("content")
	access := &facetpb.Access{NextHop: proto.String(facet.NextHop), Key: &netpb.AttachmentKey{Core: ref.Core, Facet: proto.String(string(facetOwner)), Element: ref.Element, Proposal: ref.Proposal}}
	content := []byte(`"cross-host"`)
	put := &facetpb.PutRequest{Access: access, TypeRef: typ.Bytes(), Expected: &facetpb.Expected{Condition: &facetpb.Expected_Absent{Absent: true}}, OperationId: []byte("p6-real-host-facet-put"), Payload: content}
	written, err := client.Facet().Put(ctx, put)
	if err != nil || written.GetResult() == nil {
		t.Fatalf("cross-host content write: %v %v", written, err)
	}
	read, err := client.Facet().Get(ctx, &facetpb.GetRequest{Access: access, TypeRef: typ.Bytes()})
	if err != nil || read.GetResult() == nil || string(read.GetResult().Payload) != string(content) {
		t.Fatalf("cross-host content read: %v %v", read, err)
	}
	// The Facet owner also verifies caller permissions: the manifest's inert
	// observer principal authenticates but holds no Facet permission.
	observer, err := facetsdk.New(base)
	if err != nil {
		t.Fatal(err)
	}
	defer observer.Close()
	if err = observer.Establish(ctx, values.RealmID(facet.Realm), observerID, observerSecret); err != nil {
		t.Fatalf("inert observer could not authenticate to the Facet product: %v", err)
	}
	refused, err := observer.Facet().Put(ctx, &facetpb.PutRequest{Access: access, TypeRef: typ.Bytes(), Expected: &facetpb.Expected{Condition: &facetpb.Expected_Absent{Absent: true}}, OperationId: []byte("p6-real-host-observer-put"), Payload: content})
	if err != nil || refused.GetFailure() != netpb.Failure_DENIED {
		t.Fatalf("Facet owner did not deny an unauthorized caller: %v %v", refused, err)
	}
	// Reverse contact check: the Facet's record of the Core address is
	// locally repairable with idempotent retries while the Core stays up.
	contact := accepted.GetResult().GetInspection().ContactRevision
	for i, address := range []string{"https://127.0.0.1:1", string(hop)} {
		request := &facetpb.UpdateCoreContactRequest{Ref: ref, CoreNextHop: proto.String(address), OperationId: []byte(fmt.Sprintf("p6-real-host-repair-core-%d", i)), Expected: contact}
		updated, err := client.Relationships().UpdateCoreContact(ctx, request)
		if err != nil || updated.GetResult().GetInspection().GetCoreNextHop() != address {
			t.Fatalf("repair Core contact %d: %v %v", i, updated, err)
		}
		retry, err := client.Relationships().UpdateCoreContact(ctx, request)
		if err != nil || !proto.Equal(updated, retry) {
			t.Fatalf("repair Core contact retry %d: %v %v", i, retry, err)
		}
		contact = updated.GetResult().GetInspection().ContactRevision
	}
	afterRepair, err := client.Facet().Get(ctx, &facetpb.GetRequest{Access: access, TypeRef: typ.Bytes()})
	if err != nil || afterRepair.GetResult() == nil || string(afterRepair.GetResult().Payload) != string(content) {
		t.Fatalf("content after contact repair: %v %v", afterRepair, err)
	}
	t.Logf("cross-host attachment established and content verified; stop the Facet product on its host to begin the disconnect exercise")
	// The Facet product now gets stopped on the other host. A stopped peer
	// must surface as an explicit error, never as an empty or fake success.
	probe, err := facetsdk.New(base)
	if err != nil {
		t.Fatal(err)
	}
	defer probe.Close()
	loggedIn, stopped, limit := false, false, time.Now().Add(10*time.Minute)
	for time.Now().Before(limit) && !stopped {
		if !loggedIn {
			if err = probe.Establish(ctx, values.RealmID(facet.Realm), facetID, facetSecret); err == nil {
				loggedIn = true
			} else if status.Code(err) == codes.Unavailable {
				stopped = true
			} else {
				t.Fatalf("unexpected Facet establishment while awaiting the stop: %v", err)
			}
			continue
		}
		if _, err = probe.Facet().Get(ctx, &facetpb.GetRequest{Access: access, TypeRef: typ.Bytes()}); err == nil {
			time.Sleep(500 * time.Millisecond)
			continue
		}
		switch status.Code(err) {
		case codes.Unavailable:
			stopped = true
		case codes.Unauthenticated:
			loggedIn = false
		default:
			t.Fatalf("unexpected Facet probe error while awaiting the stop: %v", err)
		}
	}
	if !stopped {
		t.Fatal("Facet product on the other host did not become unavailable")
	}
	if _, err = probe.Facet().Get(ctx, &facetpb.GetRequest{Access: access, TypeRef: typ.Bytes()}); err == nil || status.Code(err) != codes.Unavailable {
		t.Fatalf("stopped Facet product must fail explicitly, not masquerade as empty or success: %v", err)
	}
	// While the Facet process is gone, the Core side keeps its durable
	// relationship and can still repair its record of the Facet address.
	f.client.Close()
	f.connect(t)
	stoppedInspection, err := f.client.FacetControl().InspectFacet(ctx, &corepb.InspectFacetRequest{Element: proto.String(string(networkRoot))})
	if err != nil || stoppedInspection.GetResult().GetState() != corepb.AttachmentState_ACTIVE {
		t.Fatalf("relationship while the Facet product is stopped: %v %v", stoppedInspection, err)
	}
	if stoppedInspection.GetResult().GetNextHop() != facet.NextHop {
		t.Fatalf("Core lost its Facet contact while the peer was stopped: %v", stoppedInspection.GetResult().GetNextHop())
	}
	contact = stoppedInspection.GetResult().ContactRevision
	for i, address := range []string{"https://127.0.0.1:1", facet.NextHop} {
		request := &corepb.ApplyFacetRequest{OperationId: []byte(fmt.Sprintf("p6-real-host-repair-facet-%d", i)),
			Command: &corepb.PublicFacetCommand{Kind: proto.String("contact"), Element: proto.String(string(networkRoot)), Expected: contact, NextHop: proto.String(address)}}
		updated, err := f.client.FacetControl().ApplyFacet(ctx, request)
		if err != nil || updated.GetResult().GetState().GetNextHop() != address {
			t.Fatalf("repair Facet contact while stopped %d: %v %v", i, updated, err)
		}
		retry, err := f.client.FacetControl().ApplyFacet(ctx, request)
		if err != nil || !proto.Equal(updated, retry) {
			t.Fatalf("repair Facet contact retry %d: %v %v", i, retry, err)
		}
		contact = updated.GetResult().GetState().ContactRevision
	}
	t.Logf("Facet product confirmed stopped; restart it on its host from its original Stores to finish the exercise")
	// After the other host reopens the same original Stores, the content,
	// the original receipts and the acceptance must be exactly unchanged.
	client.Close()
	recovered, deadlineUp := false, time.Now().Add(10*time.Minute)
	for !recovered && time.Now().Before(deadlineUp) {
		candidate, err := facetsdk.New(base)
		if err != nil {
			t.Fatal(err)
		}
		if err = candidate.Establish(ctx, values.RealmID(facet.Realm), facetID, facetSecret); err == nil {
			client, recovered = candidate, true
			break
		}
		_ = candidate.Close()
		if status.Code(err) != codes.Unavailable {
			t.Fatalf("unexpected Facet establishment while awaiting the restart: %v", err)
		}
		time.Sleep(500 * time.Millisecond)
	}
	if !recovered {
		t.Fatal("Facet product on the other host did not come back on its original Stores")
	}
	reread, err := client.Facet().Get(ctx, &facetpb.GetRequest{Access: access, TypeRef: typ.Bytes()})
	if err != nil || reread.GetResult() == nil || string(reread.GetResult().Payload) != string(content) {
		t.Fatalf("content after the original-Store restart: %v %v", reread, err)
	}
	recoveredPut, err := client.Facet().RecoverOutcome(ctx, &facetpb.RecoverOutcomeRequest{Access: access, OperationId: put.OperationId,
		Request: &facetpb.MutationCommand{Command: &facetpb.MutationCommand_Put{Put: put}}})
	if err != nil || !proto.Equal(written.GetResult(), recoveredPut.GetResult()) {
		t.Fatalf("original OperationID receipt after the original-Store restart: %v %v", recoveredPut, err)
	}
	replayed, err := client.Relationships().AcceptAttachment(ctx, accept)
	if err != nil || !proto.Equal(accepted, replayed) {
		t.Fatalf("acceptance replay after the original-Store restart: %v %v", replayed, err)
	}
	f.client.Close()
	f.connect(t)
	finalInspection, err := f.client.FacetControl().InspectFacet(ctx, &corepb.InspectFacetRequest{Element: proto.String(string(networkRoot))})
	if err != nil || finalInspection.GetResult().GetState() != corepb.AttachmentState_ACTIVE {
		t.Fatalf("relationship after the peer restart: %v %v", finalInspection, err)
	}
	process.gracefulStop()
	t.Logf("P6 real-host Facet chain verified: two-machine attachment, per-owner caller checks, content, reverse contacts, stop and original-Store recovery at %s", facet.NextHop)
}

// TestP6RealHostFacetChainFacetSide runs on the Facet host: it provisions the
// temporary Facet and Realm Stores, exports the Facet manifest, imports the
// Core host's manifest, starts the Facet product bound to a real private
// address, rejects wrong TLS targets/authorities/credentials against the
// remote Core product, and drives its product through stop and original-Store
// restart gates.
func TestP6RealHostFacetChainFacetSide(t *testing.T) {
	bindIP, export, peer, gate := os.Getenv("UCUPA_P6_CHAIN_FACET_IP"), os.Getenv("UCUPA_P6_CHAIN_FACET_MANIFEST_OUT"), os.Getenv("UCUPA_P6_CHAIN_CORE_MANIFEST"), os.Getenv("UCUPA_P6_CHAIN_FACET_GATE")
	if bindIP == "" && export == "" && peer == "" && gate == "" {
		t.Skip("opt-in real-host Facet side of the cross-host Facet chain")
	}
	ip := net.ParseIP(bindIP)
	if ip == nil || !ip.IsPrivate() || export == "" || peer == "" || gate == "" {
		t.Fatal("real-host Facet side requires a private bind IP, a manifest export path, a peer manifest path and a gate prefix")
	}
	for _, path := range []string{export, peer, gate + ".stop", gate + ".stopped", gate + ".start", gate + ".started", gate + ".done"} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("stale control path must be removed before starting: %s (%v)", path, err)
		}
	}
	t.Cleanup(func() {
		_ = os.Remove(export)
		_ = os.Remove(peer)
		for _, suffix := range []string{".stop", ".stopped", ".start", ".started", ".done"} {
			_ = os.Remove(gate + suffix)
		}
	})
	dir := t.TempDir()
	ctx := context.Background()
	actor := values.PrincipalRef{Realm: "facet-relation", Principal: "operator"}
	observer := values.PrincipalRef{Realm: actor.Realm, Principal: "observer"}
	credential := realm.CredentialEvidence{Audience: actor.Realm}
	observerCredential := realm.CredentialEvidence{Audience: actor.Realm}
	if _, err := rand.Read(credential.ID[:]); err != nil {
		t.Fatal(err)
	}
	if _, err := rand.Read(credential.Secret[:]); err != nil {
		t.Fatal(err)
	}
	if _, err := rand.Read(observerCredential.ID[:]); err != nil {
		t.Fatal(err)
	}
	if _, err := rand.Read(observerCredential.Secret[:]); err != nil {
		t.Fatal(err)
	}
	r, err := realminstance.CreateDurable(realm.Seed{ID: actor.Realm, LocalPrincipals: []values.PrincipalID{actor.Principal, observer.Principal},
		Credentials: []realm.CredentialBootstrap{{Subject: actor.Principal, Evidence: credential}, {Subject: observer.Principal, Evidence: observerCredential}},
		Bindings:    []realm.PermissionBinding{{Subject: actor, Permission: realm.PermissionPutBinding, Scope: values.RealmScope(actor.Realm)}}}, filepath.Join(dir, "realm.db"))
	if err != nil {
		t.Fatal(err)
	}
	// No relation or content exists yet, so this provisioning adapter is
	// never consulted; the Facet product supplies the real network Core
	// source from its own configuration when it opens the same Store.
	caller, err := r.EstablishCaller(credential)
	if err != nil {
		t.Fatal(err)
	}
	links := &composition.LocalFacetCoreConnections{}
	source, err := links.ForFacetCaller(caller)
	if err != nil {
		t.Fatal(err)
	}
	realmLinks := &composition.LocalRealmConnections{}
	if err = realmLinks.BindEndpoint(values.NextHop("local:"+string(actor.Realm)), r); err != nil {
		t.Fatal(err)
	}
	instance, err := facet.CreateDurable(realmLinks, source, facetcontract.RealmInitialization{Realm: actor.Realm, NextHop: values.NextHop("local:" + string(actor.Realm))}, filepath.Join(dir, "facet.db"), facet.Config{PublicAccessPolicy: publicPolicy()})
	if err != nil {
		t.Fatal(err)
	}
	owner := instance.AuthorityRef()
	typ := fixtureType("content")
	for _, grant := range []struct {
		permission values.PermissionID
		scope      values.OperationScope
		operation  string
	}{{facetcontract.PermissionAccept, values.FacetElementScope(owner, networkRoot), "p6-real-host-accept"},
		{facetcontract.PermissionReadAcceptance, values.FacetElementScope(owner, networkRoot), "p6-real-host-read-acceptance"},
		{facetcontract.PermissionCoreContactUpdate, values.FacetElementScope(owner, networkRoot), "p6-real-host-contact-update"},
		{facetcontract.PermissionGet, values.FacetKeyScope(owner, networkRoot, typ), "p6-real-host-get"},
		{facetcontract.PermissionPut, values.FacetKeyScope(owner, networkRoot, typ), "p6-real-host-put"}} {
		if _, err = r.PutBinding(actor, realm.PermissionBinding{Subject: actor, Permission: grant.permission, Scope: grant.scope}, r.BindingSetRevision(), realm.OperationID(grant.operation)); err != nil {
			t.Fatal(err)
		}
	}
	l, err := net.Listen("tcp", net.JoinHostPort(ip.String(), "0"))
	if err != nil {
		t.Fatal(err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	if err := l.Close(); err != nil {
		t.Fatal(err)
	}
	hop := values.NextHop("https://" + net.JoinHostPort(ip.String(), strconv.Itoa(port)))
	certificate, _, pin, certPEM, keyPEM := networkCertificateForIPs(t, ip)
	writeNetworkFile(t, filepath.Join(dir, "tls.pem"), certPEM)
	writeNetworkFile(t, filepath.Join(dir, "tls.key"), keyPEM)
	manifest := p6RealHostFacetManifest{NextHop: string(hop), Expected: string(owner), Realm: string(actor.Realm),
		CredentialID:   base64.RawURLEncoding.EncodeToString(credential.ID[:]),
		Secret:         base64.RawURLEncoding.EncodeToString(credential.Secret[:]),
		ObserverID:     base64.RawURLEncoding.EncodeToString(observerCredential.ID[:]),
		ObserverSecret: base64.RawURLEncoding.EncodeToString(observerCredential.Secret[:]),
		CAPEM:          string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certificate.Certificate[1]})),
		SPKISHA256:     base64.RawURLEncoding.EncodeToString(pin[:])}
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
	if err := instance.Close(); err != nil {
		t.Fatal(err)
	}
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}
	t.Logf("Facet manifest exported to %s; waiting for the Core host manifest at %s", export, peer)
	var core p6RealHostManifest
	if err := json.Unmarshal(p6RealHostAwaitManifest(t, peer, 15*time.Minute), &core); err != nil {
		t.Fatal(err)
	}
	p6RealHostRequireOtherMachine(t, core.NextHop)
	if !values.FactAuthorityRef(core.Expected).Valid() {
		t.Fatal("invalid Core authority in the peer manifest")
	}
	coreRoots, corePin := p6RealHostRoots(t, core.CAPEM), p6RealHostPin(t, core.SPKISHA256)
	coreID, coreSecret := p6RealHostCredential(t, core.CredentialID, core.Secret)
	writeNetworkFile(t, filepath.Join(dir, "core-ca.pem"), []byte(core.CAPEM))
	coreCredential, err := json.Marshal(map[string]any{"audience": core.Realm, "credential_id": core.CredentialID, "secret": core.Secret})
	if err != nil {
		t.Fatal(err)
	}
	writeNetworkFile(t, filepath.Join(dir, "core-credential.json"), coreCredential)
	configuration := map[string]any{"listen": net.JoinHostPort(ip.String(), strconv.Itoa(port)), "next_hop": hop, "expected_facet": owner, "realm_id": actor.Realm, "realm_next_hop": values.NextHop("local:" + string(actor.Realm)),
		"facet_store": filepath.Join(dir, "facet.db"), "realm_store": filepath.Join(dir, "realm.db"),
		"certificate_file": filepath.Join(dir, "tls.pem"), "private_key_file": filepath.Join(dir, "tls.key"),
		"session_ttl": "3m", "rpc_deadline": "4s", "handshake_timeout": "1s", "connection_lifetime": "4m", "shutdown_timeout": "1s",
		"max_sessions": 16, "max_sessions_per_caller": 8, "max_connections": 16, "max_concurrent_rpcs": 8, "max_streams": 8, "max_metadata_bytes": 8192, "max_response_bytes": 4 << 20,
		"context_ttl": "1m", "observation_ttl": "1m",
		"core_services":    []any{map[string]any{"realm": actor.Realm, "principal": actor.Principal, "core": values.FactAuthorityRef(core.Expected)}},
		"core_connections": []any{map[string]any{"next_hop": values.NextHop(core.NextHop), "expected_core": values.FactAuthorityRef(core.Expected), "ca_file": filepath.Join(dir, "core-ca.pem"), "spki_sha256": values.EncodePublicBytes(corePin[:]), "credential_file": filepath.Join(dir, "core-credential.json")}}}
	startFacet := prepareServiceProductControl(t, dir, "ucupa-facet", configuration)
	process := startFacet()
	// The Core product on the other host must be listening before the
	// negative probes: this host's caller authenticates to it over the LAN.
	coreConfig := coresdk.Config{NextHop: values.NextHop(core.NextHop), Expected: values.FactAuthorityRef(core.Expected), Roots: coreRoots, SPKISHA256: corePin,
		RPCDeadline: 5 * time.Second, ConnectTimeout: 3 * time.Second, MaxResponseBytes: 8 << 20, MaxMetadataBytes: 8192}
	reached, deadline := false, time.Now().Add(15*time.Minute)
	for !reached && time.Now().Before(deadline) {
		candidate, err := coresdk.New(coreConfig)
		if err != nil {
			t.Fatal(err)
		}
		if err = candidate.Establish(ctx, values.RealmID(core.Realm), coreID, coreSecret); err == nil {
			_ = candidate.Close()
			reached = true
			break
		}
		_ = candidate.Close()
		if status.Code(err) != codes.Unavailable {
			t.Fatalf("unexpected Core establishment while waiting for the product: %v", err)
		}
		time.Sleep(500 * time.Millisecond)
	}
	if !reached {
		t.Fatal("Core product on the other host did not become reachable")
	}
	// The Core owner rejects the wrong TLS target, wrong expected authority
	// and wrong caller credential from this second machine.
	bad := coreConfig
	bad.SPKISHA256[0] ^= 0xff
	wrongTarget, err := coresdk.New(bad)
	if err != nil {
		t.Fatal(err)
	}
	if err = wrongTarget.Establish(ctx, values.RealmID(core.Realm), coreID, coreSecret); err == nil {
		t.Fatal("wrong Core TLS target accepted")
	}
	_ = wrongTarget.Close()
	bad = coreConfig
	bad.Expected = "00000000-0000-4000-8000-00000000ffff"
	wrongAuthority, err := coresdk.New(bad)
	if err != nil {
		t.Fatal(err)
	}
	if err = wrongAuthority.Establish(ctx, values.RealmID(core.Realm), coreID, coreSecret); err == nil {
		t.Fatal("wrong expected Core authority accepted")
	}
	_ = wrongAuthority.Close()
	wrongSecret := coreSecret
	wrongSecret[0] ^= 0xff
	wrongCredential, err := coresdk.New(coreConfig)
	if err != nil {
		t.Fatal(err)
	}
	if err = wrongCredential.Establish(ctx, values.RealmID(core.Realm), coreID, wrongSecret); status.Code(err) != codes.Unauthenticated {
		t.Fatalf("wrong Core caller credential accepted: %v", err)
	}
	_ = wrongCredential.Close()
	t.Logf("Facet product ready at %s and remote Core entry verified from this host; drive the chain from the Core host, then use the gate files to stop and restart", hop)
	stopped, started := false, false
	limit := time.Now().Add(30 * time.Minute)
	for time.Now().Before(limit) {
		if !stopped {
			if _, err := os.Stat(gate + ".stop"); err == nil {
				process.gracefulStop()
				stopped = true
				if err := os.WriteFile(gate+".stopped", []byte("stopped"), 0600); err != nil {
					t.Fatal(err)
				}
				t.Log("Facet product stopped on its original Stores")
			}
		} else if !started {
			if _, err := os.Stat(gate + ".start"); err == nil {
				process = startFacet()
				started = true
				if err := os.WriteFile(gate+".started", []byte("started"), 0600); err != nil {
					t.Fatal(err)
				}
				t.Log("Facet product restarted on its original Stores")
			}
		}
		if _, err := os.Stat(gate + ".done"); err == nil {
			if !stopped || !started {
				t.Fatal("real-host Facet exercise ended without the original-Store stop and restart")
			}
			process.gracefulStop()
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatal("real-host Facet exercise timed out")
}
