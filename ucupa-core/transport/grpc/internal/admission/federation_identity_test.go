package admission

import (
	federationcontract "ucupa/ucupa-core/contract/federation"

	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	netpb "ucupa/shared/bindings/go/protobuf"
	"ucupa/shared/values"

	pb "ucupa/ucupa-core/bindings/go/protobuf"
	corecontract "ucupa/ucupa-core/contract"
	grpcsession "ucupa/ucupa-core/transport/grpc/internal/session"
)

const federationTarget values.FactAuthorityRef = "00000000-0000-4000-8000-000000000001"
const federationActor values.FactAuthorityRef = "00000000-0000-4000-8000-000000000002"
const federationElement values.ElementID = "00000000-0000-4000-8000-000000000003"

type federationOwnerMock struct {
	caller                         values.CallerContext
	source                         values.FactAuthorityRef
	request                        federationcontract.FederationReadRequest
	opens, pages, checks, releases int
	openErr, errorAfter            error
	after                          func()
	part                           federationcontract.FederationReadPart
	requireCurrent                 bool
}

func (o *federationOwnerMock) AuthorityRef() values.FactAuthorityRef { return federationTarget }
func (o *federationOwnerMock) PublicCall(context.Context, values.CallerContext, []byte) ([]byte, error) {
	panic("private federation grant cannot call ordinary public owner")
}
func (o *federationOwnerMock) OpenFederationReadPart(_ context.Context, c values.CallerContext, a values.FactAuthorityRef, r federationcontract.FederationReadRequest) (federationcontract.FederationReadPart, error) {
	o.caller, o.source, o.request = c, a, r
	o.opens++
	o.part = federationcontract.FederationReadPart{Basis: values.PublicToken{Owner: federationTarget, Kind: "read-receipt", Value: values.EncodePublicBytes(make([]byte, 32))}, Boundary: r.Boundary, Purpose: r.Purpose, ReadMode: corecontract.QueryCurrentVerified}
	o.part.Subject, o.part.CutStamp, o.part.AuthorizationStamp = c.Subject, [32]byte{1}, [32]byte{2}
	if o.after != nil {
		o.after()
	}
	return o.part, o.openErr
}
func (o *federationOwnerMock) ProjectFederationReadPart(_ context.Context, c values.CallerContext, a values.FactAuthorityRef, b values.PublicToken, _ federationcontract.FederationProjectionRequest, _ *values.PublicToken, _ uint16) (federationcontract.FederationProjectionPage, error) {
	if c != o.caller || a != o.source || b != o.part.Basis {
		panic("original caller or source was replaced")
	}
	o.pages++
	if o.after != nil {
		o.after()
	}
	return federationcontract.FederationProjectionPage{Basis: b, Purpose: o.part.Purpose, Complete: true}, o.errorAfter
}
func (o *federationOwnerMock) CheckFederationReadPart(_ context.Context, c values.CallerContext, a values.FactAuthorityRef, b values.PublicToken, current bool) error {
	if c != o.caller || a != o.source || b != o.part.Basis {
		panic("original context changed")
	}
	o.checks++
	o.requireCurrent = current
	return o.errorAfter
}
func (o *federationOwnerMock) ReleaseFederationReadPart(_ context.Context, c values.CallerContext, a values.FactAuthorityRef, b values.PublicToken) error {
	if c != o.caller || a != o.source || b != o.part.Basis {
		panic("release context changed")
	}
	o.releases++
	return nil
}
func (o *federationOwnerMock) ObserveFederationReadPart(_ context.Context, c values.CallerContext, a values.FactAuthorityRef, b values.PublicToken, _ federationcontract.FederationObserveRequest) (federationcontract.FederationObservationPage, error) {
	if c != o.caller || a != o.source || b != o.part.Basis {
		panic("observation context changed")
	}
	return federationcontract.FederationObservationPage{}, o.errorAfter
}

type federationIdentityMock struct {
	caller              values.CallerContext
	err                 error
	establishes, checks int
}

