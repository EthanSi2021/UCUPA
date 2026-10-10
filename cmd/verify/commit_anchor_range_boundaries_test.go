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

func TestReviewedCoreAnchorRangeSourceAndDependencies(t *testing.T) {
	files := []string{
		"anchor_range_store_test.go", "anchor_range_fixture_test.go", "anchor_range_fault_export_test.go",
	}
	for _, name := range files {
		file := "ucupa-core/internal/commit/" + name
		if got := checkSourceLocation(file); got != "" {
			t.Errorf("reviewed Anchor/Range source %s rejected: %s", file, got)
		}
	}
	for _, file := range []string{
		"ucupa-core/internal/commit/anchor_range_extra_test.go",
		"ucupa-core/internal/commit/anchor_range/fixture_test.go",
	} {
		if got := checkSourceLocation(file); got == "" {
			t.Errorf("unreviewed Anchor/Range source accepted: %s", file)
		}
	}

	stdlib := guardOracleStdlib()
	allowed := map[string][]string{
		"anchor_range_store_test.go": {
			"errors", "path/filepath", "reflect", "testing", "ucupa/shared/values",
			"ucupa/ucupa-core/contract", "ucupa/ucupa-core/internal/commit",
		},
		"anchor_range_fixture_test.go": {
			"crypto/rand", "errors", "fmt", "testing", "ucupa/shared/values",
			"ucupa/ucupa-core/contract", "ucupa/ucupa-core/internal/commit", "ucupa/ucupa-core/internal/element",
		},
		"anchor_range_fault_export_test.go": {
			"encoding/json", "errors", "fmt", "go.etcd.io/bbolt", "ucupa/ucupa-core/internal/store",
		},
	}
	for name, dependencies := range allowed {
		file := "ucupa-core/internal/commit/" + name
		for _, dependency := range dependencies {
			if got := checkDependency(file, dependency, "ucupa", true, stdlib); got != "" {
				t.Errorf("allowed Anchor/Range dependency %s -> %s: %s", file, dependency, got)
			}
		}
	}
	for _, tc := range []struct{ file, dependency string }{
		{"ucupa-core/internal/commit/anchor_range_store_test.go", "go.etcd.io/bbolt"},
		{"ucupa-core/internal/commit/anchor_range_store_test.go", "ucupa/ucupa-core/internal/store"},
		{"ucupa-core/internal/commit/anchor_range_fixture_test.go", "ucupa/ucupa-core/internal/store"},
		{"ucupa-core/internal/commit/anchor_range_fixture_test.go", "ucupa/ucupa-core/internal/token"},
		{"ucupa-core/internal/commit/anchor_range_store_test.go", "ucupa/ucupa-core/internal/query"},
		{"ucupa-core/internal/commit/anchor_range_fault_export_test.go", "ucupa/ucupa-core/internal/commit"},
		{"ucupa-core/internal/commit/anchor_range_fault_export_test.go", "ucupa/ucupa-core/internal/element"},
		{"ucupa-core/internal/commit/anchor_range_fault_export_test.go", "ucupa/ucupa-core/transport/grpc"},
	} {
		if got := checkDependency(tc.file, tc.dependency, "ucupa", true, stdlib); got == "" {
			t.Errorf("forbidden Anchor/Range dependency accepted: %s -> %s", tc.file, tc.dependency)
		}
	}
}

func TestAnchorRangeFaultExportSurfaceIsExact(t *testing.T) {
	const file = "ucupa-core/internal/commit/anchor_range_fault_export_test.go"
	root, err := currentRepositoryRoot(t)
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
				for _, specification := range declaration.Specs {
					switch specification := specification.(type) {
					case *ast.TypeSpec:
						if ast.IsExported(specification.Name.Name) {
							t.Fatalf("%s exports type %s", file, specification.Name.Name)
						}
					case *ast.ValueSpec:
						for _, name := range specification.Names {
							if ast.IsExported(name.Name) {
								t.Fatalf("%s exports value %s", file, name.Name)
							}
						}
					}
				}
			}
		case *ast.FuncDecl:
			if declaration.Recv != nil || declaration.Name == nil || declaration.Name.Name != "CorruptAnchorShrinkHistoryForTest" || found {
				t.Fatalf("%s contains an unexpected function or method", file)
			}
			if !ast.IsExported(declaration.Name.Name) ||
				!sameStringSlice(guardFieldTypes(declaration.Type.Params), []string{"string"}) ||
				!sameStringSlice(guardFieldTypes(declaration.Type.Results), []string{"error"}) {
				t.Fatalf("%s has an unexpected CorruptAnchorShrinkHistoryForTest signature", file)
			}
			found = true
		default:
			t.Fatalf("%s contains an unexpected declaration", file)
		}
	}
	if !found {
		t.Fatalf("%s does not contain CorruptAnchorShrinkHistoryForTest", file)
	}
}

func TestAnchorRangeFaultExportIsAbsentFromProductionBuild(t *testing.T) {
	goTool, err := exec.LookPath("go")
	if err != nil {
		t.Fatal(err)
	}
	root, err := currentRepositoryRoot(t)
	if err != nil {
		t.Fatal(err)
	}
	directory, err := os.MkdirTemp(filepath.Join(root, "ucupa-core"), ".anchor-range-fault-build-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(directory)
	writeFixture(t, directory, "main.go", "package main\n\nimport commitpkg \"ucupa/ucupa-core/internal/commit\"\n\nfunc main() {\n_ = commitpkg.CorruptAnchorShrinkHistoryForTest(\"path\")\n}\n")
	outputPath := filepath.Join(directory, "consumer.exe")
	command := exec.Command(goTool, "build", "-o", outputPath, filepath.Join(directory, "main.go"))
	command.Dir = root
	command.Env = commandEnvironment(false)
	output, err := command.CombinedOutput()
	if err == nil {
		t.Fatal("production build unexpectedly included the Anchor/Range fault export")
	}
	text := string(output)
	if !strings.Contains(text, "undefined: commitpkg.CorruptAnchorShrinkHistoryForTest") || strings.Contains(text, "use of internal package") {
		t.Fatalf("production build did not reject the Anchor/Range fault export as undefined: %v\n%s", err, text)
	}
}

func currentRepositoryRoot(t *testing.T) (string, error) {
	t.Helper()
	working, err := os.Getwd()
	if err != nil {
		return "", err
	}
	return repositoryRoot(working)
}
