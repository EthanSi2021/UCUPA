package coresdk

import (
	"google.golang.org/protobuf/proto"
	"testing"
	pb "ucupa/ucupa-core/bindings/go/protobuf"
)

func publicTestStamp() []byte { return append([]byte{1}, make([]byte, 31)...) }
func publicTestGraph(operation string) *pb.PublicSourceGraph {
	principal := &pb.ServicePrincipal{Realm: proto.String("realm"), Principal: proto.String("caller")}
	a := &pb.PublicReadSource{Owner: proto.String(string(fedOwner)), Boundary: proto.String(fedElement), Principal: principal, Cut: publicTestStamp(), Authorization: publicTestStamp(), ReadMode: pb.ReadMode_CURRENT_VERIFIED.Enum()}
	b := proto.Clone(a).(*pb.PublicReadSource)
	b.Owner = proto.String(string(fedActor))
	b.Parent = &pb.PublicSourceParent{Owner: a.Owner, Anchor: b.Boundary, Delegation: publicTestStamp(), Acceptance: publicTestStamp()}
	if operation == "query" {
		a.QueryScope, a.Coverage = a.Boundary, publicTestStamp()
		b.QueryScope, b.Coverage = b.Boundary, publicTestStamp()
		a.Parents = []*pb.ParentAdjacencyEvidence{{Parent: a.Boundary, Adjacency: publicTestStamp()}}
		b.Parents = []*pb.ParentAdjacencyEvidence{{Parent: b.Boundary, Adjacency: publicTestStamp()}}
	}
	return &pb.PublicSourceGraph{Purpose: publicTestStamp(), Operation: proto.String(operation), Sources: []*pb.PublicReadSource{a, b}}
}

func TestPublicSourceGraphClosedShape(t *testing.T) {
	for name, change := range map[string]func(*pb.PublicSourceGraph){
		"empty":      func(g *pb.PublicSourceGraph) { g.Sources = nil },
		"duplicate":  func(g *pb.PublicSourceGraph) { g.Sources[1].Owner = g.Sources[0].Owner },
		"cycle":      func(g *pb.PublicSourceGraph) { g.Sources[1].Parent.Owner = g.Sources[1].Owner },
		"wrong root": func(g *pb.PublicSourceGraph) { g.Sources[0].Owner = proto.String(string(fedActor)) },
		"cut":        func(g *pb.PublicSourceGraph) { g.Sources[1].Cut = make([]byte, 32) },
		"principal":  func(g *pb.PublicSourceGraph) { g.Sources[1].Principal.Principal = proto.String("other") },
		"contact":    func(g *pb.PublicSourceGraph) { g.Sources[1].Parent.Acceptance = nil },
		"anchor":     func(g *pb.PublicSourceGraph) { g.Sources[1].Parent.Anchor = proto.String(string(fedOwner)) },
		"no scan": func(g *pb.PublicSourceGraph) {
			for _, s := range g.Sources {
				s.Coverage = nil
				s.QueryScope = nil
			}
		},
		"half scan":    func(g *pb.PublicSourceGraph) { g.Sources[1].Coverage = nil },
		"zero purpose": func(g *pb.PublicSourceGraph) { g.Purpose = make([]byte, 32) },
	} {
		t.Run(name, func(t *testing.T) {
			g := publicTestGraph("query")
			change(g)
			if _, err := validatePublicSourceGraph(g, publicTestStamp(), fedOwner); err == nil {
				t.Fatal("malformed graph accepted")
			}
		})
	}
	if _, err := validatePublicSourceGraph(nil, publicTestStamp(), fedOwner); err == nil {
		t.Fatal("closure without evidence")
	}
	if _, err := validatePublicSourceGraph(publicTestGraph("query"), nil, fedOwner); err == nil {
		t.Fatal("graph without closure")
	}
	if owners, err := validatePublicSourceGraph(publicTestGraph("query"), publicTestStamp(), fedOwner); err != nil || len(owners) != 2 {
		t.Fatal(owners, err)
	}
}
