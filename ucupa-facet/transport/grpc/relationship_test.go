package facetgrpc

import (
	"context"
	"encoding/binary"
	"errors"
	"strings"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	netpb "ucupa/shared/bindings/go/protobuf"
	"ucupa/shared/values"
	pb "ucupa/ucupa-facet/bindings/go/protobuf"
	facetcontract "ucupa/ucupa-facet/contract"
)

const serviceCore values.FactAuthorityRef = "00000000-0000-4000-8000-000000000002"
const serviceElement values.ElementID = "00000000-0000-4000-8000-000000000003"

type relationshipAdapterOwner struct {
	adapterOwner
	caller values.CallerContext
	ref    facetcontract.PublicAcceptanceRef
	key    values.FacetAcceptanceKey
}

func (o *relationshipAdapterOwner) AcceptAttachmentControl(_ context.Context, caller values.CallerContext, ref facetcontract.PublicAcceptanceRef, address values.NextHop, _ facetcontract.OperationID) (facetcontract.PublicAcceptanceOutcome, error) {
	o.caller, o.ref, o.calls = caller, ref, o.calls+1
	t, _ := values.NewPublicToken(o.AuthorityRef(), "contact-revision")
	return facetcontract.PublicAcceptanceOutcome{Status: facetcontract.Committed, Inspection: facetcontract.PublicAcceptanceInspection{Ref: ref, Found: true, CoreNextHop: address, ContactRevision: t}}, o.err
}
func (o *relationshipAdapterOwner) InspectAcceptanceControl(_ context.Context, caller values.CallerContext, ref facetcontract.PublicAcceptanceRef) (facetcontract.PublicAcceptanceInspection, error) {
	o.caller, o.ref, o.calls = caller, ref, o.calls+1
	return facetcontract.PublicAcceptanceInspection{Ref: ref}, o.err
}
func (o *relationshipAdapterOwner) UpdateCoreContactControl(c context.Context, caller values.CallerContext, id facetcontract.OperationID, ref facetcontract.PublicAcceptanceRef, _ values.PublicToken, address values.NextHop) (facetcontract.PublicAcceptanceOutcome, error) {
	return o.AcceptAttachmentControl(c, caller, ref, address, id)
}
func (o *relationshipAdapterOwner) RetireContentControl(_ context.Context, caller values.CallerContext, ref facetcontract.PublicAcceptanceRef, _ facetcontract.OperationID) (facetcontract.PublicAcceptanceOutcome, error) {
	o.caller, o.ref, o.calls = caller, ref, o.calls+1
	// The committed retire reports the disposed relationship as absent.
	return facetcontract.PublicAcceptanceOutcome{Status: facetcontract.Committed, Inspection: facetcontract.PublicAcceptanceInspection{Ref: ref}}, o.err
}
func (o *relationshipAdapterOwner) ReadAcceptanceControl(_ context.Context, caller values.CallerContext, key values.FacetAcceptanceKey) (facetcontract.AcceptanceRead, error) {
	o.caller, o.key, o.calls = caller, key, o.calls+1
	var raw [40]byte
	raw[0] = 3
	binary.BigEndian.PutUint64(raw[32:], 1<<60+1)
	cut, _ := values.NewFacetReadCut(raw[:])
	return facetcontract.AcceptanceRead{Found: true, Acceptance: facetcontract.Acceptance{Key: key, AcceptedAt: cut}, Cut: cut}, o.err
}
func (o *relationshipAdapterOwner) ReadDispositionControl(_ context.Context, caller values.CallerContext, element values.ElementID) (facetcontract.DispositionRead, error) {
	o.caller, o.calls = caller, o.calls+1
	return facetcontract.DispositionRead{}, o.err
}

func relationshipTestRef() *pb.PublicAcceptanceRef {
	p := token(netpb.TokenKind_ATTACHMENT_REVISION)
	p.Owner = proto.String(string(serviceCore))
	return &pb.PublicAcceptanceRef{Core: proto.String(string(serviceCore)), Element: proto.String(string(serviceElement)), Proposal: p}
}

