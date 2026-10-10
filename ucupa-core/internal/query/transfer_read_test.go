package query

import (
	"errors"
	"testing"
	"time"

	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	mountfact "ucupa/ucupa-core/internal/mount/fact"
	transferfact "ucupa/ucupa-core/internal/transfer/fact"
)

// Drive the real durable owner, without a public transfer entry or a writable
// test-state export. The eventual public workflow must add its own approvals.
func fenceQueryMember(t *testing.T, f queryFixture, id values.ElementID) {
	t.Helper()
	read, err := f.elements.ReadElement(queryActor, id, nil)
	if err != nil || read.Record == nil {
		t.Fatalf("source record: %#v %v", read, err)
	}
	var revision contract.Revision
	if read.Record.Entity != nil {
		revision = read.Record.Entity.Revision
	} else {
		revision = read.Record.Anchor.Revision
	}
	target, err := values.NewFactAuthorityRef()
	if err != nil {
		t.Fatal(err)
	}
	i := transferfact.Intent{Profile: transferfact.SliceTransfer, Operation: contract.OperationID("fence-" + string(id)), Actor: queryActor,
		Source: f.owner.AuthorityRef(), Target: target, Boundary: id, Expected: revision,
		Final: queryTuple(t, queryID(99000)), SourceAddress: "local:source", TargetAddress: "local:target"}
	f.owner.Lock()
	defer f.owner.Unlock()
	r, err := f.owner.BeginTransferIntentLocked(i, 4096)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.owner.FenceTransferSourceLocked(queryActor, i.Operation, r.Revision, transferfact.ParentHold{}); err != nil {
		t.Fatal(err)
	}
}

func TestParentReadEvidenceConsumesSnapshotAdmission(t *testing.T) {
	f := newQueryFixture(t, contract.Config{}, true)
	f.owner.RLock()
	candidate, valid := f.owner.CaptureReadLocked()
	f.owner.RUnlock()
	if !valid {
		t.Fatal("fixture capture")
	}
	p := candidate.Projection
	base, ok := readViewBytes(p)
	if !ok {
		t.Fatal("fixture accounting")
	}
	// Isolated accounting only, never published as owner evidence or a lease.
	p.Transfers = transferfact.NewVisibility(nil, nil, map[values.ElementID]mountfact.Record{f.ids[0]: {}}, nil, nil)
	withHold, ok := readViewBytes(p)
	if !ok || withHold != base+(64<<10) {
		t.Fatal("parent proof bypassed snapshot byte accounting", base, withHold)
	}
}

func TestTransferFenceExpiresOldPaginationButNotUnrelatedPointRead(t *testing.T) {
	ttl := time.Hour
	f := newQueryFixture(t, contract.Config{SnapshotRetention: &ttl}, true)
	page, err := f.queries.QueryRegion(queryActor, f.region, nil, 1, nil)
	if err != nil || page.Snapshot == nil || page.Continuation == nil {
		t.Fatalf("initial page: %#v %v", page, err)
	}
	defer f.queries.ReleaseReadSnapshot(queryActor, *page.Snapshot)
	lease, err := f.queries.Borrow(queryActor, *page.Snapshot)
	if err != nil {
		t.Fatal(err)
	}
	defer lease.Finish()
	if err := f.queries.Check(lease); err != nil {
		t.Fatal(err)
	}
	fenceQueryMember(t, f, f.ids[1])
	next, err := f.queries.QueryRegion(queryActor, f.region, nil, 1, page.Continuation)
	requireQueryError(t, next, err, contract.ErrReadCutExpired)
	if read, err := f.queries.ReadElementSnapshot(queryActor, f.ids[1], *page.Snapshot); !errors.Is(err, contract.ErrUnavailable) || read.Record != nil {
		t.Fatalf("inactive historical read disclosed: %#v %v", read, err)
	}
	if read, err := f.queries.ReadElementSnapshot(queryActor, f.ids[0], *page.Snapshot); err != nil || read.Record == nil {
		t.Fatalf("unrelated retained point read stopped: %#v %v", read, err)
	}
	// A successful earlier Check is not permission to disclose after a fence.
	f.owner.RLock()
	late := f.queries.CheckLocked(lease, "")
	point := f.queries.CheckLocked(lease, f.ids[1])
	f.owner.RUnlock()
	if !errors.Is(late, contract.ErrReadCutExpired) || !errors.Is(point, contract.ErrUnavailable) {
		t.Fatalf("last disclosure gate: scope=%v exact=%v", late, point)
	}
}

func TestTransferFenceFreshReadsKeepCoverageAndAbsenceDistinct(t *testing.T) {
	f := newQueryFixture(t, contract.Config{}, true)
	fenceQueryMember(t, f, f.ids[1])
	if read, err := f.elements.ReadElement(queryActor, f.ids[1], nil); !errors.Is(err, contract.ErrUnavailable) || read.Record != nil {
		t.Fatalf("source departure became ordinary absence: %#v %v", read, err)
	}
	page, err := f.queries.QueryRegion(queryActor, f.region, nil, 256, nil)
	if err != nil || page.Status != contract.QueryPartial || page.Gap != contract.QueryUnresolvedCoverage || len(page.Matches) != 2 {
		t.Fatalf("fresh intersecting query concealed gap: %#v %v", page, err)
	}
	for _, match := range page.Matches {
		if match.ElementID == f.ids[1] {
			t.Fatal("inactive member leaked into fresh matches")
		}
	}
	outside := f.region
	outside.Sphere.Center.X = 10
	page, err = f.queries.QueryRegion(queryActor, outside, nil, 256, nil)
	if err != nil || page.Status != contract.QueryComplete || len(page.Matches) != 0 {
		t.Fatalf("proved outside sphere unnecessarily stopped: %#v %v", page, err)
	}
}
