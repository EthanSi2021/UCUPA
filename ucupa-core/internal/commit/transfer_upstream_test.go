package commit

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"testing"
	"ucupa/ucupa-core/contract"
	elementfact "ucupa/ucupa-core/internal/element/fact"
	mountfact "ucupa/ucupa-core/internal/mount/fact"
	transferencoding "ucupa/ucupa-core/internal/transfer/encoding"
	transferfact "ucupa/ucupa-core/internal/transfer/fact"
)

func sourceParentFixture(t *testing.T, shape string) (*Owner, *Owner, *Owner, transferfact.ParentIntent) {
	t.Helper()
	p, s, target, carrier := carrierControlFixture(t)
	root := foundationStoreID(200)
	if _, err := applyFoundationStore(target, elementfact.OperationCreateRoot, foundationStoreActor, root, contract.SpatialTuple{}, contract.Revision{}, "slice-target-root"); err != nil {
		t.Fatal(err)
	}
	boundary := foundationStoreID(105)
	if shape != "entity" {
		anchor, leaf := foundationStoreID(106), foundationStoreID(107)
		if _, err := applyFoundationStore(s, elementfact.OperationCreateAnchor, foundationStoreActor, anchor, foundationStoreTuple(t, carrier.Mount, 0, 5), contract.Revision{}, "slice-source-anchor"); err != nil {
			t.Fatal(err)
		}
		if _, err := applyFoundationStore(s, elementfact.OperationCreateEntity, foundationStoreActor, leaf, foundationStoreTuple(t, anchor, 0, 1), contract.Revision{}, "slice-source-leaf"); err != nil {
			t.Fatal(err)
		}
		boundary = anchor
		if shape == "nested-entity" {
			boundary = leaf
		}
	}
	var expected contract.Revision
	if a, ok := s.state.elements.Anchor(boundary); ok {
		expected = a.Revision
	} else {
		e, _ := s.state.elements.Entity(boundary)
		expected = e.Revision
	}
	i := transferfact.Intent{Profile: transferfact.SliceTransfer, Operation: "slice-from-mounted-source", Actor: foundationStoreActor,
		Source: s.authority, Target: target.authority, Boundary: boundary, Expected: expected,
		Final: foundationStoreTuple(t, root, 10, 5), SourceAddress: carrier.SourceAddress, TargetAddress: carrier.TargetAddress}
	r, err := beginTransfer(t, s, i)
	if err != nil {
		t.Fatal("mounted source intent", err)
	}
	return p, s, target, transferfact.ParentIntent{Initial: r, Role: transferfact.SourceParent, Parent: p.authority, Mount: carrier.Mount,
		ExpectedElement: carrier.ExpectedElement, ExpectedMount: carrier.ExpectedMount, ParentAddress: carrier.ParentAddress}
}

func fenceWithParent(s *Owner, h transferfact.ParentHold) (transferfact.IntentRecord, error) {
	s.Lock()
	defer s.Unlock()
	r := h.Input.Initial
	return s.FenceTransferSourceLocked(r.Input.Actor, r.Input.Operation, r.Revision, h)
}

