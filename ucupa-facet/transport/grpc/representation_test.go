package facetgrpc

import (
	"bytes"
	"encoding/json"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"os"
	"testing"
	networkpb "ucupa/shared/bindings/go/protobuf"
	"ucupa/shared/values"
	facetpb "ucupa/ucupa-facet/bindings/go/protobuf"
)

func TestPublicCorpusRoundTrip(t *testing.T) {
	raw, err := os.ReadFile("../../api/conformance.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct {
		Name, Schema string
		Value        map[string]any
		Valid        bool
	}
	if err := json.Unmarshal(raw, &cases); err != nil {
		t.Fatal(err)
	}
	requests := map[string]func() proto.Message{
		"get":             func() proto.Message { return new(facetpb.GetRequest) },
		"list-types":      func() proto.Message { return new(facetpb.ListTypesRequest) },
		"put":             func() proto.Message { return new(facetpb.PutRequest) },
		"delete":          func() proto.Message { return new(facetpb.DeleteRequest) },
		"recover-outcome": func() proto.Message { return new(facetpb.RecoverOutcomeRequest) },
		"begin-observe":   func() proto.Message { return new(facetpb.BeginObserveRequest) },
		"poll":            func() proto.Message { return new(facetpb.PollChangesRequest) },
		"release-observe": func() proto.Message { return new(facetpb.ReleaseObserveRequest) },
	}
	responses := []func() proto.Message{
		func() proto.Message { return new(facetpb.GetResult) }, func() proto.Message { return new(facetpb.ListTypesResult) },
		func() proto.Message { return new(facetpb.MutationResult) },
		func() proto.Message { return new(facetpb.BeginObserveResult) }, func() proto.Message { return new(facetpb.PollChangesResult) },
		func() proto.Message { return new(facetpb.ReleaseObserveResult) },
	}
	covered := map[string]bool{}
	for _, test := range cases {
		if !test.Valid {
			continue
		}
		t.Run(test.Name, func(t *testing.T) {
			body := make(map[string]any, len(test.Value))
			for k, v := range test.Value {
				if k != "op" {
					body[k] = v
				}
			}
			var msg proto.Message
			if test.Schema == "request" {
				op, _ := test.Value["op"].(string)
				build := requests[op]
				if build == nil {
					t.Fatalf("uncovered operation %q", op)
				}
				covered[op] = true
				msg = build()
				if err := representation.FromValue(msg.ProtoReflect(), body); err != nil {
					t.Fatal(err)
				}
			} else if test.Schema == "response" {
				for _, build := range responses {
					candidate := build()
					if representation.FromValue(candidate.ProtoReflect(), body) == nil {
						msg = candidate
						break
					}
				}
				if msg == nil {
					t.Fatal("no declared response shape accepts corpus result")
				}
			} else {
				t.Fatalf("unknown corpus schema %s", test.Schema)
			}
			wire, err := proto.Marshal(msg)
			if err != nil {
				t.Fatal(err)
			}
			decoded := msg.ProtoReflect().New().Interface()
			if err := proto.Unmarshal(wire, decoded); err != nil {
				t.Fatal(err)
			}
			value, err := representation.ToValue(decoded.ProtoReflect(), 0)
			if err != nil {
				t.Fatal(err)
			}
			got, err := values.CanonicalJSONValue(value)
			if err != nil {
				t.Fatal(err)
			}
			want, err := values.CanonicalJSONValue(body)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(got, want) {
				t.Fatalf("roundtrip differs\ngot %s\nwant %s", got, want)
			}
		})
	}
	for op := range requests {
		if !covered[op] {
			t.Errorf("missing conformance coverage: %s", op)
		}
	}
}

func token(kind networkpb.TokenKind) *networkpb.Token {
	return &networkpb.Token{Owner: proto.String("00000000-0000-4000-8000-000000000001"), Kind: kind.Enum(), Value: make([]byte, 32)}
}

func TestRejectMalformedRepresentations(t *testing.T) {
	bad := []proto.Message{
		&facetpb.Expected{Condition: &facetpb.Expected_Absent{Absent: false}},
		&facetpb.Expected{}, &facetpb.MutationCommand{}, &facetpb.GetRequest{},
		&facetpb.BeginObserveRequest{Preparation: token(networkpb.TokenKind_READ_RECEIPT)},
		&facetpb.GetResult{Receipt: token(networkpb.TokenKind_READ_RECEIPT), State: facetpb.Presence_PRESENT.Enum(), TypeRef: make([]byte, 32)},
		&facetpb.GetResult{Receipt: token(networkpb.TokenKind_READ_RECEIPT), State: facetpb.Presence_NOT_FOUND.Enum(), TypeRef: make([]byte, 32), Payload: []byte{}},
		&facetpb.ListTypesResult{Receipt: token(networkpb.TokenKind_READ_RECEIPT), Types: [][]byte{{1}}},
		&facetpb.ReleaseObserveResult{Released: proto.Bool(false)},
	}
	for _, msg := range bad {
		if _, err := representation.ToValue(msg.ProtoReflect(), 0); err == nil {
			t.Errorf("accepted %T", msg)
		}
	}
	msg := &facetpb.BeginObserveRequest{Preparation: token(networkpb.TokenKind_OBSERVATION_PREPARATION)}
	msg.Preparation.ProtoReflect().SetUnknown([]byte{0x98, 0x06, 0x01})
	if _, err := representation.ToValue(msg.ProtoReflect(), 0); err == nil {
		t.Fatal("unknown nested field accepted")
	}
	for _, op := range []string{"unknown", "put_extra", "put-extra"} {
		if representation.FromValue(new(facetpb.MutationCommand).ProtoReflect(), map[string]any{"op": op}) == nil {
			t.Fatal("unknown mutation accepted")
		}
	}
}

func TestPublicAttachmentUsesOneSharedType(t *testing.T) {
	field := (&facetpb.Access{}).ProtoReflect().Descriptor().Fields().ByName("key")
	if field.Message().FullName() != protoreflect.FullName("ucupa.network.AttachmentKey") {
		t.Fatal("private or duplicated attachment key")
	}
}
