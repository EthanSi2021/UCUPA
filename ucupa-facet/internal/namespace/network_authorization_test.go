package namespace

import (
	"context"
	"errors"
	"reflect"
	"sync/atomic"
	"testing"

	"ucupa/shared/values"
	"ucupa/ucupa-facet/contract"
)

// This port models the network wait boundary, not remote protocol correctness.
// Product tests separately establish the real TLS/Realm path.
type networkBoundaryPort struct {
	*acceptanceViewAuth
	owner      *Owner
	locked     atomic.Bool
	released   atomic.Int64
	authorized func()
	validated  func()
}

func (p *networkBoundaryPort) unlocked() {
	if !p.owner.mu.TryLock() {
		p.locked.Store(true)
		return
	}
	p.owner.mu.Unlock()
}

func (p *networkBoundaryPort) AuthorizeView(ctx context.Context, address values.NextHop, realm values.RealmID, caller values.CallerContext, requirements ...values.Requirement) (values.AuthorizationDecision, values.AuthorizationView) {
	p.unlocked()
	decision, view := p.acceptanceViewAuth.AuthorizeView(ctx, address, realm, caller, requirements...)
	if p.authorized != nil {
		p.authorized()
	}
	return decision, view
}

func (p *networkBoundaryPort) ValidateView(ctx context.Context, address values.NextHop, realm values.RealmID, caller values.CallerContext, view values.AuthorizationView, requirements ...values.Requirement) values.AuthorizationDecision {
	p.unlocked()
	if p.validated != nil {
		p.validated()
	}
	return p.acceptanceViewAuth.ValidateView(ctx, address, realm, caller, view, requirements...)
}

func (p *networkBoundaryPort) ReleaseRealmEvidence(ctx context.Context, _ values.NextHop, _ values.RealmID, _ [32]byte) {
	p.unlocked()
	if ctx.Err() != nil {
		p.locked.Store(true) // cleanup must survive operation cancellation
	}
	p.released.Add(1)
}

func networkAcceptanceFixture(t *testing.T) (*publicAcceptanceFixture, *networkBoundaryPort) {
	t.Helper()
	x := newPublicAcceptanceFixture(t, false)
	p := &networkBoundaryPort{acceptanceViewAuth: x.auth, owner: x.f}
	x.f.realmPort = p
	x.f.mu.Lock()
	x.f.state.RealmContact.Current = "https://localhost:34567"
	x.f.state.RealmContact.Initial.NextHop = x.f.state.RealmContact.Current
	x.f.refreshContactLocked()
	x.f.mu.Unlock()
	return x, p
}

func TestNetworkAcceptanceWaitAndCleanupStayOutsideFactLock(t *testing.T) {
	x, p := networkAcceptanceFixture(t)
	x.accept(t, "network-accept")
	if p.locked.Load() || p.released.Load() != 1 || x.auth.checks.Load() != 1 {
		t.Fatalf("locked=%v releases=%d online rechecks=%d", p.locked.Load(), p.released.Load(), x.auth.checks.Load())
	}
	reads := x.source.reads.Load()
	x.accept(t, "network-accept")
	if x.source.reads.Load() != reads || p.released.Load() != 2 || p.locked.Load() {
		t.Fatal("original receipt changed the source or leaked its new view")
	}
	x.auth.decision.Store(uint32(values.AuthorizationDenied))
	if _, err := x.f.AcceptAttachmentControl(context.Background(), x.caller, x.ref, "local:core", "network-accept"); !errors.Is(err, values.PublicDenied) {
		t.Fatalf("unauthorized original receipt disclosure: %v", err)
	}
}

func TestNetworkAcceptanceFinalBoundaryRejectsStaleRequest(t *testing.T) {
	for _, mode := range []string{"cancel-authorize", "cancel-recheck", "contact", "aba", "closed"} {
		t.Run(mode, func(t *testing.T) {
			x, p := networkAcceptanceFixture(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			want := error(values.PublicUnavailable)
			if mode == "cancel-authorize" {
				p.authorized = cancel
			} else {
				p.validated = func() {
					switch mode {
					case "cancel-recheck":
						cancel()
					case "contact":
						x.f.mu.Lock()
						x.f.state.RealmContact.Serial++
						x.f.refreshContactLocked()
						x.f.mu.Unlock()
					case "aba":
						x.auth.version.Add(1)
						want = values.PublicInvalidContext
					case "closed":
						_ = Close(x.f)
					}
				}
			}
			before := x.f.state.clone()
			_, err := x.f.AcceptAttachmentControl(ctx, x.caller, x.ref, "local:core", "stale-request")
			if !errors.Is(err, want) || len(x.f.events) != 0 || len(x.f.state.Acceptances) != 0 || p.locked.Load() || p.released.Load() != 1 {
				t.Fatalf("err=%v want=%v events=%d locked=%v releases=%d", err, want, len(x.f.events), p.locked.Load(), p.released.Load())
			}
			if mode != "contact" && !reflect.DeepEqual(before, x.f.state) {
				t.Fatal("failed authorization changed facts")
			}
		})
	}
}

func TestNetworkPermitIsNotReusedAcrossContactGenerations(t *testing.T) {
	x, p := networkAcceptanceFixture(t)
	permit, err := x.f.authorizePublic(context.Background(), x.caller, []values.Requirement{{Permission: contract.PermissionAccept, Scope: values.FacetElementScope(x.f.authority, testElement)}})
	if err != nil {
		t.Fatal(err)
	}
	x.f.mu.Lock()
	x.f.state.RealmContact.Serial++
	x.f.refreshContactLocked()
	err = permit.guardLocked(context.Background(), false)
	x.f.mu.Unlock()
	permit.release(context.Background())
	if !errors.Is(err, values.PublicUnavailable) || p.locked.Load() || p.released.Load() != 1 {
		t.Fatal("old network decision crossed a contact generation")
	}
}
