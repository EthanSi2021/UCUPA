package transfer_test

import (
	"context"
	"net"
	"strings"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"ucupa/composition"
	netpb "ucupa/shared/bindings/go/protobuf"
	"ucupa/shared/values"
	pb "ucupa/ucupa-core/bindings/go/protobuf"
	"ucupa/ucupa-core/contract"
	coresdk "ucupa/ucupa-core/sdk/go"
	coregrpc "ucupa/ucupa-core/transport/grpc"
)

// TestNetworkCarrierThreePartyCompletesAndRetainsResults drives the full
// carrier transfer over three independent TLS hosts: approve → recover →
// settled → target readable → source fenced → idempotent re-recovery.
func TestNetworkCarrierThreePartyCompletesAndRetainsResults(t *testing.T) {
	f := newCarrierThreeParty(t)
	ctx := context.Background()
	sourceToken, targetToken := f.approve(t, 60)

	r := f.recoverNetwork(t, sourceToken, targetToken)
	if r.GetResult() == nil || r.GetResult().GetProgress() != pb.CarrierProgress_CARRIER_SETTLED {
		t.Fatalf("recovery did not settle: %+v", r)
	}

	// Target content is readable under the carrier mount.
	read, err := f.target.c.Elements().ReadElement(f.target.caller.Subject, carrierEntity, nil)
	if err != nil || read.Record == nil || read.Record.Entity == nil {
		t.Fatal("target did not install carrier content", err)
	}
	if read.Record.Entity.ContainingAnchorID != carrierMount {
		t.Fatal("content not under carrier mount")
	}

	// Source is fenced: not readable, not writable.
	if _, err := f.source.Elements().ReadElement(actor, carrierEntity, nil); err == nil {
		t.Fatal("source remained readable after settlement")
	}
	if _, err := f.source.Elements().CreateEntity(actor, "00000000-0000-4000-8000-000000000041", tuple(t, carrierMount), "late"); err == nil {
		t.Fatal("source remained writable after settlement")
	}

	// Parent mount now points to target.
	mount, err := f.parent.c.Mounts().InspectMountControl(context.Background(), f.parent.caller, carrierMount)
	if err != nil || mount.Carrier != f.target.c.AuthorityRef() {
		t.Fatal("parent binding not switched to target", mount, err)
	}

	// Idempotent re-recovery returns the same settled result.
	r2 := f.recoverNetwork(t, sourceToken, targetToken)
	if r2.GetResult() == nil || r2.GetResult().GetProgress() != pb.CarrierProgress_CARRIER_SETTLED {
		t.Fatal("repeat recovery changed result", r2)
	}
	if !proto.Equal(r.GetResult().GetRevision(), r2.GetResult().GetRevision()) {
		t.Fatal("repeat recovery changed revision")
	}

	// Original Begin result is retained.
	begin, err := f.control.Transitions().BeginCarrierTransition(ctx, &pb.BeginCarrierTransitionRequest{
		OperationId: []byte(f.binding.Operation), Mount: proto.String(string(f.binding.Command.Mount)),
		Source: proto.String(string(f.binding.Command.Source)), Target: proto.String(string(f.binding.Command.Target)),
		ExpectedElement: f.w.Command.ExpectedElement, ExpectedMount: f.w.Command.ExpectedMount,
		ParentAddress: f.w.Command.ParentAddress, SourceAddress: f.w.Command.SourceAddress, TargetAddress: f.w.Command.TargetAddress})
	if err != nil || begin.GetResult() == nil {
		t.Fatal("original begin lost", begin, err)
	}
	_ = ctx
}

// TestNetworkCarrierThreePartyParentRestartRetainsSettlement verifies that a
// parent host restart after settlement still returns the original result.
func TestNetworkCarrierThreePartyParentRestartRetainsSettlement(t *testing.T) {
	f := newCarrierThreeParty(t)
	sourceToken, targetToken := f.approve(t, 60)

	r := f.recoverNetwork(t, sourceToken, targetToken)
	if r.GetResult() == nil || r.GetResult().GetProgress() != pb.CarrierProgress_CARRIER_SETTLED {
		t.Fatalf("initial recovery failed: %+v", r)
	}

	// Restart the parent host with a reopened Core.
	f.reopenParentHost(t)

	r2 := f.recoverNetwork(t, sourceToken, targetToken)
	if r2.GetResult() == nil || r2.GetResult().GetProgress() != pb.CarrierProgress_CARRIER_SETTLED {
		t.Fatal("parent restart lost settled result", r2)
	}
	if !proto.Equal(r.GetResult().GetRevision(), r2.GetResult().GetRevision()) {
		t.Fatal("parent restart changed revision")
	}
}

