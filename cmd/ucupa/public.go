package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"time"

	"ucupa/composition"
	"ucupa/shared/values"
	corecontract "ucupa/ucupa-core/contract"
	facetcontract "ucupa/ucupa-facet/contract"
	facet "ucupa/ucupa-facet/instance"
	realm "ucupa/ucupa-realm/contract"
	realminstance "ucupa/ucupa-realm/instance"
)

// A real local public-envelope consumer. Bootstrap is an operator action;
// consumer requests never carry Principal fields or owner-private receipts.
func runPublic(output io.Writer) error {
	policy, err := realminstance.New(realm.Seed{ID: operator.Realm, LocalPrincipals: []values.PrincipalID{operator.Principal, facetService.Principal}, Bindings: []realm.PermissionBinding{binding(operator, realm.PermissionPutBinding, values.RealmScope(operator.Realm))}})
	if err != nil {
		return err
	}
	p := &values.PublicAccessPolicy{ContextTTL: time.Minute, ObservationTTL: time.Minute}
	c, err := composition.NewCluster(composition.Seed{ID: "public-example", Realm: policy, RealmNextHop: "local:realm", FacetServiceActor: facetService, CoreServiceActor: facetService, CoreNextHop: "local:core", FacetNextHop: "local:facet", CoreConfig: corecontract.Config{PublicAccessPolicy: p}, FacetConfig: facet.Config{PublicAccessPolicy: p}})
	if err != nil {
		return err
	}
	defer c.Core().Close()
	defer c.Facets().Close()
	cr, fr := c.Core().AuthorityRef(), c.Facets().AuthorityRef()
	grants := []realm.PermissionBinding{binding(operator, corecontract.PermissionCreateRoot, values.CoreElementScope(cr, worldID))}
	for _, permission := range []values.PermissionID{corecontract.PermissionCreateEntity, corecontract.PermissionReadElement, corecontract.PermissionQueryRegion, corecontract.PermissionObserveElement} {
		grants = append(grants, binding(operator, permission, values.AnchorClosureScope(cr, worldID)))
	}
	for _, permission := range []values.PermissionID{corecontract.PermissionReadElement, corecontract.PermissionFacetPropose, corecontract.PermissionFacetActivate} {
		grants = append(grants, binding(operator, permission, values.CoreElementScope(cr, localEntityID)))
	}
	for _, permission := range []values.PermissionID{corecontract.PermissionFacetProposalRead, corecontract.PermissionFacetAttachmentRead, corecontract.PermissionObserveAttachment} {
		grants = append(grants, binding(facetService, permission, values.CoreElementScope(cr, localEntityID)))
	}
	for _, permission := range []values.PermissionID{facetcontract.PermissionPut, facetcontract.PermissionGet, facetcontract.PermissionObserveKey} {
		grants = append(grants, binding(operator, permission, values.FacetKeyScope(fr, localEntityID, demoType())))
	}
	for _, permission := range []values.PermissionID{facetcontract.PermissionAccept, facetcontract.PermissionListTypes} {
		grants = append(grants, binding(operator, permission, values.FacetElementScope(fr, localEntityID)))
	}
	grants = append(grants, binding(facetService, facetcontract.PermissionReadAcceptance, values.FacetElementScope(fr, localEntityID)))
	for i, g := range grants {
		if _, err = policy.PutBinding(operator, g, policy.BindingSetRevision(), realm.OperationID(fmt.Sprintf("public-grant-%d", i))); err != nil {
			return err
		}
	}
	if _, err = c.Core().Elements().CreateRoot(operator, worldID, "public-root"); err != nil {
		return err
	}
	caller, err := policy.TrustLocalCaller(operator)
	if err != nil {
		return err
	}
	links := &composition.LocalCoreConnections{}
	if err = links.BindEndpoint("local:core", composition.CoreEndpoint{Read: c.Core().Elements(), Public: c.Core().Access()}); err != nil {
		return err
	}
	coreCall := func(r map[string]any) (map[string]any, error) {
		return demoPublicCall(r, func(raw []byte) ([]byte, error) {
			return links.PublicCall(context.Background(), "local:core", cr, caller, raw)
		})
	}
	entered, err := coreCall(map[string]any{"op": "enter", "intent": "create-entity", "reference": map[string]any{"boundary": map[string]any{"core": string(cr), "anchor": string(worldID)}, "nextHop": "local:core"}})
	if err != nil {
		return err
	}
	h := entered["handle"]
	if _, err = coreCall(map[string]any{"op": "create-entity", "handle": h, "element": string(localEntityID), "operationId": "Y3JlYXRl", "final": map[string]any{"parent": string(worldID), "pose": map[string]any{"translation": []any{0, 0, 0}, "rotation": map[string]any{"pivot": 4, "ratios": []any{0, 0, 0}}}, "range": map[string]any{"kind": "bounded-sphere", "radius": 1}}}); err != nil {
		return err
	}
	if _, err = attachDemo(c); err != nil {
		return err
	}
	address := map[string]any{"down": []any{}}
	found, err := coreCall(map[string]any{"op": "query", "handle": h, "start": address, "basis": address, "upDepth": "-1", "selector": "entities", "sphere": map[string]any{"center": []any{0, 0, 0}, "radius": 2}})
	if err != nil {
		return err
	}
	matches := found["matches"].([]any)
	if len(matches) != 1 {
		return fmt.Errorf("public discovery did not close")
	}
	read, err := coreCall(map[string]any{"op": "read", "handle": h, "address": matches[0].(map[string]any)["address"], "basis": found["receipt"]})
	if err != nil {
		return err
	}
	attachment := read["attachment"].(map[string]any)
	access := map[string]any{"key": attachment["key"], "nextHop": attachment["nextHop"]}
	facetCall := func(r map[string]any) (map[string]any, error) {
		return demoPublicCall(r, func(raw []byte) ([]byte, error) {
			return c.FacetConnections().PublicCall(context.Background(), values.NextHop(attachment["nextHop"].(string)), fr, caller, raw)
		})
	}
	typ := values.EncodePublicBytes(demoType().Bytes())
	before, err := facetCall(map[string]any{"op": "get", "access": access, "typeRef": typ, "prepareObserve": true})
	if err != nil {
		return err
	}
	if _, err = facetCall(map[string]any{"op": "put", "access": access, "typeRef": typ, "payload": values.EncodePublicBytes([]byte(`"public-content"`)), "expected": map[string]any{"absent": true}, "operationId": "cHV0"}); err != nil {
		return err
	}
	begun, err := facetCall(map[string]any{"op": "begin-observe", "preparation": before["preparation"]})
	if err != nil {
		return err
	}
	batch, err := facetCall(map[string]any{"op": "poll", "cursor": begun["cursor"]})
	if err != nil {
		return err
	}
	if len(batch["groups"].([]any)) != 1 {
		return fmt.Errorf("public observation missed commit")
	}
	listed, err := facetCall(map[string]any{"op": "list-types", "access": access})
	if err != nil {
		return err
	}
	if _, err = facetCall(map[string]any{"op": "get", "access": access, "typeRef": listed["types"].([]any)[0]}); err != nil {
		return err
	}
	if _, err = facetCall(map[string]any{"op": "release-observe", "cursor": batch["nextCursor"]}); err != nil {
		return err
	}
	if _, err = coreCall(map[string]any{"op": "close", "handle": h}); err != nil {
		return err
	}
	_, err = fmt.Fprintln(output, "public: canonical entry+create+discovery+read; direct Facet and gapless prepared observation")
	return err
}

func demoPublicCall(request map[string]any, call func([]byte) ([]byte, error)) (map[string]any, error) {
	raw, err := json.Marshal(request)
	if err != nil {
		return nil, err
	}
	value, err := values.ParsePublicJSON(raw, values.PublicJSONLimits())
	if err != nil {
		return nil, err
	}
	raw, err = values.CanonicalJSONValue(value)
	if err != nil {
		return nil, err
	}
	raw, err = call(raw)
	if err != nil {
		return nil, err
	}
	value, err = values.ParseCanonicalJSON(raw, values.PublicJSONLimits())
	if err != nil {
		return nil, err
	}
	return value.(map[string]any), nil
}
