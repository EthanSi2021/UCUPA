package commit

import (
	"errors"
	"fmt"
	"slices"
	"testing"

	"ucupa/ucupa-core/contract"
	elementfact "ucupa/ucupa-core/internal/element/fact"
	mountfact "ucupa/ucupa-core/internal/mount/fact"
	"ucupa/ucupa-core/internal/token"
	transferfact "ucupa/ucupa-core/internal/transfer/fact"
)

func acceptClosedEdges(o *Owner, f transferfact.EdgeFence, raw string, closures []transferfact.EdgeClosure) error {
	o.Lock()
	defer o.Unlock()
	if f.Source != (transferfact.SourceFence{}) {
		_, err := o.AcceptTransferTargetLocked(f.Source.Input.Actor, f.Source, raw, closures, transferfact.ParentHold{})
		return err
	}
	_, err := o.AcceptCarrierTargetLocked(f.Carrier.Permit.Reservation.Input.Actor, f.Carrier, raw, closures)
	return err
}

func TestTransferClosedEdgesReserveWithoutActivatingAndSurviveRestart(t *testing.T) {
	for _, boundary := range []int{100, 101, 104} {
		t.Run(fmt.Sprint(boundary), func(t *testing.T) {
			p, c, d, f, raw := edgeClosureFixture(t, boundary, 30)
			closed, err := closeEdge(c, foundationStoreActor, f, raw)
			if err != nil {
				t.Fatal(err)
			}
			before, carrierBefore := d.state.clone(), c.state.clone()
			closures := []transferfact.EdgeClosure{closed}
			if err := acceptClosedEdges(d, f, raw, closures); err != nil {
				t.Fatal(err)
			}
			r := d.state.transfers.targets[f.Key().Transfer]
			if !slices.Equal(r.closures, closures) || r.installed() || d.state.currentCut() != before.currentCut() ||
				d.state.elements.Counts() != before.elements.Counts() || !sameCompleteCoreState(carrierBefore, c.state) {
				t.Fatal("acceptance activated content or lost exact hold")
			}
			closures[0].Revision++ // caller memory cannot rewrite the durable promise
			if r.closures[0] != closed {
				t.Fatal("borrowed caller proof")
			}
			if err := d.state.guardTransferElement(f.Mount); !errors.Is(err, contract.ErrConflict) {
				t.Fatal("unreserved UUID", err)
			}
			if boundary != 100 {
				root, _ := d.state.elements.Anchor(f.Source.Input.Final.Parent)
				if err := d.state.guardTransferScope(root.ID); !errors.Is(err, contract.ErrConflict) {
					t.Fatal("unreserved placement", err)
				}
			}
			if err := validateCoreState(d.state); err != nil {
				t.Fatal(err)
			}
			path, saved := d.store.db.Path(), d.state.clone()
			closeFoundationStore(t, d)
			d = openFoundationStore(t, path)
			defer closeFoundationStore(t, d)
			if !sameCompleteCoreState(saved, d.state) {
				t.Fatal("restart lost closure obligations")
			}
			if err := acceptClosedEdges(d, f, raw, []transferfact.EdgeClosure{closed}); err != nil || !sameCompleteCoreState(saved, d.state) {
				t.Fatal("retry changed promise", err)
			}
			if err := acceptClosedEdges(d, f, raw, closures); !errors.Is(err, contract.ErrOperationConflict) || !sameCompleteCoreState(saved, d.state) {
				t.Fatal("changed receipt replaced promise", err)
			}
			if err := acceptClosedEdges(d, f, raw, nil); !errors.Is(err, contract.ErrOperationConflict) || !sameCompleteCoreState(saved, d.state) {
				t.Fatal("missing proof replaced promise", err)
			}
			decision, err := commitSuccessor(p, r.receipt, p.state.transfers.records[f.Source.Input.Operation].Revision)
			if err != nil {
				t.Fatal(err)
			}
			installed, err := installTransfer(d, decision)
			if err != nil {
				t.Fatal("target installation", err)
			}
			parent := transferfact.CoreInstallation{Source: installed}
			ready := completeClosedEdge(t, d, c, parent, closed)
			p.Lock()
			_, err = p.SettleTransferSourceLocked(ready)
			p.Unlock()
			if err != nil {
				t.Fatal("source completion", err)
			}
		})
	}
}

