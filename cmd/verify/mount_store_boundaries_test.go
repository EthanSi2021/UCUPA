package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"testing"
)

func TestReviewedMountStoreSourcesAndNoUnknownFiles(t *testing.T) {
	want := []string{
		"ucupa-core/internal/commit/mount_store_atomicity_test.go",
		"ucupa-core/internal/commit/mount_store_corruption_export_test.go",
		"ucupa-core/internal/commit/mount_store_corruption_test.go",
		"ucupa-core/internal/commit/mount_store_fault_export_test.go",
		"ucupa-core/internal/commit/mount_store_fixture_test.go",
		"ucupa-core/internal/commit/mount_store_history_export_test.go",
		"ucupa-core/internal/commit/mount_store_outcomes_test.go",
	}
	for _, file := range want {
		if !reviewedCommitSourceFile(path.Base(file)) {
			t.Errorf("Mount Store source is missing from the exact commit registration: %s", file)
		}
		if got := checkSourceLocation(file); got != "" {
			t.Errorf("reviewed Mount Store source %s rejected: %s", file, got)
		}
	}
	for _, file := range []string{
		"ucupa-core/internal/commit/mount_store_extra_test.go",
		"ucupa-core/internal/commit/mount_store/nested_test.go",
		"ucupa-core/internal/commit/mount_store_fixture_extra.go",
	} {
		if got := checkSourceLocation(file); got == "" {
			t.Errorf("unreviewed Mount Store source accepted: %s", file)
		}
	}
	if !reviewedCommitPackage("ucupa-core", "ucupa-core/internal/commit") ||
		reviewedCommitPackage("ucupa-facet", "ucupa-core/internal/commit") ||
		reviewedCommitPackage("ucupa-core", "ucupa-core/internal/commit/mount_store") {
		t.Fatal("Mount Store tests must remain in the single reviewed Core commit package")
	}
}

