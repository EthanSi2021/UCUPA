package commit

import (
	"ucupa/ucupa-core/contract"
	commitbinding "ucupa/ucupa-core/internal/commit/binding"
	elementfact "ucupa/ucupa-core/internal/element/fact"
	mountfact "ucupa/ucupa-core/internal/mount/fact"
)

func decideMutation(state *coreState, binding commitbinding.Binding) (retainedOutcome, error) {
	outcome, err := decideSpatialMutation(state, binding)
	if err == nil {
		state.attachmentElementChange(binding, outcome)
	}
	return outcome, err
}

// Spatial decisions are shared by live commits and recovery. The aggregate
// retains revision/cut issuance, attachment effects and final publication.
func decideSpatialMutation(state *coreState, binding commitbinding.Binding) (retainedOutcome, error) {
	cut := state.currentCut()
	plan, err := state.elements.Plan(elementfact.Input{
		Kind: binding.Kind, Element: binding.Element, Final: binding.Final, Expected: binding.Expected,
	}, elementfact.ParentContext{
		External:                 state.elementExternalParent(),
		ParentHasMountedContent:  mountfact.HasContent(state.mount.facts.View, binding.Final.Parent),
		ElementHasMountedContent: mountfact.HasContent(state.mount.facts.View, binding.Element),
	})
	if err != nil {
		return retainedOutcome{}, err
	}
	outcome := plan.Outcome()
	if outcome.Status == contract.MutationCommitted {
		revision, nextCut, err := state.issueFactChange(binding.Element)
		if err != nil {
			return retainedOutcome{}, err
		}
		outcome, err = state.elements.Apply(plan, revision)
		if err != nil {
			return retainedOutcome{}, err
		}
		cut = nextCut
	}
	return retainedOutcome{Outcome: outcome, cut: cut}, nil
}
