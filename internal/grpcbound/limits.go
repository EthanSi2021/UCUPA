// Package grpcbound bounds connections and message admission, not owner work.
package grpcbound

import (
	"context"
	"net"
	"sync"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/stats"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/tap"
)

type Listener struct {
	net.Listener
	slots    chan struct{}
	lifetime time.Duration
	mu       sync.Mutex
	closed   bool
	conns    map[*limitedConn]struct{}
}

func (l *Listener) Accept() (net.Conn, error) {
	for {
		c, err := l.Listener.Accept()
		if err != nil {
			return nil, err
		}
		select {
		case l.slots <- struct{}{}:
			wrapped := &limitedConn{Conn: c, expires: time.Now().Add(l.lifetime)}
			wrapped.release = func() {
				l.mu.Lock()
				delete(l.conns, wrapped)
				l.mu.Unlock()
				<-l.slots
			}
			l.mu.Lock()
			if l.closed {
				l.mu.Unlock()
				_ = wrapped.Close()
				return nil, net.ErrClosed
			}
			l.conns[wrapped] = struct{}{}
			l.mu.Unlock()
			_ = wrapped.SetDeadline(wrapped.expires)
			return wrapped, nil
		default:
			_ = c.Close()
		}
	}
}

// Close stops accepting but preserves already admitted connections while the
// server drains work. CloseConnections is the explicit final shutdown step.
func (l *Listener) Close() error {
	l.mu.Lock()
	l.closed = true
	l.mu.Unlock()
	return l.Listener.Close()
}

func (l *Listener) CloseConnections() {
	l.mu.Lock()
	connections := make([]*limitedConn, 0, len(l.conns))
	for c := range l.conns {
		connections = append(connections, c)
	}
	l.mu.Unlock()
	for _, c := range connections {
		_ = c.Close()
	}
}

type limitedConn struct {
	net.Conn
	expires time.Time
	once    sync.Once
	release func()
}

func (c *limitedConn) Close() error { err := c.Conn.Close(); c.once.Do(c.release); return err }
func (c *limitedConn) bounded(t time.Time) time.Time {
	if t.IsZero() || t.After(c.expires) {
		return c.expires
	}
	return t
}
func (c *limitedConn) SetDeadline(t time.Time) error     { return c.Conn.SetDeadline(c.bounded(t)) }
func (c *limitedConn) SetReadDeadline(t time.Time) error { return c.Conn.SetReadDeadline(c.bounded(t)) }
func (c *limitedConn) SetWriteDeadline(t time.Time) error {
	return c.Conn.SetWriteDeadline(c.bounded(t))
}

type cancelKey struct{}
type Stats struct{}

// Install the deadline before grpc-go builds its receive-buffer context.
// TagRPC is too late for incomplete message reads. The pinned runtime and raw
// HTTP/2 test verify this tap contract; no owner, authorization, or I/O runs here.
func Tap(deadline time.Duration) func(context.Context, *tap.Info) (context.Context, error) {
	return func(ctx context.Context, info *tap.Info) (context.Context, error) {
		content := info.Header.Get("content-type")
		if len(content) != 1 || (content[0] != "application/grpc" && content[0] != "application/grpc+proto") {
			return ctx, status.Error(codes.InvalidArgument, "protobuf gRPC required")
		}
		ctx, cancel := context.WithTimeout(ctx, deadline)
		return context.WithValue(ctx, cancelKey{}, cancel), nil
	}
}
func (Stats) TagRPC(ctx context.Context, _ *stats.RPCTagInfo) context.Context { return ctx }
func (h Stats) HandleRPC(ctx context.Context, event stats.RPCStats) {
	cancel, _ := ctx.Value(cancelKey{}).(context.CancelFunc)
	switch e := event.(type) {
	case *stats.End:
		if cancel != nil {
			cancel()
		}
	case *stats.InHeader:
		if e.Compression != "" && e.Compression != "identity" && cancel != nil {
			cancel()
		}
	}
}
func (h Stats) TagConn(ctx context.Context, _ *stats.ConnTagInfo) context.Context { return ctx }
func (h Stats) HandleConn(context.Context, stats.ConnStats)                       {}

func NewListener(l net.Listener, maximum int, lifetime time.Duration) *Listener {
	return &Listener{Listener: l, slots: make(chan struct{}, maximum), lifetime: lifetime, conns: make(map[*limitedConn]struct{})}
}
