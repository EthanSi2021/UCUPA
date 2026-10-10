package coordinator

import (
	"errors"
	"testing"

	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	federationcontract "ucupa/ucupa-core/contract/federation"
	"ucupa/ucupa-core/internal/query/federation/collect"
)

func TestCoordinatorDecodingKeepsDepthAndAddressContract(t *testing.T) {
	root := coordinatorID(1)
	request := map[string]any{"start": map[string]any{}, "basis": map[string]any{}, "upDepth": "-1", "sphere": map[string]any{"center": []any{float64(0), float64(0), float64(0)}, "radius": float64(2)}}
	got, err := decodeFederatedCollectRequest("query", root, request)
	if err != nil || got.Query.UpDepth != -1 || got.Query.Start.Origin != root || got.Query.Basis.Origin != root || got.Query.Selector != contract.QueryBoth {
		t.Fatalf("%+v %v", got, err)
	}
	for _, bad := range []any{"-2", "1.0", float64(1)} {
		request["upDepth"] = bad
		if _, err := decodeFederatedCollectRequest("query", root, request); !errors.Is(err, values.PublicInvalid) {
			t.Fatal("invalid depth accepted", bad, err)
		}
	}
	delete(request, "upDepth")
	for _, bad := range []any{map[string]any{"up": "-1"}, map[string]any{"origin": "not-an-id"}, map[string]any{"down": []any{float64(1)}}, map[string]any{"other": true}} {
		if _, err := decodeFederatedAddress(bad, root); !errors.Is(err, values.PublicInvalid) {
			t.Fatal("invalid address accepted", bad, err)
		}
	}
	if _, err := decodeFederatedCollectRequest("unknown", root, request); !errors.Is(err, values.PublicInvalid) {
		t.Fatal(err)
	}
	request["selector"] = "unsupported"
	if _, err := decodeFederatedCollectRequest("query", root, request); !errors.Is(err, values.PublicInvalid) {
		t.Fatal(err)
	}
}

func TestCoordinatorPageAndSourceDisclosureKeepCanonicalShape(t *testing.T) {
	root := coordinatorID(1)
	page := collect.Page{Partial: true, Gap: contract.QueryExactWorkLimit, Scope: root, ActualUpDepth: 3}
	encoded, err := values.CanonicalJSONValue(federatedPageJSON("query", root, page))
	want := `{"actualUpDepth":"3","gap":"EXACT_WORK_LIMIT","matches":[],"scope":"53000000-0000-4000-8000-000000000001","status":"PARTIAL"}`
	if err != nil || string(encoded) != want {
		t.Fatalf("%s %v", encoded, err)
	}
	graph := federatedSourceGraphJSON(collect.SourceGraph{Operation: "query", Purpose: [32]byte{1}})
	if graph["operation"] != "query" || graph["purpose"] != values.EncodePublicBytes([]byte{1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}) || len(graph["sources"].([]any)) != 0 {
		t.Fatal(graph)
	}
	sphere, _ := contract.NewBoundedSphere(1)
	rangeValue, err := federationcontract.NewFederationRange(contract.RangeBoundedSphere, sphere)
	if err != nil {
		t.Fatal(err)
	}
	row := federationcontract.FederationRecord{ID: coordinatorID(2), Kind: contract.ElementEntity, Range: rangeValue}
	record := federatedRecordJSON(row)
	if record["kind"] != "entity" || record["element"] != string(row.ID) || record["pose"] != nil || record["parent"] != nil || record["range"].(map[string]any)["radius"] != float64(1) {
		t.Fatal(record)
	}
	attachment := federatedAttachmentJSON(contract.PublicFacetInspection{})
	if attachment["state"] != "NONE" || attachment["key"] != nil || attachment["nextHop"] != nil {
		t.Fatal(attachment)
	}
	retired := federatedAttachmentJSON(contract.PublicFacetInspection{
		Element:         coordinatorID(2),
		State:           contract.FacetRetired,
		Target:          values.FactAuthorityRef("53000000-0000-4000-8000-000000000003"),
		KeyCore:         values.FactAuthorityRef("53000000-0000-4000-8000-000000000004"),
		Proposal:        values.PublicToken{Kind: "attachment-revision", Owner: "53000000-0000-4000-8000-000000000004", Value: "disposed-lineage-proposal"},
		ContactRevision: values.PublicToken{Kind: "contact-revision", Owner: "53000000-0000-4000-8000-000000000004", Value: "unused"},
	})
	if retired["state"] != "RETIRED" || retired["nextHop"] != nil || retired["contactRevision"] != nil {
		t.Fatal(retired)
	}
	key, _ := retired["key"].(map[string]any)
	if key == nil || key["core"] != "53000000-0000-4000-8000-000000000004" || key["facet"] != "53000000-0000-4000-8000-000000000003" || key["element"] != string(coordinatorID(2)) {
		t.Fatal(retired)
	}
}
