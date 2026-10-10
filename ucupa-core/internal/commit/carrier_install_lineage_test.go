package commit

import (
	"testing"
	"ucupa/ucupa-core/contract"
	elementfact "ucupa/ucupa-core/internal/element/fact"
	transferencoding "ucupa/ucupa-core/internal/transfer/encoding"
	transferfact "ucupa/ucupa-core/internal/transfer/fact"
)

func TestCarrierInstallationKeepsNestedSpaceFacetPredecessorAndNextTransferLineage(t *testing.T) {
	p, s, d, i := carrierControlFixture(t)
	anchor, child := foundationStoreID(106), foundationStoreID(107)
	if _, err := applyFoundationStore(s, elementfact.OperationCreateAnchor, i.Actor, anchor, foundationStoreTuple(t, i.Mount, 5, 10), contract.Revision{}, "nested-anchor"); err != nil {
		t.Fatal(err)
	}
	if _, err := applyFoundationStore(s, elementfact.OperationCreateEntity, i.Actor, child, foundationStoreTuple(t, anchor, 1, 1), contract.Revision{}, "nested-child"); err != nil {
		t.Fatal(err)
	}
	predecessor := transferFacetFixture(t, s, child, contract.FacetActive)
	c, err := reserveCarrier(t, p, i)
	if err != nil {
		t.Fatal(err)
	}
	c, err = advanceCarrier(t, p, c, true)
	if err != nil {
		t.Fatal(err)
	}
	f, raw, err := fenceCarrier(s, c)
	if err != nil {
		t.Fatal(err)
	}
	original, err := transferencoding.DecodeCarrierManifest(f, raw)
	if err != nil {
		t.Fatal(err)
	}
	a, err := acceptCarrier(d, f, raw)
	if err != nil {
		t.Fatal(err)
	}
	b, err := bindCarrier(p, a)
	if err != nil {
		t.Fatal(err)
	}
	installed, err := installCarrier(d, b)
	if err != nil {
		t.Fatal(err)
	}
	if len(original.Members()) != 3 || d.state.elements.Counts().Anchors != 1 || d.state.elements.Counts().Entities != 2 {
		t.Fatal("incomplete closure")
	}
	for _, m := range original.Members() {
		if m.Kind == contract.ElementAnchor {
			got, ok := d.state.elements.Anchor(m.ID())
			if !ok || got.Revision == m.Anchor.Revision {
				t.Fatal("missing target-issued anchor")
			}
			got.Revision = m.Anchor.Revision
			if got != m.Anchor {
				t.Fatal("rewrote anchor tuple")
			}
		} else {
			got, ok := d.state.elements.Entity(m.ID())
			if !ok || got.Revision == m.Entity.Revision {
				t.Fatal("missing target-issued entity")
			}
			got.Revision = m.Entity.Revision
			if got != m.Entity {
				t.Fatal("rewrote entity tuple")
			}
		}
	}
	r, ok := d.state.attachments.facts.Record(child)
	if !ok || r.Attachment.State != contract.FacetDetached || r.Attachment.Key != predecessor.Attachment.Key || r.Key().Valid() || r.NextHop != "" {
		t.Fatal("transferred predecessor as an active Facet")
	}
	// Capture the carried space through its actual external-parent profile.
	// This checks the next source evidence, not completion of another transfer:
	// the parent's installation settlement is a separate pending slice.
	external := ElementExternalParentForView(d.state.mount.facts.View)
	next, err := transferfact.SelectCarrier(d.authority, d.state.currentCut(), d.state.revisionSerial, d.state.elements.View, d.state.attachments.facts.View, d.state.mount.facts.View, external, transferencoding.MaxTransferManifestRecords)
	if err == nil {
		next, err = d.state.captureTransferEvidence(next)
	}
	if err != nil {
		t.Fatal("new source capture", err)
	}
	chain := next.Lineage(child)
	if len(chain) != 1 || chain[0].Controller != p.authority || chain[0].Source != s.authority || chain[0].Target != d.authority || chain[0].InstalledCut != installed.At || chain[0].Operation != i.Operation {
		t.Fatal("lost parent-controlled lineage", chain)
	}
	var old transferfact.Member
	for _, m := range original.Members() {
		if m.ID() == child {
			old = m
		}
	}
	want, err := installedCarrierLineageStep(old, installed)
	if err != nil || chain[0] != want {
		t.Fatal("wrong exact lineage", err)
	}
	if err := validateCoreState(d.state); err != nil {
		t.Fatal("batch history", err)
	}
	for _, o := range []*Owner{p, s, d} {
		path, before := o.store.db.Path(), o.state.clone()
		closeFoundationStore(t, o)
		opened := openFoundationStore(t, path)
		if !sameCompleteCoreState(before, opened.state) {
			t.Fatal("reopen changed carrier lineage")
		}
		if opened.authority == d.authority {
			last, err := opened.state.captureTransferLineage(next)
			if err != nil || len(last.Lineage(child)) != 1 || last.Lineage(child)[0] != want {
				t.Fatal("replay lost source lineage evidence", err)
			}
		}
		closeFoundationStore(t, opened)
	}
}
