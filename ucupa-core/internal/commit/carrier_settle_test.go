package commit

import (
	"errors"
	"fmt"
	"sync"
	"testing"

	"ucupa/ucupa-core/contract"
	mountfact "ucupa/ucupa-core/internal/mount/fact"
	"ucupa/ucupa-core/internal/token"
	transferfact "ucupa/ucupa-core/internal/transfer/fact"
)

func settleCarrier(o *Owner, i transferfact.CarrierInstallation, parent bool) (transferfact.CarrierSettlement, error) {
	o.Lock()
	defer o.Unlock()
	c := transferfact.CoreCompletion{Installed: transferfact.CoreInstallation{Carrier: i}, At: i.At, Revision: i.Revision}
	if parent {
		return o.SettleCarrierParentLocked(c)
	}
	return o.SettleCarrierSourceLocked(c)
}

func TestCarrierSettlementKeepsOriginalOutcomeAndReleasesOnlyParentSlot(t *testing.T) {
	for _, empty := range []bool{false, true} {
		t.Run(fmt.Sprint(empty), func(t *testing.T) {
			p, s, d, b := boundCarrierFixture(t, empty)
			i, err := installCarrier(d, b)
			if err != nil {
				t.Fatal(err)
			}
			input := b.Acceptance.Fence.Permit.Reservation.Input
			beforeParent, beforeSource, beforeTarget := p.state.clone(), s.state.clone(), d.state.clone()
			ps, err := settleCarrier(p, i, true)
			if err != nil || !ps.Valid(p.authority) {
				t.Fatal("parent", ps, err)
			}
			ss, err := settleCarrier(s, i, false)
			if err != nil || !ss.Valid(s.authority) {
				t.Fatal("source", ss, err)
			}
			old := beforeParent.mount.facts.Record(input.Mount)
			now := p.state.mount.facts.Record(input.Mount)
			if now.Slot != "" || now.Key != old.Key || now.Contact != old.Contact || now.Admission != old.Admission || now.Revision == old.Revision {
				t.Fatal("wrong parent settlement", old, now)
			}
			if beforeSource.mount.facts.Incoming() != s.state.mount.facts.Incoming() || s.state.mount.facts.Incoming().Gate != mountfact.GateFenced || !sameCompleteCoreState(beforeTarget, d.state) {
				t.Fatal("settlement changed source fence or target")
			}
			for n, o := range []*Owner{p, s} {
				want := []transferfact.CarrierSettlement{ps, ss}[n]
				before := o.state.clone()
				got, err := settleCarrier(o, i, n == 0)
				if err != nil || got != want || !sameCompleteCoreState(before, o.state) {
					t.Fatal("retry", got, err)
				}
				changed := i
				changed.At = token.FromParts(token.Scope(i.At), token.Serial(i.At)+1)
				if _, err := settleCarrier(o, changed, n == 0); !errors.Is(err, contract.ErrOperationConflict) || !sameCompleteCoreState(before, o.state) {
					t.Fatal("changed outcome", err)
				}
				path := o.store.db.Path()
				closeFoundationStore(t, o)
				reopened := openFoundationStore(t, path)
				got, err = settleCarrier(reopened, i, n == 0)
				if err != nil || got != want || !sameCompleteCoreState(before, reopened.state) {
					t.Fatal("reopen", err)
				}
				closeFoundationStore(t, reopened)
			}
		})
	}
}

