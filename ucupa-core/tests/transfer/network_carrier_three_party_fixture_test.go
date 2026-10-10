package transfer_test

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"net"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"ucupa/composition"
	netpb "ucupa/shared/bindings/go/protobuf"
	"ucupa/shared/values"
	pb "ucupa/ucupa-core/bindings/go/protobuf"
	"ucupa/ucupa-core/contract"
	core "ucupa/ucupa-core/instance"
	coresdk "ucupa/ucupa-core/sdk/go"
	coregrpc "ucupa/ucupa-core/transport/grpc"
	"ucupa/ucupa-core/transport/grpc/transitionconn"
)

// nestedCarrierMount is one source-owned mount boundary inside the carrier
// content, used to prove the uncoordinated external boundary is refused.
const nestedCarrierMount values.ElementID = "00000000-0000-4000-8000-000000000050"

// carrierThreePartyFixture stands up three independent gRPC hosts (parent,
// source, target) connected only by authenticated TLS. All carrier control,
// fence, acceptance, installation and settlement cross real network boundaries.
type carrierThreePartyFixture struct {
	*carrierFixture
	parentHost, sourceHost, targetHost       *coregrpc.Server
	parentConfig, sourceConfig, targetConfig coregrpc.Config
	control                                  *coresdk.Client // parent user session → parent host
	sourceUser                               *coresdk.Client // source approver session → source host
	targetUser                               *coresdk.Client // target approver session → target host
	w                                        *pb.CarrierApprovalBinding
	parentPool, sourcePool, targetPool       *x509.CertPool
	parentPin, sourcePin, targetPin          [32]byte
	parentCert, sourceCert, targetCert       tls.Certificate
	parentAddr, sourceAddr, targetAddr       values.NextHop
}

// newCarrierThreeParty builds the three hosts with the parent's outgoing
// carrier contact already repaired to the network source address.
func newCarrierThreeParty(t *testing.T) *carrierThreePartyFixture {
	t.Helper()
	f := newCarrierThreePartyFixture(t, true)
	return f
}

// newCarrierThreePartyBeforeContactRepair leaves the parent's outgoing carrier
// contact at its local address; the test repairs it through the public CAS.
func newCarrierThreePartyBeforeContactRepair(t *testing.T) *carrierThreePartyFixture {
	t.Helper()
	return newCarrierThreePartyFixture(t, false)
}

