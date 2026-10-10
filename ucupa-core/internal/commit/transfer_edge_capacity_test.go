package commit

import (
	"errors"
	"strings"
	"testing"
	"ucupa/shared/values"
	"ucupa/ucupa-core/internal/token"
	transferencoding "ucupa/ucupa-core/internal/transfer/encoding"
	transferfact "ucupa/ucupa-core/internal/transfer/fact"
)

func TestTransferredEdgeReservesReceiptBytesBeforeAccepting(t *testing.T) {
	c, d, parent, closed := pendingEdgeInstallation(t)
	child, err := installClosedEdge(c, parent, closed)
	if err != nil {
		t.Fatal(err)
	}
	r := d.state.transfers.targets[parent.Key()]
	raw, err := transferencoding.EncodeEdgeResults(r.closures, r.installedProof(), []transferfact.EdgeInstallation{child})
	if err != nil {
		t.Fatal(err)
	}
	// Capacity arithmetic is checked on a bounded candidate, not submitted as
	// a fabricated journal. The real completion is exercised separately below.
	s := &coreState{transfers: newTransferState()}
	s.transfers.targets[parent.Key()] = r
	s.transfers.events = []transferencoding.TransferEvent{{Manifest: strings.Repeat("m", transferencoding.MaxTransferRetainedBytes-transferencoding.MaxEdgeResultBytes)}}
	if err := s.checkTransferRetainedBytes(0); err != nil {
		t.Fatal("reserved exact limit", err)
	}
	if err := s.checkTransferRetainedBytes(1); !errors.Is(err, values.PublicResourceExhausted) {
		t.Fatal("receipt reserve consumed by another task", err)
	}
	ready, err := completeEdges(d, parent, []transferfact.EdgeInstallation{child})
	if err != nil {
		t.Fatal(err)
	}
	r.completion = ready
	s.transfers.targets[parent.Key()] = r
	if err := s.checkTransferRetainedBytes(len(raw)); err != nil {
		t.Fatal("own terminal receipt cannot consume reservation", err)
	}
	// The compact result stores one fixed-size scope, UUID, cut and revision,
	// never another copy of the full transfer input.
	maximum := ^uint64(0)
	child.At, child.Revision = token.FromParts(token.Scope(closed.At), maximum), maximum
	encoded, err := transferencoding.EncodeEdgeResults(r.closures, r.installedProof(), []transferfact.EdgeInstallation{child})
	if err != nil || len(encoded) > transferencoding.MaxEdgeResultBytes || transferencoding.ValidateEdgeResults(r.closures, r.installedProof(), encoded) != nil {
		t.Fatal("max terminal receipt exceeds reserve", len(encoded), err)
	}
	s.transfers.events[0].EdgeResults = raw
	if err := s.checkTransferTargetCapacity("", "", "", 1); !errors.Is(err, values.PublicResourceExhausted) {
		t.Fatal("new admission borrowed consumed completion room", err)
	}
}
