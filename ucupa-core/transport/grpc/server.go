package coregrpc

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"reflect"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"

	"ucupa/internal/grpcbound"
	networkpb "ucupa/shared/bindings/go/protobuf"
	"ucupa/shared/values"
	corepb "ucupa/ucupa-core/bindings/go/protobuf"
	"ucupa/ucupa-core/contract/replacement"
	transitionport "ucupa/ucupa-core/contract/transition"
	federationconn "ucupa/ucupa-core/transport/grpc/federationconn"
	admission "ucupa/ucupa-core/transport/grpc/internal/admission"
	attachmenttransport "ucupa/ucupa-core/transport/grpc/internal/attachment"
	entrytransport "ucupa/ucupa-core/transport/grpc/internal/entry"
	facettransport "ucupa/ucupa-core/transport/grpc/internal/facet"
	mounttransport "ucupa/ucupa-core/transport/grpc/internal/mount"
	transitiontransport "ucupa/ucupa-core/transport/grpc/internal/transition"
	writertransport "ucupa/ucupa-core/transport/grpc/internal/writer"
	mountconn "ucupa/ucupa-core/transport/grpc/mountconn"
	"ucupa/ucupa-core/transport/grpc/transitionconn"
)

// Owner is a fact owner's public boundary, not a Store or a new implementation.
type Owner interface {
	AuthorityRef() values.FactAuthorityRef
	PublicCall(context.Context, values.CallerContext, []byte) ([]byte, error)
}

// FacetControlOwner supplies the actual relationship-control entry of this same
// fact owner. It is optional; absent support remains RPC-time Unimplemented.
type FacetControlOwner interface {
	facettransport.Owner
	AuthorityRef() values.FactAuthorityRef
}

// AttachmentControlOwner supplies the relationship read/check entry used by
// the fixed Facet service RPCs. It is independent of the Core public-envelope
// owner and must name the same configured fact owner when the service is on.
type AttachmentControlOwner interface {
	attachmenttransport.Owner
	AuthorityRef() values.FactAuthorityRef
}

// MountControl is the transport's narrow view of the mounted-space owner.
// The configured implementation must belong to Expected; transport never
// reaches through it to the owner's state or implementation package.
type MountControl interface {
	mounttransport.Owner
	admission.MountOwner
	AuthorityRef() values.FactAuthorityRef
}

// Trusted identity ports and policy values retain one representation in the
// actual admission owner; these are host configuration, not RPC forwarding.
type Authenticator = admission.Authenticator
type ReadDelegationConfig = admission.ReadDelegationConfig
type MountApprovalConfig = admission.MountApprovalConfig
type TransitionApprovalConfig = admission.TransitionApprovalConfig
type CarrierApprovalConfig = admission.CarrierApprovalConfig
type IdentityProofIssuer = admission.IdentityProofIssuer
type ExternalIdentity = admission.ExternalIdentity
type FederationSource = admission.FederationSource
type ChallengeSource = admission.ChallengeSource

// Config has no implicit time/count limits. TLS terminates in this process.
type Config struct {
	TransitionImport      admission.TransitionImportOwner
	TransitionApprovals   *TransitionApprovalConfig
	TransitionControl     transitiontransport.Owner
	TransitionSource      transitionport.SourcePeer
	TransitionParent      transitionport.CarrierParentPeer
	TransitionTarget      transitionport.TargetPeer
	TransitionConnections *transitionconn.Connections
	CarrierApprovals      *CarrierApprovalConfig
	CarrierApproval       admission.CarrierApprovalOwner
	CarrierSource         transitionport.CarrierSourcePeer
	CarrierTarget         transitionport.CarrierTargetPeer
	WriterControl         writertransport.Owner
	FacetControl          FacetControlOwner
	ReplacementPeers      replacement.FacetPeers
	AttachmentControl     AttachmentControlOwner
	MountControl          MountControl
	Federation            FederationSource
	Expected              values.FactAuthorityRef
	NextHop               values.NextHop
	Certificate           tls.Certificate
	SessionTTL            time.Duration
	RPCDeadline           time.Duration
	HandshakeTimeout      time.Duration
	ConnectionLifetime    time.Duration
	ShutdownTimeout       time.Duration
	MaxSessions           int
	MaxSessionsPerCaller  int
	MaxConnections        int
	MaxConcurrentRPCs     int
	MaxStreams            uint32
	MaxMetadataBytes      uint32
	MaxResponseBytes      int
	ReadDelegation        *ReadDelegationConfig
	IdentityProofs        IdentityProofIssuer
	ExternalIdentity      ExternalIdentity
	ChallengeSource       ChallengeSource
	FacetServices         map[values.PrincipalRef]values.FactAuthorityRef
	MountApprovals        *MountApprovalConfig
	MountConnections      *mountconn.MountConnections
	FederationConnections *federationconn.FederationConnections
}

