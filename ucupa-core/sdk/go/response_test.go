package coresdk

import (
	"google.golang.org/protobuf/proto"
	"testing"
	networkpb "ucupa/shared/bindings/go/protobuf"
	"ucupa/shared/values"
	corepb "ucupa/ucupa-core/bindings/go/protobuf"
)

func TestMalformedSuccessResponsesAreNotAccepted(t *testing.T) {
	const owner = "00000000-0000-4000-8000-000000000001"
	for _, message := range []proto.Message{
		&corepb.CloseResponse{},
		&corepb.CloseResponse{Outcome: &corepb.CloseResponse_Result{Result: &corepb.CloseResult{Closed: proto.Bool(false)}}},
		&corepb.CloseResponse{Outcome: &corepb.CloseResponse_Failure{Failure: networkpb.Failure(999)}},
		&networkpb.Token{Owner: proto.String(owner), Kind: networkpb.TokenKind_READ_RECEIPT.Enum(), Value: []byte{1}},
		&networkpb.Vector3{Items: []float64{1, 2}},
		&corepb.ReadElementResult{Receipt: &networkpb.Token{Owner: proto.String(owner), Kind: networkpb.TokenKind_READ_RECEIPT.Enum(), Value: make([]byte, 32)}, ReadMode: corepb.ReadMode_CURRENT_VERIFIED.Enum(), State: corepb.Presence_PRESENT.Enum()},
	} {
		if validateResponse(message.ProtoReflect(), 0) == nil {
			t.Fatalf("accepted malformed %T", message)
		}
	}
	token := &networkpb.Token{Owner: proto.String(owner), Kind: networkpb.TokenKind_READ_RECEIPT.Enum(), Value: make([]byte, 32)}
	if validateTokenOwners(token.ProtoReflect(), values.FactAuthorityRef("00000000-0000-4000-8000-000000000002")) == nil {
		t.Fatal("accepted wrong owner")
	}
	valid := &corepb.CloseResponse{Outcome: &corepb.CloseResponse_Result{Result: &corepb.CloseResult{Closed: proto.Bool(true)}}}
	if err := validateResponse(valid.ProtoReflect(), 0); err != nil {
		t.Fatal(err)
	}
}
