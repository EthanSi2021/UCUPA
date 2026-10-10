package commit_test

import (
	"context"
	"errors"
	"reflect"
	"sync/atomic"
	"testing"
	"time"

	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	"ucupa/ucupa-core/internal/commit"
)

func TestMountControlLateClosuresCannotOpenNewGateOrReviveRevokedMount(t *testing.T) {
	f := newMountControlFixture(t, true)
	f.apply(t, "first-range", f.update(t, 9))
	oldAttempt := mountControlAttempt(t, f.b, contract.MountAttemptKey{ParentAuthority: f.a.owner.AuthorityRef(), OperationID: "first-range"})
	f.transportA.afterPrepare = func() {
		before := commit.CaptureCompleteStateForTest(f.b.owner)
		closed, err := f.b.mounts.MountPeer().Finish(context.Background(), f.a.owner.AuthorityRef(), contract.MountFinishRequest{Evidence: oldAttempt.Closed.Evidence})
		if err != nil || closed != oldAttempt.Closed || !commit.MatchesCompleteStateForTest(f.b.owner, before) {
			t.Fatalf("old close changed new gate: %#v %v", closed, err)
		}
		prepared, err := f.b.mounts.MountPeer().Prepare(context.Background(), f.a.owner.AuthorityRef(), oldAttempt.Request)
		if err != nil || prepared != oldAttempt.Prepared || !commit.MatchesCompleteStateForTest(f.b.owner, before) {
			t.Fatalf("late prepare changed new attempt: %#v %v", prepared, err)
		}
	}
	f.apply(t, "second-range", f.update(t, 8))
	f.transportA.afterPrepare = nil
	m, err := f.a.mounts.ReadMount(rangeStoreActor, f.mount)
	if err != nil {
		t.Fatal(err)
	}
	f.apply(t, "revoke", contract.MountCommand{Kind: contract.MountRevoke, MountAnchorUUID: f.mount, ExpectedMountRevision: m.MountRevision})
	beforeA, beforeB := commit.CaptureCompleteStateForTest(f.a.owner), commit.CaptureCompleteStateForTest(f.b.owner)
	f.apply(t, "control-establish", f.command)
	if !commit.MatchesCompleteStateForTest(f.a.owner, beforeA) || !commit.MatchesCompleteStateForTest(f.b.owner, beforeB) {
		t.Fatal("successful old receipt revived revoked relation")
	}
	if mountStoreFacts(f.a).Mount.Record(f.mount).State != contract.MountDetached || !mountStoreFacts(f.b).Mount.Incoming().Detached {
		t.Fatal("revocation was not retained")
	}
}

