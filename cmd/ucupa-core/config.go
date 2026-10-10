package main

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"io"
	"net"
	"os"
	"time"

	"ucupa/composition"
	"ucupa/shared/values"
	corecontract "ucupa/ucupa-core/contract"
	coregrpc "ucupa/ucupa-core/transport/grpc"
)

type config struct {
	Writer                *writerConfig                `json:"writer"`
	Listen                string                       `json:"listen"`
	NextHop               values.NextHop               `json:"next_hop"`
	Expected              values.FactAuthorityRef      `json:"expected_core"`
	RealmID               string                       `json:"realm_id"`
	RealmProfile          string                       `json:"realm_profile"`
	RealmNextHop          values.NextHop               `json:"realm_next_hop"`
	RealmEndpoints        []networkRealmEndpointConfig `json:"realm_endpoints"`
	CoreStore             string                       `json:"core_store"`
	RealmStore            string                       `json:"realm_store"`
	Certificate           string                       `json:"certificate_file"`
	PrivateKey            string                       `json:"private_key_file"`
	SessionTTL            string                       `json:"session_ttl"`
	RPCDeadline           string                       `json:"rpc_deadline"`
	HandshakeTimeout      string                       `json:"handshake_timeout"`
	ConnectionLifetime    string                       `json:"connection_lifetime"`
	ShutdownTimeout       string                       `json:"shutdown_timeout"`
	MaxSessions           int                          `json:"max_sessions"`
	MaxSessionsPerCaller  int                          `json:"max_sessions_per_caller"`
	MaxConnections        int                          `json:"max_connections"`
	MaxConcurrentRPCs     int                          `json:"max_concurrent_rpcs"`
	MaxStreams            uint32                       `json:"max_streams"`
	MaxMetadataBytes      uint32                       `json:"max_metadata_bytes"`
	MaxResponseBytes      int                          `json:"max_response_bytes"`
	ContextTTL            string                       `json:"context_ttl"`
	ObservationTTL        string                       `json:"observation_ttl"`
	SnapshotRetention     *string                      `json:"snapshot_retention"`
	PublicMaxContexts     *uint64                      `json:"public_max_contexts"`
	PublicMaxPerCaller    *uint64                      `json:"public_max_contexts_per_caller"`
	PublicMaxGroups       *uint64                      `json:"public_max_groups_per_observation"`
	PublicMaxBytes        *uint64                      `json:"public_max_bytes"`
	PublicMaxReads        *uint64                      `json:"public_max_concurrent_reads"`
	ReadDelegation        *delegationConfig            `json:"read_delegation"`
	IdentityIssuer        *identityIssuerConfig        `json:"identity_issuer"`
	ExternalIdentity      *externalIdentityConfig      `json:"external_identity"`
	FacetServices         []facetServiceConfig         `json:"facet_services"`
	MountApprovals        *mountApprovalConfig         `json:"mount_approvals"`
	TransitionApprovals   *transitionApprovalConfig    `json:"transition_approvals"`
	TransitionConnections *transitionConnectionConfig  `json:"transition_connections"`
	CarrierApprovals      *transitionApprovalConfig    `json:"carrier_approvals"`
	MountConnections      []mountConnectionConfig      `json:"mount_connections"`
	FacetConnections      []facetConnectionConfig      `json:"facet_connections"`
	Federation            *federationConfig            `json:"federation"`
	coreConfig            corecontract.Config
	networkConfig         coregrpc.Config
	realmBindings         []realmEndpointBinding
}

// realmEndpointBinding is one pinned network Realm address with its loaded
// client material, ready for the connections port.
type realmEndpointBinding struct {
	address  values.NextHop
	material composition.NetworkRealmEndpointConfig
}

// networkRealmEndpointConfig pins one network Realm address: TLS roots, the
// SPKI pin, and the service identity the serving Realm binds to this owner.
type networkRealmEndpointConfig struct {
	NextHop            values.NextHop `json:"next_hop"`
	CAFile             string         `json:"ca_file"`
	SPKI               string         `json:"spki_sha256"`
	ServiceCertificate string         `json:"service_certificate_file"`
	ServicePrivateKey  string         `json:"service_private_key_file"`
}

