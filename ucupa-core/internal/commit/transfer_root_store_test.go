package commit

import (
	"encoding/json"
	"errors"
	"go.etcd.io/bbolt"
	"testing"
	"ucupa/ucupa-core/contract"
	storerecord "ucupa/ucupa-core/internal/store/record"
	transferencoding "ucupa/ucupa-core/internal/transfer/encoding"
	transferfact "ucupa/ucupa-core/internal/transfer/fact"
)

func TestRootTransferStoredProfileAndRetiredRoleAreProvedByReplay(t *testing.T) {
	for _, kind := range []string{"root-marker-removed", "child-marker-added", "root-role-moved", "profile-removed", "profile-bounded", "root-has-final", "live-root-marker", "missing-controller", "wrong-controller"} {
		t.Run(kind, func(t *testing.T) {
			source, _, _ := transferIntentFixture(t)
			target := emptyRootReceiver(t)
			i := rootTransferIntent(t, source, target, "root-store")
			f, raw := frozenSource(t, source, i)
			completeFencedTransfer(t, source, target, f, raw)
			o := source
			if kind == "live-root-marker" {
				o = target
			}
			path := o.store.db.Path()
			closeFoundationStore(t, o)
			mutateFoundationStore(t, path, func(tx *bbolt.Tx) {
				b := tx.Bucket(elementBucketName)
				changeElement := func(id uint64, root bool) {
					key := currentStoreElementKey(t, foundationStoreID(id))
					var v storerecord.StoredElement
					if err := json.Unmarshal(b.Get(key), &v); err != nil {
						t.Fatal(err)
					}
					v.RootDeparture = root
					if kind == "missing-controller" {
						v.DepartureController = ""
					}
					if kind == "wrong-controller" {
						v.DepartureController = target.authority
					}
					data, err := json.Marshal(v)
					if err != nil {
						t.Fatal(err)
					}
					if err := b.Put(key, data); err != nil {
						t.Fatal(err)
					}
				}
				switch kind {
				case "missing-controller", "wrong-controller":
					changeElement(100, true)
					return
				case "root-marker-removed":
					changeElement(100, false)
					return
				case "child-marker-added":
					changeElement(102, true)
					return
				case "root-role-moved":
					changeElement(100, false)
					changeElement(102, true)
					return
				case "live-root-marker":
					changeElement(100, true)
					return
				}
				var events []transferencoding.StoredTransferEvent
				if err := json.Unmarshal(b.Get(transferJournalKey), &events); err != nil {
					t.Fatal(err)
				}
				switch kind {
				case "profile-removed":
					events[0].Input.Profile = 0
				case "profile-bounded":
					events[0].Input.Profile = transferfact.SliceTransfer
				case "root-has-final":
					v := storerecord.StoreTuple(contract.SpatialTuple{})
					events[0].Input.Final = &v
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
				t.Fatal("corrupted root reopened")
			}
			if !errors.Is(err, contract.ErrStore) {
				t.Fatal("wrong classification", err)
			}
		})
	}
}
