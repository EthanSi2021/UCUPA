package acceptance_test

import (
	"context"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	netpb "ucupa/shared/bindings/go/protobuf"
	"ucupa/shared/values"
	pb "ucupa/ucupa-core/bindings/go/protobuf"
	coresdk "ucupa/ucupa-core/sdk/go"
)

func p6FederationCheckInertCaller(t *testing.T, a *networkFixture, aManifest, bManifest, cManifest p6FederationManifest) {
	t.Helper()
	client := p6FederationClient(t, aManifest)
	if err := client.Establish(context.Background(), a.actor.Realm, a.facetServiceID, a.facetServiceSecret); err != nil {
		t.Fatalf("inert caller must authenticate at A: %v", err)
	}
	handle := p6FederationEnter(t, client, aManifest, networkRoot, pb.Intent_QUERY)
	root := &netpb.Address{Origin: proto.String(string(networkRoot)), Up: proto.Uint64(0), Down: &netpb.ElementPath{}}
	out, err := client.Core().QueryRegion(context.Background(), &pb.QueryRegionRequest{Handle: handle, Start: root, Basis: root,
		Sphere:   &pb.Sphere{Center: &netpb.Vector3{Items: []float64{0, 0, 0}}, Radius: proto.Float64(10)},
		Selector: pb.Selector_BOTH.Enum(), Limit: proto.Uint32(64)})
	if err != nil || out.GetResult() == nil {
		t.Fatalf("A-local caller with no B grant: %v %v", out, err)
	}
	result := out.GetResult()
	if result.GetStatus() != pb.Coverage_COMPLETE {
		t.Fatalf("B's proven denial did not close hidden branch: %v", result)
	}
	for _, source := range result.GetSourceGraph().GetSources() {
		if source.GetOwner() == bManifest.Expected || source.GetOwner() == cManifest.Expected {
			t.Fatal("denied source identity leaked to the original caller")
		}
	}
	for _, match := range result.Matches {
		if match.GetElement() == string(p6FederationNested) || match.GetElement() == string(p6FederationLeaf) {
			t.Fatal("B's independent authorization was bypassed")
		}
	}
}

func p6FederationWaitStopped(t *testing.T, client *coresdk.Client, m p6FederationManifest) {
	p6FederationWaitDisconnected(t, client, m, ".stop")
}

func p6FederationWaitDisconnected(t *testing.T, client *coresdk.Client, m p6FederationManifest, gateSuffix string) {
	t.Helper()
	if m.GatePrefix == "" {
		t.Fatal("missing B host gate prefix")
	}
	t.Logf("create %s on host B to interrupt its endpoint", m.GatePrefix+gateSuffix)
	deadline := time.Now().Add(10 * time.Minute)
	for time.Now().Before(deadline) {
		_, err := client.MountContact().InspectMountContact(context.Background(), &pb.InspectMountContactRequest{
			Element: proto.String(string(p6FederationMount)), Incoming: proto.Bool(true)})
		if status.Code(err) == codes.Unavailable {
			return
		}
		if err != nil {
			t.Fatalf("unexpected B probe failure while awaiting stop: %v", err)
		}
		time.Sleep(500 * time.Millisecond)
	}
	t.Fatal("B product did not stop on the other host")
}

func p6FederationCheckOffline(t *testing.T, client *coresdk.Client, original *pb.QueryRegionRequest) {
	t.Helper()
	request := proto.Clone(original).(*pb.QueryRegionRequest)
	request.Continuation = nil
	request.Limit = proto.Uint32(64)
	out, err := client.Core().QueryRegion(context.Background(), request)
	if err != nil || out.GetResult() == nil || out.GetResult().GetStatus() != pb.Coverage_PARTIAL ||
		out.GetResult().GetGap() != pb.QueryGap_UNRESOLVED_COVERAGE {
		t.Fatalf("offline known cross-machine branch masqueraded as complete/empty: %v %v", out, err)
	}
	for _, match := range out.GetResult().Matches {
		if match.GetElement() == string(p6FederationNested) || match.GetElement() == string(p6FederationLeaf) {
			t.Fatal("offline B/C fact fabricated by entry")
		}
	}
}

func p6FederationWaitRestarted(t *testing.T, m p6FederationManifest) {
	p6FederationWaitReachable(t, m, ".start")
}

func p6FederationWaitReachable(t *testing.T, m p6FederationManifest, gateSuffix string) {
	t.Helper()
	t.Logf("create %s on host B to restore its endpoint", m.GatePrefix+gateSuffix)
	client := p6FederationClient(t, m)
	id, secret := p6RealHostCredential(t, m.CredentialID, m.Secret)
	deadline := time.Now().Add(10 * time.Minute)
	for time.Now().Before(deadline) {
		err := client.Establish(context.Background(), values.RealmID(m.Realm), id, secret)
		if err == nil {
			return
		}
		if status.Code(err) != codes.Unavailable {
			t.Fatalf("unexpected B probe failure while awaiting restart: %v", err)
		}
		time.Sleep(500 * time.Millisecond)
	}
	t.Fatal("B product did not restart on the other host")
}

func p6FederationCheckOldQuery(t *testing.T, client *coresdk.Client, request *pb.QueryRegionRequest) {
	t.Helper()
	out, err := client.Core().QueryRegion(context.Background(), request)
	if err == nil && out.GetResult() != nil {
		t.Fatal("old run-local continuation silently resumed after B restart")
	}
	assertLostFederationLease(t, out.GetFailure(), err)
}
