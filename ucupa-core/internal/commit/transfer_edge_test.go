package commit

import (
	"errors"
	"fmt"
	"reflect"
	"testing"

	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	elementfact "ucupa/ucupa-core/internal/element/fact"
	mountfact "ucupa/ucupa-core/internal/mount/fact"
	transferfact "ucupa/ucupa-core/internal/transfer/fact"
)

func edgeClosureFixture(t *testing.T, boundary int, radius float64) (parent, carrier, target *Owner, f transferfact.EdgeFence, raw string) {
	t.Helper()
	parent, carrier, target, _ = carrierControlFixture(t)
	a, _ := parent.state.elements.Anchor(foundationStoreID(uint64(boundary)))
	i := transferfact.Intent{Profile: transferfact.SliceTransfer, Operation: "move-parent", Actor: foundationStoreActor,
		Source: parent.authority, Target: target.authority, Boundary: a.ID, Expected: a.Revision,
		Final: foundationStoreTuple(t, foundationStoreID(999), 0, radius), SourceAddress: "local:carrier-parent", TargetAddress: "local:new-parent"}
	if boundary == 100 {
		i = rootTransferIntent(t, parent, target, i.Operation)
	} else if _, err := applyFoundationStore(target, elementfact.OperationCreateRoot, foundationStoreActor, foundationStoreID(999), contract.SpatialTuple{}, contract.Revision{}, "new-parent-root"); err != nil {
		t.Fatal(err)
	}
	beforeCarrier, oldMount := carrier.state.clone(), parent.state.mount.facts.Record(foundationStoreID(104))
	f.Source, raw = frozenSource(t, parent, i)
	f.Mount = foundationStoreID(104)
	slice, err := decodeEdgeManifest(f, raw)
	if err != nil || !slice.HasMounts() {
		t.Fatal("missing external boundary", err)
	}
	found := false
	for _, m := range slice.Members() {
		if m.ID() == f.Mount {
			found = m.Mount == oldMount
		}
		if m.ID() == foundationStoreID(105) {
			t.Fatal("copied external carrier content")
		}
	}
	if !found || parent.state.mount.facts.Record(f.Mount).State != contract.MountNone || !sameCompleteCoreState(beforeCarrier, carrier.state) {
		t.Fatal("fence lost boundary evidence or changed another owner")
	}
	beforeTarget := target.state.clone()
	if _, err := acceptTransfer(t, target, f.Source, raw); !errors.Is(err, contract.ErrMountCoordinationRequired) || !sameCompleteCoreState(beforeTarget, target.state) {
		t.Fatal("target activated without matching child continuation", err)
	}
	if err := validateCoreState(parent.state); err != nil {
		t.Fatal("source withdrawal", err)
	}
	return
}

func closeEdge(o *Owner, actor values.PrincipalRef, f transferfact.EdgeFence, raw string) (transferfact.EdgeClosure, error) {
	o.Lock()
	defer o.Unlock()
	return o.CloseTransferredEdgeLocked(actor, f, raw)
}

func TestTransferExternalEdgeClosureKeepsContentAndSurvivesRestart(t *testing.T) {
	for _, boundary := range []int{100, 101, 104} {
		t.Run(fmt.Sprint(boundary), func(t *testing.T) {
			p, c, d, f, raw := edgeClosureFixture(t, boundary, 30)
			before := c.state.clone()
			closed, err := closeEdge(c, foundationStoreActor, f, raw)
			if err != nil || !closed.Valid(c.authority) || closed.Previous != before.mount.facts.Incoming().Admission || closed.At != c.state.currentCut() {
				t.Fatal("closure", closed, err)
			}
			if c.state.cutSerial != before.cutSerial+1 || c.state.revisionSerial != before.revisionSerial ||
				!reflect.DeepEqual(c.state.elements, before.elements) || !reflect.DeepEqual(c.state.attachments, before.attachments) ||
				c.state.mount.facts.Incoming().Gate != mountfact.GateReparenting {
				t.Fatal("closure changed carrier content, revisions or gate")
			}
			c.Lock()
			oldRead := c.CheckTransferReadLocked(before.elements.View, before.currentCut(), f.Mount, foundationStoreID(105))
			newRead := c.CheckTransferReadLocked(c.state.elements.View, c.state.currentCut(), f.Mount, foundationStoreID(105))
			candidate, ok := c.CaptureReadLocked()
			c.Unlock()
			if !errors.Is(oldRead, contract.ErrReadCutExpired) || !errors.Is(newRead, contract.ErrUnavailable) || !ok || candidate.Validate() != nil {
				t.Fatal("read safety", oldRead, newRead, ok, candidate.Validate())
			}
			entity, _ := c.state.elements.Entity(foundationStoreID(105))
			if _, err := applyFoundationStore(c, elementfact.OperationUpdateEntity, foundationStoreActor, entity.ID,
				foundationStoreTuple(t, f.Mount, 1, 1), entity.Revision, "old-parent-write"); !errors.Is(err, contract.ErrMountBusy) {
				t.Fatal("old relation admitted a write", err)
			}
			// A delayed exact original Finish returns only the original outcome;
			// it cannot change the reparenting gate or manufacture a new cut.
			beforeRetry := c.state.clone()
			original := p.state.mount.operations["carrier-original"]
			c.Lock()
			_, err = c.ApplyMountFinishLocked(p.authority, contract.MountFinishRequest{Evidence: original.Evidence()})
			c.Unlock()
			if err != nil || !sameCompleteCoreState(beforeRetry, c.state) {
				t.Fatal("old finish reopened edge", err)
			}
			for _, o := range []*Owner{p, c, d} {
				path, saved := o.store.db.Path(), o.state.clone()
				closeFoundationStore(t, o)
				reopened := openFoundationStore(t, path)
				if !sameCompleteCoreState(saved, reopened.state) {
					t.Fatal("restart changed edge evidence")
				}
				if o == c {
					again, err := closeEdge(reopened, foundationStoreActor, f, raw)
					if err != nil || again != closed || !sameCompleteCoreState(saved, reopened.state) {
						t.Fatal("closure retry", err)
					}
				}
				closeFoundationStore(t, reopened)
			}
		})
	}
}

func TestTransferExternalEdgeClosureRejectsWrongProofAndRange(t *testing.T) {
	for _, test := range []string{"range", "wrong-carrier", "wrong-actor", "wrong-member", "missing-bytes", "changed-bytes", "dual-profile"} {
		t.Run(test, func(t *testing.T) {
			radius := 30.0
			if test == "range" {
				radius = 0.5 // source anchor has no local content, carrier has a radius-1 Entity
			}
			_, c, d, f, raw := edgeClosureFixture(t, 104, radius)
			actor := foundationStoreActor
			switch test {
			case "wrong-carrier":
				c = d
			case "wrong-actor":
				actor.Principal = "not-original"
			case "wrong-member":
				f.Mount = foundationStoreID(105)
			case "missing-bytes":
				raw = ""
			case "changed-bytes":
				raw += " "
			case "dual-profile":
				f.Carrier.Revision = 1
			}
			before := c.state.clone()
			if _, err := closeEdge(c, actor, f, raw); err == nil || !sameCompleteCoreState(before, c.state) {
				t.Fatal("invalid edge left a hold", err)
			}
		})
	}
}
