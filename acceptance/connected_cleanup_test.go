package acceptance_test

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sync/atomic"
	"testing"

	"ucupa/shared/values"
	corecontract "ucupa/ucupa-core/contract"
	peercontract "ucupa/ucupa-core/contract/connected"
)

// This wrapper sees no private message fields. It loses the Nth successful
// public peer exchange, including any opaque resource receipt in the reply.
type connectedResponseLoss struct {
	base   peercontract.Transport
	nth    int64
	calls  atomic.Int64
	fired  atomic.Bool
	cancel context.CancelFunc
	// deliver exercises cancellation after success without losing the receipt.
	deliver bool
}

func (fault *connectedResponseLoss) ReadConnected(ctx context.Context, address values.NextHop, expected values.FactAuthorityRef, request peercontract.Request) (peercontract.Reply, error) {
	reply, err := fault.base.ReadConnected(ctx, address, expected, request)
	if err == nil && fault.calls.Add(1) == fault.nth {
		fault.fired.Store(true)
		if fault.cancel != nil {
			fault.cancel()
			if fault.deliver {
				return reply, nil
			}
			return peercontract.Reply{}, context.Canceled
		}
		return peercontract.Reply{}, corecontract.ErrUnavailable
	}
	return reply, err
}

func TestP3ConnectedCancellationAfterSuccessfulReplyReturnsResources(t *testing.T) {
	for _, retained := range []bool{false, true} {
		t.Run(fmt.Sprintf("retained=%t", retained), func(t *testing.T) {
			f := newConnectedFixture(t, retained, retained)
			f.configA.SnapshotCapacity, f.configB.SnapshotCapacity = 1, 1
			f.configA.ConcurrentReads, f.configB.ConcurrentReads = 1, 1
			f.reopenConnected()
			base, err := f.reads.TransportFor(f.b)
			if err != nil {
				t.Fatal(err)
			}
			for nth := int64(1); nth <= 3; nth++ {
				ctx, cancel := context.WithCancel(context.Background())
				fault := &connectedResponseLoss{base: base, nth: nth, cancel: cancel, deliver: true}
				if err := f.b.Connected().ConfigureConnectedTransport(fault); err != nil {
					t.Fatal(err)
				}
				page, err := f.b.Connected().QueryRegionPath(ctx, f.callers(), f.queryFromB(), nil, 1, nil)
				cancel()
				if !fault.fired.Load() {
					t.Fatalf("did not reach exchange %d: %v", nth, err)
				}
				assertConnectedQueryError(t, page, err, context.Canceled)
			}
			if err := f.b.Connected().ConfigureConnectedTransport(base); err != nil {
				t.Fatal(err)
			}
			first := f.queryB(1)
			if got := collectConnected(t, f.b, f.callers(), f.queryFromB(), first, 1); len(got) != 8 {
				t.Fatal("canceling delivered replies leaked the only read slot")
			}
		})
	}
}

func TestP3ConnectedLostResponsesReturnAllReadResources(t *testing.T) {
	for _, retained := range []bool{false, true} {
		for _, canceled := range []bool{false, true} {
			for nth := int64(1); nth <= 3; nth++ {
				t.Run(fmt.Sprintf("retained=%t/cancel=%t/exchange=%d", retained, canceled, nth), func(t *testing.T) {
					f := newConnectedFixture(t, retained, retained)
					f.configA.SnapshotCapacity, f.configB.SnapshotCapacity = 4, 4
					f.configA.ConcurrentReads, f.configB.ConcurrentReads = 4, 4
					f.reopenConnected()
					base, err := f.reads.TransportFor(f.b)
					if err != nil {
						t.Fatal(err)
					}
					// More losses than either configured quota: merely waiting for
					// Retained TTL cannot pass, and CurrentVerified has no such TTL.
					for attempt := 0; attempt < 6; attempt++ {
						ctx, cancel := context.WithCancel(context.Background())
						fault := &connectedResponseLoss{base: base, nth: nth}
						want := corecontract.ErrUnavailable
						if canceled {
							fault.cancel, want = cancel, context.Canceled
						}
						if err := f.b.Connected().ConfigureConnectedTransport(fault); err != nil {
							t.Fatal(err)
						}
						page, err := f.b.Connected().QueryRegionPath(ctx, f.callers(), f.queryFromB(), nil, 1, nil)
						cancel()
						if !fault.fired.Load() {
							t.Fatalf("attempt %d did not reach exchange %d (possible resource leak): %v", attempt, nth, err)
						}
						assertConnectedQueryError(t, page, err, want)
					}
					if err := f.b.Connected().ConfigureConnectedTransport(base); err != nil {
						t.Fatal(err)
					}
					fresh := f.queryB(256)
					if fresh.Status != corecontract.QueryComplete {
						t.Fatalf("lost responses exhausted read resources: %#v", fresh)
					}
				})
			}
		}
	}
}

type connectedIdentityAfterRead struct {
	base    peercontract.PeerPort
	wrong   values.FactAuthorityRef
	nth     int64
	calls   atomic.Int64
	changed atomic.Bool
}

func (fault *connectedIdentityAfterRead) AuthorityRef() values.FactAuthorityRef {
	if fault.changed.Load() {
		return fault.wrong
	}
	return fault.base.AuthorityRef()
}

func (fault *connectedIdentityAfterRead) ReadConnected(ctx context.Context, source values.FactAuthorityRef, request peercontract.Request) (peercontract.Reply, error) {
	reply, err := fault.base.ReadConnected(ctx, source, request)
	if err == nil && fault.calls.Add(1) == fault.nth {
		fault.changed.Store(true)
	}
	return reply, err
}

func TestP3ConnectedPostCallIdentityFailureReclaimsOpaqueResources(t *testing.T) {
	for _, retained := range []bool{false, true} {
		t.Run(fmt.Sprintf("retained=%t", retained), func(t *testing.T) {
			f := newConnectedFixture(t, retained, retained)
			f.configA.SnapshotCapacity, f.configB.SnapshotCapacity = 4, 4
			f.configA.ConcurrentReads, f.configB.ConcurrentReads = 4, 4
			f.reopenConnected()
			for attempt := 0; attempt < 6; attempt++ {
				fault := &connectedIdentityAfterRead{base: f.a.Connected().ConnectedPeer(), wrong: f.b.AuthorityRef(), nth: 2}
				if err := f.reads.BindEndpoint(mountAddressA, fault); err != nil {
					t.Fatal(err)
				}
				page, err := f.b.Connected().QueryRegionPath(context.Background(), f.callers(), f.queryFromB(), nil, 1, nil)
				if !fault.changed.Load() || !errors.Is(err, corecontract.ErrUnavailable) || !reflect.ValueOf(page).IsZero() {
					t.Fatalf("identity failure %d: fired=%t page=%#v error=%v", attempt, fault.changed.Load(), page, err)
				}
			}
			f.reconnectReads()
			if fresh := f.queryB(256); fresh.Status != corecontract.QueryComplete {
				t.Fatalf("identity failure leaked read resources: %#v", fresh)
			}
		})
	}
}
