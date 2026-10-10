package main

import (
	"crypto/x509"
	"errors"

	"ucupa/shared/values"
	coresdk "ucupa/ucupa-core/sdk/go"
	coregrpc "ucupa/ucupa-core/transport/grpc"
	mountconn "ucupa/ucupa-core/transport/grpc/mountconn"
)

// Each entry is a pinned connection, not an authority-indexed directory.
// Requests can select it only through their relationship's specified next hop.
type mountConnectionConfig struct {
	NextHop  values.NextHop          `json:"next_hop"`
	Expected values.FactAuthorityRef `json:"expected_core"`
	CAFile   string                  `json:"ca_file"`
	SPKI     string                  `json:"spki_sha256"`
}

func openMountConnections(entries []mountConnectionConfig, host coregrpc.Config) (*mountconn.MountConnections, func(), error) {
	bad := errors.New("invalid mount connection configuration")
	var clients []*coresdk.Client
	closeClients := func() {
		for _, c := range clients {
			_ = c.Close()
		}
	}
	ok := false
	defer func() {
		if !ok {
			closeClients()
		}
	}()
	if len(entries) == 0 {
		return nil, nil, bad
	}
	for _, entry := range entries {
		ca, err := readIdentityFile(entry.CAFile, 1<<20)
		if err != nil {
			return nil, nil, bad
		}
		roots := x509.NewCertPool()
		if !roots.AppendCertsFromPEM(ca) {
			return nil, nil, bad
		}
		pin, err := values.DecodePublicBytes(entry.SPKI)
		if err != nil || len(pin) != 32 {
			return nil, nil, bad
		}
		client, err := coresdk.New(coresdk.Config{NextHop: entry.NextHop, Expected: entry.Expected,
			Roots: roots, SPKISHA256: [32]byte(pin), RPCDeadline: host.RPCDeadline,
			ConnectTimeout: host.HandshakeTimeout, MaxResponseBytes: host.MaxResponseBytes,
			MaxMetadataBytes: host.MaxMetadataBytes, Actor: host.Expected, ActorCertificate: &host.Certificate})
		if err != nil {
			return nil, nil, bad
		}
		clients = append(clients, client)
	}
	connections, err := mountconn.NewMountConnections(host.Expected, host.MountControl, clients)
	if err != nil {
		return nil, nil, bad
	}
	ok = true
	return connections, closeClients, nil
}