func TestTransferFromMountedSourceCompletesWithoutMovingCarrier(t *testing.T) {
	for _, shape := range []string{"entity", "anchor", "nested-entity"} {
		t.Run(shape, func(t *testing.T) {
			p, s, target, pi := sourceParentFixture(t, shape)
			i := pi.Initial.Input
			sourceBefore := s.state.clone()
			if _, err := fenceTransfer(s, i, pi.Initial.Revision); !errors.Is(err, contract.ErrMountCoordinationRequired) || !sameCompleteCoreState(sourceBefore, s.state) {
				t.Fatal("fence without parent hold", err)
			}
			h, err := reserveTransferParent(p, pi)
			if err != nil {
				t.Fatal(err)
			}
			fenced, err := fenceWithParent(s, h)
			if err != nil {
				t.Fatal("source fence", err)
			}
			f, raw, err := s.TransferSourceFenceLocked(i.Actor, i.Operation)
			if err != nil {
				t.Fatal(err)
			}
			slice, err := transferencoding.DecodeTransferManifest(f, raw)
			if err != nil || slice.Profile() != transferfact.SliceTransfer || slice.SourceParent() != h || slice.ExternalParent().ID != pi.Mount {
				t.Fatal("lost source boundary or changed profile", err)
			}
			for _, m := range slice.Members() {
				if m.ID() == pi.Mount {
					t.Fatal("copied external mount into source manifest")
				}
			}
			if s.state.mount.facts.Incoming() != sourceBefore.mount.facts.Incoming() || s.state.mount.facts.Incoming().Gate != mountfact.GateOpen {
				t.Fatal("slice fence closed or replaced whole carrier")
			}
			if _, err := applyFoundationStore(s, elementfact.OperationCreateEntity, i.Actor, foundationStoreID(108), foundationStoreTuple(t, pi.Mount, 10, 1), contract.Revision{}, "source-unrelated-after-fence"); err != nil {
				t.Fatal("unrelated source writes stopped", err)
			}
			if err := s.CheckTransferReadLocked(sourceBefore.elements.View, sourceBefore.currentCut(), pi.Mount, i.Boundary); err == nil {
				t.Fatal("old read revived transferred member")
			}
			a, err := acceptTransfer(t, target, f, raw)
			if err != nil {
				t.Fatal("accept mounted source", err)
			}
			d, err := commitSuccessor(s, a, fenced.Revision)
			if err != nil {
				t.Fatal(err)
			}
			installed, err := installTransfer(target, d)
			if err != nil {
				t.Fatal(err)
			}
			settled, err := settleTransfer(s, installed)
			if err != nil {
				t.Fatal(err)
			}
			if target.state.elements.Kind(i.Boundary) == 0 || s.state.sourceHoldsIncoming() || s.state.mount.facts.Incoming() != sourceBefore.mount.facts.Incoming() {
				t.Fatal("settlement lost original carrier relationship")
			}
			if _, err := releaseTransferParent(p, h, transferfact.ParentRelease{Settled: settled}); err != nil {
				t.Fatal("actual source settlement did not release parent", err)
			}
			for _, o := range []*Owner{p, s, target} {
				path, before := o.store.db.Path(), o.state.clone()
				closeFoundationStore(t, o)
				reopened := openFoundationStore(t, path)
				if !sameCompleteCoreState(before, reopened.state) {
					t.Fatal("restart changed partial-source transfer")
				}
				if o == s {
					if retry, err := fenceWithParent(reopened, h); err != nil || retry != fenced {
						t.Fatal("source fence retry after settlement", err)
					}
				}
				closeFoundationStore(t, reopened)
			}
		})
	}
}

func TestTransferSourceParentProofIsExact(t *testing.T) {
	p, s, _, pi := sourceParentFixture(t, "nested-entity")
	h, err := reserveTransferParent(p, pi)
	if err != nil {
		t.Fatal(err)
	}
	before := s.state.clone()
	for _, mutate := range []func(*transferfact.ParentHold){
		func(h *transferfact.ParentHold) { h.Input.Role = transferfact.TargetParent },
		func(h *transferfact.ParentHold) { h.Input.Initial.Input.Final.LocalPose.Translation.X++ },
		func(h *transferfact.ParentHold) { h.Admission.OperationID = "another-admission" },
		func(h *transferfact.ParentHold) { h.Tuple.Range = foundationStoreTuple(t, pi.Mount, 0, 19).Range },
		func(h *transferfact.ParentHold) { h.Input.ParentAddress = "local:other-parent" },
	} {
		bad := h
		mutate(&bad)
		if _, err := fenceWithParent(s, bad); err == nil || !sameCompleteCoreState(before, s.state) {
			t.Fatal("wrong upstream proof changed source", err)
		}
	}
	if _, err := fenceWithParent(s, h); err != nil {
		t.Fatal(err)
	}
	f, raw, err := s.TransferSourceFenceLocked(pi.Initial.Input.Actor, pi.Initial.Input.Operation)
	if err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(*transferencoding.StoredTransferManifest){
		func(m *transferencoding.StoredTransferManifest) { m.SourceParent = nil },
		func(m *transferencoding.StoredTransferManifest) {
			m.SourceParent.Input.Role = transferfact.TargetParent
		},
		func(m *transferencoding.StoredTransferManifest) {
			m.SourceParent.Input.Initial.Input.Operation = []byte("another-source")
		},
		func(m *transferencoding.StoredTransferManifest) { m.SourceParent.At = nil },
	} {
		var m transferencoding.StoredTransferManifest
		if err := json.Unmarshal([]byte(raw), &m); err != nil {
			t.Fatal(err)
		}
		mutate(&m)
		b, err := json.Marshal(m)
		if err != nil {
			t.Fatal(err)
		}
		changed := f
		changed.ManifestDigest = sha256.Sum256(b)
		if _, err := transferencoding.DecodeTransferManifest(changed, string(b)); err == nil {
			t.Fatal("self-consistent hash authenticated an invalid parent closure")
		}
	}
}
