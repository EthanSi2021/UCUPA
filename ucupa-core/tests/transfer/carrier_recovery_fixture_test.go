package transfer_test

import (
	"context"
	"path/filepath"
	"testing"

	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	"ucupa/ucupa-core/contract/transition"
	core "ucupa/ucupa-core/instance"
	local "ucupa/ucupa-core/transport/local/transition"
)

const carrierEntity values.ElementID = "00000000-0000-4000-8000-000000000040"

type carrierFixture struct {
	parent     *fixture
	source     *core.Core
	sourcePath string
	target     *targetFixture
	binding    contract.CarrierApprovalBinding
	begin      contract.CarrierStatus
}

func newCarrierRecovery(t *testing.T, empty bool) *carrierFixture {
	return newCarrierRecoveryWithParent(t, empty, "local:parent")
}

func newCarrierRecoveryWithParent(t *testing.T, empty bool, parentAddress values.NextHop) *carrierFixture {
	t.Helper()
	path := filepath.Join(t.TempDir(), "carrier.db")
	p, source, command := newCarrierFixtureWithParent(t, path, parentAddress)
	d := newTargetFixture(t, true) // Independent Realm and importer identity.
	command.Target = d.c.AuthorityRef()
	f := &carrierFixture{parent: p, source: source, sourcePath: path, target: d,
		binding: contract.CarrierApprovalBinding{Parent: p.c.AuthorityRef(), Operation: contract.OperationID(string([]byte{255, 0, 128, 'c'})), Command: command}}
	t.Cleanup(func() { _ = f.source.Close() })
	for _, permission := range []values.PermissionID{contract.PermissionTransitionCommit, contract.PermissionTransitionInspect} {
		p.grant(t, actor, permission, values.CoreElementScope(p.c.AuthorityRef(), carrierMount), false)
	}
	p.grant(t, actor, contract.PermissionTransitionExport, values.CoreElementScope(source.AuthorityRef(), carrierMount), false)
	p.grant(t, actor, contract.PermissionCreateEntity, values.CoreElementScope(source.AuthorityRef(), carrierMount), false)
	p.grant(t, actor, contract.PermissionReadElement, values.CoreElementScope(source.AuthorityRef(), carrierEntity), false)
	d.grant(t, contract.PermissionTransitionImport, carrierMount, false)
	d.grant(t, contract.PermissionReadElement, carrierEntity, false)
	d.grant(t, contract.PermissionMountRead, carrierMount, false)
	if !empty {
		if _, err := source.Elements().CreateEntity(actor, carrierEntity, tuple(t, carrierMount), "carrier-content"); err != nil {
			t.Fatal(err)
		}
	}
	var err error
	f.begin, err = p.c.Transfers().BeginCarrierTransition(context.Background(), p.caller, f.binding.Operation, command)
	if err != nil {
		t.Fatal(err)
	}
	return f
}

func (f *carrierFixture) connection(t *testing.T, sourceApproval, targetApproval bool) *local.CarrierConnection {
	t.Helper()
	var sv, tv values.AuthorizationView
	var err error
	ctx := context.Background()
	if sourceApproval {
		sv, err = f.source.Transfers().AuthorizeCarrierExport(ctx, f.parent.caller, f.binding)
		if err != nil {
			t.Fatal(err)
		}
	}
	if targetApproval {
		tv, err = f.target.c.Transfers().AuthorizeCarrierImport(ctx, f.target.caller, f.binding)
		if err != nil {
			t.Fatal(err)
		}
	}
	c, err := local.NewCarrier(f.parent.c.Transfers(), f.source.Transfers(), f.target.c.Transfers(), f.binding, f.parent.caller, sv, f.target.caller, tv)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func (f *carrierFixture) recover(t *testing.T, c transition.CarrierTransport) contract.CarrierStatus {
	t.Helper()
	r, err := f.parent.c.Transfers().RecoverCarrierTransition(context.Background(), f.parent.caller, f.binding, c)
	if err != nil || r.Progress != contract.CarrierSettled {
		t.Fatalf("carrier recover: %+v %v", r, err)
	}
	return r
}

func (f *carrierFixture) reopen(t *testing.T) {
	t.Helper()
	f.parent.reopen(t)
	f.target.reopen(t)
	if err := f.source.Close(); err != nil {
		t.Fatal(err)
	}
	var err error
	f.source, err = core.OpenDurable(boundaryContacts{f.parent.a}, f.sourcePath, f.parent.config)
	if err != nil {
		t.Fatal(err)
	}
}

// Faults interrupt actual public owner calls, not manufactured private proofs.
type carrierBoundary struct {
	transition.CarrierTransport
	afterCheck, afterFence, afterAccept, afterInstall, afterSettle func() error
}

func (c carrierBoundary) CheckCarrierImport(ctx context.Context, a values.NextHop, o values.FactAuthorityRef, b contract.CarrierApprovalBinding) error {
	err := c.CarrierTransport.CheckCarrierImport(ctx, a, o, b)
	if err == nil && c.afterCheck != nil {
		err = c.afterCheck()
	}
	return err
}
func (c carrierBoundary) FenceCarrierSource(ctx context.Context, a values.NextHop, o values.FactAuthorityRef, b contract.CarrierApprovalBinding) (transition.CarrierFrozen, error) {
	r, err := c.CarrierTransport.FenceCarrierSource(ctx, a, o, b)
	if err == nil && c.afterFence != nil {
		err = c.afterFence()
	}
	return r, err
}
func (c carrierBoundary) AcceptCarrierTarget(ctx context.Context, a values.NextHop, o values.FactAuthorityRef, b contract.CarrierApprovalBinding) (transition.CarrierAcceptance, error) {
	r, err := c.CarrierTransport.AcceptCarrierTarget(ctx, a, o, b)
	if err == nil && c.afterAccept != nil {
		err = c.afterAccept()
	}
	return r, err
}
func (c carrierBoundary) InstallCarrierTarget(ctx context.Context, a values.NextHop, o values.FactAuthorityRef, b contract.CarrierApprovalBinding) (transition.Completion, error) {
	r, err := c.CarrierTransport.InstallCarrierTarget(ctx, a, o, b)
	if err == nil && c.afterInstall != nil {
		err = c.afterInstall()
	}
	return r, err
}
func (c carrierBoundary) SettleCarrierSource(ctx context.Context, a values.NextHop, o values.FactAuthorityRef, b contract.CarrierApprovalBinding) error {
	err := c.CarrierTransport.SettleCarrierSource(ctx, a, o, b)
	if err == nil && c.afterSettle != nil {
		err = c.afterSettle()
	}
	return err
}
