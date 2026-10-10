package commit

import (
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"testing"
	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	elementfact "ucupa/ucupa-core/internal/element/fact"
	"ucupa/ucupa-core/internal/token"
	transferencoding "ucupa/ucupa-core/internal/transfer/encoding"
	transferfact "ucupa/ucupa-core/internal/transfer/fact"
)

func emptyRootReceiver(t *testing.T) *Owner {
	t.Helper()
	o := createFoundationStore(t, filepath.Join(t.TempDir(), "root-receiver.db"))
	t.Cleanup(func() { _ = o.CloseStore() })
	return o
}

func rootTransferIntent(t *testing.T, source, target *Owner, op contract.OperationID) transferfact.Intent {
	t.Helper()
	root, ok := source.state.elements.Anchor(foundationStoreID(100))
	if !ok || !root.Root() {
		t.Fatal("missing source root")
	}
	return transferfact.Intent{Profile: transferfact.RootSuccessor, Operation: op, Actor: foundationStoreActor,
		Source: source.authority, Target: target.authority, Boundary: root.ID, Expected: root.Revision,
		SourceAddress: "local:root-source", TargetAddress: "local:root-target"}
}

func TestRootTransferCompleteSpaceIdentityAndReturn(t *testing.T) {
	for _, state := range []contract.FacetAttachmentState{contract.FacetNone, contract.FacetActive, contract.FacetDetached, contract.FacetRetired} {
		t.Run(fmt.Sprint(state), func(t *testing.T) {
			a, _, _ := transferIntentFixture(t)
			b := emptyRootReceiver(t)
			rootID, childID := foundationStoreID(100), foundationStoreID(102)
			facet := transferFacetFixture(t, a, rootID, state)
			proof := token.NewPublicSigner(a.state.publicKey, a.authority).FacetProposal(facet.Attachment.Key, facet.Attachment.OriginalProposal)
			original := a.state.clone()
			oldView, oldCut := a.state.elements.View, a.state.currentCut()
			for index, step := range []struct {
				source, target *Owner
				op             contract.OperationID
			}{{a, b, "root-a-b"}, {b, a, "root-b-a"}} {
				i := rootTransferIntent(t, step.source, step.target, step.op)
				f, raw := frozenSource(t, step.source, i)
				if step.source.state.elements.RootExists() || step.source.state.elements.Counts().Anchors != 0 || step.source.state.elements.Counts().Entities != 0 {
					t.Fatal("root fence left live space")
				}
				if root, ok := step.source.state.elements.RetiredRoot(); !ok || root != rootID {
					t.Fatal("lost retired root identity")
				}
				if err := step.source.state.guardTransferRootActivation(); !errors.Is(err, values.PublicUnavailable) {
					t.Fatal("retired source accepts new root", err)
				}
				if _, err := applyFoundationStore(step.source, elementfact.OperationCreateRoot, foundationStoreActor, foundationStoreID(3000), contract.SpatialTuple{}, contract.Revision{}, "not-a-new-space"); !errors.Is(err, values.PublicUnavailable) {
					t.Fatal("CreateRoot bypassed retired root", err)
				}
				selected, err := transferencoding.DecodeTransferManifest(f, raw)
				if err != nil || selected.Profile() != transferfact.RootSuccessor || len(selected.Ancestors()) != 0 || len(selected.Members()) != 3 {
					t.Fatal("root evidence", err)
				}
				for _, m := range selected.Members() {
					if len(selected.Lineage(m.ID())) != index {
						t.Fatal("root lineage discontinuity")
					}
				}
				before := step.target.state.clone()
				accepted, err := acceptTransfer(t, step.target, f, raw)
				if err != nil {
					t.Fatal("accept", err)
				}
				if step.target.state.elements.RootExists() || step.target.state.cutSerial != before.cutSerial {
					t.Fatal("acceptance exposed partial root")
				}
				if err := step.target.state.guardTransferRootActivation(); !errors.Is(err, contract.ErrConflict) && !errors.Is(err, values.PublicUnavailable) {
					t.Fatal("root reservation missing", err)
				}
				decision, err := commitSuccessor(step.source, accepted, step.source.state.transfers.records[i.Operation].FenceRevision)
				if err != nil {
					t.Fatal("decide", err)
				}
				installed, err := installTransfer(step.target, decision)
				if err != nil {
					t.Fatal("install", err)
				}
				if step.target.state.cutSerial != before.cutSerial+1 || step.target.state.revisionSerial != before.revisionSerial+3 {
					t.Fatal("not one atomic target cut")
				}
				root, ok := step.target.state.elements.Anchor(rootID)
				if !ok || !root.Root() || root.Parent != "" || root.LocalPose != (contract.Pose{}) || token.RevisionScope(root.Revision) != step.target.state.scope {
					t.Fatal("root identity or shape changed")
				}
				if _, retired := step.target.state.elements.RetiredRoot(); retired {
					t.Fatal("return left retired root alongside active root")
				}
				child, _ := step.target.state.elements.Entity(childID)
				oldChild, _ := original.elements.Entity(childID)
				if child.Parent != oldChild.Parent || child.LocalPose != oldChild.LocalPose || child.Range != oldChild.Range {
					t.Fatal("descendant rewritten")
				}
				current, _ := step.target.state.attachments.facts.Record(rootID)
				want := contract.FacetDetached
				if state == contract.FacetNone {
					want = contract.FacetNone
				}
				if state == contract.FacetRetired {
					want = contract.FacetRetired
				}
				if current.Attachment.State != want || current.Attachment.Key != facet.Attachment.Key || current.Attachment.OriginalProposal != proof || current.NextHop != "" {
					t.Fatal("root Facet predecessor changed")
				}
				if _, err := settleTransfer(step.source, installed); err != nil {
					t.Fatal("settle", err)
				}
				if retry, err := installTransfer(step.target, decision); err != nil || retry != installed {
					t.Fatal("install retry", err)
				}
				for _, o := range []*Owner{step.source, step.target} {
					if err := validateCoreState(o.state); err != nil {
						t.Fatal("root replay", err)
					}
				}
			}
			if !reflect.DeepEqual(a.state.operations, original.operations) {
				t.Fatal("root return rewrote original outcomes")
			}
			if err := a.CheckTransferReadLocked(oldView, oldCut, rootID, childID); !errors.Is(err, contract.ErrReadCutExpired) {
				t.Fatal("return resurrected old read context", err)
			}
			for _, o := range []*Owner{a, b} {
				path, before := o.store.db.Path(), o.state.clone()
				closeFoundationStore(t, o)
				reopened := openFoundationStore(t, path)
				if !sameCompleteCoreState(before, reopened.state) {
					t.Fatal("restart changed root or original outcomes")
				}
				closeFoundationStore(t, reopened)
			}
		})
	}
}

