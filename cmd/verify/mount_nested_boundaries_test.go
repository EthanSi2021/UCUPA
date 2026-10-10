package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path"
	"path/filepath"
	"strings"
	"testing"
)

func TestReviewedNestedMountSourcesAndNoUnknownFiles(t *testing.T) {
	want := []string{
		"ucupa-core/internal/commit/mount_nested_fixture_test.go",
		"ucupa-core/internal/commit/mount_nested_test.go",
	}
	for _, file := range want {
		if !reviewedCommitSourceFile(path.Base(file)) {
			t.Errorf("nested Mount source is missing from exact commit registration: %s", file)
		}
		if got := checkSourceLocation(file); got != "" {
			t.Errorf("reviewed nested Mount source %s rejected: %s", file, got)
		}
	}
	for _, file := range []string{
		"ucupa-core/internal/commit/mount_nested_extra_test.go",
		"ucupa-core/internal/commit/mount_nested/nested_test.go",
		"ucupa-core/internal/commit/mount_nested_fixture_extra.go",
	} {
		if got := checkSourceLocation(file); got == "" {
			t.Errorf("unreviewed nested Mount source accepted: %s", file)
		}
	}
}

func TestReviewedNestedMountDependenciesArePerFile(t *testing.T) {
	stdlib := guardOracleStdlib()
	cases := []struct {
		file string
		want []string
	}{
		{
			file: "ucupa-core/internal/commit/mount_nested_fixture_test.go",
			want: []string{"context", "fmt", "path/filepath", "testing", "shared/values", "ucupa-core/contract", "ucupa-core/internal/commit"},
		},
		{
			file: "ucupa-core/internal/commit/mount_nested_test.go",
			want: []string{"context", "errors", "fmt", "testing", "ucupa-core/contract", "ucupa-core/internal/commit", "ucupa-core/internal/mount/fact"},
		},
	}
	for _, tc := range cases {
		for _, dependency := range tc.want {
			if got := checkDependency(tc.file, dependency, "ucupa", true, stdlib); got != "" {
				t.Errorf("allowed nested Mount dependency %s -> %s: %s", tc.file, dependency, got)
			}
		}
	}
	for _, tc := range []struct {
		file string
		bad  []string
	}{
		{
			file: "ucupa-core/internal/commit/mount_nested_fixture_test.go",
			bad:  []string{"ucupa-core/internal/element", "ucupa-core/internal/mount", "ucupa-core/internal/mount/fact", "ucupa-core/internal/query", "ucupa-core/internal/store", "go.etcd.io/bbolt"},
		},
		{
			file: "ucupa-core/internal/commit/mount_nested_test.go",
			bad:  []string{"ucupa-core/internal/element", "ucupa-core/internal/mount", "ucupa-core/internal/query", "ucupa-core/internal/store", "ucupa-core/internal/token", "go.etcd.io/bbolt"},
		},
	} {
		for _, dependency := range tc.bad {
			if got := checkDependency(tc.file, dependency, "ucupa", true, stdlib); got == "" {
				t.Errorf("forbidden nested Mount dependency accepted: %s -> %s", tc.file, dependency)
			}
		}
	}
}

func TestNestedMountFilesHaveNoNonTestExportSurface(t *testing.T) {
	root, err := currentRepositoryRoot(t)
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range []string{
		"ucupa-core/internal/commit/mount_nested_fixture_test.go",
		"ucupa-core/internal/commit/mount_nested_test.go",
	} {
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(file)))
		if err != nil {
			t.Fatal(err)
		}
		parsed, err := parser.ParseFile(token.NewFileSet(), file, data, parser.ParseComments)
		if err != nil {
			t.Fatal(err)
		}
		if reason := nestedMountTestSurfaceIssue(parsed); reason != "" {
			t.Fatalf("%s: %s", file, reason)
		}
	}
}

func TestNestedMountExportSurfaceRejectsTypeValueConstAndPseudoTests(t *testing.T) {
	for _, tc := range []struct {
		name string
		src  string
	}{
		{"exported type", "package commit_test\ntype Fixture struct{}\n"},
		{"exported var", "package commit_test\nvar Fixture = 1\n"},
		{"exported const", "package commit_test\nconst Fixture = 1\n"},
		{"test without parameter", "package commit_test\nfunc TestHelper() {}\n"},
		{"test with wrong parameter", "package commit_test\nimport \"testing\"\nfunc TestHelper(t testing.T) {}\n"},
		{"test with result", "package commit_test\nimport \"testing\"\nfunc TestHelper(t *testing.T) error { return nil }\n"},
		{"test method", "package commit_test\nimport \"testing\"\ntype fixture struct{}\nfunc (fixture) TestHelper(t *testing.T) {}\n"},
		{"non-test helper", "package commit_test\nimport \"testing\"\nfunc Helper(t *testing.T) {}\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			parsed, err := parser.ParseFile(token.NewFileSet(), tc.name+".go", []byte(tc.src), parser.ParseComments)
			if err != nil {
				t.Fatal(err)
			}
			if reason := nestedMountTestSurfaceIssue(parsed); reason == "" {
				t.Fatalf("accepted invalid nested Mount test surface: %s", tc.name)
			}
		})
	}
}

func nestedMountTestSurfaceIssue(parsed *ast.File) string {
	if parsed.Name == nil || parsed.Name.Name != "commit_test" {
		return "must remain an external commit_test package"
	}
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
						return "exports a type " + specification.Name.Name
					}
				case *ast.ValueSpec:
					for _, name := range specification.Names {
						if ast.IsExported(name.Name) {
							return "exports a value " + name.Name
						}
					}
				}
			}
		case *ast.FuncDecl:
			if declaration.Name == nil || !ast.IsExported(declaration.Name.Name) {
				continue
			}
			if declaration.Recv != nil {
				return "exports method " + declaration.Name.Name
			}
			if !strings.HasPrefix(declaration.Name.Name, "Test") {
				return "exports non-test function " + declaration.Name.Name
			}
			if declaration.Type.Params == nil || len(declaration.Type.Params.List) != 1 || len(declaration.Type.Params.List[0].Names) != 1 || guardASTType(declaration.Type.Params.List[0].Type) != "*testing.T" {
				return "exports pseudo-test " + declaration.Name.Name
			}
			if declaration.Type.Results != nil && len(declaration.Type.Results.List) != 0 {
				return "exports test function with results " + declaration.Name.Name
			}
		}
	}
	return ""
}
