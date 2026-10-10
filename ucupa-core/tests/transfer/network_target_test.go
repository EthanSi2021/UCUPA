package transfer_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"ucupa/shared/values"
	pb "ucupa/ucupa-core/bindings/go/protobuf"
	"ucupa/ucupa-core/contract"
)

func TestNetworkRecoveryReleasedSessionCannotCommitAfterPeerWait(t *testing.T) {
	f := newNetworkTargetFixture(t, false)
	a := f.approval(t, 30)
	released := make(chan struct{})
	var releaseErr error
	f.sourcePort.mu.Lock()
	f.sourcePort.after = func() {
		releaseErr = f.control.Release(context.Background())
		close(released)
	}
	f.sourcePort.mu.Unlock()
	r, err := f.control.Transitions().RecoverTransition(context.Background(), &pb.RecoverTransitionRequest{Binding: f.w, ApprovalToken: a.Token})
	if err == nil && r.GetResult() != nil {
		t.Fatalf("released source session completed transfer: %v", r)
	}
	select {
	case <-released:
	case <-time.After(time.Second):
		t.Fatal("source evidence hook was not reached")
	}
	if releaseErr != nil {
		t.Fatal(releaseErr)
	}
	state, err := f.source.c.Transfers().InspectTransition(context.Background(), f.source.caller, f.b.Operation, f.b.Command.Boundary)
	if err != nil || state.Progress != contract.TransitionFenced {
		t.Fatalf("session release lost fence or committed: %v %v", state, err)
	}
	if err := f.control.Establish(context.Background(), actor.Realm, [16]byte{62}, [32]byte{63}); err != nil {
		t.Fatal(err)
	}
	r, err = f.control.Transitions().RecoverTransition(context.Background(), &pb.RecoverTransitionRequest{Binding: f.w, ApprovalToken: a.Token})
	if err != nil || r.GetResult().GetProgress() != pb.TransitionProgress_TRANSITION_SETTLED {
		t.Fatalf("fresh session could not recover original: %v %v", r, err)
	}
}

// Both directions use actual TLS services and public SDKs. Recover is still a
// local authenticated call; this does not stand in for the process acceptance gate.
func TestNetworkTransferTargetSliceAndRoot(t *testing.T) {
	for _, root := range []bool{false, true} {
		t.Run(map[bool]string{false: "slice", true: "root"}[root], func(t *testing.T) {
			f := newNetworkTargetFixture(t, root)
			link := f.link(t, f.approval(t, 30))
			recoverPair(t, f.source, f.b, link)
			r, err := f.target.c.Elements().ReadElement(f.target.caller.Subject, f.b.Command.Boundary, nil)
			if err != nil || r.Record == nil {
				t.Fatalf("target did not publish: %v", err)
			}
			if _, err := link.AcceptImport(context.Background(), "https://127.0.0.1:1", f.b.Command.Target, f.b); err == nil {
				t.Fatal("address fallback")
			}
			bad := f.b
			bad.Operation = "different"
			if _, err := link.Install(context.Background(), f.b.Command.TargetAddress, f.b.Command.Target, bad); err == nil {
				t.Fatal("different operation reused connection")
			}
			if _, err := f.user.TransitionTarget().InstallTransition(context.Background(), &pb.InstallTransitionRequest{Binding: f.w}); status.Code(err) != codes.Unauthenticated {
				t.Fatalf("user used service endpoint: %v", err)
			}
		})
	}
}

func TestNetworkTransferTargetRechecksApprovalAfterPull(t *testing.T) {
	for _, mode := range []string{"revoked", "expired", "permission-ABA"} {
		t.Run(mode, func(t *testing.T) {
			f := newNetworkTargetFixture(t, false)
			ttl := uint64(30)
			if mode == "expired" {
				ttl = 1
			}
			a := f.approval(t, ttl)
			f.sourcePort.mu.Lock()
			f.sourcePort.after = func() {
				switch mode {
				case "revoked":
					r, err := f.user.TransitionApprovals().RevokeTransitionApproval(context.Background(), &pb.RevokeTransitionApprovalRequest{Token: a.Token})
					if err != nil || !r.GetReleased() {
						t.Errorf("revoke blocked by network wait: %v %v", r, err)
					}
				case "expired":
					time.Sleep(1100 * time.Millisecond)
				case "permission-ABA":
					f.target.grant(t, contract.PermissionTransitionImport, f.b.Command.Boundary, true)
					f.target.grant(t, contract.PermissionTransitionImport, f.b.Command.Boundary, false)
				}
			}
			f.sourcePort.mu.Unlock()
			_, err := f.source.c.Transfers().RecoverTransition(context.Background(), f.source.caller, f.b, f.link(t, a))
			if err == nil {
				t.Fatal("stale approval created durable acceptance")
			}
			got, err := f.source.c.Transfers().InspectTransition(context.Background(), f.source.caller, f.b.Operation, f.b.Command.Boundary)
			if err != nil || got.Progress != contract.TransitionFenced {
				t.Fatalf("wrong durable progress: %+v %v", got, err)
			}
			if r, err := f.target.c.Elements().ReadElement(f.target.caller.Subject, f.b.Command.Boundary, nil); err == nil && r.Record != nil {
				t.Fatal("unaccepted target became visible")
			}
			// A new explicit approval can fulfil this exact fenced intent; the
			// failed call neither silently renews the view nor undoes the fence.
			recoverPair(t, f.source, f.b, f.link(t, f.approval(t, 30)))
		})
	}
}

