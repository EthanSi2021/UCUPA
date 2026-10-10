package acceptance_test

import (
	"context"
	"io"
	"net"
	"sync"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	netpb "ucupa/shared/bindings/go/protobuf"
	"ucupa/shared/values"
	corepb "ucupa/ucupa-core/bindings/go/protobuf"
	corecontract "ucupa/ucupa-core/contract"
	coresdk "ucupa/ucupa-core/sdk/go"
)

// A byte relay holds only return traffic after the SDK has authenticated.
// A separate direct connection proves commit before the held connection is
// dropped. No handler, owner, Store or production fault hook is replaced.
type responseBarrier struct {
	listener            net.Listener
	target              string
	mu                  sync.Mutex
	armed, held, closed bool
	pairs               []net.Conn
	seen, release       chan struct{}
	once                sync.Once
	wg                  sync.WaitGroup
}

func newResponseBarrier(t *testing.T, target values.NextHop) *responseBarrier {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	p := &responseBarrier{listener: l, target: addressHostPort(target), seen: make(chan struct{}), release: make(chan struct{})}
	p.wg.Add(1)
	go func() {
		defer p.wg.Done()
		for {
			client, err := l.Accept()
			if err != nil {
				return
			}
			server, err := net.DialTimeout("tcp", p.target, time.Second)
			if err != nil {
				client.Close()
				continue
			}
			p.mu.Lock()
			if p.closed {
				p.mu.Unlock()
				client.Close()
				server.Close()
				continue
			}
			p.pairs = append(p.pairs, client, server)
			p.mu.Unlock()
			p.wg.Add(2)
			go func() { defer p.wg.Done(); defer client.Close(); defer server.Close(); io.Copy(server, client) }()
			go func() {
				defer p.wg.Done()
				defer client.Close()
				defer server.Close()
				buf := make([]byte, 32<<10)
				for {
					n, err := server.Read(buf)
					if n > 0 {
						p.mu.Lock()
						hold := p.armed && !p.held
						if hold {
							p.held = true
						}
						p.mu.Unlock()
						if hold {
							close(p.seen)
							<-p.release
						}
						if _, err := client.Write(buf[:n]); err != nil {
							return
						}
					}
					if err != nil {
						return
					}
				}
			}()
		}
	}()
	t.Cleanup(func() { l.Close(); p.drop(); p.wg.Wait() })
	return p
}

func (p *responseBarrier) drop() {
	p.mu.Lock()
	p.closed = true
	for _, c := range p.pairs {
		c.Close()
	}
	p.mu.Unlock()
	p.once.Do(func() { close(p.release) })
}

func TestNetworkRealmCommittedResponseLossRecovery(t *testing.T) {
	f := newNetworkRealmContactFixture(t)
	rp := f.startRealm()
	cp := f.startCore()
	direct := f.connect(t)
	p := newResponseBarrier(t, f.coreAddress)
	cfg := f.clientConfig
	cfg.NextHop = values.NextHop("https://" + p.listener.Addr().String())
	client, err := coresdk.New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	ctx := context.Background()
	if err := client.Establish(ctx, f.actor.Realm, f.id, f.secret); err != nil {
		t.Fatal(err)
	}
	request := &corepb.CreateAnchorRequest{Handle: f.enter(t, client, corepb.Intent_CREATE_ANCHOR), Element: proto.String(networkContactChild), OperationId: []byte("realm-lost-response"), Final: networkContactTuple(10)}
	recovery := &corepb.RecoverOutcomeRequest{Reference: f.reference(), OperationId: request.OperationId, Request: &corepb.MutationCommand{Command: &corepb.MutationCommand_CreateAnchor{CreateAnchor: &corepb.CreateAnchorCommand{Element: request.Element, OperationId: request.OperationId, Final: request.Final}}}}
	p.mu.Lock()
	p.armed = true
	p.mu.Unlock()
	result := make(chan bool, 1)
	go func() {
		out, err := client.Core().CreateAnchor(ctx, request)
		result <- err != nil && out == nil
	}()
	select {
	case <-p.seen:
	case <-time.After(5 * time.Second):
		t.Fatal("response barrier not reached")
	}
	var original *corepb.RecoverOutcomeResponse
	deadline := time.Now().Add(3 * time.Second)
	for {
		original, err = direct.Core().RecoverOutcome(ctx, recovery)
		if err == nil && original.GetResult() != nil {
			break
		}
		if !time.Now().Before(deadline) {
			t.Fatalf("no proven commit before response loss: %v %v", original, err)
		}
		time.Sleep(10 * time.Millisecond)
	}
	if original.GetResult().GetStatus() != corepb.CommitStatus_COMMITTED {
		t.Fatalf("not committed: %v", original)
	}
	p.drop()
	select {
	case lost := <-result:
		if !lost {
			t.Fatal("lost connection disclosed a definitive mutation response")
		}
	case <-time.After(6 * time.Second):
		t.Fatal("lost-response request did not terminate")
	}
	cp = f.relaunchCore(t, cp, f.realmAddress)
	direct = f.connect(t)
	recovered, err := direct.Core().RecoverOutcome(ctx, recovery)
	if err != nil || !proto.Equal(original.GetResult(), recovered.GetResult()) {
		t.Fatalf("restart changed original outcome: %v %v", recovered, err)
	}
	// Current authorization controls disclosure even after a proven commit.
	rp.gracefulStop()
	if err := f.grant(corecontract.PermissionCreateAnchor, values.AnchorClosureScope(f.owner, networkContactRoot), true); err != nil {
		t.Fatal(err)
	}
	rp = f.startRealm()
	direct = f.connectReady(t)
	denied, err := direct.Core().RecoverOutcome(ctx, recovery)
	if err != nil || denied.GetFailure() != netpb.Failure_DENIED {
		t.Fatalf("revocation disclosed original outcome: %v %v", denied, err)
	}
	rp.gracefulStop()
	if err := f.grant(corecontract.PermissionCreateAnchor, values.AnchorClosureScope(f.owner, networkContactRoot), false); err != nil {
		t.Fatal(err)
	}
	rp = f.startRealm()
	direct = f.connectReady(t)
	recovered, err = direct.Core().RecoverOutcome(ctx, recovery)
	if err != nil || !proto.Equal(original.GetResult(), recovered.GetResult()) {
		t.Fatalf("regrant changed original commit: %v %v", recovered, err)
	}
	_ = rp
	_ = cp
}
