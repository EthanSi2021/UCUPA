package coresdk

import (
	"context"
	"crypto/tls"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"testing"
	"time"
	netpb "ucupa/shared/bindings/go/protobuf"
	"ucupa/shared/values"
	pb "ucupa/ucupa-core/bindings/go/protobuf"
)

const fedOwner values.FactAuthorityRef = "00000000-0000-4000-8000-000000000001"
const fedActor values.FactAuthorityRef = "00000000-0000-4000-8000-000000000002"
const fedElement = "00000000-0000-4000-8000-000000000003"

func fedToken(kind netpb.TokenKind) *netpb.Token {
	return &netpb.Token{Owner: proto.String(string(fedOwner)), Kind: kind.Enum(), Value: make([]byte, 32)}
}
func fedPart() *pb.FederationReadPart {
	p := [32]byte{1}
	return &pb.FederationReadPart{Hidden: proto.Bool(false), Basis: fedToken(netpb.TokenKind_READ_RECEIPT), Boundary: proto.String(fedElement), Purpose: p[:], ReadMode: pb.ReadMode_CURRENT_VERIFIED.Enum(), Subject: &pb.ServicePrincipal{Realm: proto.String("realm"), Principal: proto.String("caller")}, CutStamp: p[:], AuthorizationStamp: p[:]}
}
func fedClient() *Client {
	return &Client{config: Config{Expected: fedOwner, Actor: fedActor, ActorCertificate: &tls.Certificate{}, NextHop: "https://core.example:443", RPCDeadline: time.Second, MaxResponseBytes: 1 << 20}}
}

func TestFederationPartEvidenceClosedShape(t *testing.T) {
	for _, change := range []func(*pb.FederationReadPart){
		func(p *pb.FederationReadPart) { p.Hidden = nil },
		func(p *pb.FederationReadPart) { p.Hidden = proto.Bool(true) },
		func(p *pb.FederationReadPart) { p.Subject = nil },
		func(p *pb.FederationReadPart) { p.Subject.Realm = proto.String("") },
		func(p *pb.FederationReadPart) { p.CutStamp = nil },
		func(p *pb.FederationReadPart) { p.CutStamp = make([]byte, 32) },
		func(p *pb.FederationReadPart) { p.AuthorizationStamp = make([]byte, 31) },
		func(p *pb.FederationReadPart) { p.AuthorizationStamp = make([]byte, 32) },
	} {
		part := fedPart()
		change(part)
		if validateFederationPart(part, fedOwner) == nil {
			t.Fatal("malformed evidence accepted")
		}
	}
}
func TestFederationGrantAndChallengeResponseBinding(t *testing.T) {
	c := fedClient()
	p := fedPart()
	q := &pb.BeginFederationReadChallengeRequest{Actor: proto.String(string(fedActor)), TtlSeconds: proto.Uint64(5), NextHop: proto.String(string(c.config.NextHop)), Request: &pb.FederationReadRequest{Boundary: p.Boundary, Intent: pb.Intent_READ.Enum(), PrepareObserve: proto.Bool(false), Purpose: p.Purpose}}
	challenge := &pb.IdentityChallenge{Challenge: make([]byte, 32), ExpiresInSeconds: proto.Uint64(5)}
	in := &pb.ExchangeIdentityRequest{Actor: q.Actor, Challenge: challenge.Challenge}
	base := &pb.FederationReadGrant{Target: proto.String(string(fedOwner)), Actor: q.Actor, Token: make([]byte, 32), ExpiresInSeconds: proto.Uint64(5), Part: p}
	for _, name := range []string{"good", "target", "actor", "empty-token", "ttl", "boundary", "purpose", "mode", "preparation", "owner", "unknown", "basis-kind"} {
		t.Run(name, func(t *testing.T) {
			g := proto.Clone(base).(*pb.FederationReadGrant)
			switch name {
			case "target":
				g.Target = proto.String(string(fedActor))
			case "actor":
				g.Actor = proto.String(string(fedOwner))
			case "empty-token":
				g.Token = nil
			case "ttl":
				g.ExpiresInSeconds = proto.Uint64(6)
			case "boundary":
				g.Part.Boundary = proto.String(string(fedActor))
			case "purpose":
				g.Part.Purpose[0]++
			case "mode":
				g.Part.ReadMode = pb.ReadMode_READ_MODE_UNSPECIFIED.Enum()
			case "preparation":
				g.Part.Preparation = fedToken(netpb.TokenKind_OBSERVATION_PREPARATION)
			case "owner":
				g.Part.Basis.Owner = proto.String(string(fedActor))
			case "unknown":
				g.ProtoReflect().SetUnknown([]byte{0x78, 1})
			case "basis-kind":
				g.Part.Basis.Kind = netpb.TokenKind_SPACE_HANDLE.Enum()
			}
			if e := c.validateFederationExchange(q, challenge); e != nil {
				t.Fatal(e)
			}
			out := &pb.ExchangeFederationReadIdentityResponse{Outcome: &pb.ExchangeFederationReadIdentityResponse_Result{Result: g}}
			e := validateResponse(out.ProtoReflect(), 0)
			if e == nil {
				e = validateTokenOwners(out.ProtoReflect(), fedOwner)
			}
			if e == nil {
				e = c.validateFederationExchange(in, out)
			}
			if (e == nil) != (name == "good") {
				t.Fatalf("%s accepted=%v %v", name, e == nil, e)
			}
		})
	}
	if e := c.UseFederationReadGrant(base); e != nil {
		t.Fatal(e)
	}
	invoked := 0
	e := c.intercept(context.Background(), pb.CoreService_ReadElement_FullMethodName, &pb.ReadElementRequest{}, &pb.ReadElementResponse{}, nil, func(context.Context, string, any, any, *grpc.ClientConn, ...grpc.CallOption) error {
		invoked++
		return nil
	}, grpc.StaticMethod())
	if status.Code(e) != codes.PermissionDenied || invoked != 0 {
		t.Fatal("part grant called public Core")
	}
	if e := c.UseFederationReadGrant(base); status.Code(e) != codes.FailedPrecondition {
		t.Fatal("grant replaced existing establishment")
	}
}

