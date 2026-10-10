package observation_test

import (
	"context"
	"errors"
	"reflect"
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
)

const (
	root   values.ElementID = "51000000-0000-4000-8000-000000000001"
	entity values.ElementID = "51000000-0000-4000-8000-000000000002"
)

type policy struct {
	decision values.AuthorizationDecision
	calls    int
}

func (*policy) AuthorizePrincipal(values.PrincipalRef, ...values.Requirement) values.AuthorizationDecision {
	return values.AuthorizationValid
}
func (*policy) AuthorizeView(context.Context, values.CallerContext, ...values.Requirement) (values.AuthorizationDecision, values.AuthorizationView) {
	return values.AuthorizationValid, values.AuthorizationView{1}
}
func (p *policy) ValidateView(context.Context, values.CallerContext, values.AuthorizationView, ...values.Requirement) values.AuthorizationDecision {
	p.calls++
	return p.decision
}
func (*policy) CaptureDenialView(context.Context, values.CallerContext, values.Requirement) (values.AuthorizationDecision, values.AuthorizationDenialView) {
	return values.AuthorizationValid, values.AuthorizationDenialView{}
}
func (*policy) ValidateDenialView(_ context.Context, _ values.CallerContext, view values.AuthorizationDenialView, _ values.Requirement) values.AuthorizationDecision {
	if view == (values.AuthorizationDenialView{}) {
		return values.AuthorizationDenied
	}
	return values.AuthorizationChanged
}

type fixture struct {
	owner     *commit.Owner
	resources *registry.Registry
	elements  *element.Operations
	observe   *observation.Operations
	admission *access.Admission
	auth      *policy
	caller    values.CallerContext
}

func newFixture(t *testing.T) fixture {
	t.Helper()
	owner, err := commit.New("51000000-0000-4000-8000-000000000003", [32]byte{51}, contract.RealmInitialization{Realm: "fixture-realm", NextHop: "local:fixture"})
	if err != nil {
		t.Fatal(err)
	}
	auth := &policy{decision: values.AuthorizationValid}
	elements := element.New(owner, auth)
	caller := values.CallerContext{Subject: values.PrincipalRef{Realm: "observe", Principal: "writer"}}
	if _, err := elements.CreateRoot(caller.Subject, root, "root"); err != nil {
		t.Fatal(err)
	}
	if _, err := elements.CreateEntity(caller.Subject, entity, tuple(t, 0), "entity"); err != nil {
		t.Fatal(err)
	}
	limits := values.PublicLimits{ContextTTL: time.Minute, ObservationTTL: time.Minute, Contexts: 16, PerCaller: 16, Groups: 4, Bytes: 4 << 20, ConcurrentReads: 1}
	resources := registry.New(owner.AuthorityRef(), limits, owner, nil)
	owner.Lock()
	err = owner.SetPublicationSinkLocked(resources)
	owner.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	admission := access.New(auth, owner, limits)
	f := fixture{owner: owner, resources: resources, elements: elements, observe: observation.New(owner, admission, resources), admission: admission, auth: auth, caller: caller}
	t.Cleanup(func() {
		owner.Lock()
		owner.BeginCloseLocked()
		resources.CloseLocked()
		owner.Unlock()
		resources.DrainRetired()
		_ = owner.CloseStore()
	})
	return f
}

