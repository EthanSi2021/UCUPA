package transfer_test

import (
	"context"
	"errors"
	"sync"
	"testing"

	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	"ucupa/ucupa-core/contract/transition"
)

type recoveryBoundary struct {
	transition.TargetTransport
	afterCheck   func() error
	afterAccept  func() error
	afterInstall func() error
}

func (c recoveryBoundary) CheckImport(ctx context.Context, address values.NextHop, owner values.FactAuthorityRef, b contract.TransitionApprovalBinding) error {
	err := c.TargetTransport.CheckImport(ctx, address, owner, b)
	if err == nil && c.afterCheck != nil {
		err = c.afterCheck()
	}
	return err
}
func (c recoveryBoundary) AcceptImport(ctx context.Context, address values.NextHop, owner values.FactAuthorityRef, b contract.TransitionApprovalBinding) (transition.Acceptance, error) {
	a, err := c.TargetTransport.AcceptImport(ctx, address, owner, b)
	if err == nil && c.afterAccept != nil {
		err = c.afterAccept()
	}
	return a, err
}
func (c recoveryBoundary) Install(ctx context.Context, address values.NextHop, owner values.FactAuthorityRef, b contract.TransitionApprovalBinding) (transition.Completion, error) {
	r, err := c.TargetTransport.Install(ctx, address, owner, b)
	if err == nil && c.afterInstall != nil {
		err = c.afterInstall()
	}
	return r, err
}

func recoverPair(t *testing.T, s *fixture, b contract.TransitionApprovalBinding, link transition.TargetTransport) contract.TransitionStatus {
	t.Helper()
	r, err := s.c.Transfers().RecoverTransition(context.Background(), s.caller, b, link)
	if err != nil || r.Progress != contract.TransitionSettled {
		t.Fatalf("recover = %+v, %v", r, err)
	}
	return r
}

func TestPublicTransferEntityAndRootCompleteAcrossDurableOwners(t *testing.T) {
	for _, rootTransfer := range []bool{false, true} {
		t.Run(map[bool]string{false: "entity", true: "root"}[rootTransfer], func(t *testing.T) {
			s, target := newFixture(t), newTargetFixture(t, rootTransfer)
			b := beginPair(t, s, target, rootTransfer)
			link := pairConnection(t, s, target, b, true)
			done := recoverPair(t, s, b, link)
			if done.Target != target.c.AuthorityRef() {
				t.Fatal("wrong successor")
			}
			target.grant(t, contract.PermissionReadElement, entity, false)
			read, err := target.c.Elements().ReadElement(target.caller.Subject, entity, nil)
			if err != nil || read.Record.Entity.ID != entity {
				t.Fatalf("target content: %+v %v", read, err)
			}
			wantParent := targetRoot
			if rootTransfer {
				wantParent = root
			}
			if read.Record.Entity.ContainingAnchorID != wantParent {
				t.Fatalf("wrong placement: %+v", read.Record.Entity)
			}
			if _, err := s.c.Elements().ReadElement(actor, entity, nil); err == nil {
				t.Fatal("departed source remains readable")
			}
			revision := read.Record.Entity.Revision
			s.reopen(t)
			target.reopen(t)
			link = pairConnection(t, s, target, b, false)
			if got := recoverPair(t, s, b, link); got != done {
				t.Fatal("restart changed original result")
			}
			read, err = target.c.Elements().ReadElement(target.caller.Subject, entity, nil)
			if err != nil || !read.Record.Entity.Revision.Equal(revision) {
				t.Fatalf("retry reinstalled target: %v", err)
			}
			if got, err := s.c.Transfers().BeginTransition(context.Background(), s.caller, b.Operation, b.Command); err != nil || got != done {
				t.Fatalf("original input retry: %+v %v", got, err)
			}
		})
	}
}

func TestPublicTransferRecoversAcceptedPromiseWithoutNewImportGrant(t *testing.T) {
	s, target := newFixture(t), newTargetFixture(t, false)
	b := beginPair(t, s, target, false)
	link := pairConnection(t, s, target, b, true)
	lost := recoveryBoundary{TargetTransport: link, afterAccept: func() error { return values.PublicUnavailable }}
	if _, err := s.c.Transfers().RecoverTransition(context.Background(), s.caller, b, lost); !errors.Is(err, values.PublicUnavailable) {
		t.Fatalf("lost acceptance: %v", err)
	}
	status, err := s.c.Transfers().InspectTransition(context.Background(), s.caller, b.Operation, entity)
	if err != nil || status.Progress != contract.TransitionFenced {
		t.Fatalf("not fenced: %+v %v", status, err)
	}
	target.grant(t, contract.PermissionTransitionImport, entity, true)
	target.grant(t, contract.PermissionPlaceElement, targetRoot, true)
	s.reopen(t)
	target.reopen(t)
	link = pairConnection(t, s, target, b, false)
	recoverPair(t, s, b, link)
}

func TestPublicTransferRecoversInstalledPromiseWithoutNewMutationGrants(t *testing.T) {
	s, target := newFixture(t), newTargetFixture(t, false)
	b := beginPair(t, s, target, false)
	link := pairConnection(t, s, target, b, true)
	lost := recoveryBoundary{TargetTransport: link, afterInstall: func() error { return values.PublicUnavailable }}
	if _, err := s.c.Transfers().RecoverTransition(context.Background(), s.caller, b, lost); !errors.Is(err, values.PublicUnavailable) {
		t.Fatalf("lost installation: %v", err)
	}
	status, err := s.c.Transfers().InspectTransition(context.Background(), s.caller, b.Operation, entity)
	if err != nil || status.Progress != contract.TransitionCommitted {
		t.Fatalf("not committed: %+v %v", status, err)
	}
	for _, p := range []values.PermissionID{contract.PermissionTransitionExport, contract.PermissionTransitionCommit} {
		s.grant(t, actor, p, values.CoreElementScope(b.Source, entity), true)
	}
	target.grant(t, contract.PermissionTransitionImport, entity, true)
	s.reopen(t)
	target.reopen(t)
	recoverPair(t, s, b, pairConnection(t, s, target, b, false))
}

func TestPublicTransferConcurrentExactRecovery(t *testing.T) {
	s, target := newFixture(t), newTargetFixture(t, false)
	b := beginPair(t, s, target, false)
	link := pairConnection(t, s, target, b, true)
	var wg sync.WaitGroup
	for n := 0; n < 8; n++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := s.c.Transfers().RecoverTransition(context.Background(), s.caller, b, link); err != nil {
				t.Errorf("concurrent recovery: %v", err)
			}
		}()
	}
	wg.Wait()
	recoverPair(t, s, b, link)
}
