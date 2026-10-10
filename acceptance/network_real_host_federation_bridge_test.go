package acceptance_test

import (
	"io"
	"net"
	"sync"
	"testing"
	"time"
)

// This H4-only byte relay gives one product endpoint a reversible connection
// failure without changing the host firewall, routes or clock. TLS terminates
// only at the product; the relay cannot authenticate or alter an RPC.
type p6FederationBridge struct {
	listener net.Listener
	target   string
	mu       sync.Mutex
	cut      bool
	active   map[*p6FederationBridgePair]struct{}
}

type p6FederationBridgePair struct{ incoming, upstream net.Conn }

func p6FederationStartBridge(t *testing.T, ip net.IP, productListen string) *p6FederationBridge {
	t.Helper()
	listener, err := net.Listen("tcp", net.JoinHostPort(ip.String(), "0"))
	if err != nil {
		t.Fatal(err)
	}
	bridge := &p6FederationBridge{listener: listener, target: productListen,
		active: make(map[*p6FederationBridgePair]struct{})}
	go bridge.serve()
	return bridge
}

func (b *p6FederationBridge) nextHop() string { return "https://" + b.listener.Addr().String() }

func (b *p6FederationBridge) serve() {
	for {
		incoming, err := b.listener.Accept()
		if err != nil {
			return
		}
		go b.forward(incoming)
	}
}

func (b *p6FederationBridge) forward(incoming net.Conn) {
	b.mu.Lock()
	cut := b.cut
	b.mu.Unlock()
	if cut {
		_ = incoming.Close()
		return
	}
	upstream, err := net.DialTimeout("tcp", b.target, 3*time.Second)
	if err != nil {
		_ = incoming.Close()
		return
	}
	pair := &p6FederationBridgePair{incoming: incoming, upstream: upstream}
	b.mu.Lock()
	if b.cut {
		b.mu.Unlock()
		pair.close()
		return
	}
	b.active[pair] = struct{}{}
	b.mu.Unlock()
	done := make(chan struct{})
	go func() {
		_, _ = io.Copy(upstream, incoming)
		pair.close()
		close(done)
	}()
	_, _ = io.Copy(incoming, upstream)
	pair.close()
	<-done
	b.mu.Lock()
	delete(b.active, pair)
	b.mu.Unlock()
}

func (p *p6FederationBridgePair) close() {
	_ = p.incoming.Close()
	_ = p.upstream.Close()
}

func (b *p6FederationBridge) setCut(cut bool) {
	b.mu.Lock()
	b.cut = cut
	if cut {
		for pair := range b.active {
			pair.close()
		}
	}
	b.mu.Unlock()
}

func (b *p6FederationBridge) close() {
	_ = b.listener.Close()
	b.setCut(true)
}
