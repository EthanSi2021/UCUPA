package commit

import (
	"encoding/binary"
	"testing"

	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	facetfact "ucupa/ucupa-core/internal/facet/fact"
	"ucupa/ucupa-core/internal/token"
)

// Drive the actual sole-owner decisions and journal, not a synthetic attachment
// injected into live state. Facet acceptance is the finite P4 peer evidence;
// this P7.2 fixture does not transfer or manufacture Facet namespace content.
func transferFacetFixture(t *testing.T, o *Owner, id values.ElementID, state contract.FacetAttachmentState) facetfact.Record {
	t.Helper()
	if state != contract.FacetNone {
		apply := func(kind facetfact.Kind, operation contract.OperationID) {
			t.Helper()
			r, _ := o.state.attachments.facts.Record(id)
			b := facetfact.Binding{Actor: foundationStoreActor, Kind: kind, Element: id, Expected: r.Attachment.Revision}
			var evidence contract.FacetAcceptanceEvidence
			var disposition contract.FacetDispositionRead
			if kind == facetfact.Propose {
				b.Target, b.Address = values.FactAuthorityRef(foundationStoreID(789)), "local:original-facet"
			} else if kind == facetfact.Activate {
				var raw [40]byte
				raw[0] = 7
				binary.BigEndian.PutUint64(raw[32:], 2)
				cut, err := values.NewFacetReadCut(raw[:])
				if err != nil {
					t.Fatal(err)
				}
				evidence = contract.FacetAcceptanceEvidence{Key: r.Pending, AcceptedAt: cut}
			} else if kind == facetfact.Release {
				b.Target, b.Address = r.Attachment.Key.Facet, "local:original-facet"
				disposition = releaseDisposition(t, r.Attachment.Key.Facet, r.Attachment.Key)
			}
			o.Lock()
			result, code, err := o.ApplyFacetLocked(operation, b, evidence, disposition)
			o.Unlock()
			if err != nil || code != facetfact.OK || result.Status != contract.MutationCommitted {
				t.Fatal(kind, result, code, err)
			}
		}
		apply(facetfact.Propose, "facet-propose")
		apply(facetfact.Activate, "facet-activate")
		if state == contract.FacetDetached || state == contract.FacetRetired {
			apply(facetfact.Detach, "facet-detach")
		}
		if state == contract.FacetRetired {
			apply(facetfact.Release, "facet-release")
		}
	}
	r, ok := o.state.attachments.facts.Record(id)
	if !ok || r.Attachment.State != state {
		t.Fatal("source relationship", r)
	}
	return r
}

