package acceptance_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"

	"ucupa/composition"
	"ucupa/shared/values"
	corecontract "ucupa/ucupa-core/contract"
	facetcontract "ucupa/ucupa-facet/contract"
	facet "ucupa/ucupa-facet/instance"
	realm "ucupa/ucupa-realm/contract"
)

const publicRoot values.ElementID = "00000000-0000-4000-8000-00000000fa51"

type publicFacetWorld struct {
	*facetWorld
	caller values.CallerContext
	access map[string]any
	key    values.FacetAcceptanceKey
	typ    values.FacetTypeRef
}

func publicPolicy() *values.PublicAccessPolicy {
	return &values.PublicAccessPolicy{ContextTTL: time.Minute, ObservationTTL: time.Minute}
}
func wireJSON(t *testing.T, value any) []byte {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	v, err := values.ParsePublicJSON(raw, values.PublicJSONLimits())
	if err != nil {
		t.Fatal(err)
	}
	raw, err = values.CanonicalJSONValue(v)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}
func decodeWire(t *testing.T, raw []byte, err error) (map[string]any, error) {
	t.Helper()
	if err != nil {
		if raw != nil {
			t.Fatal("error disclosed body")
		}
		return nil, err
	}
	v, e := values.ParseCanonicalJSON(raw, values.PublicJSONLimits())
	if e != nil {
		t.Fatal(e)
	}
	return v.(map[string]any), nil
}
func newPublicFacetWorld(t *testing.T) *publicFacetWorld {
	return newPublicFacetWorldCredentials(t, nil)
}
func newPublicFacetWorldCredentials(t *testing.T, credentials []realm.CredentialBootstrap) *publicFacetWorld {
	t.Helper()
	w := &publicFacetWorld{facetWorld: newFacetWorldCredentials(t, corecontract.Config{PublicAccessPolicy: publicPolicy()}, facet.Config{PublicAccessPolicy: publicPolicy()}, credentials), typ: fixtureType("content")}
	w.root(t, 0, publicRoot)
	w.key = w.attach(t, 0, 0, publicRoot)
	w.grant(t, w.service, corecontract.PermissionObserveAttachment, values.CoreElementScope(w.cores[0].AuthorityRef(), publicRoot))
	w.grant(t, w.actor, facetcontract.PermissionObserveKey, values.FacetKeyScope(w.facets[0].AuthorityRef(), publicRoot, w.typ))
	w.grant(t, w.actor, facetcontract.PermissionObserveElement, values.FacetElementScope(w.facets[0].AuthorityRef(), publicRoot))
	w.grant(t, w.actor, corecontract.PermissionReadElement, values.AnchorClosureScope(w.cores[0].AuthorityRef(), publicRoot))
	var err error
	w.caller, err = w.policy.TrustLocalCaller(w.actor)
	if err != nil {
		t.Fatal(err)
	}
	links := &composition.LocalCoreConnections{}
	if err = links.BindEndpoint("local:entry", composition.CoreEndpoint{Read: w.cores[0].Elements(), Public: w.cores[0].Access()}); err != nil {
		t.Fatal(err)
	}
	call := func(request map[string]any) map[string]any {
		t.Helper()
		raw, err := links.PublicCall(context.Background(), "local:entry", w.cores[0].AuthorityRef(), w.caller, wireJSON(t, request))
		r, e := decodeWire(t, raw, err)
		if e != nil {
			t.Fatal(e)
		}
		assertPublicSchema(t, "ucupa-core", "request", request)
		assertPublicSchema(t, "ucupa-core", "response", r)
		return r
	}
	entered := call(map[string]any{"op": "enter", "reference": map[string]any{"boundary": map[string]any{"core": string(w.cores[0].AuthorityRef()), "anchor": string(publicRoot)}, "nextHop": "local:entry"}, "intent": "read"})
	read := call(map[string]any{"op": "read", "handle": entered["handle"], "address": map[string]any{"down": []any{}}})
	attachment := read["attachment"].(map[string]any)
	w.access = map[string]any{"key": attachment["key"], "nextHop": attachment["nextHop"]}
	return w
}
func (w *publicFacetWorld) request(t *testing.T, request map[string]any) (map[string]any, error) {
	t.Helper()
	raw, err := w.facetLinks.PublicCall(context.Background(), "local:facet0", w.facets[0].AuthorityRef(), w.caller, wireJSON(t, request))
	return decodeWire(t, raw, err)
}
func (w *publicFacetWorld) call(t *testing.T, request map[string]any) map[string]any {
	t.Helper()
	r, e := w.request(t, request)
	if e != nil {
		t.Fatalf("%s: %v", request["op"], e)
	}
	assertPublicSchema(t, "ucupa-facet", "request", request)
	assertPublicSchema(t, "ucupa-facet", "response", r)
	return r
}
func (w *publicFacetWorld) get(t *testing.T, prepare bool) map[string]any {
	return w.call(t, map[string]any{"op": "get", "access": w.access, "typeRef": values.EncodePublicBytes(w.typ.Bytes()), "prepareObserve": prepare})
}
func (w *publicFacetWorld) putRequest(id, payload string, expected any) map[string]any {
	return map[string]any{"op": "put", "access": w.access, "typeRef": values.EncodePublicBytes(w.typ.Bytes()), "expected": expected, "operationId": values.EncodePublicBytes([]byte(id)), "payload": values.EncodePublicBytes([]byte(payload))}
}

