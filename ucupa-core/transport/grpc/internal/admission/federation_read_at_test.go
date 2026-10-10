package admission

import (
	"context"
	"crypto/sha256"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"testing"
	"time"
	netpb "ucupa/shared/bindings/go/protobuf"
	"ucupa/shared/values"
	pb "ucupa/ucupa-core/bindings/go/protobuf"
	grpcsession "ucupa/ucupa-core/transport/grpc/internal/session"
)

func readAtChallengeFixture(t *testing.T) (*Service, *federationOwnerMock, *federationIdentityMock, context.Context, *pb.ProjectFederationReadPartRequest) {
	t.Helper()
	s, o, i, ctx, opening := federationFixture(t)
	opening.Request.Intent = pb.Intent_QUERY.Enum()
	_, g := exchangeFederation(t, s, ctx, opening)
	begin := &pb.BeginFederationReadAtChallengeRequest{Basis: g.Part.Basis, Element: proto.String(string(federationElement)), TtlSeconds: proto.Uint64(10)}
	ctx = federationGrantContext(t, s, ctx, g, pb.FederationReadService_BeginFederationReadAtChallenge_FullMethodName, begin)
	challenge, err := s.BeginFederationReadAtChallenge(ctx, begin)
	if err != nil {
		t.Fatal(err)
	}
	request := &pb.ProjectFederationReadPartRequest{Basis: g.Part.Basis, Limit: proto.Uint32(1), Request: &pb.FederationProjectionRequest{Kind: proto.String("read-at"), Element: begin.Element}, ReadChallenge: challenge.Challenge, ReadProof: &netpb.IdentityProof{Payload: []byte("read-proof"), Signature: make([]byte, 64)}}
	return s, o, i, ctx, request
}

func TestFederationReadAtProofIsSingleUseAndBound(t *testing.T) {
	for _, name := range []string{"valid", "missing", "element", "expired", "subject", "grant", "unexpected arm", "replay"} {
		t.Run(name, func(t *testing.T) {
			s, o, i, ctx, r := readAtChallengeFixture(t)
			switch name {
			case "missing":
				r.ReadProof = nil
			case "element":
				r.Request.Element = proto.String("00000000-0000-4000-8000-000000000099")
			case "expired":
				key := sha256.Sum256(r.ReadChallenge)
				c := s.challenges[key]
				c.expires = time.Now().Add(-time.Second)
				s.challenges[key] = c
			case "subject":
				i.caller.Subject.Principal = "another-principal"
			case "grant":
				key, ok := grpcsession.Grant(ctx)
				if !ok {
					t.Fatal("grant context missing")
				}
				other := [32]byte{99}
				s.grants[other] = s.grants[key]
				ctx = grpcsession.WithGrant(ctx, other)
			case "unexpected arm":
				r.Request.Kind = proto.String("query")
				r.Request.Element = nil
				r.Request.Query = &pb.FederationQueryRequest{Scope: proto.String(string(federationElement)), Selector: pb.Selector_BOTH.Enum(), Sphere: &pb.Sphere{Center: &netpb.Vector3{Items: []float64{0, 0, 0}}, Radius: proto.Float64(1)}}
			}
			out, err := s.ProjectFederationReadPart(ctx, r)
			if name == "valid" || name == "replay" {
				if err != nil || out.GetResult() == nil || o.pages != 1 {
					t.Fatal("new Read rejected", out, err, o.pages)
				}
				if name == "replay" {
					if again, err := s.ProjectFederationReadPart(ctx, r); status.Code(err) != codes.Unauthenticated || again != nil || o.pages != 1 {
						t.Fatal("Read proof replay", again, err, o.pages)
					}
				}
			} else if err == nil || out != nil || o.pages != 0 {
				t.Fatal("unproven Read reached owner", out, err, o.pages)
			}
		})
	}
}

func TestFederationReadAtChallengeDoesNotExtendGrantAndIsBounded(t *testing.T) {
	s, _, _, ctx, r := readAtChallengeFixture(t)
	key, ok := grpcsession.Grant(ctx)
	if !ok {
		t.Fatal("grant context missing")
	}
	g := s.grants[key]
	g.expires = time.Now().Add(2500 * time.Millisecond)
	s.grants[key] = g
	request := &pb.BeginFederationReadAtChallengeRequest{Basis: r.Basis, Element: r.Request.Element, TtlSeconds: proto.Uint64(10)}
	response, err := s.BeginFederationReadAtChallenge(ctx, request)
	if err != nil || response.GetExpiresInSeconds() == 0 || response.GetExpiresInSeconds() > 2 {
		t.Fatal("challenge extended old grant", response, err)
	}
	if _, err := s.BeginFederationReadAtChallenge(ctx, request); status.Code(err) != codes.ResourceExhausted {
		t.Fatal("unbounded pending challenges", err)
	}
	request.TtlSeconds = proto.Uint64(0)
	if _, err := s.BeginFederationReadAtChallenge(ctx, request); status.Code(err) != codes.InvalidArgument {
		t.Fatal("implicit challenge TTL", err)
	}
}

func TestFederationReadAtProofRecheckedAfterOwner(t *testing.T) {
	s, o, i, ctx, r := readAtChallengeFixture(t)
	o.after = func() { i.err = values.PublicDenied }
	if out, err := s.ProjectFederationReadPart(ctx, r); err == nil || out != nil || o.pages != 1 {
		t.Fatal("identity revoked during Read was disclosed", out, err, o.pages)
	}
}
