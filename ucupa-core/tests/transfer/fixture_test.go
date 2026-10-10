package transfer_test

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"ucupa/composition"
	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	core "ucupa/ucupa-core/instance"
	realm "ucupa/ucupa-realm/contract"
	realminstance "ucupa/ucupa-realm/instance"
)

var actor = values.PrincipalRef{Realm: "transfer-realm", Principal: "caller"}

const root values.ElementID = "00000000-0000-4000-8000-000000000001"
const entity values.ElementID = "00000000-0000-4000-8000-000000000002"

// A wrapper around the real Realm schedules a precise change between capturing
// a view and acquiring the Core commit lock. It does not mint an allowed view.
type authorizationBoundary struct {
	*realminstance.Realm
	mu   sync.Mutex
	next func()
}

func (a *authorizationBoundary) AuthorizeView(_ context.Context, c values.CallerContext, r ...values.Requirement) (values.AuthorizationDecision, values.AuthorizationView) {
	d, v := a.Realm.AuthorizeView(c, r...)
	a.mu.Lock()
	f := a.next
	a.next = nil
	a.mu.Unlock()
	if f != nil {
		f()
	}
	return d, v
}

func (a *authorizationBoundary) afterAuthorize(f func()) {
	a.mu.Lock()
	a.next = f
	a.mu.Unlock()
}

// transferContact is the fixed Realm contact address of the fixture's source
// Core. It matches the transfer command's SourceAddress.
var transferContact = contract.RealmInitialization{Realm: actor.Realm, NextHop: "local:source"}

// boundaryContacts exposes the fixture's single Realm through the Realm
// contact port, keeping the boundary's view interception on the AuthorizeView
// path. Address identity is not a variable in these tests.
type boundaryContacts struct {
	boundary *authorizationBoundary
}

func (boundaryContacts) Check(context.Context, values.NextHop, values.RealmID) error { return nil }

func (contacts boundaryContacts) AuthorizePrincipal(_ values.NextHop, _ values.RealmID, actor values.PrincipalRef, requirements ...values.Requirement) values.AuthorizationDecision {
	return contacts.boundary.Realm.AuthorizePrincipal(actor, requirements...)
}

func (contacts boundaryContacts) AuthorizeView(ctx context.Context, _ values.NextHop, _ values.RealmID, caller values.CallerContext, requirements ...values.Requirement) (values.AuthorizationDecision, values.AuthorizationView) {
	return contacts.boundary.AuthorizeView(ctx, caller, requirements...)
}

func (contacts boundaryContacts) ValidateView(_ context.Context, _ values.NextHop, _ values.RealmID, caller values.CallerContext, view values.AuthorizationView, requirements ...values.Requirement) values.AuthorizationDecision {
	return contacts.boundary.Realm.ValidateView(caller, view, requirements...)
}

func (contacts boundaryContacts) CaptureDenialView(_ context.Context, _ values.NextHop, _ values.RealmID, caller values.CallerContext, requirement values.Requirement) (values.AuthorizationDecision, values.AuthorizationDenialView) {
	return contacts.boundary.Realm.CaptureDenialView(caller, requirement)
}

func (contacts boundaryContacts) ValidateDenialView(_ context.Context, _ values.NextHop, _ values.RealmID, caller values.CallerContext, view values.AuthorizationDenialView, requirement values.Requirement) values.AuthorizationDecision {
	return contacts.boundary.Realm.ValidateDenialView(caller, view, requirement)
}

