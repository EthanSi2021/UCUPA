package acceptance_test

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	"ucupa/shared/values"
	pb "ucupa/ucupa-core/bindings/go/protobuf"
	corecontract "ucupa/ucupa-core/contract"
	coresdk "ucupa/ucupa-core/sdk/go"
	realm "ucupa/ucupa-realm/contract"
)

// A and C run as distinct durable Core products on host A. B runs on host B.
// This test never talks to B's Store or an internal federation port.
func TestP6RealHostFederationASide(t *testing.T) {
	runP6RealHostFederationASide(t, "UCUPA_P6_H3", false)
}

func TestP6RealHostFaultASide(t *testing.T) {
	runP6RealHostFederationASide(t, "UCUPA_P6_H4", true)
}

func runP6RealHostFederationASide(t *testing.T, prefix string, fault bool) {
	t.Helper()
	ipText, aOut, cOut, bPath := os.Getenv(prefix+"_A_IP"), os.Getenv(prefix+"_A_MANIFEST_OUT"),
		os.Getenv(prefix+"_C_MANIFEST_OUT"), os.Getenv(prefix+"_B_MANIFEST")
	if ipText == "" && aOut == "" && cOut == "" && bPath == "" {
		t.Skip("opt-in two-host federation A/C side")
	}
	ip := net.ParseIP(ipText)
	if ip == nil || !ip.IsPrivate() || aOut == "" || cOut == "" || bPath == "" || aOut == cOut || aOut == bPath || cOut == bPath {
		t.Fatal("A/C side requires a private bind IP and three separate manifest paths")
	}
	if _, err := os.Stat(bPath); !os.IsNotExist(err) {
		t.Fatalf("stale B manifest must be removed: %s (%v)", bPath, err)
	}
	a := newNetworkFixtureRealm(t, false, "p6-h3-origin")
	c := newNetworkFixtureRealm(t, false, "p6-h3-nested")
	p6FederationEmptyCarrier(t, c)
	aManifest := p6FederationBind(t, a, ip)
	cManifest := p6FederationBind(t, c, ip)
	if aManifest.NextHop == cManifest.NextHop || aManifest.Expected == cManifest.Expected {
		t.Fatal("A and C must be independent owners and listeners")
	}
	a.grant(t, corecontract.PermissionCreateAnchor, values.CoreElementScope(a.config.Expected, networkRoot), false)
	a.grant(t, corecontract.PermissionMountControl, values.CoreElementScope(a.config.Expected, p6FederationMount), false)
	a.grant(t, corecontract.PermissionMountRead, values.CoreElementScope(a.config.Expected, p6FederationMount), false)
	radius, _ := corecontract.NewBoundedSphere(10)
	rotation, _ := corecontract.NewRotation(1, 0, 0, 0)
	if _, err := a.c.Elements().CreateAnchor(a.actor, p6FederationMount, corecontract.SpatialTuple{
		Parent: networkRoot, LocalPose: corecontract.Pose{Rotation: rotation}, Range: radius}, "p6-h3-origin-mount"); err != nil {
		t.Fatal(err)
	}
	// This caller can Query at A but has no B or C grant. A's service key
	// therefore cannot substitute for the original caller at B.
	if _, err := a.r.PutBinding(a.actor, realm.PermissionBinding{Subject: a.facetService,
		Permission: corecontract.PermissionQueryRegion, Scope: values.AnchorClosureScope(a.config.Expected, networkRoot)},
		a.r.BindingSetRevision(), "p6-h3-inert-entry-query"); err != nil {
		t.Fatal(err)
	}
	c.grant(t, corecontract.PermissionMountAccept, values.CoreElementScope(c.config.Expected, p6FederationNested), false)
	c.grant(t, corecontract.PermissionCreateEntity, values.CoreElementScope(c.config.Expected, p6FederationNested), false)
	p6FederationGrantOriginal(t, c, aManifest, values.AnchorClosureScope(c.config.Expected, p6FederationNested))
	issuerKey, issuerPublic := p6FederationIssuer(t, a.dir)
	aManifest.IssuerPublicKey = issuerPublic
	p6FederationExport(t, aOut, aManifest)
	p6FederationExport(t, cOut, cManifest)
	t.Logf("A and C manifests exported to %s and %s; transfer over verified SSH; awaiting B manifest at %s", aOut, cOut, bPath)
	bManifest := p6FederationImport(t, bPath)
	p6RealHostRequireOtherMachine(t, bManifest.NextHop)
	if bManifest.Expected == aManifest.Expected || bManifest.Expected == cManifest.Expected {
		t.Fatal("B shares a Core identity with A or C")
	}
	ab, ac := p6FederationPeer(t, a.dir, bManifest), p6FederationPeer(t, a.dir, cManifest)
	cb := p6FederationPeer(t, c.dir, bManifest)
	// H3 retains multiple paginated reads across the injected outage. Give
	// both owners finite context and byte headroom so local capacity cannot mask it.
	common := map[string]any{"session_ttl": "20m", "connection_lifetime": "20m", "context_ttl": "10m", "observation_ttl": "10m",
		"public_max_bytes": uint64(256 << 20), "public_max_contexts": uint64(512), "public_max_contexts_per_caller": uint64(256)}
	if fault {
		// H4 retains one additional full query while the test bridge is cut;
		// keep that observation from being masked by the fixture's local quota.
		common["public_max_bytes"], common["public_max_contexts"], common["public_max_contexts_per_caller"] =
			uint64(512<<20), uint64(1024), uint64(512)
	}
	aConfig := map[string]any{"mount_connections": []any{ab}, "read_delegation": p6FederationTrust(t, a.dir, bManifest, cManifest),
		"identity_issuer": map[string]any{"signing_key_file": issuerKey, "max_ttl": "10m", "max_proofs": 64, "max_per_caller": 32},
		"federation":      map[string]any{"ttl": "8m", "max_parts": 16, "peers": []any{ab, ac}}}
	if fault {
		// These leases survive across both injected outages. Keep the finite
		// fixture limit above this script's own retained read parts.
		aConfig["federation"] = map[string]any{"ttl": "8m", "max_parts": 64, "peers": []any{ab, ac}}
	}
	cConfig := map[string]any{"core_store": filepath.Join(c.dir, "empty-carrier.db"), "mount_connections": []any{cb},
		"mount_approvals":   map[string]any{"max_ttl": "30s", "max_approvals": 8, "max_per_caller": 6},
		"read_delegation":   p6FederationTrust(t, c.dir, aManifest, bManifest),
		"external_identity": p6FederationExternalIdentity(t, c.dir, aManifest)}
	for key, value := range common {
		aConfig[key], cConfig[key] = value, value
	}
	startA, startC := prepareNetworkProductControl(t, a, aConfig), prepareNetworkProductControl(t, c, cConfig)
	processC, processA := startC(), startA()
	defer processA.stop()
	defer processC.stop()
	a.connect(t)
	bClient := p6FederationClient(t, bManifest)
	p6FederationWaitLogin(t, bClient, bManifest)
	p6FederationRejectWrongRemoteIdentity(t, bManifest)
	request, original := p6FederationMountPublic(t, a.client, bClient, aManifest, bManifest, p6FederationMount, []byte("p6-h3-a-to-b"))
	t.Log("A→B established through public control across the private LAN; waiting for B→C")
	p6FederationWaitNested(t, bClient, bManifest)
	processC.gracefulStop()
	p6FederationSeedContent(t, c, p6FederationSeedEntry{parent: p6FederationNested, element: p6FederationLeaf})
	processC = startC()
	oldQuery := p6FederationQuery(t, a.client, aManifest, bManifest, cManifest)
	oldQueryAt := time.Now()
	p6FederationCheckInertCaller(t, a, aManifest, bManifest, cManifest)
	if fault {
		t.Log("nested public query passed; cut only B's test bridge now")
		p6FederationWaitDisconnected(t, bClient, bManifest, ".bridge_cut")
		cutAt := time.Now()
		p6FederationCheckOffline(t, a.client, oldQuery)
		if time.Since(cutAt) > 20*time.Second {
			t.Fatal("port-scoped connection failure exceeded the bounded public query deadline")
		}
		t.Logf("H4 port-scoped cut returned explicit incomplete coverage in %s", time.Since(cutAt))
		t.Log("B product remained alive behind the cut bridge; restore the bridge now")
		p6FederationWaitReachable(t, bManifest, ".bridge_resume")
		bClient = p6FederationClient(t, bManifest)
		p6FederationWaitLogin(t, bClient, bManifest)
		p6FederationWaitNested(t, bClient, bManifest)
		oldQuery = p6FederationQuery(t, a.client, aManifest, bManifest, cManifest)
		oldQueryAt = time.Now()
	}
	t.Log("nested public Query/List/Read passed; stop B via its gate now")
	p6FederationWaitStopped(t, bClient, bManifest)
	p6FederationCheckOffline(t, a.client, oldQuery)
	t.Log("B stopped: partial coverage confirmed; restart B from its original Core/Realm Stores")
	p6FederationWaitRestarted(t, bManifest)
	if time.Since(oldQueryAt) >= 7*time.Minute {
		t.Fatal("old continuation aged out before source restart was checked; H3 restart evidence is inconclusive")
	}
	p6FederationCheckOldQuery(t, a.client, oldQuery)
	_ = p6FederationQuery(t, a.client, aManifest, bManifest, cManifest)
	processA.gracefulStop()
	processC.gracefulStop()
	processC, processA = startC(), startA()
	_ = a.client.Close()
	a.connect(t)
	p6FederationRecheckMount(t, a.client, request, original)
	_ = p6FederationQuery(t, a.client, aManifest, bManifest, cManifest)
	t.Log("A/C original-Store restart retained both mounted edges and nested public results; mark B gate done")
	processA.gracefulStop()
	processC.gracefulStop()
}

