package commit

import (
	"encoding/json"
	"errors"
	"go.etcd.io/bbolt"
	"sync"
	"testing"
	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	elementfact "ucupa/ucupa-core/internal/element/fact"
	transferencoding "ucupa/ucupa-core/internal/transfer/encoding"
	transferfact "ucupa/ucupa-core/internal/transfer/fact"
)

func TestTransferAcceptanceRejectsDamagedDurableEvidence(t *testing.T) {
	for _, kind := range []string{"missing-fence", "duplicate", "wrong-target", "digest", "manifest", "future-cut", "source-cut", "extra-source-input"} {
		t.Run(kind, func(t *testing.T) {
			o, path, f, raw := transferTargetFixture(t)
			if _, err := acceptTransfer(t, o, f, raw); err != nil {
				t.Fatal(err)
			}
			closeFoundationStore(t, o)
			mutateFoundationStore(t, path, func(tx *bbolt.Tx) {
				b := tx.Bucket(elementBucketName)
				var events []transferencoding.StoredTransferEvent
				if err := json.Unmarshal(b.Get(transferJournalKey), &events); err != nil {
					t.Fatal(err)
				}
				switch kind {
				case "missing-fence":
					events[0].Fence = nil
				case "duplicate":
					events = append(events, events[0])
				case "wrong-target":
					events[0].Fence.Input.Target = f.Input.Source
				case "digest":
					events[0].Fence.Digest[0] ^= 1
				case "manifest":
					events[0].Manifest = append(events[0].Manifest, ' ')
				case "future-cut":
					events[0].Cut++
				case "source-cut":
					events[0].Fence.Cut++
				case "extra-source-input":
					events[0].Input = &events[0].Fence.Input
				}
				data, err := json.Marshal(events)
				if err != nil {
					t.Fatal(err)
				}
				if err := b.Put(transferJournalKey, data); err != nil {
					t.Fatal(err)
				}
			})
			opened, err := OpenDurable(path, nil)
			if opened != nil {
				_ = opened.CloseStore()
				t.Fatal("opened corrupt target acceptance")
			}
			if !errors.Is(err, contract.ErrStore) {
				t.Fatal(err)
			}
		})
	}
}

func TestTransferAcceptanceRechecksCurrentPlacementAndSerializesDeletion(t *testing.T) {
	for n := 0; n < 8; n++ {
		o, path, f, raw := transferTargetFixture(t)
		parent, _ := o.state.elements.Anchor(f.Input.Final.Parent)
		start := make(chan struct{})
		var acceptErr, deleteErr error
		var wg sync.WaitGroup
		wg.Add(2)
		go func() { defer wg.Done(); <-start; _, acceptErr = acceptTransfer(t, o, f, raw) }()
		go func() {
			defer wg.Done()
			<-start
			_, deleteErr = applyFoundationStore(o, elementfact.OperationDeleteAnchor, f.Input.Actor, parent.ID, contract.SpatialTuple{}, parent.Revision, "delete-before-or-after")
		}()
		close(start)
		wg.Wait()
		if acceptErr == nil {
			if !errors.Is(deleteErr, contract.ErrConflict) {
				t.Fatal("deleted promised parent", deleteErr)
			}
			if len(o.state.transfers.targets) != 1 {
				t.Fatal("missing hold")
			}
		} else {
			if deleteErr != nil || !errors.Is(acceptErr, contract.ErrNotFound) || len(o.state.transfers.targets) != 0 {
				t.Fatal("incorrect concurrent ordering", acceptErr, deleteErr)
			}
		}
		durable := o.state.clone()
		closeFoundationStore(t, o)
		reopened := openFoundationStore(t, path)
		if !sameCompleteCoreState(durable, reopened.state) {
			t.Fatal("replay changed concurrent result")
		}
		closeFoundationStore(t, reopened)
	}
}

func TestTransferAcceptanceRejectsWrongActorTargetAndConflictingIntent(t *testing.T) {
	o, _, f, raw := transferTargetFixture(t)
	before := o.state.clone()
	o.Lock()
	_, err := o.AcceptTransferTargetLocked(values.PrincipalRef{Realm: "other", Principal: "actor"}, f, raw, nil, transferfact.ParentHold{})
	o.Unlock()
	if !errors.Is(err, values.PublicDenied) {
		t.Fatal(err)
	}
	bad := f
	bad.Input.Target = f.Input.Source
	if _, err := acceptTransfer(t, o, bad, raw); err == nil {
		t.Fatal("wrong target accepted")
	}
	if !sameCompleteCoreState(before, o.state) {
		t.Fatal("failed admission changed owner")
	}
	parent, _ := o.state.elements.Anchor(f.Input.Final.Parent)
	i := transferfact.Intent{Profile: transferfact.SliceTransfer, Operation: "outgoing-parent", Actor: f.Input.Actor, Source: o.authority, Target: f.Input.Source,
		Boundary: parent.ID, Expected: parent.Revision, Final: f.Input.Final, SourceAddress: "local:target", TargetAddress: "local:source"}
	pending, err := beginTransfer(t, o, i)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := acceptTransfer(t, o, f, raw); err != nil {
		t.Fatal(err)
	}
	if _, err := fenceTransfer(o, i, pending.Revision); !errors.Is(err, contract.ErrConflict) {
		t.Fatal("outgoing fence bypassed target hold", err)
	}
	if _, err := cancelTransfer(t, o, i, pending.Revision); err != nil {
		t.Fatal("unfenced intent cannot cancel", err)
	}
	// UUID holds are qualified by the incoming source operation, not by a
	// target's ordinary OperationID namespace: the unrelated local ID remains
	// independently usable, while a second exact source key cannot change input.
	if _, err := applyFoundationStore(o, elementfact.OperationCreateEntity, f.Input.Actor, foundationStoreID(1005), foundationStoreTuple(t, parent.ID, 50, 1), contract.Revision{}, f.Input.Operation); err != nil {
		t.Fatal("foreign operation claimed local namespace", err)
	}
	bad = f
	bad.Input.Final = foundationStoreTuple(t, parent.ID, 20, 20)
	if _, err := acceptTransfer(t, o, bad, raw); !errors.Is(err, contract.ErrOperationConflict) {
		t.Fatal(err)
	}
}
