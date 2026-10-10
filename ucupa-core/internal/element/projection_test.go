package element

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"reflect"
	"testing"
	"time"

	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	"ucupa/ucupa-core/internal/commit"
	elementfact "ucupa/ucupa-core/internal/element/fact"
)

var projectActor = values.PrincipalRef{Realm: "projection-test", Principal: "reader"}

const (
	projectRoot    values.ElementID = "20000000-0000-4000-8000-000000000001"
	projectMiddle  values.ElementID = "20000000-0000-4000-8000-000000000002"
	projectSubject values.ElementID = "20000000-0000-4000-8000-000000000003"
	projectOther   values.ElementID = "20000000-0000-4000-8000-000000000004"
)

type projectAuthorize func(values.PrincipalRef, ...values.Requirement) bool

func projectAuthorizationViewDigest(subject values.PrincipalRef, requirements []values.Requirement) values.AuthorizationView {
	return values.AuthorizationView(sha256.Sum256([]byte(fmt.Sprintf("view:%v:%v", subject, requirements))))
}

func projectAuthorizationDenialDigest(subject values.PrincipalRef, requirement values.Requirement) values.AuthorizationDenialView {
	return values.AuthorizationDenialView(sha256.Sum256([]byte(fmt.Sprintf("denial:%v:%v", subject, requirement))))
}

func (f projectAuthorize) AuthorizePrincipal(actor values.PrincipalRef, requirements ...values.Requirement) values.AuthorizationDecision {
	if f(actor, requirements...) {
		return values.AuthorizationValid
	}
	return values.AuthorizationDenied
}

func (f projectAuthorize) AuthorizeView(ctx context.Context, caller values.CallerContext, requirements ...values.Requirement) (values.AuthorizationDecision, values.AuthorizationView) {
	if f.AuthorizePrincipal(caller.Subject, requirements...) != values.AuthorizationValid {
		return values.AuthorizationDenied, values.AuthorizationView{}
	}
	return values.AuthorizationValid, projectAuthorizationViewDigest(caller.Subject, requirements)
}

func (f projectAuthorize) ValidateView(ctx context.Context, caller values.CallerContext, view values.AuthorizationView, requirements ...values.Requirement) values.AuthorizationDecision {
	decision, current := f.AuthorizeView(ctx, caller, requirements...)
	if decision != values.AuthorizationValid {
		return decision
	}
	if current != view {
		return values.AuthorizationChanged
	}
	return values.AuthorizationValid
}

func (f projectAuthorize) CaptureDenialView(ctx context.Context, caller values.CallerContext, requirement values.Requirement) (values.AuthorizationDecision, values.AuthorizationDenialView) {
	if f.AuthorizePrincipal(caller.Subject, requirement) == values.AuthorizationDenied {
		return values.AuthorizationDenied, projectAuthorizationDenialDigest(caller.Subject, requirement)
	}
	return values.AuthorizationValid, values.AuthorizationDenialView{}
}

func (f projectAuthorize) ValidateDenialView(ctx context.Context, caller values.CallerContext, view values.AuthorizationDenialView, requirement values.Requirement) values.AuthorizationDecision {
	if view == (values.AuthorizationDenialView{}) {
		return values.AuthorizationDenied
	}
	if _, current := f.CaptureDenialView(ctx, caller, requirement); current != view {
		return values.AuthorizationChanged
	}
	return values.AuthorizationValid
}

