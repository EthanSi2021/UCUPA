package main

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"net"
	"time"

	"ucupa/shared/values"
	realmgrpc "ucupa/ucupa-realm/transport/grpc"
)

// The service configuration must declare every bound explicitly; no zero
// value means unlimited, and nothing is inferred from the environment.
type config struct {
	RealmStore            string                  `json:"realm_store"`
	RealmID               values.RealmID          `json:"realm_id"`
	Listen                string                  `json:"listen"`
	NextHop               values.NextHop          `json:"next_hop"`
	Certificate           string                  `json:"certificate_file"`
	PrivateKey            string                  `json:"private_key_file"`
	ClientRoots           string                  `json:"client_roots_file"`
	AdmittedServices      []admittedServiceConfig `json:"admitted_services"`
	HandleTTL             string                  `json:"handle_ttl"`
	ChallengeTTL          string                  `json:"challenge_ttl"`
	RPCDeadline           string                  `json:"rpc_deadline"`
	HandshakeTimeout      string                  `json:"handshake_timeout"`
	ConnectionLifetime    string                  `json:"connection_lifetime"`
	ShutdownTimeout       string                  `json:"shutdown_timeout"`
	MaxHandles            int                     `json:"max_handles"`
	MaxHandlesPerCaller   int                     `json:"max_handles_per_caller"`
	MaxChallenges         int                     `json:"max_challenges"`
	MaxChallengesPerOwner int                     `json:"max_challenges_per_owner"`
	MaxRequirements       int                     `json:"max_requirements"`
	MaxConnections        int                     `json:"max_connections"`
	MaxConcurrentRPCs     int                     `json:"max_concurrent_rpcs"`
	MaxResponseBytes      int                     `json:"max_response_bytes"`
	MaxStreams            uint32                  `json:"max_streams"`
	MaxMetadataBytes      uint32                  `json:"max_metadata_bytes"`
	Foreign               *foreignConfig          `json:"foreign"`

	network realmgrpc.Config
}

// admittedServiceConfig binds one pinned client-certificate SPKI to the one
// fact owner it may act for; service admission stays separate from user
// permission.
type admittedServiceConfig struct {
	SPKISHA256 string                  `json:"spki_sha256"`
	Owner      values.FactAuthorityRef `json:"owner"`
}

func readConfig(path string) (config, error) {
	var c config
	b, err := readLimited(path, 65536)
	if err != nil {
		return c, err
	}
	if err := strictJSON(b, &c); err != nil {
		return c, err
	}
	if c.RealmStore == "" || c.RealmID == "" {
		return c, errors.New("existing Realm Store required")
	}
	if _, _, err := net.SplitHostPort(c.Listen); err != nil {
		return c, errors.New("explicit listening port required")
	}
	target, ok := c.NextHop.NetworkTarget()
	if !ok {
		return c, errors.New("invalid advertised address")
	}
	host, _, _ := net.SplitHostPort(target)
	if ip := net.ParseIP(host); ip != nil && ip.IsUnspecified() {
		return c, errors.New("unspecified advertised address")
	}
	if len(c.AdmittedServices) > 256 {
		return c, errors.New("configured service count exceeds host capacity")
	}
	c.network = realmgrpc.Config{
		Expected:              c.RealmID,
		MaxHandles:            c.MaxHandles,
		MaxHandlesPerCaller:   c.MaxHandlesPerCaller,
		MaxChallenges:         c.MaxChallenges,
		MaxChallengesPerOwner: c.MaxChallengesPerOwner,
		MaxRequirements:       c.MaxRequirements,
		MaxConnections:        c.MaxConnections,
		MaxConcurrentRPCs:     c.MaxConcurrentRPCs,
		MaxResponseBytes:      c.MaxResponseBytes,
		MaxStreams:            c.MaxStreams,
		MaxMetadataBytes:      c.MaxMetadataBytes,
	}
	c.network.AdmittedServices = make(map[[32]byte]values.FactAuthorityRef, len(c.AdmittedServices))
	for _, service := range c.AdmittedServices {
		raw, err := decodeFixedHex(service.SPKISHA256, 32)
		if err != nil || !service.Owner.Valid() {
			return c, errors.New("invalid admitted service binding")
		}
		var spki [32]byte
		copy(spki[:], raw)
		if _, duplicate := c.network.AdmittedServices[spki]; duplicate {
			return c, errors.New("invalid admitted service binding")
		}
		c.network.AdmittedServices[spki] = service.Owner
	}
	for _, item := range []struct {
		text string
		out  *time.Duration
	}{
		{c.HandleTTL, &c.network.HandleTTL},
		{c.ChallengeTTL, &c.network.ChallengeTTL},
		{c.RPCDeadline, &c.network.RPCDeadline},
		{c.HandshakeTimeout, &c.network.HandshakeTimeout},
		{c.ConnectionLifetime, &c.network.ConnectionLifetime},
		{c.ShutdownTimeout, &c.network.ShutdownTimeout},
	} {
		v, err := time.ParseDuration(item.text)
		if err != nil || v <= 0 {
			return c, errors.New("explicit positive time limits required")
		}
		*item.out = v
	}
	if c.network.HandleTTL%time.Second != 0 || c.network.ChallengeTTL%time.Second != 0 ||
		c.network.ConnectionLifetime < c.network.RPCDeadline {
		return c, errors.New("inconsistent time limits")
	}
	if c.network.MaxHandles <= 0 || c.network.MaxHandlesPerCaller <= 0 ||
		c.network.MaxChallenges <= 0 || c.network.MaxChallengesPerOwner <= 0 ||
		c.network.MaxRequirements <= 0 || c.network.MaxConnections <= 0 ||
		c.network.MaxConcurrentRPCs <= 0 || c.network.MaxResponseBytes <= 0 ||
		c.network.MaxStreams == 0 || c.network.MaxMetadataBytes < 256 {
		return c, errors.New("explicit positive resource limits required")
	}
	c.network.Certificate, err = tls.LoadX509KeyPair(c.Certificate, c.PrivateKey)
	if err != nil {
		return c, errors.New("cannot load TLS key pair")
	}
	roots, err := readClientRoots(c.ClientRoots)
	if err != nil {
		return c, err
	}
	c.network.ClientRoots = roots
	return c, nil
}

// readClientRoots loads one explicit PEM bundle of client root certificates;
// an empty or unparsable trust set is rejected rather than defaulted.
func readClientRoots(path string) (*x509.CertPool, error) {
	b, err := readLimited(path, 262144)
	if err != nil {
		return nil, err
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(b) {
		return nil, errors.New("cannot load client roots")
	}
	return pool, nil
}
