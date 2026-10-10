// Package realmsdk is a typed client for one pinned Realm network service.
// It owns the connection, typed calls, deadlines and error shape; the Realm
// kernel stays the only identity and authorization fact owner.
package realmsdk

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"net"
	"net/url"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	"ucupa/shared/values"
	realmpb "ucupa/ucupa-realm/bindings/go/protobuf"
)

// Config declares the one pinned Realm target and the optional service
// identity used for protected calls. Nothing is discovered, followed or
// downgraded to another Realm or to a local profile.
type Config struct {
	NextHop                     values.NextHop
	Expected                    values.RealmID
	Roots                       *x509.CertPool
	SPKISHA256                  [32]byte
	Certificate                 *tls.Certificate
	RPCDeadline, ConnectTimeout time.Duration
	MaxResponseBytes            int
	MaxMetadataBytes            uint32
}

type Client struct {
	conn   *grpc.ClientConn
	config Config

	// Each completed TLS transport carries its own immutable epoch in the
	// RPC peer information. Target replies can verify only that transport,
	// never whichever handshake happens to be newest when the reply returns.
	// Protected calls register under the same lock as their verification
	// check; no replacement transport may be admitted until they finish.
	stateMu           sync.Mutex
	handshakeEpoch    uint64
	verifiedEpoch     uint64
	protectedInFlight int
}

// acceptHandshake records one fully authenticated handshake. It is refused
// while a protected call is between its preflight check and its response:
// the rebuilt connection was never target-verified, and gRPC must not make
// it ready for a queued or future credential call. A refused handshake makes
// the reconnect fail; gRPC retries it once no protected call is in flight,
// and the next successful handshake invalidates the previous verification.
func (c *Client) acceptHandshake() (uint64, error) {
	c.stateMu.Lock()
	defer c.stateMu.Unlock()
	if c.protectedInFlight > 0 {
		return 0, errors.New("Realm connection replaced during a protected call")
	}
	if c.handshakeEpoch == ^uint64(0) {
		return 0, errors.New("Realm connection identity exhausted")
	}
	c.handshakeEpoch++
	return c.handshakeEpoch, nil
}

// connectionAuthInfo is local gRPC transport evidence, not wire data or a
// server-supplied assertion. Embedding TLSInfo preserves its security level.
type connectionAuthInfo struct {
	credentials.TLSInfo
	epoch uint64
}

type connectionCredentials struct {
	credentials.TransportCredentials
	client *Client
}

func (c *connectionCredentials) Clone() credentials.TransportCredentials {
	return &connectionCredentials{TransportCredentials: c.TransportCredentials.Clone(), client: c.client}
}

func (c *connectionCredentials) ClientHandshake(ctx context.Context, authority string, raw net.Conn) (net.Conn, credentials.AuthInfo, error) {
	conn, auth, err := c.TransportCredentials.ClientHandshake(ctx, authority, raw)
	if err != nil {
		return nil, nil, err
	}
	tlsInfo, ok := auth.(credentials.TLSInfo)
	if !ok {
		_ = conn.Close()
		return nil, nil, errors.New("Realm TLS transport evidence missing")
	}
	epoch, err := c.client.acceptHandshake()
	if err != nil {
		_ = conn.Close()
		return nil, nil, err
	}
	return conn, connectionAuthInfo{TLSInfo: tlsInfo, epoch: epoch}, nil
}

func New(c Config) (*Client, error) {
	target, ok := c.NextHop.NetworkTarget()
	if !ok || c.Expected == "" || c.Roots == nil || c.SPKISHA256 == ([32]byte{}) ||
		c.RPCDeadline <= 0 || c.ConnectTimeout <= 0 || c.MaxResponseBytes <= 0 || c.MaxMetadataBytes < 256 {
		return nil, errors.New("invalid Realm client configuration")
	}
	u, _ := url.Parse(string(c.NextHop))
	tlsConfig := &tls.Config{MinVersion: tls.VersionTLS13, MaxVersion: tls.VersionTLS13, RootCAs: c.Roots.Clone(), ServerName: u.Hostname(), NextProtos: []string{"h2"}}
	if c.Certificate != nil {
		certificate := *c.Certificate
		certificate.Certificate = make([][]byte, len(c.Certificate.Certificate))
		for i, der := range c.Certificate.Certificate {
			certificate.Certificate[i] = append([]byte(nil), der...)
		}
		tlsConfig.Certificates = []tls.Certificate{certificate}
	}
	pin := c.SPKISHA256
	client := &Client{config: c}
	tlsConfig.VerifyConnection = func(state tls.ConnectionState) error {
		if state.NegotiatedProtocol != "h2" || len(state.VerifiedChains) == 0 || len(state.PeerCertificates) == 0 || sha256.Sum256(state.PeerCertificates[0].RawSubjectPublicKeyInfo) != pin {
			return errors.New("Realm target authentication failed")
		}
		return nil
	}
	conn, err := grpc.NewClient("passthrough:///"+target,
		grpc.WithTransportCredentials(&connectionCredentials{TransportCredentials: credentials.NewTLS(tlsConfig), client: client}),
		grpc.WithNoProxy(),
		grpc.WithDisableServiceConfig(),
		grpc.WithDisableRetry(),
		grpc.WithContextDialer(func(ctx context.Context, address string) (net.Conn, error) {
			if address != target {
				return nil, errors.New("unexpected next hop")
			}
			ctx, cancel := context.WithTimeout(ctx, c.ConnectTimeout)
			defer cancel()
			return (&net.Dialer{}).DialContext(ctx, "tcp", target)
		}),
		grpc.WithMaxHeaderListSize(c.MaxMetadataBytes),
		grpc.WithUnaryInterceptor(client.intercept),
	)
	if err != nil {
		return nil, err
	}
	client.conn = conn
	return client, nil
}

