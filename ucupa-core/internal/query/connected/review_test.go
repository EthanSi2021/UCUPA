package connected

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	"ucupa/ucupa-core/internal/query/exchange"
)

func awaitReviewBarrier(t *testing.T, entered <-chan struct{}) {
	t.Helper()
	select {
	case <-entered:
	case <-time.After(10 * time.Second):
		t.Fatal("connected operation did not reach its deterministic barrier")
	}
}

func assertReviewTerminal(t *testing.T, page contract.ConnectedQueryPage, err error, carrier values.FactAuthorityRef) {
	t.Helper()
	if err != nil || page.Status != contract.QueryPartial || page.Gap&contract.QueryUnresolvedCoverage == 0 || page.Continuation != nil {
		t.Fatalf("lost source did not remain terminal: %#v %v", page, err)
	}
	for _, match := range page.Matches {
		if match.Source.Authority == carrier {
			t.Fatalf("terminal context disclosed carrier match: %#v", match)
		}
	}
}

func TestConnectedReviewFollowupExactReadReauthorizesBeforeDisclosure(t *testing.T) {
	f := newReviewFixture(t, true, 128)
	query := reviewRootQuery(f.root)
	page, err := f.parent.connected.QueryRegionPath(context.Background(), f.callers, query, nil, 256, nil)
	if err != nil {
		t.Fatal(err)
	}
	var revoked atomic.Bool
	var readChecks atomic.Uint32
	f.carrier.policy.set(func(_ values.PrincipalRef, requirements ...values.Requirement) bool {
		for _, requirement := range requirements {
			if requirement.Permission == contract.PermissionReadElement {
				readChecks.Add(1)
				return !revoked.Load()
			}
		}
		return true
	})
	// Revoke only after the peer completed the operation's first exact check.
	f.parentTransport.setAfter(func(request exchange.RequestData, _ exchange.ReplyData, _ error) {
		if request.Action == exchange.Check && request.Grant.Kind == exchange.Resolve {
			revoked.Store(true)
		}
	})
	path := contract.Path{OriginAnchorID: f.root, Down: []values.ElementID{f.mount, f.branch, reviewID(9121)}}
	read, err := f.parent.connected.ResolvePath(context.Background(), f.callers, path, &page.Cut)
	if !errors.Is(err, contract.ErrDenied) || !reflect.DeepEqual(read, contract.PathRead{}) || readChecks.Load() < 2 {
		t.Fatalf("followup read skipped final exact grant: checks=%d result=%#v err=%v", readChecks.Load(), read, err)
	}
	f.parentTransport.setAfter(nil)
	again, err := f.parent.connected.QueryRegionPath(context.Background(), f.callers, query, &page.Cut, 256, nil)
	if err != nil || again.Status != contract.QueryComplete || len(again.Matches) != len(page.Matches) {
		t.Fatalf("exact-only denial terminated the still-authorized query: %#v %v", again, err)
	}
}

func TestConnectedReviewConcurrentOldAtCannotReviveTerminalSource(t *testing.T) {
	for _, retained := range []bool{false, true} {
		t.Run(map[bool]string{false: "current", true: "retained"}[retained], func(t *testing.T) {
			f := newReviewFixture(t, retained, 128)
			query := reviewRootQuery(f.root)
			first, err := f.parent.connected.QueryRegionPath(context.Background(), f.callers, query, nil, 1, nil)
			if err != nil || first.Continuation == nil {
				t.Fatalf("first page: %#v %v", first, err)
			}
			entered, release := make(chan struct{}), make(chan struct{})
			f.parentTransport.armCheck(entered, release)
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			type outcome struct {
				page contract.ConnectedQueryPage
				err  error
			}
			done := make(chan outcome, 1)
			go func() {
				page, err := f.parent.connected.QueryRegionPath(ctx, f.callers, query, &first.Cut, 256, nil)
				done <- outcome{page: page, err: err}
			}()
			awaitReviewBarrier(t, entered)
			f.parentTransport.setOffline(true)
			terminal, terminalErr := f.parent.connected.QueryRegionPath(ctx, f.callers, query, nil, 256, first.Continuation)
			f.parentTransport.setOffline(false)
			close(release)
			result := <-done
			assertReviewTerminal(t, terminal, terminalErr, f.carrier.owner.AuthorityRef())
			assertReviewTerminal(t, result.page, result.err, f.carrier.owner.AuthorityRef())
			old, oldErr := f.parent.connected.QueryRegionPath(ctx, f.callers, query, &first.Cut, 256, nil)
			assertReviewTerminal(t, old, oldErr, f.carrier.owner.AuthorityRef())
			fresh, freshErr := f.parent.connected.QueryRegionPath(ctx, f.callers, query, nil, 256, nil)
			if freshErr != nil || fresh.Status != contract.QueryComplete {
				t.Fatalf("fresh context failed to reconnect: %#v %v", fresh, freshErr)
			}
		})
	}
}

