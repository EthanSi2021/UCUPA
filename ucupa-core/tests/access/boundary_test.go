package access_test

import (
	"errors"
	"reflect"
	"testing"

	"ucupa/shared/values"
	corecontract "ucupa/ucupa-core/contract"
)

func TestPublicUpdateNeedsPlaceAndQueryBasisStaysInSelectedScope(t *testing.T) {
	f := newPublicFixture(t, false, "")
	f.permit(t, corecontract.PermissionCreateAnchor, corecontract.PermissionCreateEntity, corecontract.PermissionUpdateElement, corecontract.PermissionQueryRegion)
	f.enter(t, "create-anchor")
	for _, n := range []uint64{2, 3} {
		f.call(t, map[string]any{"op": "create-anchor", "handle": f.handle, "element": string(id(n)), "final": publicFinal(id(1), 0, 10), "operationId": values.EncodePublicBytes([]byte(string(id(n))))})
	}
	made := f.create(t, 4)
	update := map[string]any{"op": "update-entity", "handle": f.handle, "element": string(id(4)), "final": publicFinal(id(2), 1, 1), "expected": made["record"].(map[string]any)["revision"], "operationId": values.EncodePublicBytes([]byte("move"))}
	if _, err := f.request(t, update); !errors.Is(err, values.PublicDenied) {
		t.Fatal(err)
	}
	f.permit(t, corecontract.PermissionPlaceElement)
	f.call(t, update)
	query := map[string]any{"op": "query", "handle": f.handle, "start": publicAddress(id(2)), "basis": publicAddress(id(3)), "upDepth": "0", "sphere": map[string]any{"center": []any{0., 0., 0.}, "radius": 10.}}
	if _, err := f.request(t, query); !errors.Is(err, values.PublicInvalid) {
		t.Fatal(err)
	}
	query["upDepth"] = "-1"
	if r := f.call(t, query); r["status"] != "COMPLETE" {
		t.Fatal(r)
	}
}

func TestPublicRetainedReadBasisCanBeUsedAgainWithoutOwningSecondSnapshot(t *testing.T) {
	f := newPublicFixture(t, true, "")
	f.permit(t, corecontract.PermissionCreateEntity, corecontract.PermissionListDirect, corecontract.PermissionReadElement, corecontract.PermissionDeleteEntity)
	f.enter(t, "list")
	created := f.create(t, 2)
	list := f.call(t, map[string]any{"op": "list", "handle": f.handle, "address": publicAddress()})
	f.call(t, map[string]any{"op": "delete-entity", "handle": f.handle, "element": string(id(2)), "expected": created["record"].(map[string]any)["revision"], "operationId": values.EncodePublicBytes([]byte("delete"))})
	first := f.call(t, map[string]any{"op": "read", "handle": f.handle, "address": publicAddress(id(2)), "basis": list["receipt"]})
	second := f.call(t, map[string]any{"op": "read", "handle": f.handle, "address": publicAddress(id(2)), "basis": first["receipt"]})
	if first["readMode"] != "RETAINED" || !reflect.DeepEqual(first["record"], second["record"]) {
		t.Fatal(first, second)
	}
	f.call(t, map[string]any{"op": "close", "handle": f.handle})
	if _, err := f.request(t, map[string]any{"op": "read", "handle": f.handle, "address": publicAddress(id(2)), "basis": second["receipt"]}); !errors.Is(err, values.PublicInvalidContext) {
		t.Fatal(err)
	}
}

func TestPublicObserverStopsFollowingElementAfterItLeavesClosure(t *testing.T) {
	f := newPublicFixture(t, false, "")
	f.permit(t, corecontract.PermissionCreateAnchor, corecontract.PermissionCreateEntity, corecontract.PermissionUpdateElement, corecontract.PermissionPlaceElement)
	f.enter(t, "create-anchor")
	for _, n := range []uint64{2, 3} {
		f.call(t, map[string]any{"op": "create-anchor", "handle": f.handle, "element": string(id(n)), "final": publicFinal(id(1), 0, 10), "operationId": values.EncodePublicBytes([]byte(string(id(n))))})
	}
	made := f.call(t, map[string]any{"op": "create-entity", "handle": f.handle, "element": string(id(4)), "final": publicFinal(id(2), 0, 1), "operationId": values.EncodePublicBytes([]byte("child"))})
	for _, p := range []values.PermissionID{corecontract.PermissionListDirect, corecontract.PermissionObserveElement} {
		f.grant(t, p, values.AnchorClosureScope(f.c.AuthorityRef(), id(2)), false)
	}
	h := f.call(t, map[string]any{"op": "enter", "intent": "list", "reference": map[string]any{"boundary": map[string]any{"core": string(f.c.AuthorityRef()), "anchor": string(id(2))}, "nextHop": "local:core"}})["handle"]
	prep := f.call(t, map[string]any{"op": "list", "handle": h, "address": publicAddress(), "prepareObserve": true})
	b := f.call(t, map[string]any{"op": "begin-observe", "preparation": prep["preparation"]})
	moved := f.call(t, map[string]any{"op": "update-entity", "handle": f.handle, "element": string(id(4)), "final": publicFinal(id(3), 0, 1), "expected": made["record"].(map[string]any)["revision"], "operationId": values.EncodePublicBytes([]byte("move-out"))})
	batch := f.call(t, map[string]any{"op": "poll", "cursor": b["cursor"]})
	if len(batch["groups"].([]any)) != 1 {
		t.Fatal(batch)
	}
	cursor := batch["nextCursor"]
	f.call(t, map[string]any{"op": "poll", "cursor": cursor})
	f.call(t, map[string]any{"op": "update-entity", "handle": f.handle, "element": string(id(4)), "final": publicFinal(id(3), 2, 1), "expected": moved["record"].(map[string]any)["revision"], "operationId": values.EncodePublicBytes([]byte("outside-update"))})
	empty := f.call(t, map[string]any{"op": "poll", "cursor": cursor})
	if len(empty["groups"].([]any)) != 0 || !reflect.DeepEqual(empty["nextCursor"], cursor) {
		t.Fatal("outside activity disclosed", empty)
	}
}