func TestP5PublicFacetPreparedGaplessRead(t *testing.T) {
	w := newPublicFacetWorld(t)
	prepared := w.get(t, true)
	if prepared["state"] != "NOT_FOUND" {
		t.Fatal(prepared)
	}
	first := w.putRequest("initial", "\"hello\"", map[string]any{"absent": true})
	put := w.call(t, first)
	if put["status"] != "COMMITTED" {
		t.Fatal(put)
	}
	if replay := w.call(t, first); !reflect.DeepEqual(put, replay) {
		t.Fatal("exact replay changed")
	}
	begun := w.call(t, map[string]any{"op": "begin-observe", "preparation": prepared["preparation"]})
	poll := map[string]any{"op": "poll", "cursor": begun["cursor"]}
	batch := w.call(t, poll)
	if len(batch["groups"].([]any)) != 1 {
		t.Fatal(batch)
	}
	if repeat := w.call(t, poll); !reflect.DeepEqual(batch, repeat) {
		t.Fatal("retry changed group identity")
	}
	empty := w.call(t, map[string]any{"op": "poll", "cursor": batch["nextCursor"]})
	if len(empty["groups"].([]any)) != 0 || !reflect.DeepEqual(empty["nextCursor"], batch["nextCursor"]) {
		t.Fatal(empty)
	}
	if _, err := w.request(t, poll); !errors.Is(err, values.PublicGap) {
		t.Fatalf("acknowledged cursor: %v", err)
	}
	listed := w.call(t, map[string]any{"op": "list-types", "access": w.access})
	if len(listed["types"].([]any)) != 1 {
		t.Fatal(listed)
	}
	got := w.get(t, false)
	if got["revision"] == nil || got["payload"] != values.EncodePublicBytes([]byte("\"hello\"")) {
		t.Fatal(got)
	}
	for _, reply := range []map[string]any{prepared, batch, listed, got} {
		encoded := wireJSON(t, reply)
		for _, private := range []string{"CoreCut", "AuthorizationView", "corePreparation", "coreCursor", "Principal", "PublicKey", "AcceptedAt"} {
			if bytes.Contains(encoded, []byte(private)) {
				t.Fatal("private service evidence disclosed", private)
			}
		}
	}
	if w.policy.Authorize(w.actor, values.Requirement{Permission: corecontract.PermissionObserveAttachment, Scope: values.CoreElementScope(w.cores[0].AuthorityRef(), publicRoot)}) {
		t.Fatal("client unexpectedly has service permission")
	}
}

