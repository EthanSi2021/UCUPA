package commit

import (
	"strings"
	"testing"
	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	"ucupa/ucupa-core/internal/token"
	transferencoding "ucupa/ucupa-core/internal/transfer/encoding"
	transferfact "ucupa/ucupa-core/internal/transfer/fact"
)

func TestCarrierBindingAndInstallationEnvelopeFitsReservedCompletionRoom(t *testing.T) {
	_, _, _, b := boundCarrierFixture(t, false)
	c := &b.Acceptance.Fence.Permit
	r := &c.Reservation
	scope := token.Scope(r.At)
	max := ^uint64(0)
	r.Input.Operation = contract.OperationID(strings.Repeat("x", 128))
	r.Input.ExpectedElement = token.RevisionFromParts(scope, r.Input.Mount, max-4)
	r.Input.ExpectedMount = token.MountRevisionFromParts(scope, r.Input.Mount, max-4)
	r.Previous.OperationID = contract.OperationID(strings.Repeat("p", 128))
	r.Previous.MountKey.ProposalRevision = token.MountRevisionFromParts(scope, r.Input.Mount, max-5)
	r.PreviousContact = token.NextHopRevisionFromParts(scope, r.Previous.MountKey, false, max-4)
	r.At = token.FromParts(scope, max-3)
	r.NextKey.ProposalRevision = token.MountRevisionFromParts(scope, r.Input.Mount, max-3)
	c.PermitAt = token.FromParts(scope, max-2)
	b.At = token.FromParts(scope, max-1)
	b.Revision = max
	b.Acceptance.At = token.FromParts(token.Scope(b.Acceptance.At), max-1)
	b.Acceptance.Revision = max - 1
	b.Acceptance.Fence.FinalCut = token.FromParts(token.Scope(b.Acceptance.Fence.FinalCut), max)
	b.Acceptance.Fence.Revision = max
	for index := range b.Acceptance.Fence.ManifestDigest {
		b.Acceptance.Fence.ManifestDigest[index] = 255
	}
	for n := 1; n <= transferencoding.MaxTransferIntentBytes; n++ {
		trial := r.Input
		trial.Actor.Principal = values.PrincipalID(strings.Repeat("p", n))
		if _, err := transferencoding.EncodeCarrierIntent(trial); err != nil {
			break
		}
		r.Input = trial
	}
	if !b.Valid() {
		t.Fatal("bad longest receipt fixture")
	}
	for _, event := range []transferencoding.TransferEvent{
		{Kind: transferencoding.TransferCarrierBound, At: b.At, CarrierAcceptance: b.Acceptance},
		{Kind: transferencoding.TransferCarrierInstalled, At: b.Acceptance.At, CarrierBinding: b},
		{Kind: transferencoding.TransferCarrierParentSettled, At: b.At, CarrierInstallation: transferfact.CarrierInstallation{Binding: b, At: token.FromParts(token.Scope(b.Acceptance.At), max), Revision: max}},
		{Kind: transferencoding.TransferCarrierSourceSettled, At: b.Acceptance.Fence.FinalCut, CarrierInstallation: transferfact.CarrierInstallation{Binding: b, At: token.FromParts(token.Scope(b.Acceptance.At), max), Revision: max}},
		{Kind: transferencoding.TransferEdgeInstalled, At: b.Acceptance.Fence.FinalCut, EdgeMount: foundationStoreID(110), EdgeParent: transferfact.CoreInstallation{Carrier: transferfact.CarrierInstallation{Binding: b, At: token.FromParts(token.Scope(b.Acceptance.At), max), Revision: max}}},
	} {
		if transferencoding.IsTransferSettlement(event.Kind) {
			i := event.CarrierInstallation
			event.Completion = transferfact.CoreCompletion{Installed: transferfact.CoreInstallation{Carrier: i}, At: i.At, Revision: i.Revision}
		}
		data, err := transferencoding.EncodeTransferJournal([]transferencoding.TransferEvent{event})
		if err != nil || len(data) > 2*transferencoding.MaxTransferIntentBytes {
			t.Fatal("terminal event exceeds admitted headroom", event.Kind, len(data), err)
		}
		decoded, err := transferencoding.LoadTransferJournal(data, token.Scope(event.At))
		if err != nil || len(decoded) != 1 || decoded[0] != event {
			t.Fatal("longest evidence roundtrip", err)
		}
	}
}
