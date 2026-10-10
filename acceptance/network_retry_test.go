package acceptance_test

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/net/http2"
	"golang.org/x/net/http2/hpack"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	networkpb "ucupa/shared/bindings/go/protobuf"
	coresdk "ucupa/ucupa-core/sdk/go"
)

// Fault injection is below gRPC handlers: REFUSED_STREAM and GOAWAY with a
// lower last-stream-id explicitly say this attempt was not processed. A gRPC
// UNAVAILABLE response does not. The test drives the unmodified product SDK.
func TestNetworkTransparentRetryKeepsOriginalDeadline(t *testing.T) {
	for _, mode := range []string{"refused", "goaway", "processed-unavailable"} {
		t.Run(mode, func(t *testing.T) {
			f := newNetworkFixture(t, false)
			l, err := net.Listen("tcp", strings.TrimPrefix(string(f.config.NextHop), "https://"))
			if err != nil {
				t.Fatal(err)
			}
			listener := tls.NewListener(l, &tls.Config{Certificates: []tls.Certificate{f.config.Certificate}, MinVersion: tls.VersionTLS13, NextProtos: []string{"h2"}})
			var count atomic.Int32
			timeouts := make(chan time.Duration, 4)
			errors := make(chan error, 4)
			var mu sync.Mutex
			var current net.Conn
			var workers sync.WaitGroup
			workers.Add(1)
			go func() {
				defer workers.Done()
				for {
					conn, err := listener.Accept()
					if err != nil {
						return
					}
					mu.Lock()
					current = conn
					mu.Unlock()
					_ = conn.SetDeadline(time.Now().Add(4 * time.Second))
					if err := injectNetworkRetry(conn, mode, &count, timeouts); err != nil {
						select {
						case errors <- err:
						default:
						}
					}
					_ = conn.Close()
				}
			}()
			t.Cleanup(func() {
				_ = listener.Close()
				mu.Lock()
				if current != nil {
					_ = current.Close()
				}
				mu.Unlock()
				workers.Wait()
			})
			cfg := f.clientConfig
			cfg.RPCDeadline = 2 * time.Second
			client, err := coresdk.New(cfg)
			if err != nil {
				t.Fatal(err)
			}
			defer client.Close()
			err = client.Establish(context.Background(), f.actor.Realm, f.id, f.secret)
			if mode == "processed-unavailable" {
				if status.Code(err) != codes.Unavailable || count.Load() != 1 {
					t.Fatalf("processed response retried or fabricated: count=%d err=%v", count.Load(), err)
				}
				return
			}
			if err != nil || count.Load() != 2 {
				select {
				case detail := <-errors:
					t.Fatalf("retry: count=%d err=%v injector=%v", count.Load(), err, detail)
				default:
					t.Fatalf("retry: count=%d err=%v", count.Load(), err)
				}
			}
			first, second := <-timeouts, <-timeouts
			if first > 2*time.Second || second <= 0 || first-second < 80*time.Millisecond {
				t.Fatalf("retry reset deadline: first=%v second=%v", first, second)
			}
		})
	}
}

func injectNetworkRetry(conn net.Conn, mode string, count *atomic.Int32, timeouts chan<- time.Duration) error {
	preface := make([]byte, len(http2.ClientPreface))
	if _, err := io.ReadFull(conn, preface); err != nil {
		return err
	}
	if string(preface) != http2.ClientPreface {
		return fmt.Errorf("missing HTTP/2 preface")
	}
	f := http2.NewFramer(conn, conn)
	f.ReadMetaHeaders = hpack.NewDecoder(4096, nil)
	if err := f.WriteSettings(); err != nil {
		return err
	}
	for {
		frame, err := f.ReadFrame()
		if err != nil {
			return err
		}
		switch frame := frame.(type) {
		case *http2.SettingsFrame:
			if !frame.IsAck() {
				if err := f.WriteSettingsAck(); err != nil {
					return err
				}
			}
		case *http2.PingFrame:
			if !frame.IsAck() {
				if err := f.WritePing(true, frame.Data); err != nil {
					return err
				}
			}
		case *http2.MetaHeadersFrame:
			var timeout time.Duration
			for _, h := range frame.Fields {
				if h.Name == "grpc-timeout" {
					timeout = parseNetworkTimeout(h.Value)
				}
			}
			timeouts <- timeout
			attempt := count.Add(1)
			if attempt == 1 {
				if mode == "processed-unavailable" {
					if err := writeNetworkHeaders(f, frame.StreamID, true, []hpack.HeaderField{{Name: ":status", Value: "200"}, {Name: "content-type", Value: "application/grpc"}, {Name: "grpc-status", Value: "14"}}); err != nil {
						return err
					}
					continue
				}
				time.Sleep(120 * time.Millisecond)
				if mode == "goaway" {
					return f.WriteGoAway(0, http2.ErrCodeNo, nil)
				}
				if err := f.WriteRSTStream(frame.StreamID, http2.ErrCodeRefusedStream); err != nil {
					return err
				}
				continue
			}
			if err := writeNetworkHeaders(f, frame.StreamID, false, []hpack.HeaderField{{Name: ":status", Value: "200"}, {Name: "content-type", Value: "application/grpc"}}); err != nil {
				return err
			}
			body, err := proto.Marshal(&networkpb.EstablishSessionResponse{Session: bytes.Repeat([]byte{1}, 32), ExpiresInSeconds: 5})
			if err != nil {
				return err
			}
			data := make([]byte, 5, 5+len(body))
			binary.BigEndian.PutUint32(data[1:], uint32(len(body)))
			if err := f.WriteData(frame.StreamID, false, append(data, body...)); err != nil {
				return err
			}
			if err := writeNetworkHeaders(f, frame.StreamID, true, []hpack.HeaderField{{Name: "grpc-status", Value: "0"}}); err != nil {
				return err
			}
			// Let the client finish the connection. Closing here with unread
			// request frames can reset TCP and discard the response on Windows.
		}
	}
}

func writeNetworkHeaders(f *http2.Framer, stream uint32, end bool, fields []hpack.HeaderField) error {
	var block bytes.Buffer
	encoder := hpack.NewEncoder(&block)
	for _, h := range fields {
		if err := encoder.WriteField(h); err != nil {
			return err
		}
	}
	return f.WriteHeaders(http2.HeadersFrameParam{StreamID: stream, BlockFragment: block.Bytes(), EndHeaders: true, EndStream: end})
}

func parseNetworkTimeout(text string) time.Duration {
	if len(text) < 2 {
		return 0
	}
	n, err := strconv.ParseInt(text[:len(text)-1], 10, 64)
	if err != nil {
		return 0
	}
	unit := map[byte]time.Duration{'H': time.Hour, 'M': time.Minute, 'S': time.Second, 'm': time.Millisecond, 'u': time.Microsecond, 'n': time.Nanosecond}[text[len(text)-1]]
	return time.Duration(n) * unit
}