func TestP7PublicFacetOpaquePayloadAndCapacity(t *testing.T) {
	w := newPublicFacetWorld(t)
	opaque := []byte{0xff, 0x00, '{', '\n', 0x7f, 0x80}
	put := w.call(t, w.putRequest("opaque", string(opaque), map[string]any{"absent": true}))
	if put["status"] != "COMMITTED" || put["revision"] == nil {
		t.Fatal(put)
	}
	got := w.get(t, false)
	if got["payload"] != values.EncodePublicBytes(opaque) {
		t.Fatalf("opaque bytes changed: %v", got["payload"])
	}
	oversize := w.putRequest("oversize", string(make([]byte, 1048577)), map[string]any{"revision": put["revision"]})
	if _, err := w.request(t, oversize); !errors.Is(err, values.PublicResourceExhausted) {
		t.Fatalf("oversize payload: %v", err)
	}
	after := w.call(t, w.putRequest("after", "still-valid", map[string]any{"revision": put["revision"]}))
	if after["status"] != "COMMITTED" {
		t.Fatal(after)
	}
	final := w.get(t, false)
	if final["payload"] != values.EncodePublicBytes([]byte("still-valid")) {
		t.Fatal(final)
	}
}

func TestP5PublicFacetControlDisconnectAndDetach(t *testing.T) {
	w := newPublicFacetWorld(t)
	prep := w.get(t, true)
	w.coreLinks.UnbindEndpoint("local:core0")
	if _, err := w.request(t, map[string]any{"op": "begin-observe", "preparation": prep["preparation"]}); !errors.Is(err, values.PublicUnavailable) {
		t.Fatal(err)
	}
	if err := w.coreLinks.BindEndpoint("local:core0", composition.LocalFacetCoreEndpoint{Read: w.cores[0].Facets(), Control: w.cores[0].Facets(), Access: w.cores[0].Access()}); err != nil {
		t.Fatal(err)
	}
	b := w.call(t, map[string]any{"op": "begin-observe", "preparation": prep["preparation"]})
	r, err := w.cores[0].Elements().ReadElement(w.actor, publicRoot, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = w.cores[0].Facets().DetachFacet(w.actor, publicRoot, r.Attachment.Revision, "detach-public"); err != nil {
		t.Fatal(err)
	}
	_, err = w.request(t, map[string]any{"op": "poll", "cursor": b["cursor"]})
	if !errors.Is(err, values.PublicUnavailable) && !errors.Is(err, values.PublicGap) {
		t.Fatal(err)
	}
	if _, err = w.request(t, map[string]any{"op": "poll", "cursor": b["cursor"]}); !errors.Is(err, values.PublicGap) {
		t.Fatal(err)
	}
}

func TestP5PublicFacetServiceAndClientAuthorizationABA(t *testing.T) {
	for _, service := range []bool{false, true} {
		t.Run(map[bool]string{false: "client", true: "service"}[service], func(t *testing.T) {
			w := newPublicFacetWorld(t)
			prep := w.get(t, true)
			subject, permission, scope := w.actor, facetcontract.PermissionObserveKey, values.FacetKeyScope(w.facets[0].AuthorityRef(), publicRoot, w.typ)
			if service {
				subject, permission, scope = w.service, corecontract.PermissionObserveAttachment, values.CoreElementScope(w.cores[0].AuthorityRef(), publicRoot)
			}
			if _, err := w.policy.DeleteBinding(w.actor, binding(subject, permission, scope), w.policy.BindingSetRevision(), realm.OperationID("revoke-observe")); err != nil {
				t.Fatal(err)
			}
			w.grant(t, subject, permission, scope)
			if _, err := w.request(t, map[string]any{"op": "begin-observe", "preparation": prep["preparation"]}); !errors.Is(err, values.PublicGap) {
				t.Fatal(err)
			}
		})
	}
}
