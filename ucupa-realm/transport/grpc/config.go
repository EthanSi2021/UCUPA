package realmgrpc

import (
	"crypto/tls"
	"crypto/x509"
	"time"

	"ucupa/shared/values"
	entry "ucupa/ucupa-realm/transport/grpc/internal/entry"
)

// Liveness is the exact configured issuer's authenticated check port. The
// service never learns an issuer address from a proof and never holds Realm
// or registry locks while the online check waits.
type Liveness = entry.Liveness

// Config declares every resource and time bound explicitly; no zero value
// means unlimited. AdmittedServices maps the pinned client-certificate SPKI
// to the one fact owner it may act for: service admission is decided here,
// user permission stays the kernel's decision.
type Config struct {
	Expected                             values.RealmID
	Certificate                          tls.Certificate
	ClientRoots                          *x509CertPool
	AdmittedServices                     map[[32]byte]values.FactAuthorityRef
	Foreign                              entry.Verifier
	ForeignLiveness                      map[values.RealmID]Liveness
	HandleTTL, ChallengeTTL              time.Duration
	RPCDeadline, HandshakeTimeout        time.Duration
	ConnectionLifetime, ShutdownTimeout  time.Duration
	MaxHandles, MaxHandlesPerCaller      int
	MaxChallenges, MaxChallengesPerOwner int
	MaxRequirements                      int
	MaxConnections, MaxConcurrentRPCs    int
	MaxResponseBytes                     int
	MaxStreams, MaxMetadataBytes         uint32
}

// x509CertPool keeps the transport's public surface free of a direct
// certificate-store type while still requiring an explicit root set.
type x509CertPool = x509.CertPool
