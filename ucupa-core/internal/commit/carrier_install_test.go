package commit

import (
	"errors"
	"testing"

	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	elementfact "ucupa/ucupa-core/internal/element/fact"
	mountfact "ucupa/ucupa-core/internal/mount/fact"
	"ucupa/ucupa-core/internal/token"
	transferfact "ucupa/ucupa-core/internal/transfer/fact"
)

func installCarrier(o *Owner, b transferfact.CarrierBinding) (transferfact.CarrierInstallation, error) {
	o.Lock()
	defer o.Unlock()
	return o.InstallCarrierTargetLocked(b)
}

func boundCarrierFixture(t *testing.T, empty bool) (*Owner, *Owner, *Owner, transferfact.CarrierBinding) {
	t.Helper()
	p, s, d, f, raw := carrierAcceptanceFixture(t, empty)
	a, err := acceptCarrier(d, f, raw)
	if err != nil {
		t.Fatal(err)
	}
	b, err := bindCarrier(p, a)
	if err != nil {
		t.Fatal(err)
	}
	return p, s, d, b
}

func TestCarrierInstallationActivatesExactContentAndRelationTogether(t *testing.T) {
	for _, empty := range []bool{false, true} {
		t.Run(map[bool]string{false: "populated", true: "empty"}[empty], func(t *testing.T) {
			p, s, d, b := boundCarrierFixture(t, empty)
			before, parent, source := d.state.clone(), p.state.clone(), s.state.clone()
			installed, err := installCarrier(d, b)
			if err != nil || !installed.Valid() || installed.Binding != b {
				t.Fatal("install", installed, err)
			}
			a, r := b.Acceptance, b.Acceptance.Fence.Permit.Reservation
			in := d.state.mount.facts.Incoming()
			if in.Key != r.NextKey || in.Admission != (mountfact.Admission{Key: contract.MountAdmissionKey{MountKey: r.NextKey, OperationID: r.Input.Operation}, RangeValue: r.Tuple.Range, DecisionCut: b.At}) || in.Gate != mountfact.GateOpen || in.Acceptance != (contract.MountPrepareRequest{}) || in.AcceptedAt != a.At || in.Contact.NextHop != r.Input.ParentAddress {
				t.Fatal("invalid incoming relation", in)
			}
			count := 1
			if empty {
				count = 0
			}
			if d.state.elements.Counts().Entities != count || d.state.elements.Counts().Anchors != 0 || d.state.cutSerial != before.cutSerial+1 || d.state.revisionSerial != before.revisionSerial+uint64(count) || len(d.state.mount.attempts) != 0 || !sameCompleteCoreState(parent, p.state) || !sameCompleteCoreState(source, s.state) {
				t.Fatal("non-atomic installation or phantom ordinary prepare")
			}
			d.RLock()
			candidate, ok := d.CaptureReadLocked()
			d.RUnlock()
			if !ok || candidate.Validate() != nil || !candidate.Projection.RevisionsValid() || candidate.Projection.carrierInstallation != installed {
				t.Fatal("finite projection invalid")
			}
			bad := *candidate
			bad.Projection.carrierInstallation = transferfact.CarrierInstallation{}
			if bad.Validate() == nil {
				t.Fatal("accepted incoming without actual install origin")
			}
			for _, change := range []func(*mountfact.Incoming){
				func(in *mountfact.Incoming) { in.Admission.DecisionCut = b.Acceptance.Fence.Permit.PermitAt },
				func(in *mountfact.Incoming) { in.Admission.Key.OperationID = "another-parent-decision" },
				func(in *mountfact.Incoming) { in.AcceptedAt = installed.At },
				func(in *mountfact.Incoming) {
					in.Contact.Revision = token.NextHopRevisionFromParts(d.state.scope, in.Key, true, token.Serial(a.At))
				},
			} {
				bad := *candidate
				changed := in
				change(&changed)
				mounts := mountfact.New()
				mounts.ReplaceIncoming(changed)
				bad.Projection.Mount = mounts.View
				if bad.Validate() == nil {
					t.Fatal("accepted contradictory installation projection")
				}
			}
			if !empty {
				v, _ := d.state.elements.Entity(foundationStoreID(105))
				if v.Parent != r.Input.Mount || token.RevisionScope(v.Revision) != d.state.scope {
					t.Fatal("changed placement or kept source revision")
				}
				if _, err := applyFoundationStore(d, elementfact.OperationUpdateEntity, r.Input.Actor, v.ID, foundationStoreTuple(t, v.Parent, 2, 1), v.Revision, "after-install"); err != nil {
					t.Fatal("installed reservation still blocked writes", err)
				}
			}
			if _, err := applyFoundationStore(d, elementfact.OperationCreateEntity, r.Input.Actor, foundationStoreID(9601), foundationStoreTuple(t, r.Input.Mount, 4, 1), contract.Revision{}, "new-content"); err != nil {
				t.Fatal(err)
			}
			d.Lock()
			_, err = d.ApplyMountAddressLocked("repair-parent", NextHopBinding{Actor: r.Input.Actor, Element: r.Input.Mount, Key: in.Key, Expected: in.Contact.Revision, NextHop: "local:repaired-parent"})
			d.Unlock()
			if err != nil {
				t.Fatal("repair issued contact", err)
			}
			after := d.state.clone()
			if got, err := installCarrier(d, b); err != nil || got != installed || !sameCompleteCoreState(after, d.state) {
				t.Fatal("retry overwrote newer state", err)
			}
			path := d.store.db.Path()
			closeFoundationStore(t, d)
			d = openFoundationStore(t, path)
			defer closeFoundationStore(t, d)
			if !sameCompleteCoreState(after, d.state) {
				t.Fatal("replay changed installation")
			}
			if got, err := installCarrier(d, b); err != nil || got != installed {
				t.Fatal("reopened retry", err)
			}
		})
	}
}

func TestCarrierInstallationRejectsDifferentBindingWithoutResidue(t *testing.T) {
	_, _, d, b := boundCarrierFixture(t, false)
	before := d.state.clone()
	for name, change := range map[string]func(*transferfact.CarrierBinding){
		"no-decision":      func(b *transferfact.CarrierBinding) { b.Revision = 0 },
		"wrong-parent-cut": func(b *transferfact.CarrierBinding) { b.At = b.Acceptance.At },
		"other-acceptance": func(b *transferfact.CarrierBinding) { b.Acceptance.Revision++ },
		"other-fence":      func(b *transferfact.CarrierBinding) { b.Acceptance.Fence.Revision++ },
		"other-target": func(b *transferfact.CarrierBinding) {
			b.Acceptance.Fence.Permit.Reservation.Input.Target = values.FactAuthorityRef(foundationStoreID(9998))
		},
	} {
		bad := b
		change(&bad)
		if _, err := installCarrier(d, bad); err == nil || !sameCompleteCoreState(before, d.state) {
			t.Fatal(name, err)
		}
	}
	if _, err := installCarrier(d, b); err != nil {
		t.Fatal(err)
	}
	before = d.state.clone()
	bad := b
	bad.Revision++
	if _, err := installCarrier(d, bad); !errors.Is(err, contract.ErrOperationConflict) || !sameCompleteCoreState(before, d.state) {
		t.Fatal("changed installed retry", err)
	}
}
