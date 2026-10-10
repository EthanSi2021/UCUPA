package acceptance_test

import (
	"context"
	"encoding/binary"
	"net"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	"ucupa/composition"
	networkpb "ucupa/shared/bindings/go/protobuf"
	"ucupa/shared/values"
	pb "ucupa/ucupa-core/bindings/go/protobuf"
	corecontract "ucupa/ucupa-core/contract"
	coresdk "ucupa/ucupa-core/sdk/go"
	coregrpc "ucupa/ucupa-core/transport/grpc"
)

// Capture a real owner's reservation, without manufacturing a retained
// request or pretending that the parent's public network entry already exists.
type mountWireCapture struct {
	request  corecontract.MountPrepareRequest
	prepared *corecontract.MountPrepared
}

func (p *mountWireCapture) Prepare(_ context.Context, _ values.NextHop, _ values.FactAuthorityRef, r corecontract.MountPrepareRequest) (corecontract.MountPrepared, error) {
	p.request = r
	if p.prepared != nil {
		return *p.prepared, nil
	}
	return corecontract.MountPrepared{}, corecontract.ErrUnavailable
}
func (*mountWireCapture) Finish(context.Context, values.NextHop, values.FactAuthorityRef, corecontract.MountFinishRequest) (corecontract.MountClosed, error) {
	return corecontract.MountClosed{}, corecontract.ErrUnavailable
}
func (*mountWireCapture) ReadDecision(context.Context, values.NextHop, values.FactAuthorityRef, corecontract.MountAttemptKey) (corecontract.MountDecisionEvidence, error) {
	return corecontract.MountDecisionEvidence{}, corecontract.ErrUnavailable
}

func serviceRevisionTest(r corecontract.ServiceRevision) *pb.ServiceRevision {
	return &pb.ServiceRevision{Incarnation: r.Incarnation[:], Sequence: proto.Uint64(r.Sequence)}
}
func servicePrincipalTest(p values.PrincipalRef) *pb.ServicePrincipal {
	return &pb.ServicePrincipal{Realm: proto.String(string(p.Realm)), Principal: proto.String(string(p.Principal))}
}
func serviceMountRequestTest(r corecontract.MountPrepareRequest) *pb.ServiceMountPrepare {
	t := r.Tuple
	ratios := t.LocalPose.Rotation.Ratios()
	p := t.LocalPose.Translation
	return &pb.ServiceMountPrepare{
		Key:         &pb.ServiceMountKey{ParentCore: proto.String(string(r.Key.ParentAuthority)), CarrierCore: proto.String(string(r.Key.CarrierAuthority)), Element: proto.String(string(r.Key.MountAnchorUUID)), Proposal: serviceRevisionTest(r.Key.ProposalRevision.ServiceEvidence())},
		OperationId: []byte(r.OperationID), Command: &pb.ServiceMountCommand{Kind: pb.MountCommandKind_MOUNT_ESTABLISH.Enum(), Element: proto.String(string(r.Command.MountAnchorUUID)), ExpectedElement: serviceRevisionTest(r.Command.ExpectedElementRevision.ServiceEvidence()), CarrierCore: proto.String(string(r.Command.CarrierAuthority)), CarrierNextHop: proto.String(string(r.Command.CarrierNextHop)), ParentNextHop: proto.String(string(r.Command.ParentNextHop))},
		ParentActor: servicePrincipalTest(r.ParentActor), CarrierActor: servicePrincipalTest(r.CarrierActor), Reserved: serviceRevisionTest(r.ReservedRevision.ServiceEvidence()),
		Tuple: &pb.SpatialTuple{Parent: proto.String(string(t.Parent)), Pose: &pb.Pose{Translation: &networkpb.Vector3{Items: []float64{p.X, p.Y, p.Z}}, Rotation: &pb.Rotation{Pivot: proto.Uint32(uint32(t.LocalPose.Rotation.Pivot())), Ratios: &networkpb.Vector3{Items: ratios[:]}}}, Range: &pb.Range{Kind: pb.RangeKind_BOUNDED_SPHERE.Enum(), Radius: proto.Float64(t.Range.Radius())}},
	}
}