func TestNetworkTransferTargetDurableRetryAfterApprovalLossAndReopen(t *testing.T) {
	for _, stage := range []string{"accept", "install"} {
		t.Run(stage, func(t *testing.T) {
			f := newNetworkTargetFixture(t, false)
			a := f.approval(t, 30)
			boundary := recoveryBoundary{TargetTransport: f.link(t, a)}
			lost := func() error { return contract.ErrUnavailable }
			want := contract.TransitionFenced
			if stage == "accept" {
				boundary.afterAccept = lost
			} else {
				boundary.afterInstall = lost
				want = contract.TransitionCommitted
			}
			if _, err := f.source.c.Transfers().RecoverTransition(context.Background(), f.source.caller, f.b, boundary); !errors.Is(err, contract.ErrUnavailable) {
				t.Fatal(err)
			}
			r, err := f.source.c.Transfers().InspectTransition(context.Background(), f.source.caller, f.b.Operation, f.b.Command.Boundary)
			if err != nil || r.Progress != want {
				t.Fatalf("lost response: %+v %v", r, err)
			}
			if stage == "accept" {
				if read, err := f.target.c.Elements().ReadElement(f.target.caller.Subject, f.b.Command.Boundary, nil); err == nil && read.Record != nil {
					t.Fatal("acceptance prematurely published content")
				}
			}
			first, err := f.peer.TransitionTarget().AcceptTransition(context.Background(), &pb.AcceptTransitionRequest{Binding: f.w, ApprovalToken: a.Token})
			if err != nil || first.GetResult() == nil {
				t.Fatalf("read original acceptance: %v %v", first, err)
			}
			f.target.grant(t, contract.PermissionTransitionImport, f.b.Command.Boundary, true)
			f.reopenTarget(t) // destroys every runtime approval and user session
			link := f.link(t, a)
			if err := link.CheckImport(context.Background(), f.b.Command.TargetAddress, f.b.Command.Target, f.b); err == nil {
				t.Fatal("restart revived runtime approval")
			}
			again, err := f.peer.TransitionTarget().AcceptTransition(context.Background(), &pb.AcceptTransitionRequest{Binding: f.w, ApprovalToken: a.Token})
			if err != nil || !proto.Equal(first, again) {
				t.Fatalf("durable original acceptance changed: %v %v", again, err)
			}
			recoverPair(t, f.source, f.b, link)
			completion, err := f.peer.TransitionTarget().InstallTransition(context.Background(), &pb.InstallTransitionRequest{Binding: f.w})
			if err != nil || completion.GetResult() == nil {
				t.Fatalf("original completion: %v %v", completion, err)
			}
			f.reopenTarget(t)
			repeated, err := f.peer.TransitionTarget().InstallTransition(context.Background(), &pb.InstallTransitionRequest{Binding: f.w})
			if err != nil || !proto.Equal(completion, repeated) {
				t.Fatalf("installation repeated: %v %v", repeated, err)
			}
		})
	}
}

func TestNetworkTransferConcurrentExactRecoveries(t *testing.T) {
	f := newNetworkTargetFixture(t, false)
	link := f.link(t, f.approval(t, 30))
	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := f.source.c.Transfers().RecoverTransition(context.Background(), f.source.caller, f.b, link)
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil && !errors.Is(err, values.PublicResourceExhausted) {
			t.Errorf("concurrent recovery: %v", err)
		}
	}
	recoverPair(t, f.source, f.b, link)
}