func TestReviewedMountStoreDependenciesArePerFile(t *testing.T) {
	stdlib := guardOracleStdlib()
	stdlib["sync/atomic"] = true
	cases := []struct {
		file string
		want []string
	}{
		{
			file: "ucupa-core/internal/commit/mount_store_atomicity_test.go",
			want: []string{
				"context", "errors", "fmt", "reflect", "testing",
				"ucupa-core/contract", "ucupa-core/internal/commit", "ucupa-core/internal/mount/fact",
			},
		},
		{
			file: "ucupa-core/internal/commit/mount_store_corruption_test.go",
			want: []string{
				"bytes", "context", "errors", "os", "testing",
				"ucupa-core/contract", "ucupa-core/internal/commit",
			},
		},
		{
			file: "ucupa-core/internal/commit/mount_store_outcomes_test.go",
			want: []string{
				"context", "errors", "fmt", "reflect", "testing",
				"ucupa-core/contract", "ucupa-core/internal/commit",
			},
		},
		{
			file: "ucupa-core/internal/commit/mount_store_fixture_test.go",
			want: []string{
				"context", "path/filepath", "testing", "shared/values",
				"ucupa-core/contract", "ucupa-core/internal/commit", "ucupa-core/internal/element", "ucupa-core/internal/mount",
			},
		},
		{
			file: "ucupa-core/internal/commit/mount_store_fault_export_test.go",
			want: []string{"sync/atomic", "ucupa-core/contract"},
		},
		{
			file: "ucupa-core/internal/commit/mount_store_corruption_export_test.go",
			want: []string{"encoding/json", "testing", "go.etcd.io/bbolt", "ucupa-core/internal/store"},
		},
		{
			file: "ucupa-core/internal/commit/mount_store_history_export_test.go",
			want: []string{
				"errors", "fmt", "go.etcd.io/bbolt", "shared/values", "ucupa-core/contract", "ucupa-core/internal/mount/fact",
			},
		},
	}
	for _, tc := range cases {
		for _, dependency := range tc.want {
			if got := checkDependency(tc.file, dependency, "ucupa", true, stdlib); got != "" {
				t.Errorf("allowed Mount Store dependency %s -> %s: %s", tc.file, dependency, got)
			}
		}
	}
	for _, tc := range []struct {
		file string
		bad  []string
	}{
		{
			file: "ucupa-core/internal/commit/mount_store_atomicity_test.go",
			bad:  []string{"ucupa-core/internal/element", "ucupa-core/internal/mount", "ucupa-core/internal/store", "go.etcd.io/bbolt"},
		},
		{
			file: "ucupa-core/internal/commit/mount_store_corruption_test.go",
			bad:  []string{"ucupa-core/internal/element", "ucupa-core/internal/mount", "ucupa-core/internal/store", "go.etcd.io/bbolt"},
		},
		{
			file: "ucupa-core/internal/commit/mount_store_outcomes_test.go",
			bad:  []string{"ucupa-core/internal/element", "ucupa-core/internal/mount", "ucupa-core/internal/mount/fact", "go.etcd.io/bbolt"},
		},
		{
			file: "ucupa-core/internal/commit/mount_store_fixture_test.go",
			bad:  []string{"ucupa-core/internal/query", "ucupa-core/internal/store", "ucupa-core/internal/token", "go.etcd.io/bbolt"},
		},
		{
			file: "ucupa-core/internal/commit/mount_store_fault_export_test.go",
			bad:  []string{"ucupa-core", "ucupa-core/internal/commit", "ucupa-core/internal/store", "go.etcd.io/bbolt"},
		},
		{
			file: "ucupa-core/internal/commit/mount_store_corruption_export_test.go",
			bad:  []string{"ucupa-core/contract", "ucupa-core/internal/commit", "ucupa-core/internal/element", "ucupa-core/internal/mount/fact", "ucupa/shared/values"},
		},
		{
			file: "ucupa-core/internal/commit/mount_store_history_export_test.go",
			bad:  []string{"ucupa-core", "ucupa-core/internal/commit", "ucupa-core/internal/element", "ucupa-core/internal/mount", "ucupa-core/internal/store", "ucupa-core/internal/token"},
		},
	} {
		for _, dependency := range tc.bad {
			if got := checkDependency(tc.file, dependency, "ucupa", true, stdlib); got == "" {
				t.Errorf("forbidden Mount Store dependency accepted: %s -> %s", tc.file, dependency)
			}
		}
	}
}

func TestMountStoreTestOnlyExportSurfacesAreExact(t *testing.T) {
	root, err := currentRepositoryRoot(t)
	if err != nil {
		t.Fatal(err)
	}
	assertMountStoreFaultSurface(t, root)
	assertMountStoreFunctionSurface(t, root, "ucupa-core/internal/commit/mount_store_corruption_export_test.go", map[string]mountStoreFunctionSpec{
		"CorruptMountJournalForTest": {params: []string{"*testing.T", "string", "string"}},
	})
	assertMountStoreFunctionSurface(t, root, "ucupa-core/internal/commit/mount_store_history_export_test.go", map[string]mountStoreFunctionSpec{
		"ValidateChangedMountOutcomeForTest": {params: []string{"*Owner", "contract.OperationID"}, results: []string{"error"}},
		"CheckMountHistoryFaultForTest":      {params: []string{"*Owner", "values.ElementID", "string"}, results: []string{"bool", "error"}},
		"InjectThirdMountStoreResultForTest": {params: []string{"*Owner", "values.PrincipalRef", "values.ElementID", "contract.MountKey"}},
	})
}

