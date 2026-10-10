package wire

import (
	"google.golang.org/protobuf/proto"
	networkpb "ucupa/shared/bindings/go/protobuf"
	"ucupa/shared/values"
)

func ControlToken(t values.PublicToken, kind networkpb.TokenKind) *networkpb.Token {
	if t == (values.PublicToken{}) {
		return nil
	}
	b, _ := values.DecodePublicBytes(t.Value)
	return &networkpb.Token{Owner: proto.String(string(t.Owner)), Kind: kind.Enum(), Value: b}
}
