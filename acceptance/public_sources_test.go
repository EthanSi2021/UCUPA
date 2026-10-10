package acceptance_test

import (
	"context"
	"errors"
	"testing"

	"ucupa/shared/values"
	corecontract "ucupa/ucupa-core/contract"
	facetcontract "ucupa/ucupa-facet/contract"
)

func TestP5OptionalFacetSourcesRemainIndependentlyOwnedStreams(t *testing.T) {
	w := newPublicFacetWorld(t)
	const second values.ElementID = "00000000-0000-4000-8000-00000000fa52"
	c := w.cores[0]
	w.grant(t, w.actor, corecontract.PermissionCreateEntity, values.CoreElementScope(c.AuthorityRef(), publicRoot))
	rotation, _ := corecontract.NewRotation(1, 0, 0, 0)
	pose, _ := corecontract.NewPose(corecontract.Vec3{}, rotation)
	sphere, _ := corecontract.NewBoundedSphere(1)
	if _, err := c.Elements().CreateEntity(w.actor, second, corecontract.SpatialTuple{Parent: publicRoot, LocalPose: pose, Range: sphere}, "second-source"); err != nil {
		t.Fatal(err)
	}
	w.attach(t, 0, 1, second)
	w.grant(t, w.service, corecontract.PermissionObserveAttachment, values.CoreElementScope(c.AuthorityRef(), second))
	w.grant(t, w.actor, facetcontract.PermissionObserveKey, values.FacetKeyScope(w.facets[1].AuthorityRef(), second, w.typ))
	coreCall := func(r map[string]any) map[string]any {
		t.Helper()
		raw, e := c.Access().PublicCall(context.Background(), w.caller, wireJSON(t, r))
		result, e := decodeWire(t, raw, e)
		if e != nil {
			t.Fatal(e)
		}
		return result
	}
	h := coreCall(map[string]any{"op": "enter", "intent": "read", "reference": map[string]any{"boundary": map[string]any{"core": string(c.AuthorityRef()), "anchor": string(publicRoot)}, "nextHop": "local:core0"}})["handle"]
	read := coreCall(map[string]any{"op": "read", "handle": h, "address": map[string]any{"down": []any{string(second)}}})
	a := read["attachment"].(map[string]any)
	access := map[string]any{"key": a["key"], "nextHop": a["nextHop"]}
	otherCall := func(r map[string]any) map[string]any {
		t.Helper()
		raw, e := w.facetLinks.PublicCall(context.Background(), "local:facet1", w.facets[1].AuthorityRef(), w.caller, wireJSON(t, r))
		result, e := decodeWire(t, raw, e)
		if e != nil {
			t.Fatal(e)
		}
		assertPublicSchema(t, "ucupa-facet", "request", r)
		assertPublicSchema(t, "ucupa-facet", "response", result)
		return result
	}
	first := w.get(t, true)
	other := otherCall(map[string]any{"op": "get", "access": access, "typeRef": values.EncodePublicBytes(w.typ.Bytes()), "prepareObserve": true})
	if _, e := w.request(t, map[string]any{"op": "begin-observe", "preparation": other["preparation"]}); !errors.Is(e, values.PublicInvalid) {
		t.Fatal("cross-owner token", e)
	}
	firstCursor := w.call(t, map[string]any{"op": "begin-observe", "preparation": first["preparation"]})["cursor"]
	otherCursor := otherCall(map[string]any{"op": "begin-observe", "preparation": other["preparation"]})["cursor"]
	otherCall(map[string]any{"op": "put", "access": access, "typeRef": values.EncodePublicBytes(w.typ.Bytes()), "payload": values.EncodePublicBytes([]byte("1")), "expected": map[string]any{"absent": true}, "operationId": "c291cmNlMg"})
	batch := otherCall(map[string]any{"op": "poll", "cursor": otherCursor})
	groups := batch["groups"].([]any)
	if len(groups) != 1 || groups[0].(map[string]any)["owner"] != string(w.facets[1].AuthorityRef()) {
		t.Fatal(batch)
	}
	empty := w.call(t, map[string]any{"op": "poll", "cursor": firstCursor})
	if len(empty["groups"].([]any)) != 0 {
		t.Fatal("unrelated source changed stream", empty)
	}
}
