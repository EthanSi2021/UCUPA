package commit

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"testing"
	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	elementfact "ucupa/ucupa-core/internal/element/fact"
	mountfact "ucupa/ucupa-core/internal/mount/fact"
	"ucupa/ucupa-core/internal/token"
	transferencoding "ucupa/ucupa-core/internal/transfer/encoding"
	transferfact "ucupa/ucupa-core/internal/transfer/fact"
)

func fenceCarrier(o *Owner, c transferfact.CarrierControl) (transferfact.CarrierFence, string, error) {
	o.Lock()
	defer o.Unlock()
	return o.FenceCarrierSourceLocked(c.Reservation.Input.Actor, c)
}

func TestCarrierFenceEntireContentAndEmptySpaceSurviveReopen(t *testing.T) {
	for _, empty := range []bool{false, true} {
		t.Run(map[bool]string{false: "populated", true: "empty"}[empty], func(t *testing.T) {
			p, s, c := carrierPermitFixture(t)
			id, mount := foundationStoreID(105), c.Reservation.Input.Mount
			if empty {
				e, _ := s.state.elements.Entity(id)
				if _, err := applyFoundationStore(s, elementfact.OperationDeleteEntity, foundationStoreActor, id, contract.SpatialTuple{}, e.Revision, "carrier-empty"); err != nil {
					t.Fatal(err)
				}
			} else {
				if _, err := applyFoundationStore(s, elementfact.OperationCreateAnchor, foundationStoreActor, foundationStoreID(106), foundationStoreTuple(t, mount, 0, 5), contract.Revision{}, "nested-anchor"); err != nil {
					t.Fatal(err)
				}
				if _, err := applyFoundationStore(s, elementfact.OperationCreateEntity, foundationStoreActor, foundationStoreID(107), foundationStoreTuple(t, foundationStoreID(106), 0, 1), contract.Revision{}, "nested-child"); err != nil {
					t.Fatal(err)
				}
			}
			before, parent := s.state.clone(), p.state.clone()
			f, raw, err := fenceCarrier(s, c)
			if err != nil || !f.Valid() || f.FinalCut != before.currentCut() {
				t.Fatal("fence", f, err)
			}
			slice, err := transferencoding.DecodeCarrierManifest(f, raw)
			if err != nil || slice.Profile() != transferfact.CarrierReplacement || slice.Boundary() != mount || len(slice.Ancestors()) != 0 {
				t.Fatal("manifest", err)
			}
			want := 3
			if empty {
				want = 0
			}
			if len(slice.Members()) != want {
				t.Fatal("incomplete carrier manifest", len(slice.Members()), want)
			}
			for _, m := range slice.Members() {
				if m.ID() == mount {
					t.Fatal("parent mount copied as local element")
				}
				d, ok := s.state.elements.Departure(m.ID())
				if !ok || d.Controller != p.authority || d.Operation != c.Reservation.Input.Operation || d.Root {
					t.Fatal("lost actual controller", d)
				}
				if !sourceVisibility(s.state).Inactive(m.ID()) {
					t.Fatal("departure reads as nonexistent")
				}
			}
			if !sameCompleteCoreState(parent, p.state) || s.state.elements.Kind(mount) != 0 || s.state.elements.RootExists() ||
				s.state.revisionSerial != before.revisionSerial || s.state.cutSerial != before.cutSerial+1 || len(s.state.operations) != len(before.operations) {
				t.Fatal("fence rewrote parent, revision, or original results")
			}
			if s.state.mount.facts.Incoming().Gate != mountfact.GateFenced || s.state.mount.facts.Incoming().Detached {
				t.Fatal("fence treated as ordinary revoke")
			}
			if got, bytes, err := fenceCarrier(s, c); err != nil || got != f || bytes != raw {
				t.Fatal("nonexact fence retry", err)
			}
			if _, err := refuseCarrier(s, c); !errors.Is(err, values.PublicUnavailable) {
				t.Fatal("fenced source promised refusal", err)
			}
			if _, err := applyFoundationStore(s, elementfact.OperationCreateEntity, foundationStoreActor, foundationStoreID(108), foundationStoreTuple(t, mount, 0, 1), contract.Revision{}, "late-carrier-write"); !errors.Is(err, contract.ErrMountBusy) {
				t.Fatal("post-fence write", err)
			}
			s.RLock()
			if err := s.CheckTransferReadLocked(before.elements.View, before.currentCut(), mount, ""); !errors.Is(err, contract.ErrReadCutExpired) {
				t.Fatal("old disclosure", err)
			}
			if err := s.CheckTransferReadLocked(s.state.elements.View, s.state.currentCut(), mount, ""); !errors.Is(err, contract.ErrUnavailable) {
				t.Fatal("empty source disclosed as complete", err)
			}
			view, ok := s.CaptureReadLocked()
			s.RUnlock()
			if !ok || view.Validate() != nil {
				t.Fatal("finite fenced read projection invalid")
			}
			view.Projection.carrierFence = transferfact.CarrierFence{}
			if view.Validate() == nil {
				t.Fatal("projection accepted fenced gate without proof")
			}
			path, durable := s.store.db.Path(), s.state.clone()
			closeFoundationStore(t, s)
			s = openFoundationStore(t, path)
			defer closeFoundationStore(t, s)
			if !sameCompleteCoreState(durable, s.state) {
				t.Fatal("replay changed fence")
			}
			if got, bytes, err := fenceCarrier(s, c); err != nil || got != f || bytes != raw {
				t.Fatal("restart lost immutable fence", err)
			}
		})
	}
}

