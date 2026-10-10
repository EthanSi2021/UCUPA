package coordinator

import (
	"context"
	"crypto/sha256"
	"fmt"
	"testing"
	"time"

	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	federationcontract "ucupa/ucupa-core/contract/federation"
	"ucupa/ucupa-core/internal/access"
	"ucupa/ucupa-core/internal/access/observation"
	"ucupa/ucupa-core/internal/access/registry"
	accessvalue "ucupa/ucupa-core/internal/access/value"
	"ucupa/ucupa-core/internal/commit"
	"ucupa/ucupa-core/internal/element"
	"ucupa/ucupa-core/internal/query"
	"ucupa/ucupa-core/internal/query/federation/source"
)

type coordinatorPolicy struct{ denyRead, denyAll bool }

func coordinatorAuthorizationDenialDigest(subject values.PrincipalRef, requirement values.Requirement) values.AuthorizationDenialView {
	return values.AuthorizationDenialView(sha256.Sum256([]byte(fmt.Sprintf("denial:%v:%v", subject, requirement))))
}

func (*coordinatorPolicy) AuthorizePrincipal(values.PrincipalRef, ...values.Requirement) values.AuthorizationDecision {
	return values.AuthorizationValid
}
func (p *coordinatorPolicy) AuthorizeView(_ context.Context, _ values.CallerContext, requirements ...values.Requirement) (values.AuthorizationDecision, values.AuthorizationView) {
	for _, r := range requirements {
		if p.denyAll || p.denyRead && r.Permission == contract.PermissionReadElement {
			return values.AuthorizationDenied, values.AuthorizationView{}
		}
	}
	return values.AuthorizationValid, values.AuthorizationView{1}
}
func (p *coordinatorPolicy) ValidateView(ctx context.Context, c values.CallerContext, _ values.AuthorizationView, requirements ...values.Requirement) values.AuthorizationDecision {
	d, _ := p.AuthorizeView(ctx, c, requirements...)
	return d
}
func (p *coordinatorPolicy) CaptureDenialView(ctx context.Context, caller values.CallerContext, requirement values.Requirement) (values.AuthorizationDecision, values.AuthorizationDenialView) {
	if d, _ := p.AuthorizeView(ctx, caller, requirement); d == values.AuthorizationDenied {
		return values.AuthorizationDenied, coordinatorAuthorizationDenialDigest(caller.Subject, requirement)
	}
	return values.AuthorizationValid, values.AuthorizationDenialView{}
}
func (p *coordinatorPolicy) ValidateDenialView(ctx context.Context, caller values.CallerContext, view values.AuthorizationDenialView, requirement values.Requirement) values.AuthorizationDecision {
	if view == (values.AuthorizationDenialView{}) {
		return values.AuthorizationDenied
	}
	if _, current := p.CaptureDenialView(ctx, caller, requirement); current != view {
		return values.AuthorizationChanged
	}
	return values.AuthorizationValid
}

type coordinatorFixture struct {
	ops      *Operations
	elements *element.Operations
	policy   *coordinatorPolicy
	caller   values.CallerContext
	root     values.ElementID
	handle   any
}

type noRemote struct{ t *testing.T }

func (n noRemote) OpenFederationRead(context.Context, values.CallerContext, values.NextHop, values.FactAuthorityRef, federationcontract.FederationReadRequest) (federationcontract.FederationReadConnection, error) {
	n.t.Error("local-only fixture unexpectedly dialed a remote source")
	return nil, values.PublicUnavailable
}

func coordinatorID(n int) values.ElementID {
	return values.ElementID(fmt.Sprintf("53000000-0000-4000-8000-%012x", n))
}