// reopenParentHost stops the parent host, reopens the parent Core, re-listens
// on the same address and rebuilds the host with the saved connections.
func (f *carrierThreePartyFixture) reopenParentHost(t *testing.T) {
	t.Helper()
	f.parentHost.Stop()
	_ = f.control.Close()
	f.parent.reopen(t)
	addr := strings.TrimPrefix(string(f.binding.Command.ParentAddress), "https://")
	l, err := net.Listen("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = l.Close() })
	f.parentConfig.TransitionControl = f.parent.c.Transfers()
	f.parentConfig.TransitionParent = f.parent.c.Transfers()
	f.parentHost, err = coregrpc.New(f.parent.c.Access(), composition.RealmNetworkAuthentication{Connections: f.parent.realmLinks(t), Owner: f.parent.c.Access()}, f.parentConfig)
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = f.parentHost.Serve(l) }()
	f.control, err = coresdk.New(coresdk.Config{NextHop: f.binding.Command.ParentAddress, Expected: f.binding.Parent,
		Roots: f.parentPool, SPKISHA256: f.parentPin,
		RPCDeadline: 5e9, ConnectTimeout: 1e9, MaxResponseBytes: 128 << 10, MaxMetadataBytes: 8192})
	if err != nil {
		t.Fatal(err)
	}
	if err := f.control.Establish(context.Background(), actor.Realm, [16]byte{62}, [32]byte{63}); err != nil {
		t.Fatal(err)
	}
}

// TestNetworkCarrierThreePartyRejectsRevokedApproval verifies that a revoked
// source or target approval prevents the recovery from settling.
func TestNetworkCarrierThreePartyRejectsRevokedApproval(t *testing.T) {
	t.Run("revoked-source-approval", func(t *testing.T) {
		f := newCarrierThreeParty(t)
		ctx := context.Background()
		sourceToken, targetToken := f.approve(t, 60)
		if _, err := f.sourceUser.CarrierApprovals().RevokeCarrierApproval(ctx, &pb.RevokeCarrierApprovalRequest{Token: sourceToken}); err != nil {
			t.Fatal(err)
		}
		r, err := f.control.Transitions().RecoverCarrierTransition(ctx, &pb.RecoverCarrierTransitionRequest{
			Binding: f.w, SourceToken: sourceToken, TargetToken: targetToken})
		if err != nil {
			t.Fatal(err)
		}
		if r.GetResult() != nil && r.GetResult().GetProgress() == pb.CarrierProgress_CARRIER_SETTLED {
			t.Fatal("revoked source approval still settled")
		}
	})

	t.Run("revoked-target-approval", func(t *testing.T) {
		f := newCarrierThreeParty(t)
		ctx := context.Background()
		sourceToken, targetToken := f.approve(t, 60)
		if _, err := f.targetUser.CarrierApprovals().RevokeCarrierApproval(ctx, &pb.RevokeCarrierApprovalRequest{Token: targetToken}); err != nil {
			t.Fatal(err)
		}
		r, err := f.control.Transitions().RecoverCarrierTransition(ctx, &pb.RecoverCarrierTransitionRequest{
			Binding: f.w, SourceToken: sourceToken, TargetToken: targetToken})
		if err != nil {
			t.Fatal(err)
		}
		if r.GetResult() != nil && r.GetResult().GetProgress() == pb.CarrierProgress_CARRIER_SETTLED {
			t.Fatal("revoked target approval still settled")
		}
	})
}

// TestNetworkCarrierThreePartyRejectsWrongIdentity verifies that only the
// authenticated designated parent may check a retained approval token.
func TestNetworkCarrierThreePartyRejectsWrongIdentity(t *testing.T) {
	f := newCarrierThreeParty(t)
	ctx := context.Background()
	sourceToken, _ := f.approve(t, 60)

	// A user session (no service TLS identity) cannot check a parent-only approval.
	if _, err := f.sourceUser.CarrierApprovals().CheckCarrierApproval(ctx, &pb.CheckCarrierApprovalRequest{
		Binding: f.w, Token: sourceToken}); status.Code(err) != codes.Unauthenticated {
		t.Fatal("user session checked parent-only approval", err)
	}

	// The target's service identity cannot check a source approval (only parent can).
	targetAsParent, err := coresdk.New(coresdk.Config{NextHop: f.binding.Command.SourceAddress, Expected: f.binding.Command.Source,
		Roots: f.sourcePool, SPKISHA256: f.sourcePin,
		Actor: f.binding.Command.Target, ActorCertificate: &f.targetCert,
		RPCDeadline: 5e9, ConnectTimeout: 1e9, MaxResponseBytes: 128 << 10, MaxMetadataBytes: 8192})
	if err != nil {
		t.Fatal(err)
	}
	defer targetAsParent.Close()
	if _, err := targetAsParent.CarrierApprovals().CheckCarrierApproval(ctx, &pb.CheckCarrierApprovalRequest{
		Binding: f.w, Token: sourceToken}); status.Code(err) != codes.Unauthenticated {
		t.Fatal("non-parent service identity checked approval", err)
	}
}

