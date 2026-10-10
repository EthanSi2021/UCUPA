package element_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"

	"ucupa/shared/values"
	corecontract "ucupa/ucupa-core/contract"
	core "ucupa/ucupa-core/instance"
)

var actor = values.PrincipalRef{Realm: "test-realm", Principal: "builder"}

var elementContact = corecontract.RealmInitialization{Realm: actor.Realm, NextHop: "local:element-realm"}

// realmPolicyContact adapts one fixed bool authorization stub to the Realm
// contact port. Address identity is not a variable in these tests, and the
// view evidence stays unavailable exactly as it was before the port existed.
type realmPolicyContact struct {
	policy interface {
		Authorize(values.PrincipalRef, ...values.Requirement) bool
	}
}

func (realmPolicyContact) Check(context.Context, values.NextHop, values.RealmID) error { return nil }

func (contact realmPolicyContact) AuthorizePrincipal(_ values.NextHop, _ values.RealmID, actor values.PrincipalRef, requirements ...values.Requirement) values.AuthorizationDecision {
	if contact.policy == nil || contact.policy.Authorize(actor, requirements...) {
		return values.AuthorizationValid
	}
	return values.AuthorizationDenied
}

func (realmPolicyContact) AuthorizeView(context.Context, values.NextHop, values.RealmID, values.CallerContext, ...values.Requirement) (values.AuthorizationDecision, values.AuthorizationView) {
	return values.AuthorizationUnavailable, values.AuthorizationView{}
}

func (realmPolicyContact) ValidateView(context.Context, values.NextHop, values.RealmID, values.CallerContext, values.AuthorizationView, ...values.Requirement) values.AuthorizationDecision {
	return values.AuthorizationUnavailable
}

func (realmPolicyContact) CaptureDenialView(context.Context, values.NextHop, values.RealmID, values.CallerContext, values.Requirement) (values.AuthorizationDecision, values.AuthorizationDenialView) {
	return values.AuthorizationUnavailable, values.AuthorizationDenialView{}
}

func (realmPolicyContact) ValidateDenialView(context.Context, values.NextHop, values.RealmID, values.CallerContext, values.AuthorizationDenialView, values.Requirement) values.AuthorizationDecision {
	return values.AuthorizationUnavailable
}

type allowAuthorizer struct{}

func (allowAuthorizer) Authorize(values.PrincipalRef, ...values.Requirement) bool { return true }

type recordingAuthorizer struct {
	mu     sync.Mutex
	calls  [][]values.Requirement
	deny   bool
	denyAt int
}

type selectiveAuthorizer struct {
	denied values.PermissionID
}

type blockingAuthorizer struct {
	mu      sync.Mutex
	calls   int
	blockAt int
	entered chan struct{}
	release chan struct{}
}

func (a *blockingAuthorizer) Authorize(_ values.PrincipalRef, _ ...values.Requirement) bool {
	a.mu.Lock()
	a.calls++
	call := a.calls
	blockAt, entered, release := a.blockAt, a.entered, a.release
	a.mu.Unlock()
	if blockAt != 0 && call == blockAt {
		close(entered)
		<-release
	}
	return true
}

func (a *recordingAuthorizer) Authorize(_ values.PrincipalRef, requirements ...values.Requirement) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.calls = append(a.calls, append([]values.Requirement(nil), requirements...))
	return !a.deny && (a.denyAt == 0 || len(a.calls) != a.denyAt)
}

func (a *selectiveAuthorizer) Authorize(_ values.PrincipalRef, requirements ...values.Requirement) bool {
	for _, requirement := range requirements {
		if requirement.Permission == a.denied {
			return false
		}
	}
	return true
}

func newCore(t *testing.T) *core.Core {
	t.Helper()
	c, err := core.New(realmPolicyContact{allowAuthorizer{}}, elementContact, corecontract.Config{})
	if err != nil {
		t.Fatal(err)
	}
	return c
}
func id(n uint64) values.ElementID {
	return values.ElementID(fmt.Sprintf("00000000-0000-4000-8000-%012x", n))
}
func pose(t *testing.T, x, y, z float64) corecontract.Pose {
	t.Helper()
	r, err := corecontract.NewRotation(1, 0, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	p, err := corecontract.NewPose(corecontract.Vec3{X: x, Y: y, Z: z}, r)
	if err != nil {
		t.Fatal(err)
	}
	return p
}
func sphere(t *testing.T, r float64) corecontract.BoundedSphere {
	t.Helper()
	s, err := corecontract.NewBoundedSphere(r)
	if err != nil {
		t.Fatal(err)
	}
	return s
}
func tuple(t *testing.T, parent values.ElementID, p corecontract.Pose, r float64) corecontract.SpatialTuple {
	return tupleWith(t, parent, p, r)
}
func tupleWith(t *testing.T, parent values.ElementID, p corecontract.Pose, r float64) corecontract.SpatialTuple {
	t.Helper()
	return corecontract.SpatialTuple{Parent: parent, LocalPose: p, Range: sphere(t, r)}
}
func createRoot(t *testing.T, c *core.Core, v values.ElementID, op corecontract.OperationID) corecontract.ElementRecord {
	t.Helper()
	r, e := c.Elements().CreateRoot(actor, v, op)
	if e != nil || r.Status != corecontract.MutationCommitted || r.Record == nil {
		t.Fatalf("create root: %#v %v", r, e)
	}
	return *r.Record
}
func createAnchor(t *testing.T, c *core.Core, v, parent values.ElementID, final corecontract.SpatialTuple, op corecontract.OperationID) corecontract.ElementRecord {
	t.Helper()
	if final.Parent != parent {
		t.Fatal("bad helper parent")
	}
	r, e := c.Elements().CreateAnchor(actor, v, final, op)
	if e != nil || r.Status != corecontract.MutationCommitted || r.Record == nil {
		t.Fatalf("create anchor: %#v %v", r, e)
	}
	return *r.Record
}
func createEntity(t *testing.T, c *core.Core, v values.ElementID, final corecontract.SpatialTuple, op corecontract.OperationID) corecontract.ElementRecord {
	t.Helper()
	r, e := c.Elements().CreateEntity(actor, v, final, op)
	if e != nil || r.Status != corecontract.MutationCommitted || r.Record == nil {
		t.Fatalf("create entity: %#v %v", r, e)
	}
	return *r.Record
}
func assertRejected(t *testing.T, r corecontract.MutationResult, err, want error) {
	t.Helper()
	if !errors.Is(err, want) || r.Status != corecontract.MutationRejected || r.Record != nil || r.OutcomeCut.Equal(corecontract.CoreReadCut{}) {
		t.Fatalf("rejected %#v err=%v want=%v", r, err, want)
	}
}
func recordID(r corecontract.ElementRecord) values.ElementID {
	if r.Kind == corecontract.ElementAnchor {
		return r.Anchor.ID
	}
	return r.Entity.ID
}

func requirementTargets(authority values.FactAuthorityRef, requirements []values.Requirement) []values.ElementID {
	targets := make([]values.ElementID, 0, len(requirements))
	for _, requirement := range requirements {
		owner, target, ok := requirement.Scope.CoreElementTarget()
		if !ok || owner != authority {
			return nil
		}
		targets = append(targets, target)
	}
	return targets
}
