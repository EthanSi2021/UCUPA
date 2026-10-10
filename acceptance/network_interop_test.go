package acceptance_test

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestNetworkIndependentImplementationCorpus(t *testing.T) {
	python := os.Getenv("UCUPA_INTEROP_PYTHON")
	if python == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			t.Fatal(err)
		}
		python = filepath.Join(home, ".cache", "ucupa-interop-python", "bin", "python")
		if runtime.GOOS == "windows" {
			python = filepath.Join(home, ".cache", "ucupa-interop-python", "Scripts", "python.exe")
		}
	}
	if _, err := os.Stat(python); err != nil {
		t.Fatalf("independent interop Python missing: set UCUPA_INTEROP_PYTHON after following testdata/interop/README.md: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	script := filepath.Join("testdata", "interop", "run.py")
	check := exec.CommandContext(ctx, python, script, "--check")
	if out, err := check.CombinedOutput(); err != nil {
		t.Fatalf("independent runtime check: %v\n%s", err, out)
	}
	dir := t.TempDir()
	descriptor := filepath.Join(dir, "public.pb")
	version := exec.CommandContext(ctx, "protoc", "--version")
	if out, err := version.CombinedOutput(); err != nil || strings.TrimSpace(string(out)) != "libprotoc 34.1" {
		t.Fatalf("independent corpus requires pinned protoc 34.1: %v %s", err, out)
	}
	gen := exec.CommandContext(ctx, "protoc", "--include_imports", "--descriptor_set_out="+descriptor, "shared/api/network.proto", "ucupa-core/api/core.proto", "ucupa-facet/api/facet.proto")
	gen.Dir = ".."
	if out, err := gen.CombinedOutput(); err != nil {
		t.Fatalf("public descriptor generation: %v\n%s", err, out)
	}
	config := interopProducts(t)
	config["descriptor"] = descriptor
	raw, err := json.Marshal(config)
	if err != nil {
		t.Fatal(err)
	}
	configuration := filepath.Join(dir, "client.json")
	writeNetworkFile(t, configuration, raw)
	cmd := exec.CommandContext(ctx, python, script, configuration)
	cmd.Env = append(os.Environ(), "PYTHONDONTWRITEBYTECODE=1")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("independent public corpus: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "INDEPENDENT_CORPUS_PASS") {
		t.Fatalf("missing interop proof: %s", out)
	}
	t.Log(strings.TrimSpace(string(out)))
}
