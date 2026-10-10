package main

import (
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"testing"
)

func TestReviewedMountControlSourcesAndNoUnknownFiles(t *testing.T) {
	want := []string{
		"ucupa-core/internal/commit/mount_control_export_test.go",
		"ucupa-core/internal/commit/mount_control_fixture_test.go",
		"ucupa-core/internal/commit/mount_control_test.go",
		"ucupa-core/internal/commit/mount_interleaving_test.go",
	}
	for _, file := range want {
		if !reviewedCommitSourceFile(path.Base(file)) {
			t.Errorf("Mount control source is missing from exact commit registration: %s", file)
		}
		if got := checkSourceLocation(file); got != "" {
			t.Errorf("reviewed Mount control source %s rejected: %s", file, got)
		}
	}
	for _, file := range []string{
		"ucupa-core/internal/commit/mount_control_extra_test.go",
		"ucupa-core/internal/commit/mount_control/nested_test.go",
		"ucupa-core/internal/commit/mount_interleaving_extra.go",
	} {
		if got := checkSourceLocation(file); got == "" {
			t.Errorf("unreviewed Mount control source accepted: %s", file)
		}
	}
}

func TestReviewedMountControlDependenciesArePerFile(t *testing.T) {
	stdlib := guardOracleStdlib()
	stdlib["sync/atomic"] = true
	cases := []struct {
		file string
		want []string
	}{
		{
			file: "ucupa-core/internal/commit/mount_control_export_test.go",
			want: []string{"ucupa-core/contract"},
		},
		{
			file: "ucupa-core/internal/commit/mount_control_fixture_test.go",
			want: []string{"context", "crypto/rand", "sync/atomic", "testing", "shared/values", "ucupa-core/contract", "ucupa-core/internal/commit", "ucupa-core/internal/element", "ucupa-core/internal/mount"},
		},
		{
			file: "ucupa-core/internal/commit/mount_control_test.go",
			want: []string{"context", "errors", "reflect", "testing", "shared/values", "ucupa-core/contract", "ucupa-core/internal/commit", "ucupa-core/internal/mount/fact"},
		},
		{
			file: "ucupa-core/internal/commit/mount_interleaving_test.go",
			want: []string{"context", "errors", "reflect", "sync/atomic", "testing", "time", "shared/values", "ucupa-core/contract", "ucupa-core/internal/commit"},
		},
	}
	for _, tc := range cases {
		for _, dependency := range tc.want {
			if got := checkDependency(tc.file, dependency, "ucupa", true, stdlib); got != "" {
				t.Errorf("allowed Mount control dependency %s -> %s: %s", tc.file, dependency, got)
			}
		}
	}
	for _, tc := range []struct {
		file string
		bad  []string
	}{
		{
			file: "ucupa-core/internal/commit/mount_control_export_test.go",
			bad:  []string{"ucupa-core", "ucupa-core/internal/commit", "ucupa-core/internal/mount", "go.etcd.io/bbolt"},
		},
		{
			file: "ucupa-core/internal/commit/mount_control_fixture_test.go",
			bad:  []string{"ucupa-core/internal/store", "ucupa-core/internal/token", "ucupa-core/internal/query", "ucupa-core/internal/facet", "go.etcd.io/bbolt"},
		},
		{
			file: "ucupa-core/internal/commit/mount_control_test.go",
			bad:  []string{"ucupa-core/internal/element", "ucupa-core/internal/mount", "ucupa-core/internal/store", "ucupa-core/internal/query", "go.etcd.io/bbolt"},
		},
		{
			file: "ucupa-core/internal/commit/mount_interleaving_test.go",
			bad:  []string{"ucupa-core/internal/element", "ucupa-core/internal/mount", "ucupa-core/internal/mount/fact", "ucupa-core/internal/store", "go.etcd.io/bbolt"},
		},
	} {
		for _, dependency := range tc.bad {
			if got := checkDependency(tc.file, dependency, "ucupa", true, stdlib); got == "" {
				t.Errorf("forbidden Mount control dependency accepted: %s -> %s", tc.file, dependency)
			}
		}
	}
}

func TestMountControlTestOnlyExportsAreExact(t *testing.T) {
	root, err := currentRepositoryRoot(t)
	if err != nil {
		t.Fatal(err)
	}
	assertQueryExportSurface(t, root, "ucupa-core/internal/commit/mount_control_export_test.go", map[string]queryFunctionSpec{
		"MountControlEventCountForTest":     {params: []string{"*Owner"}, results: []string{"int"}},
		"CheckUnrelatedMountHistoryForTest": {params: []string{"*Owner", "bool"}, results: []string{"error", "error"}},
	})
}

func TestMountControlTestOnlyExportsAreUndefinedInProduction(t *testing.T) {
	root, err := currentRepositoryRoot(t)
	if err != nil {
		t.Fatal(err)
	}
	goTool, err := exec.LookPath("go")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name string
		want string
	}{
		{"event count", "MountControlEventCountForTest"},
		{"unrelated history", "CheckUnrelatedMountHistoryForTest"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			directory, err := os.MkdirTemp(filepath.Join(root, "ucupa-core"), ".mount-control-build-")
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = os.RemoveAll(directory) })
			source := "package main\nimport commitpkg \"ucupa/ucupa-core/internal/commit\"\nvar _ = commitpkg." + tc.want + "\nfunc main() {}\n"
			writeFixture(t, directory, "main.go", source)
			outputPath := filepath.Join(directory, "consumer.exe")
			command := exec.Command(goTool, "build", "-o", outputPath, filepath.Join(directory, "main.go"))
			command.Dir = root
			command.Env = commandEnvironment(false)
			output, err := command.CombinedOutput()
			text := string(output)
			if err == nil || !strings.Contains(text, "undefined: commitpkg."+tc.want) || strings.Contains(text, "use of internal package") {
				t.Fatalf("production build did not reject Mount control export %s as undefined: %v\n%s", tc.want, err, output)
			}
		})
	}
}