func TestCarrierFenceRefusalAndRelationConflictsLeaveNoState(t *testing.T) {
	_, s, c := carrierPermitFixture(t)
	if _, err := refuseCarrier(s, c); err != nil {
		t.Fatal(err)
	}
	before := s.state.clone()
	if _, _, err := fenceCarrier(s, c); !errors.Is(err, values.PublicUnavailable) || !sameCompleteCoreState(before, s.state) {
		t.Fatal("late fence overrode refusal", err)
	}
	_, s, c = carrierPermitFixture(t)
	for _, mutate := range []func(*transferfact.CarrierControl){
		func(c *transferfact.CarrierControl) { c.PermitAt = contract.CoreReadCut{} },
		func(c *transferfact.CarrierControl) { c.Reservation.Previous.OperationID = "another-admission" },
		func(c *transferfact.CarrierControl) { c.Reservation.Input.ParentAddress = "local:wrong-parent" },
		func(c *transferfact.CarrierControl) {
			c.Reservation.Tuple.Range = foundationStoreTuple(t, c.Reservation.Input.Mount, 0, 19).Range
		},
	} {
		changed := c
		mutate(&changed)
		before = s.state.clone()
		if _, _, err := fenceCarrier(s, changed); err == nil || !sameCompleteCoreState(before, s.state) {
			t.Fatal("invalid relation consumed a fence", err)
		}
	}
	f, raw, err := fenceCarrier(s, c)
	if err != nil {
		t.Fatal(err)
	}
	changed := c
	changed.Reservation.Input.TargetAddress = "local:substitute"
	if _, _, err := fenceCarrier(s, changed); !errors.Is(err, contract.ErrOperationConflict) {
		t.Fatal("changed target reused fence", err)
	}
	// Old ordinary Finish only returns its old result; it cannot reopen the gate.
	old := s.state.mount.attempts[contract.MountAttemptKey{ParentAuthority: c.Reservation.Input.Parent, OperationID: "carrier-original"}].Closed
	s.Lock()
	_, err = s.ApplyMountFinishLocked(c.Reservation.Input.Parent, contract.MountFinishRequest{Evidence: old.Evidence})
	s.Unlock()
	if err != nil || s.state.mount.facts.Incoming().Gate != mountfact.GateFenced {
		t.Fatal("old finish reopened fence", err)
	}
	for _, mutation := range []string{"drop-member", "change-cut", "copy-mount", "bad-parent", "bad-lineage", "phantom-ancestor"} {
		var m transferencoding.StoredTransferManifest
		if err := json.Unmarshal([]byte(raw), &m); err != nil {
			t.Fatal(err)
		}
		switch mutation {
		case "drop-member":
			m.Members = nil
		case "change-cut":
			m.FinalCut++
		case "copy-mount":
			m.Members[0].ID = c.Reservation.Input.Mount
		case "bad-parent":
			m.Members[0].Element.Parent = string(foundationStoreID(999))
		case "bad-lineage":
			m.Members[0].Lineage = []transferencoding.StoredLineageStep{{}}
		case "phantom-ancestor":
			m.Ancestors = []transferencoding.StoredTransferAncestor{{ID: c.Reservation.Input.Mount}}
		}
		bytes, err := json.Marshal(m)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := transferencoding.DecodeCarrierManifest(f, string(bytes)); err == nil {
			t.Fatal("mutated bytes accepted", mutation)
		}
		if mutation != "drop-member" { // a source-authenticated empty closure is independently legitimate
			altered := f
			altered.ManifestDigest = sha256.Sum256(bytes)
			if _, err := transferencoding.DecodeCarrierManifest(altered, string(bytes)); err == nil {
				t.Fatal("invalid graph accepted with matching hash", mutation)
			}
		}
	}
	if token.Serial(f.FinalCut) >= s.state.cutSerial {
		t.Fatal("final cut was not last active cut")
	}
}
