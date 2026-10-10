package source

import (
	"context"
	"crypto/sha256"
	"fmt"
	"testing"
	"time"

	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	"ucupa/ucupa-core/internal/access"
	"ucupa/ucupa-core/internal/access/observation"
	"ucupa/ucupa-core/internal/access/registry"
	"ucupa/ucupa-core/internal/commit"
	"ucupa/ucupa-core/internal/element"
	"ucupa/ucupa-core/internal/mount"
	"ucupa/ucupa-core/internal/query"
)

type sourcePolicy struct {
	denyRead  bool
	readEpoch byte
	epoch     byte
}

func sourceAuthorizationDenialDigest(subject values.PrincipalRef, requirement values.Requirement) values.AuthorizationDenialView {
	return values.AuthorizationDenialView(sha256.Sum256([]byte(fmt.Sprintf("denial:%v:%v", subject, requirement))))
}

func (*sourcePolicy) AuthorizePrincipal(values.PrincipalRef, ...values.Requirement) values.AuthorizationDecision {
	return values.AuthorizationValid
}
func (a *sourcePolicy) AuthorizeView(_ context.Context, _ values.CallerContext, reqs ...values.Requirement) (values.AuthorizationDecision, values.AuthorizationView) {
	v := values.AuthorizationView{1, a.epoch}
	for _, r := range reqs {
		if r.Permission == contract.PermissionReadElement {
			if a.denyRead {
				return values.AuthorizationDenied, values.AuthorizationView{}
			}
			v[2] = a.readEpoch
		}
	}
	return values.AuthorizationValid, v
}
func (a *sourcePolicy) ValidateView(ctx context.Context, c values.CallerContext, v values.AuthorizationView, reqs ...values.Requirement) values.AuthorizationDecision {
	d, want := a.AuthorizeView(ctx, c, reqs...)
	if d != values.AuthorizationValid {
		return d
	}
	if v != want {
		return values.AuthorizationChanged
	}
	return values.AuthorizationValid
}
func (a *sourcePolicy) CaptureDenialView(ctx context.Context, caller values.CallerContext, requirement values.Requirement) (values.AuthorizationDecision, values.AuthorizationDenialView) {
	if d, _ := a.AuthorizeView(ctx, caller, requirement); d == values.AuthorizationDenied {
		return values.AuthorizationDenied, sourceAuthorizationDenialDigest(caller.Subject, requirement)
	}
	return values.AuthorizationValid, values.AuthorizationDenialView{}
}
func (a *sourcePolicy) ValidateDenialView(ctx context.Context, caller values.CallerContext, view values.AuthorizationDenialView, requirement values.Requirement) values.AuthorizationDecision {
	if view == (values.AuthorizationDenialView{}) {
		return values.AuthorizationDenied
	}
	if _, current := a.CaptureDenialView(ctx, caller, requirement); current != view {
		return values.AuthorizationChanged
	}
	return values.AuthorizationValid
}

type sourceFixture struct {
	ops      *Operations
	owner    *commit.Owner
	elements *element.Operations
	mounts   *mount.Operations
	auth     *sourcePolicy
	caller   values.CallerContext
	root     values.ElementID
}

func sourceID(n uint64) values.ElementID {
	return values.ElementID(fmt.Sprintf("52000000-0000-4000-8000-%012x", n))
}

func newSourceFixture(t *testing.T, retained bool) sourceFixture {
	return newSourceFixtureWithRoot(t, retained, true)
}

func newSourceCarrierFixture(t *testing.T) sourceFixture {
	return newSourceFixtureWithRoot(t, false, false)
}

func newSourceFixtureWithRoot(t *testing.T, retained, createRoot bool) sourceFixture {
	auth := &sourcePolicy{}
	return newSourceFixtureWithAuthorizer(t, retained, createRoot, auth)
}

func newSourceFixtureWithAuthorizer(t *testing.T, retained, createRoot bool, authorizer contract.Authorizer) sourceFixture {
	return newSourceFixtureWithStore(t, retained, createRoot, authorizer, "")
}

