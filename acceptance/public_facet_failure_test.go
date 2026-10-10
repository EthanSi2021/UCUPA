package acceptance_test

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"ucupa/composition"
	"ucupa/shared/values"
	corecontract "ucupa/ucupa-core/contract"
	facetcontract "ucupa/ucupa-facet/contract"
	facet "ucupa/ucupa-facet/instance"
	realm "ucupa/ucupa-realm/contract"
)

type interceptControl struct {
	composition.FacetCoreControlPort
	after func()
}

func (p *interceptControl) ReadAttachmentControl(ctx context.Context, c values.CallerContext, k values.FacetAcceptanceKey, prepare bool) (corecontract.AttachmentControlRead, error) {
	r, e := p.FacetCoreControlPort.ReadAttachmentControl(ctx, c, k, prepare)
	if p.after != nil {
		hook := p.after
		p.after = nil
		hook()
	}
	return r, e
}

func TestP5PreparedFacetDoesNotDiscloseHalfPreparedRead(t *testing.T) {
	for _, kind := range []string{"facet-write", "service-view"} {
		t.Run(kind, func(t *testing.T) {
			w := newPublicFacetWorld(t)
			peer := &interceptControl{FacetCoreControlPort: w.cores[0].Facets()}
			peer.after = func() {
				if kind == "facet-write" {
					if _, err := w.facets[0].Namespace.Put(context.Background(), w.actor, w.key, w.typ, []byte("1"), facetcontract.ExpectAbsent(), "race"); err != nil {
						t.Fatal(err)
					}
				} else {
					scope := values.CoreElementScope(w.cores[0].AuthorityRef(), publicRoot)
					if _, err := w.policy.DeleteBinding(w.actor, binding(w.service, corecontract.PermissionObserveAttachment, scope), w.policy.BindingSetRevision(), realm.OperationID("service-race")); err != nil {
						t.Fatal(err)
					}
					w.grant(t, w.service, corecontract.PermissionObserveAttachment, scope)
				}
			}
			w.coreLinks.UnbindEndpoint("local:core0")
			if err := w.coreLinks.BindEndpoint("local:core0", composition.LocalFacetCoreEndpoint{Read: w.cores[0].Facets(), Control: peer, Access: w.cores[0].Access()}); err != nil {
				t.Fatal(err)
			}
			_, err := w.request(t, map[string]any{"op": "get", "access": w.access, "typeRef": values.EncodePublicBytes(w.typ.Bytes()), "prepareObserve": true})
			want := values.PublicInvalidContext
			if kind == "service-view" {
				want = values.PublicGap
			}
			if !errors.Is(err, want) {
				t.Fatal(err)
			}
			_ = w.get(t, true) // new read and both preparations remain possible after cleanup.
		})
	}
}

func TestP5FacetPublicRevisionAndOutcomeSurviveRestartAndDetach(t *testing.T) {
	w := newPublicFacetWorld(t)
	put := w.putRequest("durable-first", "\"a\"", map[string]any{"absent": true})
	created := w.call(t, put)
	oldPrep := w.get(t, true)["preparation"]
	owner := w.facets[0].AuthorityRef()
	if err := w.facets[0].Close(); err != nil {
		t.Fatal(err)
	}
	service, err := w.policy.TrustLocalCaller(w.service)
	if err != nil {
		t.Fatal(err)
	}
	source, err := w.coreLinks.ForFacetCaller(service)
	if err != nil {
		t.Fatal(err)
	}
	w.facets[0], err = facet.OpenDurable(w.realmLinks, source, w.paths[2], facet.Config{PublicAccessPolicy: publicPolicy()})
	if err != nil {
		t.Fatal(err)
	}
	w.facetLinks.UnbindEndpoint("local:facet0")
	if err = w.facetLinks.BindEndpoint("local:facet0", w.facets[0]); err != nil {
		t.Fatal(err)
	}
	if w.facets[0].AuthorityRef() != owner {
		t.Fatal("owner changed")
	}
	if _, err = w.request(t, map[string]any{"op": "begin-observe", "preparation": oldPrep}); !errors.Is(err, values.PublicGap) {
		t.Fatal(err)
	}
	got := w.get(t, false)
	if !reflect.DeepEqual(got["revision"], created["revision"]) {
		t.Fatal("revision changed")
	}
	next := w.call(t, w.putRequest("durable-next", "\"b\"", map[string]any{"revision": created["revision"]}))
	if next["status"] != "COMMITTED" {
		t.Fatal(next)
	}
	del := map[string]any{"op": "delete", "access": w.access, "typeRef": values.EncodePublicBytes(w.typ.Bytes()), "expected": next["revision"], "operationId": values.EncodePublicBytes([]byte("durable-delete"))}
	deleted := w.call(t, del)
	r, err := w.cores[0].Elements().ReadElement(w.actor, publicRoot, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = w.cores[0].Facets().DetachFacet(w.actor, publicRoot, r.Attachment.Revision, "finish-detach"); err != nil {
		t.Fatal(err)
	}
	w.coreLinks.UnbindEndpoint("local:core0")
	recovered := w.call(t, map[string]any{"op": "recover-outcome", "access": w.access, "operationId": del["operationId"], "request": del})
	if !reflect.DeepEqual(recovered, deleted) {
		t.Fatal("pure recovery changed outcome")
	}
	if replay := w.call(t, put); !reflect.DeepEqual(replay, created) {
		t.Fatal("old expected replay changed")
	}
	changed := map[string]any{}
	for k, v := range del {
		changed[k] = v
	}
	changed["expected"] = created["revision"]
	if _, err = w.request(t, changed); !errors.Is(err, values.PublicConflict) {
		t.Fatal(err)
	}
}
