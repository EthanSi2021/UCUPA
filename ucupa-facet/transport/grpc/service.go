// Package facetgrpc adapts typed content RPCs to the existing Facet boundary.
// Connection admission and session establishment belong to its serving host.
package facetgrpc

import (
	"context"
	"errors"
	"fmt"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	networkpb "ucupa/shared/bindings/go/protobuf"
	"ucupa/shared/values"
	facetpb "ucupa/ucupa-facet/bindings/go/protobuf"
)

type Owner interface {
	AuthorityRef() values.FactAuthorityRef
	PublicCall(context.Context, values.CallerContext, []byte) ([]byte, error)
}

// Callers must obtain an already authenticated content caller from the host's
// session boundary. It must never construct one from request identity fields,
// nor substitute the fixed service caller used for Core control reads.
type Callers interface {
	Caller(context.Context) (values.CallerContext, error)
}

type Service struct {
	facetpb.UnimplementedFacetServiceServer
	owner       Owner
	callers     Callers
	expected    values.FactAuthorityRef
	nextHop     values.NextHop
	maxResponse int
}

// NewService only constructs the RPC adapter; it does not open a listener or
// install authentication. A host must supply the bounded authenticated runtime.
func NewService(owner Owner, callers Callers, expected values.FactAuthorityRef, nextHop values.NextHop, maxResponse int) (*Service, error) {
	_, network := nextHop.NetworkTarget()
	if owner == nil || callers == nil || !expected.Valid() || owner.AuthorityRef() != expected || !network || maxResponse <= 0 {
		return nil, fmt.Errorf("invalid Facet adapter configuration")
	}
	return &Service{owner: owner, callers: callers, expected: expected, nextHop: nextHop, maxResponse: maxResponse}, nil
}

func (s *Service) call(ctx context.Context, op string, in, out proto.Message) error {
	if err := ctx.Err(); err != nil {
		return status.FromContextError(err).Err()
	}
	if s.owner.AuthorityRef() != s.expected {
		return status.Error(codes.Unavailable, "target unavailable")
	}
	if proto.Size(in) > 4<<20 {
		return status.Error(codes.ResourceExhausted, "request capacity")
	}
	v, err := representation.ToValue(in.ProtoReflect(), 0)
	if err != nil {
		return status.Error(codes.InvalidArgument, "invalid request")
	}
	object := v.(map[string]any)
	object["op"] = op
	if access, ok := object["access"].(map[string]any); ok {
		key := access["key"].(map[string]any)
		if access["nextHop"] != string(s.nextHop) || key["facet"] != string(s.expected) {
			return status.Error(codes.InvalidArgument, "access does not name this entry")
		}
	}
	encoded, err := values.CanonicalJSONValue(object)
	if err != nil || len(encoded) > 4<<20 {
		return status.Error(codes.InvalidArgument, "invalid request")
	}
	caller, err := s.callers.Caller(ctx)
	if err != nil {
		switch status.Code(err) {
		case codes.Unavailable:
			return status.Error(codes.Unavailable, "authentication unavailable")
		case codes.ResourceExhausted:
			return status.Error(codes.ResourceExhausted, "authentication capacity")
		case codes.Canceled, codes.DeadlineExceeded:
			return status.Error(status.Code(err), "authentication interrupted")
		default:
			return status.Error(codes.Unauthenticated, "session required")
		}
	}
	if caller == (values.CallerContext{}) {
		return status.Error(codes.Unauthenticated, "session required")
	}
	raw, err := s.owner.PublicCall(ctx, caller, encoded)
	if s.owner.AuthorityRef() != s.expected {
		return status.Error(codes.Unavailable, "target unavailable")
	}
	m := out.ProtoReflect()
	if err != nil {
		var public values.PublicError
		if !errors.As(err, &public) {
			return status.Error(codes.Internal, "owner result unavailable")
		}
		n, ok := networkpb.Failure_value[string(public)]
		if !ok || n == 0 {
			return status.Error(codes.Internal, "owner result unavailable")
		}
		m.Set(m.Descriptor().Fields().ByName("failure"), protoreflect.ValueOfEnum(protoreflect.EnumNumber(n)))
		return nil
	}
	if len(raw) > s.maxResponse {
		return status.Error(codes.ResourceExhausted, "response capacity")
	}
	n := s.maxResponse
	v, err = values.ParseCanonicalJSON(raw, values.JSONLimits{Bytes: n, Depth: 64, Nodes: n, ContainerEntries: n, StringBytes: n})
	if err != nil {
		return status.Error(codes.Internal, "owner result unavailable")
	}
	f := m.Descriptor().Fields().ByName("result")
	result := m.NewField(f)
	if err := representation.FromValue(result.Message(), v); err != nil {
		return status.Error(codes.Internal, "owner result unavailable")
	}
	m.Set(f, result)
	if proto.Size(out) > s.maxResponse {
		return status.Error(codes.ResourceExhausted, "response capacity")
	}
	return nil
}
