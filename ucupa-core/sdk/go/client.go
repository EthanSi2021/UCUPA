// Package coresdk connects to one explicitly specified Core service. Generated
// RPC methods are reused, not mirrored as another API or owner implementation.
package coresdk

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"ucupa/internal/grpcdial"
	networkpb "ucupa/shared/bindings/go/protobuf"
	"ucupa/shared/values"
	corepb "ucupa/ucupa-core/bindings/go/protobuf"
)

// Config binds a target owner to a trusted TLS key, separately from its address.
type Config struct {
	NextHop          values.NextHop
	Expected         values.FactAuthorityRef
	Roots            *x509.CertPool
	SPKISHA256       [32]byte
	RPCDeadline      time.Duration
	ConnectTimeout   time.Duration
	MaxResponseBytes int
	MaxMetadataBytes uint32
	Actor            values.FactAuthorityRef
	ActorCertificate *tls.Certificate
}

type Client struct {
	conn                *grpc.ClientConn
	config              Config
	mu                  sync.RWMutex
	lifecycle           sync.Mutex
	session             string
	delegation          string
	federation          *corepb.FederationReadPart
	federationChallenge *pendingFederationChallenge
	sessionExpires      time.Time
	sessionAudience     values.RealmID
	sessionCredential   [16]byte
}

func New(c Config) (*Client, error) {
	if c.RPCDeadline <= 0 || c.MaxResponseBytes <= 0 {
		return nil, errors.New("invalid Core client configuration")
	}
	client := &Client{config: c}
	conn, err := grpcdial.Dial(grpcdial.Config{NextHop: c.NextHop, Expected: c.Expected, Roots: c.Roots, SPKISHA256: c.SPKISHA256, ConnectTimeout: c.ConnectTimeout, MaxMetadataBytes: c.MaxMetadataBytes, Actor: c.Actor, ActorCertificate: c.ActorCertificate}, client.intercept)
	if err != nil {
		return nil, err
	}
	client.conn = conn
	return client, nil
}

func (c *Client) Core() corepb.CoreServiceClient { return corepb.NewCoreServiceClient(c.conn) }
func (c *Client) Writer() corepb.WriterControlServiceClient {
	return corepb.NewWriterControlServiceClient(c.conn)
}

// Transitions exposes source control, not a client-side transfer coordinator.
// A failed write response must be resolved by its original operation ID.
func (c *Client) Transitions() corepb.TransitionControlServiceClient {
	return corepb.NewTransitionControlServiceClient(c.conn)
}

func (c *Client) TransitionApprovals() corepb.TransitionApprovalServiceClient {
	return corepb.NewTransitionApprovalServiceClient(c.conn)
}

func (c *Client) TransitionSource() corepb.TransitionSourceServiceClient {
	return corepb.NewTransitionSourceServiceClient(c.conn)
}

func (c *Client) TransitionTarget() corepb.TransitionTargetServiceClient {
	return corepb.NewTransitionTargetServiceClient(c.conn)
}

func (c *Client) TransitionParent() corepb.TransitionParentServiceClient {
	return corepb.NewTransitionParentServiceClient(c.conn)
}

// CarrierApprovals exposes the peer carrier approval service. Only the
// designated parent may check a retained token; create and revoke belong to
// the original approving caller's own session.
func (c *Client) CarrierApprovals() corepb.CarrierApprovalServiceClient {
	return corepb.NewCarrierApprovalServiceClient(c.conn)
}

// Target reports the immutable connection binding for trusted assembly.
func (c *Client) Target() (values.NextHop, values.FactAuthorityRef) {
	return c.config.NextHop, c.config.Expected
}
func (c *Client) AttachmentControl() corepb.AttachmentControlServiceClient {
	return corepb.NewAttachmentControlServiceClient(c.conn)
}
func (c *Client) Delegations() corepb.ReadDelegationServiceClient {
	return corepb.NewReadDelegationServiceClient(c.conn)
}
func (c *Client) FederationReads() corepb.FederationReadServiceClient {
	return corepb.NewFederationReadServiceClient(c.conn)
}
func (c *Client) MountApprovals() corepb.MountApprovalServiceClient {
	return corepb.NewMountApprovalServiceClient(c.conn)
}
func (c *Client) MountPeer() corepb.MountPeerServiceClient {
	return corepb.NewMountPeerServiceClient(c.conn)
}
func (c *Client) MountControl() corepb.MountControlServiceClient {
	return corepb.NewMountControlServiceClient(c.conn)
}
func (c *Client) MountContact() corepb.MountContactServiceClient {
	return corepb.NewMountContactServiceClient(c.conn)
}
func (c *Client) FacetControl() corepb.FacetControlServiceClient {
	return corepb.NewFacetControlServiceClient(c.conn)
}
func (c *Client) IdentityProofs() networkpb.IdentityProofServiceClient {
	return networkpb.NewIdentityProofServiceClient(c.conn)
}
func (c *Client) Close() error { return c.conn.Close() }

