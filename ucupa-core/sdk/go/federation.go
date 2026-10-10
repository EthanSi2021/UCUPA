package coresdk

import (
	"bytes"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"ucupa/shared/values"
	pb "ucupa/ucupa-core/bindings/go/protobuf"
)

type pendingFederationChallenge struct {
	challenge []byte
	request   *pb.BeginFederationReadChallengeRequest
}

func federationReadMethod(method string) bool {
	if method == pb.FederationReadService_ObserveFederationReadPart_FullMethodName {
		return true
	}
	return method == pb.FederationReadService_BeginFederationReadAtChallenge_FullMethodName || method == pb.FederationReadService_ProjectFederationReadPart_FullMethodName || method == pb.FederationReadService_CheckFederationReadPart_FullMethodName || method == pb.FederationReadService_ReleaseFederationReadPart_FullMethodName
}

// UseFederationReadGrant pins one owner-local part on a fresh actor client.
// It is not a user session, ordinary read delegation, or re-delegation right.
func (c *Client) UseFederationReadGrant(g *pb.FederationReadGrant) error {
	if g == nil || c.config.ActorCertificate == nil || g.GetTarget() != string(c.config.Expected) || g.GetActor() != string(c.config.Actor) || len(g.Token) != 32 || g.GetExpiresInSeconds() == 0 {
		return status.Error(codes.InvalidArgument, "invalid federation grant")
	}
	if err := validateResponse(g.ProtoReflect(), 0); err != nil {
		return err
	}
	if err := validateTokenOwners(g.ProtoReflect(), c.config.Expected); err != nil {
		return err
	}
	if err := validateFederationPart(g.Part, c.config.Expected); err != nil {
		return err
	}
	c.lifecycle.Lock()
	defer c.lifecycle.Unlock()
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.session != "" || c.delegation != "" {
		return status.Error(codes.FailedPrecondition, "use a fresh actor client")
	}
	c.delegation = values.EncodePublicBytes(g.Token)
	c.federation = proto.Clone(g.Part).(*pb.FederationReadPart)
	return nil
}

func (c *Client) validateFederationExchange(in, out any) error {
	bad := func() error { return status.Error(codes.DataLoss, "federation response does not match request") }
	switch r := in.(type) {
	case *pb.ObserveFederationReadPartRequest:
		return c.validateFederationObservation(r, out)
	case *pb.BeginFederationReadAtChallengeRequest:
		w, ok := out.(*pb.IdentityChallenge)
		c.mu.RLock()
		part := c.federation
		c.mu.RUnlock()
		if !ok || part == nil || !proto.Equal(part.Basis, r.Basis) || len(w.GetChallenge()) != 32 || w.GetExpiresInSeconds() == 0 || w.GetExpiresInSeconds() > r.GetTtlSeconds() {
			return bad()
		}
	case *pb.BeginFederationReadChallengeRequest:
		w, ok := out.(*pb.IdentityChallenge)
		if !ok || w.GetExpiresInSeconds() > r.GetTtlSeconds() || r.GetActor() != string(c.config.Actor) || r.GetNextHop() != string(c.config.NextHop) {
			return bad()
		}
		c.mu.Lock()
		c.federationChallenge = &pendingFederationChallenge{challenge: append([]byte(nil), w.Challenge...), request: proto.Clone(r).(*pb.BeginFederationReadChallengeRequest)}
		c.mu.Unlock()
	case *pb.ExchangeIdentityRequest:
		w, ok := out.(*pb.ExchangeFederationReadIdentityResponse)
		if !ok {
			return nil
		}
		c.mu.Lock()
		pending := c.federationChallenge
		c.federationChallenge = nil
		c.mu.Unlock()
		if pending == nil || !bytes.Equal(pending.challenge, r.Challenge) || r.GetActor() != string(c.config.Actor) {
			return bad()
		}
		if g := w.GetResult(); g != nil {
			q := pending.request.Request
			if q == nil || g.GetTarget() != string(c.config.Expected) || g.GetActor() != string(c.config.Actor) || len(g.Token) != 32 || g.GetExpiresInSeconds() == 0 || g.GetExpiresInSeconds() > pending.request.GetTtlSeconds() || g.Part.GetBoundary() != q.GetBoundary() || !bytes.Equal(g.Part.GetPurpose(), q.Purpose) || (q.Mode != nil && g.Part.GetReadMode() != q.GetMode()) || (g.Part.Preparation != nil) != q.GetPrepareObserve() {
				return bad()
			}
			if q.Incoming != nil && (g.Part.Incoming == nil || !proto.Equal(g.Part.Incoming.Key, q.Incoming)) {
				return bad()
			}
			return validateFederationPart(g.Part, c.config.Expected)
		}
	case *pb.ProjectFederationReadPartRequest:
		w, ok := out.(*pb.ProjectFederationReadPartResponse)
		if !ok {
			return bad()
		}
		p := w.GetResult()
		if p == nil {
			return nil
		}
		c.mu.RLock()
		part := c.federation
		c.mu.RUnlock()
		if part == nil || !proto.Equal(p.Basis, r.Basis) || !proto.Equal(p.Basis, part.Basis) || !bytes.Equal(p.Purpose, part.Purpose) || uint64(len(p.Records)) > uint64(r.GetLimit()) || uint64(len(p.Matches)) > uint64(r.GetLimit()) {
			return bad()
		}
		if err := validateFederationPage(p, part, c.config.Expected); err != nil {
			return err
		}
		return validateFederationProjection(r.Request, p, part, c.config.Expected)
	case *pb.FederationReadPartRequest, *pb.CheckFederationReadPartRequest:
		switch w := out.(type) {
		case *pb.CheckFederationReadPartResponse:
			if w.GetResult() != nil && !w.GetResult().GetValid() {
				return bad()
			}
		case *pb.ReleaseFederationReadPartResponse:
			if w.GetResult() != nil && !w.GetResult().GetReleased() {
				return bad()
			}
		default:
			return bad()
		}
	}
	return nil
}
