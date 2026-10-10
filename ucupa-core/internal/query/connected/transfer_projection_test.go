package connected

import (
	"errors"
	"testing"

	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	elementfact "ucupa/ucupa-core/internal/element/fact"
	"ucupa/ucupa-core/internal/geometry"
	mountfact "ucupa/ucupa-core/internal/mount/fact"
	"ucupa/ucupa-core/internal/query/exchange"
	transferfact "ucupa/ucupa-core/internal/transfer/fact"
)

func TestPendingTargetConnectedProjectionDoesNotRouteOrDisclose(t *testing.T) {
	root, pending, visible := reviewID(1), reviewID(2), reviewID(3)
	radius, _ := contract.NewBoundedSphere(10)
	tuple := contract.SpatialTuple{Parent: root, LocalPose: geometry.IdentityPose(), Range: radius}
	elements := worldTestElements([]elementfact.Anchor{{ID: root, Range: geometry.UnboundedRootRange()},
		{ID: pending, Parent: root, LocalPose: tuple.LocalPose, Range: geometry.BoundedRange(radius)}},
		[]elementfact.Entity{{ID: visible, Parent: root, LocalPose: tuple.LocalPose, Range: radius}})
	mounts := mountfact.New()
	mounts.ReplaceOutgoing(pending, mountfact.Record{State: contract.MountActive})
	v := transferfact.NewVisibility(map[values.ElementID]values.ElementID{pending: pending}, []transferfact.SourceFootprint{{Boundary: pending, Origin: tuple}}, nil, nil, nil)
	path := contract.Path{OriginAnchorID: root, Down: []values.ElementID{pending}}
	if _, err := traceConnectedPath(elements, mounts.View, v, path); !errors.Is(err, contract.ErrUnavailable) {
		t.Fatal("pending path resolved", err)
	}
	intent := exchange.Intent{Kind: exchange.Query, Path: contract.Path{OriginAnchorID: root}, Query: contract.PathRegionQuery{
		StartPath: contract.Path{OriginAnchorID: root}, BasisPath: contract.Path{OriginAnchorID: root}}}
	next, _, _, _, err := remoteConnectedPlan(exchange.Facts{Elements: elements, Mount: mounts.View, Transfers: v}, intent)
	if err != nil || next.authority != "" {
		t.Fatal("pending target became route", next, err)
	}
	world := &connectedWorld{elements: elements, mount: mounts.View, transfers: v}
	page := connectedDirectPage(world, &connectedContext{}, root, contract.Path{OriginAnchorID: root}, 1, "")
	if len(page.Records) != 1 || page.Continuation != nil || page.Status != contract.QueryPartial || page.Gap != contract.QueryUnresolvedCoverage {
		t.Fatal("pending target consumed list position", page)
	}
}
