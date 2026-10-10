package coresdk

import (
	"math"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	networkpb "ucupa/shared/bindings/go/protobuf"
	"ucupa/shared/values"
)

// Client-side structural validation does not reproduce domain adjudication.
// A decoded protobuf or gRPC OK alone is not a valid owner response.
func validateResponse(m protoreflect.Message, depth int) error {
	bad := func() error { return status.Error(codes.DataLoss, "invalid owner response") }
	if depth > 32 || len(m.GetUnknown()) != 0 {
		return bad()
	}
	d := m.Descriptor()
	if d.FullName() == "ucupa.core.MountApprovalBinding" {
		digest := m.Get(d.Fields().ByName("command_digest")).Bytes()
		op := m.Get(d.Fields().ByName("operation_id")).Bytes()
		if len(digest) != 32 || [32]byte(digest) == ([32]byte{}) || len(op) == 0 || len(op) > 128 {
			return bad()
		}
	}
	if d.FullName() == "ucupa.core.MountApproval" {
		if len(m.Get(d.Fields().ByName("token")).Bytes()) != 32 || m.Get(d.Fields().ByName("expires_in_seconds")).Uint() == 0 {
			return bad()
		}
	}
	if d.FullName() == "ucupa.core.MountApprovalCheck" || d.FullName() == "ucupa.core.ServicePrincipal" {
		p := values.PrincipalRef{Realm: values.RealmID(m.Get(d.Fields().ByName("realm")).String()), Principal: values.PrincipalID(m.Get(d.Fields().ByName("principal")).String())}
		if !p.Valid() {
			return bad()
		}
	}
	if d.FullName() == "ucupa.core.ServiceRevision" || d.FullName() == "ucupa.core.ServiceCoreCut" {
		b := m.Get(d.Fields().ByName("incarnation")).Bytes()
		if len(b) != 32 || [32]byte(b) == ([32]byte{}) || m.Get(d.Fields().ByName("sequence")).Uint() == 0 {
			return bad()
		}
	}
	if d.FullName() == "ucupa.core.ControlRead" {
		present := m.Get(d.Fields().ByName("presence")).Enum() == 1
		if present != m.Has(d.Fields().ByName("state")) || present != m.Has(d.Fields().ByName("revision")) {
			return bad()
		}
		state := m.Get(d.Fields().ByName("state")).Enum()
		if m.Has(d.Fields().ByName("active")) != (present && (state == 2 || state == 3)) {
			return bad()
		}
		if m.Has(d.Fields().ByName("proposal")) && (!present || (state != 2 && state != 3 && !m.Has(d.Fields().ByName("pending")))) {
			return bad()
		}
	}
	if d.FullName() == "ucupa.core.CheckControlResult" && !m.Get(d.Fields().ByName("valid")).Bool() {
		return bad()
	}
	if d.FullName() == "ucupa.core.IdentityChallenge" && (len(m.Get(d.Fields().ByName("challenge")).Bytes()) != 32 || m.Get(d.Fields().ByName("expires_in_seconds")).Uint() == 0) {
		return bad()
	}
	if d.FullName() == "ucupa.network.IdentityProof" && (len(m.Get(d.Fields().ByName("payload")).Bytes()) == 0 || len(m.Get(d.Fields().ByName("payload")).Bytes()) > 4096 || len(m.Get(d.Fields().ByName("signature")).Bytes()) != 64) {
		return bad()
	}
	if d.FullName() == "ucupa.network.CheckIdentityProofResponse" && !m.Get(d.Fields().ByName("valid")).Bool() {
		return bad()
	}
	if d.FullName() == "ucupa.core.ReadDelegation" && (len(m.Get(d.Fields().ByName("token")).Bytes()) != 32 || m.Get(d.Fields().ByName("expires_in_seconds")).Uint() == 0) {
		return bad()
	}
	if d.FullName() == "ucupa.network.Token" && (!values.FactAuthorityRef(m.Get(d.Fields().ByName("owner")).String()).Valid() || len(m.Get(d.Fields().ByName("value")).Bytes()) != 32) {
		return bad()
	}
	if d.FullName() == "ucupa.network.Vector3" && m.Get(d.Fields().Get(0)).List().Len() != 3 {
		return bad()
	}
	if d.FullName() == "ucupa.core.ReadElementResult" {
		present := m.Get(d.Fields().ByName("state")).Enum() == 1
		if present != m.Has(d.Fields().ByName("record")) || present != m.Has(d.Fields().ByName("attachment")) {
			return bad()
		}
	}
	for i := 0; i < d.Oneofs().Len(); i++ {
		o := d.Oneofs().Get(i)
		if !o.IsSynthetic() && m.WhichOneof(o) == nil {
			return bad()
		}
	}
	for i := 0; i < d.Fields().Len(); i++ {
		f := d.Fields().Get(i)
		if !m.Has(f) && !f.IsList() {
			if proto.GetExtension(f.Options(), networkpb.E_RequiredValue).(bool) {
				return bad()
			}
			continue
		}
		check := func(v protoreflect.Value) error {
			switch f.Kind() {
			case protoreflect.StringKind:
				switch f.Name() {
				case "owner", "core", "facet", "element", "anchor", "parent", "origin", "scope", "actor", "target", "parent_core", "carrier_core":
					if !values.ElementID(v.String()).Valid() {
						return bad()
					}
				case "next_hop", "parent_next_hop", "carrier_next_hop":
					if !values.NextHop(v.String()).Valid() {
						return bad()
					}
				}
				if d.FullName() == "ucupa.network.ElementPath" && !values.ElementID(v.String()).Valid() {
					return bad()
				}
			case protoreflect.BoolKind:
				if (f.Name() == "closed" || f.Name() == "released") && !v.Bool() {
					return bad()
				}
			case protoreflect.MessageKind:
				if kind := proto.GetExtension(f.Options(), networkpb.E_TokenKind).(networkpb.TokenKind); kind != 0 {
					m := v.Message()
					if m.Get(m.Descriptor().Fields().ByName("kind")).Enum() != protoreflect.EnumNumber(kind) {
						return bad()
					}
				}
				return validateResponse(v.Message(), depth+1)
			case protoreflect.EnumKind:
				if v.Enum() == 0 || f.Enum().Values().ByNumber(v.Enum()) == nil {
					return bad()
				}
			case protoreflect.DoubleKind:
				if math.IsNaN(v.Float()) || math.IsInf(v.Float(), 0) {
					return bad()
				}
			}
			return nil
		}
		if f.IsList() {
			l := m.Get(f).List()
			if l.Len() > 4096 {
				return bad()
			}
			for j := 0; j < l.Len(); j++ {
				if err := check(l.Get(j)); err != nil {
					return err
				}
			}
		} else {
			if err := check(m.Get(f)); err != nil {
				return err
			}
		}
	}
	return nil
}