// realmProfileNetwork reports whether this configuration assembles the Realm
// contact through pinned network endpoints instead of the local kernel.
func (c config) realmProfileNetwork() bool {
	return c.RealmProfile == "network"
}

type facetServiceConfig struct {
	Realm     values.RealmID          `json:"realm"`
	Principal values.PrincipalID      `json:"principal"`
	Facet     values.FactAuthorityRef `json:"facet"`
}

type mountApprovalConfig struct {
	MaxTTL       string `json:"max_ttl"`
	MaxApprovals int    `json:"max_approvals"`
	MaxPerCaller int    `json:"max_per_caller"`
}

func readConfig(path string) (config, error) {
	var c config
	f, err := os.Open(path)
	if err != nil {
		return c, errors.New("cannot read configuration")
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, 65537))
	if err != nil || len(data) > 65536 {
		return c, errors.New("invalid configuration size")
	}
	if _, err := values.ParsePublicJSON(data, values.PublicJSONLimits()); err != nil {
		return c, errors.New("invalid configuration JSON")
	}
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if err := d.Decode(&c); err != nil {
		return c, errors.New("invalid configuration fields")
	}
	if c.RealmProfile != "" && c.RealmProfile != "local" && c.RealmProfile != "network" {
		return c, errors.New("Realm profile must be local or network")
	}
	if c.RealmID == "" || c.CoreStore == "" {
		return c, errors.New("an existing Core Store is required")
	}
	// The two Realm profiles are explicit and mutually exclusive: the local
	// kernel binding or pinned network endpoints. Neither supplies or replaces
	// the persisted contact; open verifies it.
	if c.realmProfileNetwork() {
		if c.RealmStore != "" || c.RealmNextHop != "" || len(c.RealmEndpoints) == 0 {
			return c, errors.New("network Realm profile requires pinned endpoints only")
		}
	} else if c.RealmStore == "" || c.CoreStore == c.RealmStore || !c.RealmNextHop.IsLocal() || len(c.RealmEndpoints) != 0 {
		return c, errors.New("local Realm profile requires independent Stores and a local address")
	}
	c.coreConfig.ExpectedRealm = values.RealmID(c.RealmID)
	if _, _, err := net.SplitHostPort(c.Listen); err != nil {
		return c, errors.New("explicit listening port is required")
	}
	target, ok := c.NextHop.NetworkTarget()
	if !ok {
		return c, errors.New("invalid advertised address")
	}
	host, _, _ := net.SplitHostPort(target)
	if ip := net.ParseIP(host); ip != nil && ip.IsUnspecified() {
		return c, errors.New("unspecified advertised address")
	}
	c.networkConfig = coregrpc.Config{Expected: c.Expected, NextHop: c.NextHop, MaxSessions: c.MaxSessions, MaxSessionsPerCaller: c.MaxSessionsPerCaller, MaxConnections: c.MaxConnections, MaxConcurrentRPCs: c.MaxConcurrentRPCs, MaxStreams: c.MaxStreams, MaxMetadataBytes: c.MaxMetadataBytes, MaxResponseBytes: c.MaxResponseBytes}
	c.networkConfig.FacetServices = make(map[values.PrincipalRef]values.FactAuthorityRef, len(c.FacetServices))
	for _, service := range c.FacetServices {
		principal := values.PrincipalRef{Realm: service.Realm, Principal: service.Principal}
		if !principal.Valid() || !service.Facet.Valid() || c.networkConfig.FacetServices[principal] != "" {
			return c, errors.New("invalid or duplicate Facet service binding")
		}
		c.networkConfig.FacetServices[principal] = service.Facet
	}
	for _, entry := range []struct {
		text string
		out  *time.Duration
	}{{c.SessionTTL, &c.networkConfig.SessionTTL}, {c.RPCDeadline, &c.networkConfig.RPCDeadline}, {c.HandshakeTimeout, &c.networkConfig.HandshakeTimeout}, {c.ConnectionLifetime, &c.networkConfig.ConnectionLifetime}, {c.ShutdownTimeout, &c.networkConfig.ShutdownTimeout}} {
		v, err := time.ParseDuration(entry.text)
		if err != nil || v <= 0 {
			return c, errors.New("explicit positive time limits required")
		}
		*entry.out = v
	}
	contextTTL, e1 := time.ParseDuration(c.ContextTTL)
	observationTTL, e2 := time.ParseDuration(c.ObservationTTL)
	if e1 != nil || e2 != nil || contextTTL <= 0 || observationTTL <= 0 {
		return c, errors.New("explicit public retention times required")
	}
	c.coreConfig.PublicAccessPolicy = &values.PublicAccessPolicy{ContextTTL: contextTTL, ObservationTTL: observationTTL,
		MaxContexts: c.PublicMaxContexts, MaxContextsPerCaller: c.PublicMaxPerCaller, MaxGroupsPerObservation: c.PublicMaxGroups, MaxPublicBytes: c.PublicMaxBytes, MaxConcurrentPublicReads: c.PublicMaxReads}
	if c.SnapshotRetention != nil {
		v, err := time.ParseDuration(*c.SnapshotRetention)
		if err != nil || v <= 0 {
			return c, errors.New("invalid snapshot retention")
		}
		c.coreConfig.SnapshotRetention = &v
	}
	c.networkConfig.Certificate, err = tls.LoadX509KeyPair(c.Certificate, c.PrivateKey)
	if err != nil {
		return c, errors.New("cannot load TLS key pair")
	}
	if c.realmProfileNetwork() {
		seen := make(map[values.NextHop]bool, len(c.RealmEndpoints))
		for _, endpoint := range c.RealmEndpoints {
			if _, ok := endpoint.NextHop.NetworkTarget(); !ok || seen[endpoint.NextHop] {
				return c, errors.New("invalid or duplicate pinned Realm endpoint")
			}
			seen[endpoint.NextHop] = true
			ca, err := readIdentityFile(endpoint.CAFile, 1<<20)
			if err != nil {
				return c, errors.New("invalid pinned Realm endpoint roots")
			}
			roots := x509.NewCertPool()
			if !roots.AppendCertsFromPEM(ca) {
				return c, errors.New("invalid pinned Realm endpoint roots")
			}
			pin, err := values.DecodePublicBytes(endpoint.SPKI)
			if err != nil || len(pin) != 32 {
				return c, errors.New("invalid pinned Realm endpoint SPKI pin")
			}
			service, err := tls.LoadX509KeyPair(endpoint.ServiceCertificate, endpoint.ServicePrivateKey)
			if err != nil {
				return c, errors.New("invalid pinned Realm endpoint service identity")
			}
			c.realmBindings = append(c.realmBindings, realmEndpointBinding{address: endpoint.NextHop, material: composition.NetworkRealmEndpointConfig{
				Roots: roots, SPKISHA256: [32]byte(pin), Service: &service,
				RPCDeadline: c.networkConfig.RPCDeadline, ConnectTimeout: c.networkConfig.HandshakeTimeout,
				MaxResponseBytes: c.networkConfig.MaxResponseBytes, MaxMetadataBytes: c.networkConfig.MaxMetadataBytes,
			}})
		}
	}
	if c.ReadDelegation != nil {
		c.networkConfig.ReadDelegation, err = c.ReadDelegation.load()
		if err != nil {
			return c, err
		}
	}
	if c.MountApprovals != nil {
		ttl, err := time.ParseDuration(c.MountApprovals.MaxTTL)
		if err != nil || ttl <= 0 || ttl%time.Second != 0 || c.ReadDelegation == nil || c.MountApprovals.MaxApprovals <= 0 || c.MountApprovals.MaxPerCaller <= 0 || c.MountApprovals.MaxPerCaller > c.MountApprovals.MaxApprovals {
			return c, errors.New("invalid mount approval limits or missing peer trust")
		}
		c.networkConfig.MountApprovals = &coregrpc.MountApprovalConfig{MaxTTL: ttl, MaxApprovals: c.MountApprovals.MaxApprovals, MaxPerCaller: c.MountApprovals.MaxPerCaller}
	}
	c.networkConfig.TransitionApprovals, err = c.TransitionApprovals.load(c.ReadDelegation != nil)
	if err != nil {
		return c, err
	}
	c.networkConfig.CarrierApprovals, err = c.CarrierApprovals.loadCarrier(c.ReadDelegation != nil)
	if err != nil {
		return c, err
	}
	c.coreConfig.Writer, err = c.Writer.load()
	if err != nil {
		return c, err
	}
	return c, nil
}