func TestMountStoreTestOnlyExportsAreUndefinedInProduction(t *testing.T) {
	root, err := currentRepositoryRoot(t)
	if err != nil {
		t.Fatal(err)
	}
	goTool, err := exec.LookPath("go")
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name string
		src  string
		want string
	}{
		{
			name: "failure type",
			src:  "package main\nimport commitpkg \"ucupa/ucupa-core/internal/commit\"\nvar _ *commitpkg.MountStoreFailureForTest\nfunc main() {}\n",
			want: "undefined: commitpkg.MountStoreFailureForTest",
		},
		{
			name: "failure constructor",
			src:  "package main\nimport commitpkg \"ucupa/ucupa-core/internal/commit\"\nvar _ = commitpkg.InjectMountStoreFailureForTest\nfunc main() {}\n",
			want: "undefined: commitpkg.InjectMountStoreFailureForTest",
		},
		{
			name: "address retention",
			src:  "package main\nimport commitpkg \"ucupa/ucupa-core/internal/commit\"\nvar _ = commitpkg.MountAddressRetainedForTest\nfunc main() {}\n",
			want: "undefined: commitpkg.MountAddressRetainedForTest",
		},
		{
			name: "journal corruption",
			src:  "package main\nimport commitpkg \"ucupa/ucupa-core/internal/commit\"\nvar _ = commitpkg.CorruptMountJournalForTest\nfunc main() {}\n",
			want: "undefined: commitpkg.CorruptMountJournalForTest",
		},
		{
			name: "changed outcome",
			src:  "package main\nimport commitpkg \"ucupa/ucupa-core/internal/commit\"\nvar _ = commitpkg.ValidateChangedMountOutcomeForTest\nfunc main() {}\n",
			want: "undefined: commitpkg.ValidateChangedMountOutcomeForTest",
		},
		{
			name: "history fault",
			src:  "package main\nimport commitpkg \"ucupa/ucupa-core/internal/commit\"\nvar _ = commitpkg.CheckMountHistoryFaultForTest\nfunc main() {}\n",
			want: "undefined: commitpkg.CheckMountHistoryFaultForTest",
		},
		{
			name: "third result",
			src:  "package main\nimport commitpkg \"ucupa/ucupa-core/internal/commit\"\nvar _ = commitpkg.InjectThirdMountStoreResultForTest\nfunc main() {}\n",
			want: "undefined: commitpkg.InjectThirdMountStoreResultForTest",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			directory, err := os.MkdirTemp(filepath.Join(root, "ucupa-core"), ".mount-store-build-")
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = os.RemoveAll(directory) })
			writeFixture(t, directory, "main.go", tc.src)
			outputPath := filepath.Join(directory, "consumer.exe")
			command := exec.Command(goTool, "build", "-o", outputPath, filepath.Join(directory, "main.go"))
			command.Dir = root
			command.Env = commandEnvironment(false)
			output, err := command.CombinedOutput()
			if err == nil || !strings.Contains(string(output), tc.want) || strings.Contains(string(output), "use of internal package") {
				t.Fatalf("production build did not reject test-only Mount Store export as %q: %v\n%s", tc.want, err, output)
			}
		})
	}
}

type mountStoreFunctionSpec struct {
	params, results []string
}

