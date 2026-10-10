// Package protovalue maps declared Protobuf fields to local canonical values.
// It is private wire plumbing: it cannot authorize or invoke an owner.
package protovalue

import (
	"encoding/base64"
	"fmt"
	"math"
	"strconv"
	"strings"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	networkpb "ucupa/shared/bindings/go/protobuf"
	"ucupa/shared/values"
)

func required(f protoreflect.FieldDescriptor) bool {
	return proto.GetExtension(f.Options(), networkpb.E_RequiredValue).(bool)
}

func enumLiteral(v protoreflect.EnumValueDescriptor) string {
	if s := proto.GetExtension(v.Options(), networkpb.E_Literal).(string); s != "" {
		return s
	}
	return string(v.Name())
}

// toValue maps only declared typed fields. Field order on the wire is not an
// operation identity. Presence and opaque bytes are preserved in the local JCS.
type Codec struct {
	Validate   func(protoreflect.Message) error
	Arrays     map[protoreflect.FullName]int
	Operations map[protoreflect.FullName]bool
}

func (c Codec) ToValue(m protoreflect.Message, depth int) (any, error) {
	if depth > 32 || len(m.GetUnknown()) != 0 {
		return nil, fmt.Errorf("unknown or deep message")
	}
	d := m.Descriptor()
	if d.FullName() == "ucupa.network.Token" {
		if !values.FactAuthorityRef(m.Get(d.Fields().ByName("owner")).String()).Valid() || len(m.Get(d.Fields().ByName("value")).Bytes()) != 32 {
			return nil, fmt.Errorf("invalid token shape")
		}
	}
	if c.Validate != nil {
		if err := c.Validate(m); err != nil {
			return nil, err
		}
	}
	for i := 0; i < d.Oneofs().Len(); i++ {
		o := d.Oneofs().Get(i)
		if !o.IsSynthetic() && m.WhichOneof(o) == nil {
			return nil, fmt.Errorf("missing choice")
		}
	}
	if size, array := c.Arrays[d.FullName()]; array {
		list := m.Get(d.Fields().Get(0)).List()
		if list.Len() > 4096 || size > 0 && list.Len() != size {
			return nil, fmt.Errorf("invalid array size")
		}
		out := make([]any, list.Len())
		for i := range out {
			v, e := c.scalar(d.Fields().Get(0), list.Get(i), depth)
			if e != nil {
				return nil, e
			}
			out[i] = v
		}
		return out, nil
	}
	if c.Operations[d.FullName()] {
		f := m.WhichOneof(d.Oneofs().Get(0))
		value, err := c.ToValue(m.Get(f).Message(), depth+1)
		if err != nil {
			return nil, err
		}
		object := value.(map[string]any)
		object["op"] = strings.ReplaceAll(string(f.Name()), "_", "-")
		return object, nil
	}
	out := map[string]any{}
	for i := 0; i < d.Fields().Len(); i++ {
		f := d.Fields().Get(i)
		if !m.Has(f) && !f.IsList() {
			if required(f) {
				return nil, fmt.Errorf("missing %s", f.Name())
			}
			continue
		}
		if f.IsList() {
			list := m.Get(f).List()
			if list.Len() > 4096 {
				return nil, fmt.Errorf("array limit")
			}
			a := make([]any, list.Len())
			for j := range a {
				v, e := c.scalar(f, list.Get(j), depth)
				if e != nil {
					return nil, e
				}
				a[j] = v
			}
			out[f.JSONName()] = a
		} else {
			v, e := c.scalar(f, m.Get(f), depth)
			if e != nil {
				return nil, e
			}
			out[f.JSONName()] = v
		}
	}
	return out, nil
}

func (c Codec) scalar(f protoreflect.FieldDescriptor, v protoreflect.Value, depth int) (any, error) {
	switch f.Kind() {
	case protoreflect.MessageKind:
		if kind := proto.GetExtension(f.Options(), networkpb.E_TokenKind).(networkpb.TokenKind); kind != 0 {
			m := v.Message()
			if m.Get(m.Descriptor().Fields().ByName("kind")).Enum() != protoreflect.EnumNumber(kind) {
				return nil, fmt.Errorf("invalid token role")
			}
		}
		return c.ToValue(v.Message(), depth+1)
	case protoreflect.StringKind:
		switch f.Name() {
		case "owner", "core", "facet", "element", "anchor", "parent", "origin", "scope":
			if !values.ElementID(v.String()).Valid() {
				return nil, fmt.Errorf("invalid UUID")
			}
		case "next_hop":
			if !values.NextHop(v.String()).Valid() {
				return nil, fmt.Errorf("invalid next hop")
			}
		}
		if f.ContainingMessage().FullName() == "ucupa.network.ElementPath" && !values.ElementID(v.String()).Valid() {
			return nil, fmt.Errorf("invalid path UUID")
		}
		return v.String(), nil
	case protoreflect.BytesKind:
		if f.Name() == "operation_id" && (len(v.Bytes()) == 0 || len(v.Bytes()) > 128) {
			return nil, fmt.Errorf("invalid operation ID")
		}
		return base64.RawURLEncoding.EncodeToString(v.Bytes()), nil
	case protoreflect.BoolKind:
		return v.Bool(), nil
	case protoreflect.DoubleKind:
		n := v.Float()
		if math.IsNaN(n) || math.IsInf(n, 0) {
			return nil, fmt.Errorf("nonfinite")
		}
		if f.Name() == "radius" && n < 0 {
			return nil, fmt.Errorf("negative radius")
		}
		if n == 0 {
			n = 0
		}
		return n, nil
	case protoreflect.Uint32Kind:
		if f.Name() == "limit" && (v.Uint() == 0 || v.Uint() > 256) || f.Name() == "pivot" && (v.Uint() == 0 || v.Uint() > 4) {
			return nil, fmt.Errorf("integer range")
		}
		return float64(v.Uint()), nil
	case protoreflect.Uint64Kind:
		return strconv.FormatUint(v.Uint(), 10), nil
	case protoreflect.Sint64Kind:
		if f.Name() == "up_depth" && v.Int() < -1 {
			return nil, fmt.Errorf("invalid up depth")
		}
		return strconv.FormatInt(v.Int(), 10), nil
	case protoreflect.EnumKind:
		e := f.Enum().Values().ByNumber(v.Enum())
		if e == nil || v.Enum() == 0 {
			return nil, fmt.Errorf("unknown enum")
		}
		return enumLiteral(e), nil
	}
	return nil, fmt.Errorf("unsupported field")
}