func p6FederationRejectWrongRemoteIdentity(t *testing.T, m p6FederationManifest) {
	t.Helper()
	base := coresdk.Config{NextHop: values.NextHop(m.NextHop), Expected: values.FactAuthorityRef(m.Expected),
		Roots: p6RealHostRoots(t, m.CAPEM), SPKISHA256: p6RealHostPin(t, m.SPKISHA256),
		RPCDeadline: 5 * time.Second, ConnectTimeout: 3 * time.Second, MaxResponseBytes: 8 << 20, MaxMetadataBytes: 8192}
	id, secret := p6RealHostCredential(t, m.CredentialID, m.Secret)
	try := func(cfg coresdk.Config, secret [32]byte) error {
		client, err := coresdk.New(cfg)
		if err != nil {
			return err
		}
		defer client.Close()
		return client.Establish(context.Background(), values.RealmID(m.Realm), id, secret)
	}
	wrong := base
	wrong.SPKISHA256[0] ^= 0xff
	if err := try(wrong, secret); err == nil {
		t.Fatal("wrong cross-machine TLS pin accepted")
	}
	wrong = base
	wrong.Expected = "00000000-0000-4000-8000-00000000ffff"
	if err := try(wrong, secret); err == nil {
		t.Fatal("wrong cross-machine Core authority accepted")
	}
	secret[0] ^= 0xff
	if err := try(base, secret); status.Code(err) != codes.Unauthenticated {
		t.Fatalf("wrong cross-machine caller credential was not rejected: %v", err)
	}
}

