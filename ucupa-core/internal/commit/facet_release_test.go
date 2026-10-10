package commit

import (
	"encoding/binary"
	"errors"
	"path/filepath"
	"testing"

	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	elementfact "ucupa/ucupa-core/internal/element/fact"
	facetfact "ucupa/ucupa-core/internal/facet/fact"
)

func releaseDisposition(t *testing.T, facet values.FactAuthorityRef, key values.FacetAcceptanceKey) contract.FacetDispositionRead {
	t.Helper()
	var raw [40]byte
	raw[0] = 9
	binary.BigEndian.PutUint64(raw[32:], 4)
	cut, err := values.NewFacetReadCut(raw[:])
	if err != nil {
		t.Fatal(err)
	}
	return contract.FacetDispositionRead{Found: true, Facet: facet, Element: key.Element, Key: key, Cut: cut}
}

// A locally detached relationship holds the retired-content lineage: without a
// checked final disposition the control occupancy survives refused deletions,
// and only the matching disposition retires the record and frees the Element.
func TestFacetReleaseRetiresDetachedRecordAndFreesElement(t *testing.T) {
	path := filepath.Join(t.TempDir(), "release.db")
	ids := seedFoundationStore(t, path)
	o := openFoundationStore(t, path)
	id := ids.entityID
	detached := transferFacetFixture(t, o, id, contract.FacetDetached)
	key := detached.Attachment.Key

	// A DETACHED record keeps the Element undeletable: deleting it would erase
	// the lineage the disposition protocol needs.
	entity, _ := o.state.elements.Entity(id)
	if _, err := applyFoundationStore(o, elementfact.OperationDeleteEntity, foundationStoreActor, id, contract.SpatialTuple{}, entity.Revision, "delete-detached"); !errors.Is(err, contract.ErrConflict) {
		t.Fatal("detached deletion admitted", err)
	}

	release := facetfact.Binding{Actor: foundationStoreActor, Kind: facetfact.Release, Element: id, Target: key.Facet, Address: "local:retired-facet", Expected: detached.Attachment.Revision}
	o.Lock()
	missing, code, err := o.ApplyFacetLocked("release-missing", release, contract.FacetAcceptanceEvidence{}, contract.FacetDispositionRead{})
	o.Unlock()
	if err != nil || code != facetfact.DispositionMissing || missing.Status != contract.MutationRejected {
		t.Fatal("absent disposition admitted", missing, code, err)
	}
	foreign := releaseDisposition(t, key.Facet, key)
	foreign.Key.Core = foundationStoreOtherAuthority(t, key.Core)
	o.Lock()
	conflict, code, err := o.ApplyFacetLocked("release-foreign", release, contract.FacetAcceptanceEvidence{}, foreign)
	o.Unlock()
	if err != nil || code != facetfact.StateConflict || conflict.Status != contract.MutationRejected {
		t.Fatal("disposition for another acceptance admitted", conflict, code, err)
	}

	o.Lock()
	retired, code, err := o.ApplyFacetLocked("release-committed", release, contract.FacetAcceptanceEvidence{}, releaseDisposition(t, key.Facet, key))
	o.Unlock()
	if err != nil || code != facetfact.OK || retired.Status != contract.MutationCommitted {
		t.Fatal("matching disposition rejected", retired, code, err)
	}
	if retired.Attachment.State != contract.FacetRetired || retired.Attachment.Key != key || retired.Pending != (values.FacetAcceptanceKey{}) ||
		retired.NextHop != "" || retired.NextHopRevision != (contract.FacetContactRevision{}) {
		t.Fatal("release did not retire the record", retired)
	}
	if err := validateCoreState(o.state); err != nil {
		t.Fatal("full history replay", err)
	}
	// An exact retry resolves the original committed receipt.
	o.Lock()
	again, rErr, found := o.LookupFacetLocked("release-committed", release)
	o.Unlock()
	if !found || rErr != nil || again != retired {
		t.Fatal("release retry diverged", again, rErr, found)
	}
	// The retired record is terminal for the relationship protocol.
	renew := facetfact.Binding{Actor: foundationStoreActor, Kind: facetfact.Propose, Element: id, Target: key.Facet, Address: "local:facet-again", Expected: retired.Attachment.Revision}
	o.Lock()
	_, code, err = o.ApplyFacetLocked("retired-propose", renew, contract.FacetAcceptanceEvidence{}, contract.FacetDispositionRead{})
	o.Unlock()
	if err != nil || code != facetfact.StateConflict {
		t.Fatal("retired record admitted a new proposal", code, err)
	}

	// The released occupancy no longer holds the Element: ordinary deletion
	// proceeds and removes the retired record with the Element.
	entity, _ = o.state.elements.Entity(id)
	if _, err := applyFoundationStore(o, elementfact.OperationDeleteEntity, foundationStoreActor, id, contract.SpatialTuple{}, entity.Revision, "delete-retired"); err != nil {
		t.Fatal("retired deletion refused", err)
	}
	if err := validateCoreState(o.state); err != nil {
		t.Fatal("post-delete replay", err)
	}
	before := o.state.clone()
	closeFoundationStore(t, o)
	opened := openFoundationStore(t, path)
	defer closeFoundationStore(t, opened)
	if !sameCompleteCoreState(before, opened.state) {
		t.Fatal("reopen changed the retired record")
	}
}

func TestFacetReleaseHistoryBindsDispositionEvidence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "release-history.db")
	ids := seedFoundationStore(t, path)
	o := openFoundationStore(t, path)
	t.Cleanup(func() { _ = o.CloseStore() })
	id := ids.entityID
	detached := transferFacetFixture(t, o, id, contract.FacetDetached)
	key := detached.Attachment.Key

	o.Lock()
	before := o.state.clone()
	o.Unlock()
	// A disposition that does not belong to its own Release binding is not
	// recoverable history, exactly like unbound acceptance evidence.
	proposal := facetfact.Binding{Actor: foundationStoreActor, Kind: facetfact.Propose, Element: id, Target: key.Facet, Address: "local:facet-again", Expected: detached.Attachment.Revision}
	forged, _, missing := before.prepareAttachment("forged-disposition", proposal, contract.FacetAcceptanceEvidence{}, releaseDisposition(t, key.Facet, key))
	if missing != facetfact.OK || validateOperationHistory(forged) == nil {
		t.Fatal("history accepted an unbound disposition", missing)
	}
	misbound := facetfact.Binding{Actor: foundationStoreActor, Kind: facetfact.Release, Element: id, Target: key.Facet, Address: "local:retired-facet", Expected: detached.Attachment.Revision}
	stray := releaseDisposition(t, key.Facet, key)
	stray.Facet = foundationStoreOtherAuthority(t, key.Facet)
	rewritten, _, _ := before.prepareAttachment("forged-stray-disposition", misbound, contract.FacetAcceptanceEvidence{}, stray)
	if validateOperationHistory(rewritten) == nil {
		t.Fatal("history accepted a disposition from another Facet")
	}
}