type fixture struct {
	c       *core.Core
	a       *authorizationBoundary
	caller  values.CallerContext
	command contract.TransitionCommand
	config  contract.Config
	path    string
	serial  int
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	r, err := realminstance.New(realm.Seed{ID: actor.Realm, LocalPrincipals: []values.PrincipalID{actor.Principal, "other"}, Bindings: []realm.PermissionBinding{
		{Subject: actor, Permission: realm.PermissionPutBinding, Scope: values.RealmScope(actor.Realm)},
		{Subject: actor, Permission: realm.PermissionDeleteBinding, Scope: values.RealmScope(actor.Realm)},
	}, Credentials: []realm.CredentialBootstrap{{Subject: actor.Principal, Evidence: realm.CredentialEvidence{Audience: actor.Realm, ID: realm.CredentialID{62}, Secret: realm.CredentialSecret{63}}}}})
	if err != nil {
		t.Fatal(err)
	}
	f := &fixture{a: &authorizationBoundary{Realm: r}, path: filepath.Join(t.TempDir(), "source.db"),
		config: contract.Config{PublicAccessPolicy: &values.PublicAccessPolicy{ContextTTL: time.Minute, ObservationTTL: time.Minute}}}
	f.c, err = core.CreateDurable(boundaryContacts{f.a}, transferContact, f.path, f.config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.c.Close() })
	f.caller, err = r.TrustLocalCaller(actor)
	if err != nil {
		t.Fatal(err)
	}
	f.grant(t, actor, contract.PermissionCreateRoot, values.CoreElementScope(f.c.AuthorityRef(), root), false)
	f.grant(t, actor, contract.PermissionCreateEntity, values.CoreElementScope(f.c.AuthorityRef(), root), false)
	f.grant(t, actor, contract.PermissionPlaceElement, values.CoreElementScope(f.c.AuthorityRef(), root), false)
	if _, err := f.c.Elements().CreateRoot(actor, root, "root"); err != nil {
		t.Fatal(err)
	}
	if _, err := f.c.Elements().CreateEntity(actor, entity, tuple(t, root), "entity"); err != nil {
		t.Fatal(err)
	}
	f.grant(t, actor, contract.PermissionReadElement, values.AnchorClosureScope(f.c.AuthorityRef(), root), false)
	target, err := values.NewFactAuthorityRef()
	if err != nil {
		t.Fatal(err)
	}
	f.command = contract.TransitionCommand{Kind: contract.TransitionSlice, Target: target, Boundary: entity,
		Expected: f.revision(t, entity), Final: tuple(t, root), SourceAddress: "local:source", TargetAddress: "local:target"}
	return f
}

func tuple(t *testing.T, parent values.ElementID) contract.SpatialTuple {
	t.Helper()
	r, err := contract.NewRotation(1, 0, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	p, err := contract.NewPose(contract.Vec3{}, r)
	if err != nil {
		t.Fatal(err)
	}
	s, err := contract.NewBoundedSphere(1)
	if err != nil {
		t.Fatal(err)
	}
	return contract.SpatialTuple{Parent: parent, LocalPose: p, Range: s}
}

func (f *fixture) grant(t *testing.T, subject values.PrincipalRef, permission values.PermissionID, scope values.OperationScope, remove bool) {
	t.Helper()
	f.serial++
	b := realm.PermissionBinding{Subject: subject, Permission: permission, Scope: scope}
	var err error
	if remove {
		_, err = f.a.DeleteBinding(actor, b, f.a.BindingSetRevision(), realm.OperationID(fmt.Sprint(f.serial)))
	} else {
		_, err = f.a.PutBinding(actor, b, f.a.BindingSetRevision(), realm.OperationID(fmt.Sprint(f.serial)))
	}
	if err != nil {
		t.Fatal(err)
	}
}

func (f *fixture) permit(t *testing.T, permission values.PermissionID) {
	f.grant(t, actor, permission, values.CoreElementScope(f.c.AuthorityRef(), f.command.Boundary), false)
}

func (f *fixture) public(t *testing.T, v map[string]any) map[string]any {
	t.Helper()
	raw, err := values.CanonicalJSONValue(v)
	if err != nil {
		t.Fatal(err)
	}
	raw, err = f.c.Access().PublicCall(context.Background(), f.caller, raw)
	if err != nil {
		t.Fatal(err)
	}
	var result map[string]any
	if err := json.Unmarshal(raw, &result); err != nil {
		t.Fatal(err)
	}
	return result
}

func (f *fixture) revision(t *testing.T, element values.ElementID) values.PublicToken {
	t.Helper()
	entered := f.public(t, map[string]any{"op": "enter", "reference": map[string]any{"boundary": map[string]any{
		"core": string(f.c.AuthorityRef()), "anchor": string(root)}, "nextHop": "local:source"}, "intent": "read"})
	down := []any{}
	if element != root {
		down = append(down, string(element))
	}
	r := f.public(t, map[string]any{"op": "read", "handle": entered["handle"], "address": map[string]any{"down": down}})
	v := r["record"].(map[string]any)["revision"].(map[string]any)
	return values.PublicToken{Owner: values.FactAuthorityRef(v["owner"].(string)), Kind: v["kind"].(string), Value: v["value"].(string)}
}

func (f *fixture) reopen(t *testing.T) {
	t.Helper()
	if err := f.c.Close(); err != nil {
		t.Fatal(err)
	}
	var err error
	f.c, err = core.OpenDurable(boundaryContacts{f.a}, f.path, f.config)
	if err != nil {
		t.Fatal(err)
	}
}

// realmLinks binds the fixture's Realm at the owner's durable contact address
// for the network authentication adapter.
func (f *fixture) realmLinks(t *testing.T) *composition.LocalRealmConnections {
	t.Helper()
	links := &composition.LocalRealmConnections{}
	if err := links.BindEndpoint(transferContact.NextHop, f.a.Realm); err != nil {
		t.Fatal(err)
	}
	return links
}