// fromValue decodes an owner-produced canonical value into the declared result
// type; unsupported fields fail closed rather than disappearing in translation.
func (c Codec) FromValue(m protoreflect.Message, raw any) error {
	d := m.Descriptor()
	if c.Operations[d.FullName()] {
		object, ok := raw.(map[string]any)
		if !ok {
			return fmt.Errorf("invalid operation")
		}
		op, ok := object["op"].(string)
		if !ok {
			return fmt.Errorf("missing operation")
		}
		f := d.Fields().ByName(protoreflect.Name(strings.ReplaceAll(op, "-", "_")))
		if f == nil || strings.ReplaceAll(string(f.Name()), "_", "-") != op {
			return fmt.Errorf("unknown operation")
		}
		body := make(map[string]any, len(object)-1)
		for k, v := range object {
			if k != "op" {
				body[k] = v
			}
		}
		child := m.NewField(f)
		if err := c.FromValue(child.Message(), body); err != nil {
			return err
		}
		m.Set(f, child)
		_, err := c.ToValue(m, 0)
		return err
	}
	if _, array := c.Arrays[d.FullName()]; array {
		a, ok := raw.([]any)
		if !ok {
			return fmt.Errorf("invalid array")
		}
		f := d.Fields().Get(0)
		l := m.Mutable(f).List()
		for _, item := range a {
			v, e := c.fromScalar(m, f, item)
			if e != nil {
				return e
			}
			l.Append(v)
		}
		return nil
	}
	object, ok := raw.(map[string]any)
	if !ok {
		return fmt.Errorf("invalid object")
	}
	for name, raw := range object {
		f := d.Fields().ByJSONName(name)
		if f == nil {
			return fmt.Errorf("unexpected result field %s", name)
		}
		if f.IsList() {
			a, ok := raw.([]any)
			if !ok {
				return fmt.Errorf("invalid list")
			}
			l := m.Mutable(f).List()
			for _, item := range a {
				v, e := c.fromScalar(m, f, item)
				if e != nil {
					return e
				}
				l.Append(v)
			}
		} else {
			v, e := c.fromScalar(m, f, raw)
			if e != nil {
				return e
			}
			if o := f.ContainingOneof(); o != nil && !o.IsSynthetic() && m.WhichOneof(o) != nil {
				return fmt.Errorf("ambiguous result")
			}
			m.Set(f, v)
		}
	}
	_, err := c.ToValue(m, 0)
	return err
}

func (c Codec) fromScalar(m protoreflect.Message, f protoreflect.FieldDescriptor, raw any) (protoreflect.Value, error) {
	bad := func() (protoreflect.Value, error) { return protoreflect.Value{}, fmt.Errorf("invalid %s", f.Name()) }
	switch f.Kind() {
	case protoreflect.MessageKind:
		var child protoreflect.Message
		if f.IsList() {
			child = m.NewField(f).List().NewElement().Message()
		} else {
			child = m.NewField(f).Message()
		}
		if e := c.FromValue(child, raw); e != nil {
			return protoreflect.Value{}, e
		}
		return protoreflect.ValueOfMessage(child), nil
	case protoreflect.StringKind:
		s, ok := raw.(string)
		if !ok {
			return bad()
		}
		return protoreflect.ValueOfString(s), nil
	case protoreflect.BytesKind:
		s, ok := raw.(string)
		if !ok {
			return bad()
		}
		b, e := base64.RawURLEncoding.Strict().DecodeString(s)
		if e != nil {
			return bad()
		}
		return protoreflect.ValueOfBytes(b), nil
	case protoreflect.BoolKind:
		b, ok := raw.(bool)
		if !ok {
			return bad()
		}
		return protoreflect.ValueOfBool(b), nil
	case protoreflect.DoubleKind:
		n, ok := raw.(float64)
		if !ok || math.IsNaN(n) || math.IsInf(n, 0) {
			return bad()
		}
		return protoreflect.ValueOfFloat64(n), nil
	case protoreflect.Uint32Kind:
		n, ok := raw.(float64)
		if !ok || n < 0 || n > math.MaxUint32 || math.Trunc(n) != n {
			return bad()
		}
		return protoreflect.ValueOfUint32(uint32(n)), nil
	case protoreflect.Uint64Kind:
		s, ok := raw.(string)
		if !ok {
			return bad()
		}
		n, e := strconv.ParseUint(s, 10, 64)
		if e != nil {
			return bad()
		}
		return protoreflect.ValueOfUint64(n), nil
	case protoreflect.EnumKind:
		s, ok := raw.(string)
		if !ok {
			return bad()
		}
		for i := 1; i < f.Enum().Values().Len(); i++ {
			v := f.Enum().Values().Get(i)
			if enumLiteral(v) == s {
				return protoreflect.ValueOfEnum(v.Number()), nil
			}
		}
	}
	return bad()
}
