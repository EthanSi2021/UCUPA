package commit

import (
	"errors"
	"fmt"
	"sync"
	"testing"

	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	transferfact "ucupa/ucupa-core/internal/transfer/fact"
)

func beginPublicCarrier(o *Owner, i transferfact.CarrierIntent, command contract.CarrierCommand) (contract.CarrierStatus, error) {
	o.Lock()
	defer o.Unlock()
	return o.BeginPublicCarrierLocked(i.Actor, i.Operation, command)
}

func inspectPublicCarrier(o *Owner, i transferfact.CarrierIntent) (contract.CarrierStatus, error) {
	o.Lock()
	defer o.Unlock()
	return o.InspectPublicCarrierLocked(i.Actor, i.Operation, i.Mount)
}

func cancelPublicCarrier(o *Owner, i transferfact.CarrierIntent, expected values.PublicToken) (contract.CarrierStatus, error) {
	o.Lock()
	defer o.Unlock()
	return o.CancelPublicCarrierLocked(i.Actor, i.Operation, i.Mount, expected)
}

func TestPublicCarrierOriginalReservationCancellationAndReopen(t *testing.T) {
	p, source, target, i := carrierControlFixture(t)
	command := p.publicCarrierCommand(i)
	beforeSource, beforeTarget := source.state.clone(), target.state.clone()
	begin, err := beginPublicCarrier(p, i, command)
	if err != nil || begin.Parent != p.authority || begin.Source != source.authority || begin.Target != target.authority || begin.Progress != contract.CarrierReserved || !begin.Revision.Valid() {
		t.Fatal(begin, err)
	}
	if !sameCompleteCoreState(beforeSource, source.state) || !sameCompleteCoreState(beforeTarget, target.state) {
		t.Fatal("parent reservation modified a peer")
	}
	bad := begin.Revision
	bad.Value = values.EncodePublicBytes(make([]byte, 32))
	if _, err := cancelPublicCarrier(p, i, bad); err == nil {
		t.Fatal("forged comparison cancelled")
	}
	cancelled, err := cancelPublicCarrier(p, i, begin.Revision)
	if err != nil || cancelled.Progress != contract.CarrierCancelled || cancelled.Revision == begin.Revision {
		t.Fatal(cancelled, err)
	}
	before := p.state.clone()
	path := p.store.db.Path()
	closeFoundationStore(t, p)
	p = openFoundationStore(t, path)
	defer closeFoundationStore(t, p)
	if got, err := beginPublicCarrier(p, i, command); err != nil || got != begin {
		t.Fatal("Begin lost original reservation", got, err)
	}
	if got, err := cancelPublicCarrier(p, i, begin.Revision); err != nil || got != cancelled {
		t.Fatal("Cancel lost original result", got, err)
	}
	if got, err := inspectPublicCarrier(p, i); err != nil || got != cancelled {
		t.Fatal("Inspect lost latest state", got, err)
	}
	if !sameCompleteCoreState(before, p.state) {
		t.Fatal("retries wrote new state")
	}
	for _, mutate := range []func(*contract.CarrierCommand){
		func(c *contract.CarrierCommand) { c.Source, c.Target = c.Target, c.Source },
		func(c *contract.CarrierCommand) { c.ParentAddress = "local:other-parent" },
		func(c *contract.CarrierCommand) { c.SourceAddress = "local:other-source" },
		func(c *contract.CarrierCommand) { c.TargetAddress = "local:other-target" },
		func(c *contract.CarrierCommand) {
			c.ExpectedMount = p.SigningLocked().MountRevision(p.state.mount.facts.Record(i.Mount).Revision)
		},
	} {
		changed := command
		mutate(&changed)
		if _, err := beginPublicCarrier(p, i, changed); !errors.Is(err, contract.ErrOperationConflict) {
			t.Fatal("changed original input accepted", err)
		}
	}
	other := i
	other.Actor.Principal = "other"
	if _, err := beginPublicCarrier(p, other, command); !errors.Is(err, contract.ErrOperationConflict) {
		t.Fatal("changed actor accepted", err)
	}
	if _, err := inspectPublicCarrier(p, other); !errors.Is(err, values.PublicDenied) {
		t.Fatal("other actor saw retained control", err)
	}
	other = i
	other.Mount = foundationStoreID(100)
	if _, err := inspectPublicCarrier(p, other); !errors.Is(err, values.PublicDenied) {
		t.Fatal("wrong mount scope saw control", err)
	}
}

