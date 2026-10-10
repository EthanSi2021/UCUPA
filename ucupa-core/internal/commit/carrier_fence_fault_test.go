package commit

import (
	"errors"
	"fmt"
	"sync"
	"testing"
	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	elementfact "ucupa/ucupa-core/internal/element/fact"
	mountfact "ucupa/ucupa-core/internal/mount/fact"
	transferencoding "ucupa/ucupa-core/internal/transfer/encoding"
	transferfact "ucupa/ucupa-core/internal/transfer/fact"
)

func TestCarrierFenceStoreFaultAndConcurrentExactRetry(t *testing.T) {
	for _, after := range []bool{false, true} {
		t.Run(fmt.Sprint(after), func(t *testing.T) {
			_, s, c := carrierPermitFixture(t)
			before := s.state.clone()
			fault := func() error { return errors.New("carrier fence commit fault") }
			if after {
				s.store.failAfterCommit = fault
			} else {
				s.store.failBeforeCommit = fault
			}
			if _, _, err := fenceCarrier(s, c); !errors.Is(err, contract.ErrStore) || sameCompleteCoreState(before, s.state) == after {
				t.Fatal("wrong durable fault side", err)
			}
			path, durable := s.store.db.Path(), s.state.clone()
			closeFoundationStore(t, s)
			s = openFoundationStore(t, path)
			defer closeFoundationStore(t, s)
			if !sameCompleteCoreState(durable, s.state) {
				t.Fatal("reopen differs")
			}
			var wg sync.WaitGroup
			var results [8]transferfact.CarrierFence
			var bytes [8]string
			var errs [8]error
			for n := range results {
				wg.Add(1)
				go func(n int) { defer wg.Done(); results[n], bytes[n], errs[n] = fenceCarrier(s, c) }(n)
			}
			wg.Wait()
			for n := range results {
				if errs[n] != nil || results[n] != results[0] || bytes[n] != bytes[0] {
					t.Fatal("nonexact retry", n, errs[n])
				}
			}
			if len(s.state.transfers.events) != len(before.transfers.events)+1 || s.state.cutSerial != before.cutSerial+1 {
				t.Fatal("duplicate source fence")
			}
			if err := validateOperationHistory(s.state); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestCarrierFenceAndRefusalSerializeInOneSourceOwner(t *testing.T) {
	for n := 0; n < 8; n++ {
		_, s, c := carrierPermitFixture(t)
		before := s.state.clone()
		start := make(chan struct{})
		done := make(chan error, 2)
		go func() { <-start; _, _, err := fenceCarrier(s, c); done <- err }()
		go func() { <-start; _, err := refuseCarrier(s, c); done <- err }()
		close(start)
		a, b := <-done, <-done
		if (a == nil) == (b == nil) || a != nil && !errors.Is(a, values.PublicUnavailable) || b != nil && !errors.Is(b, values.PublicUnavailable) {
			t.Fatal("competing source dispositions", a, b)
		}
		r := s.state.transfers.carrierSources[c.Reservation.Input.Key()]
		if r.Refusal.Valid() == r.Fence.Valid() || len(s.state.transfers.events) != len(before.transfers.events)+1 {
			t.Fatal("source retained both or neither dispositions")
		}
		if r.Refusal.Valid() && (!s.state.elements.Equal(before.elements) || s.state.currentCut() != before.currentCut()) {
			t.Fatal("refusal changed source facts")
		}
		if err := validateOperationHistory(s.state); err != nil {
			t.Fatal(err)
		}
	}
}

func TestCarrierFinalCutContainsExactlyTheWritesThatWonTheFence(t *testing.T) {
	for n := 0; n < 8; n++ {
		_, s, c := carrierPermitFixture(t)
		id := foundationStoreID(111)
		final := foundationStoreTuple(t, c.Reservation.Input.Mount, 0, 1)
		start := make(chan struct{})
		done := make(chan error, 1)
		go func() {
			<-start
			_, err := applyFoundationStore(s, elementfact.OperationCreateEntity, foundationStoreActor, id, final, contract.Revision{}, "race-carrier-write")
			done <- err
		}()
		close(start)
		f, raw, err := fenceCarrier(s, c)
		if err != nil {
			t.Fatal(err)
		}
		writeErr := <-done
		if writeErr != nil && !errors.Is(writeErr, contract.ErrMountBusy) {
			t.Fatal(writeErr)
		}
		slice, err := transferencoding.DecodeCarrierManifest(f, raw)
		if err != nil {
			t.Fatal(err)
		}
		included := false
		for _, m := range slice.Members() {
			included = included || m.ID() == id
		}
		if included != (writeErr == nil) {
			t.Fatal("acknowledged write missing from final cut")
		}
		if err := validateOperationHistory(s.state); err != nil {
			t.Fatal(err)
		}
	}
}

func TestCarrierFenceReplayRejectsHistoryOrGateTampering(t *testing.T) {
	_, s, c := carrierPermitFixture(t)
	if _, _, err := fenceCarrier(s, c); err != nil {
		t.Fatal(err)
	}
	for _, change := range []string{"manifest", "permit", "gate", "refusal", "departure-controller"} {
		candidate := s.state.clone()
		e := &candidate.transfers.events[len(candidate.transfers.events)-1]
		switch change {
		case "manifest":
			e.Manifest = "{}"
		case "permit":
			e.CarrierPermit.Reservation.Previous.OperationID = "wrong-admission"
		case "gate":
			in := candidate.mount.facts.Incoming()
			in.Gate = mountfact.GateOpen
			candidate.mount.facts.ReplaceIncoming(in)
		case "refusal":
			e.Kind = transferencoding.TransferCarrierRefused
			e.Manifest = ""
		case "departure-controller":
			r := candidate.transfers.carrierSources[c.Reservation.Input.Key()]
			r.Fence.Permit.Reservation.Input.Parent = c.Reservation.Input.Target
			candidate.transfers.carrierSources[c.Reservation.Input.Key()] = r
		}
		if err := validateOperationHistory(candidate); err == nil {
			t.Fatal("tampered source replay accepted", change)
		}
	}
	bad := s.state.clone()
	in := bad.mount.facts.Incoming()
	in.Gate = mountfact.GateOpen
	bad.mount.facts.ReplaceIncoming(in)
	if validateCurrentSnapshot(bad) == nil {
		t.Fatal("current validator silently reopened fenced incoming")
	}
}
