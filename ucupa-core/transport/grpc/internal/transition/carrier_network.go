package transition

import (
	"context"
	"time"

	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	port "ucupa/ucupa-core/contract/transition"
)

// Carriers supplies the parent's one bounded orchestration transport for a
// single recovery invocation, bound to both retained peer approval tokens.
type Carriers interface {
	Carrier(contract.CarrierApprovalBinding, []byte, []byte) (port.CarrierTransport, error)
}

// CarrierPlanReader reads the parent's durable plan and decision over one
// authenticated fixed-peer connection.
type CarrierPlanReader interface {
	ReadCarrierPlan(context.Context, values.NextHop, values.FactAuthorityRef, contract.CarrierApprovalBinding) (port.CarrierPlan, error)
	ReadCarrierDecision(context.Context, values.NextHop, values.FactAuthorityRef, contract.CarrierApprovalBinding) (port.CarrierDecision, error)
}

// CarrierSourceReader pulls the fenced carrier content from the source.
type CarrierSourceReader interface {
	ReadCarrierSource(context.Context, values.NextHop, values.FactAuthorityRef, contract.CarrierApprovalBinding) (port.CarrierFrozen, error)
}

// CarrierTargetReader is the source host's bounded view of the target: the
// tokenless import preflight and the durable completion evidence.
type CarrierTargetReader interface {
	CheckCarrierImport(context.Context, values.NextHop, values.FactAuthorityRef, contract.CarrierApprovalBinding) error
	ReadCarrierCompletion(context.Context, values.NextHop, values.FactAuthorityRef, contract.CarrierApprovalBinding) (port.Completion, error)
}

// CarrierSourceAdmission resolves the retained original export approval for one
// fence dispatch. Expired approvals yield zero values: an existing durable
// fence may still be replayed, but no first fence can be authorized.
type CarrierSourceAdmission interface {
	ResolveCarrierExport(context.Context, contract.CarrierApprovalBinding, []byte) (values.CallerContext, values.AuthorizationView, time.Time, error)
}

// CarrierTargetAdmission serves the target's two authenticated import checks
// and the frozen acceptance entry. It owns no transfer state.
type CarrierTargetAdmission interface {
	CheckedCarrierImport(context.Context, contract.CarrierApprovalBinding, []byte) (values.CallerContext, error)
	CheckCarrierImportPreflight(context.Context, contract.CarrierApprovalBinding) error
	AcceptFrozenCarrier(context.Context, contract.CarrierApprovalBinding, []byte, port.CarrierPlan, port.CarrierFrozen) (port.CarrierAcceptance, error)
}

// carrierSourceTransport serves the source owner's fence and settle calls. It
// forwards exactly the parent plan read, the target preflight and the target
// completion read; every other port is out of role.
type carrierSourceTransport struct {
	binding contract.CarrierApprovalBinding
	parent  CarrierPlanReader
	targets CarrierTargetReader
}

func (t carrierSourceTransport) check(b contract.CarrierApprovalBinding, address values.NextHop, expected, want values.FactAuthorityRef, next values.NextHop) error {
	if b != t.binding || address != next || expected != want {
		return contract.ErrConflict
	}
	return nil
}

func (t carrierSourceTransport) ReadCarrierPlan(ctx context.Context, address values.NextHop, expected values.FactAuthorityRef, b contract.CarrierApprovalBinding) (port.CarrierPlan, error) {
	if err := t.check(b, address, expected, b.Parent, b.Command.ParentAddress); err != nil {
		return port.CarrierPlan{}, err
	}
	return t.parent.ReadCarrierPlan(ctx, address, expected, b)
}

func (t carrierSourceTransport) CheckCarrierImport(ctx context.Context, address values.NextHop, expected values.FactAuthorityRef, b contract.CarrierApprovalBinding) error {
	if err := t.check(b, address, expected, b.Command.Target, b.Command.TargetAddress); err != nil {
		return err
	}
	return t.targets.CheckCarrierImport(ctx, address, expected, b)
}

func (t carrierSourceTransport) ReadCarrierCompletion(ctx context.Context, address values.NextHop, expected values.FactAuthorityRef, b contract.CarrierApprovalBinding) (port.Completion, error) {
	if err := t.check(b, address, expected, b.Command.Target, b.Command.TargetAddress); err != nil {
		return port.Completion{}, err
	}
	return t.targets.ReadCarrierCompletion(ctx, address, expected, b)
}