func p6FederationWaitNested(t *testing.T, b *coresdk.Client, manifest p6FederationManifest) {
	t.Helper()
	id, secret := p6RealHostCredential(t, manifest.CredentialID, manifest.Secret)
	deadline := time.Now().Add(10 * time.Minute)
	for time.Now().Before(deadline) {
		out, err := b.MountControl().InspectMount(context.Background(), &pb.InspectMountRequest{Element: proto.String(string(p6FederationNested))})
		if err == nil && out.GetResult() != nil && out.GetResult().GetState() == uint32(corecontract.MountActive) {
			return
		}
		// B reopens its original Store after fixture provisioning. Its former
		// process-local session cannot authenticate to the restarted product.
		if status.Code(err) == codes.Unauthenticated {
			if loginErr := b.Establish(context.Background(), values.RealmID(manifest.Realm), id, secret); loginErr != nil &&
				status.Code(loginErr) != codes.Unavailable && status.Code(loginErr) != codes.DeadlineExceeded {
				t.Fatalf("re-establish B session after product restart: %v", loginErr)
			}
		} else if err != nil && status.Code(err) != codes.Unavailable && status.Code(err) != codes.DeadlineExceeded {
			t.Fatalf("inspect B nested mount: %v", err)
		}
		time.Sleep(500 * time.Millisecond)
	}
	t.Fatal("B→C public mount did not become active")
}
