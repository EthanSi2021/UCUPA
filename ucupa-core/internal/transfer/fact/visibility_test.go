package fact

import (
	"reflect"
	"testing"

	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	mountfact "ucupa/ucupa-core/internal/mount/fact"
	"ucupa/ucupa-core/internal/token"
)

func TestVisibilityOwnsOnlyImmutableFiniteReadEvidence(t *testing.T) {
	inactive := map[values.ElementID]values.ElementID{id(2): id(2), id(3): id(2)}
	fences := []SourceFootprint{{Boundary: id(2), Origin: tuple(t, id(1), 5, 20)}}
	v := NewVisibility(inactive, fences, nil, nil, nil)
	delete(inactive, id(3))
	inactive[id(4)] = id(2)
	fences[0].Boundary = id(4)
	if !v.Inactive(id(3)) || v.Inactive(id(4)) || v.Boundary(id(3)) != id(2) {
		t.Fatal("input mutation changed read evidence")
	}
	if members, envelopes, parents := v.Counts(); members != 2 || envelopes != 1 || parents != 0 {
		t.Fatalf("accounting omitted evidence: %d %d", members, envelopes)
	}
	for f := range v.Fences() {
		if f.Boundary != id(2) || f.Origin.Parent != id(1) {
			t.Fatal("envelope changed")
		}
		f.Origin.Parent = id(4)
	}
	for f := range v.Fences() {
		if f.Origin.Parent != id(1) {
			t.Fatal("iteration exposed mutable state")
		}
	}
	// Fail a future accidental addition of full records, manifests or journals.
	// The fourth field is the audited terminal successor evidence map and the
	// fifth the pending own-decision map that defers its proof.
	if reflect.TypeOf(v).NumField() != 5 || reflect.TypeOf(SourceFootprint{}).NumField() != 3 {
		t.Fatal("read evidence shape requires a fresh retention and privacy audit")
	}
	var empty Visibility
	if empty.Inactive(id(2)) || empty.Boundary(id(2)) != "" {
		t.Fatal("empty evidence invented a departure")
	}
	for range empty.Fences() {
		t.Fatal("empty evidence invented an envelope")
	}
}

func TestParentReadEvidenceIsExactImmutableAndNotAWritePermit(t *testing.T) {
	key := contract.MountKey{ParentAuthority: sourceOwner, MountAnchorUUID: id(5), CarrierAuthority: targetOwner,
		ProposalRevision: token.MountRevisionFromParts([32]byte{1}, id(5), 80)}
	r := mountfact.Record{State: contract.MountActive, Key: key, Slot: "parent-hold",
		Admission: contract.MountAdmissionKey{MountKey: key, OperationID: "admit"},
		Revision:  token.MountRevisionFromParts([32]byte{1}, id(5), 90), Contact: mountfact.Contact{NextHop: "local:carrier"}}
	parents := map[values.ElementID]mountfact.Record{id(5): r}
	v := NewVisibility(nil, nil, parents, nil, nil)
	delete(parents, id(5))
	if members, fences, held := v.Counts(); members != 0 || fences != 0 || held != 1 || !v.OutgoingReadable(id(5), r) {
		t.Fatal("held relation copy/accounting changed")
	}
	if (Visibility{}).OutgoingReadable(id(5), r) || v.OutgoingReadable(id(6), r) {
		t.Fatal("ordinary occupied slot or another mount was opened")
	}
	for name, change := range map[string]func(*mountfact.Record){
		"slot":      func(r *mountfact.Record) { r.Slot = "other" },
		"address":   func(r *mountfact.Record) { r.Contact.NextHop = "local:repaired" },
		"revision":  func(r *mountfact.Record) { r.Revision = token.MountRevisionFromParts([32]byte{1}, id(5), 91) },
		"admission": func(r *mountfact.Record) { r.Admission.OperationID = "new" },
		"state":     func(r *mountfact.Record) { r.State = contract.MountDetached },
		"carrier":   func(r *mountfact.Record) { r.Key.CarrierAuthority = sourceOwner },
	} {
		t.Run(name, func(t *testing.T) {
			changed := r
			change(&changed)
			if v.OutgoingReadable(id(5), changed) {
				t.Fatal("old read promise authorized changed relationship")
			}
		})
	}
	r.Slot = ""
	if !(Visibility{}).OutgoingReadable(id(5), r) {
		t.Fatal("ordinary stable relationship stopped")
	}
}
