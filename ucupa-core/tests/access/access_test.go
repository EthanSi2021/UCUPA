package access_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
	"time"

	jsonschema "github.com/santhosh-tekuri/jsonschema/v6"

	"ucupa/shared/values"
	corecontract "ucupa/ucupa-core/contract"
	core "ucupa/ucupa-core/instance"
	realm "ucupa/ucupa-realm/contract"
	realminstance "ucupa/ucupa-realm/instance"
)

type publicFixture struct {
	c      *core.Core
	r      *realminstance.Realm
	caller values.CallerContext
	ref    map[string]any
	handle any
	serial int
}

// publicRealmContacts exposes the fixture's single Realm through the Realm
// contact port at its fixed local address. Address identity is not a variable
// in these scenarios.
type publicRealmContacts struct {
	realm *realminstance.Realm
}

func (publicRealmContacts) Check(context.Context, values.NextHop, values.RealmID) error { return nil }

func (contacts publicRealmContacts) AuthorizePrincipal(_ values.NextHop, _ values.RealmID, actor values.PrincipalRef, requirements ...values.Requirement) values.AuthorizationDecision {
	return contacts.realm.AuthorizePrincipal(actor, requirements...)
}

func (contacts publicRealmContacts) AuthorizeView(_ context.Context, _ values.NextHop, _ values.RealmID, caller values.CallerContext, requirements ...values.Requirement) (values.AuthorizationDecision, values.AuthorizationView) {
	return contacts.realm.AuthorizeView(caller, requirements...)
}

func (contacts publicRealmContacts) ValidateView(_ context.Context, _ values.NextHop, _ values.RealmID, caller values.CallerContext, view values.AuthorizationView, requirements ...values.Requirement) values.AuthorizationDecision {
	return contacts.realm.ValidateView(caller, view, requirements...)
}

func (contacts publicRealmContacts) CaptureDenialView(_ context.Context, _ values.NextHop, _ values.RealmID, caller values.CallerContext, requirement values.Requirement) (values.AuthorizationDecision, values.AuthorizationDenialView) {
	return contacts.realm.CaptureDenialView(caller, requirement)
}

func (contacts publicRealmContacts) ValidateDenialView(_ context.Context, _ values.NextHop, _ values.RealmID, caller values.CallerContext, view values.AuthorizationDenialView, requirement values.Requirement) values.AuthorizationDecision {
	return contacts.realm.ValidateDenialView(caller, view, requirement)
}

var coreWireSchemas sync.Map

func assertCoreWireSchema(t *testing.T, name string, value any) {
	t.Helper()
	var schema *jsonschema.Schema
	if cached, ok := coreWireSchemas.Load(name); ok {
		schema = cached.(*jsonschema.Schema)
	} else {
		path, err := filepath.Abs(filepath.Join("..", "..", "api", name+".schema.json"))
		if err != nil {
			t.Fatal(err)
		}
		schema, err = jsonschema.NewCompiler().Compile(path)
		if err != nil {
			t.Fatal(err)
		}
		coreWireSchemas.Store(name, schema)
	}
	if err := schema.Validate(value); err != nil {
		t.Fatalf("actual %s failed wire schema: %v", name, err)
	}
}

func newPublicFixture(t *testing.T, retained bool, path string) *publicFixture {
	t.Helper()
	f := &publicFixture{}
	var err error
	f.r, err = realminstance.New(realm.Seed{ID: actor.Realm, LocalPrincipals: []values.PrincipalID{actor.Principal}, Bindings: []realm.PermissionBinding{
		{Subject: actor, Permission: realm.PermissionPutBinding, Scope: values.RealmScope(actor.Realm)},
		{Subject: actor, Permission: realm.PermissionDeleteBinding, Scope: values.RealmScope(actor.Realm)},
	}})
	if err != nil {
		t.Fatal(err)
	}
	policy := &values.PublicAccessPolicy{ContextTTL: time.Minute, ObservationTTL: time.Minute}
	config := corecontract.Config{PublicAccessPolicy: policy}
	if retained {
		duration := time.Minute
		config.SnapshotRetention = &duration
	}
	address := values.NextHop("local:public-realm")
	initial := corecontract.RealmInitialization{Realm: f.r.ID(), NextHop: address}
	if path == "" {
		f.c, err = core.New(publicRealmContacts{f.r}, initial, config)
	} else {
		f.c, err = core.CreateDurable(publicRealmContacts{f.r}, initial, path, config)
	}
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.c.Close() })
	f.grant(t, corecontract.PermissionCreateRoot, values.CoreElementScope(f.c.AuthorityRef(), id(1)), false)
	createRoot(t, f.c, id(1), "public-root")
	f.ref = map[string]any{"boundary": map[string]any{"core": string(f.c.AuthorityRef()), "anchor": string(id(1))}, "nextHop": "local:core"}
	f.caller, err = f.r.TrustLocalCaller(actor)
	if err != nil {
		t.Fatal(err)
	}
	return f
}