// UseReadDelegation installs a target-issued read capability on a fresh actor
// client. It does not authenticate a user, resolve an address or refresh a grant.
// The caller obtains it on demand through its own authenticated target client.
func (c *Client) UseReadDelegation(grant *corepb.ReadDelegation) error {
	if grant == nil || c.config.ActorCertificate == nil || grant.GetTarget() != string(c.config.Expected) || grant.GetActor() != string(c.config.Actor) || len(grant.Token) != 32 || grant.GetExpiresInSeconds() == 0 {
		return status.Error(codes.InvalidArgument, "invalid read delegation")
	}
	if err := validateResponse(grant.ProtoReflect(), 0); err != nil {
		return err
	}
	if err := validateTokenOwners(grant.ProtoReflect(), c.config.Expected); err != nil {
		return err
	}
	c.lifecycle.Lock()
	defer c.lifecycle.Unlock()
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.session != "" || c.delegation != "" {
		return status.Error(codes.FailedPrecondition, "use a fresh actor client")
	}
	c.delegation = values.EncodePublicBytes(grant.Token)
	return nil
}

// Establish authenticates directly to the selected service. It does not accept
// a self-declared Principal and does not retain the credential secret.
func (c *Client) Establish(ctx context.Context, audience values.RealmID, id [16]byte, secret [32]byte) error {
	c.lifecycle.Lock()
	defer c.lifecycle.Unlock()
	return c.establish(ctx, audience, id, secret)
}

// EnsureSession is explicit fixed-service authentication maintenance. It does
// not retry business calls or preserve handles across a new establishment.
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
		return ctx.Err()
	}
	return c.establish(ctx, audience, id, secret)
}

