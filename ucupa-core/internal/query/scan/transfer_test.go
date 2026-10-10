package scan

import (
	"errors"
	"testing"

	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	elementfact "ucupa/ucupa-core/internal/element/fact"
	"ucupa/ucupa-core/internal/geometry"
	mountfact "ucupa/ucupa-core/internal/mount/fact"
	transferfact "ucupa/ucupa-core/internal/transfer/fact"
)

func TestTransferCoverageUsesActualScopeAndTransformedEnvelope(t *testing.T) {
	s := elementfact.New()
	root, parent, sibling, boundary := scanID(1), scanID(2), scanID(3), scanID(4)
	scanAnchor(t, &s, root, "", 0)
	scanAnchor(t, &s, parent, root, 5)
	scanAnchor(t, &s, sibling, root, 0)
	pose, _ := contract.NewPose(contract.Vec3{X: 2}, geometry.IdentityPose().Rotation)
	radius, _ := contract.NewBoundedSphere(1)
	fence := transferfact.SourceFootprint{Boundary: boundary, Origin: contract.SpatialTuple{Parent: parent, LocalPose: pose, Range: radius}}
	v := transferfact.NewVisibility(map[values.ElementID]values.ElementID{boundary: boundary}, []transferfact.SourceFootprint{fence}, nil, nil, nil)
	q := contract.RegionQuery{StartAnchorID: root, BasisAnchorID: root, Selector: contract.QueryEntities}
	for _, test := range []struct {
		name  string
		scope values.ElementID
		basis values.ElementID
		x     float64
		gap   bool
	}{
		{"root tangent", root, root, 8, true},
		{"root outside", root, root, 9, false},
		{"translated basis", root, parent, 3, true},
		{"unrelated subtree", sibling, sibling, 7, false},
		{"direct parent", parent, parent, 2, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			q.StartAnchorID, q.BasisAnchorID, q.Sphere.Center.X = test.scope, test.basis, test.x
			gap, err := TransferGap(s.View, v, q)
			if err != nil || gap != test.gap {
				t.Fatalf("gap=%v err=%v, want %v", gap, err, test.gap)
			}
		})
	}
	q.StartAnchorID, q.BasisAnchorID = boundary, root
	if _, err := TransferGap(s.View, v, q); !errors.Is(err, contract.ErrUnavailable) {
		t.Fatalf("inactive scope became empty: %v", err)
	}
	q.StartAnchorID, q.BasisAnchorID = root, boundary
	if _, err := TransferGap(s.View, v, q); !errors.Is(err, contract.ErrUnavailable) {
		t.Fatalf("inactive basis became valid: %v", err)
	}
}

func TestTransferCoverageCannotProveExclusionPastProjectionLimit(t *testing.T) {
	s := elementfact.New()
	root := scanID(1)
	scanAnchor(t, &s, root, "", 0)
	parent := root
	for i := 0; i < geometry.MaxExactProjectionEdges+1; i++ {
		child := scanID(10 + i)
		scanAnchor(t, &s, child, parent, 0)
		parent = child
	}
	radius, _ := contract.NewBoundedSphere(1)
	v := transferfact.NewVisibility(nil, []transferfact.SourceFootprint{{Boundary: scanID(9000), Origin: contract.SpatialTuple{Parent: parent, LocalPose: geometry.IdentityPose(), Range: radius}}}, nil, nil, nil)
	q := contract.RegionQuery{StartAnchorID: root, BasisAnchorID: root, Sphere: contract.QuerySphere{Center: contract.Vec3{X: 1000}}, Selector: contract.QueryEntities}
	if gap, err := TransferGap(s.View, v, q); err != nil || !gap {
		t.Fatalf("unproved projection exclusion reported complete: %v %v", gap, err)
	}
}

func TestPendingInstalledMembersDoNotConsumePageOrOpenMount(t *testing.T) {
	s := elementfact.New()
	root, pending, hidden, visible := scanID(1), scanID(2), scanID(3), scanID(4)
	scanAnchor(t, &s, root, "", 0)
	scanAnchor(t, &s, pending, root, 0)
	scanEntity(t, &s, hidden, pending, 0, 0)
	scanEntity(t, &s, visible, root, 0, 0)
	mounts := mountfact.New()
	mounts.ReplaceOutgoing(pending, mountfact.Record{State: contract.MountActive})
	visibility := transferfact.NewVisibility(map[values.ElementID]values.ElementID{pending: pending, hidden: pending}, nil, nil, nil, nil)
	q := contract.RegionQuery{StartAnchorID: root, BasisAnchorID: root, Selector: contract.QueryEntities}
	page, err := Coverage(s.View, mounts.View, visibility, [32]byte{}, q, contract.CoreReadCut{}, 1, nil, false)
	if err != nil || len(page.Matches) != 1 || page.Matches[0].ElementID != visible || page.Continuation != nil {
		t.Fatal("pending member consumed page or opened mount", page, err)
	}
	q.BasisAnchorID = pending
	if _, err := Coverage(s.View, mounts.View, visibility, [32]byte{}, q, contract.CoreReadCut{}, 1, nil, false); !errors.Is(err, contract.ErrUnavailable) {
		t.Fatal("pending basis became usable", err)
	}
}
