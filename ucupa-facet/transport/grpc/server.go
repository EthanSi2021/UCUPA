package facetgrpc

import (
	"context"
	"crypto/tls"
	"errors"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"net"
	"sync"
	"time"
	"ucupa/internal/grpcbound"
	networkpb "ucupa/shared/bindings/go/protobuf"
	"ucupa/shared/values"
	facetpb "ucupa/ucupa-facet/bindings/go/protobuf"
	writertransport "ucupa/ucupa-facet/transport/grpc/internal/writer"
)

type Authenticator interface {
	Establish(context.Context, values.RealmID, [16]byte, [32]byte) (values.CallerContext, error)
}

// All resource and time bounds are explicit; no listening or store creation
// occurs during construction. The host owns the durable Facet and Realm.
type Config struct {
	Expected                                                                       values.FactAuthorityRef
	NextHop                                                                        values.NextHop
	Certificate                                                                    tls.Certificate
	SessionTTL, RPCDeadline, HandshakeTimeout, ConnectionLifetime, ShutdownTimeout time.Duration
	MaxSessions, MaxSessionsPerCaller, MaxConnections, MaxConcurrentRPCs           int
	MaxStreams, MaxMetadataBytes                                                   uint32
	MaxResponseBytes                                                               int
	CoreServices                                                                   map[values.PrincipalRef]values.FactAuthorityRef
}

type Server struct {
	networkpb.UnimplementedSessionServiceServer
	facetpb.UnimplementedFacetRelationshipServiceServer
	facetpb.UnimplementedFacetAcceptanceServiceServer
	facetpb.UnimplementedNamespaceReplacementServiceServer
	config   Config
	owner    Owner
	auth     Authenticator
	grpc     *grpc.Server
	mu       sync.Mutex
	sessions map[[32]byte]session
	stopped  bool
	stopOnce sync.Once
	active   chan struct{}
	running  sync.WaitGroup
	listener *grpcbound.Listener
}

func New(owner Owner, auth Authenticator, c Config) (*Server, error) {
	_, network := c.NextHop.NetworkTarget()
	if owner == nil || auth == nil || !c.Expected.Valid() || owner.AuthorityRef() != c.Expected || !network || len(c.Certificate.Certificate) == 0 || c.Certificate.PrivateKey == nil ||
		c.SessionTTL <= 0 || c.SessionTTL%time.Second != 0 || c.RPCDeadline <= 0 || c.HandshakeTimeout <= 0 || c.ConnectionLifetime < c.RPCDeadline || c.ShutdownTimeout <= 0 ||
		c.MaxSessions <= 0 || c.MaxSessionsPerCaller <= 0 || c.MaxSessionsPerCaller > c.MaxSessions || c.MaxConnections <= 0 || c.MaxConcurrentRPCs <= 0 || c.MaxStreams == 0 || c.MaxMetadataBytes < 256 || c.MaxResponseBytes <= 0 {
		return nil, errors.New("invalid Facet service configuration")
	}
	if c.CoreServices != nil {
		services := make(map[values.PrincipalRef]values.FactAuthorityRef, len(c.CoreServices))
		for principal, core := range c.CoreServices {
			if !principal.Valid() || !core.Valid() || core == c.Expected {
				return nil, errors.New("invalid fixed Core service")
			}
			services[principal] = core
		}
		c.CoreServices = services
	}
	if len(c.CoreServices) != 0 {
		if _, ok := owner.(acceptanceControlOwner); !ok {
			return nil, errors.New("Facet acceptance owner required")
		}
		if _, ok := owner.(replacementOwner); !ok {
			return nil, errors.New("namespace replacement owner required")
		}
	}
	s := &Server{config: c, owner: owner, auth: auth, sessions: make(map[[32]byte]session), active: make(chan struct{}, c.MaxConcurrentRPCs)}
	service, err := NewService(owner, s, c.Expected, c.NextHop, c.MaxResponseBytes)
	if err != nil {
		return nil, err
	}
	tlsConfig := &tls.Config{Certificates: []tls.Certificate{c.Certificate}, MinVersion: tls.VersionTLS13, MaxVersion: tls.VersionTLS13, NextProtos: []string{"h2"}, VerifyConnection: func(state tls.ConnectionState) error {
		if state.NegotiatedProtocol != "h2" {
			return errors.New("HTTP/2 negotiation required")
		}
		return nil
	}}
	s.grpc = grpc.NewServer(grpc.Creds(credentials.NewTLS(tlsConfig)), grpc.ConnectionTimeout(c.HandshakeTimeout), grpc.MaxRecvMsgSize(4<<20), grpc.MaxSendMsgSize(c.MaxResponseBytes), grpc.MaxHeaderListSize(c.MaxMetadataBytes), grpc.MaxConcurrentStreams(c.MaxStreams), grpc.InTapHandle(grpcbound.Tap(c.RPCDeadline)), grpc.StatsHandler(grpcbound.Stats{}), grpc.UnaryInterceptor(s.intercept))
	facetpb.RegisterFacetServiceServer(s.grpc, service)
	writerOwner, _ := owner.(writertransport.Owner)
	facetpb.RegisterWriterControlServiceServer(s.grpc, writertransport.New(writerOwner, c.Expected, s))
	facetpb.RegisterFacetRelationshipServiceServer(s.grpc, s)
	facetpb.RegisterFacetAcceptanceServiceServer(s.grpc, s)
	facetpb.RegisterNamespaceReplacementServiceServer(s.grpc, s)
	networkpb.RegisterSessionServiceServer(s.grpc, s)
	return s, nil
}

