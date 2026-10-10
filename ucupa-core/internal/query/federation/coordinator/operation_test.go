package coordinator

import (
	"errors"
	"reflect"
	"testing"

	"ucupa/shared/values"
	accessvalue "ucupa/ucupa-core/internal/access/value"
)

func TestCoordinatorPaginationKeepsModeAndFixedConditions(t *testing.T) {
	for _, retained := range []bool{false, true} {
		t.Run(map[bool]string{false: "current", true: "retained"}[retained], func(t *testing.T) {
			f := newCoordinatorFixture(t, retained)
			f.create(t, 2, 0)
			f.create(t, 3, 1)
			request := map[string]any{"op": "list", "handle": f.handle, "address": map[string]any{}, "limit": float64(1)}
			first, err := f.call(t, "list", request)
			if err != nil || first["continuation"] == nil {
				t.Fatalf("first: %v %v", first, err)
			}
			if first["status"] != "PARTIAL" || len(first["records"].([]any)) != 1 {
				t.Fatal(first)
			}
			f.move(t, 3, 7)
			request["continuation"] = first["continuation"]
			request["limit"] = float64(2)
			second, err := f.call(t, "list", request)
			if !retained {
				if !errors.Is(err, values.PublicInvalidContext) {
					t.Fatalf("current changed cut: %v %v", second, err)
				}
				return
			}
			if err != nil || second["readMode"] != "RETAINED" || second["status"] != "COMPLETE" || second["continuation"] != nil {
				t.Fatalf("second: %v %v", second, err)
			}
			rows := second["records"].([]any)
			if len(rows) != 1 {
				t.Fatal(rows)
			}
			row := rows[0].(map[string]any)
			if row["element"] != string(coordinatorID(3)) || row["pose"].(map[string]any)["translation"].([]any)[0] != float64(1) {
				t.Fatal("mixed latest value into retained page", row)
			}
			// Modifying a query condition does not turn a continuation into a new query.
			request["address"] = map[string]any{"down": []any{string(coordinatorID(2))}}
			if _, err := f.call(t, "list", request); !errors.Is(err, values.PublicInvalidContext) {
				t.Fatal("changed conditions reused receipt", err)
			}
		})
	}
}

func TestCoordinatorReadAtKeepsIndependentPermissionAndOldValue(t *testing.T) {
	f := newCoordinatorFixture(t, true)
	f.create(t, 2, 1)
	query := map[string]any{"op": "query", "handle": f.handle, "start": map[string]any{}, "basis": map[string]any{}, "sphere": map[string]any{"center": []any{float64(0), float64(0), float64(0)}, "radius": float64(10)}, "selector": "entities"}
	page, err := f.call(t, "query", query)
	if err != nil {
		t.Fatal(err)
	}
	read := map[string]any{"op": "read", "handle": f.handle, "basis": page["receipt"], "address": map[string]any{"down": []any{string(coordinatorID(2))}}}
	f.policy.denyRead = true
	if _, err := f.call(t, "read", read); !errors.Is(err, values.PublicDenied) {
		t.Fatal("query permission upgraded to read", err)
	}
	f.policy.denyRead = false
	first, err := f.call(t, "read", read)
	if err != nil {
		t.Fatal(err)
	}
	f.move(t, 2, 8)
	second, err := f.call(t, "read", read)
	if err != nil || !reflect.DeepEqual(first["record"], second["record"]) || !reflect.DeepEqual(first["attachment"], second["attachment"]) {
		t.Fatalf("derived basis changed: %v %v", second, err)
	}
	if second["record"].(map[string]any)["pose"].(map[string]any)["translation"].([]any)[0] != float64(1) {
		t.Fatal(second)
	}
}

func TestCoordinatorObservationUsesSharedPublicationAndRelease(t *testing.T) {
	f := newCoordinatorFixture(t, true)
	f.create(t, 2, 0)
	page, err := f.call(t, "list", map[string]any{"op": "list", "handle": f.handle, "address": map[string]any{}, "prepareObserve": true})
	if err != nil || page["preparation"] == nil {
		t.Fatalf("%v %v", page, err)
	}
	f.move(t, 2, 4)
	begin, err := f.call(t, "begin-observe", map[string]any{"op": "begin-observe", "preparation": page["preparation"]})
	if err != nil {
		t.Fatal(err)
	}
	poll := map[string]any{"op": "poll", "cursor": begin["cursor"]}
	first, err := f.call(t, "poll", poll)
	if err != nil || len(first["groups"].([]any)) != 1 {
		t.Fatalf("%v %v", first, err)
	}
	repeat, err := f.call(t, "poll", poll)
	if err != nil || !reflect.DeepEqual(first, repeat) {
		t.Fatal("poll retry changed", repeat, err)
	}
	first["groups"].([]any)[0].(map[string]any)["category"].([]any)[0] = "corrupt"
	again, err := f.call(t, "poll", poll)
	if err != nil || !reflect.DeepEqual(again, repeat) {
		t.Fatal("result mutated queue", again, err)
	}
	ack, err := f.call(t, "poll", map[string]any{"op": "poll", "cursor": repeat["nextCursor"]})
	if err != nil || len(ack["groups"].([]any)) != 0 {
		t.Fatal("ack failed", ack, err)
	}
	release := map[string]any{"op": "release-observe", "cursor": ack["nextCursor"]}
	if _, err := f.call(t, "release-observe", release); err != nil {
		t.Fatal(err)
	}
	if _, err := f.call(t, "release-observe", release); err != nil {
		t.Fatal("release not idempotent", err)
	}
	if _, err := f.call(t, "poll", poll); !errors.Is(err, values.PublicGap) {
		t.Fatal("released cursor remained valid", err)
	}
}

func TestCoordinatorRemovedSpacePreventsDisclosure(t *testing.T) {
	f := newCoordinatorFixture(t, true)
	f.create(t, 2, 0)
	token, err := accessvalue.Token(f.handle, "space-handle")
	if err != nil {
		t.Fatal(err)
	}
	f.ops.owner.Lock()
	handle, err := f.ops.registry.ResolveSpaceLocked(f.caller, token)
	f.ops.registry.RemoveLocked(handle.Ref)
	f.ops.owner.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.call(t, "list", map[string]any{"op": "list", "handle": f.handle, "address": map[string]any{}}); !errors.Is(err, values.PublicInvalidContext) {
		t.Fatal("removed handle disclosed", err)
	}
}
