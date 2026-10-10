package federation_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"ucupa/composition"
	"ucupa/shared/values"
	corecontract "ucupa/ucupa-core/contract"
	core "ucupa/ucupa-core/instance"
)

// This fixture owns only public instance handles and explicit test endpoints.
// It creates the real A -> B -> C mounts; no private facts or resource tables.
type federationFixture struct {
	owners  []*core.Core
	root    values.ElementID
	callers corecontract.LocalCallers
}

var storeTestActor = values.PrincipalRef{Realm: "store-realm", Principal: "store-writer"}

type publicTestAuth struct{}

func (publicTestAuth) Authorize(values.PrincipalRef, ...values.Requirement) bool { return true }
func (publicTestAuth) AuthorizeView(values.CallerContext, ...values.Requirement) (values.AuthorizationDecision, values.AuthorizationView) {
	return values.AuthorizationValid, values.AuthorizationView{1}
}
func (publicTestAuth) ValidateView(values.CallerContext, values.AuthorizationView, ...values.Requirement) values.AuthorizationDecision {
	return values.AuthorizationValid
}

// federationPolicy is the per-owner authorization surface the fixture's Realm
// contact adapter delegates to. Views stay policy-owned because every public
// call in this suite must present valid view evidence.
type federationPolicy interface {
	Authorize(values.PrincipalRef, ...values.Requirement) bool
	AuthorizeView(values.CallerContext, ...values.Requirement) (values.AuthorizationDecision, values.AuthorizationView)
	ValidateView(values.CallerContext, values.AuthorizationView, ...values.Requirement) values.AuthorizationDecision
}

// federationRealmContact adapts one federationPolicy stub to the Realm contact
// port. Federation denial evidence is not exercised by this suite, so the
// denial capture ports stay unavailable.
type federationRealmContact struct {
	policy federationPolicy
}

func (federationRealmContact) Check(context.Context, values.NextHop, values.RealmID) error {
	return nil
}

func (contact federationRealmContact) AuthorizePrincipal(_ values.NextHop, _ values.RealmID, actor values.PrincipalRef, requirements ...values.Requirement) values.AuthorizationDecision {
	if contact.policy == nil || contact.policy.Authorize(actor, requirements...) {
		return values.AuthorizationValid
	}
	return values.AuthorizationDenied
}

func (contact federationRealmContact) AuthorizeView(_ context.Context, _ values.NextHop, _ values.RealmID, caller values.CallerContext, requirements ...values.Requirement) (values.AuthorizationDecision, values.AuthorizationView) {
	return contact.policy.AuthorizeView(caller, requirements...)
}

func (contact federationRealmContact) ValidateView(_ context.Context, _ values.NextHop, _ values.RealmID, caller values.CallerContext, view values.AuthorizationView, requirements ...values.Requirement) values.AuthorizationDecision {
	return contact.policy.ValidateView(caller, view, requirements...)
}

func (federationRealmContact) CaptureDenialView(context.Context, values.NextHop, values.RealmID, values.CallerContext, values.Requirement) (values.AuthorizationDecision, values.AuthorizationDenialView) {
	return values.AuthorizationUnavailable, values.AuthorizationDenialView{}
}