func TestCarrierSettlementStoreFaultAndConcurrentRetry(t *testing.T) {
	for _, parent := range []bool{false, true} {
		for _, after := range []bool{false, true} {
			t.Run(fmt.Sprintf("parent=%v/after=%v", parent, after), func(t *testing.T) {
				p, s, d, b := boundCarrierFixture(t, false)
				i, err := installCarrier(d, b)
				if err != nil {
					t.Fatal(err)
				}
				o := s
				if parent {
					o = p
				}
				before := o.state.clone()
				fail := func() error { return errors.New("carrier settlement storage fault") }
				if after {
					o.store.failAfterCommit = fail
				} else {
					o.store.failBeforeCommit = fail
				}
				if _, err := settleCarrier(o, i, parent); !errors.Is(err, contract.ErrStore) || sameCompleteCoreState(before, o.state) == after {
					t.Fatal("commit side", err)
				}
				path, durable := o.store.db.Path(), o.state.clone()
				closeFoundationStore(t, o)
				o = openFoundationStore(t, path)
				defer closeFoundationStore(t, o)
				if !sameCompleteCoreState(durable, o.state) {
					t.Fatal("reopen changed result")
				}
				var wg sync.WaitGroup
				var results [8]transferfact.CarrierSettlement
				var errs [8]error
				for n := range errs {
					wg.Add(1)
					go func(n int) { defer wg.Done(); results[n], errs[n] = settleCarrier(o, i, parent) }(n)
				}
				wg.Wait()
				for n, err := range errs {
					if err != nil || results[n] != results[0] {
						t.Fatal("concurrent retry", err)
					}
				}
				if len(o.state.transfers.events) != len(before.transfers.events)+1 || o.state.cutSerial != before.cutSerial+1 {
					t.Fatal("duplicate settlement")
				}
				if err := validateCoreState(o.state); err != nil {
					t.Fatal(err)
				}
			})
		}
	}
}

func TestCarrierSettlementAllowsOrdinaryRangeControlWithoutRewritingReceipt(t *testing.T) {
	p, s, d, b := boundCarrierFixture(t, false)
	i, err := installCarrier(d, b)
	if err != nil {
		t.Fatal(err)
	}
	ps, err := settleCarrier(p, i, true)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := settleCarrier(s, i, false); err != nil {
		t.Fatal(err)
	}
	r := b.Acceptance.Fence.Permit.Reservation
	input := r.Input
	opID := contract.OperationID("range-after-carrier")
	command := contract.MountCommand{Kind: contract.MountUpdate, MountAnchorUUID: input.Mount, ExpectedElementRevision: input.ExpectedElement,
		ExpectedMountRevision: p.state.mount.facts.Record(input.Mount).Revision, Final: foundationStoreTuple(t, r.Tuple.Parent, 0, 10)}
	p.Lock()
	err = p.ApplyMountIntentLocked(contract.LocalCallers{d.authority: input.Actor}, opID, MountBinding{Actor: input.Actor, Command: command})
	p.Unlock()
	if err != nil {
		t.Fatal("range intent", err)
	}
	request := p.state.mount.operations[opID].Request
	d.Lock()
	prepared, err := d.ApplyMountPrepareLocked(p.authority, request, true)
	d.Unlock()
	if err != nil || !prepared.Prepared {
		t.Fatal("range prepare", prepared, err)
	}
	p.Lock()
	err = p.ApplyMountEventLocked(MountEvent{Kind: MountEventDecide, Operation: opID, Prepared: prepared, Authorized: true})
	p.Unlock()
	if err != nil {
		t.Fatal("range decision", err)
	}
	d.Lock()
	closed, err := d.ApplyMountFinishLocked(p.authority, contract.MountFinishRequest{Evidence: p.state.mount.operations[opID].Evidence()})
	d.Unlock()
	if err != nil {
		t.Fatal("range finish", err)
	}
	p.Lock()
	err = p.ApplyMountEventLocked(MountEvent{Kind: MountEventSettle, Operation: opID, Closed: closed})
	p.Unlock()
	if err != nil {
		t.Fatal("range settle", err)
	}
	if got, err := settleCarrier(p, i, true); err != nil || got != ps {
		t.Fatal("old receipt", err)
	}
	if p.state.mount.operations[input.Operation].CarrierSettlement != ps || d.state.mount.facts.Incoming().Admission.RangeValue != command.Final.Range {
		t.Fatal("lost range or terminal receipt")
	}
	for _, o := range []*Owner{p, d} {
		if err := validateCoreState(o.state); err != nil {
			t.Fatal(err)
		}
		path, before := o.store.db.Path(), o.state.clone()
		closeFoundationStore(t, o)
		opened := openFoundationStore(t, path)
		if !sameCompleteCoreState(before, opened.state) {
			t.Fatal("replay lost ordinary range")
		}
		closeFoundationStore(t, opened)
	}
	// The old carrier remains unavailable, even though the new relation now
	// has an ordinary range decision rather than its installation admission.
	if err := s.CheckTransferReadLocked(s.state.elements.View, s.state.currentCut(), input.Mount, ""); !errors.Is(err, contract.ErrUnavailable) {
		t.Fatal("old source reopened", err)
	}
}
