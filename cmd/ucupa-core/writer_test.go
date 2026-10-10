package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriterStartupQualificationAndSecretFile(t *testing.T) {
	var disabled *writerConfig
	if c, err := disabled.load(); c != nil || err != nil {
		t.Fatal(c, err)
	}
	enabled, err := (&writerConfig{Handoff: true}).load()
	if err != nil || enabled.Install != nil || !enabled.Handoff {
		t.Fatal(enabled, err)
	}
	path := filepath.Join(t.TempDir(), "private-installation")
	for _, n := range []int{0, 31, 32, 33, 10000} {
		raw := bytes.Repeat([]byte{71}, n)
		if err := os.WriteFile(path, raw, 0600); err != nil {
			t.Fatal(err)
		}
		cfg := &writerConfig{Handoff: true, InstallOperation: []byte("install"), InstallSecretFile: path}
		loaded, err := cfg.load()
		if n == 32 {
			if err != nil || loaded.Install.Secret != [32]byte(raw) {
				t.Fatal("qualified installation rejected", err)
			}
		} else {
			if err == nil || loaded != nil {
				t.Fatal("invalid secret accepted", n)
			}
			if strings.Contains(err.Error(), path) || strings.Contains(err.Error(), "GGGG") {
				t.Fatal("secret or path disclosed", err)
			}
		}
	}
	for _, cfg := range []*writerConfig{{InstallOperation: []byte("install")}, {InstallSecretFile: path}} {
		if c, err := cfg.load(); c != nil || err == nil {
			t.Fatal("invalid configuration accepted")
		}
	}
	if err := os.WriteFile(path, make([]byte, 32), 0600); err != nil {
		t.Fatal(err)
	}
	if c, err := (&writerConfig{Handoff: true, InstallOperation: []byte("install"), InstallSecretFile: path}).load(); c != nil || err == nil {
		t.Fatal("zero capability accepted")
	}
}