func (federationRealmContact) ValidateDenialView(context.Context, values.NextHop, values.RealmID, values.CallerContext, values.AuthorizationDenialView, values.Requirement) values.AuthorizationDecision {
	return values.AuthorizationUnavailable
}
func federationConfig(retained bool) corecontract.Config {
	config := corecontract.Config{PublicAccessPolicy: &values.PublicAccessPolicy{ContextTTL: time.Hour, ObservationTTL: time.Hour}}
	if retained {
		duration := time.Hour
		config.SnapshotRetention = &duration
	}
	return config
}
func storeTestID(serial uint64) values.ElementID {
	return values.ElementID(fmt.Sprintf("20000000-0000-4000-8000-%012x", serial))
}
func nestedMountAddress(index int) values.NextHop {
	return values.NextHop(fmt.Sprintf("local:nested-%d", index))
}
func storeTestTuple(t *testing.T, parent values.ElementID, x, radius float64) corecontract.SpatialTuple {
	t.Helper()
	rotation, err := corecontract.NewRotation(1, 0, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	pose, err := corecontract.NewPose(corecontract.Vec3{X: x}, rotation)
	if err != nil {
		t.Fatal(err)
	}
	sphere, err := corecontract.NewBoundedSphere(radius)
	if err != nil {
		t.Fatal(err)
	}
	return corecontract.SpatialTuple{Parent: parent, LocalPose: pose, Range: sphere}
}
func newFederationFixture(t *testing.T, configs []corecontract.Config, policies []federationPolicy) (*federationFixture, *vectorTestConnections, values.CallerContext) {
	t.Helper()
	f := &federationFixture{root: storeTestID(9700), callers: make(corecontract.LocalCallers)}
	mounts := &composition.LocalMountConnections{}
	connections := &vectorTestConnections{owners: make(map[values.NextHop]sourcePort)}
	for i, config := range configs {
		var policy federationPolicy = publicTestAuth{}
		if policies != nil && policies[i] != nil {
			policy = policies[i]
		}
		initial := corecontract.RealmInitialization{Realm: storeTestActor.Realm, NextHop: nestedMountAddress(i)}
		owner, err := core.New(federationRealmContact{policy}, initial, config)
		if err != nil {
			t.Fatal(err)
		}
		f.owners = append(f.owners, owner)
		f.callers[owner.AuthorityRef()] = storeTestActor
		connections.owners[nestedMountAddress(i)] = owner.Federation()
		if err := mounts.BindEndpoint(nestedMountAddress(i), owner.Mounts().MountPeer()); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() {
		for _, owner := range f.owners {
			if err := owner.Close(); err != nil {
				t.Error(err)
			}
		}
	})
	connections.source = f.owners[0].AuthorityRef()
	for _, owner := range f.owners {
		transport, err := mounts.TransportFor(owner)
		if err != nil {
			t.Fatal(err)
		}
		if err := owner.Mounts().ConfigureMountTransport(transport); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := f.owners[0].Elements().CreateRoot(storeTestActor, f.root, "nested-root"); err != nil {
		t.Fatal(err)
	}
	return f, connections, values.CallerContext{Subject: storeTestActor}
}
func vectorFixture(t *testing.T, retained bool) (*federationFixture, *vectorTestConnections, values.CallerContext) {
	return vectorFixtureWithPolicy(t, retained, nil)
}
func vectorFixtureWithPolicy(t *testing.T, retained bool, policies []federationPolicy) (*federationFixture, *vectorTestConnections, values.CallerContext) {
	t.Helper()
	f, connections, caller := newFederationFixture(t, []corecontract.Config{federationConfig(retained), federationConfig(retained), federationConfig(retained)}, policies)
	f.establish(t, 0, 1, storeTestID(9710), f.root, 20, "vector-ab")
	f.establish(t, 1, 2, storeTestID(9720), storeTestID(9710), 10, "vector-bc")
	return f, connections, caller
}
func (f *federationFixture) establish(t *testing.T, parent, carrier int, id, parentID values.ElementID, radius float64, op corecontract.OperationID) {
	t.Helper()
	anchor, err := f.owners[parent].Elements().CreateAnchor(storeTestActor, id, storeTestTuple(t, parentID, 0, radius), corecontract.OperationID("anchor-"+string(id)))
	if err != nil || anchor.Record == nil {
		t.Fatalf("anchor: %+v %v", anchor, err)
	}
	result, err := f.owners[parent].Mounts().ApplyMount(context.Background(), f.callers, op, corecontract.MountCommand{
		Kind: corecontract.MountEstablish, MountAnchorUUID: id, ExpectedElementRevision: anchor.Record.Anchor.Revision,
		CarrierAuthority: f.owners[carrier].AuthorityRef(), CarrierNextHop: nestedMountAddress(carrier), ParentNextHop: nestedMountAddress(parent),
	})
	if err != nil || result.Decision != corecontract.MountCommitted || !result.Settled {
		t.Fatalf("establish %s: %+v %v", op, result, err)
	}
}
func observationVectorWrite(t *testing.T, f *federationFixture, index int, id uint64) {
	t.Helper()
	parents := []values.ElementID{f.root, storeTestID(9710), storeTestID(9720)}
	if _, err := f.owners[index].Elements().CreateEntity(storeTestActor, storeTestID(id), storeTestTuple(t, parents[index], 0, 1), corecontract.OperationID(storeTestID(id))); err != nil {
		t.Fatal(err)
	}
}

// Explicit protocol request data, not a call into the implementation's JSON
// projection helper. Tests exercise the public parser and address semantics.
func pathJSON(origin values.ElementID, down []values.ElementID) map[string]any {
	chain := make([]any, len(down))
	for i, id := range down {
		chain[i] = string(id)
	}
	return map[string]any{"origin": string(origin), "up": "0", "down": chain}
}