func (f *publicFixture) grant(t *testing.T, permission values.PermissionID, scope values.OperationScope, remove bool) {
	t.Helper()
	f.serial++
	binding := realm.PermissionBinding{Subject: actor, Permission: permission, Scope: scope}
	var err error
	if remove {
		_, err = f.r.DeleteBinding(actor, binding, f.r.BindingSetRevision(), realm.OperationID(fmt.Sprint(f.serial)))
	} else {
		_, err = f.r.PutBinding(actor, binding, f.r.BindingSetRevision(), realm.OperationID(fmt.Sprint(f.serial)))
	}
	if err != nil {
		t.Fatal(err)
	}
}
func (f *publicFixture) permit(t *testing.T, permissions ...values.PermissionID) {
	for _, p := range permissions {
		f.grant(t, p, values.AnchorClosureScope(f.c.AuthorityRef(), id(1)), false)
	}
}
func (f *publicFixture) request(t *testing.T, request map[string]any) (map[string]any, error) {
	t.Helper()
	raw, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	value, err := values.ParsePublicJSON(raw, values.PublicJSONLimits())
	if err != nil {
		t.Fatal(err)
	}
	raw, err = values.CanonicalJSONValue(value)
	if err != nil {
		t.Fatal(err)
	}
	out, err := f.c.Access().PublicCall(context.Background(), f.caller, raw)
	if err != nil {
		if out != nil {
			t.Fatal("error disclosed response")
		}
		return nil, err
	}
	decoded, err := values.ParseCanonicalJSON(out, values.PublicJSONLimits())
	if err != nil {
		t.Fatal(err)
	}
	assertCoreWireSchema(t, "request", value)
	assertCoreWireSchema(t, "response", decoded)
	return decoded.(map[string]any), nil
}
func (f *publicFixture) call(t *testing.T, request map[string]any) map[string]any {
	t.Helper()
	response, err := f.request(t, request)
	if err != nil {
		t.Fatalf("%s: %v", request["op"], err)
	}
	return response
}
func (f *publicFixture) enter(t *testing.T, intent string) {
	f.handle = f.call(t, map[string]any{"op": "enter", "reference": f.ref, "intent": intent})["handle"]
}
func publicAddress(ids ...values.ElementID) map[string]any {
	down := []any{}
	for _, id := range ids {
		down = append(down, string(id))
	}
	return map[string]any{"down": down}
}
func publicFinal(parent values.ElementID, x, radius float64) map[string]any {
	return map[string]any{"parent": string(parent), "pose": map[string]any{"translation": []any{x, 0., 0.}, "rotation": map[string]any{"pivot": 4., "ratios": []any{0., 0., 0.}}}, "range": map[string]any{"kind": "bounded-sphere", "radius": radius}}
}
func (f *publicFixture) create(t *testing.T, n int) map[string]any {
	return f.call(t, map[string]any{"op": "create-entity", "handle": f.handle, "element": string(id(uint64(n))), "final": publicFinal(id(1), 0, 1), "operationId": values.EncodePublicBytes([]byte(fmt.Sprint("entity-", n)))})
}

func TestPublicWriteOnlyEntryAndClosureDoesNotImplyRead(t *testing.T) {
	f := newPublicFixture(t, false, "")
	f.permit(t, corecontract.PermissionCreateEntity)
	f.enter(t, "create-entity")
	created := f.create(t, 2)
	if created["status"] != "COMMITTED" {
		t.Fatal(created)
	}
	_, err := f.request(t, map[string]any{"op": "read", "handle": f.handle, "address": publicAddress(id(2))})
	if !errors.Is(err, values.PublicDenied) {
		t.Fatal(err)
	}
	f.permit(t, corecontract.PermissionQueryRegion, corecontract.PermissionReadElement)
	query := f.call(t, map[string]any{"op": "query", "handle": f.handle, "start": publicAddress(), "basis": publicAddress(), "upDepth": "-1", "sphere": map[string]any{"center": []any{0., 0., 0.}, "radius": 2.}, "selector": "entities"})
	if query["status"] != "COMPLETE" || len(query["matches"].([]any)) != 1 {
		t.Fatal(query)
	}
	match := query["matches"].([]any)[0].(map[string]any)
	if _, exists := match["attachment"]; exists {
		t.Fatal("discovery leaked attachment")
	}
	read := f.call(t, map[string]any{"op": "read", "handle": f.handle, "address": match["address"], "basis": query["receipt"]})
	if read["state"] != "PRESENT" || read["attachment"].(map[string]any)["state"] != "NONE" {
		t.Fatal(read)
	}
}

