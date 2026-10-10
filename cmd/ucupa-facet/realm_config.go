package main

import (
	"crypto/tls"
	"crypto/x509"
	"errors"

	"ucupa/composition"
	"ucupa/shared/values"
)

type realmEndpointConfig struct {
	NextHop            values.NextHop `json:"next_hop"`
	CAFile             string         `json:"ca_file"`
	SPKI               string         `json:"spki_sha256"`
	ServiceCertificate string         `json:"service_certificate_file"`
	ServicePrivateKey  string         `json:"service_private_key_file"`
}

type realmEndpointBinding struct {
	address  values.NextHop
	material composition.NetworkRealmEndpointConfig
}

func (c config) checkRealmProfile() error {
	if c.RealmProfile != "" && c.RealmProfile != "local" && c.RealmProfile != "network" {
		return errors.New("Realm profile must be local or network")
	}
	if c.RealmProfile == "network" {
		if c.RealmStore != "" || c.RealmNextHop != "" || len(c.RealmEndpoints) == 0 || len(c.RealmEndpoints) > 256 {
			return errors.New("network Realm profile requires bounded pinned endpoints only")
		}
	} else if c.RealmStore == "" || c.RealmStore == c.FacetStore || !c.RealmNextHop.IsLocal() || len(c.RealmEndpoints) != 0 {
		return errors.New("local Realm profile requires independent Stores and a local address")
	}
	return nil
}

func (c *config) loadRealmEndpoints() error {
	seen := make(map[values.NextHop]bool, len(c.RealmEndpoints))
	for _, endpoint := range c.RealmEndpoints {
		if _, ok := endpoint.NextHop.NetworkTarget(); !ok || seen[endpoint.NextHop] {
			return errors.New("invalid or duplicate pinned Realm endpoint")
		}
		seen[endpoint.NextHop] = true
		ca, err := readLimited(endpoint.CAFile, 1<<20)
		if err != nil {
			return errors.New("cannot read pinned Realm roots")
		}
		roots := x509.NewCertPool()
		if !roots.AppendCertsFromPEM(ca) {
			return errors.New("invalid pinned Realm roots")
		}
		pin, err := values.DecodePublicBytes(endpoint.SPKI)
		if err != nil || len(pin) != 32 {
			return errors.New("invalid pinned Realm SPKI")
		}
		service, err := tls.LoadX509KeyPair(endpoint.ServiceCertificate, endpoint.ServicePrivateKey)
		if err != nil {
			return errors.New("cannot load pinned Realm service identity")
		}
		c.realmBindings = append(c.realmBindings, realmEndpointBinding{endpoint.NextHop, composition.NetworkRealmEndpointConfig{
			Roots: roots, SPKISHA256: [32]byte(pin), Service: &service,
			RPCDeadline: c.networkConfig.RPCDeadline, ConnectTimeout: c.networkConfig.HandshakeTimeout,
			MaxResponseBytes: c.networkConfig.MaxResponseBytes, MaxMetadataBytes: c.networkConfig.MaxMetadataBytes,
		}})
	}
	return nil
}
