package commit

import (
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"testing"
	"ucupa/ucupa-core/contract"
	transferencoding "ucupa/ucupa-core/internal/transfer/encoding"
	transferfact "ucupa/ucupa-core/internal/transfer/fact"
)

func TestCarrierRefusalAndAbortFaultReplayAndConcurrentRetry(t *testing.T) {
	for _, abort := range []bool{false, true} {
		for _, after := range []bool{false, true} {
			t.Run(fmt.Sprintf("abort-%t/after-commit-%t", abort, after), func(t *testing.T) {
				p, s, c := carrierPermitFixture(t)
				var r transferfact.CarrierRefusal
				if abort {
					var err error
					r, err = refuseCarrier(s, c)
					if err != nil {
						t.Fatal(err)
					}
				}
				owner := s
				if abort {
					owner = p
				}
				run := func() error {
					if abort {
						_, err := cancelPermittedCarrier(p, c, r)
						return err
					}
					_, err := refuseCarrier(s, c)
					return err
				}
				before := owner.state.clone()
				fault := func() error { return errors.New("refusal/abort commit fault") }
				if after {
					owner.store.failAfterCommit = fault
				} else {
					owner.store.failBeforeCommit = fault
				}
				if err := run(); !errors.Is(err, contract.ErrStore) || sameCompleteCoreState(before, owner.state) == after {
					t.Fatal("incorrect commit fault side", err)
				}
				for _, ptr := range []**Owner{&p, &s} {
					o := *ptr
					path, durable := o.store.db.Path(), o.state.clone()
					closeFoundationStore(t, o)
					*ptr = openFoundationStore(t, path)
					if !sameCompleteCoreState(durable, (*ptr).state) {
						t.Fatal("restart changed durable result")
					}
				}
				defer closeFoundationStore(t, p)
				defer closeFoundationStore(t, s)
				owner = s
				if abort {
					owner = p
				}
				count := len(owner.state.transfers.events)
				var wg sync.WaitGroup
				var errs [8]error
				for n := range errs {
					wg.Add(1)
					go func(n int) { defer wg.Done(); errs[n] = run() }(n)
				}
				wg.Wait()
				for _, err := range errs {
					if err != nil {
						t.Fatal("exact retry failed", err)
					}
				}
				if !after {
					count++
				}
				if len(owner.state.transfers.events) != count {
					t.Fatal("concurrent retry added multiple outcomes")
				}
			})
		}
	}
}

func TestCarrierRefusalCodecRejectsMissingOrForeignVariants(t *testing.T) {
	p, s, c := carrierPermitFixture(t)
	r, err := refuseCarrier(s, c)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = cancelPermittedCarrier(p, c, r); err != nil {
		t.Fatal(err)
	}
	for _, o := range []*Owner{p, s} {
		data, err := transferencoding.EncodeTransferJournal(o.state.transfers.events)
		if err != nil {
			t.Fatal(err)
		}
		for _, mutation := range []string{"remove", "null-cut", "wrong-role", "duplicate", "old-cut"} {
			var stored []transferencoding.StoredTransferEvent
			if err := json.Unmarshal(data, &stored); err != nil {
				t.Fatal(err)
			}
			last := &stored[len(stored)-1]
			switch mutation {
			case "remove":
				last.CarrierPermit, last.CarrierRefusal = nil, nil
			case "null-cut":
				if o == s {
					last.CarrierPermit.PermitAt = nil
				} else {
					last.CarrierRefusal.At = nil
				}
			case "wrong-role":
				last.Kind = transferencoding.TransferCarrierCancelled
			case "duplicate":
				stored = append(stored, *last)
			case "old-cut":
				if o == s {
					last.CarrierPermit.ReservationAt = last.CarrierPermit.PermitAt
				} else {
					last.CarrierRefusal.ReservationAt.Serial--
				}
			}
			bad, err := json.Marshal(stored)
			if err != nil {
				t.Fatal(err)
			}
			events, err := transferencoding.LoadTransferJournal(bad, o.state.scope)
			if err != nil {
				continue
			}
			// Decodable values still must reproduce the original owner's
			// retained finite facts and exact ordered commits, not just JSON.
			candidate := o.state.clone()
			candidate.transfers.events = events
			if err := validateOperationHistory(candidate); err == nil {
				t.Fatal("modified refusal history accepted", mutation)
			}
		}
	}
}
