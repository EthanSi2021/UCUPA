package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestReviewedCoreCommitProjectionDependencies(t *testing.T) {
	stdlib := guardOracleStdlib()
	cases := []struct {
		file    string
		allowed []string
	}{
		{
			file: "ucupa-core/internal/commit/projection_fault_export_test.go",
			allowed: []string{
				"fmt", "testing", "ucupa/shared/values",
			},
		},
		{
			file: "ucupa-core/internal/commit/projection_authorization_test.go",
			allowed: []string{
				"crypto/rand", "errors", "fmt", "reflect", "sync", "testing", "time",
				"ucupa/shared/values", "ucupa/ucupa-core/contract", "ucupa/ucupa-core/internal/commit",
				"ucupa/ucupa-core/internal/element",
			},
		},
	}
	for _, tc := range cases {
		if got := checkSourceLocation(tc.file); got != "" {
			t.Errorf("reviewed Core projection test %s rejected: %s", tc.file, got)
		}
		for _, dependency := range tc.allowed {
			if got := checkDependency(tc.file, dependency, "ucupa", true, stdlib); got != "" {
				t.Errorf("allowed Core projection dependency %s -> %s: %s", tc.file, dependency, got)
			}
		}
	}
	for _, tc := range []struct {
		file         string
		dependencies []string
	}{
		{
			file: "ucupa-core/internal/commit/projection_fault_export_test.go",
			dependencies: []string{
				"ucupa/ucupa-core", "ucupa/ucupa-core/internal/commit", "ucupa/ucupa-core/internal/element",
				"ucupa/ucupa-core/internal/store", "ucupa/ucupa-core/internal/query", "ucupa/ucupa-core/internal/token",
				"ucupa/ucupa-core/transport/grpc", "ucupa/ucupa-core/sdk/go", "go.etcd.io/bbolt",
			},
		},
		{
			file: "ucupa-core/internal/commit/projection_authorization_test.go",
			dependencies: []string{
				"ucupa/ucupa-core/internal/store", "ucupa/ucupa-core/internal/query/oracle",
				"ucupa/ucupa-core/internal/token", "ucupa/ucupa-core/internal/geometry",
				"ucupa/ucupa-core/internal/mount/fact", "go.etcd.io/bbolt",
				"ucupa/ucupa-core/transport/grpc", "ucupa/ucupa-core/sdk/go",
			},
		},
	} {
		for _, dependency := range tc.dependencies {
			if got := checkDependency(tc.file, dependency, "ucupa", true, stdlib); got == "" {
				t.Errorf("forbidden Core projection dependency accepted: %s -> %s", tc.file, dependency)
			}
		}
	}
	for _, file := range []string{
		"ucupa-core/internal/commit/projection_extra_test.go",
		"ucupa-core/internal/commit/projection/extra_test.go",
		"ucupa-facet/internal/commit/projection_authorization_test.go",
	} {
		if got := checkSourceLocation(file); got == "" {
			t.Errorf("unreviewed Core projection source accepted: %s", file)
		}
	}
}

func TestCoreProjectionFaultExportSurfaceIsExact(t *testing.T) {
	const file = "ucupa-core/internal/commit/projection_fault_export_test.go"
	root, err := repositoryRootForGuardOracle(t)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(file)))
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := parser.ParseFile(token.NewFileSet(), file, data, parser.ParseComments)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, declaration := range parsed.Decls {
		switch declaration := declaration.(type) {
		case *ast.GenDecl:
			if declaration.Tok != token.IMPORT {
				t.Fatalf("%s contains an unexpected non-import declaration", file)
			}
		case *ast.FuncDecl:
			if found || declaration.Recv != nil || declaration.Name == nil || declaration.Name.Name != "CorruptProjectionParentForTest" {
				t.Fatalf("%s contains an unexpected function or method", file)
			}
			if !sameStringSlice(guardFieldTypes(declaration.Type.Params), []string{"*Owner", "string"}) ||
				!sameStringSlice(guardFieldTypes(declaration.Type.Results), []string{"error"}) {
				t.Fatalf("%s has an unexpected CorruptProjectionParentForTest signature", file)
			}
			found = true
		default:
			t.Fatalf("%s contains an unexpected declaration", file)
		}
	}
	if !found {
		t.Fatalf("%s does not contain CorruptProjectionParentForTest", file)
	}
}

func TestCoreProjectionFaultExportIsAbsentFromProductionBuild(t *testing.T) {
	goTool, err := exec.LookPath("go")
	if err != nil {
		t.Fatal(err)
	}
	root, err := repositoryRootForGuardOracle(t)
	if err != nil {
		t.Fatal(err)
	}
	directory, err := os.MkdirTemp(filepath.Join(root, "ucupa-core"), ".projection-fault-build-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(directory)
	writeFixture(t, directory, "main.go", "package main\n\nimport commitpkg \"ucupa/ucupa-core/internal/commit\"\n\nfunc main() {\nvar owner *commitpkg.Owner\n_ = commitpkg.CorruptProjectionParentForTest(owner, \"missing parent\")\n}\n")
	outputPath := filepath.Join(directory, "consumer.exe")
	command := exec.Command(goTool, "build", "-o", outputPath, filepath.Join(directory, "main.go"))
	command.Dir = root
	command.Env = commandEnvironment(false)
	output, err := command.CombinedOutput()
	if err == nil {
		t.Fatal("production build unexpectedly included the projection fault export")
	}
	text := string(output)
	if !strings.Contains(text, "undefined: commitpkg.CorruptProjectionParentForTest") || strings.Contains(text, "use of internal package") {
		t.Fatalf("production build did not reject the projection fault export as undefined: %v\n%s", err, text)
	}
}
