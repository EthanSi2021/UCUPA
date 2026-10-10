package attachment

import (
	"context"
	"encoding/binary"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	networkpb "ucupa/shared/bindings/go/protobuf"
	"ucupa/shared/values"
	corepb "ucupa/ucupa-core/bindings/go/protobuf"
	corecontract "ucupa/ucupa-core/contract"
	grpcsession "ucupa/ucupa-core/transport/grpc/internal/session"
)

const (
	testOwner   values.FactAuthorityRef = "00000000-0000-4000-8000-000000000001"
	testFacet   values.FactAuthorityRef = "00000000-0000-4000-8000-000000000002"
	testElement values.ElementID        = "00000000-0000-4000-8000-000000000003"
)

var _ corepb.AttachmentControlServiceServer = (*Service)(nil)

type ownerStub struct {
	err   error
	calls int
}

func (o *ownerStub) ResolveProposalControl(context.Context, values.CallerContext, values.FactAuthorityRef, values.ElementID, values.PublicToken) (corecontract.FacetProposalRead, error) {
	o.calls++
	return corecontract.FacetProposalRead{}, o.err
}

func (o *ownerStub) ReadProposalControl(context.Context, values.CallerContext, values.FacetAcceptanceKey) (corecontract.FacetProposalRead, error) {
	o.calls++
	return corecontract.FacetProposalRead{}, o.err
}

func (o *ownerStub) ReadAttachmentControl(context.Context, values.CallerContext, values.FacetAcceptanceKey, bool) (corecontract.AttachmentControlRead, error) {
	o.calls++
	return corecontract.AttachmentControlRead{}, o.err
}

func (o *ownerStub) CheckAttachmentControl(context.Context, values.CallerContext, values.FacetAcceptanceKey, values.PublicToken) error {
	o.calls++
	return o.err
}

func (o *ownerStub) ReadFacetReplacementRecord(corecontract.OperationID, values.ElementID) (corecontract.FacetReplacementRecord, error) {
	o.calls++
	return corecontract.FacetReplacementRecord{}, o.err
}

func (o *ownerStub) ReadSuccessorControl(context.Context, values.CallerContext, values.ElementID) (corecontract.FacetSuccessorRead, error) {
	o.calls++
	return corecontract.FacetSuccessorRead{}, o.err
}

func attachmentFixture(t *testing.T) (*Service, *ownerStub, context.Context, *corepb.ServiceAttachmentKey) {
	t.Helper()
	owner := &ownerStub{err: values.PublicDenied}
	caller := values.CallerContext{Subject: values.PrincipalRef{Realm: "realm", Principal: "caller"}}
	service := New(owner, testOwner, map[values.PrincipalRef]values.FactAuthorityRef{caller.Subject: testFacet})
	var raw [76]byte
	raw[0] = 1
	copy(raw[32:68], testElement)
	binary.BigEndian.PutUint64(raw[68:], 1)
	_, err := values.NewAttachmentRevision(raw[:])
	if err != nil {
		t.Fatal(err)
	}
	incarnation := append([]byte(nil), raw[:32]...)
	key := &corepb.ServiceAttachmentKey{Core: proto.String(string(testOwner)), Facet: proto.String(string(testFacet)), Element: proto.String(string(testElement)), Proposal: &corepb.ServiceRevision{Incarnation: incarnation, Sequence: proto.Uint64(1)}}
	return service, owner, grpcsession.WithCaller(context.Background(), caller), key
}

func publicTokenWire(t *testing.T, owner values.FactAuthorityRef, kind networkpb.TokenKind) *networkpb.Token {
	t.Helper()
	raw := [32]byte{7}
	value, err := values.PublicTokenFromBytes(owner, map[networkpb.TokenKind]string{
		networkpb.TokenKind_ATTACHMENT_REVISION:     "attachment-revision",
		networkpb.TokenKind_OBSERVATION_PREPARATION: "observation-preparation",
	}[kind], raw)
	if err != nil {
		t.Fatal(err)
	}
	bytes, err := values.DecodePublicBytes(value.Value)
	if err != nil {
		t.Fatal(err)
	}
	return &networkpb.Token{Owner: proto.String(string(owner)), Kind: kind.Enum(), Value: bytes}
}