func assertMountStoreFaultSurface(t *testing.T, root string) {
	t.Helper()
	const file = "ucupa-core/internal/commit/mount_store_fault_export_test.go"
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(file)))
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := parser.ParseFile(token.NewFileSet(), file, data, parser.ParseComments)
	if err != nil {
		t.Fatal(err)
	}
	foundType, foundCalls := false, false
	wantFunctions := map[string]mountStoreFunctionSpec{
		"InjectMountStoreFailureForTest": {params: []string{"*Owner", "bool", "int", "error"}, results: []string{"*MountStoreFailureForTest"}},
		"MountAddressRetainedForTest":    {params: []string{"*Owner", "contract.OperationID"}, results: []string{"bool"}},
	}
	foundFunctions := map[string]bool{}
	for _, declaration := range parsed.Decls {
		switch declaration := declaration.(type) {
		case *ast.GenDecl:
			if declaration.Tok == token.IMPORT {
				continue
			}
			for _, specification := range declaration.Specs {
				switch specification := specification.(type) {
				case *ast.TypeSpec:
					if !ast.IsExported(specification.Name.Name) {
						continue
					}
					if specification.Name.Name != "MountStoreFailureForTest" || foundType {
						t.Fatalf("%s exports an unexpected or duplicate type", file)
					}
					structure, ok := specification.Type.(*ast.StructType)
					if !ok || len(structure.Fields.List) != 1 {
						t.Fatalf("%s has changed MountStoreFailureForTest representation", file)
					}
					field := structure.Fields.List[0]
					if len(field.Names) != 1 || field.Names[0].Name != "calls" || ast.IsExported(field.Names[0].Name) || guardASTType(field.Type) != "atomic.Int64" {
						t.Fatalf("%s exposes or changes the failure counter", file)
					}
					foundType = true
				case *ast.ValueSpec:
					for _, name := range specification.Names {
						if ast.IsExported(name.Name) {
							t.Fatalf("%s exports value %s", file, name.Name)
						}
					}
				}
			}
		case *ast.FuncDecl:
			if declaration.Name == nil || !ast.IsExported(declaration.Name.Name) {
				continue
			}
			if declaration.Recv != nil {
				if declaration.Name.Name != "Calls" || foundCalls || len(declaration.Recv.List) != 1 || guardASTType(declaration.Recv.List[0].Type) != "*MountStoreFailureForTest" {
					t.Fatalf("%s exports an unexpected method", file)
				}
				if !sameStringSlice(expandedASTFieldTypes(declaration.Type.Params), nil) ||
					!sameStringSlice(expandedASTFieldTypes(declaration.Type.Results), []string{"int"}) {
					t.Fatalf("%s Calls signature changed", file)
				}
				foundCalls = true
				continue
			}
			expected, ok := wantFunctions[declaration.Name.Name]
			if !ok || foundFunctions[declaration.Name.Name] {
				t.Fatalf("%s exports an unexpected function %s", file, declaration.Name.Name)
			}
			if !sameStringSlice(expandedASTFieldTypes(declaration.Type.Params), expected.params) ||
				!sameStringSlice(expandedASTFieldTypes(declaration.Type.Results), expected.results) {
				t.Fatalf("%s %s signature changed", file, declaration.Name.Name)
			}
			foundFunctions[declaration.Name.Name] = true
		}
	}
	if !foundType || !foundCalls || len(foundFunctions) != len(wantFunctions) {
		t.Fatalf("%s exported surface changed: type=%v Calls=%v functions=%v", file, foundType, foundCalls, foundFunctions)
	}
}

func assertMountStoreFunctionSurface(t *testing.T, root, file string, want map[string]mountStoreFunctionSpec) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(file)))
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := parser.ParseFile(token.NewFileSet(), file, data, parser.ParseComments)
	if err != nil {
		t.Fatal(err)
	}
	found := map[string]bool{}
	for _, declaration := range parsed.Decls {
		switch declaration := declaration.(type) {
		case *ast.GenDecl:
			if declaration.Tok == token.IMPORT {
				continue
			}
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
		case *ast.FuncDecl:
			if declaration.Name == nil || !ast.IsExported(declaration.Name.Name) {
				continue
			}
			if declaration.Recv != nil {
				t.Fatalf("%s exports method %s", file, declaration.Name.Name)
			}
			expected, ok := want[declaration.Name.Name]
			if !ok || found[declaration.Name.Name] {
				t.Fatalf("%s exports unexpected function %s", file, declaration.Name.Name)
			}
			if !sameStringSlice(expandedASTFieldTypes(declaration.Type.Params), expected.params) ||
				!sameStringSlice(expandedASTFieldTypes(declaration.Type.Results), expected.results) {
				t.Fatalf("%s %s signature changed", file, declaration.Name.Name)
			}
			found[declaration.Name.Name] = true
		}
	}
	if len(found) != len(want) {
		t.Fatalf("%s exported surface changed: got=%v want=%v", file, found, want)
	}
}