func TestPublicPreparedReadCoversBeforeBeginAndNoopsDoNotNotify(t *testing.T) {
	f := newPublicFixture(t, false, "")
	f.permit(t, corecontract.PermissionQueryRegion, corecontract.PermissionObserveElement, corecontract.PermissionCreateEntity)
	f.enter(t, "query")
	query := map[string]any{"op": "query", "handle": f.handle, "start": publicAddress(), "basis": publicAddress(), "sphere": map[string]any{"center": []any{0., 0., 0.}, "radius": 1.}, "selector": "entities", "prepareObserve": true}
	read := f.call(t, query)
	if len(read["matches"].([]any)) != 0 {
		t.Fatal(read)
	}
	f.create(t, 2)
	f.create(t, 2) // exact replay, one committed group only
	begun := f.call(t, map[string]any{"op": "begin-observe", "preparation": read["preparation"]})
	poll := map[string]any{"op": "poll", "cursor": begun["cursor"]}
	first := f.call(t, poll)
	repeat := f.call(t, poll)
	if len(first["groups"].([]any)) != 1 || !reflect.DeepEqual(first, repeat) {
		t.Fatal(first, repeat)
	}
	next := f.call(t, map[string]any{"op": "poll", "cursor": first["nextCursor"]})
	if len(next["groups"].([]any)) != 0 || !reflect.DeepEqual(next["nextCursor"], first["nextCursor"]) {
		t.Fatal(next)
	}
	_, err := f.request(t, poll)
	if !errors.Is(err, values.PublicGap) {
		t.Fatal("reclaimed cursor", err)
	}
}

func TestPublicRevokeRegrantCannotActivateOldPreparation(t *testing.T) {
	f := newPublicFixture(t, false, "")
	f.permit(t, corecontract.PermissionListDirect, corecontract.PermissionObserveElement)
	f.enter(t, "list")
	read := f.call(t, map[string]any{"op": "list", "handle": f.handle, "address": publicAddress(), "prepareObserve": true})
	scope := values.AnchorClosureScope(f.c.AuthorityRef(), id(1))
	f.grant(t, corecontract.PermissionObserveElement, scope, true)
	f.grant(t, corecontract.PermissionObserveElement, scope, false)
	_, err := f.request(t, map[string]any{"op": "begin-observe", "preparation": read["preparation"]})
	if !errors.Is(err, values.PublicGap) {
		t.Fatal(err)
	}
}

func TestPublicPaginationRetainsOnlyWhenConfigured(t *testing.T) {
	for _, retained := range []bool{false, true} {
		t.Run(fmt.Sprint(retained), func(t *testing.T) {
			f := newPublicFixture(t, retained, "")
			f.permit(t, corecontract.PermissionListDirect, corecontract.PermissionReadElement, corecontract.PermissionCreateEntity)
			f.enter(t, "list")
			f.create(t, 2)
			f.create(t, 3)
			request := map[string]any{"op": "list", "handle": f.handle, "address": publicAddress(), "limit": 1}
			first := f.call(t, request)
			f.create(t, 4)
			request["continuation"] = first["continuation"]
			second, err := f.request(t, request)
			if !retained {
				if !errors.Is(err, values.PublicInvalidContext) {
					t.Fatal(second, err)
				}
				return
			}
			if err != nil || second["status"] != "COMPLETE" || second["readMode"] != "RETAINED" {
				t.Fatal(second, err)
			}
			if len(second["records"].([]any)) != 1 {
				t.Fatal(second)
			}
		})
	}
}

func TestPublicRecoveryAfterBoundaryDeletionAndRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "core.db")
	f := newPublicFixture(t, false, path)
	f.permit(t, corecontract.PermissionCreateAnchor, corecontract.PermissionDeleteAnchor)
	f.enter(t, "create-anchor")
	created := f.call(t, map[string]any{"op": "create-anchor", "handle": f.handle, "element": string(id(2)), "final": publicFinal(id(1), 0, 10), "operationId": values.EncodePublicBytes([]byte("anchor"))})
	f.grant(t, corecontract.PermissionDeleteAnchor, values.AnchorClosureScope(f.c.AuthorityRef(), id(2)), false)
	childRef := map[string]any{"boundary": map[string]any{"core": string(f.c.AuthorityRef()), "anchor": string(id(2))}, "nextHop": "local:core"}
	handle := f.call(t, map[string]any{"op": "enter", "reference": childRef, "intent": "delete-anchor"})["handle"]
	original := map[string]any{"op": "delete-anchor", "element": string(id(2)), "operationId": values.EncodePublicBytes([]byte("delete")), "expected": created["record"].(map[string]any)["revision"]}
	request := map[string]any{}
	for k, v := range original {
		request[k] = v
	}
	request["handle"] = handle
	deleted := f.call(t, request)
	if err := f.c.Close(); err != nil {
		t.Fatal(err)
	}
	var err error
	f.c, err = core.OpenDurable(publicRealmContacts{f.r}, path, corecontract.Config{PublicAccessPolicy: &values.PublicAccessPolicy{ContextTTL: time.Minute, ObservationTTL: time.Minute}})
	if err != nil {
		t.Fatal(err)
	}
	got := f.call(t, map[string]any{"op": "recover-outcome", "reference": childRef, "operationId": original["operationId"], "request": original})
	if !reflect.DeepEqual(got, deleted) {
		t.Fatal("recovery changed durable public revision", got, deleted)
	}
}
