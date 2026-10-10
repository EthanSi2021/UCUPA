package federationconn

import (
	"context"
	"google.golang.org/protobuf/proto"
	"time"
	netpb "ucupa/shared/bindings/go/protobuf"
	"ucupa/shared/values"
	pb "ucupa/ucupa-core/bindings/go/protobuf"
	coresdk "ucupa/ucupa-core/sdk/go"
)

func (p *networkFederationPart) proveReadAt(ctx context.Context, client *coresdk.Client, request *pb.ProjectFederationReadPartRequest) error {
	f := p.requester
	if f == nil || ctx.Err() != nil {
		return values.PublicUnavailable
	}
	p.mu.Lock()
	expires := p.expires
	p.mu.Unlock()
	ttl := min(time.Until(expires).Truncate(time.Second), time.Until(f.expires).Truncate(time.Second), f.connections.ttl)
	if ttl <= 0 {
		return values.PublicInvalidContext
	}
	challenge, err := client.FederationReads().BeginFederationReadAtChallenge(ctx, &pb.BeginFederationReadAtChallengeRequest{Basis: request.Basis, Element: request.Request.Element, TtlSeconds: proto.Uint64(uint64(ttl / time.Second))})
	if err != nil {
		return federationRemoteError(err, 0)
	}
	if len(challenge.GetChallenge()) != 32 || challenge.GetExpiresInSeconds() == 0 || challenge.GetExpiresInSeconds() > uint64(ttl/time.Second) {
		return values.PublicUnavailable
	}
	payload, signature, err := f.issuer.Issue(ctx, f.caller, f.connections.source, [32]byte(challenge.Challenge), time.Duration(challenge.GetExpiresInSeconds())*time.Second)
	if err != nil {
		return err
	}
	request.ReadChallenge = append([]byte(nil), challenge.Challenge...)
	request.ReadProof = &netpb.IdentityProof{Payload: payload, Signature: signature}
	return nil
}
