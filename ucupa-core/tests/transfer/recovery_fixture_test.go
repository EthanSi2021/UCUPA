package transfer_test

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"ucupa/composition"
	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	core "ucupa/ucupa-core/instance"
	local "ucupa/ucupa-core/transport/local/transition"
	realm "ucupa/ucupa-realm/contract"
	realminstance "ucupa/ucupa-realm/instance"
)

const targetRoot values.ElementID = "00000000-0000-4000-8000-000000000003"

// targetContact is the fixed Realm contact address of the target fixture's
// Core. It matches the transfer command's TargetAddress.
var targetContact = contract.RealmInitialization{Realm: "target-realm", NextHop: "local:target"}

type targetFixture struct {
	c      *core.Core
	a      *authorizationBoundary
	caller values.CallerContext
	path   string
	config contract.Config
	serial int
}

func newTargetFixture(t *testing.T, empty bool) *targetFixture {
	t.Helper()
	actor := values.PrincipalRef{Realm: "target-realm", Principal: "importer"}
	r, err := realminstance.New(realm.Seed{ID: actor.Realm, LocalPrincipals: []values.PrincipalID{actor.Principal}, Bindings: []realm.PermissionBinding{
		{Subject: actor, Permission: realm.PermissionPutBinding, Scope: values.RealmScope(actor.Realm)},
		{Subject: actor, Permission: realm.PermissionDeleteBinding, Scope: values.RealmScope(actor.Realm)},
	}, Credentials: []realm.CredentialBootstrap{{Subject: actor.Principal, Evidence: realm.CredentialEvidence{Audience: actor.Realm, ID: realm.CredentialID{72}, Secret: realm.CredentialSecret{73}}}}})
	if err != nil {
		t.Fatal(err)
	}
	f := &targetFixture{a: &authorizationBoundary{Realm: r}, path: filepath.Join(t.TempDir(), "target.db"),
		config: contract.Config{PublicAccessPolicy: &values.PublicAccessPolicy{ContextTTL: time.Minute, ObservationTTL: time.Minute}}}
	f.caller, err = r.TrustLocalCaller(actor)
	if err != nil {
		t.Fatal(err)
	}
	f.c, err = core.CreateDurable(boundaryContacts{f.a}, targetContact, f.path, f.config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.c.Close() })
	if !empty {
		f.grant(t, contract.PermissionCreateRoot, targetRoot, false)
		if _, err := f.c.Elements().CreateRoot(actor, targetRoot, "target-root"); err != nil {
			t.Fatal(err)
		}
	}
	return f
}

func (f *targetFixture) grant(t *testing.T, p values.PermissionID, boundary values.ElementID, remove bool) {
	t.Helper()
	f.serial++
	b := realm.PermissionBinding{Subject: f.caller.Subject, Permission: p, Scope: values.CoreElementScope(f.c.AuthorityRef(), boundary)}
	var err error
	if remove {
		_, err = f.a.DeleteBinding(f.caller.Subject, b, f.a.BindingSetRevision(), realm.OperationID(fmt.Sprint(f.serial)))
	} else {
		_, err = f.a.PutBinding(f.caller.Subject, b, f.a.BindingSetRevision(), realm.OperationID(fmt.Sprint(f.serial)))
	}
	if err != nil {
		t.Fatal(err)
	}
}

func (f *targetFixture) reopen(t *testing.T) {
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
func (f *targetFixture) realmLinks(t *testing.T) *composition.LocalRealmConnections {
	t.Helper()
	links := &composition.LocalRealmConnections{}
	if err := links.BindEndpoint(targetContact.NextHop, f.a.Realm); err != nil {
		t.Fatal(err)
	}
	return links
}

func beginPair(t *testing.T, source *fixture, target *targetFixture, rootTransfer bool) contract.TransitionApprovalBinding {
	t.Helper()
	source.command.Target = target.c.AuthorityRef()
	source.command.Final = tuple(t, targetRoot)
	if rootTransfer {
		source.command.Kind, source.command.Boundary = contract.TransitionRoot, root
		source.command.Final, source.command.Expected = contract.SpatialTuple{}, source.revision(t, root)
	}
	for _, p := range []values.PermissionID{contract.PermissionTransitionExport, contract.PermissionTransitionCommit, contract.PermissionTransitionInspect} {
		source.permit(t, p)
	}
	b := contract.TransitionApprovalBinding{Source: source.c.AuthorityRef(), Operation: contract.OperationID(string([]byte{255, 0, 128, 'r'})), Command: source.command}
	if _, err := source.c.Transfers().BeginTransition(context.Background(), source.caller, b.Operation, b.Command); err != nil {
		t.Fatal(err)
	}
	target.grant(t, contract.PermissionTransitionImport, b.Command.Boundary, false)
	if !rootTransfer {
		target.grant(t, contract.PermissionPlaceElement, targetRoot, false)
	}
	return b
}

func pairConnection(t *testing.T, source *fixture, target *targetFixture, b contract.TransitionApprovalBinding, authorize bool) *local.Connection {
	t.Helper()
	var view values.AuthorizationView
	var err error
	if authorize {
		view, err = target.c.Transfers().AuthorizeTransitionImport(context.Background(), target.caller, b)
		if err != nil {
			t.Fatal(err)
		}
	}
	c, err := local.New(source.c.Transfers(), target.c.Transfers(), b, target.caller, view)
	if err != nil {
		t.Fatal(err)
	}
	return c
}
