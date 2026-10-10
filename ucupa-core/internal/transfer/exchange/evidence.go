// Package exchange contains opaque adapter-to-owner transfer evidence. These are
// not wire messages or permits: operations obtain them from designated peers.
package exchange

import (
	"ucupa/ucupa-core/contract"
	"ucupa/ucupa-core/internal/transfer/fact"
)

type Frozen struct {
	binding  contract.TransitionApprovalBinding
	fence    fact.SourceFence
	manifest string
}
type Acceptance struct{ receipt fact.TargetAcceptance }
type Decision struct {
	binding  contract.TransitionApprovalBinding
	decision fact.SourceDecision
}
type Completion struct{ result fact.CoreCompletion }

func Freeze(b contract.TransitionApprovalBinding, f fact.SourceFence, manifest string) Frozen {
	return Frozen{binding: b, fence: f, manifest: manifest}
}
func OpenFrozen(v Frozen) (contract.TransitionApprovalBinding, fact.SourceFence, string) {
	return v.binding, v.fence, v.manifest
}
func Accept(v fact.TargetAcceptance) Acceptance         { return Acceptance{receipt: v} }
func OpenAcceptance(v Acceptance) fact.TargetAcceptance { return v.receipt }
func Decide(b contract.TransitionApprovalBinding, d fact.SourceDecision) Decision {
	return Decision{binding: b, decision: d}
}
func OpenDecision(v Decision) (contract.TransitionApprovalBinding, fact.SourceDecision) {
	return v.binding, v.decision
}
func Complete(v fact.CoreCompletion) Completion       { return Completion{result: v} }
func OpenCompletion(v Completion) fact.CoreCompletion { return v.result }