// A response from this Core cannot smuggle another owner's receipt/handle into
// an authenticated channel. This does not try to reinterpret Facet identities.
func validateTokenOwners(m protoreflect.Message, owner values.FactAuthorityRef) error {
	if handled, err := validateTransitionApprovalOwners(m, owner); handled {
		return err
	}
	if handled, err := validateCarrierApprovalOwners(m, owner); handled {
		return err
	}
	if handled, err := validateFacetResponseOwners(m, owner); handled {
		return err
	}
	if handled, err := validatePublicReadOwners(m, owner); handled {
		return err
	}
	if (m.Descriptor().FullName() == "ucupa.core.MountApproval" || m.Descriptor().FullName() == "ucupa.core.MountApprovalCheck") && m.Get(m.Descriptor().Fields().ByName("target")).String() != string(owner) {
		return status.Error(codes.DataLoss, "unexpected approval target")
	}
	if m.Descriptor().FullName() == "ucupa.core.ControlRead" && m.Get(m.Descriptor().Fields().ByName("core")).String() != string(owner) {
		return status.Error(codes.DataLoss, "unexpected control owner")
	}
	if m.Descriptor().FullName() == "ucupa.network.Token" && m.Get(m.Descriptor().Fields().ByName("owner")).String() != string(owner) {
		return status.Error(codes.DataLoss, "unexpected response owner")
	}
	var err error
	m.Range(func(f protoreflect.FieldDescriptor, v protoreflect.Value) bool {
		if f.Kind() == protoreflect.MessageKind {
			if f.IsList() {
				for i := 0; i < v.List().Len(); i++ {
					if err = validateTokenOwners(v.List().Get(i).Message(), owner); err != nil {
						return false
					}
				}
			} else {
				err = validateTokenOwners(v.Message(), owner)
			}
		}
		return err == nil
	})
	return err
}
