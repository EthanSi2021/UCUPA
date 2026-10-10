package grpcbound

import (
	"context"
	"net"
	"sync"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/stats"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/tap"
)

func TestConnectionDeadlineCannotOutliveAdmission(t *testing.T) {
	a, b := net.Pipe()
	defer b.Close()
	expires := time.Now().Add(time.Hour)
	c := &limitedConn{Conn: a, expires: expires, release: func() {}}
	defer c.Close()
	for _, input := range []time.Time{{}, expires.Add(time.Hour), expires} {
		if got := c.bounded(input); !got.Equal(expires) {
			t.Fatalf("deadline escaped admission lifetime: %v", got)
		}
	}
	earlier := expires.Add(-time.Minute)
	if got := c.bounded(earlier); !got.Equal(earlier) {
		t.Fatal("shorter caller deadline was extended")
	}
}

func TestConnectionCloseReleasesSlotExactlyOnce(t *testing.T) {
	a, b := net.Pipe()
	defer b.Close()
	var mu sync.Mutex
	releases := 0
	c := &limitedConn{Conn: a, release: func() { mu.Lock(); releases++; mu.Unlock() }}
	var workers sync.WaitGroup
	for range 16 {
		workers.Go(func() { _ = c.Close() })
	}
	workers.Wait()
	if releases != 1 {
		t.Fatalf("released %d times", releases)
	}
}

func TestListenerDrainDoesNotKillAdmittedConnection(t *testing.T) {
	raw, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	l := NewListener(raw, 1, time.Minute)
	defer l.Close()
	defer l.CloseConnections()
	client, err := net.DialTimeout("tcp", raw.Addr().String(), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	server, err := l.Accept()
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	if err := l.Close(); err != nil {
		t.Fatal(err)
	}
	_ = client.SetDeadline(time.Now().Add(time.Second))
	_ = server.SetDeadline(time.Now().Add(time.Second))
	if _, err := client.Write([]byte{7}); err != nil {
		t.Fatal(err)
	}
	var data [1]byte
	if _, err := server.Read(data[:]); err != nil || data[0] != 7 {
		t.Fatalf("drain interrupted admitted connection: %v", err)
	}
	l.CloseConnections()
	if _, err := client.Read(data[:]); err == nil {
		t.Fatal("forced close left connection usable")
	}
	if len(l.slots) != 0 || len(l.conns) != 0 {
		t.Fatal("forced close retained slots or connection records")
	}
}

func TestMessageAdmissionDeadlineAndCancellation(t *testing.T) {
	for _, content := range []string{"application/grpc", "application/grpc+proto"} {
		parent, cancel := context.WithTimeout(context.Background(), time.Second)
		ctx, err := Tap(time.Hour)(parent, &tap.Info{Header: metadata.Pairs("content-type", content)})
		if err != nil {
			cancel()
			t.Fatal(err)
		}
		got, _ := ctx.Deadline()
		want, _ := parent.Deadline()
		if !got.Equal(want) {
			t.Fatal("admission extended caller deadline")
		}
		(Stats{}).HandleRPC(ctx, &stats.End{})
		if ctx.Err() != context.Canceled || parent.Err() != nil {
			t.Fatal("end did not release only the admitted context")
		}
		cancel()
	}
	for _, headers := range []metadata.MD{
		{}, metadata.Pairs("content-type", "application/json"),
		metadata.Pairs("content-type", "application/grpc", "content-type", "application/grpc"),
	} {
		if _, err := Tap(time.Second)(context.Background(), &tap.Info{Header: headers}); status.Code(err) != codes.InvalidArgument {
			t.Fatalf("invalid media type admitted: %v", err)
		}
	}
	ctx, err := Tap(time.Minute)(context.Background(), &tap.Info{Header: metadata.Pairs("content-type", "application/grpc")})
	if err != nil {
		t.Fatal(err)
	}
	(Stats{}).HandleRPC(ctx, &stats.InHeader{Compression: "gzip"})
	if ctx.Err() != context.Canceled {
		t.Fatal("unsupported compression retained receive context")
	}
}
