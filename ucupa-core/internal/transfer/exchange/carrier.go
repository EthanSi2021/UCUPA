package exchange

import (
	"ucupa/ucupa-core/contract"
	"ucupa/ucupa-core/internal/transfer/fact"
)

// These finite opaque values are produced only by designated peer operations.
// They are not user-authored proofs or independent transfer state.
type CarrierPlan struct {
	binding contract.CarrierApprovalBinding
	control fact.CarrierControl
}
type CarrierFrozen struct {
	binding  contract.CarrierApprovalBinding
	fence    fact.CarrierFence
	manifest string
}
type CarrierAcceptance struct{ receipt fact.CarrierAcceptance }
type CarrierDecision struct {
	binding  contract.CarrierApprovalBinding
	decision fact.CarrierBinding
}

func PlanCarrier(b contract.CarrierApprovalBinding, c fact.CarrierControl) CarrierPlan {
	return CarrierPlan{b, c}
}
func OpenCarrierPlan(p CarrierPlan) (contract.CarrierApprovalBinding, fact.CarrierControl) {
	return p.binding, p.control
}
func FreezeCarrier(b contract.CarrierApprovalBinding, f fact.CarrierFence, raw string) CarrierFrozen {
	return CarrierFrozen{b, f, raw}
}
func OpenCarrierFrozen(f CarrierFrozen) (contract.CarrierApprovalBinding, fact.CarrierFence, string) {
	return f.binding, f.fence, f.manifest
}
func AcceptCarrier(a fact.CarrierAcceptance) CarrierAcceptance         { return CarrierAcceptance{a} }
func OpenCarrierAcceptance(a CarrierAcceptance) fact.CarrierAcceptance { return a.receipt }
func DecideCarrier(b contract.CarrierApprovalBinding, d fact.CarrierBinding) CarrierDecision {
	return CarrierDecision{b, d}
}
func OpenCarrierDecision(d CarrierDecision) (contract.CarrierApprovalBinding, fact.CarrierBinding) {
	return d.binding, d.decision
}
