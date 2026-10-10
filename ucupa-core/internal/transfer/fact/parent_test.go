package fact

import (
	"testing"

	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	"ucupa/ucupa-core/internal/token"
)

func parentFixture(t *testing.T, role ParentRole) ParentHold {
	t.Helper()
	p, err := values.NewFactAuthorityRef()
	if err != nil {
		t.Fatal(err)
	}
	i := Intent{Profile: SliceTransfer, Operation: "transfer", Actor: values.PrincipalRef{Realm: "r", Principal: "p"},
		Source: sourceOwner, Target: targetOwner, Boundary: id(2), Expected: revision(2),
		Final: tuple(t, id(8), 0, 1), SourceAddress: "local:source", TargetAddress: "local:target"}
	scope := [32]byte{7}
	pi := ParentIntent{Initial: IntentRecord{Input: i, Progress: IntentPending, Admission: token.FromParts(token.RevisionScope(i.Expected), 4), Revision: 5},
		Role: role, Parent: p, Mount: id(8), ExpectedElement: token.RevisionFromParts(scope, id(8), 2), ExpectedMount: token.MountRevisionFromParts(scope, id(8), 9), ParentAddress: "local:parent"}
	k := contract.MountKey{ParentAuthority: p, CarrierAuthority: pi.Carrier(), MountAnchorUUID: pi.Mount, ProposalRevision: token.MountRevisionFromParts(scope, pi.Mount, 3)}
	return ParentHold{Input: pi, Admission: contract.MountAdmissionKey{MountKey: k, OperationID: "original"}, Tuple: tuple(t, id(1), 0, 20),
		Contact: token.NextHopRevisionFromParts(scope, k, false, 3), At: token.FromParts(scope, 10), Revision: 7}
}

func TestParentHoldShapeAndQualifiedSlot(t *testing.T) {
	for _, role := range []ParentRole{SourceParent, TargetParent} {
		h := parentFixture(t, role)
		if !h.Valid() {
			t.Fatal("valid hold rejected")
		}
		for _, change := range []func(*ParentHold){
			func(h *ParentHold) { h.Input.Role = 0 },
			func(h *ParentHold) { h.Input.Initial.Progress = IntentFenced },
			func(h *ParentHold) { h.Input.Initial.FinalCut = h.Input.Initial.Admission },
			func(h *ParentHold) { h.Input.Parent = h.Input.Carrier() },
			func(h *ParentHold) { h.Admission.OperationID = "" },
			func(h *ParentHold) { h.Admission.MountKey.CarrierAuthority = h.Input.Parent },
			func(h *ParentHold) { h.Tuple.Parent = h.Input.Mount },
			func(h *ParentHold) {
				h.Contact = token.NextHopRevisionFromParts(token.Scope(h.At), h.Admission.MountKey, true, 3)
			},
			func(h *ParentHold) { h.At = token.FromParts(token.Scope(h.At), 9) },
		} {
			bad := h
			change(&bad)
			if bad.Valid() {
				t.Fatal("bad parent evidence accepted")
			}
		}
		i := h.Input
		changed := i
		changed.Initial.Input.Final = tuple(t, id(8), 2, 1)
		if changed.Slot() != i.Slot() {
			t.Fatal("changed input escaped exact-operation conflict")
		}
		changed = i
		changed.Initial.Input.Source = h.Input.Parent
		if changed.Slot() == i.Slot() {
			t.Fatal("different controllers share slot")
		}
		changed = i
		changed.Role = 3 - i.Role
		if changed.Slot() == i.Slot() {
			t.Fatal("two roles share slot")
		}
	}
}

func TestParentReleaseNeedsExactSourceTerminal(t *testing.T) {
	h := parentFixture(t, TargetParent)
	cancelled := h.Input.Initial
	cancelled.Progress, cancelled.Revision = IntentCancelled, cancelled.Revision+1
	cancel := ParentRelease{Cancelled: cancelled}
	if !cancel.ValidFor(h) || !(ParentControl{Hold: h, Revision: h.Revision}).Valid() {
		t.Fatal("valid cancellation or pending control rejected")
	}
	for _, change := range []func(*ParentRelease){
		func(r *ParentRelease) { r.Cancelled.Progress = IntentPending },
		func(r *ParentRelease) { r.Cancelled.Revision = h.Input.Initial.Revision },
		func(r *ParentRelease) { r.Cancelled.Input.TargetAddress = "local:another" },
		func(r *ParentRelease) { r.Cancelled.FenceRevision = 1 },
	} {
		r := cancel
		change(&r)
		if r.ValidFor(h) {
			t.Fatal("nonterminal or different cancellation released hold")
		}
	}
	sourceScope, targetScope := token.Scope(h.Input.Initial.Admission), [32]byte{3}
	f := SourceFence{Input: h.Input.Initial.Input, FinalCut: token.FromParts(sourceScope, 5), ManifestDigest: [32]byte{1}}
	a := TargetAcceptance{Fence: f, At: token.FromParts(targetScope, 2), Revision: 1}
	d := SourceDecision{Acceptance: a, At: token.FromParts(sourceScope, 6), Revision: 7}
	installed := TargetInstallation{Decision: d, At: token.FromParts(targetScope, 3), Revision: 2}
	r := ParentRelease{Settled: SourceSettlement{Installation: installed, At: token.FromParts(sourceScope, 7), Revision: 8}}
	if !r.ValidFor(h) {
		t.Fatal("valid settled proof rejected")
	}
	for _, change := range []func(*ParentRelease){
		func(r *ParentRelease) { r.Cancelled = cancelled },
		func(r *ParentRelease) { r.Settled.At = d.At },
		func(r *ParentRelease) { r.Settled.Revision = d.Revision },
		func(r *ParentRelease) {
			r.Settled.Installation.Decision.Acceptance.Fence.Input.Final = tuple(t, id(8), 1, 1)
		},
		func(r *ParentRelease) { r.Settled.Installation.At = a.At },
	} {
		bad := r
		change(&bad)
		if bad.ValidFor(h) {
			t.Fatal("invalid completion proof released hold")
		}
	}
	c := ParentControl{Hold: h, Release: r, ReleasedAt: token.FromParts(token.Scope(h.At), token.Serial(h.At)+1), Revision: h.Revision + 1}
	if !c.Valid() {
		t.Fatal("valid released control rejected")
	}
	c.ReleasedAt = h.At
	if c.Valid() {
		t.Fatal("release did not advance parent")
	}
}