func TestRelationshipAdapterPreservesAuthenticatedCaller(t *testing.T) {
	o := &relationshipAdapterOwner{}
	s := &Server{owner: o, config: Config{Expected: o.AuthorityRef()}}
	caller := values.CallerContext{Subject: values.PrincipalRef{Realm: "realm", Principal: "person"}, Credential: [16]byte{2}, Establishment: [32]byte{3}, Proof: [32]byte{4}}
	ctx := context.WithValue(context.Background(), callerKey{}, caller)
	request := &pb.AcceptAttachmentRequest{Ref: relationshipTestRef(), CoreNextHop: proto.String("https://core.example:443"), OperationId: []byte("accept")}
	result, err := s.AcceptAttachment(ctx, request)
	if err != nil || result.GetResult() == nil || o.caller != caller || o.ref.Core != serviceCore || o.ref.Proposal.Owner != serviceCore {
		t.Fatalf("caller lost: %+v %v", result, err)
	}
	if _, err = s.AcceptAttachment(context.Background(), request); status.Code(err) != codes.Unauthenticated || o.calls != 1 {
		t.Fatalf("unestablished caller reached owner: %v", err)
	}
	request.Ref.Proposal.Owner = proto.String(string(o.AuthorityRef()))
	if _, err = s.AcceptAttachment(ctx, request); status.Code(err) != codes.InvalidArgument || o.calls != 1 {
		t.Fatalf("wrong proposal issuer reached owner: %v", err)
	}
	request.Ref = relationshipTestRef()
	o.err = values.PublicInvalidContext
	result, err = s.AcceptAttachment(ctx, request)
	if err != nil || result.GetFailure() != netpb.Failure_INVALID_CONTEXT || result.GetResult() != nil {
		t.Fatalf("auth invalidation not preserved: %+v %v", result, err)
	}
	o.err = errors.New("private store secret")
	_, err = s.AcceptAttachment(ctx, request)
	if status.Code(err) != codes.Internal || strings.Contains(err.Error(), "secret") {
		t.Fatalf("private error leaked: %v", err)
	}
}

func TestPrivateAcceptanceRequiresConfiguredCoreAndOwnerAuthorization(t *testing.T) {
	o := &relationshipAdapterOwner{}
	service := values.CallerContext{Subject: values.PrincipalRef{Realm: "realm", Principal: "service"}, Proof: [32]byte{5}}
	s := &Server{owner: o, config: Config{Expected: o.AuthorityRef(), CoreServices: map[values.PrincipalRef]values.FactAuthorityRef{service.Subject: serviceCore}}}
	incarnation := make([]byte, 32)
	incarnation[0] = 1
	request := &pb.ReadAcceptanceRequest{Key: &pb.ServiceAttachmentKey{Core: proto.String(string(serviceCore)), Facet: proto.String(string(o.AuthorityRef())), Element: proto.String(string(serviceElement)), Proposal: &pb.ServiceRevision{Incarnation: incarnation, Sequence: proto.Uint64(1<<60 + 1)}}}
	for _, caller := range []values.CallerContext{{}, {Subject: values.PrincipalRef{Realm: "realm", Principal: "person"}}} {
		ctx := context.WithValue(context.Background(), callerKey{}, caller)
		if _, err := s.ReadAcceptance(ctx, request); status.Code(err) != codes.PermissionDenied || o.calls != 0 {
			t.Fatalf("unbound Core read private evidence: %v", err)
		}
	}
	ctx := context.WithValue(context.Background(), callerKey{}, service)
	result, err := s.ReadAcceptance(ctx, request)
	if err != nil || o.caller != service || !proto.Equal(result.GetResult().Key, request.Key) || result.GetResult().GetAcceptedAt().GetSequence() != 1<<60+1 {
		t.Fatalf("service evidence: %+v %v", result, err)
	}
	other := proto.Clone(request).(*pb.ReadAcceptanceRequest)
	other.Key.Core = proto.String(string(o.AuthorityRef()))
	if _, err := s.ReadAcceptance(ctx, other); status.Code(err) != codes.InvalidArgument || o.calls != 1 {
		t.Fatalf("fixed Core substituted another Core: %v", err)
	}
	o.err = values.PublicDenied
	denied, err := s.ReadAcceptance(ctx, request)
	if err != nil || denied.GetFailure() != netpb.Failure_DENIED || denied.GetResult() != nil {
		t.Fatalf("configured identity bypassed owner permission: %+v %v", denied, err)
	}
}

