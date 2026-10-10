package acceptance_test

import (
	"bytes"
	"context"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"

	netpb "ucupa/shared/bindings/go/protobuf"
	"ucupa/shared/values"
	pb "ucupa/ucupa-core/bindings/go/protobuf"
	coresdk "ucupa/ucupa-core/sdk/go"
)

func p6FederationMountPublic(t *testing.T, parent, carrier *coresdk.Client, parentManifest, carrierManifest p6FederationManifest,
	anchor values.ElementID, operation []byte) (*pb.ApplyMountRequest, *pb.ApplyMountResponse) {
	t.Helper()
	ctx := context.Background()
	inspection, err := parent.MountControl().InspectMount(ctx, &pb.InspectMountRequest{Element: proto.String(string(anchor))})
	if err != nil || inspection.GetResult() == nil {
		t.Fatalf("public mount inspection: %v %v", inspection, err)
	}
	command := &pb.PublicMountCommand{Kind: pb.MountCommandKind_MOUNT_ESTABLISH.Enum(), Element: proto.String(string(anchor)),
		ExpectedElement: inspection.GetResult().ElementRevision, CarrierCore: proto.String(carrierManifest.Expected),
		CarrierNextHop: proto.String(carrierManifest.NextHop), ParentNextHop: proto.String(parentManifest.NextHop)}
	described, err := parent.MountControl().DescribeMount(ctx, &pb.DescribeMountRequest{OperationId: operation, Command: command})
	if err != nil || !described.GetResult().GetRequired() {
		t.Fatalf("public mount description: %v %v", described, err)
	}
	approved, err := carrier.MountApprovals().CreateMountApproval(ctx, &pb.CreateMountApprovalRequest{
		Binding: described.GetResult().Binding, TtlSeconds: proto.Uint64(30)})
	if err != nil || approved.GetResult() == nil {
		t.Fatalf("independent carrier approval: %v %v", approved, err)
	}
	request := &pb.ApplyMountRequest{OperationId: operation, Command: command, Approval: approved.GetResult()}
	deadline := time.Now().Add(15 * time.Second)
	for {
		out, err := parent.MountControl().ApplyMount(ctx, request)
		if err == nil && out.GetResult() != nil && out.GetResult().GetDecision() == pb.MountDecisionKind_MOUNT_COMMITTED && out.GetResult().GetSettled() {
			return request, out
		}
		if time.Now().After(deadline) || err == nil && out.GetFailure() != netpb.Failure_UNAVAILABLE {
			t.Fatalf("public mount application: %v %v", out, err)
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func p6FederationRecheckMount(t *testing.T, client *coresdk.Client, request *pb.ApplyMountRequest, original *pb.ApplyMountResponse) {
	t.Helper()
	retry := proto.Clone(request).(*pb.ApplyMountRequest)
	retry.Approval = nil // A durable decision never requires a fresh B approval.
	out, err := client.MountControl().ApplyMount(context.Background(), retry)
	if err != nil || !proto.Equal(out, original) {
		t.Fatalf("original OperationID mount result changed across restart: %v %v", out, err)
	}
}

func p6FederationQuery(t *testing.T, client *coresdk.Client, a, b, c p6FederationManifest) *pb.QueryRegionRequest {
	t.Helper()
	ctx := context.Background()
	handle := p6FederationEnter(t, client, a, networkRoot, pb.Intent_QUERY)
	root := &netpb.Address{Origin: proto.String(string(networkRoot)), Up: proto.Uint64(0), Down: &netpb.ElementPath{}}
	request := &pb.QueryRegionRequest{Handle: handle, Start: root, Basis: root,
		Sphere:   &pb.Sphere{Center: &netpb.Vector3{Items: []float64{0, 0, 0}}, Radius: proto.Float64(10)},
		Selector: pb.Selector_BOTH.Enum(), Limit: proto.Uint32(1)}
	seen := map[string]bool{}
	var firstGraph *pb.PublicSourceGraph
	var firstClosure []byte
	var firstContinuation *netpb.Token
	var nestedAddress *netpb.Address
	finished := false
	for page := 0; page < 12; page++ {
		out, err := client.Core().QueryRegion(ctx, request)
		if err != nil || out.GetResult() == nil {
			t.Fatalf("public federated QueryRegion page %d: %v %v", page, out, err)
		}
		result := out.GetResult()
		if result.GetGap() != pb.QueryGap_QUERY_GAP_UNSPECIFIED || result.SourceGraph == nil || len(result.SourceGraph.Sources) != 3 || len(result.Closure) != 32 {
			t.Fatalf("nested cross-machine source was not complete on page %d: %v", page, result)
		}
		if page == 0 {
			firstGraph = proto.Clone(result.SourceGraph).(*pb.PublicSourceGraph)
			firstClosure = bytes.Clone(result.Closure)
			firstContinuation = result.Continuation
			if firstContinuation == nil {
				t.Fatal("query did not exercise continuation")
			}
		} else if !proto.Equal(firstGraph, result.SourceGraph) || !bytes.Equal(firstClosure, result.Closure) {
			t.Fatal("source graph or closure changed across public pages")
		}
		for _, source := range result.SourceGraph.Sources {
			if source.Coverage != nil && len(source.Parents) == 0 {
				t.Fatal("query source lacks parent closure")
			}
		}
		for _, match := range result.Matches {
			if seen[match.GetElement()] {
				t.Fatal("duplicate element across query pages")
			}
			seen[match.GetElement()] = true
			if match.GetElement() == string(p6FederationLeaf) {
				nestedAddress = match.Address
				if match.Revision.GetOwner() != c.Expected {
					t.Fatal("nested fact owner changed at entry")
				}
			}
		}
		if result.Continuation == nil {
			if result.GetStatus() != pb.Coverage_COMPLETE {
				t.Fatalf("final nested query page was incomplete: %v", result)
			}
			finished = true
			break
		}
		request.Continuation = result.Continuation
	}
	if !finished || !seen[string(p6FederationMount)] || !seen[string(p6FederationNested)] || !seen[string(p6FederationLeaf)] || nestedAddress == nil {
		t.Fatalf("public query omitted A/B/C path: %v", seen)
	}
	owners := map[string]bool{}
	for _, source := range firstGraph.Sources {
		owners[source.GetOwner()] = true
	}
	if !owners[a.Expected] || !owners[b.Expected] || !owners[c.Expected] {
		t.Fatalf("source graph omitted an owner: %v", owners)
	}
	read, err := client.Core().ReadElement(ctx, &pb.ReadElementRequest{Handle: handle, Address: nestedAddress})
	if err != nil || read.GetResult().GetRecord().GetElement() != string(p6FederationLeaf) || read.GetResult().GetRecord().GetRevision().GetOwner() != c.Expected {
		t.Fatalf("nested public ReadElement: %v %v", read, err)
	}
	listAddress := &netpb.Address{Origin: proto.String(string(networkRoot)), Down: &netpb.ElementPath{Items: []string{string(p6FederationMount)}}}
	list := &pb.ListDirectRequest{Handle: p6FederationEnter(t, client, a, networkRoot, pb.Intent_LIST), Address: listAddress, Limit: proto.Uint32(1)}
	first, err := client.Core().ListDirect(ctx, list)
	if err != nil || first.GetResult() == nil || first.GetResult().Continuation == nil || len(first.GetResult().Records) != 1 {
		t.Fatalf("public ListDirect first page: %v %v", first, err)
	}
	list.Continuation = first.GetResult().Continuation
	second, err := client.Core().ListDirect(ctx, list)
	if err != nil || second.GetResult() == nil || len(second.GetResult().Records) == 0 || !proto.Equal(first.GetResult().SourceGraph, second.GetResult().SourceGraph) ||
		!bytes.Equal(first.GetResult().Closure, second.GetResult().Closure) {
		t.Fatalf("public ListDirect continuation: %v %v", second, err)
	}
	if first.GetResult().Records[0].GetElement() >= second.GetResult().Records[0].GetElement() {
		t.Fatal("public list repeated or reordered records")
	}
	// Structural depth counts anchors, not transports: starting at C's
	// incoming anchor and ascending once selects B's parent mount, although
	// both A→B and B→C are network calls in this two-machine layout.
	nestedStart := &netpb.Address{Origin: proto.String(string(networkRoot)), Down: &netpb.ElementPath{
		Items: []string{string(p6FederationMount), string(p6FederationNested)}}}
	parentBasis := &netpb.Address{Origin: proto.String(string(networkRoot)), Down: &netpb.ElementPath{
		Items: []string{string(p6FederationMount)}}}
	depth, err := client.Core().QueryRegion(ctx, &pb.QueryRegionRequest{Handle: handle, Start: nestedStart, Basis: parentBasis,
		Sphere:  &pb.Sphere{Center: &netpb.Vector3{Items: []float64{0, 0, 0}}, Radius: proto.Float64(10)},
		UpDepth: proto.Int64(1), Selector: pb.Selector_BOTH.Enum(), Limit: proto.Uint32(64)})
	if err != nil || depth.GetResult() == nil || depth.GetResult().GetScope() != string(p6FederationMount) ||
		depth.GetResult().GetActualUpDepth() != 1 || depth.GetResult().GetStatus() != pb.Coverage_COMPLETE {
		t.Fatalf("network hop changed structural depth: %v %v", depth, err)
	}
	foundLeaf := false
	for _, match := range depth.GetResult().Matches {
		foundLeaf = foundLeaf || match.GetElement() == string(p6FederationLeaf)
	}
	if !foundLeaf {
		t.Fatal("depth-one nested query lost C's descendant")
	}
	request.Continuation = firstContinuation
	return request
}
