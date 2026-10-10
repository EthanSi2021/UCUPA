package fact

import (
	"testing"

	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	mountfact "ucupa/ucupa-core/internal/mount/fact"
	"ucupa/ucupa-core/internal/token"
)

func TestClosedEdgeEvidenceIsExactTargetBoundAndImmutable(t *testing.T) {
	e, facets, mounts := tree(t)
	a, _ := e.Anchor(id(5))
	carrier := values.FactAuthorityRef("00000000-0000-4000-8000-000000000030")
	key := contract.MountKey{ParentAuthority: sourceOwner, CarrierAuthority: carrier, MountAnchorUUID: a.ID, ProposalRevision: token.MountRevisionFromParts([32]byte{1}, a.ID, 80)}
	r := mountfact.Record{State: contract.MountActive, Key: key, Admission: contract.MountAdmissionKey{MountKey: key, OperationID: "mount"},
		Revision: token.MountRevisionFromParts([32]byte{1}, a.ID, 90), Contact: mountfact.Contact{NextHop: "local:carrier", Revision: token.NextHopRevisionFromParts([32]byte{1}, key, false, 95)}}
	mounts.ReplaceOutgoing(a.ID, r)
	s, err := Select(sourceOwner, cut(), e.View, facets.View, mounts.View, a.ID, 2)
	if err != nil {
		t.Fatal(err)
	}
	f := EdgeFence{Mount: a.ID, Source: SourceFence{Input: Intent{Profile: SliceTransfer, Source: sourceOwner, Target: targetOwner,
		Operation: "move", Actor: values.PrincipalRef{Realm: "r", Principal: "p"}, Boundary: a.ID, Expected: a.Revision,
		Final: tuple(t, id(10), 0, 15), SourceAddress: "local:source", TargetAddress: "local:target"}, FinalCut: cut(), ManifestDigest: [32]byte{1}}}
	oldRange, _ := a.Range.BoundedSphere()
	c := EdgeClosure{Fence: f, Previous: mountfact.Admission{Key: r.Admission, RangeValue: oldRange, DecisionCut: token.FromParts([32]byte{1}, 90)},
		Range: f.Source.Input.Final.Range, At: token.FromParts([32]byte{3}, 5), Revision: 1}
	proofs := []EdgeClosure{c}
	accepted, err := s.WithClosedEdges(f, proofs)
	if err != nil || !accepted.edgesClosedFor(targetOwner) || accepted.edgesClosedFor(carrier) || s.edgesClosedFor(targetOwner) {
		t.Fatal("target-bound evidence", err)
	}
	proofs[0].Revision++
	read := accepted.ClosedEdges()
	if read[0] != c {
		t.Fatal("borrowed input")
	}
	read[0].Revision++
	if accepted.ClosedEdges()[0] != c {
		t.Fatal("writable proof projection")
	}
	for _, mode := range []string{"wrong-cut", "wrong-source", "wrong-boundary", "wrong-profile", "extra"} {
		t.Run(mode, func(t *testing.T) {
			bad, proof := f, c
			switch mode {
			case "wrong-cut":
				bad.Source.FinalCut = token.FromParts([32]byte{1}, 99)
			case "wrong-source":
				bad.Source.Input.Source = carrier
			case "wrong-boundary":
				bad.Mount = id(2)
			case "wrong-profile":
				bad.Source.Input.Profile = RootSuccessor
			case "extra":
				proof.Fence.Mount = id(2)
			}
			if _, err := s.WithClosedEdges(bad, []EdgeClosure{proof}); err == nil {
				t.Fatal("mismatched common evidence accepted")
			}
		})
	}
}
