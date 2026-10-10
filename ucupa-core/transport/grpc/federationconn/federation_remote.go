package federationconn

import (
	federationcontract "ucupa/ucupa-core/contract/federation"

	"context"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	netpb "ucupa/shared/bindings/go/protobuf"
	"ucupa/shared/values"

	pb "ucupa/ucupa-core/bindings/go/protobuf"
	corecontract "ucupa/ucupa-core/contract"
	coresdk "ucupa/ucupa-core/sdk/go"
	"ucupa/ucupa-core/transport/grpc/internal/wire"
)

func (f *federationCallerConnections) OpenFederationRead(ctx context.Context, caller values.CallerContext, address values.NextHop, target values.FactAuthorityRef, request federationcontract.FederationReadRequest) (federationcontract.FederationReadConnection, error) {
	if f == nil || ctx == nil || caller != f.caller || !target.Valid() {
		return nil, values.PublicInvalid
	}
	if ctx.Err() != nil {
		return nil, values.PublicUnavailable
	}
	cfg, exists := f.connections.peers[address]
	if !exists || cfg.Expected != target {
		return nil, values.PublicUnavailable
	}
	started := time.Now()
	ttl := min(f.connections.ttl, time.Until(f.expires).Truncate(time.Second))
	if ttl <= 0 {
		return nil, values.PublicInvalidContext
	}
	p, err := f.connections.reserve()
	if err != nil {
		return nil, err
	}
	keep := false
	defer func() {
		if !keep {
			p.close()
		}
	}()
	client, err := coresdk.New(cfg)
	if err != nil {
		return nil, values.PublicUnavailable
	}
	defer func() {
		if !keep {
			_ = client.Close()
		}
	}()
	w := wire.FederationRequestWire(request)
	challenge, err := client.FederationReads().BeginFederationReadChallenge(ctx, &pb.BeginFederationReadChallengeRequest{Actor: proto.String(string(f.connections.source)), NextHop: proto.String(string(address)), TtlSeconds: proto.Uint64(uint64(ttl / time.Second)), Request: w})
	if err != nil {
		return nil, federationRemoteError(err, 0)
	}
	if len(challenge.GetChallenge()) != 32 || challenge.GetExpiresInSeconds() == 0 || challenge.GetExpiresInSeconds() > uint64(ttl/time.Second) {
		return nil, values.PublicUnavailable
	}
	proofTTL := time.Duration(challenge.GetExpiresInSeconds()) * time.Second
	payload, signature, err := f.issuer.Issue(ctx, caller, f.connections.source, [32]byte(challenge.Challenge), proofTTL)
	if err != nil {
		return nil, err
	}
	response, err := client.FederationReads().ExchangeFederationReadIdentity(ctx, &pb.ExchangeIdentityRequest{Actor: proto.String(string(f.connections.source)), Challenge: challenge.Challenge, Proof: &netpb.IdentityProof{Payload: payload, Signature: signature}})
	if err != nil || response.GetFailure() != 0 {
		return nil, federationRemoteError(err, response.GetFailure())
	}
	grant := response.GetResult()
	if grant == nil || grant.GetTarget() != string(target) || grant.GetActor() != string(f.connections.source) || grant.GetExpiresInSeconds() == 0 || grant.GetExpiresInSeconds() > uint64(ttl/time.Second) {
		return nil, values.PublicUnavailable
	}
	part, err := wire.FederationPart(grant.Part)
	if err != nil || part.Basis.Owner != target || part.Boundary != request.Boundary || part.Purpose != request.Purpose || (part.Preparation != nil) != request.PrepareObserve ||
		request.Mode == corecontract.QueryCurrentVerified && part.ReadMode != corecontract.QueryCurrentVerified ||
		request.Incoming.Valid() && (part.Incoming == nil || part.Incoming.Key != request.Incoming) {
		return nil, values.PublicUnavailable
	}
	if err := client.UseFederationReadGrant(grant); err != nil {
		return nil, values.PublicUnavailable
	}
	if err := f.issuer.Check(ctx, payload, signature); err != nil {
		return nil, err
	}
	if ctx.Err() != nil {
		return nil, values.PublicUnavailable
	}
	// Measuring from before the first network call is deliberately conservative:
	// no rounded server TTL can extend the original session or local lease.
	expires := started.Add(time.Duration(grant.GetExpiresInSeconds()) * time.Second)
	if f.expires.Before(expires) {
		expires = f.expires
	}
	p.requester = f
	if err := p.install(client, part, expires); err != nil {
		return nil, err
	}
	keep = true
	return p, nil
}

