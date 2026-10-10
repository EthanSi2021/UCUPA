package facetsdk

import (
	"google.golang.org/protobuf/proto"
	"testing"
	networkpb "ucupa/shared/bindings/go/protobuf"
	"ucupa/shared/values"
	facetpb "ucupa/ucupa-facet/bindings/go/protobuf"
)

const responseOwner values.FactAuthorityRef = "00000000-0000-4000-8000-000000000001"

func responseToken(k networkpb.TokenKind) *networkpb.Token {
	return &networkpb.Token{Owner: proto.String(string(responseOwner)), Kind: k.Enum(), Value: make([]byte, 32)}
}
func TestFacetSDKRejectsMalformedSuccess(t *testing.T) {
	for _, m := range []proto.Message{
		&facetpb.GetResponse{},
		&facetpb.GetResponse{Outcome: &facetpb.GetResponse_Failure{Failure: networkpb.Failure_FAILURE_UNSPECIFIED}},
		&facetpb.GetResult{Receipt: responseToken(networkpb.TokenKind_READ_RECEIPT), State: facetpb.Presence_PRESENT.Enum(), TypeRef: make([]byte, 32)},
		&facetpb.GetResult{Receipt: responseToken(networkpb.TokenKind_READ_RECEIPT), State: facetpb.Presence_NOT_FOUND.Enum(), TypeRef: make([]byte, 32), Payload: []byte{}},
		&facetpb.GetResult{Receipt: responseToken(networkpb.TokenKind_OBSERVATION_CURSOR), State: facetpb.Presence_NOT_FOUND.Enum(), TypeRef: make([]byte, 32)},
		&facetpb.ReleaseObserveResult{Released: proto.Bool(false)},
		&facetpb.ListTypesResult{Receipt: responseToken(networkpb.TokenKind_READ_RECEIPT), Types: [][]byte{{1}}},
		&facetpb.ChangeGroup{ChangeId: responseToken(networkpb.TokenKind_CHANGE_ID), Owner: proto.String(string(responseOwner)), Category: []facetpb.Category{facetpb.Category_CONTENT, facetpb.Category_CONTENT}, Basis: responseToken(networkpb.TokenKind_CHANGE_BASIS)},
	} {
		if validateResponse(m.ProtoReflect(), responseOwner, 0) == nil {
			t.Errorf("accepted %T", m)
		}
	}
	valid := &facetpb.GetResult{Receipt: responseToken(networkpb.TokenKind_READ_RECEIPT), State: facetpb.Presence_NOT_FOUND.Enum(), TypeRef: make([]byte, 32)}
	if err := validateResponse(valid.ProtoReflect(), responseOwner, 0); err != nil {
		t.Fatal(err)
	}
	valid.Receipt.Owner = proto.String("00000000-0000-4000-8000-000000000099")
	if validateResponse(valid.ProtoReflect(), responseOwner, 0) == nil {
		t.Fatal("foreign owner accepted")
	}
	valid.Receipt = responseToken(networkpb.TokenKind_READ_RECEIPT)
	valid.Receipt.ProtoReflect().SetUnknown([]byte{0x98, 0x06, 0x01})
	if validateResponse(valid.ProtoReflect(), responseOwner, 0) == nil {
		t.Fatal("unknown nested field accepted")
	}
}
