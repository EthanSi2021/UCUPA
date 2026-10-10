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

func TestReviewedQueryCommitSourcesAndNoUnknownFiles(t *testing.T) {
	want := []string{
		"ucupa-core/internal/commit/query_conformance_test.go",
		"ucupa-core/internal/commit/query_corruption_test.go",
		"ucupa-core/internal/commit/query_durability_test.go",
		"ucupa-core/internal/commit/query_fault_export_test.go",
		"ucupa-core/internal/commit/query_fixture_export_test.go",
		"ucupa-core/internal/commit/query_fixture_test.go",
		"ucupa-core/internal/commit/query_limits_test.go",
	}
	for _, file := range want {
		if !reviewedCommitSourceFile(path.Base(file)) {
			t.Errorf("query conformance source is missing from exact commit registration: %s", file)
		}
		if got := checkSourceLocation(file); got != "" {
			t.Errorf("reviewed query conformance source %s rejected: %s", file, got)
		}
	}
	for _, file := range []string{
		"ucupa-core/internal/commit/query_extra_test.go",
		"ucupa-core/internal/commit/query/nested_test.go",
		"ucupa-core/internal/commit/query_fixture_extra.go",
	} {
		if got := checkSourceLocation(file); got == "" {
			t.Errorf("unreviewed query conformance source accepted: %s", file)
		}
	}
}

func TestReviewedQueryCommitDependenciesArePerFile(t *testing.T) {
	stdlib := guardOracleStdlib()
	cases := []struct {
		file string
		want []string
	}{
		{
			file: "ucupa-core/internal/commit/query_conformance_test.go",
			want: []string{"errors", "math", "testing", "shared/values", "ucupa-core/contract", "ucupa-core/internal/commit"},
		},
		{
			file: "ucupa-core/internal/commit/query_limits_test.go",
			want: []string{"errors", "reflect", "testing", "shared/values", "ucupa-core/contract", "ucupa-core/internal/commit", "ucupa-core/internal/geometry", "ucupa-core/internal/query/oracle"},
		},
		{
			file: "ucupa-core/internal/commit/query_corruption_test.go",
			want: []string{"testing", "ucupa-core/contract", "ucupa-core/internal/commit"},
		},
		{
			file: "ucupa-core/internal/commit/query_durability_test.go",
			want: []string{"bytes", "errors", "math", "path/filepath", "reflect", "testing", "shared/values", "ucupa-core/contract", "ucupa-core/internal/commit"},
		},
		{
			file: "ucupa-core/internal/commit/query_fixture_test.go",
			want: []string{"crypto/rand", "errors", "os", "reflect", "testing", "shared/values", "ucupa-core/contract", "ucupa-core/internal/commit", "ucupa-core/internal/element", "ucupa-core/internal/query", "ucupa-core/internal/query/oracle"},
		},
		{
			file: "ucupa-core/internal/commit/query_fixture_export_test.go",
			want: []string{"fmt", "math", "testing", "shared/values", "ucupa-core/contract", "ucupa-core/internal/commit/binding", "ucupa-core/internal/element/fact", "ucupa-core/internal/geometry"},
		},
		{
			file: "ucupa-core/internal/commit/query_fault_export_test.go",
			want: []string{"fmt", "ucupa-core/contract"},
		},
	}
	for _, tc := range cases {
		for _, dependency := range tc.want {
			if got := checkDependency(tc.file, dependency, "ucupa", true, stdlib); got != "" {
				t.Errorf("allowed query conformance dependency %s -> %s: %s", tc.file, dependency, got)
			}
		}
	}
	for _, tc := range []struct {
		file string
		bad  []string
	}{
		{
			file: "ucupa-core/internal/commit/query_conformance_test.go",
			bad:  []string{"ucupa-core/internal/element", "ucupa-core/internal/query", "ucupa-core/internal/query/oracle", "ucupa-core/internal/store", "go.etcd.io/bbolt"},
		},
		{
			file: "ucupa-core/internal/commit/query_limits_test.go",
			bad:  []string{"ucupa-core/internal/element", "ucupa-core/internal/mount", "ucupa-core/internal/store", "ucupa-core/internal/token", "go.etcd.io/bbolt"},
		},
		{
			file: "ucupa-core/internal/commit/query_corruption_test.go",
			bad:  []string{"ucupa-core/internal/element", "ucupa-core/internal/query", "ucupa-core/internal/query/oracle", "ucupa-core/internal/store", "go.etcd.io/bbolt"},
		},
		{
			file: "ucupa-core/internal/commit/query_durability_test.go",
			bad:  []string{"ucupa-core/internal/element", "ucupa-core/internal/query", "ucupa-core/internal/query/oracle", "ucupa-core/internal/store", "go.etcd.io/bbolt"},
		},
		{
			file: "ucupa-core/internal/commit/query_fixture_test.go",
			bad:  []string{"ucupa-core/internal/mount", "ucupa-core/internal/store", "ucupa-core/internal/token", "go.etcd.io/bbolt"},
		},
		{
			file: "ucupa-core/internal/commit/query_fixture_export_test.go",
			bad:  []string{"ucupa-core/internal/commit", "ucupa-core/internal/element", "ucupa-core/internal/query", "ucupa-core/internal/query/oracle", "ucupa-core/internal/store", "ucupa-core/internal/token"},
		},
		{
			file: "ucupa-core/internal/commit/query_fault_export_test.go",
			bad:  []string{"ucupa-core", "ucupa-core/internal/element", "ucupa-core/internal/query", "ucupa-core/internal/store", "go.etcd.io/bbolt"},
		},
	} {
		for _, dependency := range tc.bad {
			if got := checkDependency(tc.file, dependency, "ucupa", true, stdlib); got == "" {
				t.Errorf("forbidden query conformance dependency accepted: %s -> %s", tc.file, dependency)
			}
		}
	}
}

