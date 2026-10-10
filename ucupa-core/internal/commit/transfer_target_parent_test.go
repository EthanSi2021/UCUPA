package commit

import (
	"errors"
	"strings"
	"testing"
	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	elementfact "ucupa/ucupa-core/internal/element/fact"
	"ucupa/ucupa-core/internal/token"
	transferencoding "ucupa/ucupa-core/internal/transfer/encoding"
	transferfact "ucupa/ucupa-core/internal/transfer/fact"
)

func targetParentCase(t *testing.T, shape string) (*Owner, *Owner, *Owner, transferfact.ParentHold, transferfact.SourceFence, string) {
	t.Helper()
	p, source, target, pi := transferParentFixture(t)
	if shape != "direct-entity" {
		i := pi.Initial.Input
		if _, err := cancelTransfer(t, source, i, pi.Initial.Revision); err != nil {
			t.Fatal(err)
		}
		i.Operation = contract.OperationID(shape)
		if strings.HasSuffix(shape, "anchor") {
			anchor, child := foundationStoreID(202), foundationStoreID(203)
			if _, err := applyFoundationStore(source, elementfact.OperationCreateAnchor, i.Actor, anchor,
				foundationStoreTuple(t, foundationStoreID(200), 0, 3), contract.Revision{}, "target-source-anchor"); err != nil {
				t.Fatal(err)
			}
			if _, err := applyFoundationStore(source, elementfact.OperationCreateEntity, i.Actor, child,
				foundationStoreTuple(t, anchor, 0, 1), contract.Revision{}, "target-source-child"); err != nil {
				t.Fatal(err)
			}
			a, _ := source.state.elements.Anchor(anchor)
			i.Boundary, i.Expected = anchor, a.Revision
			i.Final = foundationStoreTuple(t, pi.Mount, 3, 3)
		}
		if strings.HasPrefix(shape, "nested") {
			parent := foundationStoreID(205)
			if _, err := applyFoundationStore(target, elementfact.OperationCreateAnchor, i.Actor, parent,
				foundationStoreTuple(t, pi.Mount, 0, 10), contract.Revision{}, "target-local-parent"); err != nil {
				t.Fatal(err)
			}
			i.Final.Parent = parent
		}
		r, err := beginTransfer(t, source, i)
		if err != nil {
			t.Fatal(err)
		}
		pi.Initial = r
	}
	h, err := reserveTransferParent(p, pi)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fenceTransfer(source, pi.Initial.Input, pi.Initial.Revision); err != nil {
		t.Fatal(err)
	}
	f, raw, err := source.TransferSourceFenceLocked(pi.Initial.Input.Actor, pi.Initial.Input.Operation)
	if err != nil {
		t.Fatal(err)
	}
	return p, source, target, h, f, raw
}

func acceptWithParent(o *Owner, f transferfact.SourceFence, raw string, h transferfact.ParentHold) (transferfact.TargetAcceptance, error) {
	o.Lock()
	defer o.Unlock()
	return o.AcceptTransferTargetLocked(f.Input.Actor, f, raw, nil, h)
}

func TestTransferMountedTargetPreservesPlacementAndOriginalResults(t *testing.T) {
	for _, shape := range []string{"direct-entity", "nested-entity", "direct-anchor", "nested-anchor"} {
		t.Run(shape, func(t *testing.T) {
			p, source, target, h, f, raw := targetParentCase(t, shape)
			i, before := f.Input, target.state.clone()
			slice, err := transferencoding.DecodeTransferManifest(f, raw)
			if err != nil {
				t.Fatal(err)
			}
			if placement, err := target.PreflightTransferTargetLocked(slice, i.Final, transferencoding.MaxTransferManifestRecords); err != nil || placement.Final() != i.Final || !sameCompleteCoreState(before, target.state) {
				t.Fatal("read-only placement rejected mounted target or reserved it", err)
			}
			if _, err := acceptTransfer(t, target, f, raw); !errors.Is(err, contract.ErrMountCoordinationRequired) || !sameCompleteCoreState(before, target.state) {
				t.Fatal("acceptance without parent hold", err)
			}
			a, err := acceptWithParent(target, f, raw, h)
			if err != nil || !target.state.transferHoldsIncoming() || target.state.elements.Kind(i.Boundary) != 0 {
				t.Fatal("acceptance enabled source prematurely or lost hold", err)
			}
			if _, err := applyFoundationStore(target, elementfact.OperationCreateEntity, i.Actor, foundationStoreID(206),
				foundationStoreTuple(t, h.Input.Mount, 15, 1), contract.Revision{}, "target-unrelated"); err != nil {
				t.Fatal("unrelated target writes blocked", err)
			}
			if strings.HasPrefix(shape, "nested") {
				parent, _ := target.state.elements.Anchor(i.Final.Parent)
				if _, err := applyFoundationStore(target, elementfact.OperationUpdateAnchor, i.Actor, parent.ID,
					foundationStoreTuple(t, parent.Parent, 1, 10), parent.Revision, "target-ancestor-move"); !errors.Is(err, contract.ErrConflict) {
					t.Fatal("accepted local ancestor moved", err)
				}
			}
			// Durable acceptance survives reopening before installation.
			path, retained := target.store.db.Path(), target.state.clone()
			closeFoundationStore(t, target)
			target = openFoundationStore(t, path)
			if !sameCompleteCoreState(retained, target.state) {
				t.Fatal("restart lost placement hold")
			}
			fenced := source.state.transfers.records[i.Operation]
			d, err := commitSuccessor(source, a, fenced.Revision)
			if err != nil {
				t.Fatal(err)
			}
			installed, err := installTransfer(target, d)
			if err != nil || target.state.transferHoldsIncoming() || target.state.mount.facts.Incoming() != before.mount.facts.Incoming() || target.state.elements.Kind(h.Input.Mount) != 0 {
				t.Fatal("installation changed carrier, copied external parent or retained target hold", err)
			}
			parent, _ := target.state.elements.ParentOf(i.Boundary)
			if parent != i.Final.Parent || token.Scope(installed.At) != target.state.scope {
				t.Fatal("wrong target placement or issuance")
			}
			settled, err := settleTransfer(source, installed)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := releaseTransferParent(p, h, transferfact.ParentRelease{Settled: settled}); err != nil {
				t.Fatal(err)
			}
			for _, o := range []*Owner{p, source, target} {
				path, retained := o.store.db.Path(), o.state.clone()
				closeFoundationStore(t, o)
				reopened := openFoundationStore(t, path)
				if !sameCompleteCoreState(retained, reopened.state) {
					t.Fatal("replay changed settled mounted target transfer")
				}
				if o == target {
					if retry, err := acceptWithParent(reopened, f, raw, h); err != nil || retry != a {
						t.Fatal("original acceptance not retained", err)
					}
					if retry, err := installTransfer(reopened, d); err != nil || retry != installed {
						t.Fatal("original installation not retained", err)
					}
				}
				closeFoundationStore(t, reopened)
			}
		})
	}
}

