package composition

import (
	"context"
	"strings"

	"google.golang.org/protobuf/proto"
	networkpb "ucupa/shared/bindings/go/protobuf"
	"ucupa/shared/values"
	corepb "ucupa/ucupa-core/bindings/go/protobuf"
)

func controlWireToken(v any, owner values.FactAuthorityRef, kind networkpb.TokenKind) (*networkpb.Token, error) {
	m, ok := v.(map[string]any)
	if !ok || len(m) != 3 || m["owner"] != string(owner) || m["kind"] != strings.ReplaceAll(strings.ToLower(kind.String()), "_", "-") {
		return nil, values.PublicInvalid
	}
	s, ok := m["value"].(string)
	if !ok {
		return nil, values.PublicInvalid
	}
	b, err := values.DecodePublicBytes(s)
	if err != nil || len(b) != 32 {
		return nil, values.PublicInvalid
	}
	return &networkpb.Token{Owner: proto.String(string(owner)), Kind: kind.Enum(), Value: b}, nil
}

func controlJSONToken(t *networkpb.Token) map[string]any {
	return map[string]any{"owner": t.GetOwner(), "kind": strings.ReplaceAll(strings.ToLower(t.GetKind().String()), "_", "-"), "value": values.EncodePublicBytes(t.Value)}
}

// ControlCall adapts only the three existing service observation operations.
// No arbitrary JSON RPC is sent over the network or accepted from a user.
func (s *NetworkFacetCoreSource) ControlCall(ctx context.Context, address values.NextHop, key values.FacetAcceptanceKey, raw []byte) ([]byte, error) {
	c, err := s.peer(ctx, address, key)
	if err != nil {
		return nil, err
	}
	v, err := values.ParseCanonicalJSON(raw, values.PublicJSONLimits())
	if err != nil {
		return nil, values.PublicInvalid
	}
	m, ok := v.(map[string]any)
	if !ok || len(m) != 2 {
		return nil, values.PublicInvalid
	}
	var result map[string]any
	var failure networkpb.Failure
	switch m["op"] {
	case "begin-observe":
		token, e := controlWireToken(m["preparation"], key.Core, networkpb.TokenKind_OBSERVATION_PREPARATION)
		if e != nil {
			return nil, e
		}
		r, e := c.Core().BeginObserve(ctx, &corepb.BeginObserveRequest{Preparation: token})
		if e != nil || r == nil {
			return nil, values.PublicUnavailable
		}
		failure = r.GetFailure()
		if r.GetResult() != nil {
			result = map[string]any{"cursor": controlJSONToken(r.GetResult().Cursor)}
		}
	case "poll":
		token, e := controlWireToken(m["cursor"], key.Core, networkpb.TokenKind_OBSERVATION_CURSOR)
		if e != nil {
			return nil, e
		}
		r, e := c.Core().PollChanges(ctx, &corepb.PollChangesRequest{Cursor: token})
		if e != nil || r == nil {
			return nil, values.PublicUnavailable
		}
		failure = r.GetFailure()
		if r.GetResult() != nil {
			groups := make([]any, 0, len(r.GetResult().Groups))
			for _, g := range r.GetResult().Groups {
				categories := make([]any, len(g.Category))
				for i, category := range g.Category {
					categories[i] = strings.ToLower(category.String())
				}
				groups = append(groups, map[string]any{"changeId": controlJSONToken(g.ChangeId), "owner": g.GetOwner(), "category": categories, "basis": controlJSONToken(g.Basis)})
			}
			result = map[string]any{"groups": groups, "nextCursor": controlJSONToken(r.GetResult().NextCursor)}
		}
	case "release-observe":
		token, e := controlWireToken(m["preparation"], key.Core, networkpb.TokenKind_OBSERVATION_PREPARATION)
		if e != nil {
			return nil, e
		}
		r, e := c.Core().ReleaseObserve(ctx, &corepb.ReleaseObserveRequest{Context: &corepb.ReleaseObserveRequest_Preparation{Preparation: token}})
		if e != nil || r == nil {
			return nil, values.PublicUnavailable
		}
		failure = r.GetFailure()
		if r.GetResult() != nil {
			result = map[string]any{"released": r.GetResult().GetReleased()}
		}
	default:
		return nil, values.PublicInvalid
	}
	if failure != 0 {
		return nil, values.PublicError(failure.String())
	}
	if result == nil {
		return nil, values.PublicUnavailable
	}
	return values.CanonicalJSONValue(result)
}
