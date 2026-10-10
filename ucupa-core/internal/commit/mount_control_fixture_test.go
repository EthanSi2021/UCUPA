package commit_test

import (
	"context"
	"crypto/rand"
	"sync/atomic"
	"testing"

	"ucupa/shared/values"
	corecontract "ucupa/ucupa-core/contract"
	"ucupa/ucupa-core/internal/commit"
	"ucupa/ucupa-core/internal/element"
	"ucupa/ucupa-core/internal/mount"
)

type mountControlPolicy struct{ denied atomic.Bool }

func (policy *mountControlPolicy) AuthorizePrincipal(values.PrincipalRef, ...values.Requirement) values.AuthorizationDecision {
	if policy.denied.Load() {
		return values.AuthorizationDenied
	}
	return values.AuthorizationValid
}

func (policy *mountControlPolicy) AuthorizeView(ctx context.Context, caller values.CallerContext, requirements ...values.Requirement) (values.AuthorizationDecision, values.AuthorizationView) {
	if policy.AuthorizePrincipal(caller.Subject, requirements...) != values.AuthorizationValid {
		return values.AuthorizationDenied, values.AuthorizationView{}
	}
	return values.AuthorizationValid, authorizationViewDigest(caller.Subject, requirements)
}

func (policy *mountControlPolicy) ValidateView(ctx context.Context, caller values.CallerContext, view values.AuthorizationView, requirements ...values.Requirement) values.AuthorizationDecision {
	decision, current := policy.AuthorizeView(ctx, caller, requirements...)
	if decision != values.AuthorizationValid {
		return decision
	}
	if current != view {
		return values.AuthorizationChanged
	}
	return values.AuthorizationValid
}

func (policy *mountControlPolicy) CaptureDenialView(ctx context.Context, caller values.CallerContext, requirement values.Requirement) (values.AuthorizationDecision, values.AuthorizationDenialView) {
	if policy.AuthorizePrincipal(caller.Subject, requirement) == values.AuthorizationDenied {
		return values.AuthorizationDenied, authorizationDenialDigest(caller.Subject, requirement)
	}
	return values.AuthorizationValid, values.AuthorizationDenialView{}
}

func (policy *mountControlPolicy) ValidateDenialView(ctx context.Context, caller values.CallerContext, view values.AuthorizationDenialView, requirement values.Requirement) values.AuthorizationDecision {
	if view == (values.AuthorizationDenialView{}) {
		return values.AuthorizationDenied
	}
	if _, current := policy.CaptureDenialView(ctx, caller, requirement); current != view {
		return values.AuthorizationChanged
	}
	return values.AuthorizationValid
}

type mountControlFixture struct {
	a, b                   *mountStoreOwner
	policyA, policyB       *mountControlPolicy
	root, mount            values.ElementID
	command                corecontract.MountCommand
	callers                corecontract.LocalCallers
	transportA, transportB *mountStoreTransport
}

func newMountControlFixture(t *testing.T, active bool) *mountControlFixture {
	t.Helper()
	f := &mountControlFixture{policyA: &mountControlPolicy{}, policyB: &mountControlPolicy{}, root: rangeStoreID(9200), mount: rangeStoreID(9201)}
	f.a, f.b = newMountControlOwner(t, f.policyA), newMountControlOwner(t, f.policyB)
	var err error
	if _, err = f.a.elements.CreateRoot(rangeStoreActor, f.root, "control-root"); err != nil {
		t.Fatal(err)
	}
	m, err := f.a.elements.CreateAnchor(rangeStoreActor, f.mount, rangeStoreTuple(t, f.root, 0, 10), "control-mount")
	if err != nil {
		t.Fatal(err)
	}
	f.command = corecontract.MountCommand{Kind: corecontract.MountEstablish, MountAnchorUUID: f.mount, ExpectedElementRevision: m.Record.Anchor.Revision, CarrierAuthority: f.b.owner.AuthorityRef(), CarrierNextHop: "local:b", ParentNextHop: "local:a"}
	f.callers = corecontract.LocalCallers{f.a.owner.AuthorityRef(): rangeStoreActor, f.b.owner.AuthorityRef(): rangeStoreActor}
	f.transportA = &mountStoreTransport{source: f.a, endpoints: map[values.NextHop]*mountStoreOwner{"local:b": f.b, "local:b-new": f.b}}
	f.transportB = &mountStoreTransport{source: f.b, endpoints: map[values.NextHop]*mountStoreOwner{"local:a": f.a, "local:a-new": f.a}}
	if err = f.a.mounts.ConfigureMountTransport(f.transportA); err != nil {
		t.Fatal(err)
	}
	if err = f.b.mounts.ConfigureMountTransport(f.transportB); err != nil {
		t.Fatal(err)
	}
	if active {
		f.apply(t, "control-establish", f.command)
	}
	return f
}

func (f *mountControlFixture) apply(t *testing.T, id corecontract.OperationID, command corecontract.MountCommand) corecontract.MountControlResult {
	t.Helper()
	result, err := f.a.mounts.ApplyMount(context.Background(), f.callers, id, command)
	if err != nil || result.Decision != corecontract.MountCommitted || !result.Settled {
		t.Fatalf("ApplyMount %s: %#v %v", id, result, err)
	}
	for _, owner := range []*mountStoreOwner{f.a, f.b} {
		if err := commit.ValidateCompleteStateForTest(owner.owner); err != nil {
			t.Fatalf("invalid final Core: %v", err)
		}
	}
	return result
}

func (f *mountControlFixture) update(t *testing.T, radius float64) corecontract.MountCommand {
	t.Helper()
	m, err := f.a.mounts.ReadMount(rangeStoreActor, f.mount)
	if err != nil {
		t.Fatal(err)
	}
	r, err := f.a.elements.ReadElement(rangeStoreActor, f.mount, nil)
	if err != nil {
		t.Fatal(err)
	}
	return corecontract.MountCommand{Kind: corecontract.MountUpdate, MountAnchorUUID: f.mount, ExpectedElementRevision: r.Record.Anchor.Revision, ExpectedMountRevision: m.MountRevision, Final: rangeStoreTuple(t, f.root, 0, radius)}
}

func newMountControlOwner(t *testing.T, policy *mountControlPolicy) *mountStoreOwner {
	t.Helper()
	authority, err := values.NewFactAuthorityRef()
	if err != nil {
		t.Fatal(err)
	}
	var scope [32]byte
	if _, err := rand.Read(scope[:]); err != nil {
		t.Fatal(err)
	}
	owner, err := commit.New(authority, scope, corecontract.RealmInitialization{Realm: "fixture-realm", NextHop: "local:fixture"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		owner.Lock()
		owner.BeginCloseLocked()
		owner.Unlock()
		if err := owner.CloseStore(); err != nil {
			t.Error(err)
		}
	})
	return &mountStoreOwner{owner: owner, elements: element.New(owner, policy), mounts: mount.New(owner, policy, nil)}
}

func mountControlCut(node *mountStoreOwner) corecontract.CoreReadCut {
	node.owner.RLock()
	defer node.owner.RUnlock()
	return node.owner.CurrentCutLocked()
}

func mountControlAttempt(t *testing.T, node *mountStoreOwner, key corecontract.MountAttemptKey) commit.PeerAttempt {
	t.Helper()
	node.owner.RLock()
	defer node.owner.RUnlock()
	attempt, ok := node.owner.PeerAttemptLocked(key)
	if !ok {
		t.Fatalf("missing peer attempt %#v", key)
	}
	return attempt
}
