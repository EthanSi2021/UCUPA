package main

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"errors"
	"io"
	"net"
	"os"
	"time"

	"ucupa/shared/values"
	facet "ucupa/ucupa-facet/instance"
	facetgrpc "ucupa/ucupa-facet/transport/grpc"
)

type config struct {
	Writer               *writerConfig           `json:"writer"`
	Listen               string                  `json:"listen"`
	NextHop              values.NextHop          `json:"next_hop"`
	Expected             values.FactAuthorityRef `json:"expected_facet"`
	RealmID              values.RealmID          `json:"realm_id"`
	RealmProfile         string                  `json:"realm_profile"`
	RealmNextHop         values.NextHop          `json:"realm_next_hop"`
	RealmEndpoints       []realmEndpointConfig   `json:"realm_endpoints"`
	FacetStore           string                  `json:"facet_store"`
	RealmStore           string                  `json:"realm_store"`
	Certificate          string                  `json:"certificate_file"`
	PrivateKey           string                  `json:"private_key_file"`
	SessionTTL           string                  `json:"session_ttl"`
	RPCDeadline          string                  `json:"rpc_deadline"`
	HandshakeTimeout     string                  `json:"handshake_timeout"`
	ConnectionLifetime   string                  `json:"connection_lifetime"`
	ShutdownTimeout      string                  `json:"shutdown_timeout"`
	MaxSessions          int                     `json:"max_sessions"`
	MaxSessionsPerCaller int                     `json:"max_sessions_per_caller"`
	MaxConnections       int                     `json:"max_connections"`
	MaxConcurrentRPCs    int                     `json:"max_concurrent_rpcs"`
	MaxStreams           uint32                  `json:"max_streams"`
	MaxMetadataBytes     uint32                  `json:"max_metadata_bytes"`
	MaxResponseBytes     int                     `json:"max_response_bytes"`
	ContextTTL           string                  `json:"context_ttl"`
	ObservationTTL       string                  `json:"observation_ttl"`
	PublicMaxContexts    *uint64                 `json:"public_max_contexts"`
	PublicMaxPerCaller   *uint64                 `json:"public_max_contexts_per_caller"`
	PublicMaxGroups      *uint64                 `json:"public_max_groups_per_observation"`
	PublicMaxBytes       *uint64                 `json:"public_max_bytes"`
	PublicMaxReads       *uint64                 `json:"public_max_concurrent_reads"`
	CoreConnections      []coreConnection        `json:"core_connections"`
	CoreServices         []coreServiceConfig     `json:"core_services"`
	ownerConfig          facet.Config
	networkConfig        facetgrpc.Config
	realmBindings        []realmEndpointBinding
}

type coreServiceConfig struct {
	Realm     values.RealmID          `json:"realm"`
	Principal values.PrincipalID      `json:"principal"`
	Core      values.FactAuthorityRef `json:"core"`
}

func readLimited(path string, maximum int64) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, errors.New("cannot read configured file")
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, maximum+1))
	if err != nil || int64(len(b)) > maximum {
		return nil, errors.New("invalid configured file size")
	}
	return b, nil
}

func strictJSON(b []byte, out any) error {
	if _, err := values.ParsePublicJSON(b, values.PublicJSONLimits()); err != nil {
		return errors.New("invalid configuration JSON")
	}
	d := json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	if err := d.Decode(out); err != nil {
		return errors.New("invalid configuration fields")
	}
	return nil
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
	if len(c.CoreConnections) > 256 {
		return c, errors.New("configured source count exceeds host capacity")
	}
	if c.RealmID == "" || !c.Expected.Valid() || c.FacetStore == "" {
		return c, errors.New("existing Facet Store and expected identities required")
	}
	if err := c.checkRealmProfile(); err != nil {
		return c, err
	}
	c.ownerConfig.ExpectedRealm = c.RealmID
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
	c.networkConfig = facetgrpc.Config{Expected: c.Expected, NextHop: c.NextHop, MaxSessions: c.MaxSessions, MaxSessionsPerCaller: c.MaxSessionsPerCaller, MaxConnections: c.MaxConnections, MaxConcurrentRPCs: c.MaxConcurrentRPCs, MaxStreams: c.MaxStreams, MaxMetadataBytes: c.MaxMetadataBytes, MaxResponseBytes: c.MaxResponseBytes}
	if len(c.CoreServices) > 256 {
		return c, errors.New("configured service count exceeds host capacity")
	}
	c.networkConfig.CoreServices = make(map[values.PrincipalRef]values.FactAuthorityRef, len(c.CoreServices))
	for _, s := range c.CoreServices {
		p := values.PrincipalRef{Realm: s.Realm, Principal: s.Principal}
		if !p.Valid() || !s.Core.Valid() || c.networkConfig.CoreServices[p] != "" {
			return c, errors.New("invalid fixed Core service binding")
		}
		c.networkConfig.CoreServices[p] = s.Core
	}
	for _, item := range []struct {
		text string
		out  *time.Duration
	}{{c.SessionTTL, &c.networkConfig.SessionTTL}, {c.RPCDeadline, &c.networkConfig.RPCDeadline}, {c.HandshakeTimeout, &c.networkConfig.HandshakeTimeout}, {c.ConnectionLifetime, &c.networkConfig.ConnectionLifetime}, {c.ShutdownTimeout, &c.networkConfig.ShutdownTimeout}} {
		v, err := time.ParseDuration(item.text)
		if err != nil || v <= 0 {
			return c, errors.New("explicit positive time limits required")
		}
		*item.out = v
	}
	contextTTL, e1 := time.ParseDuration(c.ContextTTL)
	observationTTL, e2 := time.ParseDuration(c.ObservationTTL)
	if e1 != nil || e2 != nil || contextTTL <= 0 || observationTTL <= 0 {
		return c, errors.New("explicit retention times required")
	}
	c.ownerConfig.PublicAccessPolicy = &values.PublicAccessPolicy{ContextTTL: contextTTL, ObservationTTL: observationTTL, MaxContexts: c.PublicMaxContexts, MaxContextsPerCaller: c.PublicMaxPerCaller, MaxGroupsPerObservation: c.PublicMaxGroups, MaxPublicBytes: c.PublicMaxBytes, MaxConcurrentPublicReads: c.PublicMaxReads}
	c.networkConfig.Certificate, err = tls.LoadX509KeyPair(c.Certificate, c.PrivateKey)
	if err != nil {
		return c, errors.New("cannot load TLS key pair")
	}
	c.ownerConfig.Writer, err = c.Writer.load()
	if err != nil {
		return c, err
	}
	if err := c.loadRealmEndpoints(); err != nil {
		return c, err
	}
	return c, nil
}
