package transition

import (
	"context"
	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	"ucupa/ucupa-core/internal/transfer/exchange"
)

type CarrierPlan = exchange.CarrierPlan
type CarrierFrozen = exchange.CarrierFrozen
type CarrierAcceptance = exchange.CarrierAcceptance
type CarrierDecision = exchange.CarrierDecision

// CarrierTransport is consumed by the three actual owners. Implementations
// bind source and target approvals separately, and never refresh them implicitly.
type CarrierTransport interface {
	ReadCarrierPlan(context.Context, values.NextHop, values.FactAuthorityRef, contract.CarrierApprovalBinding) (CarrierPlan, error)
	ReadCarrierDecision(context.Context, values.NextHop, values.FactAuthorityRef, contract.CarrierApprovalBinding) (CarrierDecision, error)
	CheckCarrierExport(context.Context, values.NextHop, values.FactAuthorityRef, contract.CarrierApprovalBinding) error
	CheckCarrierImport(context.Context, values.NextHop, values.FactAuthorityRef, contract.CarrierApprovalBinding) error
	FenceCarrierSource(context.Context, values.NextHop, values.FactAuthorityRef, contract.CarrierApprovalBinding) (CarrierFrozen, error)
	ReadCarrierSource(context.Context, values.NextHop, values.FactAuthorityRef, contract.CarrierApprovalBinding) (CarrierFrozen, error)
	AcceptCarrierTarget(context.Context, values.NextHop, values.FactAuthorityRef, contract.CarrierApprovalBinding) (CarrierAcceptance, error)
	InstallCarrierTarget(context.Context, values.NextHop, values.FactAuthorityRef, contract.CarrierApprovalBinding) (Completion, error)
	ReadCarrierCompletion(context.Context, values.NextHop, values.FactAuthorityRef, contract.CarrierApprovalBinding) (Completion, error)
	SettleCarrierSource(context.Context, values.NextHop, values.FactAuthorityRef, contract.CarrierApprovalBinding) error
}

type CarrierParentPeer interface {
	AuthorityRef() values.FactAuthorityRef
	ReadCarrierPlan(context.Context, values.FactAuthorityRef, contract.CarrierApprovalBinding) (CarrierPlan, error)
	ReadCarrierDecision(context.Context, values.FactAuthorityRef, contract.CarrierApprovalBinding) (CarrierDecision, error)
}
type CarrierSourcePeer interface {
	AuthorityRef() values.FactAuthorityRef
	CheckCarrierExport(context.Context, values.CallerContext, contract.CarrierApprovalBinding, values.AuthorizationView) error
	FenceCarrierSource(context.Context, values.FactAuthorityRef, values.CallerContext, contract.CarrierApprovalBinding, values.AuthorizationView, CarrierTransport) (CarrierFrozen, error)
	ReadCarrierSource(context.Context, values.FactAuthorityRef, contract.CarrierApprovalBinding, CarrierTransport) (CarrierFrozen, error)
	SettleCarrierSource(context.Context, values.FactAuthorityRef, contract.CarrierApprovalBinding, CarrierTransport) error
}
type CarrierTargetPeer interface {
	AuthorityRef() values.FactAuthorityRef
	CheckCarrierImport(context.Context, values.CallerContext, contract.CarrierApprovalBinding, values.AuthorizationView) error
	AcceptCarrierTarget(context.Context, values.FactAuthorityRef, values.CallerContext, contract.CarrierApprovalBinding, values.AuthorizationView, CarrierTransport) (CarrierAcceptance, error)
	InstallCarrierTarget(context.Context, values.FactAuthorityRef, contract.CarrierApprovalBinding, CarrierTransport) (Completion, error)
	ReadCarrierCompletion(context.Context, values.FactAuthorityRef, contract.CarrierApprovalBinding, CarrierTransport) (Completion, error)
}