func TestPublicCarrierCannotCancelAfterPermitOrBinding(t *testing.T) {
	p, _, _, i := carrierControlFixture(t)
	begin, err := beginPublicCarrier(p, i, p.publicCarrierCommand(i))
	if err != nil {
		t.Fatal(err)
	}
	c := p.state.mount.operations[i.Operation].Carrier
	if _, err := advanceCarrier(t, p, c, true); err != nil {
		t.Fatal(err)
	}
	permitted, err := inspectPublicCarrier(p, i)
	if err != nil || permitted.Progress != contract.CarrierPermitted || permitted.Revision == begin.Revision {
		t.Fatal(permitted, err)
	}
	before := p.state.clone()
	if _, err := cancelPublicCarrier(p, i, begin.Revision); !errors.Is(err, values.PublicUnavailable) || !sameCompleteCoreState(before, p.state) {
		t.Fatal("cancel inferred a source refusal", err)
	}
	p, _, target, binding := boundCarrierFixture(t, false)
	i = binding.Acceptance.Fence.Permit.Reservation.Input
	committed, err := inspectPublicCarrier(p, i)
	if err != nil || committed.Progress != contract.CarrierCommitted {
		t.Fatal(committed, err)
	}
	original, err := beginPublicCarrier(p, i, p.publicCarrierCommand(i))
	if err != nil || original.Progress != contract.CarrierReserved {
		t.Fatal("committed Begin is not original", original, err)
	}
	if _, err := cancelPublicCarrier(p, i, original.Revision); !errors.Is(err, values.PublicUnavailable) {
		t.Fatal("cancelled committed binding", err)
	}
	installed, err := installCarrier(target, binding)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := settleCarrier(p, installed, true); err != nil {
		t.Fatal(err)
	}
	settled, err := inspectPublicCarrier(p, i)
	if err != nil || settled.Progress != contract.CarrierSettled || settled.Revision == committed.Revision {
		t.Fatal(settled, err)
	}
}

func TestPublicCarrierFaultBoundariesKeepExactComparisons(t *testing.T) {
	for _, cancel := range []bool{false, true} {
		for _, after := range []bool{false, true} {
			t.Run(fmt.Sprintf("cancel=%v/after=%v", cancel, after), func(t *testing.T) {
				p, _, _, i := carrierControlFixture(t)
				command := p.publicCarrierCommand(i)
				var expected values.PublicToken
				if cancel {
					begin, err := beginPublicCarrier(p, i, command)
					if err != nil {
						t.Fatal(err)
					}
					expected = begin.Revision
				}
				run := func() (contract.CarrierStatus, error) {
					if cancel {
						return cancelPublicCarrier(p, i, expected)
					}
					return beginPublicCarrier(p, i, command)
				}
				fail := func() error { return errors.New("public carrier store failure") }
				if after {
					p.store.failAfterCommit = fail
				} else {
					p.store.failBeforeCommit = fail
				}
				if _, err := run(); !errors.Is(err, contract.ErrStore) {
					t.Fatal("missing durable fault", err)
				}
				path := p.store.db.Path()
				closeFoundationStore(t, p)
				p = openFoundationStore(t, path)
				defer closeFoundationStore(t, p)
				var wg sync.WaitGroup
				var results [8]contract.CarrierStatus
				var errs [8]error
				for n := range results {
					wg.Add(1)
					go func(n int) { defer wg.Done(); results[n], errs[n] = run() }(n)
				}
				wg.Wait()
				for n := range results {
					if errs[n] != nil || results[n] != results[0] || !results[n].Revision.Valid() {
						t.Fatal("concurrent retry changed public result", results[n], errs[n])
					}
				}
			})
		}
	}
}

func TestPublicCarrierRejectsBadCASAndOrdinaryOperationReuseBeforeReservation(t *testing.T) {
	p, _, _, i := carrierControlFixture(t)
	command := p.publicCarrierCommand(i)
	before := p.state.clone()
	for _, mutate := range []func(*contract.CarrierCommand){
		func(c *contract.CarrierCommand) { c.ExpectedElement.Value = values.EncodePublicBytes(make([]byte, 32)) },
		func(c *contract.CarrierCommand) { c.ExpectedMount.Value = values.EncodePublicBytes(make([]byte, 32)) },
		func(c *contract.CarrierCommand) { c.ExpectedMount.Owner = c.Source },
		func(c *contract.CarrierCommand) { c.ExpectedElement = c.ExpectedMount },
		func(c *contract.CarrierCommand) { c.SourceAddress = "local:wrong-source" },
		func(c *contract.CarrierCommand) { c.Target = p.authority },
		func(c *contract.CarrierCommand) { c.Source = c.Target },
		func(c *contract.CarrierCommand) { c.ParentAddress = "" },
	} {
		changed := command
		mutate(&changed)
		if _, err := beginPublicCarrier(p, i, changed); err == nil || !sameCompleteCoreState(before, p.state) {
			t.Fatal("invalid comparison or relation reserved a slot", changed, err)
		}
	}
	for _, id := range []contract.OperationID{"carrier-original", "carrier-root"} {
		other := i
		other.Operation = id
		if _, err := beginPublicCarrier(p, other, command); !errors.Is(err, contract.ErrOperationConflict) || !sameCompleteCoreState(before, p.state) {
			t.Fatal("reused an ordinary operation", id, err)
		}
	}
	i.Operation = contract.OperationID("\xff\x00carrier")
	begin, err := beginPublicCarrier(p, i, command)
	if err != nil || begin.Operation != i.Operation {
		t.Fatal("raw operation identity was normalized", begin, err)
	}
	other := i
	other.Operation = "competing-replacement"
	if _, err := beginPublicCarrier(p, other, command); !errors.Is(err, contract.ErrRevisionConflict) {
		t.Fatal("old mount revision could reserve another operation", err)
	}
	command.ExpectedMount = p.SigningLocked().MountRevision(p.state.mount.facts.Record(i.Mount).Revision)
	if _, err := beginPublicCarrier(p, other, command); !errors.Is(err, contract.ErrMountBusy) {
		t.Fatal("fresh comparison bypassed occupied mount slot", err)
	}
}