func TestMountControlConcurrentExactApplyConvergesWithoutDoubleCommit(t *testing.T) {
	f := newMountControlFixture(t, false)
	entered, release := make(chan struct{}), make(chan struct{})
	var calls atomic.Int32
	f.transportA.afterPrepare = func() {
		if calls.Add(1) == 1 {
			close(entered)
			<-release
		}
	}
	type outcome struct {
		result contract.MountControlResult
		err    error
	}
	first := make(chan outcome, 1)
	go func() {
		r, e := f.a.mounts.ApplyMount(context.Background(), f.callers, "concurrent", f.command)
		first <- outcome{r, e}
	}()
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("Prepare did not reach barrier")
	}
	second, err := f.a.mounts.ApplyMount(context.Background(), f.callers, "concurrent", f.command)
	close(release)
	if err != nil || second.Decision != contract.MountCommitted || !second.Settled {
		t.Fatalf("concurrent second: %#v %v", second, err)
	}
	select {
	case got := <-first:
		if got.err != nil || !reflect.DeepEqual(got.result, second) {
			t.Fatalf("duplicate results diverged: %#v %v", got.result, got.err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("first Apply deadlocked")
	}
	if commit.MountControlEventCountForTest(f.a.owner) != 3 || commit.MountControlEventCountForTest(f.b.owner) != 2 {
		t.Fatalf("duplicate phase commits: A=%d B=%d", commit.MountControlEventCountForTest(f.a.owner), commit.MountControlEventCountForTest(f.b.owner))
	}
	if err := commit.ValidateCompleteStateForTest(f.a.owner); err != nil {
		t.Fatal(err)
	}
	if err := commit.ValidateCompleteStateForTest(f.b.owner); err != nil {
		t.Fatal(err)
	}
}

type malformedMountTransport struct {
	*mountStoreTransport
	prepare bool
}

func (transport malformedMountTransport) Prepare(ctx context.Context, address values.NextHop, expected values.FactAuthorityRef, request contract.MountPrepareRequest) (contract.MountPrepared, error) {
	result, err := transport.mountStoreTransport.Prepare(ctx, address, expected, request)
	if transport.prepare {
		result.Cut = contract.CoreReadCut{}
	}
	return result, err
}

func (transport malformedMountTransport) Finish(ctx context.Context, address values.NextHop, expected values.FactAuthorityRef, request contract.MountFinishRequest) (contract.MountClosed, error) {
	result, err := transport.mountStoreTransport.Finish(ctx, address, expected, request)
	if !transport.prepare {
		result.Cut = contract.CoreReadCut{}
	}
	return result, err
}

func TestMountControlMalformedPeerEvidenceStaysRecoverable(t *testing.T) {
	for _, prepare := range []bool{true, false} {
		t.Run(map[bool]string{true: "prepare", false: "close"}[prepare], func(t *testing.T) {
			f := newMountControlFixture(t, false)
			if err := f.a.mounts.ConfigureMountTransport(malformedMountTransport{mountStoreTransport: f.transportA, prepare: prepare}); err != nil {
				t.Fatal(err)
			}
			result, err := f.a.mounts.ApplyMount(context.Background(), f.callers, "malformed", f.command)
			if err != nil || result.Settled || result.BlockedBy != contract.MountBlockedOwnerMismatch {
				t.Fatalf("malformed response destroyed known result: %#v %v", result, err)
			}
			if prepare && result.Decision != contract.MountPending {
				t.Fatal("invalid Prepare committed")
			}
			if !prepare && result.Decision != contract.MountCommitted {
				t.Fatal("invalid close changed original decision")
			}
			if err = f.a.mounts.ConfigureMountTransport(f.transportA); err != nil {
				t.Fatal(err)
			}
			f.apply(t, "malformed", f.command)
		})
	}
}

func TestMountControlPeerSourceAndChangedOriginalInputAreRejected(t *testing.T) {
	f := newMountControlFixture(t, false)
	if err := f.a.mounts.ConfigureMountTransport(nil); err != nil {
		t.Fatal(err)
	}
	if _, err := f.a.mounts.ApplyMount(context.Background(), f.callers, "source", f.command); err != nil {
		t.Fatal(err)
	}
	request := mountStoreOperation(t, f.a, "source").Request
	before := commit.CaptureCompleteStateForTest(f.b.owner)
	if _, err := f.b.mounts.MountPeer().Prepare(context.Background(), f.b.owner.AuthorityRef(), request); !errors.Is(err, contract.ErrInvalid) || !commit.MatchesCompleteStateForTest(f.b.owner, before) {
		t.Fatalf("wrong source accepted: %v", err)
	}
	prepared, err := f.b.mounts.MountPeer().Prepare(context.Background(), f.a.owner.AuthorityRef(), request)
	if err != nil || !prepared.Prepared {
		t.Fatal(err)
	}
	request.Command.ParentNextHop = "local:changed"
	before = commit.CaptureCompleteStateForTest(f.b.owner)
	if _, err = f.b.mounts.MountPeer().Prepare(context.Background(), f.a.owner.AuthorityRef(), request); !errors.Is(err, contract.ErrOperationConflict) || !commit.MatchesCompleteStateForTest(f.b.owner, before) {
		t.Fatalf("mutated retry rebound initial address: %v", err)
	}
}

func TestMountCurrentSnapshotDoesNotWalkHistoricalControlOutcomes(t *testing.T) {
	f := newMountControlFixture(t, true)
	current, full := commit.CheckUnrelatedMountHistoryForTest(f.a.owner, false)
	if current != nil {
		t.Fatalf("current snapshot scanned unrelated retained history: %v", current)
	}
	if full == nil {
		t.Fatal("full validation ignored malformed history")
	}
	current, full = commit.CheckUnrelatedMountHistoryForTest(f.b.owner, true)
	if current != nil {
		t.Fatalf("current snapshot scanned unrelated peer history: %v", current)
	}
	if full == nil {
		t.Fatal("full validation ignored malformed peer history")
	}
}

func TestMountControlTransportTypedNilIsRejected(t *testing.T) {
	f := newMountControlFixture(t, false)
	var transport *mountStoreTransport
	if err := f.a.mounts.ConfigureMountTransport(transport); !errors.Is(err, contract.ErrInvalid) {
		t.Fatalf("typed nil transport: %v", err)
	}
	f.apply(t, "after-invalid-transport", f.command)
}