// TestNetworkCarrierThreePartyRejectsChangedBinding verifies that a modified
// binding is rejected after settlement.
func TestNetworkCarrierThreePartyRejectsChangedBinding(t *testing.T) {
	f := newCarrierThreeParty(t)
	ctx := context.Background()
	sourceToken, targetToken := f.approve(t, 60)

	r := f.recoverNetwork(t, sourceToken, targetToken)
	if r.GetResult() == nil || r.GetResult().GetProgress() != pb.CarrierProgress_CARRIER_SETTLED {
		t.Fatalf("setup recovery failed: %+v", r)
	}

	changed := proto.Clone(f.w).(*pb.CarrierApprovalBinding)
	changed.Command.TargetAddress = proto.String("local:other")
	if _, err := f.control.Transitions().RecoverCarrierTransition(ctx, &pb.RecoverCarrierTransitionRequest{
		Binding: changed, SourceToken: sourceToken, TargetToken: targetToken}); err == nil {
		t.Fatal("changed binding accepted")
	}
}

// TestNetworkCarrierThreePartyApprovalCapacity verifies that per-caller
// approval limits are enforced.
func TestNetworkCarrierThreePartyApprovalCapacity(t *testing.T) {
	f := newCarrierThreeParty(t)
	ctx := context.Background()
	for i := 0; i < 4; i++ {
		if _, err := f.sourceUser.CarrierApprovals().CreateCarrierApproval(ctx, &pb.CreateCarrierApprovalRequest{
			Binding: f.w, TtlSeconds: proto.Uint64(60)}); err != nil {
			t.Fatalf("approval %d: %v", i, err)
		}
	}
	if _, err := f.sourceUser.CarrierApprovals().CreateCarrierApproval(ctx, &pb.CreateCarrierApprovalRequest{
		Binding: f.w, TtlSeconds: proto.Uint64(60)}); status.Code(err) != codes.ResourceExhausted {
		t.Fatal("approval capacity not enforced", err)
	}
}

// TestNetworkCarrierThreePartyParentPermissionRequired verifies that recovery
// requires the inspect permission on the parent mount scope.
func TestNetworkCarrierThreePartyParentPermissionRequired(t *testing.T) {
	f := newCarrierThreeParty(t)
	ctx := context.Background()
	sourceToken, targetToken := f.approve(t, 60)

	f.parent.grant(t, actor, contract.PermissionTransitionInspect, values.CoreElementScope(f.binding.Parent, carrierMount), true)
	r, err := f.control.Transitions().RecoverCarrierTransition(ctx, &pb.RecoverCarrierTransitionRequest{
		Binding: f.w, SourceToken: sourceToken, TargetToken: targetToken})
	if err != nil {
		t.Fatal(err)
	}
	if r.GetError() != netpb.Failure_DENIED {
		t.Fatal("revoked inspect permission still recovered", r)
	}
}

// TestNetworkCarrierThreePartyRequiresRepairedSourceContact verifies that the
// parent's retained outgoing carrier contact must first be repaired through its
// public CAS to the actual network source address.
func TestNetworkCarrierThreePartyRequiresRepairedSourceContact(t *testing.T) {
	f := newCarrierThreePartyBeforeContactRepair(t)
	if err := f.beginCarrier(t); err == nil {
		t.Fatal("reserved the operation against a stale local carrier contact")
	}
	if err := f.repairSourceContact(t); err != nil {
		t.Fatal(err)
	}
	if err := f.beginCarrier(t); err != nil {
		t.Fatal("repaired contact still refused", err)
	}
	sourceToken, targetToken := f.approve(t, 60)
	r := f.recoverNetwork(t, sourceToken, targetToken)
	if r.GetResult() == nil || r.GetResult().GetProgress() != pb.CarrierProgress_CARRIER_SETTLED {
		t.Fatalf("recovery after contact repair did not settle: %+v", r)
	}
}