func (t carrierSourceTransport) ReadCarrierDecision(context.Context, values.NextHop, values.FactAuthorityRef, contract.CarrierApprovalBinding) (port.CarrierDecision, error) {
	return port.CarrierDecision{}, values.PublicInvalid
}
func (t carrierSourceTransport) CheckCarrierExport(context.Context, values.NextHop, values.FactAuthorityRef, contract.CarrierApprovalBinding) error {
	return values.PublicInvalid
}
func (t carrierSourceTransport) FenceCarrierSource(context.Context, values.NextHop, values.FactAuthorityRef, contract.CarrierApprovalBinding) (port.CarrierFrozen, error) {
	return port.CarrierFrozen{}, values.PublicInvalid
}
func (t carrierSourceTransport) ReadCarrierSource(context.Context, values.NextHop, values.FactAuthorityRef, contract.CarrierApprovalBinding) (port.CarrierFrozen, error) {
	return port.CarrierFrozen{}, values.PublicInvalid
}
func (t carrierSourceTransport) AcceptCarrierTarget(context.Context, values.NextHop, values.FactAuthorityRef, contract.CarrierApprovalBinding) (port.CarrierAcceptance, error) {
	return port.CarrierAcceptance{}, values.PublicInvalid
}
func (t carrierSourceTransport) InstallCarrierTarget(context.Context, values.NextHop, values.FactAuthorityRef, contract.CarrierApprovalBinding) (port.Completion, error) {
	return port.Completion{}, values.PublicInvalid
}
func (t carrierSourceTransport) SettleCarrierSource(context.Context, values.NextHop, values.FactAuthorityRef, contract.CarrierApprovalBinding) error {
	return values.PublicInvalid
}

// carrierTargetTransport serves the target owner's install and completion
// calls. Acceptance uses the admission frozen wrapper instead, so no approval
// critical section waits on this transport.
type carrierTargetTransport struct {
	binding contract.CarrierApprovalBinding
	parent  CarrierPlanReader
	sources CarrierSourceReader
}

func (t carrierTargetTransport) ReadCarrierPlan(ctx context.Context, address values.NextHop, expected values.FactAuthorityRef, b contract.CarrierApprovalBinding) (port.CarrierPlan, error) {
	if b != t.binding || address != b.Command.ParentAddress || expected != b.Parent {
		return port.CarrierPlan{}, contract.ErrConflict
	}
	return t.parent.ReadCarrierPlan(ctx, address, expected, b)
}

func (t carrierTargetTransport) ReadCarrierDecision(ctx context.Context, address values.NextHop, expected values.FactAuthorityRef, b contract.CarrierApprovalBinding) (port.CarrierDecision, error) {
	if b != t.binding || address != b.Command.ParentAddress || expected != b.Parent {
		return port.CarrierDecision{}, contract.ErrConflict
	}
	return t.parent.ReadCarrierDecision(ctx, address, expected, b)
}

func (t carrierTargetTransport) ReadCarrierSource(ctx context.Context, address values.NextHop, expected values.FactAuthorityRef, b contract.CarrierApprovalBinding) (port.CarrierFrozen, error) {
	if b != t.binding || address != b.Command.SourceAddress || expected != b.Command.Source {
		return port.CarrierFrozen{}, contract.ErrConflict
	}
	return t.sources.ReadCarrierSource(ctx, address, expected, b)
}

func (t carrierTargetTransport) CheckCarrierExport(context.Context, values.NextHop, values.FactAuthorityRef, contract.CarrierApprovalBinding) error {
	return values.PublicInvalid
}
func (t carrierTargetTransport) CheckCarrierImport(context.Context, values.NextHop, values.FactAuthorityRef, contract.CarrierApprovalBinding) error {
	return values.PublicInvalid
}
func (t carrierTargetTransport) FenceCarrierSource(context.Context, values.NextHop, values.FactAuthorityRef, contract.CarrierApprovalBinding) (port.CarrierFrozen, error) {
	return port.CarrierFrozen{}, values.PublicInvalid
}
func (t carrierTargetTransport) AcceptCarrierTarget(context.Context, values.NextHop, values.FactAuthorityRef, contract.CarrierApprovalBinding) (port.CarrierAcceptance, error) {
	return port.CarrierAcceptance{}, values.PublicInvalid
}
func (t carrierTargetTransport) InstallCarrierTarget(context.Context, values.NextHop, values.FactAuthorityRef, contract.CarrierApprovalBinding) (port.Completion, error) {
	return port.Completion{}, values.PublicInvalid
}
func (t carrierTargetTransport) ReadCarrierCompletion(context.Context, values.NextHop, values.FactAuthorityRef, contract.CarrierApprovalBinding) (port.Completion, error) {
	return port.Completion{}, values.PublicInvalid
}
func (t carrierTargetTransport) SettleCarrierSource(context.Context, values.NextHop, values.FactAuthorityRef, contract.CarrierApprovalBinding) error {
	return values.PublicInvalid
}
