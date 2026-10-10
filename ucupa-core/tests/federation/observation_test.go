package federation_test

import (
	"errors"
	"reflect"
	"testing"
	"ucupa/shared/values"
)

func TestFederationPublicObservationPreparedQueryAndList(t *testing.T) {
	for _, operation := range []string{"query", "list"} {
		t.Run(operation, func(t *testing.T) {
			f, connections, caller := vectorFixture(t, false)
			handle := federatedTestHandle(t, f, caller, connections, operation)
			call := func(request map[string]any) map[string]any {
				t.Helper()
				result, err := federatedTestCall(t, f.owners[0], caller, connections, request)
				if err != nil {
					t.Fatal(request["op"], err)
				}
				return result
			}
			request := map[string]any{"op": operation, "handle": handle, "prepareObserve": true}
			if operation == "query" {
				request["start"], request["basis"] = pathJSON(f.root, nil), pathJSON(f.root, nil)
				request["sphere"] = map[string]any{"center": []any{0., 0., 0.}, "radius": 100.}
			} else {
				request["address"] = pathJSON(f.root, []values.ElementID{storeTestID(9710), storeTestID(9720)})
			}
			initial := call(request)
			if initial["preparation"] == nil {
				t.Fatal("no public preparation", initial)
			}
			for i := 0; i < 3; i++ {
				observationVectorWrite(t, f, i, uint64(9850+i))
			}
			begin := map[string]any{"op": "begin-observe", "preparation": initial["preparation"]}
			cursor := call(begin)["cursor"]
			if !reflect.DeepEqual(cursor, call(begin)["cursor"]) {
				t.Fatal("Begin not idempotent")
			}
			poll := map[string]any{"op": "poll", "cursor": cursor, "limit": float64(1)}
			first := call(poll)
			if len(first["groups"].([]any)) != 1 {
				t.Fatal("missing initial-cut change", first)
			}
			poll["limit"] = float64(3)
			larger := call(poll)
			if len(larger["groups"].([]any)) != 3 || !reflect.DeepEqual(larger["groups"].([]any)[0], first["groups"].([]any)[0]) {
				t.Fatal("retry changed prefix", larger)
			}
			for i, raw := range larger["groups"].([]any) {
				group := raw.(map[string]any)
				if group["owner"] != string(f.owners[i].AuthorityRef()) || group["changeId"].(map[string]any)["owner"] != group["owner"] {
					t.Fatal("source rewritten", group)
				}
			}
			poll["cursor"] = larger["nextCursor"]
			if len(call(poll)["groups"].([]any)) != 0 {
				t.Fatal("ack replayed source groups")
			}
			if !reflect.DeepEqual(cursor, call(begin)["cursor"]) {
				t.Fatal("Begin renewed original cursor")
			}
			if _, err := federatedTestCall(t, f.owners[0], caller, connections, map[string]any{"op": "poll", "cursor": cursor}); !errors.Is(err, values.PublicGap) {
				t.Fatal("acknowledged cursor reused", err)
			}
			call(map[string]any{"op": "release-observe", "preparation": initial["preparation"]})
			call(map[string]any{"op": "close", "handle": handle})
			connections.requireReleased(t)
		})
	}
}

func TestFederationPublicPreparedQueryRequiresClosedSources(t *testing.T) {
	f, connections, caller := vectorFixture(t, false)
	delete(connections.owners, nestedMountAddress(2))
	handle := federatedTestHandle(t, f, caller, connections, "query")
	request := map[string]any{"op": "query", "handle": handle, "prepareObserve": true, "start": pathJSON(f.root, nil), "basis": pathJSON(f.root, nil), "sphere": map[string]any{"center": []any{0., 0., 0.}, "radius": 100.}}
	if _, err := federatedTestCall(t, f.owners[0], caller, connections, request); !errors.Is(err, values.PublicUnavailable) {
		t.Fatal("unobserved branch accepted", err)
	}
	if _, err := federatedTestCall(t, f.owners[0], caller, connections, map[string]any{"op": "close", "handle": handle}); err != nil {
		t.Fatal(err)
	}
	connections.requireReleased(t)
}

func TestFederationPublicPreparedReadTracksOnlyPathAndTarget(t *testing.T) {
	f, connections, caller := vectorFixture(t, false)
	m, n, leaf := storeTestID(9710), storeTestID(9720), storeTestID(9880)
	created, err := f.owners[2].Elements().CreateEntity(storeTestActor, leaf, storeTestTuple(t, n, 0, 1), "observe-exact-leaf")
	if err != nil {
		t.Fatal(err)
	}
	handle := federatedTestHandle(t, f, caller, connections, "read")
	call := func(request map[string]any) map[string]any {
		t.Helper()
		result, err := federatedTestCall(t, f.owners[0], caller, connections, request)
		if err != nil {
			t.Fatal(request["op"], err)
		}
		return result
	}
	read := call(map[string]any{"op": "read", "handle": handle, "address": pathJSON(f.root, []values.ElementID{m, n, leaf}), "prepareObserve": true})
	if _, err := federatedTestCall(t, f.owners[0], caller, connections, map[string]any{"op": "read", "handle": handle, "address": pathJSON(f.root, []values.ElementID{m, n, leaf}), "basis": read["receipt"], "prepareObserve": true}); !errors.Is(err, values.PublicInvalid) {
		t.Fatal("history retroactively prepared", err)
	}
	for i := 0; i < 3; i++ {
		observationVectorWrite(t, f, i, uint64(9881+i))
	}
	begin := call(map[string]any{"op": "begin-observe", "preparation": read["preparation"]})
	poll := map[string]any{"op": "poll", "cursor": begin["cursor"]}
	if len(call(poll)["groups"].([]any)) != 0 {
		t.Fatal("unrelated change frequency leaked")
	}
	if _, err := f.owners[2].Elements().UpdateEntity(storeTestActor, leaf, storeTestTuple(t, n, 1, 1), created.Record.Entity.Revision, "observe-exact-move"); err != nil {
		t.Fatal(err)
	}
	page := call(poll)
	if len(page["groups"].([]any)) != 1 || page["groups"].([]any)[0].(map[string]any)["owner"] != string(f.owners[2].AuthorityRef()) {
		t.Fatal("target change missing", page)
	}
	poll["cursor"] = page["nextCursor"]
	call(poll)
	// The mount anchor is owned by A, not its carrier B. Moving that anchor
	// changes the disclosed path while leaving the remote entity unchanged.
	anchor, err := f.owners[0].Elements().ReadElement(storeTestActor, m, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.owners[0].Elements().UpdateAnchor(storeTestActor, m, storeTestTuple(t, f.root, 1, 20), anchor.Record.Anchor.Revision, "observe-path-move"); err != nil {
		t.Fatal(err)
	}
	page = call(poll)
	if len(page["groups"].([]any)) != 1 || page["groups"].([]any)[0].(map[string]any)["owner"] != string(f.owners[0].AuthorityRef()) {
		t.Fatal("ancestor path change missing", page)
	}
	call(map[string]any{"op": "close", "handle": handle})
	connections.requireReleased(t)
}
