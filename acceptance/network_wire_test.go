package acceptance_test

import (
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"

	"golang.org/x/net/http2"
	"golang.org/x/net/http2/hpack"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"ucupa/composition"
	networkpb "ucupa/shared/bindings/go/protobuf"
	"ucupa/shared/values"
	corepb "ucupa/ucupa-core/bindings/go/protobuf"
	coregrpc "ucupa/ucupa-core/transport/grpc"
)

func TestNetworkIncompleteMessageHasServerDeadline(t *testing.T) {
	f := newNetworkFixture(t, false)
	f.config.RPCDeadline = 150 * time.Millisecond
	l, err := net.Listen("tcp", strings.TrimPrefix(string(f.config.NextHop), "https://"))
	if err != nil {
		t.Fatal(err)
	}
	f.config.FacetControl = f.c.Facets()
	f.config.MountControl = f.c.Mounts()
	f.server, err = coregrpc.New(f.c.Access(), composition.RealmNetworkAuthentication{Connections: f.realmLinks, Owner: f.c.Access()}, f.config)
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = f.server.Serve(l) }()
	conn, err := tls.Dial("tcp", l.Addr().String(), &tls.Config{RootCAs: f.clientConfig.Roots, ServerName: "127.0.0.1", MinVersion: tls.VersionTLS13, MaxVersion: tls.VersionTLS13, NextProtos: []string{"h2"}})
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(3 * time.Second))
	if _, err := conn.Write([]byte(http2.ClientPreface)); err != nil {
		t.Fatal(err)
	}
	framer := http2.NewFramer(conn, conn)
	if err := framer.WriteSettings(); err != nil {
		t.Fatal(err)
	}
	var headers bytes.Buffer
	encoder := hpack.NewEncoder(&headers)
	for _, field := range []hpack.HeaderField{{Name: ":method", Value: "POST"}, {Name: ":scheme", Value: "https"}, {Name: ":authority", Value: l.Addr().String()}, {Name: ":path", Value: networkpb.SessionService_EstablishSession_FullMethodName}, {Name: "content-type", Value: "application/grpc"}, {Name: "te", Value: "trailers"}, {Name: "ucupa-target", Value: string(f.config.Expected)}} {
		if err := encoder.WriteField(field); err != nil {
			t.Fatal(err)
		}
	}
	start := time.Now()
	if err := framer.WriteHeaders(http2.HeadersFrameParam{StreamID: 1, BlockFragment: headers.Bytes(), EndHeaders: true}); err != nil {
		t.Fatal(err)
	}
	// Announce 100 bytes, supply one, omit END_STREAM and grpc-timeout.
	if err := framer.WriteData(1, false, []byte{0, 0, 0, 0, 100, 1}); err != nil {
		t.Fatal(err)
	}
	for {
		frame, err := framer.ReadFrame()
		if err != nil {
			if time.Since(start) > 2*time.Second {
				t.Fatalf("server did not bound incomplete read: %v", err)
			}
			break
		}
		if h, ok := frame.(*http2.HeadersFrame); ok && h.StreamID == 1 && h.StreamEnded() {
			break
		}
		if r, ok := frame.(*http2.RSTStreamFrame); ok && r.StreamID == 1 {
			break
		}
	}
	if time.Since(start) > 2*time.Second {
		t.Fatal("server did not bound stream lifetime")
	}
}

func TestNetworkSameOperationAcrossLocalAndProtobuf(t *testing.T) {
	f := newNetworkFixture(t, true)
	caller, err := (composition.RealmNetworkAuthentication{Connections: f.realmLinks, Owner: f.c.Access()}).Establish(context.Background(), f.actor.Realm, f.id, f.secret)
	if err != nil {
		t.Fatal(err)
	}
	call := func(input string) map[string]any {
		t.Helper()
		request, err := values.ParsePublicJSON([]byte(input), values.PublicJSONLimits())
		if err != nil {
			t.Fatal(err)
		}
		encoded, err := values.CanonicalJSONValue(request)
		if err != nil {
			t.Fatal(err)
		}
		raw, err := f.c.Access().PublicCall(context.Background(), caller, encoded)
		if err != nil {
			t.Fatal(err)
		}
		v, err := values.ParseCanonicalJSON(raw, values.PublicJSONLimits())
		if err != nil {
			t.Fatal(err)
		}
		return v.(map[string]any)
	}
	entered := call(fmt.Sprintf(`{"op":"enter","reference":{"boundary":{"core":%q,"anchor":%q},"nextHop":%q},"intent":"create-entity"}`, f.config.Expected, networkRoot, f.config.NextHop))
	handle, err := values.CanonicalJSONValue(entered["handle"])
	if err != nil {
		t.Fatal(err)
	}
	op := []byte("same-local-and-protobuf")
	local := call(fmt.Sprintf(`{"op":"create-entity","handle":%s,"element":%q,"operationId":%q,"final":{"parent":%q,"pose":{"translation":[0,0,0],"rotation":{"pivot":4,"ratios":[0,0,0]}},"range":{"kind":"bounded-sphere","radius":1}}}`, handle, networkEntity, values.EncodePublicBytes(op), networkRoot))
	wire, err := f.client.Core().CreateEntity(context.Background(), &corepb.CreateEntityRequest{Handle: f.enter(t, corepb.Intent_CREATE_ENTITY), Element: proto.String(networkEntity), OperationId: op, Final: networkTuple(1)})
	if err != nil || wire.GetResult().GetRecord() == nil {
		t.Fatalf("cross representation replay: %v %v", wire, err)
	}
	got := wire.GetResult().GetRecord()
	revision := local["record"].(map[string]any)["revision"].(map[string]any)
	if values.EncodePublicBytes(got.GetRevision().GetValue()) != revision["value"] {
		t.Fatal("representation created another revision")
	}
	changed := networkTuple(2)
	conflict, err := f.client.Core().CreateEntity(context.Background(), &corepb.CreateEntityRequest{Handle: f.enter(t, corepb.Intent_CREATE_ENTITY), Element: proto.String(networkEntity), OperationId: op, Final: changed})
	if err != nil || conflict.GetFailure() != networkpb.Failure_CONFLICT {
		t.Fatalf("changed request: %v %v", conflict, err)
	}
}

func TestNetworkBadCredentialEvidenceDoesNotEstablishSession(t *testing.T) {
	f := newNetworkFixture(t, true)
	if err := f.client.Release(context.Background()); err != nil {
		t.Fatal(err)
	}
	wrong := f.secret
	wrong[0] ^= 1
	for _, input := range []struct {
		audience values.RealmID
		secret   [32]byte
	}{{f.actor.Realm, wrong}, {"wrong-realm", f.secret}} {
		if err := f.client.Establish(context.Background(), input.audience, f.id, input.secret); status.Code(err) != codes.Unauthenticated {
			t.Fatalf("bad credential: %v", err)
		}
	}
	if _, err := f.client.Core().Enter(context.Background(), &corepb.EnterRequest{Reference: f.reference(), Intent: corepb.Intent_READ.Enum()}); status.Code(err) != codes.Unauthenticated {
		t.Fatalf("bad credentials produced session: %v", err)
	}
}
