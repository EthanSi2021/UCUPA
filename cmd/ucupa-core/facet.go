package main

import (
	"crypto/x509"
	"errors"
	"io"
	"os"
	"ucupa/composition"
	"ucupa/shared/values"
	facetsdk "ucupa/ucupa-facet/sdk/go"
)

type facetConnectionConfig struct {
	NextHop        values.NextHop          `json:"next_hop"`
	Expected       values.FactAuthorityRef `json:"expected_facet"`
	CAFile         string                  `json:"ca_file"`
	SPKI           string                  `json:"spki_sha256"`
	CredentialFile string                  `json:"credential_file"`
}

func readFacetConnectionFile(path string, limit int64) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, errors.New("configured Facet file unavailable")
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil || int64(len(b)) > limit {
		return nil, errors.New("invalid configured Facet file")
	}
	return b, nil
}

type facetCredentialFile string

func (path facetCredentialFile) Evidence() (values.RealmID, [16]byte, [32]byte, error) {
	bad := errors.New("Facet service credential unavailable")
	b, err := readFacetConnectionFile(string(path), 4096)
	if err != nil {
		return "", [16]byte{}, [32]byte{}, bad
	}
	defer clear(b)
	v, err := values.ParsePublicJSON(b, values.PublicJSONLimits())
	if err != nil {
		return "", [16]byte{}, [32]byte{}, bad
	}
	m, ok := v.(map[string]any)
	if !ok || len(m) != 3 {
		return "", [16]byte{}, [32]byte{}, bad
	}
	a, aok := m["audience"].(string)
	i, iok := m["credential_id"].(string)
	s, sok := m["secret"].(string)
	id, e1 := values.DecodePublicBytes(i)
	secret, e2 := values.DecodePublicBytes(s)
	defer clear(secret)
	if !aok || !iok || !sok || a == "" || e1 != nil || e2 != nil || len(id) != 16 || len(secret) != 32 {
		return "", [16]byte{}, [32]byte{}, bad
	}
	return values.RealmID(a), [16]byte(id), [32]byte(secret), nil
}
func (c config) openFacetSource() (*composition.NetworkCoreFacetSource, func(), error) {
	bad := errors.New("invalid Facet connection configuration")
	var clients []*facetsdk.Client
	closeClients := func() {
		for _, client := range clients {
			_ = client.Close()
		}
	}
	ok := false
	defer func() {
		if !ok {
			closeClients()
		}
	}()
	if len(c.FacetConnections) == 0 || len(c.FacetConnections) > 256 {
		return nil, nil, bad
	}
	var endpoints []composition.CoreFacetEndpoint
	for _, e := range c.FacetConnections {
		ca, err := readFacetConnectionFile(e.CAFile, 1<<20)
		if err != nil {
			return nil, nil, bad
		}
		roots := x509.NewCertPool()
		pin, err := values.DecodePublicBytes(e.SPKI)
		if !roots.AppendCertsFromPEM(ca) || err != nil || len(pin) != 32 || e.CredentialFile == "" {
			return nil, nil, bad
		}
		credentials := facetCredentialFile(e.CredentialFile)
		_, _, secret, err := credentials.Evidence()
		clear(secret[:])
		if err != nil {
			return nil, nil, bad
		}
		client, err := facetsdk.New(facetsdk.Config{NextHop: e.NextHop, Expected: e.Expected, Roots: roots, SPKISHA256: [32]byte(pin), RPCDeadline: c.networkConfig.RPCDeadline, ConnectTimeout: c.networkConfig.HandshakeTimeout, MaxResponseBytes: c.MaxResponseBytes, MaxMetadataBytes: c.MaxMetadataBytes})
		if err != nil {
			return nil, nil, bad
		}
		clients = append(clients, client)
		endpoints = append(endpoints, composition.CoreFacetEndpoint{Address: e.NextHop, Facet: e.Expected, Client: client, Credentials: credentials})
	}
	source, err := composition.NewNetworkCoreFacetSource(c.Expected, endpoints)
	if err != nil {
		return nil, nil, bad
	}
	ok = true
	return source, closeClients, nil
}
