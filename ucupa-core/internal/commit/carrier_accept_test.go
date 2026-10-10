package commit

import (
	"errors"
	"testing"

	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	elementfact "ucupa/ucupa-core/internal/element/fact"
	transferfact "ucupa/ucupa-core/internal/transfer/fact"
)

func carrierAcceptanceFixture(t *testing.T, empty bool) (parent, source, target *Owner, fence transferfact.CarrierFence, raw string) {
	t.Helper()
	p, s, d, i := carrierControlFixture(t)
	if empty {
		id := foundationStoreID(105)
		v, _ := s.state.elements.Entity(id)
		if _, err := applyFoundationStore(s, elementfact.OperationDeleteEntity, i.Actor, id, contract.SpatialTuple{}, v.Revision, "empty-source"); err != nil {
			t.Fatal(err)
		}
	}
	c, err := reserveCarrier(t, p, i)
	if err != nil {
		t.Fatal(err)
	}
	c, err = advanceCarrier(t, p, c, true)
	if err != nil {
		t.Fatal(err)
	}
	f, bytes, err := fenceCarrier(s, c)
	if err != nil {
		t.Fatal(err)
	}
	return p, s, d, f, bytes
}

func acceptCarrier(o *Owner, f transferfact.CarrierFence, raw string) (transferfact.CarrierAcceptance, error) {
	o.Lock()
	defer o.Unlock()
	return o.AcceptCarrierTargetLocked(f.Permit.Reservation.Input.Actor, f, raw, nil)
}

func TestCarrierAcceptanceRetainsPrivateContentAndReceiverAcrossRestart(t *testing.T) {
	for _, empty := range []bool{true, false} {
		t.Run(map[bool]string{true: "empty", false: "populated"}[empty], func(t *testing.T) {
			p, s, d, f, raw := carrierAcceptanceFixture(t, empty)
			before, parent, source := d.state.clone(), p.state.clone(), s.state.clone()
			a, err := acceptCarrier(d, f, raw)
			if err != nil || !a.Valid() || a.Fence != f || a.At != before.currentCut() || a.Revision != 1 {
				t.Fatal("acceptance", a, err)
			}
			if !sameCoreFacts(before, d.state) || len(d.state.operations) != len(before.operations) || len(d.state.commits) != len(before.commits) ||
				!sameCompleteCoreState(parent, p.state) || !sameCompleteCoreState(source, s.state) {
				t.Fatal("acceptance activated facts or changed another owner")
			}
			key := f.Permit.Reservation.Input.Key()
			r := d.state.transfers.targets[key]
			want := 1
			if empty {
				want = 0
			}
			if r.carrier != a || r.receipt != (transferfact.TargetAcceptance{}) || len(r.members) != want || d.state.mount.facts.Incoming().Key.Valid() || d.state.elements.Counts().Claims != 0 {
				t.Fatal("candidate escaped its private target reservation")
			}
			for n := 0; n < 2; n++ {
				if got, err := acceptCarrier(d, f, raw); err != nil || got != a || len(d.state.transfers.events) != 1 {
					t.Fatal("retry", got, err)
				}
			}
			if _, err := acceptCarrier(d, f, raw+" "); !errors.Is(err, contract.ErrOperationConflict) {
				t.Fatal("changed retry bytes", err)
			}
			path, durable := d.store.db.Path(), d.state.clone()
			closeFoundationStore(t, d)
			d = openFoundationStore(t, path)
			defer closeFoundationStore(t, d)
			if !sameCompleteCoreState(durable, d.state) {
				t.Fatal("replay lost durable acceptance")
			}
			if got, err := acceptCarrier(d, f, raw); err != nil || got != a {
				t.Fatal("reopened retry", err)
			}
			if _, err := applyFoundationStore(d, elementfact.OperationCreateRoot, foundationStoreActor, foundationStoreID(9500), contract.SpatialTuple{}, contract.Revision{}, "steal-root"); !errors.Is(err, contract.ErrConflict) {
				t.Fatal("receiver position was not reserved", err)
			}
			other, request := competingRootMount(t, d)
			d.Lock()
			prepared, err := d.ApplyMountPrepareLocked(other.authority, request, true)
			d.Unlock()
			if err != nil || prepared.Prepared || prepared.Rejection != contract.MountAbortConflict {
				t.Fatal("ordinary mount bypassed reservation", prepared, err)
			}
			if got, err := acceptCarrier(d, f, raw); err != nil || got != a {
				t.Fatal("intervening refused mount changed original receipt", err)
			}
			if err := validateCoreState(d.state); err != nil {
				t.Fatal("complete history", err)
			}
		})
	}
}

