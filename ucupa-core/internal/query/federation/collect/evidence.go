package collect

import (
	"ucupa/shared/values"
	federationcontract "ucupa/ucupa-core/contract/federation"
)

// evidenceNode is private capture evidence, not a directory or disclosure.
type evidenceNode struct {
	part     federationcontract.FederationReadPart
	parent   values.PublicToken
	outgoing federationcontract.FederationMountRelation
}

func freezeEvidence(part federationcontract.FederationReadPart) federationcontract.FederationReadPart {
	if part.Incoming != nil {
		copy := *part.Incoming
		part.Incoming = &copy
	}
	if part.Preparation != nil {
		copy := *part.Preparation
		part.Preparation = &copy
	}
	return part
}

func sameEvidence(a, b federationcontract.FederationReadPart) bool {
	if (a.Incoming == nil) != (b.Incoming == nil) || (a.Preparation == nil) != (b.Preparation == nil) {
		return false
	}
	if a.Incoming != nil && *a.Incoming != *b.Incoming {
		return false
	}
	if a.Preparation != nil && *a.Preparation != *b.Preparation {
		return false
	}
	a.Incoming, b.Incoming, a.Preparation, b.Preparation = nil, nil, nil, nil
	return a == b
}

func (v *sources) checkEvidenceGraph() error {
	if len(v.parts) == 0 || len(v.graph) != len(v.parts) {
		return values.PublicInvalidContext
	}
	seen := make(map[string]values.PublicToken, len(v.parts))
	for i, connection := range v.parts {
		part := connection.Part()
		node, ok := v.graph[part.Basis.Value]
		if !ok || !sameEvidence(part, node.part) || part.Subject != v.caller.Subject || part.Purpose != v.request.Purpose {
			return values.PublicInvalidContext
		}
		if _, duplicate := seen[part.Basis.Value]; duplicate {
			return values.PublicInvalidContext
		}
		if i == 0 {
			if node.parent != (values.PublicToken{}) || part.Basis.Owner != v.source || part.Hidden {
				return values.PublicInvalidContext
			}
		} else if seen[node.parent.Value] != node.parent || !node.parent.Valid() ||
			node.outgoing.Key.ParentAuthority != node.parent.Owner || node.outgoing.Key.CarrierAuthority != part.Basis.Owner ||
			!edgeMatches(node.outgoing, part.Incoming) {
			return values.PublicInvalidContext
		}
		seen[part.Basis.Value] = part.Basis
	}
	return nil
}