func TestRetireContentEncodesProvenAbsenceAndPreservesCaller(t *testing.T) {
	o := &relationshipAdapterOwner{}
	s := &Server{owner: o, config: Config{Expected: o.AuthorityRef()}}
	caller := values.CallerContext{Subject: values.PrincipalRef{Realm: "realm", Principal: "person"}, Credential: [16]byte{2}, Establishment: [32]byte{3}, Proof: [32]byte{4}}
	ctx := context.WithValue(context.Background(), callerKey{}, caller)
	request := &pb.RetireContentRequest{Ref: relationshipTestRef(), OperationId: []byte("retire")}
	result, err := s.RetireContent(ctx, request)
	if err != nil || result.GetResult() == nil || result.GetResult().GetStatus() != pb.CommitStatus_COMMITTED || o.caller != caller || o.calls != 1 {
		t.Fatalf("retire lost caller or result: %+v %v", result, err)
	}
	inspection := result.GetResult().GetInspection()
	if inspection == nil || inspection.GetFound() || !proto.Equal(inspection.Ref, request.GetRef()) || inspection.CoreNextHop != nil || inspection.ContactRevision != nil {
		t.Fatalf("retire result is not the proven absence: %+v", inspection)
	}
	o.err = values.PublicDenied
	denied, err := s.RetireContent(ctx, request)
	if err != nil || denied.GetFailure() != netpb.Failure_DENIED || denied.GetResult() != nil {
		t.Fatalf("retire refusal lost its failure: %+v %v", denied, err)
	}
}

func TestReadDispositionRequiresFixedCoreAndBindsEvidence(t *testing.T) {
	o := &relationshipAdapterOwner{}
	service := values.CallerContext{Subject: values.PrincipalRef{Realm: "realm", Principal: "service"}, Proof: [32]byte{5}}
	s := &Server{owner: o, config: Config{Expected: o.AuthorityRef(), CoreServices: map[values.PrincipalRef]values.FactAuthorityRef{service.Subject: serviceCore}}}
	ctx := context.WithValue(context.Background(), callerKey{}, service)
	bad := &pb.ReadDispositionRequest{Element: proto.String("not-an-element")}
	if _, err := s.ReadDisposition(ctx, bad); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("invalid element accepted: %v", err)
	}
	person := values.CallerContext{Subject: values.PrincipalRef{Realm: "realm", Principal: "person"}, Proof: [32]byte{6}}
	personCtx := context.WithValue(context.Background(), callerKey{}, person)
	if _, err := s.ReadDisposition(personCtx, &pb.ReadDispositionRequest{Element: proto.String(string(serviceElement))}); status.Code(err) != codes.PermissionDenied {
		t.Fatalf("unbound Core read disposition: %v", err)
	}
	// The owner reports no disposition; the absent shape carries no evidence.
	absent, err := s.ReadDisposition(ctx, &pb.ReadDispositionRequest{Element: proto.String(string(serviceElement))})
	if err != nil || absent.GetResult().GetFound() || absent.GetResult().Key != nil || absent.GetResult().Cut != nil {
		t.Fatalf("absent disposition malformed: %+v %v", absent, err)
	}
}
