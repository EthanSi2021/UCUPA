package federationconn

import (
	"context"

	"google.golang.org/protobuf/proto"
	netpb "ucupa/shared/bindings/go/protobuf"
	"ucupa/shared/values"

	pb "ucupa/ucupa-core/bindings/go/protobuf"
	"ucupa/ucupa-core/contract"
	federationcontract "ucupa/ucupa-core/contract/federation"
	"ucupa/ucupa-core/transport/grpc/internal/wire"
)

func (p *networkFederationPart) Observe(ctx context.Context, r federationcontract.FederationObserveRequest) (federationcontract.FederationObservationPage, error) {
	if ctx == nil {
		return federationcontract.FederationObservationPage{}, values.PublicInvalid
	}
	c, part, err := p.current()
	if err != nil {
		return federationcontract.FederationObservationPage{}, err
	}
	if part.Preparation == nil {
		return federationcontract.FederationObservationPage{}, values.PublicInvalidContext
	}
	w := &pb.ObserveFederationReadPartRequest{Basis: wire.ControlToken(part.Basis, netpb.TokenKind_READ_RECEIPT), Operation: proto.String(r.Operation)}
	if r.Operation == "begin" {
		if r.Cursor != nil || r.Limit != 0 {
			return federationcontract.FederationObservationPage{}, values.PublicInvalid
		}
	} else if r.Operation == "poll" {
		if r.Cursor == nil || !r.Cursor.Valid() || r.Cursor.Owner != part.Basis.Owner || r.Cursor.Kind != "observation-cursor" || r.Limit == 0 || r.Limit > 256 {
			return federationcontract.FederationObservationPage{}, values.PublicInvalid
		}
		w.Cursor, w.Limit = wire.ControlToken(*r.Cursor, netpb.TokenKind_OBSERVATION_CURSOR), proto.Uint32(uint32(r.Limit))
	} else {
		return federationcontract.FederationObservationPage{}, values.PublicInvalid
	}
	response, err := c.FederationReads().ObserveFederationReadPart(ctx, w)
	if err != nil || response.GetFailure() != 0 {
		return federationcontract.FederationObservationPage{}, federationRemoteError(err, response.GetFailure())
	}
	page := response.GetResult()
	if page == nil {
		return federationcontract.FederationObservationPage{}, values.PublicUnavailable
	}
	cursor, err := wire.FederationToken(page.Cursor, netpb.TokenKind_OBSERVATION_CURSOR, "observation-cursor")
	if err != nil {
		return federationcontract.FederationObservationPage{}, values.PublicUnavailable
	}
	out := federationcontract.FederationObservationPage{Cursor: cursor}
	for _, group := range page.Groups {
		id, e1 := wire.FederationToken(group.ChangeId, netpb.TokenKind_CHANGE_ID, "change-id")
		basis, e2 := wire.FederationToken(group.Basis, netpb.TokenKind_CHANGE_BASIS, "change-basis")
		if e1 != nil || e2 != nil {
			return federationcontract.FederationObservationPage{}, values.PublicUnavailable
		}
		item := federationcontract.FederationChangeGroup{ID: id, Basis: basis}
		terminalCategory := false
		for _, category := range group.Category {
			switch category {
			case pb.Category_SPATIAL:
				item.Categories = append(item.Categories, "spatial")
			case pb.Category_ATTACHMENT:
				item.Categories = append(item.Categories, "attachment")
			case pb.Category_TERMINAL:
				terminalCategory = true
				item.Categories = append(item.Categories, "terminal")
			default:
				return federationcontract.FederationObservationPage{}, values.PublicUnavailable
			}
		}
		// Terminal evidence is one exclusive group: exactly the terminal
		// category and the statement itself, never mixed with ordinary
		// changes and never accepted without its proof.
		if terminalCategory && (len(group.Category) != 1 || group.GetTerminal() == nil) || !terminalCategory && group.GetTerminal() != nil {
			return federationcontract.FederationObservationPage{}, values.PublicUnavailable
		}
		if statement := group.GetTerminal(); statement != nil {
			item.Terminal = &contract.TransitionTerminal{
				Operation: contract.OperationID(statement.GetOperationId()),
				Source:    values.FactAuthorityRef(statement.GetSource()),
				Target:    values.FactAuthorityRef(statement.GetTarget()),
				FinalCut:  statement.GetFinalCut(),
				Settled:   statement.GetSettled(),
				NextHop:   values.NextHop(statement.GetNextHop()),
			}
		}
		out.Groups = append(out.Groups, item)
	}
	if _, _, err := p.current(); err != nil {
		return federationcontract.FederationObservationPage{}, err
	}
	return out, nil
}