func fedRelation() *pb.FederationMountRelation {
	key := &pb.ServiceMountKey{ParentCore: proto.String(string(fedOwner)), CarrierCore: proto.String(string(fedActor)), Element: proto.String(fedElement), Proposal: &pb.ServiceRevision{Incarnation: append([]byte{1}, make([]byte, 31)...), Sequence: proto.Uint64(1<<60 + 3)}}
	return &pb.FederationMountRelation{Incoming: proto.Bool(false), State: proto.Uint32(2), Key: key, Admission: &pb.ServiceMountAdmission{Key: proto.Clone(key).(*pb.ServiceMountKey), OperationId: []byte("exact")}, Revision: fedToken(netpb.TokenKind_MOUNT_REVISION), Contact: fedToken(netpb.TokenKind_CONTACT_REVISION), NextHop: proto.String("https://other.example:443"), Range: &pb.Range{Kind: pb.RangeKind_BOUNDED_SPHERE.Enum(), Radius: proto.Float64(3)}, WriteGateOpen: proto.Bool(true)}
}
func TestFederationRelationRejectsOwnerKeyAdmissionAndContactTampering(t *testing.T) {
	for _, name := range []string{"good", "owner", "key", "admission", "sequence", "contact-owner", "contact-kind", "address", "range", "state"} {
		t.Run(name, func(t *testing.T) {
			r := fedRelation()
			switch name {
			case "owner":
				r.Key.ParentCore = proto.String(string(fedActor))
			case "key":
				r.Key.Element = proto.String(string(fedOwner))
			case "admission":
				r.Admission.OperationId = nil
			case "sequence":
				r.Key.Proposal.Sequence = proto.Uint64(0)
			case "contact-owner":
				r.Contact.Owner = proto.String(string(fedActor))
			case "contact-kind":
				r.Contact.Kind = netpb.TokenKind_MOUNT_REVISION.Enum()
			case "address":
				r.NextHop = proto.String("bad")
			case "range":
				r.Range.Radius = proto.Float64(-1)
			case "state":
				r.State = proto.Uint32(999)
			}
			e := validateResponse(r.ProtoReflect(), 0)
			if e == nil {
				e = validateTokenOwners(r.ProtoReflect(), fedOwner)
			}
			if e == nil {
				e = validateFederationRelation(r, fedOwner)
			}
			if (e == nil) != (name == "good") {
				t.Fatalf("%s: %v", name, e)
			}
		})
	}
	incoming := fedRelation()
	incoming.Incoming = proto.Bool(true)
	incoming.Key.ParentCore, incoming.Key.CarrierCore = proto.String(string(fedActor)), proto.String(string(fedOwner))
	incoming.Admission.Key = proto.Clone(incoming.Key).(*pb.ServiceMountKey)
	if e := validateFederationRelation(incoming, fedOwner); e != nil {
		t.Fatal("incoming token owner must remain reporting Core:", e)
	}
}