func TestTransferFacetPredecessorDurableAcrossOwnersAndReturn(t *testing.T) {
	for _, state := range []contract.FacetAttachmentState{contract.FacetNone, contract.FacetActive, contract.FacetDetached, contract.FacetRetired} {
		t.Run(map[contract.FacetAttachmentState]string{contract.FacetNone: "none", contract.FacetActive: "active", contract.FacetDetached: "detached", contract.FacetRetired: "retired"}[state], func(t *testing.T) {
			a, _, initial := transferIntentFixture(t)
			b, c := transferReceiver(t, foundationStoreID(999)), transferReceiver(t, foundationStoreID(1999))
			id := foundationStoreID(102)
			original := transferFacetFixture(t, a, id, state)
			expectedProposal := token.NewPublicSigner(a.state.publicKey, a.authority).FacetProposal(original.Attachment.Key, original.Attachment.OriginalProposal)
			root, _ := a.state.elements.ParentOf(initial.Boundary)
			steps := []struct {
				source, target *Owner
				parent         values.ElementID
				op             contract.OperationID
			}{
				{a, b, foundationStoreID(999), "facet-a-b"}, {b, c, foundationStoreID(1999), "facet-b-c"}, {c, a, root, "facet-c-a"},
			}
			for _, step := range steps {
				f, raw := outgoingTransfer(t, step.source, step.target, initial.Boundary, step.parent, step.op)
				if _, present := step.source.state.attachments.facts.Record(id); present {
					t.Fatal("fenced source still exposes current relationship")
				}
				installed := completeFencedTransfer(t, step.source, step.target, f, raw)
				r, ok := step.target.state.attachments.facts.Record(id)
				want := contract.FacetDetached
				if state == contract.FacetNone {
					want = contract.FacetNone
				}
				if state == contract.FacetRetired {
					// A retired lineage is terminal: the transfer carries it as
					// RETIRED so the target can never re-propose or re-release it.
					want = contract.FacetRetired
				}
				if !ok || r.Attachment.State != want || r.Attachment.Key != original.Attachment.Key || r.Attachment.OriginalProposal != expectedProposal {
					t.Fatal("lost exact predecessor", r)
				}
				if r.Key().Valid() || r.Pending.Valid() || r.NextHop != "" || r.Attachment.NextHop != "" || r.Contact != (contract.FacetContactRevision{}) || r.Attachment.NextHopRevision != (contract.FacetContactRevision{}) {
					t.Fatal("old contact or live key reactivated", r)
				}
				if r.Attachment.Revision != facetfact.AttachmentRevisionForCut(id, installed.At) {
					t.Fatal("did not issue current target relationship revision")
				}
				public := facetfact.PublicInspection(token.NewPublicSigner(step.target.state.publicKey, step.target.authority), id, r.Result(installed.At, contract.MutationUnchanged))
				if public.Revision.Owner != step.target.authority || public.Proposal != expectedProposal || public.KeyCore != original.Attachment.Key.Core || public.NextHop != "" || public.ContactRevision.Valid() {
					t.Fatal("mixed public owners", public)
				}
				if state == contract.FacetRetired {
					// The transferred retired lineage stays terminal at the new
					// Core: it can never become a fresh proposal again, and a
					// second release has nothing left to release.
					renew := facetfact.Binding{Actor: foundationStoreActor, Kind: facetfact.Propose, Element: id, Target: original.Attachment.Key.Facet, Address: "local:next-facet", Expected: r.Attachment.Revision}
					step.target.Lock()
					_, code, err := step.target.ApplyFacetLocked("retired-transfer-propose", renew, contract.FacetAcceptanceEvidence{}, contract.FacetDispositionRead{})
					step.target.Unlock()
					if err != nil || code != facetfact.StateConflict {
						t.Fatal("transferred retired lineage admitted a new proposal", code, err)
					}
					rerelease := facetfact.Binding{Actor: foundationStoreActor, Kind: facetfact.Release, Element: id, Target: original.Attachment.Key.Facet, Address: "local:next-facet", Expected: r.Attachment.Revision}
					step.target.Lock()
					_, code, err = step.target.ApplyFacetLocked("retired-transfer-release", rerelease, contract.FacetAcceptanceEvidence{}, releaseDisposition(t, original.Attachment.Key.Facet, original.Attachment.Key))
					step.target.Unlock()
					if err != nil || code != facetfact.StateConflict {
						t.Fatal("transferred retired lineage admitted a second release", code, err)
					}
				}
				if _, err := installTransfer(step.target, installed.Decision); err != nil {
					t.Fatal("retry", err)
				}
				if err := validateCoreState(step.target.state); err != nil {
					t.Fatal("target replay", err)
				}
				if err := validateCoreState(step.source.state); err != nil {
					t.Fatal("source replay", err)
				}
			}
			for _, owner := range []*Owner{a, b, c} {
				path, before := owner.store.db.Path(), owner.state.clone()
				closeFoundationStore(t, owner)
				opened := openFoundationStore(t, path)
				if !sameCompleteCoreState(before, opened.state) {
					t.Fatal("reopen changed predecessor")
				}
				closeFoundationStore(t, opened)
			}
		})
	}
}
