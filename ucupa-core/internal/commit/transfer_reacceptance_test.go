package commit

import (
	"encoding/binary"
	"errors"
	"testing"

	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	elementfact "ucupa/ucupa-core/internal/element/fact"
	facetfact "ucupa/ucupa-core/internal/facet/fact"
	"ucupa/ucupa-core/internal/token"
	transferfact "ucupa/ucupa-core/internal/transfer/fact"
)

// A completed Core transfer keeps the stored content DETACHED with predecessor
// evidence. This drives the whole owner journal: a re-acceptance proposal is
// a durable obligation that ordinary cancellation, Element deletion and
// repeated Core transfer must not erase, and activation alone settles it and
// renews the acceptance key for further transfers.
func TestTransferReacceptanceObligationAndActivation(t *testing.T) {
	a, _, initial := transferIntentFixture(t)
	b := transferReceiver(t, foundationStoreID(999))
	id := foundationStoreID(102)
	transferFacetFixture(t, a, id, contract.FacetActive)
	f, raw := outgoingTransfer(t, a, b, initial.Boundary, foundationStoreID(999), "a-b")
	completeFencedTransfer(t, a, b, f, raw)

	installed, ok := b.state.attachments.facts.Record(id)
	if !ok || installed.Attachment.State != contract.FacetDetached ||
		!installed.Attachment.OriginalProposal.Valid() || installed.ReacceptancePending() {
		t.Fatal("installed record lost its predecessor", installed)
	}
	predecessor := installed.Attachment.Key

	b.Lock()
	proposed, code, err := b.ApplyFacetLocked("re-accept-propose",
		facetfact.Binding{Actor: foundationStoreActor, Kind: facetfact.Propose, Element: id, Target: predecessor.Facet, Address: "local:original-facet", Expected: installed.Attachment.Revision},
		contract.FacetAcceptanceEvidence{}, contract.FacetDispositionRead{})
	b.Unlock()
	if err != nil || code != facetfact.OK || proposed.Status != contract.MutationCommitted {
		t.Fatal("same-Facet re-proposal rejected", proposed, code, err)
	}
	if !proposed.Pending.Valid() || proposed.Pending.Core != b.authority || proposed.Pending.Facet != predecessor.Facet ||
		proposed.Pending.Proposal != proposed.Attachment.Revision {
		t.Fatal("re-proposal lost its new owner", proposed)
	}
	pending, _ := b.state.attachments.facts.Record(id)
	if !pending.ReacceptancePending() {
		t.Fatal("open re-acceptance obligation not detected")
	}

	// Ordinary cancellation must not erase the obligation.
	b.Lock()
	_, code, err = b.ApplyFacetLocked("re-accept-cancel",
		facetfact.Binding{Actor: foundationStoreActor, Kind: facetfact.Cancel, Element: id, Expected: proposed.Attachment.Revision},
		contract.FacetAcceptanceEvidence{}, contract.FacetDispositionRead{})
	b.Unlock()
	if err != nil || code != facetfact.StateConflict {
		t.Fatal("ordinary cancellation erased the obligation", code, err)
	}

	// Ordinary Element deletion must not erase the obligation either.
	entity, exists := b.state.elements.Entity(id)
	if !exists {
		t.Fatal("pending Element vanished")
	}
	if _, err := applyFoundationStore(b, elementfact.OperationDeleteEntity, foundationStoreActor, id, contract.SpatialTuple{}, entity.Revision, "delete-pending"); !errors.Is(err, contract.ErrConflict) {
		t.Fatal("ordinary deletion erased the obligation", err)
	}

	// A repeated Core transfer is refused while the obligation is open: the
	// pending attachment keeps the Element out of every transfer capture.
	blockedTarget, err := values.NewFactAuthorityRef()
	if err != nil {
		t.Fatal(err)
	}
	anchor, _ := b.state.elements.Anchor(initial.Boundary)
	blocked := transferfact.Intent{Profile: transferfact.SliceTransfer, Operation: "b-c-blocked", Actor: foundationStoreActor, Source: b.authority,
		Target: blockedTarget, Boundary: initial.Boundary, Expected: anchor.Revision, Final: foundationStoreTuple(t, foundationStoreID(1999), 0, 20),
		SourceAddress: "local:source", TargetAddress: "local:target"}
	if _, err := beginTransfer(t, b, blocked); !errors.Is(err, contract.ErrAttachmentStateConflict) {
		t.Fatal("repeated Core transfer erased the obligation", err)
	}

	// Activation alone settles the obligation with the new acceptance key.
	var evidenceRaw [40]byte
	evidenceRaw[0] = 7
	binary.BigEndian.PutUint64(evidenceRaw[32:], 2)
	acceptedAt, err := values.NewFacetReadCut(evidenceRaw[:])
	if err != nil {
		t.Fatal(err)
	}
	b.Lock()
	activated, code, err := b.ApplyFacetLocked("re-accept-activate",
		facetfact.Binding{Actor: foundationStoreActor, Kind: facetfact.Activate, Element: id, Expected: proposed.Attachment.Revision},
		contract.FacetAcceptanceEvidence{Key: proposed.Pending, AcceptedAt: acceptedAt}, contract.FacetDispositionRead{})
	b.Unlock()
	if err != nil || code != facetfact.OK || activated.Status != contract.MutationCommitted {
		t.Fatal("re-acceptance activation rejected", activated, code, err)
	}
	if activated.Attachment.State != contract.FacetActive || activated.Attachment.Key != proposed.Pending ||
		activated.Attachment.OriginalProposal != (values.PublicToken{}) {
		t.Fatal("activation did not renew the acceptance key", activated)
	}
	settled, _ := b.state.attachments.facts.Record(id)
	if settled.ReacceptancePending() {
		t.Fatal("activated record still reports an open obligation")
	}

	// The settled record no longer blocks Core transfer preflight.
	b.Lock()
	_, err = b.PreflightTransferSourceLocked(initial.Boundary, anchor.Revision, 100)
	b.Unlock()
	if err != nil {
		t.Fatal("settled obligation still blocks Core transfer", err)
	}

	// The renewed key continues the transfer chain: the next Core captures
	// the new owner's own signed proposal comparison.
	c := transferReceiver(t, foundationStoreID(1999))
	f2, raw2 := outgoingTransfer(t, b, c, initial.Boundary, foundationStoreID(1999), "b-c-renewed")
	completeFencedTransfer(t, b, c, f2, raw2)
	renewed, ok := c.state.attachments.facts.Record(id)
	if !ok || renewed.Attachment.State != contract.FacetDetached || renewed.Attachment.Key != proposed.Pending {
		t.Fatal("continued transfer lost the renewed key", renewed)
	}
	wantComparison := token.NewPublicSigner(b.state.publicKey, b.authority).FacetProposal(proposed.Pending, values.PublicToken{})
	if renewed.Attachment.OriginalProposal != wantComparison {
		t.Fatal("continued transfer did not capture the new owner's proposal", renewed)
	}

	for _, owner := range []*Owner{a, b, c} {
		if err := validateCoreState(owner.state); err != nil {
			t.Fatal("replay", err)
		}
		path, before := owner.store.db.Path(), owner.state.clone()
		closeFoundationStore(t, owner)
		reopened := openFoundationStore(t, path)
		if !sameCompleteCoreState(before, reopened.state) {
			t.Fatal("restart changed the re-acceptance record")
		}
		_ = reopened.CloseStore()
	}
}
