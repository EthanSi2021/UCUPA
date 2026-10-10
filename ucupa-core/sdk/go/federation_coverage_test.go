package coresdk

import (
	"google.golang.org/protobuf/proto"
	"testing"
	pb "ucupa/ucupa-core/bindings/go/protobuf"
)

func TestFederationQueryRequiresCoverageEvidence(t *testing.T) {
	q := &pb.FederationProjectionRequest{Kind: proto.String("query"), Query: &pb.FederationQueryRequest{Scope: proto.String(fedElement)}}
	for _, value := range [][]byte{nil, {}, make([]byte, 31), make([]byte, 32), make([]byte, 33), append([]byte{1}, make([]byte, 31)...)} {
		p := &pb.FederationProjectionPage{Coverage: value, Parents: []*pb.ParentAdjacencyEvidence{{Parent: proto.String(fedElement), Adjacency: publicTestStamp()}}}
		err := validateFederationProjection(q, p, fedPart(), fedOwner)
		valid := len(value) == 32 && value[0] == 1
		if (err == nil) != valid {
			t.Fatalf("coverage len=%d: %v", len(value), err)
		}
	}
}
