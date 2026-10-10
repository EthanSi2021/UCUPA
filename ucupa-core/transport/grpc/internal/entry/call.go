package entry

import (
	"context"
	"errors"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"

	networkpb "ucupa/shared/bindings/go/protobuf"
	"ucupa/shared/values"
	grpcsession "ucupa/ucupa-core/transport/grpc/internal/session"
	"ucupa/ucupa-core/transport/grpc/internal/wire"
)

func (s *Service) call(ctx context.Context, op string, request, response proto.Message) error {
	value, err := wire.ToValue(request.ProtoReflect(), 0)
	if err != nil {
		return status.Error(codes.InvalidArgument, "invalid request")
	}
	object := value.(map[string]any)
	object["op"] = op
	if ref, ok := object["reference"].(map[string]any); ok {
		if ref["nextHop"] != string(s.config.NextHop) {
			return status.Error(codes.InvalidArgument, "reference does not name this entry")
		}
	}
	encoded, err := values.CanonicalJSONValue(object)
	if err != nil || len(encoded) > 4<<20 {
		return status.Error(codes.InvalidArgument, "invalid request")
	}
	caller, ok := grpcsession.Caller(ctx)
	if !ok {
		return status.Error(codes.Unauthenticated, "session required")
	}
	result, err := s.publicOwnerCall(ctx, caller, encoded)
	if err := s.admission.CheckDelegatedResult(ctx, caller); err != nil {
		return err
	}
	m := response.ProtoReflect()
	fields := m.Descriptor().Fields()
	if err != nil {
		var public values.PublicError
		if !errors.As(err, &public) {
			return status.Error(codes.Internal, "owner result unavailable")
		}
		n, ok := networkpb.Failure_value[string(public)]
		if !ok || n == 0 {
			return status.Error(codes.Internal, "owner result unavailable")
		}
		m.Set(fields.ByName("failure"), protoreflect.ValueOfEnum(protoreflect.EnumNumber(n)))
		return nil
	}
	// Request limits do not bound a result containing many individually valid
	// records. Bound the canonical response and its materialization explicitly.
	if len(result) > s.config.MaxResponseBytes {
		return status.Error(codes.ResourceExhausted, "response capacity")
	}
	limit := s.config.MaxResponseBytes
	value, err = values.ParseCanonicalJSON(result, values.JSONLimits{Bytes: limit, Depth: 64, Nodes: limit, ContainerEntries: limit, StringBytes: limit})
	if err != nil {
		return status.Error(codes.Internal, "owner result unavailable")
	}
	f := fields.ByName("result")
	v := m.NewField(f)
	if err := wire.FromValue(v.Message(), value); err != nil {
		return status.Error(codes.Internal, "owner result unavailable")
	}
	m.Set(f, v)
	if proto.Size(response) > s.config.MaxResponseBytes {
		return status.Error(codes.ResourceExhausted, "response capacity")
	}
	return nil
}
