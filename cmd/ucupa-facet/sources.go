package main

import (
	"crypto/x509"
	"errors"

	"ucupa/composition"
	"ucupa/shared/values"
	coresdk "ucupa/ucupa-core/sdk/go"
)

type coreConnection struct {
	NextHop        values.NextHop          `json:"next_hop"`
	Expected       values.FactAuthorityRef `json:"expected_core"`
	CAFile         string                  `json:"ca_file"`
	SPKI           string                  `json:"spki_sha256"`
	CredentialFile string                  `json:"credential_file"`
}

type credentialFile string

func (path credentialFile) Evidence() (values.RealmID, [16]byte, [32]byte, error) {
	var v struct {
		Audience values.RealmID `json:"audience"`
		ID       string         `json:"credential_id"`
		Secret   string         `json:"secret"`
	}
	bad := errors.New("service credential unavailable")
	b, err := readLimited(string(path), 4096)
	if err != nil {
		return "", [16]byte{}, [32]byte{}, bad
	}
	defer clear(b)
	if err = strictJSON(b, &v); err != nil {
		return "", [16]byte{}, [32]byte{}, bad
	}
	id, e1 := values.DecodePublicBytes(v.ID)
	secret, e2 := values.DecodePublicBytes(v.Secret)
	defer clear(secret)
	if v.Audience == "" || e1 != nil || e2 != nil || len(id) != 16 || len(secret) != 32 {
		return "", [16]byte{}, [32]byte{}, bad
	}
	return v.Audience, [16]byte(id), [32]byte(secret), nil
}

func (c config) openCoreSource() (*composition.NetworkFacetCoreSource, func(), error) {
	var clients []*coresdk.Client
	closeClients := func() {
		for _, c := range clients {
			_ = c.Close()
		}
	}
	var endpoints []composition.FacetCoreEndpoint
	bad := errors.New("invalid Core connection configuration")
	for _, e := range c.CoreConnections {
		ca, err := readLimited(e.CAFile, 1<<20)
		if err != nil {
			closeClients()
			return nil, nil, bad
		}
		roots := x509.NewCertPool()
		pin, err := values.DecodePublicBytes(e.SPKI)
		if !roots.AppendCertsFromPEM(ca) || err != nil || len(pin) != 32 || e.CredentialFile == "" {
			closeClients()
			return nil, nil, bad
		}
		credentials := credentialFile(e.CredentialFile)
		_, _, secret, err := credentials.Evidence()
		clear(secret[:])
		if err != nil {
			closeClients()
			return nil, nil, bad
		}
		client, err := coresdk.New(coresdk.Config{NextHop: e.NextHop, Expected: e.Expected, Roots: roots, SPKISHA256: [32]byte(pin), RPCDeadline: c.networkConfig.RPCDeadline, ConnectTimeout: c.networkConfig.HandshakeTimeout, MaxMetadataBytes: c.MaxMetadataBytes, MaxResponseBytes: c.MaxResponseBytes})
		if err != nil {
			closeClients()
			return nil, nil, bad
		}
		clients = append(clients, client)
		endpoints = append(endpoints, composition.FacetCoreEndpoint{Address: e.NextHop, Core: e.Expected, Client: client, Credentials: credentials})
	}
	source, err := composition.NewNetworkFacetCoreSource(c.Expected, endpoints)
	if err != nil {
		closeClients()
		return nil, nil, bad
	}
	return source, closeClients, nil
}
