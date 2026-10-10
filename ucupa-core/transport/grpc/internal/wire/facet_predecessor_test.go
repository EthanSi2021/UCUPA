package wire

import (
	"testing"

	"google.golang.org/protobuf/proto"
	"ucupa/shared/values"
	pb "ucupa/ucupa-core/bindings/go/protobuf"
	"ucupa/ucupa-core/contract"
)

func TestFacetPredecessorPreservesSeparateKeyOwnerThroughProtobuf(t *testing.T) {
	current := values.FactAuthorityRef("00000000-0000-4000-8000-000000000901")
	former := values.FactAuthorityRef("00000000-0000-4000-8000-000000000902")
	facet := values.FactAuthorityRef("00000000-0000-4000-8000-000000000903")
	revision, _ := values.PublicTokenFromBytes(current, "attachment-revision", [32]byte{4})
	proposal, _ := values.PublicTokenFromBytes(former, "attachment-revision", [32]byte{5})
	x := contract.PublicFacetInspection{Element: values.ElementID("00000000-0000-4000-8000-000000000904"), State: contract.FacetDetached, Revision: revision, Target: facet, KeyCore: former, Proposal: proposal}
	w := PublicFacetInspectionWire(x)
	if w.NextHop != nil || w.ContactRevision != nil || w.PendingProposal != nil {
		t.Fatal("detached relation serialized live contact")
	}
	raw, err := proto.Marshal(w)
	if err != nil {
		t.Fatal(err)
	}
	var decoded pb.PublicFacetInspection
	if err := proto.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	back, err := FederationAttachment(&decoded)
	if err != nil || back != x {
		t.Fatal("wire changed historical owner or comparison", back, err)
	}
}