func TestTransferClosedEdgesRejectIncompleteOrMismatchedProofWithoutResidue(t *testing.T) {
	_, c, d, f, raw := edgeClosureFixture(t, 104, 30)
	closed, err := closeEdge(c, foundationStoreActor, f, raw)
	if err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"missing", "duplicate", "foreign-mount", "wrong-fence", "old-range", "new-range", "future-decision", "wrong-admission", "zero-cut", "wrong-carrier"} {
		t.Run(mode, func(t *testing.T) {
			proof := closed
			switch mode {
			case "foreign-mount":
				proof.Fence.Mount = foundationStoreID(101)
			case "wrong-fence":
				proof.Fence.Source.Input.Operation = "different-transfer"
			case "old-range":
				proof.Previous.RangeValue = foundationStoreTuple(t, f.Mount, 0, 21).Range
			case "new-range":
				proof.Range = foundationStoreTuple(t, f.Mount, 0, 29).Range
			case "future-decision":
				proof.Previous.DecisionCut = token.FromParts(token.Scope(f.Source.FinalCut), token.Serial(f.Source.FinalCut)+1)
			case "wrong-admission":
				proof.Previous.Key.OperationID = "different-mount"
			case "zero-cut":
				proof.At = contract.CoreReadCut{}
			case "wrong-carrier":
				proof.Previous.Key.MountKey.CarrierAuthority = d.authority
			}
			proofs := []transferfact.EdgeClosure{proof}
			if mode == "missing" {
				proofs = nil
			}
			if mode == "duplicate" {
				proofs = append(proofs, proof)
			}
			before := d.state.clone()
			if err := acceptClosedEdges(d, f, raw, proofs); err == nil || !sameCompleteCoreState(before, d.state) {
				t.Fatal("invalid proof retained", err)
			}
		})
	}
}

func TestTransferClosedEdgesRequireEveryBoundaryAndCanonicalRetry(t *testing.T) {
	p, c, d, _ := carrierControlFixture(t)
	c2 := emptyRootReceiver(t)
	mount := foundationStoreID(110)
	if _, err := applyFoundationStore(p, elementfact.OperationCreateAnchor, foundationStoreActor, mount,
		foundationStoreTuple(t, foundationStoreID(101), 0, 10), contract.Revision{}, "second-mount"); err != nil {
		t.Fatal(err)
	}
	establishEdge(t, p, c2, mount)
	f, raw := frozenSource(t, p, rootTransferIntent(t, p, d, "two-edges"))
	proofs := make([]transferfact.EdgeClosure, 0, 2)
	for n, child := range []*Owner{c, c2} {
		id := foundationStoreID(104)
		if n == 1 {
			id = mount
		}
		closed, err := closeEdge(child, foundationStoreActor, transferfact.EdgeFence{Source: f, Mount: id}, raw)
		if err != nil {
			t.Fatal(err)
		}
		proofs = append(proofs, closed)
	}
	common := transferfact.EdgeFence{Source: f, Mount: f.Input.Boundary}
	before := d.state.clone()
	if err := acceptClosedEdges(d, common, raw, proofs[:1]); !errors.Is(err, contract.ErrMountCoordinationRequired) || !sameCompleteCoreState(before, d.state) {
		t.Fatal("omitted external boundary", err)
	}
	if err := acceptClosedEdges(d, common, raw, proofs); err != nil {
		t.Fatal(err)
	}
	saved := d.state.clone()
	slices.Reverse(proofs)
	if err := acceptClosedEdges(d, common, raw, proofs); err != nil || !sameCompleteCoreState(saved, d.state) {
		t.Fatal("order-only retry changed promise", err)
	}
	if c.state.mount.facts.Incoming().Gate != mountfact.GateReparenting || c2.state.mount.facts.Incoming().Gate != mountfact.GateReparenting {
		t.Fatal("acceptance reopened a child")
	}
	decision, err := commitSuccessor(p, d.state.transfers.targets[f.Key()].receipt, p.state.transfers.records[f.Input.Operation].Revision)
	if err != nil {
		t.Fatal(err)
	}
	i, err := installTransfer(d, decision)
	if err != nil {
		t.Fatal(err)
	}
	parent := transferfact.CoreInstallation{Source: i}
	results := make([]transferfact.EdgeInstallation, 0, 2)
	for _, child := range []*Owner{c, c2} {
		var proof transferfact.EdgeClosure
		for _, item := range proofs {
			if item.Previous.Key.MountKey.CarrierAuthority == child.authority {
				proof = item
			}
		}
		r, err := installClosedEdge(child, parent, proof)
		if err != nil {
			t.Fatal(err)
		}
		results = append(results, r)
	}
	beforeComplete := d.state.clone()
	for _, incomplete := range [][]transferfact.EdgeInstallation{nil, results[:1], {results[0], results[0]}} {
		if _, err := completeEdges(d, parent, incomplete); err == nil || !sameCompleteCoreState(beforeComplete, d.state) {
			t.Fatal("missing or repeated child released target", err)
		}
	}
	// The source cannot clear its gap using installation alone when its
	// retained manifest still names child obligations.
	p.Lock()
	_, err = p.SettleTransferSourceLocked(transferfact.CoreCompletion{Installed: parent, At: parent.Cut(), Revision: parent.Revision()})
	p.Unlock()
	if !errors.Is(err, contract.ErrMountCoordinationRequired) {
		t.Fatal("premature source settlement", err)
	}
	ready, err := completeEdges(d, parent, results)
	if err != nil {
		t.Fatal(err)
	}
	saved = d.state.clone()
	slices.Reverse(results)
	if again, err := completeEdges(d, parent, results); err != nil || again != ready || !sameCompleteCoreState(saved, d.state) {
		t.Fatal("receipt order changed completion", err)
	}
	results[0].Revision++
	if _, err := completeEdges(d, parent, results); !errors.Is(err, contract.ErrOperationConflict) || !sameCompleteCoreState(saved, d.state) {
		t.Fatal("completion retry replaced original receipt", err)
	}
}
