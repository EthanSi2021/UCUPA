package connected_test

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"ucupa/shared/values"
	corecontract "ucupa/ucupa-core/contract"
	core "ucupa/ucupa-core/instance"
)

var localActor = values.PrincipalRef{Realm: "oracle-realm", Principal: "oracle"}

var connectedContact = corecontract.RealmInitialization{Realm: localActor.Realm, NextHop: "local:connected-realm"}

// localRealmContact adapts the switching bool policy stub to the Realm contact
// port. Address identity is not a variable here, and view evidence stays
// unavailable exactly as it was before the port existed.
type localRealmContact struct{ policy *switchPolicy }

func (localRealmContact) Check(context.Context, values.NextHop, values.RealmID) error { return nil }

func (contact localRealmContact) AuthorizePrincipal(_ values.NextHop, _ values.RealmID, actor values.PrincipalRef, requirements ...values.Requirement) values.AuthorizationDecision {
	if contact.policy == nil || contact.policy.Authorize(actor, requirements...) {
		return values.AuthorizationValid
	}
	return values.AuthorizationDenied
}

func (localRealmContact) AuthorizeView(context.Context, values.NextHop, values.RealmID, values.CallerContext, ...values.Requirement) (values.AuthorizationDecision, values.AuthorizationView) {
	return values.AuthorizationUnavailable, values.AuthorizationView{}
}

func (localRealmContact) ValidateView(context.Context, values.NextHop, values.RealmID, values.CallerContext, values.AuthorizationView, ...values.Requirement) values.AuthorizationDecision {
	return values.AuthorizationUnavailable
}

func (localRealmContact) CaptureDenialView(context.Context, values.NextHop, values.RealmID, values.CallerContext, values.Requirement) (values.AuthorizationDecision, values.AuthorizationDenialView) {
	return values.AuthorizationUnavailable, values.AuthorizationDenialView{}
}

func (localRealmContact) ValidateDenialView(context.Context, values.NextHop, values.RealmID, values.CallerContext, values.AuthorizationDenialView, values.Requirement) values.AuthorizationDecision {
	return values.AuthorizationUnavailable
}

type authorizeFunc func(values.PrincipalRef, ...values.Requirement) bool
type switchPolicy struct {
	mu sync.RWMutex
	fn authorizeFunc
}

func (p *switchPolicy) set(fn authorizeFunc) { p.mu.Lock(); p.fn = fn; p.mu.Unlock() }
func (p *switchPolicy) Authorize(actor values.PrincipalRef, requirements ...values.Requirement) bool {
	p.mu.RLock()
	fn := p.fn
	p.mu.RUnlock()
	// Invoke outside the fixture lock: the original diagnostic test changes
	// facts from this callback, and the real authorization call is reentrant.
	if fn == nil {
		return true
	}
	return fn(actor, requirements...)
}

type localCore struct {
	*core.Core
	policy *switchPolicy
}

func localFixture(t *testing.T, config corecontract.Config) (*localCore, corecontract.RegionQuery, []values.ElementID) {
	t.Helper()
	policy := &switchPolicy{}
	instance, err := core.New(localRealmContact{policy: policy}, connectedContact, config)
	if err != nil {
		t.Fatal(err)
	}
	c := &localCore{Core: instance, policy: policy}
	t.Cleanup(func() { _ = c.Close() })
	root := localCreateRoot(t, c, localID(91000), "snapshot-root")
	ids := []values.ElementID{localID(91001), localID(91002), localID(91003)}
	for _, id := range ids {
		localCreateEntity(t, c, id, localTuple(t, root, 0, 0, 0, 0), corecontract.OperationID("snapshot-"+string(id)))
	}
	return c, corecontract.RegionQuery{StartAnchorID: root, BasisAnchorID: root, Selector: corecontract.QueryEntities}, ids
}

func localID(n uint64) values.ElementID {
	return values.ElementID(fmt.Sprintf("10000000-0000-4000-8000-%012x", n))
}

func localTuple(t *testing.T, parent values.ElementID, x, y, z, radius float64) corecontract.SpatialTuple {
	t.Helper()
	rotation, err := corecontract.NewRotation(1, 0, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	pose, err := corecontract.NewPose(corecontract.Vec3{X: x, Y: y, Z: z}, rotation)
	if err != nil {
		t.Fatal(err)
	}
	sphere, err := corecontract.NewBoundedSphere(radius)
	if err != nil {
		t.Fatal(err)
	}
	return corecontract.SpatialTuple{Parent: parent, LocalPose: pose, Range: sphere}
}

func localCreateRoot(t *testing.T, c *localCore, id values.ElementID, op corecontract.OperationID) values.ElementID {
	t.Helper()
	result, err := c.Elements().CreateRoot(localActor, id, op)
	if err != nil || result.Record == nil {
		t.Fatalf("root %#v err=%v", result, err)
	}
	return result.Record.Anchor.ID
}

func localCreateAnchor(t *testing.T, c *localCore, id values.ElementID, tuple corecontract.SpatialTuple, op corecontract.OperationID) values.ElementID {
	t.Helper()
	result, err := c.Elements().CreateAnchor(localActor, id, tuple, op)
	if err != nil || result.Record == nil {
		t.Fatalf("anchor %#v err=%v", result, err)
	}
	return result.Record.Anchor.ID
}

func localCreateEntity(t *testing.T, c *localCore, id values.ElementID, tuple corecontract.SpatialTuple, op corecontract.OperationID) values.ElementID {
	t.Helper()
	result, err := c.Elements().CreateEntity(localActor, id, tuple, op)
	if err != nil || result.Record == nil {
		t.Fatalf("entity %#v err=%v", result, err)
	}
	return result.Record.Entity.ID
}