func TestServiceMapsAllAttachmentOwnerErrors(t *testing.T) {
	service, owner, ctx, key := attachmentFixture(t)
	proposal := publicTokenWire(t, testOwner, networkpb.TokenKind_ATTACHMENT_REVISION)
	preparation := publicTokenWire(t, testOwner, networkpb.TokenKind_OBSERVATION_PREPARATION)
	cases := []struct {
		name string
		call func() (networkpb.Failure, error)
	}{
		{name: "read", call: func() (networkpb.Failure, error) {
			out, err := service.ReadControl(ctx, &corepb.ReadControlRequest{Key: key})
			return out.GetFailure(), err
		}},
		{name: "proposal", call: func() (networkpb.Failure, error) {
			out, err := service.ReadProposalControl(ctx, &corepb.ReadControlRequest{Key: key})
			return out.GetFailure(), err
		}},
		{name: "check", call: func() (networkpb.Failure, error) {
			out, err := service.CheckControl(ctx, &corepb.CheckControlRequest{Key: key, Preparation: preparation})
			return out.GetFailure(), err
		}},
		{name: "resolve", call: func() (networkpb.Failure, error) {
			out, err := service.ResolveProposalControl(ctx, &corepb.ResolveProposalControlRequest{Element: proto.String(string(testElement)), Proposal: proposal})
			return out.GetFailure(), err
		}},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			failure, err := testCase.call()
			if err != nil || failure != networkpb.Failure_DENIED {
				t.Fatalf("owner error was not mapped: %v %v", failure, err)
			}
		})
	}
	if owner.calls != len(cases) {
		t.Fatalf("owner calls = %d, want %d", owner.calls, len(cases))
	}
}

func TestServiceCopiesFixedFacetRelation(t *testing.T) {
	service, owner, ctx, key := attachmentFixture(t)
	caller, ok := grpcsession.Caller(ctx)
	if !ok {
		t.Fatal("caller context missing")
	}
	services := map[values.PrincipalRef]values.FactAuthorityRef{caller.Subject: testFacet}
	service = New(owner, testOwner, services)
	delete(services, caller.Subject)
	out, err := service.ReadControl(ctx, &corepb.ReadControlRequest{Key: key})
	if err != nil || out.GetFailure() != networkpb.Failure_DENIED {
		t.Fatalf("fixed relation was not copied: %v %v", out.GetFailure(), err)
	}
}

func TestServiceEmptyRelationDoesNotRequireAttachmentOwner(t *testing.T) {
	service := New(nil, testOwner, nil)
	out, err := service.ReadControl(context.Background(), &corepb.ReadControlRequest{})
	if status.Code(err) != codes.PermissionDenied || out != nil {
		t.Fatalf("empty relation changed unavailable-owner behavior: %v %v", out, err)
	}
}

func TestServiceDelegatedContextRejectedBeforeRequestParsing(t *testing.T) {
	service, owner, ctx, _ := attachmentFixture(t)
	ctx = grpcsession.WithGrant(ctx, [32]byte{1})
	out, err := service.ResolveProposalControl(ctx, &corepb.ResolveProposalControlRequest{})
	if status.Code(err) != codes.PermissionDenied || out != nil || owner.calls != 0 {
		t.Fatalf("delegated context reached attachment owner: %v %v calls=%d", out, err, owner.calls)
	}
}

func TestServiceDelegatedGrantOnlyBlocksResolve(t *testing.T) {
	service, owner, ctx, key := attachmentFixture(t)
	ctx = grpcsession.WithGrant(ctx, [32]byte{1})
	preparation := publicTokenWire(t, testOwner, networkpb.TokenKind_OBSERVATION_PREPARATION)
	cases := []struct {
		name string
		call func() (networkpb.Failure, error)
	}{
		{name: "read", call: func() (networkpb.Failure, error) {
			out, err := service.ReadControl(ctx, &corepb.ReadControlRequest{Key: key})
			if out == nil {
				return 0, err
			}
			return out.GetFailure(), err
		}},
		{name: "proposal", call: func() (networkpb.Failure, error) {
			out, err := service.ReadProposalControl(ctx, &corepb.ReadControlRequest{Key: key})
			if out == nil {
				return 0, err
			}
			return out.GetFailure(), err
		}},
		{name: "check", call: func() (networkpb.Failure, error) {
			out, err := service.CheckControl(ctx, &corepb.CheckControlRequest{Key: key, Preparation: preparation})
			if out == nil {
				return 0, err
			}
			return out.GetFailure(), err
		}},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			failure, err := testCase.call()
			if err != nil || failure != networkpb.Failure_DENIED {
				t.Fatalf("delegated grant changed %s control path: %v %v", testCase.name, failure, err)
			}
		})
	}
	if owner.calls != len(cases) {
		t.Fatalf("delegated non-resolve calls = %d, want %d", owner.calls, len(cases))
	}
}
