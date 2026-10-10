package main

import (
	"encoding/json"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestPersistedGOFLAGSDoesNotNarrowValidation(t *testing.T) {
	goTool, err := findGo()
	if err != nil {
		t.Fatal(err)
	}
	root := fixture(t)
	// Only this temporary GOENV file is used; never change the user's Go config.
	writeFixture(t, root, "test-go-env", "GOFLAGS=-run=NoSuchTest\n")
	t.Setenv("GOENV", filepath.Join(root, "test-go-env"))
	t.Setenv("GOFLAGS", "")
	protected := commandEnvironment(false)
	empty := append([]string(nil), protected...)
	for i, item := range empty {
		key, _, _ := strings.Cut(item, "=")
		if strings.EqualFold(key, "GOFLAGS") {
			empty[i] = "GOFLAGS="
		}
	}
	run := func(env []string, args ...string) ([]byte, error) {
		cmd := exec.Command(goTool, args...)
		cmd.Dir, cmd.Env = root, env
		return cmd.CombinedOutput()
	}
	for _, tc := range []struct {
		name string
		env  []string
		want string
	}{
		{"empty falls back to persisted filter", empty, "-run=NoSuchTest"},
		{"space overrides with zero flags", protected, " "},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, err := run(tc.env, "env", "-json", "GOFLAGS")
			if err != nil {
				t.Fatalf("go env GOFLAGS: %v\n%s", err, out)
			}
			var got map[string]string
			if err := json.Unmarshal(out, &got); err != nil {
				t.Fatal(err)
			}
			if got["GOFLAGS"] != tc.want {
				t.Fatalf("GOFLAGS=%q, want %q", got["GOFLAGS"], tc.want)
			}
		})
	}
	// A deliberately failing child test proves the gate really executes tests.
	// It is a standalone temporary package, never a recursive verifier run.
	writeFixture(t, root, "sentinel_test.go", "package sentinel\n\nimport \"testing\"\n\nfunc TestMustRun(t *testing.T) { t.Fatal(\"VERIFY_TEST_EXECUTED\") }\n")
	if out, err := run(empty, "test", "./...", "-count=1"); err != nil || strings.Contains(string(out), "VERIFY_TEST_EXECUTED") {
		t.Fatalf("empty GOFLAGS should demonstrate the skipped-test defect: %v\n%s", err, out)
	}
	if out, err := run(protected, "test", "./...", "-count=1"); err == nil || !strings.Contains(string(out), "VERIFY_TEST_EXECUTED") {
		t.Fatalf("protected environment did not execute the child test: %v\n%s", err, out)
	}
}
