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

func TestReviewedCoreCommitOracleFaultDependencies(t *testing.T) {
	stdlib := guardOracleStdlib()
	cases := []struct {
		file    string
		allowed []string
	}{
		{
			file: "ucupa-core/internal/commit/oracle_fault_export_test.go",
			allowed: []string{
				"fmt", "testing", "ucupa/shared/values", "ucupa/ucupa-core/contract",
				"ucupa/ucupa-core/internal/commit/binding", "ucupa/ucupa-core/internal/element/fact",
				"ucupa/ucupa-core/internal/geometry", "ucupa/ucupa-core/internal/token",
			},
		},
		{
			file: "ucupa-core/internal/commit/oracle_boundary_test.go",
			allowed: []string{
				"crypto/rand", "errors", "fmt", "testing", "ucupa/shared/values",
				"ucupa/ucupa-core/contract", "ucupa/ucupa-core/internal/commit",
				"ucupa/ucupa-core/internal/element", "ucupa/ucupa-core/internal/query/oracle",
			},
		},
	}
	for _, tc := range cases {
		if got := checkSourceLocation(tc.file); got != "" {
			t.Errorf("reviewed Core commit oracle test %s rejected: %s", tc.file, got)
		}
		for _, dependency := range tc.allowed {
			if got := checkDependency(tc.file, dependency, "ucupa", true, stdlib); got != "" {
				t.Errorf("allowed Core commit oracle dependency %s -> %s: %s", tc.file, dependency, got)
			}
		}
	}
	for _, tc := range []struct {
		file         string
		dependencies []string
	}{
		{
			file: "ucupa-core/internal/commit/oracle_fault_export_test.go",
			dependencies: []string{
				"ucupa/ucupa-core", "ucupa/ucupa-core/internal/commit", "ucupa/ucupa-core/internal/store",
				"ucupa/ucupa-core/internal/query/oracle", "ucupa/ucupa-core/internal/mount/fact",
				"go.etcd.io/bbolt", "ucupa/ucupa-core/transport/grpc", "ucupa/ucupa-core/sdk/go",
			},
		},
		{
			file: "ucupa-core/internal/commit/oracle_boundary_test.go",
			dependencies: []string{
				"ucupa/ucupa-core/internal/store", "ucupa/ucupa-core/internal/token",
				"ucupa/ucupa-core/internal/query", "ucupa/ucupa-core/internal/query/scan",
				"ucupa/ucupa-core/internal/geometry", "go.etcd.io/bbolt",
				"ucupa/ucupa-core/transport/grpc", "ucupa/ucupa-core/sdk/go",
			},
		},
	} {
		for _, dependency := range tc.dependencies {
			if got := checkDependency(tc.file, dependency, "ucupa", true, stdlib); got == "" {
				t.Errorf("forbidden Core commit oracle dependency accepted: %s -> %s", tc.file, dependency)
			}
		}
	}
	for _, file := range []string{
		"ucupa-core/internal/commit/oracle_extra_test.go",
		"ucupa-facet/internal/commit/oracle_boundary_test.go",
	} {
		if got := checkSourceLocation(file); got == "" {
			t.Errorf("unreviewed Core commit oracle source accepted: %s", file)
		}
	}
}

