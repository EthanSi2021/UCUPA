package main

import (
	"crypto/x509"
	"errors"
	"io"
	"os"
	"time"

	"ucupa/shared/values"
	coregrpc "ucupa/ucupa-core/transport/grpc"
)

// No address is learned from this trust binding. The relation/reference still
// supplies the exact service address; only an explicitly named actor may act.
type delegationConfig struct {
	ClientCAFile string                             `json:"client_ca_file"`
	ClientSPKI   map[values.FactAuthorityRef]string `json:"client_spki_sha256"`
	MaxTTL       string                             `json:"max_ttl"`
	MaxGrants    int                                `json:"max_grants"`
	MaxPerCaller int                                `json:"max_per_caller"`
}

func (c *delegationConfig) load() (*coregrpc.ReadDelegationConfig, error) {
	bad := errors.New("invalid read delegation configuration")
	ttl, err := time.ParseDuration(c.MaxTTL)
	if err != nil || ttl <= 0 || ttl%time.Second != 0 || len(c.ClientSPKI) == 0 || c.MaxGrants <= 0 || c.MaxPerCaller <= 0 || c.MaxPerCaller > c.MaxGrants {
		return nil, bad
	}
	file, err := os.Open(c.ClientCAFile)
	if err != nil {
		return nil, bad
	}
	defer file.Close()
	pem, err := io.ReadAll(io.LimitReader(file, (1<<20)+1))
	if err != nil || len(pem) > 1<<20 {
		return nil, bad
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(pem) {
		return nil, bad
	}
	out := &coregrpc.ReadDelegationConfig{ClientRoots: roots, Peers: make(map[values.FactAuthorityRef][32]byte), MaxTTL: ttl, MaxGrants: c.MaxGrants, MaxPerCaller: c.MaxPerCaller}
	for actor, encoded := range c.ClientSPKI {
		pin, err := values.DecodePublicBytes(encoded)
		if !actor.Valid() || err != nil || len(pin) != 32 || [32]byte(pin) == ([32]byte{}) {
			return nil, bad
		}
		out.Peers[actor] = [32]byte(pin)
	}
	return out, nil
}
