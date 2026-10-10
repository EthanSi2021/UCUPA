package acceptance_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	core "ucupa/ucupa-core/instance"
	realminstance "ucupa/ucupa-realm/instance"
)

// This wrapper changes a real Realm grant after it issued the view. The Core
// must still validate that original view at its single commit boundary.
type writerViewRace struct {
	*realminstance.Realm
	hook func()
}

func (a *writerViewRace) Check(context.Context, values.NextHop, values.RealmID) error { return nil }

func (a *writerViewRace) AuthorizePrincipal(_ values.NextHop, _ values.RealmID, actor values.PrincipalRef, rs ...values.Requirement) values.AuthorizationDecision {
	return a.Realm.AuthorizePrincipal(actor, rs...)
}

func (a *writerViewRace) AuthorizeView(_ context.Context, _ values.NextHop, _ values.RealmID, c values.CallerContext, rs ...values.Requirement) (values.AuthorizationDecision, values.AuthorizationView) {
	d, v := a.Realm.AuthorizeView(c, rs...)
	if len(rs) == 1 && rs[0].Permission == contract.PermissionHandoffWriter && a.hook != nil {
		h := a.hook
		a.hook = nil
		h()
	}
	return d, v
}

func (a *writerViewRace) ValidateView(_ context.Context, _ values.NextHop, _ values.RealmID, c values.CallerContext, view values.AuthorizationView, rs ...values.Requirement) values.AuthorizationDecision {
	return a.Realm.ValidateView(c, view, rs...)
}

func (a *writerViewRace) CaptureDenialView(_ context.Context, _ values.NextHop, _ values.RealmID, c values.CallerContext, requirement values.Requirement) (values.AuthorizationDecision, values.AuthorizationDenialView) {
	return a.Realm.CaptureDenialView(c, requirement)
}

func (a *writerViewRace) ValidateDenialView(_ context.Context, _ values.NextHop, _ values.RealmID, c values.CallerContext, view values.AuthorizationDenialView, requirement values.Requirement) values.AuthorizationDecision {
	return a.Realm.ValidateDenialView(c, view, requirement)
}

func TestWriterCorePermissionsSnapshotsAndABA(t *testing.T) {
	cfg := &values.WriterStartup{Handoff: true}
	f := newNetworkFixtureWriter(t, false, "writer-boundary", cfg)
	caller, err := f.r.TrustLocalCaller(f.actor)
	if err != nil {
		t.Fatal(err)
	}
	install, err := values.NewWriterInstallation("boundary")
	if err != nil {
		t.Fatal(err)
	}
	request := values.WriterHandoffRequest{Operation: install.Operation, ExpectedEpoch: 1, Target: install.Commitment()}
	ctx := context.Background()
	if _, err = f.c.Writer().HandoffWriter(ctx, caller, request); !errors.Is(err, values.PublicDenied) {
		t.Fatal("CRUD permissions became writer approval", err)
	}
	f.grant(t, contract.PermissionHandoffWriter, values.AuthorityScope("00000000-0000-4000-8000-000000009999"), false)
	if _, err = f.c.Writer().HandoffWriter(ctx, caller, request); !errors.Is(err, values.PublicDenied) {
		t.Fatal("other Authority became approval", err)
	}
	scope := values.AuthorityScope(f.c.AuthorityRef())
	f.grant(t, contract.PermissionInspectWriter, scope, false)
	f.grant(t, contract.PermissionHandoffWriter, scope, false)
	if err = f.c.Close(); err != nil {
		t.Fatal(err)
	}
	ttl := time.Minute
	auth := &writerViewRace{Realm: f.r}
	f.c, err = core.OpenDurable(auth, filepath.Join(f.dir, "core.db"), contract.Config{Writer: cfg, SnapshotRetention: &ttl, PublicAccessPolicy: &values.PublicAccessPolicy{ContextTTL: ttl, ObservationTTL: ttl}})
	if err != nil {
		t.Fatal(err)
	}
	for _, permission := range []values.PermissionID{contract.PermissionCreateEntity, contract.PermissionReadElement, contract.PermissionQueryRegion} {
		f.grant(t, permission, values.CoreElementScope(f.c.AuthorityRef(), networkRoot), false)
	}
	f.grant(t, contract.PermissionReadElement, values.CoreElementScope(f.c.AuthorityRef(), networkEntity), false)
	rotation, _ := contract.NewRotation(1, 0, 0, 0)
	pose, _ := contract.NewPose(contract.Vec3{}, rotation)
	sphere, _ := contract.NewBoundedSphere(1)
	tuple := contract.SpatialTuple{Parent: networkRoot, LocalPose: pose, Range: sphere}
	for index, id := range []values.ElementID{networkEntity, "00000000-0000-4000-8000-000000000003"} {
		if _, err = f.c.Elements().CreateEntity(f.actor, id, tuple, contract.OperationID([]string{"one", "two"}[index])); err != nil {
			t.Fatal(err)
		}
	}
	q := contract.RegionQuery{StartAnchorID: networkRoot, BasisAnchorID: networkRoot, Selector: contract.QueryEntities}
	page, err := f.c.Queries().QueryRegion(f.actor, q, nil, 1, nil)
	if err != nil || page.Snapshot == nil || page.Continuation == nil {
		t.Fatalf("snapshot %#v %v", page, err)
	}
	auth.hook = func() {
		f.grant(t, contract.PermissionHandoffWriter, scope, true)
		f.grant(t, contract.PermissionHandoffWriter, scope, false)
	}
	if _, err = f.c.Writer().HandoffWriter(ctx, caller, request); !errors.Is(err, values.PublicInvalidContext) {
		t.Fatal("ABA authorization was accepted", err)
	}
	if s, err := f.c.Writer().InspectWriter(ctx, caller, ""); err != nil || s.Pending || s.Epoch != 1 {
		t.Fatalf("denial occupied writer slot: %+v %v", s, err)
	}
	if _, err = f.c.Writer().HandoffWriter(ctx, caller, request); err != nil {
		t.Fatal(err)
	}
	if _, err = f.c.Elements().ReadElement(f.actor, networkRoot, nil); err == nil {
		t.Fatal("fenced writer disclosed current facts")
	}
	if p, err := f.c.Queries().QueryRegion(f.actor, q, nil, 1, page.Continuation); err == nil || len(p.Matches) != 0 {
		t.Fatalf("old continuation disclosed: %#v %v", p, err)
	}
	if r, err := f.c.Queries().ReadElementSnapshot(f.actor, networkEntity, *page.Snapshot); err == nil || r.Record != nil {
		t.Fatalf("old snapshot disclosed: %#v %v", r, err)
	}
	if _, err = f.c.Writer().InspectWriter(ctx, values.CallerContext{Subject: f.actor}, request.Operation); !errors.Is(err, values.PublicDenied) {
		t.Fatal("forged caller", err)
	}
	f.grant(t, contract.PermissionInspectWriter, scope, true)
	if _, err = f.c.Writer().InspectWriter(ctx, caller, request.Operation); !errors.Is(err, values.PublicDenied) {
		t.Fatal("closed writer bypassed current authorization", err)
	}
}