func projectTuple(t *testing.T, parent values.ElementID, x, radius float64) contract.SpatialTuple {
	t.Helper()
	rotation, err := contract.NewRotation(1, 0, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	pose, err := contract.NewPose(contract.Vec3{X: x}, rotation)
	if err != nil {
		t.Fatal(err)
	}
	extent, err := contract.NewBoundedSphere(radius)
	if err != nil {
		t.Fatal(err)
	}
	return contract.SpatialTuple{Parent: parent, LocalPose: pose, Range: extent}
}

func projectFixture(t *testing.T) (*Operations, *commit.Owner) {
	t.Helper()
	authority, err := values.NewFactAuthorityRef()
	if err != nil {
		t.Fatal(err)
	}
	owner, err := commit.New(authority, [32]byte{17}, contract.RealmInitialization{Realm: "fixture-realm", NextHop: "local:fixture"})
	if err != nil {
		t.Fatal(err)
	}
	ops := New(owner, projectAuthorize(func(values.PrincipalRef, ...values.Requirement) bool { return true }))
	if _, err := ops.CreateRoot(projectActor, projectRoot, "root"); err != nil {
		t.Fatal(err)
	}
	if _, err := ops.CreateAnchor(projectActor, projectMiddle, projectTuple(t, projectRoot, 10, 20), "middle"); err != nil {
		t.Fatal(err)
	}
	if _, err := ops.CreateEntity(projectActor, projectSubject, projectTuple(t, projectMiddle, 2, 1), "subject"); err != nil {
		t.Fatal(err)
	}
	return ops, owner
}

func TestProjectionActualEntryKeepsPathAuthorizationAndExactResult(t *testing.T) {
	ops, owner := projectFixture(t)
	var calls [][]values.ElementID
	ops.authorizer = projectAuthorize(func(actor values.PrincipalRef, requirements ...values.Requirement) bool {
		if actor != projectActor {
			t.Error("actor changed")
			return false
		}
		var ids []values.ElementID
		for _, requirement := range requirements {
			authority, id, ok := requirement.Scope.CoreElementTarget()
			if !ok || authority != owner.AuthorityRef() || requirement.Permission != contract.PermissionReadElement {
				t.Error("projection requirement changed")
				return false
			}
			ids = append(ids, id)
		}
		calls = append(calls, ids)
		return true
	})
	result, err := ops.ProjectToAncestor(projectActor, projectSubject, projectRoot, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := [][]values.ElementID{{projectSubject, projectRoot}, {projectSubject, projectMiddle, projectRoot}}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("authorization targets: %v", calls)
	}
	owner.RLock()
	facts := owner.FactsLocked()
	entity, exists := facts.Elements.Entity(projectSubject)
	owner.RUnlock()
	if !exists || result.Pose != projectTuple(t, projectRoot, 12, 1).LocalPose || result.SubjectRevision != entity.Revision || result.Cut != facts.CurrentCut() {
		t.Fatalf("projection differs from same-cut path: %#v", result)
	}
	calls = nil
	identity, err := ops.ProjectToAncestor(projectActor, projectMiddle, projectMiddle, nil)
	if err != nil || identity.Pose != projectTuple(t, projectRoot, 0, 1).LocalPose || !reflect.DeepEqual(calls, [][]values.ElementID{{projectMiddle}, {projectMiddle}}) {
		t.Fatalf("identity/deduplicated path: %#v %v %v", identity, calls, err)
	}
}

func TestProjectionActualEntryRechecksPermissionCutAndRetirement(t *testing.T) {
	for _, reason := range []string{"denied", "written", "closed"} {
		t.Run(reason, func(t *testing.T) {
			ops, owner := projectFixture(t)
			calls := 0
			ops.authorizer = projectAuthorize(func(_ values.PrincipalRef, requirements ...values.Requirement) bool {
				if requirements[0].Permission != contract.PermissionReadElement {
					return true
				}
				calls++
				if calls != 2 {
					return true
				}
				switch reason {
				case "denied":
					return false
				case "written":
					_, err := ops.CreateEntity(projectActor, projectOther, projectTuple(t, projectRoot, 0, 1), "between-authorizations")
					if err != nil {
						t.Error(err)
					}
				case "closed":
					owner.Lock()
					owner.BeginCloseLocked()
					owner.Unlock()
				}
				return true
			})
			type reply struct {
				result contract.Projection
				err    error
			}
			done := make(chan reply, 1)
			go func() {
				result, err := ops.ProjectToAncestor(projectActor, projectSubject, projectRoot, nil)
				done <- reply{result, err}
			}()
			want := map[string]error{"denied": contract.ErrDenied, "written": contract.ErrReadCutExpired, "closed": contract.ErrStore}[reason]
			select {
			case out := <-done:
				if !errors.Is(out.err, want) || out.result != (contract.Projection{}) {
					t.Fatalf("return gate: %#v %v, want %v", out.result, out.err, want)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("path authorization held the owner lock")
			}
		})
	}
}

func TestProjectionBrokenPathKeepsDiscoveredScopePrefix(t *testing.T) {
	for _, parent := range []values.ElementID{projectOther, projectMiddle} {
		builder := elementfact.NewProjection()
		builder.AddAnchor(projectRoot, elementfact.Anchor{ID: projectRoot})
		builder.AddAnchor(projectMiddle, elementfact.Anchor{ID: projectMiddle, Parent: parent})
		builder.AddEntity(projectSubject, elementfact.Entity{ID: projectSubject, Parent: projectMiddle})
		facts := commit.Facts{Elements: builder.Build().View}
		ids, _, _, err := subjectPathToFrameBoundary(facts, projectSubject)
		want := []values.ElementID{projectSubject, projectMiddle}
		if parent == projectOther {
			want = append(want, projectOther)
		}
		if !errors.Is(err, contract.ErrStore) || !reflect.DeepEqual(ids, want) {
			t.Fatalf("corrupt path prefix: %v %v", ids, err)
		}
	}
}

func TestProjectionNilAndCutErrorOrder(t *testing.T) {
	var absent *Operations
	if out, err := absent.ProjectToAncestor(projectActor, projectSubject, projectRoot, nil); !errors.Is(err, contract.ErrInvalid) || out != (contract.Projection{}) {
		t.Fatalf("nil entry: %#v %v", out, err)
	}
	ops, _ := projectFixture(t)
	old, err := ops.ReadElement(projectActor, projectSubject, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ops.CreateEntity(projectActor, projectOther, projectTuple(t, projectRoot, 0, 1), "advance"); err != nil {
		t.Fatal(err)
	}
	if out, err := ops.ProjectToAncestor(projectActor, projectSubject, projectRoot, &old.Cut); !errors.Is(err, contract.ErrReadCutExpired) || out != (contract.Projection{}) {
		t.Fatalf("stale cut: %#v %v", out, err)
	}
	ops.authorizer = projectAuthorize(func(values.PrincipalRef, ...values.Requirement) bool { return false })
	if _, err := ops.ProjectToAncestor(projectActor, projectSubject, projectRoot, &old.Cut); !errors.Is(err, contract.ErrDenied) {
		t.Fatalf("cut disclosed before authorization: %v", err)
	}
}