func TestConnectedReviewCurrentContextsKeepOnlyBoundedMetadata(t *testing.T) {
	f := newReviewFixture(t, false, 2)
	query := reviewRootQuery(f.root)
	var oldest contract.ConnectedCut
	for i := 0; i < 7; i++ {
		page, err := f.parent.connected.QueryRegionPath(context.Background(), f.callers, query, nil, 256, nil)
		if err != nil || page.Status != contract.QueryComplete || page.ReadMode != contract.QueryCurrentVerified {
			t.Fatalf("query %d: %#v %v", i, page, err)
		}
		if i == 0 {
			oldest = page.Cut
		}
		for _, owner := range []*reviewOwner{f.parent, f.carrier} {
			registry := owner.connected.registry
			if registry == nil {
				t.Fatal("connected query did not create its registry")
			}
			registry.mu.Lock()
			contexts, leases, active := len(registry.contexts), len(registry.leases), registry.active
			registry.mu.Unlock()
			if leases != 0 || active != 0 || contexts > 2 {
				t.Fatalf("unbounded connected metadata: contexts=%d leases=%d active=%d", contexts, leases, active)
			}
		}
	}
	page, err := f.parent.connected.QueryRegionPath(context.Background(), f.callers, query, &oldest, 256, nil)
	if !errors.Is(err, contract.ErrReadCutExpired) || !reflect.DeepEqual(page, contract.ConnectedQueryPage{}) {
		t.Fatalf("evicted context silently recaptured latest: %#v %v", page, err)
	}
}

func TestConnectedReviewCloseAndReleaseInvalidateInFlightRead(t *testing.T) {
	for _, retained := range []bool{false, true} {
		t.Run(map[bool]string{false: "current", true: "retained"}[retained], func(t *testing.T) {
			f := newReviewFixture(t, retained, 128)
			query := reviewRootQuery(f.root)
			first, err := f.parent.connected.QueryRegionPath(context.Background(), f.callers, query, nil, 1, nil)
			if err != nil {
				t.Fatal(err)
			}
			entered, release := make(chan struct{}), make(chan struct{})
			f.parentTransport.armCheck(entered, release)
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			done := make(chan error, 1)
			go func() {
				page, err := f.parent.connected.QueryRegionPath(ctx, f.callers, query, &first.Cut, 256, nil)
				if !reflect.DeepEqual(page, contract.ConnectedQueryPage{}) {
					err = errors.New("closed in-flight context disclosed a page")
				}
				done <- err
			}()
			awaitReviewBarrier(t, entered)
			var wait sync.WaitGroup
			wait.Add(2)
			go func() {
				defer wait.Done()
				_ = f.parent.connected.ReleaseConnectedCut(ctx, f.callers, first.Cut)
			}()
			go func() {
				defer wait.Done()
				f.parent.close()
			}()
			wait.Wait()
			close(release)
			if err := <-done; !errors.Is(err, contract.ErrReadCutExpired) {
				t.Fatalf("closed concurrent read: %v", err)
			}
			registry := f.parent.connected.registry
			registry.mu.Lock()
			active, leases, contexts := registry.active, len(registry.leases), len(registry.contexts)
			registry.mu.Unlock()
			if active != 0 || leases != 0 || contexts != 0 {
				t.Fatalf("closed owner retained work: active=%d leases=%d contexts=%d", active, leases, contexts)
			}
		})
	}
}

func TestConnectedReviewNoncontributingSourceReturnsNoRecordsWithoutContentGrant(t *testing.T) {
	for _, retained := range []bool{false, true} {
		t.Run(map[bool]string{false: "current", true: "retained"}[retained], func(t *testing.T) {
			f := newReviewFixture(t, retained, 128)
			query := reviewRootQuery(f.root)
			page, err := f.parent.connected.QueryRegionPath(context.Background(), f.callers, query, nil, 256, nil)
			if err != nil {
				t.Fatal(err)
			}
			f.carrier.policy.set(func(values.PrincipalRef, ...values.Requirement) bool { return false })
			verified := 0
			f.parentTransport.setAfter(func(request exchange.RequestData, reply exchange.ReplyData, err error) {
				if request.Action != exchange.Borrow {
					return
				}
				if err != nil || !request.VerifyOnly || !reply.Facts.Present ||
					reply.Facts.Elements.Counts().Anchors != 0 || reply.Facts.Elements.Counts().Entities != 0 ||
					!reply.Grant.VerifyOnly || len(reply.Grant.Prefix) != 0 || len(reply.Grant.Adjacency) != 0 {
					t.Fatalf("noncontributing peer disclosed content or required original query: %#v %#v %v", request, reply, err)
				}
				verified++
			})
			read, err := f.parent.connected.ResolvePath(context.Background(), f.callers, contract.Path{OriginAnchorID: f.root}, &page.Cut)
			if err != nil || read.State != contract.ExistencePresent || read.Record == nil || reviewRecordID(*read.Record) != f.root || verified == 0 {
				t.Fatalf("noncontributing source blocked local exact read: verified=%d result=%#v err=%v", verified, read, err)
			}
		})
	}
}