func TestCoreOracleFaultExportSurfaceIsExact(t *testing.T) {
	const file = "ucupa-core/internal/commit/oracle_fault_export_test.go"
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
	want := map[string]struct {
		params  []string
		results []string
	}{
		"InjectOracleCorruptionForTest": {
			params: []string{"*Owner", "string"}, results: []string{"error"},
		},
		"BuildOracleEdgeFixtureForTest": {
			params: []string{"*testing.T", "*Owner"}, results: []string{"values.ElementID", "values.ElementID"},
		},
		"AddOracleOverflowForTest": {
			params: []string{"*testing.T", "*Owner"}, results: []string{"values.ElementID"},
		},
	}
	found := make(map[string]bool)
	for _, declaration := range parsed.Decls {
		switch declaration := declaration.(type) {
		case *ast.GenDecl:
			if declaration.Tok == token.IMPORT {
				continue
			}
			for _, specification := range declaration.Specs {
				if typeSpec, ok := specification.(*ast.TypeSpec); ok && ast.IsExported(typeSpec.Name.Name) {
					t.Fatalf("%s exports unexpected type %s", file, typeSpec.Name.Name)
				}
				if valueSpec, ok := specification.(*ast.ValueSpec); ok {
					for _, name := range valueSpec.Names {
						if ast.IsExported(name.Name) {
							t.Fatalf("%s exports unexpected value %s", file, name.Name)
						}
					}
				}
			}
		case *ast.FuncDecl:
			if declaration.Recv != nil || declaration.Name == nil {
				if declaration.Recv != nil && declaration.Name != nil && ast.IsExported(declaration.Name.Name) {
					t.Fatalf("%s exports unexpected method %s", file, declaration.Name.Name)
				}
				continue
			}
			if !ast.IsExported(declaration.Name.Name) {
				continue
			}
			expected, ok := want[declaration.Name.Name]
			if !ok || found[declaration.Name.Name] {
				t.Fatalf("%s contains unexpected or duplicate exported function %s", file, declaration.Name.Name)
			}
			if !sameStringSlice(guardFieldTypes(declaration.Type.Params), expected.params) ||
				!sameStringSlice(guardFieldTypes(declaration.Type.Results), expected.results) {
				t.Fatalf("%s function %s has an unexpected signature", file, declaration.Name.Name)
			}
			found[declaration.Name.Name] = true
		}
	}
	if len(found) != len(want) {
		t.Fatalf("%s exported functions=%v, want exactly %v", file, found, want)
	}
}

func TestCoreOracleFaultExportsAreAbsentFromProductionBuild(t *testing.T) {
	goTool, err := exec.LookPath("go")
	if err != nil {
		t.Fatal(err)
	}
	root, err := repositoryRootForGuardOracle(t)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name string
		body string
	}{
		{name: "InjectOracleCorruptionForTest", body: "_ = commitpkg.InjectOracleCorruptionForTest(owner, fault)"},
		{name: "BuildOracleEdgeFixtureForTest", body: "_, _ = commitpkg.BuildOracleEdgeFixtureForTest(test, owner)"},
		{name: "AddOracleOverflowForTest", body: "_ = commitpkg.AddOracleOverflowForTest(test, owner)"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			directory, err := os.MkdirTemp(filepath.Join(root, "ucupa-core"), ".oracle-fault-build-")
			if err != nil {
				t.Fatal(err)
			}
			defer os.RemoveAll(directory)
			writeFixture(t, directory, "main.go", "package main\n\nimport (\n\tcommitpkg \"ucupa/ucupa-core/internal/commit\"\n\t\"ucupa/shared/values\"\n\t\"testing\"\n)\n\nfunc main() {\nvar owner *commitpkg.Owner\nvar test *testing.T\nvar element values.ElementID\nvar fault string\n_ = owner\n_ = test\n_ = element\n_ = fault\n"+tc.body+"\n}\n")
			outputPath := filepath.Join(directory, "consumer.exe")
			command := exec.Command(goTool, "build", "-o", outputPath, filepath.Join(directory, "main.go"))
			command.Dir = root
			command.Env = commandEnvironment(false)
			output, err := command.CombinedOutput()
			if err == nil {
				t.Fatal("production build unexpectedly included an oracle fault export")
			}
			text := string(output)
			if !strings.Contains(text, "undefined: commitpkg."+tc.name) || strings.Contains(text, "use of internal package") {
				t.Fatalf("production build did not reject %s as an undefined test-only symbol: %v\n%s", tc.name, err, text)
			}
		})
	}
}
