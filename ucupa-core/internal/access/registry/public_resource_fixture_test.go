package registry_test

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

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

var resourceActor = values.PrincipalRef{Realm: "oracle-realm", Principal: "oracle"}

// Fixed trusted policy isolates resource behavior, just as in the original
// root fixture. Real Realm credential and view changes are tested separately.
type resourceAuth struct{}

func (resourceAuth) AuthorizePrincipal(values.PrincipalRef, ...values.Requirement) values.AuthorizationDecision {
	return values.AuthorizationValid
}
func (resourceAuth) AuthorizeView(context.Context, values.CallerContext, ...values.Requirement) (values.AuthorizationDecision, values.AuthorizationView) {
	return values.AuthorizationValid, values.AuthorizationView{1}
}
func (resourceAuth) ValidateView(context.Context, values.CallerContext, values.AuthorizationView, ...values.Requirement) values.AuthorizationDecision {
	return values.AuthorizationValid
}
func (resourceAuth) CaptureDenialView(context.Context, values.CallerContext, values.Requirement) (values.AuthorizationDecision, values.AuthorizationDenialView) {
	return values.AuthorizationValid, values.AuthorizationDenialView{}
}
func (resourceAuth) ValidateDenialView(_ context.Context, _ values.CallerContext, view values.AuthorizationDenialView, _ values.Requirement) values.AuthorizationDecision {
	if view == (values.AuthorizationDenialView{}) {
		return values.AuthorizationDenied
	}
	return values.AuthorizationChanged
}

type resourceFixture struct {
	owner     *commit.Owner
	resources *registry.Registry
	admission *access.Admission
	limit     uint64
	entry     *entry.Operations
	elements  *element.Operations
	sources   *source.Operations
}

// Assemble the actual public envelope and its actual consumers around one
// commit owner. The tests mutate only this registry's three fixed test faults;
// no replacement facade, fake read implementation or duplicate fact store.
func newResourceComponents(t *testing.T, config contract.Config) *resourceFixture {
	t.Helper()
	authority, err := values.NewFactAuthorityRef()
	if err != nil {
		t.Fatal(err)
	}
	owner, err := commit.New(authority, [32]byte{91}, contract.RealmInitialization{Realm: "fixture-realm", NextHop: "local:fixture"})
	if err != nil {
		t.Fatal(err)
	}
	settings, err := query.NormalizeConfig(config)
	if err != nil {
		t.Fatal(err)
	}
	auth := resourceAuth{}
	admission := access.New(auth, owner, settings.Public)
	queries := query.New(owner, auth, [32]byte{92}, settings)
	resources := registry.New(authority, settings.Public, owner, queries)
	observe := observation.New(owner, admission, resources)
	sources := source.New(owner, admission, resources, queries, observe)
	federation := coordinator.New(owner, admission, resources, sources, observe)
	operations := entry.New(owner, admission, resources, local.New(owner, admission, resources, queries), federation, element.NewPublic(owner, admission, resources))
	f := &resourceFixture{owner: owner, resources: resources, admission: admission, limit: settings.Public.ConcurrentReads, entry: operations, elements: element.New(owner, auth), sources: sources}
	owner.Lock()
	err = owner.SetPublicationSinkLocked(resources)
	owner.Unlock()
	t.Cleanup(func() {
		owner.Lock()
		owner.BeginCloseLocked()
		resources.CloseLocked()
		owner.Unlock()
		resources.DrainRetired()
		queries.Close()
		_ = owner.CloseStore()
	})
	if err != nil {
		t.Fatal(err)
	}
	return f
}

func newResourceFixture(t *testing.T) (*resourceFixture, any, any) {
	t.Helper()
	f := newResourceComponents(t, contract.Config{PublicAccessPolicy: &values.PublicAccessPolicy{ContextTTL: time.Hour, ObservationTTL: time.Hour}})
	root := resourceID(91500)
	if _, err := f.elements.CreateRoot(resourceActor, root, "root"); err != nil {
		t.Fatal(err)
	}
	h, err := f.call(t, map[string]any{"op": "enter", "intent": "list", "reference": map[string]any{"boundary": map[string]any{"core": string(f.sources.AuthorityRef()), "anchor": string(root)}, "nextHop": "local:core"}})
	if err != nil {
		t.Fatal(err)
	}
	p, err := f.call(t, map[string]any{"op": "list", "handle": h["handle"], "address": map[string]any{"down": []any{}}, "prepareObserve": true})
	if err != nil {
		t.Fatal(err)
	}
	return f, h["handle"], p["preparation"]
}

func resourceID(n uint64) values.ElementID {
	return values.ElementID(fmt.Sprintf("10000000-0000-4000-8000-%012x", n))
}

func (f *resourceFixture) create(t *testing.T, n uint64, operation contract.OperationID) {
	t.Helper()
	rotation, err := contract.NewRotation(1, 0, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	pose, err := contract.NewPose(contract.Vec3{}, rotation)
	if err != nil {
		t.Fatal(err)
	}
	rangeValue, err := contract.NewBoundedSphere(0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.elements.CreateEntity(resourceActor, resourceID(n), contract.SpatialTuple{Parent: resourceID(91500), LocalPose: pose, Range: rangeValue}, operation); err != nil {
		t.Fatal(err)
	}
}

func (f *resourceFixture) call(t *testing.T, request map[string]any) (map[string]any, error) {
	t.Helper()
	raw, err := json.Marshal(request)
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
	out, err := f.entry.PublicCall(context.Background(), values.CallerContext{Subject: resourceActor}, raw)
	if err != nil {
		return nil, err
	}
	v, err = values.ParseCanonicalJSON(out, values.PublicJSONLimits())
	if err != nil {
		t.Fatal(err)
	}
	return v.(map[string]any), nil
}

// Occupy the real shared Admission instead of directly setting its counter.
func (f *resourceFixture) occupySlots(t *testing.T) func() {
	t.Helper()
	count := uint64(0)
	released := false
	release := func() {
		if !released {
			released = true
			for i := uint64(0); i < count; i++ {
				f.admission.Release()
			}
		}
	}
	t.Cleanup(release)
	for ; count < f.limit; count++ {
		if err := f.admission.Acquire(); err != nil {
			t.Fatal(err)
		}
	}
	return release
}
