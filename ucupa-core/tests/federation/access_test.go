package federation_test

import (
	"errors"
	"testing"
	"ucupa/shared/values"
)

func TestFederationPublicNestedQueryReadListAndClose(t *testing.T) {
	f, connections, caller := vectorFixture(t, true)
	m, n, leaf, sibling := storeTestID(9710), storeTestID(9720), storeTestID(9750), storeTestID(9701)
	if _, err := f.owners[2].Elements().CreateEntity(storeTestActor, leaf, storeTestTuple(t, n, 0, 1), "public-leaf"); err != nil {
		t.Fatal(err)
	}
	if _, err := f.owners[0].Elements().CreateEntity(storeTestActor, sibling, storeTestTuple(t, f.root, 0, 1), "public-sibling"); err != nil {
		t.Fatal(err)
	}
	handle := federatedTestHandle(t, f, caller, connections, "query")
	request := map[string]any{"op": "query", "handle": handle, "start": pathJSON(f.root, []values.ElementID{m, n}), "basis": pathJSON(f.root, nil), "upDepth": "-1", "selector": "entities", "sphere": map[string]any{"center": []any{0., 0., 0.}, "radius": 100.}, "limit": float64(1)}
	first, err := federatedTestCall(t, f.owners[0], caller, connections, request)
	if err != nil {
		t.Fatal(err)
	}
	if first["scope"] != string(f.root) || first["actualUpDepth"] != "2" || first["status"] != "PARTIAL" || first["readMode"] != "RETAINED" {
		t.Fatal(first)
	}
	row := first["matches"].([]any)[0].(map[string]any)
	if row["element"] != string(sibling) {
		t.Fatal("not UUID order", row)
	}
	request["continuation"] = first["continuation"]
	second, err := federatedTestCall(t, f.owners[0], caller, connections, request)
	if err != nil {
		t.Fatal(err)
	}
	if second["status"] != "COMPLETE" || len(second["matches"].([]any)) != 1 {
		t.Fatal(second)
	}
	if first["closure"] == nil || first["closure"] != second["closure"] {
		t.Fatal("pagination changed source commitment", first, second)
	}
	stamp, err := values.DecodePublicBytes(first["closure"].(string))
	if err != nil || len(stamp) != 32 || [32]byte(stamp) == ([32]byte{}) {
		t.Fatal("invalid aggregate commitment", err)
	}
	row = second["matches"].([]any)[0].(map[string]any)
	if row["element"] != string(leaf) {
		t.Fatal(row)
	}
	read, err := federatedTestCall(t, f.owners[0], caller, connections, map[string]any{"op": "read", "handle": handle, "address": row["address"]})
	if err != nil || read["state"] != "PRESENT" || read["attachment"] == nil {
		t.Fatalf("read: %+v %v", read, err)
	}
	if read["record"].(map[string]any)["revision"].(map[string]any)["owner"] != string(f.owners[2].AuthorityRef()) {
		t.Fatal("source identity lost", read)
	}
	list, err := federatedTestCall(t, f.owners[0], caller, connections, map[string]any{"op": "list", "handle": handle, "address": pathJSON(f.root, []values.ElementID{m, n})})
	if err != nil || list["status"] != "COMPLETE" || len(list["records"].([]any)) != 1 {
		t.Fatalf("list: %+v %v", list, err)
	}
	if _, err := federatedTestCall(t, f.owners[0], caller, connections, map[string]any{"op": "close", "handle": handle}); err != nil {
		t.Fatal(err)
	}
	connections.requireReleased(t)
}

func TestFederationPublicCurrentContinuationDoesNotReadLatest(t *testing.T) {
	f, connections, caller := vectorFixture(t, false)
	handle := federatedTestHandle(t, f, caller, connections, "query")
	request := map[string]any{"op": "query", "handle": handle, "start": pathJSON(f.root, nil), "basis": pathJSON(f.root, nil), "sphere": map[string]any{"center": []any{0., 0., 0.}, "radius": 100.}, "limit": float64(1)}
	first, err := federatedTestCall(t, f.owners[0], caller, connections, request)
	if err != nil {
		t.Fatal(err)
	}
	if first["readMode"] != "CURRENT_VERIFIED" || first["continuation"] == nil {
		t.Fatal(first)
	}
	if _, err := f.owners[2].Elements().CreateEntity(storeTestActor, storeTestID(9752), storeTestTuple(t, storeTestID(9720), 0, 1), "after-first-page"); err != nil {
		t.Fatal(err)
	}
	request["continuation"] = first["continuation"]
	if _, err := federatedTestCall(t, f.owners[0], caller, connections, request); !errors.Is(err, values.PublicInvalidContext) {
		t.Fatal("silently changed source cut", err)
	}
}

func TestFederationPublicMissingRelevantBranchIsPartial(t *testing.T) {
	f, connections, caller := vectorFixture(t, true)
	delete(connections.owners, nestedMountAddress(2))
	handle := federatedTestHandle(t, f, caller, connections, "query")
	request := map[string]any{"op": "query", "handle": handle, "start": pathJSON(f.root, nil), "basis": pathJSON(f.root, nil), "sphere": map[string]any{"center": []any{0., 0., 0.}, "radius": 100.}}
	result, err := federatedTestCall(t, f.owners[0], caller, connections, request)
	if err != nil || result["status"] != "PARTIAL" || result["gap"] != "UNRESOLVED_COVERAGE" {
		t.Fatalf("undefined partial: %+v %v", result, err)
	}
	request["start"] = pathJSON(f.root, []values.ElementID{storeTestID(9710), storeTestID(9720), storeTestID(9730)})
	if _, err := federatedTestCall(t, f.owners[0], caller, connections, request); !errors.Is(err, values.PublicUnavailable) {
		t.Fatal("missing semantic start produced result", err)
	}
}
