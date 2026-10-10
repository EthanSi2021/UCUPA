package main

import (
	"errors"
	"io"
	"os"
	"ucupa/shared/values"
)

// The install secret belongs to controlled startup, never the RPC or main log.
// The previous authorization_binding label is gone: an old config naming it is
// rejected by the strict config decoder instead of being silently ignored.
type writerConfig struct {
	Handoff           bool   `json:"handoff"`
	InstallOperation  []byte `json:"install_operation"`
	InstallSecretFile string `json:"install_secret_file"`
}

func (c *writerConfig) load() (*values.WriterStartup, error) {
	if c == nil {
		return nil, nil
	}
	out := &values.WriterStartup{Handoff: c.Handoff}
	if (len(c.InstallOperation) == 0) != (c.InstallSecretFile == "") {
		return nil, errors.New("writer installation requires operation and secret file")
	}
	if len(c.InstallOperation) != 0 {
		f, err := os.Open(c.InstallSecretFile)
		if err != nil {
			return nil, errors.New("cannot read writer installation")
		}
		defer f.Close()
		raw, err := io.ReadAll(io.LimitReader(f, 33))
		defer clear(raw)
		if err != nil || len(raw) != 32 {
			return nil, errors.New("invalid writer installation")
		}
		out.Install = &values.WriterInstallation{Operation: string(c.InstallOperation), Secret: [32]byte(raw)}
	}
	if !out.Valid() {
		return nil, errors.New("invalid writer startup configuration")
	}
	return out, nil
}
