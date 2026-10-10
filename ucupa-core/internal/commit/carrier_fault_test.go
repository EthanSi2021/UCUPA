package commit

import (
	"errors"
	"fmt"
	"sync"
	"testing"

	"ucupa/ucupa-core/contract"
	"ucupa/ucupa-core/internal/token"
	transferfact "ucupa/ucupa-core/internal/transfer/fact"
)

func TestCarrierPreparationFaultsAndExactConcurrentRetries(t *testing.T) {
	for _, stage := range []string{"reserve", "permit", "cancel"} {
		for _, after := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/committed-%t", stage, after), func(t *testing.T) {
				parent, source, target, i := carrierControlFixture(t)
				var initial transferfact.CarrierControl
				if stage != "reserve" {
					var err error
					initial, err = reserveCarrier(t, parent, i)
					if err != nil {
						t.Fatal(err)
					}
				}
				run := func() (transferfact.CarrierControl, error) {
					if stage == "reserve" {
						return reserveCarrier(t, parent, i)
					}
					return advanceCarrier(t, parent, initial, stage == "permit")
				}
				before := parent.state.clone()
				fail := func() error { return errors.New("carrier durable fault") }
				if after {
					parent.store.failAfterCommit = fail
				} else {
					parent.store.failBeforeCommit = fail
				}
				if _, err := run(); !errors.Is(err, contract.ErrStore) {
					t.Fatal("fault not exercised", err)
				}
				if sameCompleteCoreState(before, parent.state) == after {
					t.Fatal("wrong durable side")
				}
				for _, ptr := range []**Owner{&parent, &source, &target} {
					o := *ptr
					path, durable := o.store.db.Path(), o.state.clone()
					closeFoundationStore(t, o)
					*ptr = openFoundationStore(t, path)
					t.Cleanup(func() { _ = (*ptr).CloseStore() })
					if !sameCompleteCoreState(durable, (*ptr).state) {
						t.Fatal("reopen invented progress")
					}
				}
				cut := token.Serial(parent.state.currentCut())
				var results [8]transferfact.CarrierControl
				var errs [8]error
				var wg sync.WaitGroup
				for n := range results {
					wg.Add(1)
					go func(n int) { defer wg.Done(); results[n], errs[n] = run() }(n)
				}
				wg.Wait()
				for n := range results {
					if errs[n] != nil || results[n] != results[0] || !results[n].Valid() {
						t.Fatal("retry mismatch", n, errs[n])
					}
				}
				want := cut
				if !after {
					want++
				}
				if token.Serial(parent.state.currentCut()) != want {
					t.Fatal("retry repeated durable transition")
				}
			})
		}
	}
}

func TestCarrierPermitAndCancelRaceHasOneDurableWinner(t *testing.T) {
	for iteration := 0; iteration < 16; iteration++ {
		parent, _, _, i := carrierControlFixture(t)
		c, err := reserveCarrier(t, parent, i)
		if err != nil {
			t.Fatal(err)
		}
		var results [2]transferfact.CarrierControl
		var errs [2]error
		var wg sync.WaitGroup
		for n := range errs {
			wg.Add(1)
			go func(n int) { defer wg.Done(); results[n], errs[n] = advanceCarrier(t, parent, c, n == 0) }(n)
		}
		wg.Wait()
		if (errs[0] == nil) == (errs[1] == nil) {
			t.Fatal("permit and cancel did not serialize", errs)
		}
		winner := results[0]
		if errs[0] != nil {
			winner = results[1]
		}
		if !winner.Valid() || token.Serial(parent.state.currentCut()) != token.Serial(c.Reservation.At)+1 {
			t.Fatal("multiple transitions")
		}
		path, before := parent.store.db.Path(), parent.state.clone()
		closeFoundationStore(t, parent)
		o := openFoundationStore(t, path)
		if !sameCompleteCoreState(before, o.state) {
			t.Fatal("winner lost on restart")
		}
		closeFoundationStore(t, o)
	}
}