func tuple(t *testing.T, x float64) contract.SpatialTuple {
	t.Helper()
	rotation, err := contract.NewRotation(1, 0, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	pose, err := contract.NewPose(contract.Vec3{X: x}, rotation)
	if err != nil {
		t.Fatal(err)
	}
	sphere, err := contract.NewBoundedSphere(1)
	if err != nil {
		t.Fatal(err)
	}
	return contract.SpatialTuple{Parent: root, LocalPose: pose, Range: sphere}
}

func (f fixture) prepare(t *testing.T, source bool) (registry.PreparationUse, registry.ReadUse) {
	t.Helper()
	f.owner.Lock()
	defer f.owner.Unlock()
	req := access.ClosureRequirements(f.owner.AuthorityRef(), root, contract.PermissionObserveElement)
	cut := f.owner.CurrentCutLocked()
	if source {
		reservation, err := f.resources.ReserveLocked(4096)
		if err != nil {
			t.Fatal(err)
		}
		read, err := f.resources.IssueSourceLocked(reservation, f.caller, root, cut, values.AuthorizationView{1}, req, registry.SourceConfig{Source: f.owner.AuthorityRef(), Mode: contract.QueryCurrentVerified, Request: federationcontract.FederationReadRequest{Boundary: root, Intent: "read", PrepareObserve: true, Purpose: [32]byte{1}}})
		if err != nil {
			t.Fatal(err)
		}
		prep, err := f.resources.PrepareSourceLocked(read.Ref, req, values.AuthorizationView{1}, cut, true)
		if err != nil {
			t.Fatal(err)
		}
		return prep, read
	}
	space, err := f.resources.IssueSpaceLocked(f.caller, root)
	if err != nil {
		t.Fatal(err)
	}
	prep, err := f.resources.PrepareLocalLocked(space.Ref, req, values.AuthorizationView{1}, cut, entity, true)
	if err != nil {
		t.Fatal(err)
	}
	return prep, registry.ReadUse{}
}

func (f fixture) move(t *testing.T) {
	t.Helper()
	old, err := f.elements.ReadElement(f.caller.Subject, entity, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.elements.UpdateEntity(f.caller.Subject, entity, tuple(t, 1), old.Record.Entity.Revision, "move"); err != nil {
		t.Fatal(err)
	}
}

func (f fixture) begin(t *testing.T, prep registry.PreparationUse) map[string]any {
	t.Helper()
	out, err := f.observe.Run(context.Background(), f.caller, "begin-observe", map[string]any{"op": "begin-observe", "preparation": accessvalue.TokenJSON(prep.Token)})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestObservationLocalAndSourceSharePublishedQueue(t *testing.T) {
	for _, source := range []bool{false, true} {
		t.Run(map[bool]string{false: "local", true: "source"}[source], func(t *testing.T) {
			f := newFixture(t)
			prep, read := f.prepare(t, source)
			if source {
				if _, err := f.observe.Run(context.Background(), f.caller, "begin-observe", map[string]any{"op": "begin-observe", "preparation": accessvalue.TokenJSON(prep.Token)}); !errors.Is(err, values.PublicInvalidContext) {
					t.Fatal("unbound exact source observed", err)
				}
				f.owner.Lock()
				err := f.resources.BindSourceObservationLocked(read.Ref, map[values.ElementID]bool{root: true, entity: true}, entity, true, f.owner.CurrentCutLocked(), 4096)
				f.owner.Unlock()
				if err != nil {
					t.Fatal(err)
				}
			}
			begin := f.begin(t, prep)
			f.move(t)
			request := map[string]any{"op": "poll", "cursor": begin["cursor"]}
			permit, err := f.admission.Recheck(context.Background(), f.caller, values.AuthorizationView{1}, access.ClosureRequirements(f.owner.AuthorityRef(), root, contract.PermissionObserveElement), true)
			if err != nil {
				t.Fatal(err)
			}
			f.owner.Lock()
			page, err := f.observe.RunLocked(context.Background(), f.caller, "poll", request, permit)
			f.owner.Unlock()
			if err != nil || len(page["groups"].([]any)) != 1 {
				t.Fatal("actual commit not published", page, err)
			}
			repeat, err := f.observe.Run(context.Background(), f.caller, "poll", request)
			if err != nil || !reflect.DeepEqual(page, repeat) {
				t.Fatal("retry changed", repeat, err)
			}
			group := page["groups"].([]any)[0].(map[string]any)
			group["category"].([]any)[0] = "client-corruption"
			repeat, err = f.observe.Run(context.Background(), f.caller, "poll", request)
			if err != nil || repeat["groups"].([]any)[0].(map[string]any)["category"].([]any)[0] != "spatial" {
				t.Fatal("returned group aliases registry", repeat, err)
			}
			ack, err := f.observe.Run(context.Background(), f.caller, "poll", map[string]any{"op": "poll", "cursor": repeat["nextCursor"], "limit": float64(1)})
			if err != nil || len(ack["groups"].([]any)) != 0 {
				t.Fatal("ack failed", ack, err)
			}
			release := map[string]any{"op": "release-observe", "preparation": accessvalue.TokenJSON(prep.Token)}
			for i := 0; i < 2; i++ {
				out, err := f.observe.Run(context.Background(), f.caller, "release-observe", release)
				if err != nil || out["released"] != true {
					t.Fatal("release not idempotent", out, err)
				}
			}
			if _, err := f.observe.Run(context.Background(), f.caller, "poll", request); !errors.Is(err, values.PublicGap) {
				t.Fatal("released cursor live", err)
			}
		})
	}
}

func TestObservationPreservesPermissionBeforeLimitAndStickyGap(t *testing.T) {
	for _, decision := range []values.AuthorizationDecision{values.AuthorizationDenied, values.AuthorizationChanged, values.AuthorizationUnavailable} {
		t.Run(string(rune('0'+decision)), func(t *testing.T) {
			f := newFixture(t)
			prep, _ := f.prepare(t, false)
			begin := f.begin(t, prep)
			f.auth.decision = decision
			request := map[string]any{"op": "poll", "cursor": begin["cursor"], "limit": "invalid"}
			want := error(values.PublicGap)
			if decision == values.AuthorizationDenied {
				want = values.PublicDenied
			}
			if decision == values.AuthorizationUnavailable {
				want = values.PublicUnavailable
			}
			if _, err := f.observe.Run(context.Background(), f.caller, "poll", request); !errors.Is(err, want) {
				t.Fatalf("limit overtook authorization: %v, want %v", err, want)
			}
			f.auth.decision = values.AuthorizationValid
			want = values.PublicGap
			if decision == values.AuthorizationUnavailable {
				want = values.PublicInvalid
			}
			if _, err := f.observe.Run(context.Background(), f.caller, "poll", request); !errors.Is(err, want) {
				t.Fatal("gap continuity changed", err, want)
			}
		})
	}
}

func TestObservationBindingShapeAndClosedPriority(t *testing.T) {
	f := newFixture(t)
	prep, _ := f.prepare(t, false)
	request := map[string]any{"op": "begin-observe", "preparation": accessvalue.TokenJSON(prep.Token)}
	other := f.caller
	other.Establishment[0] = 2
	if _, err := f.observe.Run(context.Background(), other, "begin-observe", request); !errors.Is(err, values.PublicDenied) || f.auth.calls != 0 {
		t.Fatal("caller binding bypassed", err)
	}
	bad := prep.Token
	bad.Owner = "51000000-0000-4000-8000-000000000099"
	if _, err := f.observe.Run(context.Background(), f.caller, "begin-observe", map[string]any{"op": "begin-observe", "preparation": accessvalue.TokenJSON(bad)}); !errors.Is(err, values.PublicInvalid) {
		t.Fatal(err)
	}
	f.begin(t, prep)
	f.owner.Lock()
	f.owner.BeginCloseLocked()
	f.owner.Unlock()
	if _, err := f.observe.Run(context.Background(), f.caller, "begin-observe", request); !errors.Is(err, values.PublicUnavailable) {
		t.Fatal("closed owner usable", err)
	}
	request["extra"] = true
	if _, err := f.observe.Run(context.Background(), f.caller, "begin-observe", request); !errors.Is(err, values.PublicInvalid) {
		t.Fatal("shape priority changed", err)
	}
}
