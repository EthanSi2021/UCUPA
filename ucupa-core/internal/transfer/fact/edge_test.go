package fact

import (
	"errors"
	"testing"

	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	mountfact "ucupa/ucupa-core/internal/mount/fact"
	"ucupa/ucupa-core/internal/token"
)

func TestSelectExternalEdgePreservesOnlyStableExactBoundary(t *testing.T) {
	e, f, m := tree(t)
	key := contract.MountKey{ParentAuthority: sourceOwner, MountAnchorUUID: id(5), CarrierAuthority: targetOwner,
		ProposalRevision: token.MountRevisionFromParts([32]byte{1}, id(5), 80)}
	record := mountfact.Record{State: contract.MountActive, Key: key, Admission: contract.MountAdmissionKey{MountKey: key, OperationID: "mount"},
		Revision: token.MountRevisionFromParts([32]byte{1}, id(5), 90), Contact: mountfact.Contact{NextHop: "local:carrier",
			Revision: token.NextHopRevisionFromParts([32]byte{1}, key, false, 95)}}
	m.ReplaceOutgoing(id(5), record)
	s, err := Select(sourceOwner, cut(), e.View, f.View, m.View, id(5), 2)
	if err != nil || !s.HasMounts() || len(s.Members()) != 1 || s.Members()[0].Mount != record {
		t.Fatal("lost stable boundary", err)
	}
	for name, change := range map[string]func(*mountfact.Record){
		"slot":             func(r *mountfact.Record) { r.Slot = "pending" },
		"detached":         func(r *mountfact.Record) { r.State = contract.MountDetached },
		"admission":        func(r *mountfact.Record) { r.Admission.OperationID = "" },
		"foreign-revision": func(r *mountfact.Record) { r.Revision = token.MountRevisionFromParts([32]byte{2}, id(5), 90) },
		"future-revision":  func(r *mountfact.Record) { r.Revision = token.MountRevisionFromParts([32]byte{1}, id(5), 101) },
		"future-contact": func(r *mountfact.Record) {
			r.Contact.Revision = token.NextHopRevisionFromParts([32]byte{1}, key, false, 101)
		},
		"incoming-contact": func(r *mountfact.Record) {
			r.Contact.Revision = token.NextHopRevisionFromParts([32]byte{1}, key, true, 95)
		},
		"no-address": func(r *mountfact.Record) { r.Contact.NextHop = "" },
	} {
		t.Run(name, func(t *testing.T) {
			bad := record
			change(&bad)
			badMounts := m.Clone()
			badMounts.ReplaceOutgoing(id(5), bad)
			_, err := Select(sourceOwner, cut(), e.View, f.View, badMounts.View, id(5), 2)
			if !errors.Is(err, contract.ErrMountCoordinationRequired) && !errors.Is(err, contract.ErrMountBusy) {
				t.Fatal("invalid boundary selected", err)
			}
		})
	}
}

func TestEdgeClosureBindsOriginalOwnersAndExplicitFinalRange(t *testing.T) {
	e, _, _ := tree(t)
	a, _ := e.Anchor(id(5))
	carrier := values.FactAuthorityRef("00000000-0000-4000-8000-000000000030")
	key := contract.MountKey{ParentAuthority: sourceOwner, CarrierAuthority: carrier, MountAnchorUUID: id(5), ProposalRevision: token.MountRevisionFromParts([32]byte{1}, id(5), 80)}
	f := EdgeFence{Mount: id(5), Source: SourceFence{Input: Intent{Profile: SliceTransfer, Source: sourceOwner, Target: targetOwner,
		Operation: "move", Actor: values.PrincipalRef{Realm: "r", Principal: "p"}, Boundary: id(5), Expected: a.Revision,
		Final: tuple(t, id(10), 0, 15), SourceAddress: "local:source", TargetAddress: "local:target"}, FinalCut: cut(), ManifestDigest: [32]byte{1}}}
	m := Member{Kind: contract.ElementAnchor, Anchor: a, Mount: mountfact.Record{State: contract.MountActive}}
	r, err := f.EdgeRange(m)
	if err != nil || r != f.Source.Input.Final.Range {
		t.Fatal("ignored caller's final range", err)
	}
	c := EdgeClosure{Fence: f, Previous: mountfact.Admission{Key: contract.MountAdmissionKey{MountKey: key, OperationID: "mount"},
		RangeValue: sphere(t, 20), DecisionCut: token.FromParts([32]byte{1}, 90)}, Range: r, At: token.FromParts([32]byte{3}, 5), Revision: 1}
	if !c.Valid(carrier) || c.Valid(sourceOwner) || c.Valid(targetOwner) {
		t.Fatal("closure owner binding")
	}
	installation := CoreInstallation{Source: TargetInstallation{Decision: SourceDecision{
		Acceptance: TargetAcceptance{Fence: f.Source, At: token.FromParts([32]byte{2}, 10), Revision: 1},
		At:         token.FromParts([32]byte{1}, 101), Revision: 5}, At: token.FromParts([32]byte{2}, 11), Revision: 2}}
	delegation, err := installation.Delegation(c)
	if err != nil || delegation.MountKey.ParentAuthority != targetOwner || delegation.MountKey.CarrierAuthority != carrier || delegation.MountKey.MountAnchorUUID != c.Fence.Mount || installation.Address() != f.Source.Input.TargetAddress {
		t.Fatal("new parent relation did not preserve exact owners", err)
	}
	completion := CoreCompletion{Installed: installation, At: token.FromParts([32]byte{2}, 12), Revision: 3}
	if !completion.Valid() || !(CoreCompletion{Installed: installation, At: installation.Cut(), Revision: installation.Revision()}).Valid() {
		t.Fatal("valid completion profiles rejected")
	}
	for name, mutate := range map[string]func(*CoreCompletion){
		"other-owner":            func(v *CoreCompletion) { v.At = token.FromParts([32]byte{3}, 12) },
		"older-cut":              func(v *CoreCompletion) { v.At = token.FromParts([32]byte{2}, 10) },
		"same-cut-new-revision":  func(v *CoreCompletion) { v.At = installation.Cut() },
		"later-cut-old-revision": func(v *CoreCompletion) { v.Revision = installation.Revision() },
		"both-profiles":          func(v *CoreCompletion) { v.Installed.Carrier.Revision = 1 },
	} {
		t.Run(name, func(t *testing.T) {
			bad := completion
			mutate(&bad)
			if bad.Valid() {
				t.Fatal("invalid completion accepted")
			}
		})
	}
	edge := EdgeInstallation{Parent: installation, Closure: c, At: token.FromParts([32]byte{3}, 6), Revision: 2}
	if !edge.Valid() {
		t.Fatal("matching edge receipt invalid")
	}
	edge.At = c.At
	if edge.Valid() {
		t.Fatal("closure cut reused as installation")
	}
	c.Fence.Source.Input.Target = carrier
	if c.Valid(carrier) {
		t.Fatal("accepted self-parent cycle")
	}
	f.Source.Input.Boundary, f.Source.Input.Expected = id(2), revision(2)
	if r, err := f.EdgeRange(m); err != nil || r != sphere(t, 20) {
		t.Fatal("surrounding slice rewrote inner mount range", err)
	}
}
