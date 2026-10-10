package admission

import (
	federationcontract "ucupa/ucupa-core/contract/federation"

	"context"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	netpb "ucupa/shared/bindings/go/protobuf"

	pb "ucupa/ucupa-core/bindings/go/protobuf"
	"ucupa/ucupa-core/transport/grpc/internal/rpcfailure"
	"ucupa/ucupa-core/transport/grpc/internal/wire"
)

func (s *Service) ObserveFederationReadPart(ctx context.Context, in *pb.ObserveFederationReadPartRequest) (*pb.ObserveFederationReadPartResponse, error) {
	o, g, key, err := s.federationCaller(ctx, in.GetBasis())
	if err != nil {
		return nil, err
	}
	if g.federation.Preparation == nil {
		return nil, status.Error(codes.PermissionDenied, "observation not prepared")
	}
	r := federationcontract.FederationObserveRequest{Operation: in.GetOperation()}
	if r.Operation == "begin" {
		if in.Cursor != nil || in.Limit != nil {
			return nil, status.Error(codes.InvalidArgument, "invalid observation begin")
		}
	} else if r.Operation == "poll" {
		if in.GetLimit() == 0 || in.GetLimit() > 256 {
			return nil, status.Error(codes.InvalidArgument, "invalid observation limit")
		}
		cursor, e := wire.FederationToken(in.Cursor, netpb.TokenKind_OBSERVATION_CURSOR, "observation-cursor")
		if e != nil || cursor.Owner != s.config.Expected {
			return nil, status.Error(codes.InvalidArgument, "invalid observation cursor")
		}
		r.Cursor, r.Limit = &cursor, uint16(in.GetLimit())
	} else {
		return nil, status.Error(codes.InvalidArgument, "invalid observation operation")
	}
	basis, err := wire.FederationToken(in.Basis, netpb.TokenKind_READ_RECEIPT, "read-receipt")
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid read basis")
	}
	page, err := o.ObserveFederationReadPart(ctx, g.caller, g.actor, basis, r)
	out := &pb.ObserveFederationReadPartResponse{}
	if err != nil {
		failure, e := rpcfailure.MountPeer(err)
		if e != nil {
			return nil, e
		}
		out.Outcome = &pb.ObserveFederationReadPartResponse_Failure{Failure: failure}
	} else {
		w := &pb.FederationObservationPage{Cursor: wire.ControlToken(page.Cursor, netpb.TokenKind_OBSERVATION_CURSOR)}
		for _, group := range page.Groups {
			item := &pb.ChangeGroup{ChangeId: wire.ControlToken(group.ID, netpb.TokenKind_CHANGE_ID), Basis: wire.ControlToken(group.Basis, netpb.TokenKind_CHANGE_BASIS), Owner: proto.String(string(s.config.Expected))}
			terminalCategory := false
			for _, category := range group.Categories {
				switch category {
				case "spatial":
					item.Category = append(item.Category, pb.Category_SPATIAL)
				case "attachment":
					item.Category = append(item.Category, pb.Category_ATTACHMENT)
				case "terminal":
					terminalCategory = true
					item.Category = append(item.Category, pb.Category_TERMINAL)
				default:
					return nil, status.Error(codes.Internal, "invalid observation category")
				}
			}
			// Terminal evidence is one exclusive group: exactly the terminal
			// category and the statement itself, never mixed with ordinary
			// changes and never asserted without its proof.
			if terminalCategory && (len(group.Categories) != 1 || group.Terminal == nil) || !terminalCategory && group.Terminal != nil {
				return nil, status.Error(codes.Internal, "invalid terminal evidence group")
			}
			if group.Terminal != nil {
				item.Terminal = &pb.TransitionTerminal{
					OperationId: []byte(group.Terminal.Operation),
					Source:      proto.String(string(group.Terminal.Source)),
					Target:      proto.String(string(group.Terminal.Target)),
					FinalCut:    proto.Uint64(group.Terminal.FinalCut),
					Settled:     proto.Bool(group.Terminal.Settled),
					NextHop:     proto.String(string(group.Terminal.NextHop)),
				}
			}
			w.Groups = append(w.Groups, item)
		}
		out.Outcome = &pb.ObserveFederationReadPartResponse_Result{Result: w}
	}
	if err := s.finishFederationCall(ctx, g, key, out); err != nil {
		return nil, err
	}
	return out, nil
}
