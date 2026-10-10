package coresdk

import (
	"google.golang.org/protobuf/proto"
	"testing"
	netpb "ucupa/shared/bindings/go/protobuf"
	pb "ucupa/ucupa-core/bindings/go/protobuf"
)

func TestFederationObservationResponseClosure(t *testing.T) {
	for _, name := range []string{"valid", "hidden groups", "basis", "not prepared", "too many", "owner", "category", "duplicate", "begin groups", "missing cursor"} {
		t.Run(name, func(t *testing.T) {
			c := fedClient()
			c.federation = fedPart()
			c.federation.Preparation = fedToken(netpb.TokenKind_OBSERVATION_PREPARATION)
			r := &pb.ObserveFederationReadPartRequest{Basis: proto.Clone(c.federation.Basis).(*netpb.Token), Operation: proto.String("poll"), Cursor: fedToken(netpb.TokenKind_OBSERVATION_CURSOR), Limit: proto.Uint32(1)}
			g := &pb.ChangeGroup{ChangeId: fedToken(netpb.TokenKind_CHANGE_ID), Basis: fedToken(netpb.TokenKind_CHANGE_BASIS), Owner: proto.String(string(fedOwner)), Category: []pb.Category{pb.Category_SPATIAL}}
			p := &pb.FederationObservationPage{Cursor: fedToken(netpb.TokenKind_OBSERVATION_CURSOR), Groups: []*pb.ChangeGroup{g}}
			switch name {
			case "hidden groups":
				c.federation.Hidden = proto.Bool(true)
			case "basis":
				r.Basis.Value[0] = 1
			case "not prepared":
				c.federation.Preparation = nil
			case "too many":
				p.Groups = append(p.Groups, proto.Clone(g).(*pb.ChangeGroup))
			case "owner":
				g.Owner = proto.String(string(fedActor))
			case "category":
				g.Category = []pb.Category{pb.Category_CONTENT}
			case "duplicate":
				g.Category = append(g.Category, pb.Category_SPATIAL)
			case "begin groups":
				r.Operation = proto.String("begin")
				r.Cursor = nil
				r.Limit = nil
			case "missing cursor":
				p.Cursor = nil
			}
			out := &pb.ObserveFederationReadPartResponse{Outcome: &pb.ObserveFederationReadPartResponse_Result{Result: p}}
			if err := c.validateFederationExchange(r, out); (err == nil) != (name == "valid") {
				t.Fatal(name, err)
			}
		})
	}
}