func (p *networkFederationPart) current() (*coresdk.Client, federationcontract.FederationReadPart, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed || p.client == nil || !time.Now().Before(p.expires) {
		return nil, federationcontract.FederationReadPart{}, values.PublicInvalidContext
	}
	return p.client, cloneFederationPart(p.info), nil
}

func (p *networkFederationPart) Project(ctx context.Context, request federationcontract.FederationProjectionRequest, continuation *values.PublicToken, limit uint16) (federationcontract.FederationProjectionPage, error) {
	if ctx == nil || limit == 0 || limit > 256 {
		return federationcontract.FederationProjectionPage{}, values.PublicInvalid
	}
	c, part, err := p.current()
	if err != nil {
		return federationcontract.FederationProjectionPage{}, err
	}
	w := &pb.ProjectFederationReadPartRequest{Basis: wire.ControlToken(part.Basis, netpb.TokenKind_READ_RECEIPT), Limit: proto.Uint32(uint32(limit)), Request: wire.FederationProjectionRequestWire(request)}
	if request.Kind == "read-at" {
		if err := p.proveReadAt(ctx, c, w); err != nil {
			return federationcontract.FederationProjectionPage{}, err
		}
	}
	if continuation != nil {
		if continuation.Owner != part.Basis.Owner || continuation.Kind != "query-continuation" || !continuation.Valid() {
			return federationcontract.FederationProjectionPage{}, values.PublicInvalid
		}
		w.Continuation = wire.ControlToken(*continuation, netpb.TokenKind_QUERY_CONTINUATION)
	}
	r, err := c.FederationReads().ProjectFederationReadPart(ctx, w)
	if err != nil || r.GetFailure() != 0 {
		return federationcontract.FederationProjectionPage{}, federationRemoteError(err, r.GetFailure())
	}
	page, err := wire.FederationPage(r.GetResult())
	if err != nil || page.Basis != part.Basis || page.Purpose != part.Purpose || len(page.Records) > int(limit) || len(page.Matches) > int(limit) {
		return federationcontract.FederationProjectionPage{}, values.PublicUnavailable
	}
	if _, _, err := p.current(); err != nil {
		return federationcontract.FederationProjectionPage{}, err
	}
	return page, nil
}

func (p *networkFederationPart) Check(ctx context.Context, requireCurrent bool) error {
	if ctx == nil {
		return values.PublicInvalid
	}
	c, part, err := p.current()
	if err != nil {
		return err
	}
	r, err := c.FederationReads().CheckFederationReadPart(ctx, &pb.CheckFederationReadPartRequest{Basis: wire.ControlToken(part.Basis, netpb.TokenKind_READ_RECEIPT), RequireCurrent: proto.Bool(requireCurrent)})
	if err != nil || r.GetFailure() != 0 {
		return federationRemoteError(err, r.GetFailure())
	}
	if !r.GetResult().GetValid() {
		return values.PublicUnavailable
	}
	_, _, err = p.current()
	return err
}

func (p *networkFederationPart) Release(ctx context.Context) error {
	if ctx == nil {
		return values.PublicInvalid
	}
	c, part, err := p.current()
	defer p.close()
	if err != nil {
		return nil // Local release is idempotent even after its finite expiry.
	}
	r, err := c.FederationReads().ReleaseFederationReadPart(ctx, &pb.FederationReadPartRequest{Basis: wire.ControlToken(part.Basis, netpb.TokenKind_READ_RECEIPT)})
	if err != nil || r.GetFailure() != 0 {
		return federationRemoteError(err, r.GetFailure())
	}
	if !r.GetResult().GetReleased() {
		return values.PublicUnavailable
	}
	return nil
}

func federationRemoteError(err error, failure netpb.Failure) error {
	if err == nil && failure != 0 {
		return values.PublicError(failure.String())
	}
	switch status.Code(err) {
	case codes.PermissionDenied, codes.Unauthenticated:
		return values.PublicDenied
	case codes.ResourceExhausted:
		return values.PublicResourceExhausted
	default:
		return values.PublicUnavailable
	}
}
