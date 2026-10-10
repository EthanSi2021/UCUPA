package commit

import (
	"errors"
	"reflect"
	"testing"

	"ucupa/ucupa-core/contract"
	mountfact "ucupa/ucupa-core/internal/mount/fact"
	transferfact "ucupa/ucupa-core/internal/transfer/fact"
)

func installClosedEdge(o *Owner, parent transferfact.CoreInstallation, closed transferfact.EdgeClosure) (transferfact.EdgeInstallation, error) {
	o.Lock()
	defer o.Unlock()
	return o.InstallTransferredEdgeLocked(parent, closed.Fence.Mount)
}

func completeEdges(o *Owner, parent transferfact.CoreInstallation, results []transferfact.EdgeInstallation) (transferfact.CoreCompletion, error) {
	o.Lock()
	defer o.Unlock()
	return o.CompleteTransferTargetLocked(parent, results)
}

func completeClosedEdge(t *testing.T, d, c *Owner, parent transferfact.CoreInstallation, closed transferfact.EdgeClosure) transferfact.CoreCompletion {
	t.Helper()
	before := d.state.clone()
	if _, err := completeEdges(d, parent, nil); !errors.Is(err, contract.ErrMountCoordinationRequired) || !sameCompleteCoreState(before, d.state) {
		t.Fatal("completed without child result", err)
	}
	if err := d.state.guardTransferElement(closed.Fence.Mount); !errors.Is(err, contract.ErrConflict) {
		t.Fatal("lost pending placement hold", err)
	}
	d.Lock()
	err := d.CheckTransferReadLocked(d.state.elements.View, d.state.currentCut(), closed.Fence.Mount, closed.Fence.Mount)
	d.Unlock()
	if !errors.Is(err, contract.ErrUnavailable) {
		t.Fatal("pending branch disclosed as ready", err)
	}
	childBefore := c.state.clone()
	child, err := installClosedEdge(c, parent, closed)
	if err != nil || !child.Valid() {
		t.Fatal("matching child installation", err)
	}
	key, err := parent.Delegation(closed)
	if err != nil {
		t.Fatal(err)
	}
	out, in := d.state.mount.facts.Record(closed.Fence.Mount), c.state.mount.facts.Incoming()
	if out.Admission != key || in.Key != key.MountKey || in.Admission.Key != key || in.Gate != mountfact.GateOpen ||
		in.Contact.NextHop != parent.Address() || in.Admission.RangeValue != closed.Range ||
		!reflect.DeepEqual(c.state.elements, childBefore.elements) || c.state.revisionSerial != childBefore.revisionSerial {
		t.Fatal("reparent changed content or lost exact matching relation")
	}
	c.Lock()
	oldErr := c.CheckTransferReadLocked(childBefore.elements.View, childBefore.currentCut(), closed.Fence.Mount, "")
	currentErr := c.CheckTransferReadLocked(c.state.elements.View, c.state.currentCut(), closed.Fence.Mount, "")
	view, captured := c.CaptureReadLocked()
	c.Unlock()
	if !errors.Is(oldErr, contract.ErrReadCutExpired) || currentErr != nil || !captured || view.Validate() != nil {
		t.Fatal("reparent read cut", oldErr, currentErr)
	}
	ready, err := completeEdges(d, parent, []transferfact.EdgeInstallation{child})
	if err != nil || !ready.Valid() || ready.At == parent.Cut() || !d.state.transfers.targets[parent.Key()].complete() {
		t.Fatal("target readiness", err)
	}
	if err := d.state.guardTransferElement(closed.Fence.Mount); err != nil {
		t.Fatal("completed placement stayed blocked", err)
	}
	if sourceVisibility(d.state).Inactive(closed.Fence.Mount) {
		t.Fatal("completed branch stayed invisible")
	}
	for _, o := range []*Owner{c, d} {
		if err := validateCoreState(o.state); err != nil {
			t.Fatal("complete state", err)
		}
	}
	savedChild, savedTarget := c.state.clone(), d.state.clone()
	if again, err := installClosedEdge(c, parent, closed); err != nil || again != child || !sameCompleteCoreState(savedChild, c.state) {
		t.Fatal("child retry", err)
	}
	if again, err := completeEdges(d, parent, []transferfact.EdgeInstallation{child}); err != nil || again != ready || !sameCompleteCoreState(savedTarget, d.state) {
		t.Fatal("completion retry", err)
	}
	return ready
}