func TestRootTransferRejectsOccupiedTargetAndProfileConfusion(t *testing.T) {
	source, _, _ := transferIntentFixture(t)
	target := emptyRootReceiver(t)
	i := rootTransferIntent(t, source, target, "root")
	for _, mutate := range []func(*transferfact.Intent){
		func(i *transferfact.Intent) {
			i.Profile = transferfact.SliceTransfer
			i.Final = foundationStoreTuple(t, foundationStoreID(999), 0, 20)
		},
		func(i *transferfact.Intent) {
			a, _ := source.state.elements.Anchor(foundationStoreID(101))
			i.Boundary, i.Expected = a.ID, a.Revision
		},
		func(i *transferfact.Intent) {
			e, _ := source.state.elements.Entity(foundationStoreID(102))
			i.Boundary, i.Expected = e.ID, e.Revision
		},
		func(i *transferfact.Intent) { i.Final = foundationStoreTuple(t, foundationStoreID(999), 0, 20) },
		func(i *transferfact.Intent) { i.Profile = 0 },
	} {
		input, before := i, source.state.clone()
		mutate(&input)
		if _, err := beginTransfer(t, source, input); err == nil {
			t.Fatal("accepted mismatched root profile")
		}
		if !sameCompleteCoreState(before, source.state) {
			t.Fatal("rejected profile claimed operation")
		}
	}
	// The target creating its own root after source preparation cannot be
	// overwritten, even if it has the same UUID as the outgoing root.
	for _, id := range []values.ElementID{foundationStoreID(999), i.Boundary} {
		occupied := transferReceiver(t, id)
		s, _, _ := transferIntentFixture(t)
		input := rootTransferIntent(t, s, occupied, "occupied-root")
		f, raw := frozenSource(t, s, input)
		before := occupied.state.clone()
		if _, err := acceptTransfer(t, occupied, f, raw); err == nil {
			t.Fatal("overwrote independent root")
		}
		if !sameCompleteCoreState(before, occupied.state) {
			t.Fatal("occupied rejection left state")
		}
	}
}