func TestNetworkMountPrepareConsumesExactOriginalApproval(t *testing.T) {
	b, a, service, _ := mountApprovalFixture(t, true)
	ctx := context.Background()
	element := values.ElementID(networkEntity)
	a.grant(t, corecontract.PermissionMountControl, values.CoreElementScope(a.c.AuthorityRef(), element), false)
	a.grant(t, corecontract.PermissionCreateAnchor, values.CoreElementScope(a.c.AuthorityRef(), networkRoot), false)
	b.grant(t, corecontract.PermissionMountAccept, values.CoreElementScope(b.c.AuthorityRef(), element), false)
	sphere, _ := corecontract.NewBoundedSphere(10)
	rotation, _ := corecontract.NewRotation(1, 0, 0, 0)
	created, err := a.c.Elements().CreateAnchor(a.actor, element, corecontract.SpatialTuple{Parent: networkRoot, LocalPose: corecontract.Pose{Rotation: rotation}, Range: sphere}, "peer-anchor")
	if err != nil {
		t.Fatal(err)
	}
	command := corecontract.MountCommand{Kind: corecontract.MountEstablish, MountAnchorUUID: element, ExpectedElementRevision: created.Record.Anchor.Revision, CarrierAuthority: b.c.AuthorityRef(), CarrierNextHop: b.config.NextHop, ParentNextHop: a.config.NextHop}
	const operation corecontract.OperationID = "peer-prepare"
	digest, err := a.c.Mounts().CommandCommitment(a.c.AuthorityRef(), b.c.AuthorityRef(), operation, command)
	if err != nil {
		t.Fatal(err)
	}
	binding := &pb.MountApprovalBinding{Actor: proto.String(string(a.c.AuthorityRef())), Element: proto.String(string(element)), OperationId: []byte(operation), CommandDigest: digest[:]}
	approval := createMountApproval(t, b, binding, 5)
	capture := new(mountWireCapture)
	if err = a.c.Mounts().ConfigureMountTransport(capture); err != nil {
		t.Fatal(err)
	}
	if _, err = a.c.Mounts().ApplyMount(ctx, corecontract.LocalCallers{a.c.AuthorityRef(): a.actor, b.c.AuthorityRef(): b.actor}, operation, command); err != nil {
		t.Fatal(err)
	}
	request := &pb.PrepareMountRequest{Request: serviceMountRequestTest(capture.request), Approval: &pb.CheckMountApprovalRequest{Binding: binding, Token: approval.Token}}
	bad := proto.Clone(request).(*pb.PrepareMountRequest)
	bad.Request.CarrierActor.Principal = proto.String("invented")
	if _, err = service.MountPeer().PrepareMount(ctx, bad); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("forged user: %v", err)
	}
	bad = proto.Clone(request).(*pb.PrepareMountRequest)
	bad.Request.Command.CarrierNextHop = proto.String("https://other.example:443")
	if _, err = service.MountPeer().PrepareMount(ctx, bad); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("changed command: %v", err)
	}
	if _, err = b.client.MountPeer().PrepareMount(ctx, request); status.Code(err) != codes.Unauthenticated {
		t.Fatalf("user in service role: %v", err)
	}
	got, err := service.MountPeer().PrepareMount(ctx, request)
	if err != nil || !got.GetResult().GetPrepared() || got.GetResult().GetRejection() != pb.MountRejection_MOUNT_NO_REJECTION {
		t.Fatalf("prepare: %v %v", got, err)
	}
	again, err := service.MountPeer().PrepareMount(ctx, request)
	if err != nil || !proto.Equal(got, again) {
		t.Fatalf("exact replay: %v %v", again, err)
	}
	if _, err = b.client.MountApprovals().RevokeMountApproval(ctx, &pb.RevokeMountApprovalRequest{Token: approval.Token}); err != nil {
		t.Fatal(err)
	}
	if _, err = service.MountPeer().PrepareMount(ctx, request); status.Code(err) != codes.Unauthenticated {
		t.Fatalf("revoked replay: %v", err)
	}
	// Feed the real response into the local parent driver. The parent network
	// application entry is deliberately not claimed by this peer-port test.
	cutBytes := make([]byte, 40)
	copy(cutBytes, got.GetResult().Cut.Incarnation)
	binary.BigEndian.PutUint64(cutBytes[32:], got.GetResult().Cut.GetSequence())
	cutEvidence, err := values.NewCoreCutEvidence(cutBytes)
	if err != nil {
		t.Fatal(err)
	}
	cut, err := corecontract.CoreReadCutFromEvidence(cutEvidence)
	if err != nil {
		t.Fatal(err)
	}
	capture.prepared = &corecontract.MountPrepared{Request: capture.request, Prepared: true, Cut: cut}
	decided, err := a.c.Mounts().ApplyMount(ctx, corecontract.LocalCallers{a.c.AuthorityRef(): a.actor, b.c.AuthorityRef(): b.actor}, operation, command)
	if err != nil || decided.Decision != corecontract.MountCommitted || decided.Settled {
		t.Fatalf("decision: %#v %v", decided, err)
	}
	a.config.ReadDelegation = &coregrpc.ReadDelegationConfig{ClientRoots: b.clientConfig.Roots, Peers: map[values.FactAuthorityRef][32]byte{b.c.AuthorityRef(): b.clientConfig.SPKISHA256}, MaxTTL: 5 * time.Second, MaxGrants: 3, MaxPerCaller: 2}
	l, err := net.Listen("tcp", strings.TrimPrefix(string(a.config.NextHop), "https://"))
	if err != nil {
		t.Fatal(err)
	}
	a.config.FacetControl = a.c.Facets()
	a.config.MountControl = a.c.Mounts()
	a.server, err = coregrpc.New(a.c.Access(), composition.RealmNetworkAuthentication{Connections: a.realmLinks, Owner: a.c.Access()}, a.config)
	if err != nil {
		_ = l.Close()
		t.Fatal(err)
	}
	go func() { _ = a.server.Serve(l) }()
	cfg := a.clientConfig
	cfg.Actor = b.c.AuthorityRef()
	cfg.ActorCertificate = &b.config.Certificate
	back, err := coresdk.New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer back.Close()
	read, err := back.MountPeer().ReadMountDecision(ctx, &pb.ReadMountDecisionRequest{ParentCore: proto.String(string(a.c.AuthorityRef())), Actor: proto.String(string(b.c.AuthorityRef())), OperationId: []byte(operation)})
	if err != nil || read.GetResult().GetDecision() != pb.MountDecisionKind_MOUNT_COMMITTED {
		t.Fatalf("decision read: %v %v", read, err)
	}
	closed, err := service.MountPeer().FinishMount(ctx, &pb.FinishMountRequest{Evidence: read.GetResult()})
	if err != nil || closed.GetResult() == nil || !proto.Equal(closed.GetResult().Evidence, read.GetResult()) {
		t.Fatalf("finish after approval revocation: %v %v", closed, err)
	}
	closedAgain, err := service.MountPeer().FinishMount(ctx, &pb.FinishMountRequest{Evidence: read.GetResult()})
	if err != nil || !proto.Equal(closed, closedAgain) {
		t.Fatalf("finish replay: %v %v", closedAgain, err)
	}
	wrong := proto.Clone(read.GetResult()).(*pb.ServiceMountDecision)
	wrong.Request.OperationId = []byte("another-attempt")
	if out, err := service.MountPeer().FinishMount(ctx, &pb.FinishMountRequest{Evidence: wrong}); err == nil && out.GetResult() != nil {
		t.Fatalf("altered finish accepted: %v", out)
	}
}
