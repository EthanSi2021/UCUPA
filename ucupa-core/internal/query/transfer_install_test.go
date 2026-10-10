package query

import (
	"encoding/binary"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	"ucupa/ucupa-core/internal/commit"
	"ucupa/ucupa-core/internal/element"
	facetfact "ucupa/ucupa-core/internal/facet/fact"
	transferfact "ucupa/ucupa-core/internal/transfer/fact"
)

func TestTransferInstalledBatchSupportsRealSnapshotPagination(t *testing.T) {
	for _, state := range []contract.FacetAttachmentState{contract.FacetNone, contract.FacetActive, contract.FacetDetached, contract.FacetRetired} {
		t.Run(map[contract.FacetAttachmentState]string{contract.FacetNone: "none", contract.FacetActive: "active", contract.FacetDetached: "detached", contract.FacetRetired: "retired"}[state], func(t *testing.T) { testTransferInstalledSnapshot(t, state) })
	}
}

func testTransferInstalledSnapshot(t *testing.T, facetState contract.FacetAttachmentState) {
	ttl := time.Hour
	source := newQueryFixture(t, contract.Config{SnapshotRetention: &ttl}, true)
	boundary, first, second, root := queryID(99100), queryID(99101), queryID(99102), queryID(99200)
	created, err := source.elements.CreateAnchor(queryActor, boundary, queryTuple(t, source.region.StartAnchorID), "outbound-anchor")
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []values.ElementID{first, second} {
		if _, err := source.elements.CreateEntity(queryActor, id, queryTuple(t, boundary), contract.OperationID(string(id))); err != nil {
			t.Fatal(err)
		}
	}
	original := prepareTransferQueryFacet(t, source, first, facetState)
	authority, err := values.NewFactAuthorityRef()
	if err != nil {
		t.Fatal(err)
	}
	target, err := commit.CreateDurable(filepath.Join(t.TempDir(), "target-query.db"), authority, [32]byte{20}, contract.RealmInitialization{Realm: "fixture-realm", NextHop: "local:fixture"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	settings, err := NormalizeConfig(contract.Config{SnapshotRetention: &ttl})
	if err != nil {
		t.Fatal(err)
	}
	authorizer := queryAuthorize(func(values.PrincipalRef, ...values.Requirement) bool { return true })
	queries := New(target, authorizer, [32]byte{24}, settings)
	elements := element.New(target, authorizer)
	defer func() { queries.Close(); _ = target.CloseStore() }()
	if _, err := elements.CreateRoot(queryActor, root, "target-root"); err != nil {
		t.Fatal(err)
	}
	region := contract.RegionQuery{StartAnchorID: root, BasisAnchorID: root, Selector: contract.QueryEntities}
	old, err := queries.QueryRegion(queryActor, region, nil, 1, nil)
	if err != nil || len(old.Matches) != 0 || old.Snapshot == nil {
		t.Fatal("old empty target", old, err)
	}
	defer queries.ReleaseReadSnapshot(queryActor, *old.Snapshot)
	i := transferfact.Intent{Profile: transferfact.SliceTransfer, Operation: "query-transfer", Actor: queryActor, Source: source.owner.AuthorityRef(), Target: authority,
		Boundary: boundary, Expected: created.Record.Anchor.Revision, Final: queryTuple(t, root), SourceAddress: "local:source", TargetAddress: "local:target"}
	oldSource, err := source.queries.QueryRegion(queryActor, source.region, nil, 1, nil)
	if err != nil || oldSource.Snapshot == nil || oldSource.Continuation == nil {
		t.Fatal("original source page", err)
	}
	defer source.queries.ReleaseReadSnapshot(queryActor, *oldSource.Snapshot)
	source.owner.Lock()
	intent, err := source.owner.BeginTransferIntentLocked(i, 4096)
	if err != nil {
		source.owner.Unlock()
		t.Fatal(err)
	}
	fenced, err := source.owner.FenceTransferSourceLocked(queryActor, i.Operation, intent.Revision, transferfact.ParentHold{})
	if err != nil {
		source.owner.Unlock()
		t.Fatal(err)
	}
	f, raw, err := source.owner.TransferSourceFenceLocked(queryActor, i.Operation)
	source.owner.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	target.Lock()
	a, err := target.AcceptTransferTargetLocked(queryActor, f, raw, nil, transferfact.ParentHold{})
	target.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	source.owner.Lock()
	d, err := source.owner.CommitTransferSourceLocked(queryActor, a, fenced.Revision)
	source.owner.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	target.Lock()
	installed, err := target.InstallTransferTargetLocked(d)
	target.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	// Target installation alone is not permission to erase the source gap.
	unsettled, err := source.queries.QueryRegion(queryActor, source.region, nil, 256, nil)
	if err != nil || unsettled.Status != contract.QueryPartial || unsettled.Gap != contract.QueryUnresolvedCoverage {
		t.Fatal("prematurely cleared source", unsettled, err)
	}
	source.owner.Lock()
	_, err = source.owner.SettleTransferSourceLocked(transferfact.CoreCompletion{Installed: transferfact.CoreInstallation{Source: installed}, At: installed.At, Revision: installed.Revision})
	source.owner.Unlock()
	if err != nil {
		t.Fatal("source settlement", err)
	}
	settled, err := source.queries.QueryRegion(queryActor, source.region, nil, 256, nil)
	if err != nil || settled.Status != contract.QueryComplete || len(settled.Matches) != len(source.ids) {
		t.Fatal("settled source cached old gap", settled, err)
	}
	_, err = source.queries.QueryRegion(queryActor, source.region, nil, 1, oldSource.Continuation)
	if !errors.Is(err, contract.ErrReadCutExpired) {
		t.Fatal("settlement revived old page", err)
	}
	if _, err := source.elements.ReadElement(queryActor, first, nil); !errors.Is(err, contract.ErrUnavailable) {
		t.Fatal("settled departure became absence", err)
	}
	// A point not present in the old snapshot remains outside that snapshot's
	// addressable records (the existing ErrInvalid rule), not a fresh read.
	oldRead, err := queries.ReadElementSnapshot(queryActor, first, *old.Snapshot)
	if !errors.Is(err, contract.ErrInvalid) || oldRead.Record != nil {
		t.Fatal("old snapshot mixed with installed batch", oldRead, err)
	}
	if rootRead, err := queries.ReadElementSnapshot(queryActor, root, *old.Snapshot); err != nil || rootRead.Record == nil {
		t.Fatal("unchanged old snapshot no longer valid", err)
	}
	page, err := queries.QueryRegion(queryActor, region, nil, 1, nil)
	if err != nil || len(page.Matches) != 1 || page.Continuation == nil || page.Snapshot == nil {
		t.Fatal("batch first page", page, err)
	}
	defer queries.ReleaseReadSnapshot(queryActor, *page.Snapshot)
	read, err := queries.ReadElementSnapshot(queryActor, first, *page.Snapshot)
	wantFacetState := contract.FacetDetached
	if facetState == contract.FacetNone {
		wantFacetState = contract.FacetNone
	}
	if facetState == contract.FacetRetired {
		// The transferred retired lineage stays terminal: the target snapshot
		// discloses RETIRED, never a fresh NONE or a re-acceptable DETACHED.
		wantFacetState = contract.FacetRetired
	}
	if err != nil || read.Record == nil || read.Attachment.State != wantFacetState || !read.Attachment.Revision.Valid() || read.Attachment.Key != original.Key || read.Attachment.NextHop != "" {
		t.Fatal("installed same-cut record", read, err)
	}
	source.owner.RLock()
	originalProposal := source.owner.SigningLocked().FacetProposal(original.Key, original.OriginalProposal)
	source.owner.RUnlock()
	if read.Attachment.OriginalProposal != originalProposal {
		t.Fatal("snapshot changed original proposal")
	}
	if facetState == contract.FacetNone || facetState == contract.FacetRetired {
		// A released retired occupancy no longer holds the Element, exactly
		// like a never-attached one: ordinary deletion proceeds.
		if _, err := elements.DeleteEntity(queryActor, first, read.Record.Entity.Revision, "after-install-delete"); err != nil {
			t.Fatal(err)
		}
	} else {
		// The transferred-in DETACHED lineage survives install: without a
		// final disposition, ordinary deletion must stay refused.
		if _, err := elements.DeleteEntity(queryActor, first, read.Record.Entity.Revision, "after-install-delete"); !errors.Is(err, contract.ErrConflict) {
			t.Fatal("detached deletion without a disposition admitted", err)
		}
		moved := queryTuple(t, root)
		moved.Range, err = contract.NewBoundedSphere(1)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := elements.UpdateEntity(queryActor, first, moved, read.Record.Entity.Revision, "after-install-update"); err != nil {
			t.Fatal(err)
		}
	}
	next, err := queries.QueryRegion(queryActor, region, nil, 1, page.Continuation)
	if err != nil || len(next.Matches) != 1 || next.Matches[0].ElementID == page.Matches[0].ElementID || next.Continuation != nil {
		t.Fatal("batch continuation", next, err)
	}
	retained, err := queries.ReadElementSnapshot(queryActor, first, *page.Snapshot)
	if err != nil || retained.Record == nil || retained.Record.Entity.Revision != read.Record.Entity.Revision || retained.Attachment != read.Attachment {
		t.Fatal("later write corrupted retained batch", err)
	}
}

func prepareTransferQueryFacet(t *testing.T, source queryFixture, id values.ElementID, state contract.FacetAttachmentState) contract.FacetAttachment {
	t.Helper()
	read, err := source.elements.ReadElement(queryActor, id, nil)
	if err != nil {
		t.Fatal(err)
	}
	if state == contract.FacetNone {
		return read.Attachment
	}
	apply := func(kind facetfact.Kind, expected values.AttachmentRevision, evidence contract.FacetAcceptanceEvidence, disposition contract.FacetDispositionRead) contract.FacetAttachmentResult {
		t.Helper()
		b := facetfact.Binding{Actor: queryActor, Kind: kind, Element: id, Expected: expected}
		if kind == facetfact.Propose || kind == facetfact.Release {
			b.Target, b.Address = values.FactAuthorityRef(queryID(99777)), "local:facet"
		}
		source.owner.Lock()
		r, code, err := source.owner.ApplyFacetLocked(contract.OperationID("snapshot-facet-"+string(rune('0'+kind))), b, evidence, disposition)
		source.owner.Unlock()
		if err != nil || code != facetfact.OK || r.Status != contract.MutationCommitted {
			t.Fatal(r, code, err)
		}
		return r
	}
	p := apply(facetfact.Propose, read.Attachment.Revision, contract.FacetAcceptanceEvidence{}, contract.FacetDispositionRead{})
	var raw [40]byte
	raw[0] = 12
	binary.BigEndian.PutUint64(raw[32:], 3)
	cut, err := values.NewFacetReadCut(raw[:])
	if err != nil {
		t.Fatal(err)
	}
	r := apply(facetfact.Activate, p.Attachment.Revision, contract.FacetAcceptanceEvidence{Key: p.Pending, AcceptedAt: cut}, contract.FacetDispositionRead{})
	if state == contract.FacetDetached || state == contract.FacetRetired {
		r = apply(facetfact.Detach, r.Attachment.Revision, contract.FacetAcceptanceEvidence{}, contract.FacetDispositionRead{})
	}
	if state == contract.FacetRetired {
		disposed, err := values.NewFacetReadCut(raw[:])
		if err != nil {
			t.Fatal(err)
		}
		r = apply(facetfact.Release, r.Attachment.Revision, contract.FacetAcceptanceEvidence{},
			contract.FacetDispositionRead{Found: true, Facet: r.Attachment.Key.Facet, Element: id, Key: r.Attachment.Key, Cut: disposed})
	}
	return r.Attachment
}