func newCarrierThreePartyFixture(t *testing.T, repairedContact bool) *carrierThreePartyFixture {
	t.Helper()
	listen := func() net.Listener {
		l, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = l.Close() })
		return l
	}
	pl, sl, tl := listen(), listen(), listen()
	parentAddr := values.NextHop("https://" + pl.Addr().String())
	sourceAddr := values.NextHop("https://" + sl.Addr().String())
	targetAddr := values.NextHop("https://" + tl.Addr().String())

	// Build the parent fixture and source Core with the network parent address.
	path := filepath.Join(t.TempDir(), "carrier.db")
	p, source, command := newCarrierFixtureWithParent(t, path, parentAddr)
	d := newTargetFixture(t, true) // Independent Realm and importer identity.
	command.Target = d.c.AuthorityRef()
	command.SourceAddress = sourceAddr
	command.TargetAddress = targetAddr

	f := &carrierThreePartyFixture{
		carrierFixture: &carrierFixture{parent: p, source: source, sourcePath: path, target: d,
			binding: contract.CarrierApprovalBinding{Parent: p.c.AuthorityRef(), Operation: contract.OperationID(string([]byte{255, 0, 128, 'c'})), Command: command}},
		parentAddr: parentAddr, sourceAddr: sourceAddr, targetAddr: targetAddr,
	}
	t.Cleanup(func() { _ = f.source.Close() })
	for _, permission := range []values.PermissionID{contract.PermissionTransitionCommit, contract.PermissionTransitionInspect} {
		p.grant(t, actor, permission, values.CoreElementScope(p.c.AuthorityRef(), carrierMount), false)
	}
	p.grant(t, actor, contract.PermissionTransitionExport, values.CoreElementScope(source.AuthorityRef(), carrierMount), false)
	p.grant(t, actor, contract.PermissionCreateEntity, values.CoreElementScope(source.AuthorityRef(), carrierMount), false)
	p.grant(t, actor, contract.PermissionReadElement, values.CoreElementScope(source.AuthorityRef(), carrierEntity), false)
	d.grant(t, contract.PermissionTransitionImport, carrierMount, false)
	d.grant(t, contract.PermissionReadElement, carrierEntity, false)
	d.grant(t, contract.PermissionMountRead, carrierMount, false)
	if _, err := source.Elements().CreateEntity(actor, carrierEntity, tuple(t, carrierMount), "carrier-content"); err != nil {
		t.Fatal(err)
	}
	var err error
	if repairedContact {
		if err := f.repairSourceContact(t); err != nil {
			t.Fatal(err)
		}
		if err := f.beginCarrier(t); err != nil {
			t.Fatal(err)
		}
	}

	// TLS identities: one certificate per host.
	parentCert, _, parentPin := peerCertificate(t)
	sourceCert, _, sourcePin := peerCertificate(t)
	targetCert, _, targetPin := peerCertificate(t)
	f.parentPin, f.sourcePin, f.targetPin = parentPin, sourcePin, targetPin
	f.parentCert, f.sourceCert, f.targetCert = parentCert, sourceCert, targetCert

	// Build client root pools from the peer DER bytes.
	poolFor := func(der []byte) *x509.CertPool {
		leaf, err := x509.ParseCertificate(der)
		if err != nil {
			t.Fatal(err)
		}
		pool := x509.NewCertPool()
		pool.AddCert(leaf)
		return pool
	}
	parentPool, sourcePool, targetPool := poolFor(parentCert.Certificate[0]), poolFor(sourceCert.Certificate[0]), poolFor(targetCert.Certificate[0])
	f.parentPool, f.sourcePool, f.targetPool = parentPool, sourcePool, targetPool

	base := coregrpc.Config{SessionTTL: time.Minute, RPCDeadline: 5 * time.Second, HandshakeTimeout: time.Second, ConnectionLifetime: time.Minute, ShutdownTimeout: time.Second,
		MaxSessions: 8, MaxSessionsPerCaller: 4, MaxConnections: 16, MaxConcurrentRPCs: 16, MaxStreams: 16, MaxMetadataBytes: 8192, MaxResponseBytes: 128 << 10}
	approvalConfig := &coregrpc.CarrierApprovalConfig{MaxTTL: time.Minute, MaxApprovals: 8, MaxPerCaller: 4}

	// --- Parent host ---
	parentToSource, err := coresdk.New(coresdk.Config{NextHop: sourceAddr, Expected: command.Source, Roots: sourcePool, SPKISHA256: sourcePin,
		Actor: f.binding.Parent, ActorCertificate: &parentCert, RPCDeadline: 5 * time.Second, ConnectTimeout: time.Second, MaxResponseBytes: 128 << 10, MaxMetadataBytes: 8192})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = parentToSource.Close() })
	parentToTarget, err := coresdk.New(coresdk.Config{NextHop: targetAddr, Expected: command.Target, Roots: targetPool, SPKISHA256: targetPin,
		Actor: f.binding.Parent, ActorCertificate: &parentCert, RPCDeadline: 5 * time.Second, ConnectTimeout: time.Second, MaxResponseBytes: 128 << 10, MaxMetadataBytes: 8192})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = parentToTarget.Close() })
	parentConnections, err := transitionconn.NewConnections(f.binding.Parent, []*coresdk.Client{parentToSource, parentToTarget}, 4, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	parentClientRoots := x509.NewCertPool()
	for _, der := range [][]byte{sourceCert.Certificate[0], targetCert.Certificate[0]} {
		leaf, err := x509.ParseCertificate(der)
		if err != nil {
			t.Fatal(err)
		}
		parentClientRoots.AddCert(leaf)
	}
	f.parentConfig = base
	f.parentConfig.Expected, f.parentConfig.NextHop, f.parentConfig.Certificate = f.binding.Parent, parentAddr, parentCert
	f.parentConfig.TransitionControl = p.c.Transfers()
	f.parentConfig.TransitionParent = p.c.Transfers()
	f.parentConfig.TransitionTarget = p.c.Transfers()
	f.parentConfig.TransitionConnections = parentConnections
	f.parentConfig.ReadDelegation = &coregrpc.ReadDelegationConfig{
		ClientRoots: parentClientRoots,
		Peers:       map[values.FactAuthorityRef][32]byte{command.Source: sourcePin, command.Target: targetPin},
		MaxTTL:      time.Minute, MaxGrants: 8, MaxPerCaller: 4}
	f.parentHost, err = coregrpc.New(p.c.Access(), composition.RealmNetworkAuthentication{Connections: p.realmLinks(t), Owner: p.c.Access()}, f.parentConfig)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(f.parentHost.Stop)
	go func() { _ = f.parentHost.Serve(pl) }()

	// --- Source host ---
	sourceToParent, err := coresdk.New(coresdk.Config{NextHop: parentAddr, Expected: f.binding.Parent, Roots: parentPool, SPKISHA256: parentPin,
		Actor: command.Source, ActorCertificate: &sourceCert, RPCDeadline: 5 * time.Second, ConnectTimeout: time.Second, MaxResponseBytes: 128 << 10, MaxMetadataBytes: 8192})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sourceToParent.Close() })
	sourceToTarget, err := coresdk.New(coresdk.Config{NextHop: targetAddr, Expected: command.Target, Roots: targetPool, SPKISHA256: targetPin,
		Actor: command.Source, ActorCertificate: &sourceCert, RPCDeadline: 5 * time.Second, ConnectTimeout: time.Second, MaxResponseBytes: 128 << 10, MaxMetadataBytes: 8192})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sourceToTarget.Close() })
	sourceConnections, err := transitionconn.NewConnections(command.Source, []*coresdk.Client{sourceToParent, sourceToTarget}, 4, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	sourceClientRoots := x509.NewCertPool()
	for _, der := range [][]byte{parentCert.Certificate[0], targetCert.Certificate[0]} {
		leaf, err := x509.ParseCertificate(der)
		if err != nil {
			t.Fatal(err)
		}
		sourceClientRoots.AddCert(leaf)
	}
	f.sourceConfig = base
	f.sourceConfig.Expected, f.sourceConfig.NextHop, f.sourceConfig.Certificate = command.Source, sourceAddr, sourceCert
	f.sourceConfig.TransitionSource = source.Transfers()
	f.sourceConfig.TransitionTarget = source.Transfers()
	f.sourceConfig.CarrierSource = source.Transfers()
	f.sourceConfig.CarrierApprovals = approvalConfig
	f.sourceConfig.CarrierApproval = source.Transfers()
	f.sourceConfig.TransitionConnections = sourceConnections
	f.sourceConfig.ReadDelegation = &coregrpc.ReadDelegationConfig{
		ClientRoots: sourceClientRoots,
		Peers:       map[values.FactAuthorityRef][32]byte{f.binding.Parent: parentPin, command.Target: targetPin},
		MaxTTL:      time.Minute, MaxGrants: 8, MaxPerCaller: 4}
	f.sourceHost, err = coregrpc.New(source.Access(), composition.RealmNetworkAuthentication{Connections: p.realmLinks(t), Owner: source.Access()}, f.sourceConfig)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(f.sourceHost.Stop)
	go func() { _ = f.sourceHost.Serve(sl) }()

	// --- Target host ---
	targetToParent, err := coresdk.New(coresdk.Config{NextHop: parentAddr, Expected: f.binding.Parent, Roots: parentPool, SPKISHA256: parentPin,
		Actor: command.Target, ActorCertificate: &targetCert, RPCDeadline: 5 * time.Second, ConnectTimeout: time.Second, MaxResponseBytes: 128 << 10, MaxMetadataBytes: 8192})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = targetToParent.Close() })
	targetToSource, err := coresdk.New(coresdk.Config{NextHop: sourceAddr, Expected: command.Source, Roots: sourcePool, SPKISHA256: sourcePin,
		Actor: command.Target, ActorCertificate: &targetCert, RPCDeadline: 5 * time.Second, ConnectTimeout: time.Second, MaxResponseBytes: 128 << 10, MaxMetadataBytes: 8192})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = targetToSource.Close() })
	targetConnections, err := transitionconn.NewConnections(command.Target, []*coresdk.Client{targetToParent, targetToSource}, 4, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	targetClientRoots := x509.NewCertPool()
	for _, der := range [][]byte{parentCert.Certificate[0], sourceCert.Certificate[0]} {
		leaf, err := x509.ParseCertificate(der)
		if err != nil {
			t.Fatal(err)
		}
		targetClientRoots.AddCert(leaf)
	}
	f.targetConfig = base
	f.targetConfig.Expected, f.targetConfig.NextHop, f.targetConfig.Certificate = command.Target, targetAddr, targetCert
	f.targetConfig.TransitionTarget = d.c.Transfers()
	f.targetConfig.TransitionImport = d.c.Transfers()
	f.targetConfig.CarrierTarget = d.c.Transfers()
	f.targetConfig.CarrierApprovals = approvalConfig
	f.targetConfig.CarrierApproval = d.c.Transfers()
	f.targetConfig.TransitionConnections = targetConnections
	f.targetConfig.ReadDelegation = &coregrpc.ReadDelegationConfig{
		ClientRoots: targetClientRoots,
		Peers:       map[values.FactAuthorityRef][32]byte{f.binding.Parent: parentPin, command.Source: sourcePin},
		MaxTTL:      time.Minute, MaxGrants: 8, MaxPerCaller: 4}
	f.targetHost, err = coregrpc.New(d.c.Access(), composition.RealmNetworkAuthentication{Connections: d.realmLinks(t), Owner: d.c.Access()}, f.targetConfig)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(f.targetHost.Stop)
	go func() { _ = f.targetHost.Serve(tl) }()

	// --- User session clients ---
	f.control, err = coresdk.New(coresdk.Config{NextHop: parentAddr, Expected: f.binding.Parent, Roots: parentPool, SPKISHA256: parentPin,
		RPCDeadline: 5 * time.Second, ConnectTimeout: time.Second, MaxResponseBytes: 128 << 10, MaxMetadataBytes: 8192})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.control.Close() })
	if err := f.control.Establish(context.Background(), actor.Realm, [16]byte{62}, [32]byte{63}); err != nil {
		t.Fatal(err)
	}
	f.sourceUser, err = coresdk.New(coresdk.Config{NextHop: sourceAddr, Expected: command.Source, Roots: sourcePool, SPKISHA256: sourcePin,
		RPCDeadline: 5 * time.Second, ConnectTimeout: time.Second, MaxResponseBytes: 128 << 10, MaxMetadataBytes: 8192})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.sourceUser.Close() })
	if err := f.sourceUser.Establish(context.Background(), actor.Realm, [16]byte{62}, [32]byte{63}); err != nil {
		t.Fatal(err)
	}
	f.targetUser, err = coresdk.New(coresdk.Config{NextHop: targetAddr, Expected: command.Target, Roots: targetPool, SPKISHA256: targetPin,
		RPCDeadline: 5 * time.Second, ConnectTimeout: time.Second, MaxResponseBytes: 128 << 10, MaxMetadataBytes: 8192})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.targetUser.Close() })
	if err := f.targetUser.Establish(context.Background(), d.caller.Subject.Realm, [16]byte{72}, [32]byte{73}); err != nil {
		t.Fatal(err)
	}

	// Wire binding for RPC requests.
	c := f.binding.Command
	e, _ := values.DecodePublicBytes(c.ExpectedElement.Value)
	m, _ := values.DecodePublicBytes(c.ExpectedMount.Value)
	f.w = &pb.CarrierApprovalBinding{Parent: proto.String(string(f.binding.Parent)), Command: &pb.BeginCarrierTransitionRequest{OperationId: []byte(f.binding.Operation),
		Source: proto.String(string(c.Source)), Target: proto.String(string(c.Target)), Mount: proto.String(string(c.Mount)),
		ExpectedElement: &netpb.Token{Owner: proto.String(string(f.binding.Parent)), Kind: netpb.TokenKind_RECORD_REVISION.Enum(), Value: e},
		ExpectedMount:   &netpb.Token{Owner: proto.String(string(f.binding.Parent)), Kind: netpb.TokenKind_MOUNT_REVISION.Enum(), Value: m},
		ParentAddress:   proto.String(string(c.ParentAddress)), SourceAddress: proto.String(string(c.SourceAddress)), TargetAddress: proto.String(string(c.TargetAddress))}}
	return f
}

