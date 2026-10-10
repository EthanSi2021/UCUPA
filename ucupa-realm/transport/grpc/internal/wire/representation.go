package wire

import (
	"fmt"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/reflect/protoreflect"

	"ucupa/internal/protovalue"
)

var Representation = protovalue.Codec{Validate: validateShape}

func Exact32(raw []byte) ([32]byte, error) {
	var out [32]byte
	if len(raw) != 32 {
		return out, status.Error(codes.InvalidArgument, "invalid handle")
	}
	copy(out[:], raw)
	return out, nil
}

// These checks concern wire shape only. Identity, authorization continuity
// and every authorization decision remain the original kernel's work.
func validateShape(m protoreflect.Message) error {
	d := m.Descriptor()
	field := func(name string, size int) error {
		f := d.Fields().ByName(protoreflect.Name(name))
		if m.Has(f) && len(m.Get(f).Bytes()) != size {
			return fmt.Errorf("invalid %s", name)
		}
		return nil
	}
	switch d.FullName() {
	case "ucupa.realm.EstablishCallerRequest":
		if err := field("credential_id", 16); err != nil {
			return err
		}
		return field("secret", 32)
	case "ucupa.realm.BeginChallengeResponse":
		return field("challenge", 32)
	case "ucupa.realm.EstablishForeignCallerRequest":
		if f := d.Fields().ByName("payload"); m.Has(f) && len(m.Get(f).Bytes()) > 4096 {
			return fmt.Errorf("invalid payload")
		}
		if err := field("signature", 64); err != nil {
			return err
		}
		return field("challenge", 32)
	case "ucupa.realm.EstablishCallerResponse", "ucupa.realm.EstablishForeignCallerResponse", "ucupa.realm.ForkCallerResponse":
		return field("caller", 32)
	case "ucupa.realm.ForkCallerRequest":
		return field("caller", 32)
	case "ucupa.realm.ReleaseHandleRequest":
		return field("handle", 32)
	case "ucupa.realm.AuthorizeViewRequest":
		if err := field("caller", 32); err != nil {
			return err
		}
	case "ucupa.realm.ValidateViewRequest":
		if err := field("caller", 32); err != nil {
			return err
		}
		return field("view", 32)
	case "ucupa.realm.CaptureDenialViewRequest":
		if err := field("caller", 32); err != nil {
			return err
		}
	case "ucupa.realm.ValidateDenialViewRequest":
		if err := field("caller", 32); err != nil {
			return err
		}
		return field("denial", 32)
	}
	if f := d.Fields().ByName("requirements"); f != nil && f.IsList() && m.Get(f).List().Len() > MaxRequirements {
		return fmt.Errorf("too many requirements")
	}
	return nil
}
