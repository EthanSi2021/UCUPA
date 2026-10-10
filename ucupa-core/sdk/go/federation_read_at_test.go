package coresdk

import (
	"google.golang.org/protobuf/proto"
	"testing"
	netpb "ucupa/shared/bindings/go/protobuf"
	pb "ucupa/ucupa-core/bindings/go/protobuf"
)

func TestFederationReadAtChallengeResponseBinding(t *testing.T) {
	for _, name := range []string{"valid", "ttl", "zero", "nonce", "basis", "no grant"} {
		t.Run(name, func(t *testing.T) {
			c := fedClient()
			c.federation = fedPart()
			request := &pb.BeginFederationReadAtChallengeRequest{Basis: proto.Clone(c.federation.Basis).(*netpb.Token), Element: proto.String(fedElement), TtlSeconds: proto.Uint64(5)}
			response := &pb.IdentityChallenge{Challenge: make([]byte, 32), ExpiresInSeconds: proto.Uint64(5)}
			switch name {
			case "ttl":
				response.ExpiresInSeconds = proto.Uint64(6)
			case "zero":
				response.ExpiresInSeconds = proto.Uint64(0)
			case "nonce":
				response.Challenge = []byte{1}
			case "basis":
				request.Basis.Value[0] = 1
			case "no grant":
				c.federation = nil
			}
			if err := c.validateFederationExchange(request, response); (err == nil) != (name == "valid") {
				t.Fatal("unexpected validation", err)
			}
		})
	}
}