// Target reports the pinned Realm target for trusted assembly.
func (c *Client) Target() (values.NextHop, values.RealmID) {
	return c.config.NextHop, c.config.Expected
}

func (c *Client) TargetService() realmpb.RealmTargetServiceClient {
	return realmpb.NewRealmTargetServiceClient(c.conn)
}
func (c *Client) IdentityService() realmpb.RealmIdentityServiceClient {
	return realmpb.NewRealmIdentityServiceClient(c.conn)
}
func (c *Client) AuthorizationService() realmpb.RealmAuthorizationServiceClient {
	return realmpb.NewRealmAuthorizationServiceClient(c.conn)
}

func (c *Client) Close() error { return c.conn.Close() }

// CheckTarget confirms the actual Realm identity behind the pinned address
// before any user credential is presented. It grants nothing. A successful
// check is bound to the connection that answered it and must be repeated
// after any connection rebuild.
func (c *Client) CheckTarget(ctx context.Context) (values.RealmID, error) {
	_, err := c.TargetService().CheckTarget(ctx, &realmpb.CheckTargetRequest{})
	if err != nil {
		return "", err
	}
	return c.config.Expected, nil
}

// intercept bounds every call with the configured deadline, pins the target
// metadata and rejects caller-supplied transport controls; responses are
// validated before any caller sees them. Every method except the read-only
// target check is protected: user credentials and authorization payloads are
// only sent after a successful target verification on the current connection,
// and a replacement handshake completing while such a call is between its
// preflight and its response is refused, so it can never become the transport
// the protected request leaves on.
func (c *Client) intercept(ctx context.Context, method string, in, out any, cc *grpc.ClientConn, invoke grpc.UnaryInvoker, opts ...grpc.CallOption) error {
	if ctx == nil {
		return status.Error(codes.InvalidArgument, "context required")
	}
	ctx, cancel := context.WithTimeout(ctx, c.config.RPCDeadline)
	defer cancel()
	if md, ok := metadata.FromOutgoingContext(ctx); ok && len(md) != 0 {
		return status.Error(codes.InvalidArgument, "caller metadata is not accepted")
	}
	if len(opts) != 1 {
		return status.Error(codes.InvalidArgument, "custom RPC options are not accepted")
	}
	if _, ok := opts[0].(grpc.StaticMethodCallOption); !ok {
		return status.Error(codes.InvalidArgument, "custom RPC options are not accepted")
	}
	// A protected call registers itself and checks the verification under
	// one lock, before gRPC can select or rebuild the sending transport: a
	// replacement handshake from that moment on is refused, so the request
	// can only leave on the connection the target check verified.
	protected := method != realmpb.RealmTargetService_CheckTarget_FullMethodName
	var before uint64
	if protected {
		c.stateMu.Lock()
		if !(c.verifiedEpoch != 0 && c.verifiedEpoch == c.handshakeEpoch) {
			c.stateMu.Unlock()
			return status.Error(codes.FailedPrecondition, "Realm target not verified on the current connection")
		}
		c.protectedInFlight++
		before = c.handshakeEpoch
		c.stateMu.Unlock()
		defer func() {
			c.stateMu.Lock()
			c.protectedInFlight--
			c.stateMu.Unlock()
		}()
	} else {
		// A new verification attempt is not permission to keep using a prior
		// result if this attempt fails. An older reply cannot erase a newer
		// successful verification: registration below compares actual peers.
		c.stateMu.Lock()
		c.verifiedEpoch = 0
		c.stateMu.Unlock()
	}
	var respondingPeer peer.Peer
	opts = append(opts, grpc.WaitForReady(false), grpc.MaxCallSendMsgSize(4<<20), grpc.MaxCallRecvMsgSize(c.config.MaxResponseBytes), grpc.Peer(&respondingPeer))
	md := metadata.Pairs("ucupa-realm-target", string(c.config.Expected))
	if err := invoke(metadata.NewOutgoingContext(ctx, md), method, in, out, cc, opts...); err != nil {
		return err
	}
	message, ok := out.(proto.Message)
	if !ok {
		return status.Error(codes.DataLoss, "invalid response")
	}
	if err := validateRealmResponse(message.ProtoReflect(), 0); err != nil {
		return err
	}
	auth, ok := respondingPeer.AuthInfo.(connectionAuthInfo)
	if !ok || auth.epoch == 0 {
		return status.Error(codes.DataLoss, "Realm response transport evidence missing")
	}
	c.stateMu.Lock()
	defer c.stateMu.Unlock()
	if protected {
		if auth.epoch != before || c.handshakeEpoch != before {
			return status.Error(codes.DataLoss, "Realm connection replaced during the call")
		}
		return nil
	}
	if auth.epoch != c.handshakeEpoch {
		return status.Error(codes.Unavailable, "Realm connection replaced during target verification")
	}
	target, ok := out.(*realmpb.CheckTargetResponse)
	if !ok || target.GetRealmId() != string(c.config.Expected) {
		return status.Error(codes.Unavailable, "Realm identity mismatch")
	}
	c.verifiedEpoch = auth.epoch
	return nil
}
