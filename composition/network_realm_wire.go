package composition

import (
	"google.golang.org/protobuf/proto"

	"ucupa/shared/values"
	realmpb "ucupa/ucupa-realm/bindings/go/protobuf"
)

// maxNetworkRequirements mirrors the fixed wire ceiling for one requirement
// set. A caller cannot ask for more than the protocol can ever answer.
const maxNetworkRequirements = 64

func networkHandle(raw []byte) ([32]byte, bool) {
	var handle [32]byte
	if len(raw) != 32 {
		return [32]byte{}, false
	}
	copy(handle[:], raw)
	return handle, true
}

func networkView(raw []byte) (values.AuthorizationView, bool) {
	view, ok := networkHandle(raw)
	return values.AuthorizationView(view), ok
}

func networkDenial(raw []byte) (values.AuthorizationDenialView, bool) {
	denial, ok := networkHandle(raw)
	return values.AuthorizationDenialView(denial), ok
}

// requirementToWire encodes one typed kernel requirement in the exact wire
// shape the receiving Realm expects. Scope kinds stay distinct; no scope is
// generalized, defaulted or dropped during encoding.
func requirementToWire(requirement values.Requirement) (*realmpb.Requirement, bool) {
	if requirement.Permission == "" {
		return nil, false
	}
	scope := &realmpb.Scope{}
	if authority, element, ok := requirement.Scope.CoreElementTarget(); ok {
		scope.Kind = realmpb.ScopeKind_CORE_ELEMENT.Enum()
		scope.Authority, scope.Element = proto.String(string(authority)), proto.String(string(element))
	} else if authority, element, ok := requirement.Scope.FacetElementTarget(); ok {
		scope.Kind = realmpb.ScopeKind_FACET_ELEMENT.Enum()
		scope.Authority, scope.Element = proto.String(string(authority)), proto.String(string(element))
	} else if authority, key, ok := requirement.Scope.FacetKeyTarget(); ok {
		scope.Kind = realmpb.ScopeKind_FACET_KEY.Enum()
		scope.Authority, scope.Element, scope.Facet = proto.String(string(authority)), proto.String(string(key.Element)), key.Type.Bytes()
	} else if id, ok := requirement.Scope.RealmTarget(); ok {
		scope.Kind = realmpb.ScopeKind_REALM.Enum()
		scope.Realm = proto.String(string(id))
	} else if authority, ok := requirement.Scope.AuthorityTarget(); ok {
		scope.Kind = realmpb.ScopeKind_AUTHORITY.Enum()
		scope.Authority = proto.String(string(authority))
	} else if authority, element, ok := requirement.Scope.AnchorClosureTarget(); ok {
		scope.Kind = realmpb.ScopeKind_ANCHOR_CLOSURE.Enum()
		scope.Authority, scope.Element = proto.String(string(authority)), proto.String(string(element))
	} else {
		return nil, false
	}
	return &realmpb.Requirement{Permission: proto.String(string(requirement.Permission)), Scope: scope}, true
}

// requirementsToWire applies the fixed protocol ceiling; an empty set or one
// above the ceiling can never be answered and is refused before any request.
func requirementsToWire(requirements []values.Requirement) ([]*realmpb.Requirement, bool) {
	if len(requirements) == 0 || len(requirements) > maxNetworkRequirements {
		return nil, false
	}
	wire := make([]*realmpb.Requirement, len(requirements))
	for i, requirement := range requirements {
		encoded, ok := requirementToWire(requirement)
		if !ok {
			return nil, false
		}
		wire[i] = encoded
	}
	return wire, true
}