// approve creates carrier approvals on both source and target hosts via their
// authenticated user sessions and returns the tokens.
func (f *carrierThreePartyFixture) approve(t *testing.T, ttlSeconds uint64) (sourceToken, targetToken []byte) {
	t.Helper()
	ctx := context.Background()
	sr, err := f.sourceUser.CarrierApprovals().CreateCarrierApproval(ctx, &pb.CreateCarrierApprovalRequest{Binding: f.w, TtlSeconds: proto.Uint64(ttlSeconds)})
	if err != nil || sr.GetResult() == nil {
		t.Fatalf("source approval: %v %v", sr, err)
	}
	tr, err := f.targetUser.CarrierApprovals().CreateCarrierApproval(ctx, &pb.CreateCarrierApprovalRequest{Binding: f.w, TtlSeconds: proto.Uint64(ttlSeconds)})
	if err != nil || tr.GetResult() == nil {
		t.Fatalf("target approval: %v %v", tr, err)
	}
	return sr.GetResult().Token, tr.GetResult().Token
}

// recoverNetwork drives the parent-side public recovery RPC over TLS.
func (f *carrierThreePartyFixture) recoverNetwork(t *testing.T, sourceToken, targetToken []byte) *pb.CarrierTransitionResponse {
	t.Helper()
	r, err := f.control.Transitions().RecoverCarrierTransition(context.Background(), &pb.RecoverCarrierTransitionRequest{
		Binding: f.w, SourceToken: sourceToken, TargetToken: targetToken})
	if err != nil {
		t.Fatal(err)
	}
	return r
}