func newSourceFixtureWithStore(t *testing.T, retained, createRoot bool, authorizer contract.Authorizer, path string) sourceFixture {
	t.Helper()
	authority, err := values.NewFactAuthorityRef()
	if err != nil {
		t.Fatal(err)
	}
	owner, err := commit.New(authority, [32]byte{52}, contract.RealmInitialization{Realm: "fixture-realm", NextHop: "local:fixture"})
	if err != nil {
		t.Fatal(err)
	}
	if path != "" {
		owner, err = commit.CreateDurable(path, authority, [32]byte{52}, contract.RealmInitialization{Realm: "fixture-realm", NextHop: "local:fixture"}, nil)
		if err != nil {
			t.Fatal(err)
		}
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
	admission := access.New(authorizer, owner, settings.Public)
	queries := query.New(owner, authorizer, [32]byte{53}, settings)
	resources := registry.New(authority, settings.Public, owner, queries)
	ops := New(owner, admission, resources, queries, observation.New(owner, admission, resources))
	auth, _ := authorizer.(*sourcePolicy)
	f := sourceFixture{ops: ops, owner: owner, elements: element.New(owner, authorizer), mounts: mount.New(owner, authorizer, admission), auth: auth, caller: values.CallerContext{Subject: values.PrincipalRef{Realm: "source-test", Principal: "writer"}}, root: sourceID(1)}
	if createRoot {
		if _, err := f.elements.CreateRoot(f.caller.Subject, f.root, "root"); err != nil {
			t.Fatal(err)
		}
	}
	owner.Lock()
	err = owner.SetPublicationSinkLocked(resources)
	owner.Unlock()
	if err != nil {
		t.Fatal(err)
	}
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

// sourceMountTransport is the same exact-address, identity-checked local
// transport used by the real Mount operations. The optional channels pause one
// Prepare only after the parent has durably reserved its operation; they do not
// fabricate a mount fact or bypass the peer protocol.
type sourceMountTransport struct {
	source       values.FactAuthorityRef
	endpoints    map[values.NextHop]*mount.Operations
	pauseEntered chan<- struct{}
	pauseRelease <-chan struct{}
}

func (transport *sourceMountTransport) target(address values.NextHop, expected values.FactAuthorityRef) (contract.MountPeerPort, error) {
	target := transport.endpoints[address]
	if target == nil {
		return nil, contract.ErrUnavailable
	}
	if target.AuthorityRef() != expected {
		return nil, contract.ErrOwnerMismatch
	}
	return target.MountPeer(), nil
}

func (transport *sourceMountTransport) Prepare(ctx context.Context, address values.NextHop, expected values.FactAuthorityRef, request contract.MountPrepareRequest) (contract.MountPrepared, error) {
	peer, err := transport.target(address, expected)
	if err != nil {
		return contract.MountPrepared{}, err
	}
	if transport.pauseEntered != nil {
		select {
		case transport.pauseEntered <- struct{}{}:
		default:
		}
		select {
		case <-transport.pauseRelease:
		case <-ctx.Done():
			return contract.MountPrepared{}, contract.ErrUnavailable
		case <-time.After(3 * time.Second):
			return contract.MountPrepared{}, contract.ErrUnavailable
		}
	}
	return peer.Prepare(ctx, transport.source, request)
}

func (transport *sourceMountTransport) Finish(ctx context.Context, address values.NextHop, expected values.FactAuthorityRef, request contract.MountFinishRequest) (contract.MountClosed, error) {
	peer, err := transport.target(address, expected)
	if err != nil {
		return contract.MountClosed{}, err
	}
	return peer.Finish(ctx, transport.source, request)
}

func (transport *sourceMountTransport) ReadDecision(ctx context.Context, address values.NextHop, expected values.FactAuthorityRef, key contract.MountAttemptKey) (contract.MountDecisionEvidence, error) {
	peer, err := transport.target(address, expected)
	if err != nil {
		return contract.MountDecisionEvidence{}, err
	}
	return peer.ReadDecision(ctx, transport.source, key)
}

func connectSourceMounts(t *testing.T, parent, carrier sourceFixture) *sourceMountTransport {
	t.Helper()
	parentTransport := &sourceMountTransport{source: parent.ops.AuthorityRef(), endpoints: map[values.NextHop]*mount.Operations{"local:source-carrier": carrier.mounts}}
	carrierTransport := &sourceMountTransport{source: carrier.ops.AuthorityRef(), endpoints: map[values.NextHop]*mount.Operations{"local:source-parent": parent.mounts}}
	if err := parent.mounts.ConfigureMountTransport(parentTransport); err != nil {
		t.Fatal(err)
	}
	if err := carrier.mounts.ConfigureMountTransport(carrierTransport); err != nil {
		t.Fatal(err)
	}
	return parentTransport
}

func sourceTuple(t *testing.T, parent values.ElementID, x float64) contract.SpatialTuple {
	t.Helper()
	return sourceTupleWithRadius(t, parent, x, 1)
}

func sourceTupleWithRadius(t *testing.T, parent values.ElementID, x, radius float64) contract.SpatialTuple {
	t.Helper()
	rotation, err := contract.NewRotation(1, 0, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	pose, err := contract.NewPose(contract.Vec3{X: x}, rotation)
	if err != nil {
		t.Fatal(err)
	}
	sphere, err := contract.NewBoundedSphere(radius)
	if err != nil {
		t.Fatal(err)
	}
	return contract.SpatialTuple{Parent: parent, LocalPose: pose, Range: sphere}
}

func (f sourceFixture) create(t *testing.T, n uint64, x float64) {
	t.Helper()
	f.createUnder(t, n, f.root, x)
}

func (f sourceFixture) createUnder(t *testing.T, n uint64, parent values.ElementID, x float64) values.ElementID {
	t.Helper()
	id := sourceID(n)
	if _, err := f.elements.CreateEntity(f.caller.Subject, id, sourceTuple(t, parent, x), contract.OperationID(id)); err != nil {
		t.Fatal(err)
	}
	return id
}

func (f sourceFixture) createAnchor(t *testing.T, n uint64, parent values.ElementID, x, radius float64) values.ElementID {
	t.Helper()
	id := sourceID(n)
	if _, err := f.elements.CreateAnchor(f.caller.Subject, id, sourceTupleWithRadius(t, parent, x, radius), contract.OperationID(id)); err != nil {
		t.Fatal(err)
	}
	return id
}

func (f sourceFixture) move(t *testing.T, n uint64, x float64) {
	t.Helper()
	id := sourceID(n)
	old, err := f.elements.ReadElement(f.caller.Subject, id, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.elements.UpdateEntity(f.caller.Subject, id, sourceTuple(t, f.root, x), old.Record.Entity.Revision, "move"); err != nil {
		t.Fatal(err)
	}
}