func (c *Client) establish(ctx context.Context, audience values.RealmID, id [16]byte, secret [32]byte) error {
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
	if existing, ok := metadata.FromOutgoingContext(ctx); ok && len(existing) != 0 {
		return status.Error(codes.InvalidArgument, "caller metadata is not accepted")
	}
	md := metadata.Pairs("ucupa-target", string(c.config.Expected))
	usedSession := ""
	c.mu.RLock()
	delegation := c.delegation
	federation := c.federation
	c.mu.RUnlock()
	if federation != nil && !federationReadMethod(method) {
		return status.Error(codes.PermissionDenied, "federation grant is part-only")
	}
	if delegation != "" {
		if method == networkpb.SessionService_EstablishSession_FullMethodName || method == networkpb.SessionService_ReleaseSession_FullMethodName {
			return status.Error(codes.PermissionDenied, "delegation is not a user session")
		}
		md.Set("authorization", "Delegation "+delegation)
	}
	serviceCheck := method == corepb.MountApprovalService_CheckMountApproval_FullMethodName || method == networkpb.IdentityProofService_CheckIdentityProof_FullMethodName || method == corepb.ReadDelegationService_BeginIdentityChallenge_FullMethodName || method == corepb.ReadDelegationService_ExchangeIdentity_FullMethodName
	serviceCheck = serviceCheck || method == corepb.TransitionApprovalService_CheckTransitionApproval_FullMethodName
	serviceCheck = serviceCheck || method == corepb.TransitionSourceService_ReadTransitionChunk_FullMethodName || method == corepb.TransitionSourceService_ReadTransitionDecision_FullMethodName
	serviceCheck = serviceCheck || method == corepb.TransitionTargetService_AcceptTransition_FullMethodName || method == corepb.TransitionTargetService_InstallTransition_FullMethodName
	serviceCheck = serviceCheck || method == corepb.TransitionParentService_ReadCarrierPlan_FullMethodName || method == corepb.TransitionParentService_ReadCarrierDecision_FullMethodName
	serviceCheck = serviceCheck || method == corepb.CarrierApprovalService_CheckCarrierApproval_FullMethodName
	serviceCheck = serviceCheck || method == corepb.TransitionSourceService_FenceCarrierSource_FullMethodName || method == corepb.TransitionSourceService_ReadCarrierSourceChunk_FullMethodName || method == corepb.TransitionSourceService_SettleCarrierSource_FullMethodName
	serviceCheck = serviceCheck || method == corepb.TransitionTargetService_CheckCarrierImport_FullMethodName || method == corepb.TransitionTargetService_AcceptCarrierTarget_FullMethodName ||
		method == corepb.TransitionTargetService_InstallCarrierTarget_FullMethodName || method == corepb.TransitionTargetService_ReadCarrierCompletion_FullMethodName
	serviceCheck = serviceCheck || method == corepb.FederationReadService_BeginFederationReadChallenge_FullMethodName || method == corepb.FederationReadService_ExchangeFederationReadIdentity_FullMethodName
	serviceCheck = serviceCheck || method == corepb.MountPeerService_PrepareMount_FullMethodName || method == corepb.MountPeerService_FinishMount_FullMethodName || method == corepb.MountPeerService_ReadMountDecision_FullMethodName
	if serviceCheck && (c.config.ActorCertificate == nil || delegation != "") {
		return status.Error(codes.Unauthenticated, "service identity required")
	}
	if err := validateTransitionSourceRequest(in, c.config.Expected, c.config.Actor); err != nil {
		return err
	}
	if err := validateTransitionRecoveryRequest(in, c.config.Expected); err != nil {
		return err
	}
	if err := validateCarrierRequest(in, c.config.Expected); err != nil {
		return err
	}
	if err := validateCarrierParentRequest(in, c.config.Expected, c.config.Actor); err != nil {
		return err
	}
	if err := validateCarrierPeerRequest(in, c.config.Expected, c.config.Actor); err != nil {
		return err
	}
	if err := validateTransitionTargetRequest(in, c.config.Expected, c.config.Actor); err != nil {
		return err
	}
	if method == corepb.MountApprovalService_CheckMountApproval_FullMethodName {
		r, ok := in.(*corepb.CheckMountApprovalRequest)
		if !ok || r.GetBinding().GetActor() != string(c.config.Actor) {
			return status.Error(codes.Unauthenticated, "wrong mount actor")
		}
	}
	if method == corepb.TransitionApprovalService_CheckTransitionApproval_FullMethodName {
		r, ok := in.(*corepb.CheckTransitionApprovalRequest)
		if !ok || r.GetBinding().GetSource() != string(c.config.Actor) {
			return status.Error(codes.Unauthenticated, "wrong transition source")
		}
	}
	if method != networkpb.SessionService_EstablishSession_FullMethodName && !serviceCheck {
		c.mu.RLock()
		session := c.session
		c.mu.RUnlock()
		if session == "" && delegation == "" {
			return status.Error(codes.Unauthenticated, "establish a session first")
		}
		if delegation == "" {
			usedSession = session
			md.Set("authorization", "Bearer "+session)
		}
	}
	ctx = metadata.NewOutgoingContext(ctx, md)
	// Options supplied at a call site cannot silently enable wait-for-ready,
	// compress a message, change the subtype, or enlarge the profile's limits.
	if len(opts) != 1 {
		return status.Error(codes.InvalidArgument, "custom RPC options are not accepted")
	}
	// The generated stub supplies StaticMethod(), even when the caller supplied none.
	if _, ok := opts[0].(grpc.StaticMethodCallOption); !ok {
		return status.Error(codes.InvalidArgument, "custom RPC options are not accepted")
	}
	opts = append(opts, grpc.WaitForReady(false), grpc.MaxCallSendMsgSize(4<<20), grpc.MaxCallRecvMsgSize(c.config.MaxResponseBytes))
	if err := invoke(ctx, method, in, out, cc, opts...); err != nil {
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
	if err := validateResponse(message.ProtoReflect(), 0); err != nil {
		return err
	}
	if request, ok := in.(*corepb.CreateMountApprovalRequest); ok {
		response, ok := out.(*corepb.CreateMountApprovalResponse)
		if !ok {
			return status.Error(codes.DataLoss, "invalid approval response")
		}
		if result := response.GetResult(); result != nil && (!proto.Equal(result.Binding, request.Binding) || result.GetExpiresInSeconds() > request.GetTtlSeconds()) {
			return status.Error(codes.DataLoss, "approval does not match request")
		}
	}
	if err := validateTokenOwners(message.ProtoReflect(), c.config.Expected); err != nil {
		return err
	}
	if err := validateFacetControlExchange(in, out, c.config.Expected); err != nil {
		return err
	}
	if err := validateTransitionExchange(in, out, c.config.Expected); err != nil {
		return err
	}
	if err := validateCarrierExchange(in, out, c.config.Expected); err != nil {
		return err
	}
	if err := validateCarrierParentExchange(in, out); err != nil {
		return err
	}
	if err := validateCarrierPeerExchange(in, out, c.config.Expected); err != nil {
		return err
	}
	if err := validateTransitionApprovalExchange(in, out, c.config.Expected); err != nil {
		return err
	}
	if err := validateTransitionSourceExchange(in, out); err != nil {
		return err
	}
	if err := validateTransitionTargetExchange(in, out); err != nil {
		return err
	}
	if err := c.validateFederationExchange(in, out); err != nil {
		return err
	}
	return validateMountExchange(in, out, c.config.Expected, c.config.Actor)
}
