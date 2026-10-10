package realmgrpc

import (
	"crypto/tls"
	"errors"
	"net"
	"reflect"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"

	"ucupa/internal/grpcbound"
	"ucupa/shared/values"
	realmpb "ucupa/ucupa-realm/bindings/go/protobuf"
	admission "ucupa/ucupa-realm/transport/grpc/internal/admission"
	entry "ucupa/ucupa-realm/transport/grpc/internal/entry"
	wire "ucupa/ucupa-realm/transport/grpc/internal/wire"
)

type Server struct {
	config    Config
	grpc      *grpc.Server
	mu        sync.Mutex
	stopOnce  sync.Once
	listener  *grpcbound.Listener
	admission *admission.Registry
	service   *entry.Service
}

func New(kernel entry.Kernel, c Config) (*Server, error) { return newServer(kernel, c, time.Now) }

func newServer(kernel entry.Kernel, c Config, now func() time.Time) (*Server, error) {
	if kernel == nil || kernel.ID() == "" {
		return nil, errors.New("Realm kernel required")
	}
	if active, ok := kernel.ActiveID(); !ok || active != c.Expected {
		return nil, errors.New("Realm identity mismatch")
	}
	if len(c.Certificate.Certificate) == 0 || c.Certificate.PrivateKey == nil || c.ClientRoots == nil ||
		c.HandleTTL <= 0 || c.HandleTTL%time.Second != 0 || c.ChallengeTTL <= 0 || c.ChallengeTTL%time.Second != 0 ||
		c.RPCDeadline <= 0 || c.HandshakeTimeout <= 0 || c.ConnectionLifetime < c.RPCDeadline || c.ShutdownTimeout <= 0 ||
		c.MaxHandles <= 0 || c.MaxHandlesPerCaller <= 0 || c.MaxHandlesPerCaller > c.MaxHandles ||
		c.MaxChallenges <= 0 || c.MaxChallengesPerOwner <= 0 || c.MaxChallengesPerOwner > c.MaxChallenges ||
		c.MaxRequirements <= 0 || c.MaxRequirements > wire.MaxRequirements ||
		c.MaxConnections <= 0 || c.MaxConcurrentRPCs <= 0 || c.MaxResponseBytes <= 0 ||
		c.MaxStreams == 0 || c.MaxMetadataBytes < 256 {
		return nil, errors.New("invalid Realm service configuration")
	}
	services := make(map[[32]byte]values.FactAuthorityRef, len(c.AdmittedServices))
	for spki, owner := range c.AdmittedServices {
		if spki == ([32]byte{}) || !owner.Valid() {
			return nil, errors.New("invalid admitted service")
		}
		services[spki] = owner
	}
	c.AdmittedServices = services
	// A typed nil verifier was an absent optional verifier before the port
	// extraction; converting it to an interface must not enable foreign identity.
	if c.Foreign != nil && reflect.ValueOf(c.Foreign).Kind() == reflect.Pointer && reflect.ValueOf(c.Foreign).IsNil() {
		c.Foreign = nil
	}
	if c.Foreign != nil {
		if len(c.ForeignLiveness) == 0 {
			return nil, errors.New("issuer trust requires liveness sources")
		}
	} else if len(c.ForeignLiveness) != 0 {
		return nil, errors.New("liveness sources require issuer trust")
	}
	registry := admission.New(admission.Config{Expected: c.Expected, AdmittedServices: services, MaxConcurrentRPCs: c.MaxConcurrentRPCs, ChallengeTTL: c.ChallengeTTL, MaxChallenges: c.MaxChallenges, MaxChallengesPerOwner: c.MaxChallengesPerOwner}, now)
	service := entry.New(kernel, entry.Config{Expected: c.Expected, Foreign: c.Foreign, ForeignLiveness: c.ForeignLiveness, HandleTTL: c.HandleTTL, ChallengeTTL: c.ChallengeTTL, MaxHandles: c.MaxHandles, MaxHandlesPerCaller: c.MaxHandlesPerCaller, MaxRequirements: c.MaxRequirements}, registry, now)
	server := &Server{config: c, admission: registry, service: service}
	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{c.Certificate},
		MinVersion:   tls.VersionTLS13, MaxVersion: tls.VersionTLS13, NextProtos: []string{"h2"},
		// Protected calls require an admitted client certificate; the read-only
		// target check deliberately stays reachable without service admission.
		ClientAuth: tls.VerifyClientCertIfGiven, ClientCAs: c.ClientRoots.Clone(),
		VerifyConnection: func(state tls.ConnectionState) error {
			if state.NegotiatedProtocol != "h2" {
				return errors.New("HTTP/2 negotiation required")
			}
			if len(state.PeerCertificates) > 0 && len(state.VerifiedChains) == 0 {
				return errors.New("client certificate not verified")
			}
			return nil
		},
	}
	server.grpc = grpc.NewServer(
		grpc.Creds(credentials.NewTLS(tlsConfig)),
		grpc.ConnectionTimeout(c.HandshakeTimeout),
		grpc.MaxRecvMsgSize(4<<20),
		grpc.MaxSendMsgSize(c.MaxResponseBytes),
		grpc.MaxHeaderListSize(c.MaxMetadataBytes),
		grpc.MaxConcurrentStreams(c.MaxStreams),
		grpc.InTapHandle(grpcbound.Tap(c.RPCDeadline)),
		grpc.StatsHandler(grpcbound.Stats{}),
		grpc.UnaryInterceptor(registry.Intercept),
	)
	realmpb.RegisterRealmTargetServiceServer(server.grpc, service)
	realmpb.RegisterRealmIdentityServiceServer(server.grpc, service)
	realmpb.RegisterRealmAuthorizationServiceServer(server.grpc, service)
	return server, nil
}

// Serve admits bounded connections until Stop. Serving or restarting a
// stopped instance is rejected; the host owns the listener lifetime.
func (s *Server) Serve(l net.Listener) error {
	if l == nil {
		return errors.New("listener required")
	}
	s.mu.Lock()
	if s.admission.Stopped() || s.listener != nil {
		s.mu.Unlock()
		_ = l.Close()
		return errors.New("service already serving or stopped")
	}
	limited := grpcbound.NewListener(l, s.config.MaxConnections, s.config.ConnectionLifetime)
	s.listener = limited
	s.mu.Unlock()
	return s.grpc.Serve(limited)
}

// Stop drops every run-local reference and challenge: nothing issued by this
// run survives into another. Shutdown waits for admitted work within the
// configured timeout, then closes remaining connections. Serving or serving
// again after Stop is rejected; the host owns the listener lifetime.
func (s *Server) Stop() {
	s.stopOnce.Do(func() {
		s.mu.Lock()
		s.admission.Stop()
		l := s.listener
		s.mu.Unlock()
		// GracefulStop closes the listener itself, so the accept loop sees the
		// quit flag before the close error and Serve returns nil. A deadline
		// overrun forces the remainder closed.
		done := make(chan struct{})
		go func() { s.grpc.GracefulStop(); close(done) }()
		select {
		case <-done:
		case <-time.After(s.config.ShutdownTimeout):
			s.grpc.Stop()
			<-done
		}
		if l != nil {
			l.CloseConnections()
		}
	})
}