func TestCarrierAcceptanceRejectsChangedSourceAndCallerWithoutResidue(t *testing.T) {
	_, _, d, f, raw := carrierAcceptanceFixture(t, false)
	before := d.state.clone()
	d.Lock()
	_, err := d.AcceptCarrierTargetLocked(values.PrincipalRef{Realm: "r", Principal: "other"}, f, raw, nil)
	d.Unlock()
	if !errors.Is(err, values.PublicDenied) || !sameCompleteCoreState(before, d.state) {
		t.Fatal("wrong actor", err)
	}
	for name, change := range map[string]func(*transferfact.CarrierFence){
		"target": func(f *transferfact.CarrierFence) {
			f.Permit.Reservation.Input.Target = f.Permit.Reservation.Input.Source
		},
		"source-endpoint": func(f *transferfact.CarrierFence) { f.Permit.Reservation.Input.SourceAddress = "local:wrong-source" },
		"parent-endpoint": func(f *transferfact.CarrierFence) { f.Permit.Reservation.Input.ParentAddress = "local:wrong-parent" },
		"no-fence":        func(f *transferfact.CarrierFence) { f.Revision = 0 },
		"wrong-cut":       func(f *transferfact.CarrierFence) { f.FinalCut = f.Permit.PermitAt },
		"digest":          func(f *transferfact.CarrierFence) { f.ManifestDigest[0] ^= 1 },
		"no-permit":       func(f *transferfact.CarrierFence) { f.Permit.PermitAt = contract.CoreReadCut{} },
	} {
		bad := f
		change(&bad)
		if _, err := acceptCarrier(d, bad, raw); err == nil || !sameCompleteCoreState(before, d.state) {
			t.Fatal(name, "accepted or changed state", err)
		}
	}
	if _, err := acceptCarrier(d, f, raw+" "); err == nil || !sameCompleteCoreState(before, d.state) {
		t.Fatal("bad manifest accepted", err)
	}
	if _, err := acceptCarrier(d, f, raw); err != nil {
		t.Fatal(err)
	}
	before = d.state.clone()
	bad := f
	bad.Revision++
	if _, err := acceptCarrier(d, bad, raw); !errors.Is(err, contract.ErrOperationConflict) || !sameCompleteCoreState(before, d.state) {
		t.Fatal("source fence substitution", err)
	}
	// The same qualified ID cannot switch from parent-controlled carrier
	// replacement to a source-controlled slice/root transaction.
	root := f.Permit.Reservation.Input
	local := transferfact.SourceFence{Input: transferfact.Intent{Profile: transferfact.RootSuccessor, Operation: root.Operation, Actor: root.Actor, Source: root.Parent, Target: root.Target,
		Boundary: root.Mount, Expected: root.ExpectedElement, SourceAddress: root.ParentAddress, TargetAddress: root.TargetAddress}}
	if _, err := acceptTransfer(t, d, local, raw); !errors.Is(err, contract.ErrOperationConflict) || !sameCompleteCoreState(before, d.state) {
		t.Fatal("cross-profile ID collision", err)
	}
}

func TestCarrierAcceptanceDoesNotOverwriteExistingReceiverSpace(t *testing.T) {
	for _, mode := range []string{"active-root", "incoming-mount", "other-root-transfer"} {
		t.Run(mode, func(t *testing.T) {
			_, _, d, f, raw := carrierAcceptanceFixture(t, true)
			switch mode {
			case "active-root":
				if _, err := applyFoundationStore(d, elementfact.OperationCreateRoot, foundationStoreActor, foundationStoreID(9500), contract.SpatialTuple{}, contract.Revision{}, "root-first"); err != nil {
					t.Fatal(err)
				}
			case "incoming-mount":
				p, request := competingRootMount(t, d)
				d.Lock()
				prepared, err := d.ApplyMountPrepareLocked(p.authority, request, true)
				d.Unlock()
				if err != nil || !prepared.Prepared {
					t.Fatal(prepared, err)
				}
			case "other-root-transfer":
				s, _, _ := transferIntentFixture(t)
				i := rootTransferIntent(t, s, d, "other-root")
				f, raw := frozenSource(t, s, i)
				if _, err := acceptTransfer(t, d, f, raw); err != nil {
					t.Fatal(err)
				}
			}
			before := d.state.clone()
			if _, err := acceptCarrier(d, f, raw); err == nil || !sameCompleteCoreState(before, d.state) {
				t.Fatal("overwrote receiver", err)
			}
		})
	}
}
