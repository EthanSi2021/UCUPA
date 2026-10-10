package commit

import (
	"errors"

	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	commitbinding "ucupa/ucupa-core/internal/commit/binding"
	elementfact "ucupa/ucupa-core/internal/element/fact"
	coretoken "ucupa/ucupa-core/internal/token"
)

// A public retry asks for one admitted outcome, not the operation journal.
// Authorization and live handle checks stay at the caller's original points.
func (state *coreState) lookupPublicMutation(id contract.OperationID, binding commitbinding.Binding, recovering bool) (contract.MutationResult, error, bool) {
	retained, exists := state.operations[id]
	if !exists {
		return contract.MutationResult{}, nil, false
	}
	if retained.binding.Actor != binding.Actor {
		if recovering {
			return contract.MutationResult{}, values.PublicOutcomeUnknown, true
		}
		return contract.MutationResult{}, values.PublicConflict, true
	}
	if retained.binding.Boundary != binding.Boundary || retained.binding.PublicRequest != binding.PublicRequest {
		return contract.MutationResult{}, values.PublicConflict, true
	}
	result, err := retained.outcome.Outcome.Result(retained.outcome.cut)
	return result, err, true
}

func (state *coreState) publicExpectedRevision(element values.ElementID, token values.PublicToken) contract.Revision {
	if token == (values.PublicToken{}) {
		return contract.Revision{}
	}
	var current contract.Revision
	if anchor, ok := state.elements.Anchor(element); ok {
		current = anchor.Revision
	}
	if entity, ok := state.elements.Entity(element); ok {
		current = entity.Revision
	}
	if coretoken.ValidRevision(current) && coretoken.PublicElementRevision(state.publicKey, state.authority, current) == token {
		return current
	}
	// Record a deterministic foreign comparison value for failed public CAS,
	// without inventing an issued local Revision or reversing the public MAC.
	foreign := state.scope
	foreign[0] ^= 0xff
	if foreign == ([32]byte{}) {
		foreign[1] = 1
	}
	return coretoken.RevisionFromParts(foreign, element, 1)
}

func (state *coreState) publicBindingMembers(binding commitbinding.Binding) bool {
	if _, exists := state.elements.Anchor(binding.Boundary); !exists {
		return false
	}
	if binding.Kind >= elementfact.OperationUpdateAnchor && !elementfact.Member(state.elements.View, binding.Element, binding.Boundary) {
		return false
	}
	if binding.Final.Parent != "" && !elementfact.Member(state.elements.View, binding.Final.Parent, binding.Boundary) {
		return false
	}
	return true
}

func (state *coreState) validatePublicAdmission(binding commitbinding.Binding) error {
	if binding.Boundary == "" {
		if binding.PublicRequest != "" {
			return contract.ErrStore
		}
		return nil
	}
	parsed, err := values.ParseCanonicalJSON([]byte(binding.PublicRequest), values.PublicJSONLimits())
	if err != nil {
		return contract.ErrStore
	}
	request, ok := parsed.(map[string]any)
	if !ok {
		return contract.ErrStore
	}
	original, _, expected, err := commitbinding.ParsePublicMutation(request)
	if err != nil || expected != (values.PublicToken{}) && expected.Owner != state.authority {
		return contract.ErrStore
	}
	original.Actor, original.Boundary = binding.Actor, binding.Boundary
	original.Expected = state.publicExpectedRevision(original.Element, expected)
	if !commitbinding.EqualBindings(original, binding) || !state.publicBindingMembers(binding) {
		return errors.New("public operation does not match its admitted request/closure")
	}
	return nil
}

func (owner *Owner) LookupPublicMutationLocked(id contract.OperationID, binding commitbinding.Binding, recovering bool) (contract.MutationResult, error, bool) {
	return owner.state.lookupPublicMutation(id, binding, recovering)
}
func (owner *Owner) PublicBindingMembersLocked(binding commitbinding.Binding) bool {
	return owner.state.publicBindingMembers(binding)
}
func (owner *Owner) PublicExpectedRevisionLocked(element values.ElementID, expected values.PublicToken) contract.Revision {
	return owner.state.publicExpectedRevision(element, expected)
}