func (s *Server) Serve(l net.Listener) error {
	if l == nil {
		return errors.New("listener required")
	}
	s.mu.Lock()
	if s.stopped || s.listener != nil {
		s.mu.Unlock()
		_ = l.Close()
		return errors.New("service already serving or stopped")
	}
	limited := grpcbound.NewListener(l, s.config.MaxConnections, s.config.ConnectionLifetime)
	s.listener = limited
	s.mu.Unlock()
	return s.grpc.Serve(limited)
}

func (s *Server) Stop() {
	s.stopOnce.Do(func() {
		s.mu.Lock()
		s.stopped = true
		clear(s.sessions)
		l := s.listener
		s.mu.Unlock()
		if l != nil {
			_ = l.Close()
		}
		done := make(chan struct{})
		go func() { s.running.Wait(); close(done) }()
		select {
		case <-done:
		case <-time.After(s.config.ShutdownTimeout):
		}
		if l != nil {
			l.CloseConnections()
		}
		s.grpc.Stop()
	})
}

type callerKey struct{}

func (s *Server) Caller(ctx context.Context) (values.CallerContext, error) {
	caller, ok := ctx.Value(callerKey{}).(values.CallerContext)
	if !ok {
		return values.CallerContext{}, status.Error(codes.Unauthenticated, "session required")
	}
	return caller, nil
}

func (s *Server) intercept(ctx context.Context, in any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
	md, _ := metadata.FromIncomingContext(ctx)
	target := md.Get("ucupa-target")
	if len(target) != 1 || !values.FactAuthorityRef(target[0]).Valid() {
		return nil, status.Error(codes.InvalidArgument, "invalid target")
	}
	if target[0] != string(s.config.Expected) {
		return nil, status.Error(codes.Unavailable, "target unavailable")
	}
	select {
	case s.active <- struct{}{}:
		defer func() { <-s.active }()
	default:
		return nil, status.Error(codes.ResourceExhausted, "request capacity")
	}
	if err := ctx.Err(); err != nil {
		return nil, status.FromContextError(err).Err()
	}
	s.mu.Lock()
	stopped := s.stopped
	if !stopped {
		s.running.Add(1)
	}
	s.mu.Unlock()
	if stopped {
		return nil, status.Error(codes.Unavailable, "service stopped")
	}
	defer s.running.Done()
	message, ok := in.(proto.Message)
	if !ok {
		return nil, status.Error(codes.InvalidArgument, "invalid message")
	}
	if _, err := representation.ToValue(message.ProtoReflect(), 0); err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid message")
	}
	if info.FullMethod != networkpb.SessionService_EstablishSession_FullMethodName && info.FullMethod != networkpb.SessionService_ReleaseSession_FullMethodName {
		caller, err := s.caller(md)
		if err != nil {
			return nil, err
		}
		ctx = context.WithValue(ctx, callerKey{}, caller)
	}
	return handler(ctx, in)
}