func (i *federationIdentityMock) Establish(context.Context, []byte, []byte, values.FactAuthorityRef, [32]byte) (values.CallerContext, time.Time, error) {
	i.establishes++
	return i.caller, time.Now().Add(time.Minute), i.err
}
func (i *federationIdentityMock) Check(context.Context, []byte, []byte, values.FactAuthorityRef, [32]byte) error {
	i.checks++
	return i.err
}
func federationFixture(t *testing.T) (*Service, *federationOwnerMock, *federationIdentityMock, context.Context, *pb.BeginFederationReadChallengeRequest) {
	t.Helper()
	o := &federationOwnerMock{}
	i := &federationIdentityMock{caller: values.CallerContext{Subject: values.PrincipalRef{Realm: "realm", Principal: "original"}, Credential: [16]byte{1}, Establishment: [32]byte{2}, Proof: [32]byte{3}}}
	// Unit boundary fixture: TLS verification itself is exercised by independent
	// process acceptance tests, not this synthetic verified peer context.
	cert := &x509.Certificate{RawSubjectPublicKeyInfo: []byte("coordinator")}
	ctx := peer.NewContext(context.Background(), &peer.Peer{AuthInfo: credentials.TLSInfo{State: tls.ConnectionState{VerifiedChains: [][]*x509.Certificate{{cert}}, PeerCertificates: []*x509.Certificate{cert}}}})
	s := &Service{owner: o, config: Config{Federation: o, Expected: federationTarget, NextHop: "https://core.example:443", RPCDeadline: time.Second, MaxResponseBytes: 1 << 20, ExternalIdentity: i, ReadDelegation: &ReadDelegationConfig{Peers: map[values.FactAuthorityRef][32]byte{federationActor: sha256.Sum256(cert.RawSubjectPublicKeyInfo)}, MaxTTL: time.Minute, MaxGrants: 4, MaxPerCaller: 2}}, grants: map[[32]byte]readGrant{}, pending: map[values.PrincipalRef]int{}, challenges: map[[32]byte]identityChallenge{}}
	purpose := [32]byte{9}
	r := &pb.BeginFederationReadChallengeRequest{Actor: proto.String(string(federationActor)), TtlSeconds: proto.Uint64(30), NextHop: proto.String(string(s.config.NextHop)), Request: &pb.FederationReadRequest{Boundary: proto.String(string(federationElement)), Intent: pb.Intent_READ.Enum(), PrepareObserve: proto.Bool(false), Purpose: purpose[:]}}
	return s, o, i, ctx, r
}
func exchangeFederation(t *testing.T, s *Service, ctx context.Context, r *pb.BeginFederationReadChallengeRequest) (*pb.ExchangeIdentityRequest, *pb.FederationReadGrant) {
	t.Helper()
	challenge, e := s.BeginFederationReadChallenge(ctx, r)
	if e != nil {
		t.Fatal(e)
	}
	in := &pb.ExchangeIdentityRequest{Actor: r.Actor, Challenge: challenge.Challenge, Proof: &netpb.IdentityProof{Payload: []byte("proof"), Signature: make([]byte, 64)}}
	out, e := s.ExchangeFederationReadIdentity(ctx, in)
	if e != nil || out.GetResult() == nil {
		t.Fatalf("exchange: %v %v", out, e)
	}
	return in, out.GetResult()
}
func federationGrantContext(t *testing.T, s *Service, ctx context.Context, g *pb.FederationReadGrant, method string, in proto.Message) context.Context {
	t.Helper()
	md := metadata.Pairs("authorization", "Delegation "+values.EncodePublicBytes(g.Token))
	c, key, e := s.delegatedCaller(ctx, md, method, in)
	if e != nil {
		t.Fatal(e)
	}
	ctx = grpcsession.WithCaller(ctx, c)
	return grpcsession.WithGrant(ctx, key)
}