func TestQueryCommitTestOnlyExportsAreExact(t *testing.T) {
	root, err := currentRepositoryRoot(t)
	if err != nil {
		t.Fatal(err)
	}
	assertQueryExportSurface(t, root, "ucupa-core/internal/commit/query_fixture_export_test.go", map[string]queryFunctionSpec{
		"BuildQueryEdgeFixtureForTest":     {params: []string{"*testing.T", "*Owner"}, results: []string{"[]values.ElementID"}},
		"AddQueryOverflowForTest":          {params: []string{"*testing.T", "*Owner"}},
		"BuildQueryRationalFixtureForTest": {params: []string{"*testing.T", "*Owner"}, results: []string{"values.ElementID"}},
	})
	assertQueryExportSurface(t, root, "ucupa-core/internal/commit/query_fault_export_test.go", map[string]queryFunctionSpec{
		"InjectQueryRegionCorruptionForTest": {params: []string{"*Owner", "string"}, results: []string{"error"}},
		"QueryOutcomesMatchForTest":          {params: []string{"*Owner", "CompleteStateProofForTest"}, results: []string{"int", "bool"}},
	})
}

func TestQueryCommitTestOnlyExportsAreUndefinedInProduction(t *testing.T) {
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
			name: "edge fixture",
			src:  "package main\nimport commitpkg \"ucupa/ucupa-core/internal/commit\"\nvar _ = commitpkg.BuildQueryEdgeFixtureForTest\nfunc main() {}\n",
			want: "undefined: commitpkg.BuildQueryEdgeFixtureForTest",
		},
		{
			name: "overflow fixture",
			src:  "package main\nimport commitpkg \"ucupa/ucupa-core/internal/commit\"\nvar _ = commitpkg.AddQueryOverflowForTest\nfunc main() {}\n",
			want: "undefined: commitpkg.AddQueryOverflowForTest",
		},
		{
			name: "rational fixture",
			src:  "package main\nimport commitpkg \"ucupa/ucupa-core/internal/commit\"\nvar _ = commitpkg.BuildQueryRationalFixtureForTest\nfunc main() {}\n",
			want: "undefined: commitpkg.BuildQueryRationalFixtureForTest",
		},
		{
			name: "corruption bridge",
			src:  "package main\nimport commitpkg \"ucupa/ucupa-core/internal/commit\"\nvar _ = commitpkg.InjectQueryRegionCorruptionForTest\nfunc main() {}\n",
			want: "undefined: commitpkg.InjectQueryRegionCorruptionForTest",
		},
		{
			name: "outcome proof bridge",
			src:  "package main\nimport commitpkg \"ucupa/ucupa-core/internal/commit\"\nvar _ = commitpkg.QueryOutcomesMatchForTest\nfunc main() {}\n",
			want: "undefined: commitpkg.QueryOutcomesMatchForTest",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			directory, err := os.MkdirTemp(filepath.Join(root, "ucupa-core"), ".query-commit-build-")
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
			text := string(output)
			if err == nil || !strings.Contains(text, tc.want) || strings.Contains(text, "use of internal package") {
				t.Fatalf("production build did not reject query test export as %q: %v\n%s", tc.want, err, output)
			}
		})
	}
}

type queryFunctionSpec struct {
	params, results []string
}

func assertQueryExportSurface(t *testing.T, root, file string, want map[string]queryFunctionSpec) {
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
			if !sameStringSlice(expandedQueryASTFieldTypes(declaration.Type.Params), expected.params) ||
				!sameStringSlice(expandedQueryASTFieldTypes(declaration.Type.Results), expected.results) {
				t.Fatalf("%s %s signature changed", file, declaration.Name.Name)
			}
			found[declaration.Name.Name] = true
		}
	}
	if len(found) != len(want) {
		t.Fatalf("%s exported surface changed: got=%v want=%v", file, found, want)
	}
}

func expandedQueryASTFieldTypes(fields *ast.FieldList) []string {
	if fields == nil {
		return nil
	}
	types := make([]string, 0, len(fields.List))
	for _, field := range fields.List {
		if len(field.Names) == 0 {
			types = append(types, queryASTType(field.Type))
			continue
		}
		for range field.Names {
			types = append(types, queryASTType(field.Type))
		}
	}
	return types
}

func queryASTType(expression ast.Expr) string {
	if array, ok := expression.(*ast.ArrayType); ok {
		if array.Len != nil {
			return ""
		}
		return "[]" + queryASTType(array.Elt)
	}
	return guardASTType(expression)
}
