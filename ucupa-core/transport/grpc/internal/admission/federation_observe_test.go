package admission

import (
	federationcontract "ucupa/ucupa-core/contract/federation"

	"context"
	"google.golang.org/protobuf/proto"
	"testing"
	netpb "ucupa/shared/bindings/go/protobuf"
	"ucupa/shared/values"

	pb "ucupa/ucupa-core/bindings/go/protobuf"
)

type observingFederationOwner struct{ *federationOwnerMock }

func (o observingFederationOwner) OpenFederationReadPart(ctx context.Context, c values.CallerContext, a values.FactAuthorityRef, r federationcontract.FederationReadRequest) (federationcontract.FederationReadPart, error) {
	part, err := o.federationOwnerMock.OpenFederationReadPart(ctx, c, a, r)
	if r.PrepareObserve {
		p := part.Basis
		p.Kind = "observation-preparation"
		part.Preparation = &p
	}
	return part, err
}
func (o observingFederationOwner) ObserveFederationReadPart(_ context.Context, c values.CallerContext, a values.FactAuthorityRef, b values.PublicToken, _ federationcontract.FederationObserveRequest) (federationcontract.FederationObservationPage, error) {
	if c != o.caller || a != o.source || b != o.part.Basis {
		panic("observation requester changed")
	}
	o.pages++
	if o.after != nil {
		o.after()
	}
	cursor := b
	cursor.Kind = "observation-cursor"
	return federationcontract.FederationObservationPage{Cursor: cursor}, nil
}

func TestFederationObservationNetworkBoundary(t *testing.T) {
	for _, name := range []string{"valid", "not prepared", "no grant", "basis", "begin limit", "unknown", "poll missing", "poll oversized", "identity after"} {
		t.Run(name, func(t *testing.T) {
			s, o, i, ctx, opening := federationFixture(t)
			s.config.Federation = observingFederationOwner{o}
			opening.Request.PrepareObserve = proto.Bool(name != "not prepared")
			_, g := exchangeFederation(t, s, ctx, opening)
			r := &pb.ObserveFederationReadPartRequest{Basis: proto.Clone(g.Part.Basis).(*netpb.Token), Operation: proto.String("begin")}
			ctx = federationGrantContext(t, s, ctx, g, pb.FederationReadService_ObserveFederationReadPart_FullMethodName, r)
			switch name {
			case "no grant":
				ctx = context.Background()
			case "basis":
				r.Basis.Value[0] = 1
			case "begin limit":
				r.Limit = proto.Uint32(0)
			case "unknown":
				r.Operation = proto.String("read")
			case "poll missing":
				r.Operation = proto.String("poll")
				r.Limit = proto.Uint32(1)
			case "poll oversized":
				r.Operation = proto.String("poll")
				r.Limit = proto.Uint32(257)
			case "identity after":
				o.after = func() { i.err = values.PublicDenied }
			}
			out, err := s.ObserveFederationReadPart(ctx, r)
			if name == "valid" {
				if err != nil || out.GetResult() == nil || o.pages != 1 {
					t.Fatal(out, err)
				}
			} else {
				wantCalls := 0
				if name == "identity after" {
					wantCalls = 1
				}
				if err == nil || out != nil || o.pages != wantCalls {
					t.Fatal("unsafe observation reached caller", out, err, o.pages)
				}
			}
		})
	}
}