func TestTransferMountedTargetRejectsWrongParentProof(t *testing.T) {
	_, _, target, h, f, raw := targetParentCase(t, "direct-entity")
	before := target.state.clone()
	for _, mutate := range []func(*transferfact.ParentHold){
		func(h *transferfact.ParentHold) { h.Input.Role = transferfact.SourceParent },
		func(h *transferfact.ParentHold) { h.Input.Initial.Input.Final.LocalPose.Translation.X++ },
		func(h *transferfact.ParentHold) { h.Admission.OperationID = "another-admission" },
		func(h *transferfact.ParentHold) { h.Input.ParentAddress = "local:wrong-parent" },
		func(h *transferfact.ParentHold) { h.Tuple.Range = foundationStoreTuple(t, h.Input.Mount, 0, 19).Range },
		func(h *transferfact.ParentHold) {
			h.Input.Initial.Admission = token.FromParts(token.Scope(f.FinalCut), token.Serial(f.FinalCut)+1)
		},
	} {
		bad := h
		mutate(&bad)
		if _, err := acceptWithParent(target, f, raw, bad); err == nil || !sameCompleteCoreState(before, target.state) {
			t.Fatal("invalid parent proof reserved placement", err)
		}
	}
	if _, err := acceptWithParent(target, f, raw, h); err != nil {
		t.Fatal(err)
	}
	if _, err := acceptWithParent(target, f, raw, transferfact.ParentHold{}); !errors.Is(err, contract.ErrOperationConflict) {
		t.Fatal("retry omitted original parent proof", err)
	}
	if _, err := target.AcceptTransferTargetLocked(values.PrincipalRef{Realm: "other", Principal: "other"}, f, raw, nil, h); !errors.Is(err, values.PublicDenied) {
		t.Fatal("another actor obtained acceptance", err)
	}
}

func TestTransferMountedTargetRangeAndContactRepair(t *testing.T) {
	p, source, target, h, f, raw := targetParentCase(t, "direct-entity")
	slice, err := transferencoding.DecodeTransferManifest(f, raw)
	if err != nil {
		t.Fatal(err)
	}
	before := target.state.clone()
	if _, err := target.PreflightTransferTargetLocked(slice, foundationStoreTuple(t, h.Input.Mount, 20, 1), transferencoding.MaxTransferManifestRecords); !errors.Is(err, contract.ErrConflict) || !sameCompleteCoreState(before, target.state) {
		t.Fatal("external parent range was not enforced", err)
	}
	if _, err := target.PreflightTransferTargetLocked(slice, foundationStoreTuple(t, foundationStoreID(999), 0, 1), transferencoding.MaxTransferManifestRecords); !errors.Is(err, contract.ErrNotFound) {
		t.Fatal("unknown local parent invented", err)
	}
	accepted, err := acceptWithParent(target, f, raw, h)
	if err != nil {
		t.Fatal(err)
	}
	// Repair changes only the target's saved path to the same parent. An
	// already durable acceptance installs from its retained relation, not by
	// contacting the obsolete address copied in the original hold.
	in := target.state.mount.facts.Incoming()
	target.Lock()
	_, err = target.ApplyMountAddressLocked("target-parent-repair", NextHopBinding{Actor: f.Input.Actor,
		Element: h.Input.Mount, Key: in.Key, Expected: in.Contact.Revision, NextHop: "local:repaired-parent"})
	target.Unlock()
	if err != nil || !target.state.transferHoldsIncoming() {
		t.Fatal("repair blocked or dropped placement obligation", err)
	}
	d, err := commitSuccessor(source, accepted, source.state.transfers.records[f.Input.Operation].Revision)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := installTransfer(target, d); err != nil {
		t.Fatal("installation after local contact repair", err)
	}
	if target.state.mount.facts.Incoming().Contact.NextHop != "local:repaired-parent" || p.state.mount.facts.Record(h.Input.Mount).Slot != h.Input.Slot() {
		t.Fatal("install overwrote repair or bypassed parent release")
	}
	path, durable := target.store.db.Path(), target.state.clone()
	closeFoundationStore(t, target)
	target = openFoundationStore(t, path)
	defer closeFoundationStore(t, target)
	if !sameCompleteCoreState(durable, target.state) {
		t.Fatal("replay lost repair or original acceptance proof")
	}
}
