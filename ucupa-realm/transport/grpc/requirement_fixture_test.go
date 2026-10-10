package realmgrpc

import (
	"google.golang.org/protobuf/proto"

	"ucupa/shared/values"
	realmpb "ucupa/ucupa-realm/bindings/go/protobuf"
)

const requirementsTestOwner values.FactAuthorityRef = "00000000-0000-4000-8000-000000000001"

const requirementsTestElement values.ElementID = "00000000-0000-4000-8000-000000000010"

func coreElementRequirement(permission string) *realmpb.Requirement {
	return &realmpb.Requirement{
		Permission: proto.String(permission),
		Scope: &realmpb.Scope{
			Kind:      realmpb.ScopeKind_CORE_ELEMENT.Enum(),
			Authority: proto.String(string(requirementsTestOwner)),
			Element:   proto.String(string(requirementsTestElement)),
		},
	}
}