func coordinatorTuple(t *testing.T, root values.ElementID, x float64) contract.SpatialTuple {
	t.Helper()
	r, err := contract.NewRotation(1, 0, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	p, err := contract.NewPose(contract.Vec3{X: x}, r)
	if err != nil {
		t.Fatal(err)
	}
	s, err := contract.NewBoundedSphere(1)
	if err != nil {
		t.Fatal(err)
	}
	return contract.SpatialTuple{Parent: root, LocalPose: p, Range: s}
}

// This fixture assembles the real shared components, rather than replacing
// source/collector or exposing any production owner state for testing.
func newCoordinatorFixture(t *testing.T, retained bool) coordinatorFixture {
	t.Helper()
	authority, err := values.NewFactAuthorityRef()
	if err != nil {
		t.Fatal(err)
	}
	owner, err := commit.New(authority, [32]byte{61}, contract.RealmInitialization{Realm: "fixture-realm", NextHop: "local:fixture"})
	if err != nil {
		t.Fatal(err)
	}
	config := contract.Config{PublicAccessPolicy: &values.PublicAccessPolicy{ContextTTL: time.Hour, ObservationTTL: time.Hour}}
	if retained {
		ttl := time.Hour
		config.SnapshotRetention = &ttl
	}
	settings, err := query.NormalizeConfig(config)
	if err != nil {
		t.Fatal(err)
	}
	settings.Public.ConcurrentReads = 1
	policy := &coordinatorPolicy{}
	admission := access.New(policy, owner, settings.Public)
	queries := query.New(owner, policy, [32]byte{62}, settings)
	resources := registry.New(authority, settings.Public, owner, queries)
	observations := observation.New(owner, admission, resources)
	sources := source.New(owner, admission, resources, queries, observations)
	f := coordinatorFixture{ops: New(owner, admission, resources, sources, observations), elements: element.New(owner, policy), policy: policy, caller: values.CallerContext{Subject: values.PrincipalRef{Realm: "coordinator-test", Principal: "writer"}}, root: coordinatorID(1)}
	if _, err := f.elements.CreateRoot(f.caller.Subject, f.root, "root"); err != nil {
		t.Fatal(err)
	}
	owner.Lock()
	err = owner.SetPublicationSinkLocked(resources)
	handle, handleErr := resources.IssueSpaceLocked(f.caller, f.root)
	owner.Unlock()
	if err != nil || handleErr != nil {
		t.Fatalf("fixture publication/entry: %v %v", err, handleErr)
	}
	f.handle = accessvalue.TokenJSON(handle.Token)
	t.Cleanup(func() {
		owner.Lock()
		owner.BeginCloseLocked()
		resources.CloseLocked()
		owner.Unlock()
		resources.DrainRetired()
		queries.Close()
		_ = owner.CloseStore()
	})
	return f
}

func (f coordinatorFixture) create(t *testing.T, n int, x float64) {
	t.Helper()
	id := coordinatorID(n)
	if _, err := f.elements.CreateEntity(f.caller.Subject, id, coordinatorTuple(t, f.root, x), contract.OperationID(id)); err != nil {
		t.Fatal(err)
	}
}
func (f coordinatorFixture) move(t *testing.T, n int, x float64) {
	t.Helper()
	id := coordinatorID(n)
	read, err := f.elements.ReadElement(f.caller.Subject, id, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.elements.UpdateEntity(f.caller.Subject, id, coordinatorTuple(t, f.root, x), read.Record.Entity.Revision, "move"); err != nil {
		t.Fatal(err)
	}
}
func (f coordinatorFixture) call(t *testing.T, operation string, request map[string]any) (map[string]any, error) {
	t.Helper()
	if err := f.ops.admission.Acquire(); err != nil {
		return nil, err
	}
	defer f.ops.admission.Release()
	defer f.ops.registry.DrainRetired()
	ctx := access.WithAdmission(context.Background(), f.ops.admission)
	if operation == "begin-observe" || operation == "poll" || operation == "release-observe" {
		return f.ops.Observe(ctx, f.caller, operation, request)
	}
	return f.ops.Read(ctx, f.caller, operation, request, noRemote{t})
}