// recoverNetworkEventually re-drives the same bounded parent recovery until it
// settles. A peer process that has just returned also has to re-establish the
// retained client connections, so the caller re-issues the same retained
// approvals; every attempt resumes from the parent's durable milestones.
func (f *carrierThreePartyFixture) recoverNetworkEventually(t *testing.T, sourceToken, targetToken []byte) *pb.CarrierTransitionResponse {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	var last *pb.CarrierTransitionResponse
	for time.Now().Before(deadline) {
		last = f.recoverNetwork(t, sourceToken, targetToken)
		if last.GetResult() != nil {
			return last
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("recovery did not settle after the peer returned: %+v", last)
	return nil
}

// repairSourceContact moves the parent's retained outgoing carrier contact to
// the network source address through its public CAS. The reservation compares
// the stored next hop, so the repair must precede the reservation.
func (f *carrierThreePartyFixture) repairSourceContact(t *testing.T) error {
	t.Helper()
	contact, err := f.parent.c.Mounts().InspectMountContact(context.Background(), f.parent.caller, carrierMount, false)
	if err != nil {
		return err
	}
	_, err = f.parent.c.Mounts().RepairMountContact(context.Background(), f.parent.caller, "carrier-network-contact", carrierMount, false, contact.Revision, f.sourceAddr)
	return err
}

func (f *carrierThreePartyFixture) beginCarrier(t *testing.T) error {
	t.Helper()
	status, err := f.parent.c.Transfers().BeginCarrierTransition(context.Background(), f.parent.caller, f.binding.Operation, f.binding.Command)
	if err != nil {
		return err
	}
	f.begin = status
	return nil
}

func listenOn(t *testing.T, address values.NextHop) net.Listener {
	t.Helper()
	l, err := net.Listen("tcp", strings.TrimPrefix(string(address), "https://"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = l.Close() })
	return l
}

// restartSourceHost simulates an independent source process restart: the host
// stops, the Core reopens from its Store, the host re-listens on the same
// address and the approver session is re-established. Runtime approvals and
// sessions are not durable; the source's carrier progress is.
func (f *carrierThreePartyFixture) restartSourceHost(t *testing.T) {
	t.Helper()
	f.sourceHost.Stop()
	if err := f.source.Close(); err != nil {
		t.Fatal(err)
	}
	var err error
	f.source, err = core.OpenDurable(boundaryContacts{f.parent.a}, f.sourcePath, f.parent.config)
	if err != nil {
		t.Fatal(err)
	}
	f.sourceConfig.TransitionSource = f.source.Transfers()
	f.sourceConfig.TransitionTarget = f.source.Transfers()
	f.sourceConfig.CarrierSource = f.source.Transfers()
	f.sourceConfig.CarrierApproval = f.source.Transfers()
	f.sourceHost, err = coregrpc.New(f.source.Access(), composition.RealmNetworkAuthentication{Connections: f.parent.realmLinks(t), Owner: f.source.Access()}, f.sourceConfig)
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = f.sourceHost.Serve(listenOn(t, f.sourceAddr)) }()
	_ = f.sourceUser.Close()
	f.sourceUser, err = coresdk.New(coresdk.Config{NextHop: f.sourceAddr, Expected: f.binding.Command.Source, Roots: f.sourcePool, SPKISHA256: f.sourcePin,
		RPCDeadline: 5 * time.Second, ConnectTimeout: time.Second, MaxResponseBytes: 128 << 10, MaxMetadataBytes: 8192})
	if err != nil {
		t.Fatal(err)
	}
	if err := f.sourceUser.Establish(context.Background(), actor.Realm, [16]byte{62}, [32]byte{63}); err != nil {
		t.Fatal(err)
	}
}

// restartTargetHost simulates an independent target process restart with the
// same shape as restartSourceHost.
func (f *carrierThreePartyFixture) restartTargetHost(t *testing.T) {
	t.Helper()
	f.targetHost.Stop()
	f.target.reopen(t)
	var err error
	f.targetConfig.TransitionTarget = f.target.c.Transfers()
	f.targetConfig.TransitionImport = f.target.c.Transfers()
	f.targetConfig.CarrierTarget = f.target.c.Transfers()
	f.targetConfig.CarrierApproval = f.target.c.Transfers()
	f.targetHost, err = coregrpc.New(f.target.c.Access(), composition.RealmNetworkAuthentication{Connections: f.target.realmLinks(t), Owner: f.target.c.Access()}, f.targetConfig)
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = f.targetHost.Serve(listenOn(t, f.targetAddr)) }()
	_ = f.targetUser.Close()
	f.targetUser, err = coresdk.New(coresdk.Config{NextHop: f.targetAddr, Expected: f.binding.Command.Target, Roots: f.targetPool, SPKISHA256: f.targetPin,
		RPCDeadline: 5 * time.Second, ConnectTimeout: time.Second, MaxResponseBytes: 128 << 10, MaxMetadataBytes: 8192})
	if err != nil {
		t.Fatal(err)
	}
	if err := f.targetUser.Establish(context.Background(), f.target.caller.Subject.Realm, [16]byte{72}, [32]byte{73}); err != nil {
		t.Fatal(err)
	}
}

