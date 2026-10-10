package local

import (
	"testing"

	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	"ucupa/ucupa-core/internal/commit"
	elementfact "ucupa/ucupa-core/internal/element/fact"
	facetfact "ucupa/ucupa-core/internal/facet/fact"
	"ucupa/ucupa-core/internal/geometry"
	mountfact "ucupa/ucupa-core/internal/mount/fact"
	"ucupa/ucupa-core/internal/token"
	transferfact "ucupa/ucupa-core/internal/transfer/fact"
)

func TestPendingTargetIsHiddenFromPublicLocalProjection(t *testing.T) {
	root := values.ElementID("00000000-0000-4000-8000-000000000001")
	pending := values.ElementID("00000000-0000-4000-8000-000000000002")
	visible := values.ElementID("00000000-0000-4000-8000-000000000003")
	radius, _ := contract.NewBoundedSphere(1)
	tuple := contract.SpatialTuple{Parent: root, LocalPose: geometry.IdentityPose(), Range: radius}
	b := elementfact.NewProjection()
	b.AddAnchor(root, elementfact.Anchor{ID: root, Range: geometry.UnboundedRootRange()})
	b.AddAnchor(pending, elementfact.Anchor{ID: pending, Parent: root, LocalPose: tuple.LocalPose, Range: geometry.BoundedRange(radius)})
	b.AddEntity(visible, elementfact.Entity{ID: visible, Parent: root, LocalPose: tuple.LocalPose, Range: radius})
	attachments := facetfact.New()
	for _, id := range []values.ElementID{root, pending, visible} {
		attachments.ElementCreated(id, token.FromParts([32]byte{1}, 3))
	}
	v := commit.ReadView{Authority: values.FactAuthorityRef(root), Elements: b.Build().View, Mount: mountfact.New().View, Attachments: attachments.View,
		Transfers: transferfact.NewVisibility(map[values.ElementID]values.ElementID{pending: pending}, []transferfact.SourceFootprint{{Boundary: pending, Origin: tuple}}, nil, nil, nil)}
	path := map[string]any{"down": []any{string(pending)}}
	if _, err := publicAddressForViews(v.Elements, v.Mount, v.Transfers, root, path); err != values.PublicUnavailable {
		t.Fatal("pending address resolved", err)
	}
	signer := token.NewPublicSigner([32]byte{1}, v.Authority)
	page, last, err := publicListJSON(v, signer, root, map[string]any{}, "", 1)
	if err != nil || page["status"] != "PARTIAL" || last != "" || len(page["records"].([]any)) != 1 {
		t.Fatal("pending list leaked or consumed position", page, err)
	}
	request := map[string]any{"start": map[string]any{}, "basis": map[string]any{}, "selector": "both",
		"sphere": map[string]any{"center": []any{float64(0), float64(0), float64(0)}, "radius": float64(100)}}
	page, last, err = publicQueryJSON(v, signer, root, request, "", 10)
	if err != nil || page["status"] != "PARTIAL" || page["gap"] != "UNRESOLVED_COVERAGE" || last != "" || len(page["matches"].([]any)) != 2 {
		t.Fatal("pending query lost coverage", page, err)
	}
}
