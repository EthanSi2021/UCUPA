package commit_test

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"testing"

	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	"ucupa/ucupa-core/internal/access"
	"ucupa/ucupa-core/internal/access/entry"
	"ucupa/ucupa-core/internal/access/observation"
	"ucupa/ucupa-core/internal/access/registry"
	"ucupa/ucupa-core/internal/commit"
	"ucupa/ucupa-core/internal/element"
	"ucupa/ucupa-core/internal/query"
	"ucupa/ucupa-core/internal/query/federation/coordinator"
	"ucupa/ucupa-core/internal/query/federation/source"
	"ucupa/ucupa-core/internal/query/local"
)

var publicCommitActor = values.PrincipalRef{Realm: "oracle-realm", Principal: "oracle"}

// This fixed trusted policy is the original public durability fixture. Realm
// credential, revocation and ABA integration remains in the public access tests.
type publicCommitAuth struct{}

func (publicCommitAuth) AuthorizePrincipal(values.PrincipalRef, ...values.Requirement) values.AuthorizationDecision {
	return values.AuthorizationValid
}
func (publicCommitAuth) AuthorizeView(context.Context, values.CallerContext, ...values.Requirement) (values.AuthorizationDecision, values.AuthorizationView) {
	return values.AuthorizationValid, values.AuthorizationView{1}
}
func (publicCommitAuth) ValidateView(context.Context, values.CallerContext, values.AuthorizationView, ...values.Requirement) values.AuthorizationDecision {
	return values.AuthorizationValid
}
func (publicCommitAuth) CaptureDenialView(context.Context, values.CallerContext, values.Requirement) (values.AuthorizationDecision, values.AuthorizationDenialView) {
	return values.AuthorizationValid, values.AuthorizationDenialView{}
}
func (publicCommitAuth) ValidateDenialView(_ context.Context, _ values.CallerContext, view values.AuthorizationDenialView, _ values.Requirement) values.AuthorizationDecision {
	if view == (values.AuthorizationDenialView{}) {
		return values.AuthorizationDenied
	}
	return values.AuthorizationChanged
}

type publicCommitFixture struct {
	owner     *commit.Owner
	resources *registry.Registry
	queries   *query.Operations
	entry     *entry.Operations
	elements  *element.Operations
}

// Only this fixture sees the actual commit owner needed by the existing
// test-only Store fault seam. Requests still run through the real public
// envelope, operations, commit publication and shared observation registry.
func createPublicCommitFixture(path string, config contract.Config, reopen bool) (*publicCommitFixture, error) {
	settings, err := query.NormalizeConfig(config)
	if err != nil {
		return nil, err
	}
	var run [32]byte
	if _, err := rand.Read(run[:]); err != nil {
		return nil, err
	}
	var owner *commit.Owner
	if reopen {
		owner, err = commit.OpenDurable(path, nil)
	} else {
		var scope [32]byte
		if _, err := rand.Read(scope[:]); err != nil {
			return nil, err
		}
		authority, identityErr := values.NewFactAuthorityRef()
		if identityErr != nil {
			return nil, identityErr
		}
		owner, err = commit.CreateDurable(path, authority, scope, contract.RealmInitialization{Realm: "fixture-realm", NextHop: "local:fixture"}, nil)
	}
	if err != nil {
		return nil, err
	}
	auth := publicCommitAuth{}
	admission := access.New(auth, owner, settings.Public)
	queries := query.New(owner, auth, run, settings)
	resources := registry.New(owner.AuthorityRef(), settings.Public, owner, queries)
	observe := observation.New(owner, admission, resources)
	sources := source.New(owner, admission, resources, queries, observe)
	federation := coordinator.New(owner, admission, resources, sources, observe)
	operations := entry.New(owner, admission, resources, local.New(owner, admission, resources, queries),
		federation, element.NewPublic(owner, admission, resources))
	fixture := &publicCommitFixture{owner: owner, resources: resources, queries: queries,
		entry: operations, elements: element.New(owner, auth)}
	owner.Lock()
	err = owner.SetPublicationSinkLocked(resources)
	owner.Unlock()
	if err != nil {
		_ = fixture.close()
		return nil, err
	}
	return fixture, nil
}

func (f *publicCommitFixture) close() error {
	f.owner.Lock()
	if !f.owner.BeginCloseLocked() {
		f.owner.Unlock()
		return nil
	}
	f.resources.CloseLocked()
	f.owner.Unlock()
	f.resources.DrainRetired()
	f.queries.Close()
	return f.owner.CloseStore()
}

func publicCommitID(n uint64) values.ElementID {
	return values.ElementID(fmt.Sprintf("10000000-0000-4000-8000-%012x", n))
}

func publicCommitRoot(t *testing.T, f *publicCommitFixture, id values.ElementID, operation contract.OperationID) values.ElementID {
	t.Helper()
	result, err := f.elements.CreateRoot(publicCommitActor, id, operation)
	if err != nil || result.Record == nil {
		t.Fatalf("root %#v err=%v", result, err)
	}
	return result.Record.Anchor.ID
}

func publicCommitCall(t *testing.T, f *publicCommitFixture, request map[string]any) (map[string]any, error) {
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
	out, err := f.entry.PublicCall(context.Background(), values.CallerContext{Subject: publicCommitActor}, raw)
	if err != nil {
		return nil, err
	}
	value, err = values.ParseCanonicalJSON(out, values.PublicJSONLimits())
	if err != nil {
		t.Fatal(err)
	}
	return value.(map[string]any), nil
}
