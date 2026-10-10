// Package facetsdk is a typed client for one specified Facet, not an owner.
package facetsdk

import (
	"context"
	"crypto/x509"
	"errors"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"sync"
	"time"
	"ucupa/internal/grpcdial"
	networkpb "ucupa/shared/bindings/go/protobuf"
	"ucupa/shared/values"
	facetpb "ucupa/ucupa-facet/bindings/go/protobuf"
)

type Config struct {
	NextHop                     values.NextHop
	Expected                    values.FactAuthorityRef
	Roots                       *x509.CertPool
	SPKISHA256                  [32]byte
	RPCDeadline, ConnectTimeout time.Duration
	MaxResponseBytes            int
	MaxMetadataBytes            uint32
}
type Client struct {
	conn              *grpc.ClientConn
	config            Config
	mu                sync.RWMutex
	lifecycle         sync.Mutex
	session           string
	sessionExpires    time.Time
	sessionAudience   values.RealmID
	sessionCredential [16]byte
}

func New(c Config) (*Client, error) {
	if c.RPCDeadline <= 0 || c.MaxResponseBytes <= 0 {
		return nil, errors.New("invalid Facet client configuration")
	}
	client := &Client{config: c}
	conn, err := grpcdial.Dial(grpcdial.Config{NextHop: c.NextHop, Expected: c.Expected, Roots: c.Roots, SPKISHA256: c.SPKISHA256, ConnectTimeout: c.ConnectTimeout, MaxMetadataBytes: c.MaxMetadataBytes}, client.intercept)
	if err != nil {
		return nil, err
	}
	client.conn = conn
	return client, nil
}
func (c *Client) Facet() facetpb.FacetServiceClient { return facetpb.NewFacetServiceClient(c.conn) }
func (c *Client) Writer() facetpb.WriterControlServiceClient {
	return facetpb.NewWriterControlServiceClient(c.conn)
}
func (c *Client) Relationships() facetpb.FacetRelationshipServiceClient {
	return facetpb.NewFacetRelationshipServiceClient(c.conn)
}
func (c *Client) Acceptance() facetpb.FacetAcceptanceServiceClient {
	return facetpb.NewFacetAcceptanceServiceClient(c.conn)
}
func (c *Client) Replacement() facetpb.NamespaceReplacementServiceClient {
	return facetpb.NewNamespaceReplacementServiceClient(c.conn)
}

// Target reports the immutable specified connection for trusted assembly.
func (c *Client) Target() (values.NextHop, values.FactAuthorityRef) {
	return c.config.NextHop, c.config.Expected
}
func (c *Client) Close() error { return c.conn.Close() }
func (c *Client) Establish(ctx context.Context, audience values.RealmID, id [16]byte, secret [32]byte) error {
	c.lifecycle.Lock()
	defer c.lifecycle.Unlock()
	return c.establish(ctx, audience, id, secret)
}

// EnsureSession maintains one fixed service's authentication, not a business
// retry. New credentials or an expired lease establish a new bounded session.
func (c *Client) EnsureSession(ctx context.Context, audience values.RealmID, id [16]byte, secret [32]byte) error {
	if ctx == nil {
		return status.Error(codes.InvalidArgument, "context required")
	}
	c.lifecycle.Lock()
	defer c.lifecycle.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	c.mu.RLock()
	fresh := c.session != "" && c.sessionAudience == audience && c.sessionCredential == id && time.Now().Before(c.sessionExpires)
	c.mu.RUnlock()
	if fresh {
		return nil
	}
	return c.establish(ctx, audience, id, secret)
}

func (c *Client) establish(ctx context.Context, audience values.RealmID, id [16]byte, secret [32]byte) error {
	if ctx == nil {
		return status.Error(codes.InvalidArgument, "context required")
	}
	started := time.Now()
	in := &networkpb.EstablishSessionRequest{Audience: proto.String(string(audience)), CredentialId: id[:], Secret: secret[:]}
	out, err := networkpb.NewSessionServiceClient(c.conn).EstablishSession(ctx, in)
	clear(in.Secret)
	if err != nil {
		return err
	}
	if len(out.Session) != 32 || out.ExpiresInSeconds == 0 || out.ExpiresInSeconds > uint64((1<<63-1)/int64(time.Second)) || len(out.ProtoReflect().GetUnknown()) != 0 {
		return status.Error(codes.DataLoss, "invalid session response")
	}
	c.mu.Lock()
	c.session = values.EncodePublicBytes(out.Session)
	c.sessionExpires = started.Add(time.Duration(out.ExpiresInSeconds) * time.Second)
	c.sessionAudience, c.sessionCredential = audience, id
	c.mu.Unlock()
	return nil
}
func (c *Client) Release(ctx context.Context) error {
	c.lifecycle.Lock()
	defer c.lifecycle.Unlock()
	_, err := networkpb.NewSessionServiceClient(c.conn).ReleaseSession(ctx, &networkpb.ReleaseSessionRequest{})
	if err == nil {
		c.mu.Lock()
		c.session = ""
		c.mu.Unlock()
	}
	return err
}
func (c *Client) intercept(ctx context.Context, method string, in, out any, cc *grpc.ClientConn, invoke grpc.UnaryInvoker, opts ...grpc.CallOption) error {
	ctx, cancel := context.WithTimeout(ctx, c.config.RPCDeadline)
	defer cancel()
	if md, ok := metadata.FromOutgoingContext(ctx); ok && len(md) != 0 {
		return status.Error(codes.InvalidArgument, "caller metadata is not accepted")
	}
	md := metadata.Pairs("ucupa-target", string(c.config.Expected))
	usedSession := ""
	if method != networkpb.SessionService_EstablishSession_FullMethodName {
		c.mu.RLock()
		session := c.session
		c.mu.RUnlock()
		if session == "" {
			return status.Error(codes.Unauthenticated, "establish a session first")
		}
		usedSession = session
		md.Set("authorization", "Bearer "+session)
	}
	if len(opts) != 1 {
		return status.Error(codes.InvalidArgument, "custom RPC options are not accepted")
	}
	if _, ok := opts[0].(grpc.StaticMethodCallOption); !ok {
		return status.Error(codes.InvalidArgument, "custom RPC options are not accepted")
	}
	opts = append(opts, grpc.WaitForReady(false), grpc.MaxCallSendMsgSize(4<<20), grpc.MaxCallRecvMsgSize(c.config.MaxResponseBytes))
	if err := invoke(metadata.NewOutgoingContext(ctx, md), method, in, out, cc, opts...); err != nil {
		if status.Code(err) == codes.Unauthenticated && usedSession != "" {
			c.mu.Lock()
			if c.session == usedSession {
				c.session = ""
			}
			c.mu.Unlock()
		}
		return err
	}
	message, ok := out.(proto.Message)
	if !ok {
		return status.Error(codes.DataLoss, "invalid response")
	}
	if err := validateResponse(message.ProtoReflect(), c.config.Expected, 0); err != nil {
		return err
	}
	return validateRelationshipExchange(in, out, c.config.Expected)
}