type Server struct {
	config    Config
	grpc      *grpc.Server
	stopOnce  sync.Once
	admission *admission.Service
}

func New(owner Owner, auth Authenticator, c Config) (*Server, error) {
	if c.TransitionParent != nil && (reflect.ValueOf(c.TransitionParent).Kind() == reflect.Pointer && reflect.ValueOf(c.TransitionParent).IsNil() || c.TransitionParent.AuthorityRef() != c.Expected) {
		return nil, errors.New("transition parent must be the configured owner")
	}
	if c.TransitionTarget != nil && (reflect.ValueOf(c.TransitionTarget).Kind() == reflect.Pointer && reflect.ValueOf(c.TransitionTarget).IsNil() || c.TransitionTarget.AuthorityRef() != c.Expected) {
		return nil, errors.New("transition target must be the configured owner")
	}
	if c.TransitionTarget != nil || c.TransitionConnections != nil {
		if c.TransitionTarget == nil || !c.TransitionConnections.ConfiguredFor(c.Expected) || c.ReadDelegation == nil {
			return nil, errors.New("transition target requires fixed source connections and service trust")
		}
	}
	if c.TransitionImport != nil && (reflect.ValueOf(c.TransitionImport).Kind() == reflect.Pointer && reflect.ValueOf(c.TransitionImport).IsNil() || c.TransitionImport.AuthorityRef() != c.Expected) {
		return nil, errors.New("invalid transition import owner")
	}
	if c.TransitionControl != nil && (reflect.ValueOf(c.TransitionControl).Kind() == reflect.Pointer && reflect.ValueOf(c.TransitionControl).IsNil() || c.TransitionControl.AuthorityRef() != c.Expected) {
		return nil, errors.New("invalid transition owner")
	}
	if c.TransitionSource != nil && (reflect.ValueOf(c.TransitionSource).Kind() == reflect.Pointer && reflect.ValueOf(c.TransitionSource).IsNil() || c.TransitionSource.AuthorityRef() != c.Expected) {
		return nil, errors.New("transition source must be the configured owner")
	}
	if c.CarrierSource != nil && (reflect.ValueOf(c.CarrierSource).Kind() == reflect.Pointer && reflect.ValueOf(c.CarrierSource).IsNil() || c.CarrierSource.AuthorityRef() != c.Expected) {
		return nil, errors.New("carrier source must be the configured owner")
	}
	if c.CarrierTarget != nil && (reflect.ValueOf(c.CarrierTarget).Kind() == reflect.Pointer && reflect.ValueOf(c.CarrierTarget).IsNil() || c.CarrierTarget.AuthorityRef() != c.Expected) {
		return nil, errors.New("carrier target must be the configured owner")
	}
	if c.WriterControl != nil && (reflect.ValueOf(c.WriterControl).Kind() == reflect.Pointer && reflect.ValueOf(c.WriterControl).IsNil() || c.WriterControl.AuthorityRef() != c.Expected) {
		return nil, errors.New("invalid writer owner")
	}
	_, network := c.NextHop.NetworkTarget()
	if owner == nil || auth == nil || !c.Expected.Valid() || owner.AuthorityRef() != c.Expected || !network || len(c.Certificate.Certificate) == 0 || c.Certificate.PrivateKey == nil ||
		c.SessionTTL <= 0 || c.SessionTTL%time.Second != 0 || c.RPCDeadline <= 0 || c.HandshakeTimeout <= 0 || c.ConnectionLifetime < c.RPCDeadline || c.ShutdownTimeout <= 0 ||
		c.MaxSessions <= 0 || c.MaxSessionsPerCaller <= 0 || c.MaxSessionsPerCaller > c.MaxSessions || c.MaxConnections <= 0 || c.MaxConcurrentRPCs <= 0 || c.MaxStreams == 0 || c.MaxMetadataBytes < 256 || c.MaxResponseBytes <= 0 {
		return nil, fmt.Errorf("invalid gRPC service configuration")
	}
	if c.FacetControl != nil {
		if reflect.ValueOf(c.FacetControl).Kind() == reflect.Pointer && reflect.ValueOf(c.FacetControl).IsNil() ||
			c.FacetControl.AuthorityRef() != c.Expected {
			return nil, errors.New("invalid Facet control owner")
		}
	}
	if c.MountControl != nil {
		v := reflect.ValueOf(c.MountControl)
		if v.Kind() == reflect.Pointer && v.IsNil() || c.MountControl.AuthorityRef() != c.Expected {
			return nil, errors.New("invalid Mount control owner")
		}
	}
	if c.Federation != nil {
		v := reflect.ValueOf(c.Federation)
		if v.Kind() == reflect.Pointer && v.IsNil() || c.Federation.AuthorityRef() != c.Expected {
			return nil, errors.New("invalid Federation source owner")
		}
	}
	delegation, err := admission.CopyReadDelegationConfig(c.ReadDelegation)
	if err != nil {
		return nil, err
	}
	if delegation != nil {
		if _, ok := auth.(interface {
			ForkCaller(context.Context, values.CallerContext) (values.CallerContext, error)
		}); !ok {
			return nil, errors.New("delegation requires authenticated caller derivation")
		}
	}
	c.ReadDelegation = delegation
	if err := admission.ValidateTransitionApprovals(c.TransitionApprovals, delegation, c.TransitionImport); err != nil {
		return nil, err
	}
	if c.TransitionApprovals != nil {
		copied := *c.TransitionApprovals
		c.TransitionApprovals = &copied
	}
	if err := admission.ValidateCarrierApprovals(c.CarrierApprovals, delegation, c.CarrierApproval); err != nil {
		return nil, err
	}
	if c.CarrierApprovals != nil {
		copied := *c.CarrierApprovals
		c.CarrierApprovals = &copied
	}
	if (c.CarrierSource != nil || c.CarrierTarget != nil) && (c.CarrierApprovals == nil || c.TransitionConnections == nil || !c.TransitionConnections.ConfiguredFor(c.Expected)) {
		return nil, errors.New("carrier peers require approvals and fixed peer connections")
	}
	entryConfig := entrytransport.Config{NextHop: c.NextHop, MaxResponseBytes: c.MaxResponseBytes, FederationConnections: c.FederationConnections, IdentityProofs: c.IdentityProofs}
	if err := entrytransport.ValidateFederation(owner, c.Expected, entryConfig); err != nil {
		return nil, err
	}
	if c.MountConnections != nil {
		if !c.MountConnections.ConfiguredFor(c.Expected) {
			return nil, errors.New("invalid mount control connections")
		}
		if c.MountControl == nil {
			return nil, errors.New("authenticated mount control owner required")
		}
	}
	if err := admission.ValidateMountApprovals(c.MountApprovals, delegation, c.MountControl); err != nil {
		return nil, err
	}
	if c.MountApprovals != nil {
		copied := *c.MountApprovals
		c.MountApprovals = &copied
	}
	services := make(map[values.PrincipalRef]values.FactAuthorityRef, len(c.FacetServices))
	for principal, facet := range c.FacetServices {
		if !principal.Valid() || !facet.Valid() {
			return nil, errors.New("invalid fixed Facet service binding")
		}
		services[principal] = facet
	}
	if len(services) != 0 {
		if c.AttachmentControl == nil {
			return nil, errors.New("attachment control owner required")
		}
		value := reflect.ValueOf(c.AttachmentControl)
		if value.Kind() == reflect.Pointer && value.IsNil() || c.AttachmentControl.AuthorityRef() != c.Expected {
			return nil, errors.New("attachment control owner required")
		}
	}
	c.FacetServices = services
	if (c.IdentityProofs != nil || c.ExternalIdentity != nil) && delegation == nil {
		return nil, errors.New("identity proof checking requires explicit service peer trust")
	}
	s := &Server{config: c}
	s.admission = admission.New(owner, auth, admission.Config{
		Expected: c.Expected, NextHop: c.NextHop, SessionTTL: c.SessionTTL, RPCDeadline: c.RPCDeadline,
		MaxSessions: c.MaxSessions, MaxSessionsPerCaller: c.MaxSessionsPerCaller, MaxConcurrentRPCs: c.MaxConcurrentRPCs,
		MaxResponseBytes: c.MaxResponseBytes, ReadDelegation: c.ReadDelegation, IdentityProofs: c.IdentityProofs,
		ExternalIdentity: c.ExternalIdentity, ChallengeSource: c.ChallengeSource, MountApprovals: c.MountApprovals, MountControl: c.MountControl, Federation: c.Federation,
		TransitionApprovals: c.TransitionApprovals, TransitionImport: c.TransitionImport, TransitionTarget: c.TransitionTarget,
		CarrierApprovals: c.CarrierApprovals, CarrierApproval: c.CarrierApproval, CarrierTarget: c.CarrierTarget,
	})
	tlsConfig := &tls.Config{Certificates: []tls.Certificate{c.Certificate}, MinVersion: tls.VersionTLS13, MaxVersion: tls.VersionTLS13, NextProtos: []string{"h2"}, VerifyConnection: func(state tls.ConnectionState) error {
		if state.NegotiatedProtocol != "h2" {
			return errors.New("HTTP/2 negotiation required")
		}
		return nil
	}}
	if delegation != nil {
		tlsConfig.ClientAuth = tls.VerifyClientCertIfGiven
		tlsConfig.ClientCAs = delegation.ClientRoots
	}
	s.grpc = grpc.NewServer(grpc.Creds(credentials.NewTLS(tlsConfig)),
		grpc.ConnectionTimeout(c.HandshakeTimeout), grpc.MaxRecvMsgSize(4<<20), grpc.MaxSendMsgSize(c.MaxResponseBytes), grpc.MaxHeaderListSize(c.MaxMetadataBytes),
		grpc.MaxConcurrentStreams(c.MaxStreams), grpc.InTapHandle(grpcbound.Tap(c.RPCDeadline)), grpc.StatsHandler(grpcbound.Stats{}), grpc.UnaryInterceptor(s.admission.Intercept))
	corepb.RegisterCoreServiceServer(s.grpc, entrytransport.New(owner, s.admission, entryConfig))
	corepb.RegisterWriterControlServiceServer(s.grpc, writertransport.New(c.WriterControl, c.Expected, s.admission))
	var transitionTargets transitiontransport.Targets
	var carriers transitiontransport.Carriers
	if c.TransitionConnections != nil {
		transitionTargets = c.TransitionConnections
		carriers = c.TransitionConnections
	}
	corepb.RegisterTransitionControlServiceServer(s.grpc, transitiontransport.New(c.TransitionControl, c.Expected, s.admission, transitionTargets, carriers))
	corepb.RegisterTransitionApprovalServiceServer(s.grpc, s.admission)
	corepb.RegisterTransitionSourceServiceServer(s.grpc, transitiontransport.NewSource(c.TransitionSource, c.Expected, s.admission, c.CarrierSource, s.admission, c.TransitionConnections, c.TransitionConnections))
	corepb.RegisterTransitionParentServiceServer(s.grpc, transitiontransport.NewParent(c.TransitionParent, c.Expected, s.admission))
	corepb.RegisterTransitionTargetServiceServer(s.grpc, transitiontransport.NewTarget(c.TransitionTarget, c.Expected, s.admission, c.TransitionConnections, c.CarrierTarget, s.admission, c.TransitionConnections, c.TransitionConnections))
	corepb.RegisterCarrierApprovalServiceServer(s.grpc, s.admission)
	networkpb.RegisterSessionServiceServer(s.grpc, s.admission)
	corepb.RegisterReadDelegationServiceServer(s.grpc, s.admission)
	networkpb.RegisterIdentityProofServiceServer(s.grpc, s.admission)
	corepb.RegisterAttachmentControlServiceServer(s.grpc, attachmenttransport.New(c.AttachmentControl, c.Expected, c.FacetServices))
	corepb.RegisterMountApprovalServiceServer(s.grpc, s.admission)
	corepb.RegisterMountPeerServiceServer(s.grpc, s.admission)
	mount := mounttransport.New(c.MountControl, c.Expected, c.MountConnections)
	corepb.RegisterMountControlServiceServer(s.grpc, mount)
	corepb.RegisterMountContactServiceServer(s.grpc, mount)
	corepb.RegisterFacetControlServiceServer(s.grpc, facettransport.New(c.FacetControl, c.ReplacementPeers))
	corepb.RegisterFederationReadServiceServer(s.grpc, s.admission)
	return s, nil
}

// Serve takes ownership of listener. Connections and pre-authentication streams
// have independent finite bounds. It does not open or initialize owner Stores.
func (s *Server) Serve(l net.Listener) error {
	limited, err := s.admission.BindListener(l, s.config.MaxConnections, s.config.ConnectionLifetime)
	if err != nil {
		return err
	}
	return s.grpc.Serve(limited)
}

// Stop bounds graceful shutdown, then forces transport closure. The host closes
// its owners afterwards; no transport path rewrites or rolls back facts.
func (s *Server) Stop() {
	s.stopOnce.Do(s.stop)
}

func (s *Server) stop() {
	listener := s.admission.BeginStop()
	if listener != nil {
		_ = listener.Close()
	}
	done := make(chan struct{})
	go func() { s.admission.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(s.config.ShutdownTimeout):
	}
	// Drain admitted work ourselves. In the pinned grpc-go runtime,
	// GracefulStop may hold its mutex while waiting for an uncooperative
	// handler, preventing concurrent Stop from forcing closure.
	// Close raw connections too: unfinished TLS handshakes are not yet gRPC
	// transports and must not extend shutdown to their handshake deadline.
	if listener != nil {
		listener.CloseConnections()
	}
	s.grpc.Stop()
}
