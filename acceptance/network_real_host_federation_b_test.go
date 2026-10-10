package acceptance_test

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"

	"ucupa/shared/values"
	pb "ucupa/ucupa-core/bindings/go/protobuf"
	corecontract "ucupa/ucupa-core/contract"
	coresdk "ucupa/ucupa-core/sdk/go"
)

// B runs alone on host B. The first real mount arrives from A on the other
// machine; B then uses the public product RPCs to mount C back on host A.
func TestP6RealHostFederationBSide(t *testing.T) {
	runP6RealHostFederationBSide(t, "UCUPA_P6_H3", false)
}

func TestP6RealHostFaultBSide(t *testing.T) {
	runP6RealHostFederationBSide(t, "UCUPA_P6_H4", true)
}

func runP6RealHostFederationBSide(t *testing.T, prefix string, fault bool) {
	t.Helper()
	ipText, out, aPath, cPath, gate := os.Getenv(prefix+"_B_IP"), os.Getenv(prefix+"_B_MANIFEST_OUT"),
		os.Getenv(prefix+"_A_MANIFEST"), os.Getenv(prefix+"_C_MANIFEST"), os.Getenv(prefix+"_B_GATE")
	if ipText == "" && out == "" && aPath == "" && cPath == "" && gate == "" {
		t.Skip("opt-in two-host federation B side")
	}
	ip := net.ParseIP(ipText)
	if ip == nil || !ip.IsPrivate() || out == "" || aPath == "" || cPath == "" || gate == "" || out == aPath || out == cPath || aPath == cPath {
		t.Fatal("B side requires a private bind IP, three separate manifest paths and a gate prefix")
	}
	gateSuffixes := []string{".stop", ".stopped", ".start", ".started", ".done",
		".bridge_cut", ".bridge_cut_done", ".bridge_resume", ".bridge_resumed"}
	paths := []string{aPath, cPath, out}
	for _, suffix := range gateSuffixes {
		paths = append(paths, gate+suffix)
	}
	for _, path := range paths {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("stale H3 control path at %s: %v", path, err)
		}
	}
	t.Cleanup(func() {
		for _, suffix := range gateSuffixes {
			_ = os.Remove(gate + suffix)
		}
	})
	b := newNetworkFixtureRealm(t, false, "p6-h3-carrier")
	p6FederationEmptyCarrier(t, b)
	bManifest := p6FederationBind(t, b, ip)
	productListen := strings.TrimPrefix(bManifest.NextHop, "https://")
	var bridge *p6FederationBridge
	if fault {
		bridge = p6FederationStartBridge(t, ip, productListen)
		bManifest.NextHop = bridge.nextHop()
		b.config.NextHop, b.clientConfig.NextHop = values.NextHop(bManifest.NextHop), values.NextHop(bManifest.NextHop)
		defer bridge.close()
	}
	bManifest.GatePrefix = gate
	b.grant(t, corecontract.PermissionMountAccept, values.CoreElementScope(b.config.Expected, p6FederationMount), false)
	b.grant(t, corecontract.PermissionMountRead, values.CoreElementScope(b.config.Expected, p6FederationMount), false)
	b.grant(t, corecontract.PermissionCreateAnchor, values.CoreElementScope(b.config.Expected, p6FederationMount), false)
	b.grant(t, corecontract.PermissionCreateEntity, values.CoreElementScope(b.config.Expected, p6FederationMount), false)
	b.grant(t, corecontract.PermissionMountControl, values.CoreElementScope(b.config.Expected, p6FederationNested), false)
	b.grant(t, corecontract.PermissionMountRead, values.CoreElementScope(b.config.Expected, p6FederationNested), false)
	p6FederationExport(t, out, bManifest)
	t.Logf("B manifest exported to %s; transfer over verified SSH; awaiting A/C manifests", out)
	aManifest, cManifest := p6FederationImport(t, aPath), p6FederationImport(t, cPath)
	p6RealHostRequireOtherMachine(t, aManifest.NextHop)
	p6RealHostRequireOtherMachine(t, cManifest.NextHop)
	if aManifest.NextHop == cManifest.NextHop || aManifest.Expected == cManifest.Expected ||
		aManifest.Expected == bManifest.Expected || cManifest.Expected == bManifest.Expected {
		t.Fatal("the two remote Core identities/listeners and B must be distinct")
	}
	p6FederationGrantOriginal(t, b, aManifest, values.AnchorClosureScope(b.config.Expected, p6FederationMount))
	ba, bc := p6FederationPeer(t, b.dir, aManifest), p6FederationPeer(t, b.dir, cManifest)
	config := map[string]any{"core_store": filepath.Join(b.dir, "empty-carrier.db"),
		"mount_connections": []any{bc, ba}, "mount_approvals": map[string]any{"max_ttl": "30s", "max_approvals": 8, "max_per_caller": 6},
		"read_delegation":   p6FederationTrust(t, b.dir, aManifest, cManifest),
		"external_identity": p6FederationExternalIdentity(t, b.dir, aManifest),
		"session_ttl":       "20m", "connection_lifetime": "20m", "context_ttl": "10m", "observation_ttl": "10m",
		"public_max_bytes": uint64(256 << 20), "public_max_contexts": uint64(512), "public_max_contexts_per_caller": uint64(256)}
	if fault {
		config["listen"] = productListen
		config["public_max_bytes"], config["public_max_contexts"], config["public_max_contexts_per_caller"] =
			uint64(512<<20), uint64(1024), uint64(512)
	}
	startB := prepareNetworkProductControl(t, b, config)
	process := startB()
	defer process.stop()
	b.connect(t)
	p6FederationWaitIncoming(t, b.client)
	process.gracefulStop()
	p6FederationSeedContent(t, b,
		p6FederationSeedEntry{parent: p6FederationMount, element: p6FederationNested, anchor: true},
		p6FederationSeedEntry{parent: p6FederationMount, element: p6FederationExtra})
	process = startB()
	_ = b.client.Close()
	b.connect(t)
	cClient := p6FederationClient(t, cManifest)
	p6FederationWaitLogin(t, cClient, cManifest)
	p6FederationRejectWrongRemoteIdentity(t, cManifest)
	request, original := p6FederationMountPublic(t, b.client, cClient, bManifest, cManifest,
		p6FederationNested, []byte("p6-h3-b-to-c"))
	// C will reopen for disposable content provisioning. Do not keep using a
	// session from the product process that existed before that restart.
	if err := cClient.Close(); err != nil {
		t.Fatal(err)
	}
	t.Log("B→C public mount established over the LAN; await A's nested query, then create gate.stop")
	if fault {
		p6FederationWaitGate(t, gate+".bridge_cut", 15*time.Minute)
		bridge.setCut(true)
		live, err := net.DialTimeout("tcp", productListen, 2*time.Second)
		if err != nil {
			t.Fatalf("H4 bridge cut also stopped the B product listener: %v", err)
		}
		_ = live.Close()
		p6FederationWriteGate(t, gate+".bridge_cut_done")
		p6FederationWaitGate(t, gate+".bridge_resume", 10*time.Minute)
		bridge.setCut(false)
		p6FederationWriteGate(t, gate+".bridge_resumed")
	}
	p6FederationWaitGate(t, gate+".stop", 15*time.Minute)
	if fault {
		process.stop() // abrupt product death; no system-wide network or clock change
	} else {
		process.gracefulStop()
	}
	p6FederationWriteGate(t, gate+".stopped")
	t.Log("B stopped; A must observe explicit incomplete coverage, then create gate.start")
	p6FederationWaitGate(t, gate+".start", 10*time.Minute)
	process = startB() // exact original B Core and Realm Stores
	_ = b.client.Close()
	b.connect(t)
	p6FederationRecheckMount(t, b.client, request, original)
	p6FederationWriteGate(t, gate+".started")
	t.Log("B reopened its original Core/Realm Stores and recovered the exact B→C mount receipt; after A finishes create gate.done")
	p6FederationWaitGate(t, gate+".done", 10*time.Minute)
	process.gracefulStop()
}

func p6FederationWaitIncoming(t *testing.T, b *coresdk.Client) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Minute)
	for time.Now().Before(deadline) {
		out, err := b.MountContact().InspectMountContact(context.Background(), &pb.InspectMountContactRequest{
			Element: proto.String(string(p6FederationMount)), Incoming: proto.Bool(true)})
		if err == nil && out.GetResult() != nil {
			return
		}
		time.Sleep(500 * time.Millisecond)
	}
	t.Fatal("incoming A→B public mount did not become available")
}

func p6FederationWaitGate(t *testing.T, path string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for H3 control gate %s", path)
}

func p6FederationWriteGate(t *testing.T, path string) {
	t.Helper()
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
}
