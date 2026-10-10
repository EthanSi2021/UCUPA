package coresdk

import (
	"google.golang.org/protobuf/proto"
	"testing"
	pb "ucupa/ucupa-core/bindings/go/protobuf"
)

func TestParentEvidenceRequiresClosedOrderedShape(t *testing.T) {
	good := &pb.ParentAdjacencyEvidence{Parent: proto.String(fedElement), Adjacency: publicTestStamp()}
	if err := validateParentEvidence([]*pb.ParentAdjacencyEvidence{good}, fedElement, true); err != nil {
		t.Fatal(err)
	}
	for _, bad := range [][]*pb.ParentAdjacencyEvidence{
		nil, {nil}, {good, good}, {{Parent: good.Parent}}, {{Parent: proto.String("not-an-id"), Adjacency: good.Adjacency}},
	} {
		if err := validateParentEvidence(bad, fedElement, true); err == nil {
			t.Fatal("malformed parent proof accepted")
		}
	}
	if err := validateParentEvidence([]*pb.ParentAdjacencyEvidence{good}, string(fedOwner), true); err == nil {
		t.Fatal("scope not covered")
	}
}