// TestNetworkCarrierThreePartyRejectsExternalMountBoundary verifies that source
// content holding an uncoordinated external mount boundary is refused before any
// withdrawal, and that the refusal leaves the parent, source and target intact.
func TestNetworkCarrierThreePartyRejectsExternalMountBoundary(t *testing.T) {
	f := newCarrierThreeParty(t)
	f.nestedMountInsideCarrierContent(t)
	ctx := context.Background()
	sourceToken, targetToken := f.approve(t, 60)

	r, err := f.control.Transitions().RecoverCarrierTransition(ctx, &pb.RecoverCarrierTransitionRequest{
		Binding: f.w, SourceToken: sourceToken, TargetToken: targetToken})
	if err == nil && r.GetResult() != nil {
		t.Fatalf("recovery settled across an uncoordinated external boundary: %+v", r)
	}
	if err == nil && r.GetError() != netpb.Failure_UNAVAILABLE {
		t.Fatalf("external boundary reported as %v", r.GetError())
	}

	// No source member was withdrawn and the source gate stayed open.
	read, err := f.source.Elements().ReadElement(actor, carrierEntity, nil)
	if err != nil || read.Record == nil || read.Record.Entity == nil {
		t.Fatal("source content withdrawn despite the refusal", read, err)
	}
	mount, err := f.source.Mounts().ReadMount(actor, carrierMount)
	if err != nil || !mount.WriteGateOpen {
		t.Fatal("source write gate closed despite the refusal", mount, err)
	}
	// The parent still points at the source and the target installed nothing.
	control, err := f.parent.c.Mounts().InspectMountControl(context.Background(), f.parent.caller, carrierMount)
	if err != nil || control.Carrier != f.source.AuthorityRef() {
		t.Fatal("parent binding changed despite the refusal", control, err)
	}
	if installed, err := f.target.c.Elements().ReadElement(f.target.caller.Subject, carrierEntity, nil); err == nil && installed.Record != nil {
		t.Fatal("target installed content despite the refusal")
	}
}

// TestNetworkCarrierThreePartySourceAndTargetRestartRetainSettlement verifies
// that independent source and target process restarts preserve the durable
// fence, installation and settled result.
func TestNetworkCarrierThreePartySourceAndTargetRestartRetainSettlement(t *testing.T) {
	f := newCarrierThreeParty(t)
	sourceToken, targetToken := f.approve(t, 60)

	r := f.recoverNetwork(t, sourceToken, targetToken)
	if r.GetResult() == nil || r.GetResult().GetProgress() != pb.CarrierProgress_CARRIER_SETTLED {
		t.Fatalf("initial recovery failed: %+v", r)
	}

	f.restartSourceHost(t)
	f.restartTargetHost(t)

	if _, err := f.source.Elements().ReadElement(actor, carrierEntity, nil); err == nil {
		t.Fatal("source restart revived withdrawn content")
	}
	read, err := f.target.c.Elements().ReadElement(f.target.caller.Subject, carrierEntity, nil)
	if err != nil || read.Record == nil || read.Record.Entity == nil {
		t.Fatal("target restart lost installed content", read, err)
	}
	if read.Record.Entity.ContainingAnchorID != carrierMount {
		t.Fatal("target restart moved installed content")
	}

	r2 := f.recoverNetwork(t, sourceToken, targetToken)
	if r2.GetResult() == nil || r2.GetResult().GetProgress() != pb.CarrierProgress_CARRIER_SETTLED {
		t.Fatal("restart lost the settled result", r2)
	}
	if !proto.Equal(r.GetResult().GetRevision(), r2.GetResult().GetRevision()) {
		t.Fatal("restart changed the settled revision")
	}
}

// TestNetworkCarrierThreePartyRefusesDuringSourceHostOutage verifies that a
// source process outage refuses the recovery without withdrawing source content
// and without a partial installation, and that the operation still settles once
// the source returns and re-approves.
func TestNetworkCarrierThreePartyRefusesDuringSourceHostOutage(t *testing.T) {
	f := newCarrierThreeParty(t)
	ctx := context.Background()
	sourceToken, targetToken := f.approve(t, 60)

	f.sourceHost.Stop()
	if r, err := f.control.Transitions().RecoverCarrierTransition(ctx, &pb.RecoverCarrierTransitionRequest{
		Binding: f.w, SourceToken: sourceToken, TargetToken: targetToken}); err == nil && r.GetResult() != nil {
		t.Fatalf("recovery completed while the source host was down: %+v", r)
	}
	read, err := f.source.Elements().ReadElement(actor, carrierEntity, nil)
	if err != nil || read.Record == nil || read.Record.Entity == nil {
		t.Fatal("source content withdrawn during the outage", read, err)
	}
	mount, err := f.source.Mounts().ReadMount(actor, carrierMount)
	if err != nil || !mount.WriteGateOpen {
		t.Fatal("source write gate closed during the outage", mount, err)
	}
	if installed, err := f.target.c.Elements().ReadElement(f.target.caller.Subject, carrierEntity, nil); err == nil && installed.Record != nil {
		t.Fatal("target installed content during the outage")
	}

	f.restartSourceHost(t)
	// The source process lost its runtime approval; the binding is unchanged.
	sourceToken, targetToken = f.approve(t, 60)
	r := f.recoverNetworkEventually(t, sourceToken, targetToken)
	if r.GetResult().GetProgress() != pb.CarrierProgress_CARRIER_SETTLED {
		t.Fatalf("recovery did not settle after the source returned: %+v", r)
	}
}