func TestFederationIdentityBindsOriginalCallerAndIsolatesPartGrant(t *testing.T) {
	s, o, i, ctx, r := federationFixture(t)
	original := proto.Clone(r).(*pb.BeginFederationReadChallengeRequest)
	ch, e := s.BeginFederationReadChallenge(ctx, r)
	if e != nil {
		t.Fatal(e)
	}
	r.Request.Purpose[0] = 10
	r.Request.Boundary = proto.String(string(federationActor))
	in := &pb.ExchangeIdentityRequest{Actor: r.Actor, Challenge: ch.Challenge, Proof: &netpb.IdentityProof{Payload: []byte("proof"), Signature: make([]byte, 64)}}
	out, e := s.ExchangeFederationReadIdentity(ctx, in)
	if e != nil || out.GetResult() == nil {
		t.Fatalf("exchange: %v %v", out, e)
	}
	g := out.GetResult()
	if o.caller != i.caller || o.source != federationActor || o.request.Purpose != [32]byte(original.Request.Purpose) || string(o.request.Boundary) != original.Request.GetBoundary() {
		t.Fatal("challenge mutation changed owner request or original context")
	}
	if _, e := s.ExchangeFederationReadIdentity(ctx, in); status.Code(e) != codes.Unauthenticated || o.opens != 1 {
		t.Fatalf("challenge replay opened another part: %v", e)
	}
	request := &pb.ProjectFederationReadPartRequest{Basis: g.Part.Basis, Limit: proto.Uint32(2), Request: &pb.FederationProjectionRequest{Kind: proto.String("list"), Element: proto.String(string(federationElement))}}
	pctx := federationGrantContext(t, s, ctx, g, pb.FederationReadService_ProjectFederationReadPart_FullMethodName, request)
	page, e := s.ProjectFederationReadPart(pctx, request)
	if e != nil || !page.GetResult().GetComplete() || o.pages != 1 {
		t.Fatalf("page: %v %v", page, e)
	}
	checked, e := s.CheckFederationReadPart(pctx, &pb.CheckFederationReadPartRequest{Basis: g.Part.Basis, RequireCurrent: proto.Bool(true)})
	if e != nil || !checked.GetResult().GetValid() || !o.requireCurrent {
		t.Fatalf("seal: %v %v", checked, e)
	}
	md := metadata.Pairs("authorization", "Delegation "+values.EncodePublicBytes(g.Token))
	for _, method := range []string{pb.CoreService_ReadElement_FullMethodName, pb.CoreService_BeginObserve_FullMethodName, pb.ReadDelegationService_CreateReadDelegation_FullMethodName, pb.FederationReadService_ExchangeFederationReadIdentity_FullMethodName} {
		if _, _, e := s.delegatedCaller(ctx, md, method, &pb.ReadElementRequest{Handle: g.Part.Basis}); status.Code(e) != codes.Unauthenticated {
			t.Fatalf("part grant escaped to %s: %v", method, e)
		}
	}
	wrong := proto.Clone(request).(*pb.ProjectFederationReadPartRequest)
	wrong.Basis.Value[0]++
	if _, _, e := s.delegatedCaller(ctx, md, pb.FederationReadService_ProjectFederationReadPart_FullMethodName, wrong); status.Code(e) != codes.Unauthenticated {
		t.Fatal("another part accepted")
	}
	if _, e := s.ProjectFederationReadPart(grpcsession.WithCaller(ctx, i.caller), request); status.Code(e) != codes.Unauthenticated {
		t.Fatal("plain user session became part grant")
	}
	released, e := s.ReleaseFederationReadPart(pctx, &pb.FederationReadPartRequest{Basis: g.Part.Basis})
	if e != nil || !released.GetResult().GetReleased() || o.releases != 1 || len(s.grants) != 0 {
		t.Fatalf("release: %v %v", released, e)
	}
}

