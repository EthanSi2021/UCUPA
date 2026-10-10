package realmsdk

import (
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"

	networkpb "ucupa/shared/bindings/go/protobuf"
)

// maxResponseRequirements mirrors the fixed protocol ceiling the service
// enforces; a larger reply is not a shape this SDK accepts.
const maxResponseRequirements = 64

// validateRealmResponse rejects malformed success replies before a caller
// sees them: wire shape only, never permission or content semantics.
func validateRealmResponse(m protoreflect.Message, depth int) error {
	if depth > 32 || len(m.GetUnknown()) != 0 {
		return status.Error(codes.DataLoss, "invalid Realm response")
	}
	d := m.Descriptor()
	fields := d.Fields()
	for i := 0; i < fields.Len(); i++ {
		f := fields.Get(i)
		if !m.Has(f) && !f.IsList() {
			if proto.GetExtension(f.Options(), networkpb.E_RequiredValue).(bool) {
				return status.Error(codes.DataLoss, "invalid Realm response")
			}
			continue
		}
		if f.IsList() {
			list := m.Get(f).List()
			if list.Len() > maxResponseRequirements {
				return status.Error(codes.DataLoss, "invalid Realm response")
			}
			for j := 0; j < list.Len(); j++ {
				if err := checkRealmResponseField(f, list.Get(j), depth); err != nil {
					return err
				}
			}
			continue
		}
		if err := checkRealmResponseField(f, m.Get(f), depth); err != nil {
			return err
		}
	}
	return nil
}

func checkRealmResponseField(f protoreflect.FieldDescriptor, v protoreflect.Value, depth int) error {
	bad := func() error { return status.Error(codes.DataLoss, "invalid Realm response") }
	switch f.Kind() {
	case protoreflect.MessageKind:
		return validateRealmResponse(v.Message(), depth+1)
	case protoreflect.EnumKind:
		if v.Enum() == 0 || f.Enum().Values().ByNumber(v.Enum()) == nil {
			return bad()
		}
	case protoreflect.BytesKind:
		switch f.Name() {
		case "caller", "view", "denial", "challenge", "handle":
			if len(v.Bytes()) != 32 {
				return bad()
			}
		case "signature":
			if len(v.Bytes()) != 64 {
				return bad()
			}
		case "credential_id":
			if len(v.Bytes()) != 16 {
				return bad()
			}
		case "secret":
			if len(v.Bytes()) != 32 {
				return bad()
			}
		}
	}
	return nil
}