// nestedMountInsideCarrierContent establishes one active source-owned mount
// boundary inside the carrier content. The captured source manifest then holds
// an external boundary this workflow does not coordinate.
func (f *carrierThreePartyFixture) nestedMountInsideCarrierContent(t *testing.T) {
	t.Helper()
	nested, err := core.CreateDurable(boundaryContacts{f.parent.a}, transferContact, filepath.Join(t.TempDir(), "nested.db"), f.parent.config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = nested.Close() })
	f.parent.grant(t, actor, contract.PermissionCreateAnchor, values.CoreElementScope(f.source.AuthorityRef(), carrierMount), false)
	for _, permission := range []values.PermissionID{contract.PermissionMountControl, contract.PermissionMountRead} {
		f.parent.grant(t, actor, permission, values.CoreElementScope(f.source.AuthorityRef(), nestedCarrierMount), false)
	}
	for _, permission := range []values.PermissionID{contract.PermissionMountAccept, contract.PermissionMountRead} {
		f.parent.grant(t, actor, permission, values.CoreElementScope(nested.AuthorityRef(), nestedCarrierMount), false)
	}
	anchor, err := f.source.Elements().CreateAnchor(actor, nestedCarrierMount, tuple(t, carrierMount), "nested-carrier-anchor")
	if err != nil || anchor.Record == nil || anchor.Record.Anchor == nil {
		t.Fatal(anchor, err)
	}
	// Keep every existing local route and add the nested pair.
	connections := &composition.LocalMountConnections{}
	for address, peer := range map[values.NextHop]contract.MountPeerPort{
		"local:parent":         f.parent.c.Mounts().MountPeer(),
		"local:carrier":        f.source.Mounts().MountPeer(),
		"local:nested-parent":  f.source.Mounts().MountPeer(),
		"local:nested-carrier": nested.Mounts().MountPeer(),
	} {
		if err := connections.BindEndpoint(address, peer); err != nil {
			t.Fatal(err)
		}
	}
	for _, owner := range []*core.Core{f.source, nested} {
		transport, err := connections.TransportFor(owner)
		if err != nil {
			t.Fatal(err)
		}
		if err := owner.Mounts().ConfigureMountTransport(transport); err != nil {
			t.Fatal(err)
		}
	}
	command := contract.MountCommand{Kind: contract.MountEstablish, MountAnchorUUID: nestedCarrierMount,
		ExpectedElementRevision: anchor.Record.Anchor.Revision, CarrierAuthority: nested.AuthorityRef(),
		ParentNextHop: "local:nested-parent", CarrierNextHop: "local:nested-carrier"}
	result, err := f.source.Mounts().ApplyMount(context.Background(),
		contract.LocalCallers{f.source.AuthorityRef(): actor, nested.AuthorityRef(): actor}, "nested-mount", command)
	if err != nil || result.Decision != contract.MountCommitted || !result.Settled {
		t.Fatal("nested mount fixture", result, err)
	}
}
