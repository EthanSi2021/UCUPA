package coresdk

import (
	"google.golang.org/protobuf/proto"
	"testing"
	pb "ucupa/ucupa-core/bindings/go/protobuf"
)

func TestQueryMatchNeedsEveryScopeLocalParent(t *testing.T) {
	root, parent, leaf := string(fedOwner), fedElement, string(fedActor)
	evidence := func(ids ...string) []*pb.ParentAdjacencyEvidence {
		var out []*pb.ParentAdjacencyEvidence
		for _, id := range ids {
			out = append(out, &pb.ParentAdjacencyEvidence{Parent: proto.String(id), Adjacency: publicTestStamp()})
		}
		return out
	}
	path := []string{parent, leaf}
	if err := validateQueryParentPath(root, root, leaf, pb.ElementKind_ENTITY, path, evidence(root, parent)); err != nil {
		t.Fatal(err)
	}
	if err := validateQueryParentPath(parent, root, leaf, pb.ElementKind_ENTITY, path, evidence(parent)); err != nil {
		t.Fatal("upstream prefix incorrectly required", err)
	}
	for _, parents := range [][]*pb.ParentAdjacencyEvidence{nil, evidence(root), evidence(parent)} {
		if err := validateQueryParentPath(root, root, leaf, pb.ElementKind_ENTITY, path, parents); err == nil {
			t.Fatal("missing parent accepted")
		}
	}
	if err := validateQueryParentPath(parent, root, leaf, pb.ElementKind_ENTITY, []string{leaf}, evidence(parent)); err == nil {
		t.Fatal("scope absent from path")
	}
	if err := validateQueryParentPath(root, root, leaf, pb.ElementKind_ENTITY, []string{parent, parent, leaf}, evidence(root, parent)); err == nil {
		t.Fatal("cyclic path")
	}
	if err := validateQueryParentPath(leaf, root, leaf, pb.ElementKind_ENTITY, path, evidence(leaf)); err == nil {
		t.Fatal("entity used as scope")
	}
	if err := validateQueryParentPath(root, root, parent, pb.ElementKind_ANCHOR, []string{parent}, evidence(root)); err == nil {
		t.Fatal("matched anchor missing its adjacency")
	}
}

func TestCompletePublicQueryCannotOmitParentEvidence(t *testing.T) {
	r := &pb.QueryRegionResult{Status: pb.Coverage_COMPLETE.Enum(), SourceGraph: publicTestGraph("query")}
	if err := validatePublicQueryEvidence(r); err != nil {
		t.Fatal(err)
	}
	r.SourceGraph.Sources[1].Parents = nil
	if err := validatePublicQueryEvidence(r); err == nil {
		t.Fatal("complete query without parent evidence")
	}
	r.Status = pb.Coverage_PARTIAL.Enum()
	if err := validatePublicQueryEvidence(r); err != nil {
		t.Fatal("partial exact-work gap requires no fake proof", err)
	}
}
