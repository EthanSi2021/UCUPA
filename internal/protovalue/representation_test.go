package protovalue

import (
	"bytes"
	"google.golang.org/protobuf/proto"
	"testing"
	networkpb "ucupa/shared/bindings/go/protobuf"
	"ucupa/shared/values"
)

func TestTokenRoundTripAndNoUnknownFields(t *testing.T) {
	c := Codec{}
	msg := &networkpb.Token{Owner: proto.String("00000000-0000-4000-8000-000000000001"), Kind: networkpb.TokenKind_READ_RECEIPT.Enum(), Value: make([]byte, 32)}
	value, err := c.ToValue(msg.ProtoReflect(), 0)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := values.CanonicalJSONValue(value)
	if err != nil {
		t.Fatal(err)
	}
	want := []byte(`{"kind":"read-receipt","owner":"00000000-0000-4000-8000-000000000001","value":"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"}`)
	if !bytes.Equal(encoded, want) {
		t.Fatalf("representation %s", encoded)
	}
	var decoded networkpb.Token
	if err := c.FromValue(decoded.ProtoReflect(), value); err != nil || !proto.Equal(msg, &decoded) {
		t.Fatalf("roundtrip %v %v", &decoded, err)
	}
	object := value.(map[string]any)
	object["private"] = "hidden"
	if c.FromValue(new(networkpb.Token).ProtoReflect(), object) == nil {
		t.Fatal("unknown canonical field accepted")
	}
	msg.ProtoReflect().SetUnknown([]byte{0x98, 0x06, 0x01})
	if _, err := c.ToValue(msg.ProtoReflect(), 0); err == nil {
		t.Fatal("unknown wire field accepted")
	}
}

func TestCommonShapeRejectsInvalidTokenAndRole(t *testing.T) {
	c := Codec{}
	for _, msg := range []*networkpb.Token{
		{},
		{Owner: proto.String("not-an-owner"), Kind: networkpb.TokenKind_READ_RECEIPT.Enum(), Value: make([]byte, 32)},
		{Owner: proto.String("00000000-0000-4000-8000-000000000001"), Kind: networkpb.TokenKind(777).Enum(), Value: make([]byte, 32)},
		{Owner: proto.String("00000000-0000-4000-8000-000000000001"), Kind: networkpb.TokenKind_READ_RECEIPT.Enum(), Value: make([]byte, 31)},
	} {
		if _, err := c.ToValue(msg.ProtoReflect(), 0); err == nil {
			t.Fatalf("invalid token accepted %v", msg)
		}
	}
	id := proto.String("00000000-0000-4000-8000-000000000001")
	key := &networkpb.AttachmentKey{Core: id, Facet: id, Element: id, Proposal: &networkpb.Token{Owner: id, Kind: networkpb.TokenKind_READ_RECEIPT.Enum(), Value: make([]byte, 32)}}
	if _, err := c.ToValue(key.ProtoReflect(), 0); err == nil {
		t.Fatal("wrong proposal role accepted")
	}
	key.Proposal.Kind = networkpb.TokenKind_ATTACHMENT_REVISION.Enum()
	if _, err := c.ToValue(key.ProtoReflect(), 0); err != nil {
		t.Fatal(err)
	}
	if _, err := c.ToValue(key.ProtoReflect(), 33); err == nil {
		t.Fatal("depth bound ignored")
	}
}
