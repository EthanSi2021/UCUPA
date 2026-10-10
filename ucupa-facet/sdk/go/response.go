package facetsdk

import (
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	networkpb "ucupa/shared/bindings/go/protobuf"
	"ucupa/shared/values"
)

// Reject malformed success and foreign-owner receipts before handing them to
// a caller. This checks representation, not permission or content semantics.
func validateResponse(m protoreflect.Message, owner values.FactAuthorityRef, depth int) error {
	bad := func() error { return status.Error(codes.DataLoss, "invalid owner response") }
	if depth > 32 || len(m.GetUnknown()) != 0 {
		return bad()
	}
	d := m.Descriptor()
	fields := d.Fields()
	for i := 0; i < d.Oneofs().Len(); i++ {
		o := d.Oneofs().Get(i)
		if !o.IsSynthetic() && m.WhichOneof(o) == nil {
			return bad()
		}
	}
	if d.FullName() == "ucupa.network.Token" && (m.Get(fields.ByName("owner")).String() != string(owner) || len(m.Get(fields.ByName("value")).Bytes()) != 32) {
		return bad()
	}
	if d.FullName() == "ucupa.facet.GetResult" {
		state := m.Get(fields.ByName("state")).Enum()
		payload, revision := m.Has(fields.ByName("payload")), m.Has(fields.ByName("revision"))
		if state == 1 && (!payload || !revision) || state == 2 && (payload || revision) {
			return bad()
		}
	}
	if d.FullName() == "ucupa.facet.PublicAcceptanceRef" {
		if !values.FactAuthorityRef(m.Get(fields.ByName("core")).String()).Valid() || !values.ElementID(m.Get(fields.ByName("element")).String()).Valid() {
			return bad()
		}
	}
	if d.FullName() == "ucupa.facet.PublicAcceptanceInspection" {
		found := m.Get(fields.ByName("found")).Bool()
		address, revision := m.Has(fields.ByName("core_next_hop")), m.Has(fields.ByName("contact_revision"))
		if found && (!address || !revision || !values.NextHop(m.Get(fields.ByName("core_next_hop")).String()).Valid()) || !found && (address || revision) {
			return bad()
		}
	}
	if d.FullName() == "ucupa.facet.ChangeGroup" {
		if m.Get(fields.ByName("owner")).String() != string(owner) {
			return bad()
		}
		l := m.Get(fields.ByName("category")).List()
		seen := map[protoreflect.EnumNumber]bool{}
		if l.Len() < 1 || l.Len() > 4 {
			return bad()
		}
		for i := 0; i < l.Len(); i++ {
			n := l.Get(i).Enum()
			if seen[n] {
				return bad()
			}
			seen[n] = true
		}
	}
	for i := 0; i < fields.Len(); i++ {
		f := fields.Get(i)
		if !m.Has(f) && !f.IsList() {
			if proto.GetExtension(f.Options(), networkpb.E_RequiredValue).(bool) {
				return bad()
			}
			continue
		}
		check := func(v protoreflect.Value) error {
			switch f.Kind() {
			case protoreflect.MessageKind:
				if kind := proto.GetExtension(f.Options(), networkpb.E_TokenKind).(networkpb.TokenKind); kind != 0 && v.Message().Get(v.Message().Descriptor().Fields().ByName("kind")).Enum() != protoreflect.EnumNumber(kind) {
					return bad()
				}
				tokenOwner := owner
				// Only this field is a comparison issued by the related Core.
				// All Facet receipt/contact/content tokens retain this Facet owner.
				if d.FullName() == "ucupa.facet.PublicAcceptanceRef" && f.Name() == "proposal" {
					tokenOwner = values.FactAuthorityRef(m.Get(fields.ByName("core")).String())
				}
				return validateResponse(v.Message(), tokenOwner, depth+1)
			case protoreflect.EnumKind:
				if v.Enum() == 0 || f.Enum().Values().ByNumber(v.Enum()) == nil {
					return bad()
				}
			case protoreflect.BytesKind:
				if (f.Name() == "type_ref" || f.Name() == "types") && len(v.Bytes()) != 32 {
					return bad()
				}
			case protoreflect.BoolKind:
				if f.Name() == "released" && !v.Bool() {
					return bad()
				}
			}
			return nil
		}
		if f.IsList() {
			l := m.Get(f).List()
			if l.Len() > 256 {
				return bad()
			}
			for j := 0; j < l.Len(); j++ {
				if err := check(l.Get(j)); err != nil {
					return err
				}
			}
		} else if err := check(m.Get(f)); err != nil {
			return err
		}
	}
	return nil
}
