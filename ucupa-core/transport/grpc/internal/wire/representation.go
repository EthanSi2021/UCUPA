package wire

import (
	"fmt"
	"google.golang.org/protobuf/reflect/protoreflect"
	"ucupa/internal/protovalue"
)

var representation = protovalue.Codec{
	Arrays:     map[protoreflect.FullName]int{"ucupa.network.Vector3": 3, "ucupa.network.ElementPath": 0},
	Operations: map[protoreflect.FullName]bool{"ucupa.core.MutationCommand": true},
	Validate: func(m protoreflect.Message) error {
		d := m.Descriptor()
		if d.FullName() == "ucupa.core.Range" {
			kind := m.Get(d.Fields().ByName("kind")).Enum()
			radius := m.Has(d.Fields().ByName("radius"))
			if kind == 1 && !radius || kind == 2 && radius {
				return fmt.Errorf("invalid range shape")
			}
		}
		return nil
	},
}

func ToValue(m protoreflect.Message, depth int) (any, error) { return representation.ToValue(m, depth) }
func FromValue(m protoreflect.Message, raw any) error        { return representation.FromValue(m, raw) }