func TestFederationExchangeFailureCleanupAndProofLiveness(t *testing.T) {
	for _, kind := range []string{"open-denied", "proof-unavailable", "proof-revoked", "owner-check-failed", "capacity", "oversize"} {
		t.Run(kind, func(t *testing.T) {
			s, o, i, ctx, r := federationFixture(t)
			ch, e := s.BeginFederationReadChallenge(ctx, r)
			if e != nil {
				t.Fatal(e)
			}
			switch kind {
			case "open-denied":
				o.openErr = values.PublicDenied
			case "proof-unavailable":
				o.after = func() { i.err = values.PublicUnavailable }
			case "proof-revoked":
				o.after = func() { i.err = values.PublicDenied }
			case "owner-check-failed":
				o.errorAfter = values.PublicInvalidContext
			case "capacity":
				s.config.ReadDelegation.MaxGrants = 0
			case "oversize":
				s.config.MaxResponseBytes = 1
			}
			in := &pb.ExchangeIdentityRequest{Actor: r.Actor, Challenge: ch.Challenge, Proof: &netpb.IdentityProof{Payload: []byte("proof"), Signature: make([]byte, 64)}}
			out, e := s.ExchangeFederationReadIdentity(ctx, in)
			if e == nil && out.GetFailure() == 0 {
				t.Fatal("failed exchange published result")
			}
			if len(s.grants) != 0 || len(s.pending) != 0 {
				t.Fatal("failed exchange leaked transport admission")
			}
			if kind != "open-denied" && kind != "capacity" && o.releases != 1 {
				t.Fatal("failed exchange retained owner part")
			}
			if _, e := s.ExchangeFederationReadIdentity(ctx, in); status.Code(e) != codes.Unauthenticated {
				t.Fatal("failed challenge reusable")
			}
		})
	}
	s, o, i, ctx, r := federationFixture(t)
	_, g := exchangeFederation(t, s, ctx, r)
	request := &pb.ProjectFederationReadPartRequest{Basis: g.Part.Basis, Limit: proto.Uint32(1), Request: &pb.FederationProjectionRequest{Kind: proto.String("list"), Element: proto.String(string(federationElement))}}
	pctx := federationGrantContext(t, s, ctx, g, pb.FederationReadService_ProjectFederationReadPart_FullMethodName, request)
	o.after = func() { i.err = values.PublicDenied }
	out, e := s.ProjectFederationReadPart(pctx, request)
	if status.Code(e) != codes.Unauthenticated || out != nil || len(s.grants) != 0 {
		t.Fatalf("post-read revocation exposed data: %v %v", out, e)
	}
}

func TestFederationChallengeAdmissionAndActorAuthentication(t *testing.T) {
	s, _, _, ctx, r := federationFixture(t)
	for _, change := range []func(*pb.BeginFederationReadChallengeRequest){func(w *pb.BeginFederationReadChallengeRequest) { w.TtlSeconds = proto.Uint64(0) }, func(w *pb.BeginFederationReadChallengeRequest) { w.TtlSeconds = proto.Uint64(61) }, func(w *pb.BeginFederationReadChallengeRequest) { w.Request.Purpose = make([]byte, 32) }, func(w *pb.BeginFederationReadChallengeRequest) { w.Request.Intent = pb.Intent_CREATE_ANCHOR.Enum() }, func(w *pb.BeginFederationReadChallengeRequest) { w.Request.Mode = pb.ReadMode_RETAINED.Enum() }, func(w *pb.BeginFederationReadChallengeRequest) { w.NextHop = proto.String("https://other.example:443") }} {
		w := proto.Clone(r).(*pb.BeginFederationReadChallengeRequest)
		change(w)
		if _, e := s.BeginFederationReadChallenge(ctx, w); status.Code(e) != codes.InvalidArgument {
			t.Fatalf("bad challenge accepted: %v", e)
		}
	}
	if _, e := s.BeginFederationReadChallenge(context.Background(), r); status.Code(e) != codes.Unauthenticated {
		t.Fatal("missing actor TLS accepted")
	}
	if _, e := s.BeginFederationReadChallenge(metadata.NewIncomingContext(ctx, metadata.Pairs("authorization", "Bearer anything")), r); status.Code(e) != codes.Unauthenticated {
		t.Fatal("bearer mixed into actor challenge")
	}
	for n := 0; n < 2; n++ {
		if _, e := s.BeginFederationReadChallenge(ctx, r); e != nil {
			t.Fatal(e)
		}
	}
	if _, e := s.BeginFederationReadChallenge(ctx, r); status.Code(e) != codes.ResourceExhausted {
		t.Fatal("unbounded actor challenges")
	}
}
