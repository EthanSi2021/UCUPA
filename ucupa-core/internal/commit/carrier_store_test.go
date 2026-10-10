package commit

import (
	"encoding/json"
	"errors"
	"go.etcd.io/bbolt"
	"strings"
	"testing"
	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	storerecord "ucupa/ucupa-core/internal/store/record"
	"ucupa/ucupa-core/internal/token"
	transferencoding "ucupa/ucupa-core/internal/transfer/encoding"
	transferfact "ucupa/ucupa-core/internal/transfer/fact"
)

func TestCarrierIntentRejectionsDoNotClaimControl(t *testing.T) {
	parent, _, _, i := carrierControlFixture(t)
	for _, change := range []func(*transferfact.CarrierIntent){
		func(i *transferfact.CarrierIntent) { i.Actor = values.PrincipalRef{} },
		func(i *transferfact.CarrierIntent) { i.Parent = i.Source },
		func(i *transferfact.CarrierIntent) { i.Target = i.Source },
		func(i *transferfact.CarrierIntent) { i.Mount = foundationStoreID(999) },
		func(i *transferfact.CarrierIntent) {
			i.ExpectedMount = token.MountRevisionFromParts(parent.state.scope, i.Mount, 1)
		},
		func(i *transferfact.CarrierIntent) {
			i.ExpectedElement = token.RevisionFromParts(parent.state.scope, i.Mount, 1)
		},
		func(i *transferfact.CarrierIntent) { i.SourceAddress = "local:stale" },
		func(i *transferfact.CarrierIntent) { i.TargetAddress = "" },
		func(i *transferfact.CarrierIntent) { i.Operation = "carrier-original" },
		func(i *transferfact.CarrierIntent) { i.Operation = "carrier-root" },
		func(i *transferfact.CarrierIntent) {
			i.Actor.Principal = values.PrincipalID(strings.Repeat("x", transferencoding.MaxTransferIntentBytes+1))
		},
	} {
		bad := i
		change(&bad)
		before := parent.state.clone()
		if _, err := reserveCarrier(t, parent, bad); err == nil {
			t.Fatal("invalid intent admitted")
		}
		if !sameCompleteCoreState(before, parent.state) {
			t.Fatal("rejected intent left control residue")
		}
	}
	i.Actor = values.PrincipalRef{Realm: "\xff\x00", Principal: "\xfe"}
	i.Operation = "\x80\x00"
	c, err := reserveCarrier(t, parent, i)
	if err != nil {
		t.Fatal("opaque identity rejected", err)
	}
	parent.RLock()
	_, err = parent.CarrierReplacementLocked(foundationStoreActor, i.Operation)
	parent.RUnlock()
	if !errors.Is(err, values.PublicDenied) {
		t.Fatal("another caller read original operation", err)
	}
	bad := i
	bad.TargetAddress = "local:different"
	if _, err = reserveCarrier(t, parent, bad); !errors.Is(err, contract.ErrOperationConflict) {
		t.Fatal("operation retargeted", err)
	}
	path := parent.store.db.Path()
	closeFoundationStore(t, parent)
	o := openFoundationStore(t, path)
	defer o.CloseStore()
	if got, err := reserveCarrier(t, o, i); err != nil || got != c {
		t.Fatal("opaque bytes changed in journal", got, err)
	}
}

func TestCarrierPreparationRejectsCorruptedHistory(t *testing.T) {
	// Replay proves reachable history, not resistance to replacing the whole
	// database with a different internally consistent history. These mutations
	// break exact input, event shape, chronology or one-way milestones.
	for _, kind := range []string{"missing-input", "changed-mount", "wrong-expected", "second-terminal", "extra-slice", "extra-contact-cut", "reordered", "duplicate"} {
		t.Run(kind, func(t *testing.T) {
			parent, _, _, i := carrierControlFixture(t)
			c, err := reserveCarrier(t, parent, i)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = advanceCarrier(t, parent, c, true); err != nil {
				t.Fatal(err)
			}
			path := parent.store.db.Path()
			closeFoundationStore(t, parent)
			mutateFoundationStore(t, path, func(tx *bbolt.Tx) {
				b := tx.Bucket(elementBucketName)
				var events []transferencoding.StoredTransferEvent
				if err := json.Unmarshal(b.Get(transferJournalKey), &events); err != nil {
					t.Fatal(err)
				}
				switch kind {
				case "missing-input":
					events[0].Carrier = nil
				case "changed-mount":
					events[0].Carrier.Mount = foundationStoreID(999)
				case "wrong-expected":
					events[1].CarrierExpected.Serial--
				case "second-terminal":
					extra := events[1]
					extra.Kind = transferencoding.TransferCarrierCancelled
					extra.Cut++
					events = append(events, extra)
				case "extra-slice":
					events[0].Input = &transferencoding.StoredTransferIntent{}
				case "extra-contact-cut":
					events[0].CarrierExpected = storerecord.StoreMountCut(c.Reservation.At)
				case "reordered":
					events[0], events[1] = events[1], events[0]
				case "duplicate":
					events = append(events, events[1])
				}
				data, err := json.Marshal(events)
				if err != nil {
					t.Fatal(err)
				}
				if err = b.Put(transferJournalKey, data); err != nil {
					t.Fatal(err)
				}
			})
			o, err := OpenDurable(path, nil)
			if o != nil {
				_ = o.CloseStore()
				t.Fatal("corrupt history reopened")
			}
			if !errors.Is(err, contract.ErrStore) {
				t.Fatal(err)
			}
		})
	}
}
