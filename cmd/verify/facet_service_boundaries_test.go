package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCoreFacetServiceFilesHaveExactLocationsAndDependencies(t *testing.T) {
	const directory = "ucupa-core/internal/facet"
	for _, file := range []string{"service_attachment.go", "service_proposal.go"} {
		if reason := checkSourceLocation(directory + "/" + file); reason != "" {
			t.Errorf("reviewed Core Facet service source rejected: %s: %s", file, reason)
		}
	}
	for _, file := range []string{
		"service_extra.go", "service_attachment_test.go", "nested/service_proposal.go",
	} {
		if reason := checkSourceLocation(directory + "/" + file); reason == "" {
			t.Errorf("unreviewed Core Facet service source accepted: %s", file)
		}
	}

	stdlib := guardOracleStdlib()
	for file, dependencies := range map[string][]string{
		"service_attachment.go": {
			"shared/values", "ucupa-core/contract", "ucupa-core/internal/access/registry",
			"ucupa-core/internal/access/value",
		},
		"service_proposal.go": {"shared/values", "ucupa-core/contract"},
	} {
		for _, dependency := range dependencies {
			if reason := checkDependency(directory+"/"+file, moduleImport("ucupa", dependency), "ucupa", false, stdlib); reason != "" {
				t.Errorf("allowed Core Facet service dependency %s -> %s: %s", file, dependency, reason)
			}
		}
	}
	for file, dependencies := range map[string][]string{
		"service_attachment.go": {
			"ucupa-core", "ucupa-core/internal/commit", "ucupa-core/internal/element",
			"ucupa-core/internal/facet/fact", "ucupa-core/internal/mount", "ucupa-core/internal/query",
			"ucupa-core/internal/store", "ucupa-core/transport/grpc", "ucupa-facet/contract",
			"go.etcd.io/bbolt",
		},
		"service_proposal.go": {
			"ucupa-core", "ucupa-core/internal/access/registry", "ucupa-core/internal/commit",
			"ucupa-core/internal/element", "ucupa-core/internal/facet/fact", "ucupa-core/internal/mount",
			"ucupa-core/internal/query", "ucupa-core/internal/store", "ucupa-core/transport/grpc",
			"go.etcd.io/bbolt",
		},
	} {
		for _, dependency := range dependencies {
			if reason := checkDependency(directory+"/"+file, moduleImport("ucupa", dependency), "ucupa", false, stdlib); reason == "" {
				t.Errorf("forbidden Core Facet service dependency accepted: %s -> %s", file, dependency)
			}
		}
	}
}

func TestCoreFacetServiceExportMethodsAreExact(t *testing.T) {
	root, err := currentRepositoryRoot(t)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]map[string]bool{
		"service_attachment.go": {"ReadAttachmentControl": true, "CheckAttachmentControl": true, "ReadSuccessorControl": true},
		"service_proposal.go":   {"ResolveProposalControl": true, "ReadProposalControl": true},
	}
	fset := token.NewFileSet()
	for file, expected := range want {
		filename := filepath.Join(root, "ucupa-core", "internal", "facet", file)
		parsed, err := parser.ParseFile(fset, filename, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", filename, err)
		}
		found := make(map[string]bool)
		for _, declaration := range parsed.Decls {
			switch declaration := declaration.(type) {
			case *ast.GenDecl:
				if declaration.Tok == token.TYPE || declaration.Tok == token.VAR || declaration.Tok == token.CONST {
					for _, spec := range declaration.Specs {
						for _, name := range facetServiceDeclarationNames(spec) {
							if ast.IsExported(name) {
								t.Errorf("%s exports unexpected declaration %s", file, name)
							}
						}
					}
				}
			case *ast.FuncDecl:
				if !ast.IsExported(declaration.Name.Name) {
					continue
				}
				if declaration.Recv == nil || !expected[declaration.Name.Name] {
					t.Errorf("%s exports unexpected function or receiver method %s", file, declaration.Name.Name)
					continue
				}
				if len(declaration.Recv.List) != 1 || guardASTType(declaration.Recv.List[0].Type) != "*Operations" {
					t.Errorf("%s %s has an unexpected receiver", file, declaration.Name.Name)
				}
				found[declaration.Name.Name] = true
			}
		}
		for name := range expected {
			if !found[name] {
				t.Errorf("%s is missing expected service method %s", file, name)
			}
		}
	}
}

func TestCoreFacetServiceTestsHaveExactImportsAndExternalSurface(t *testing.T) {
	const directory = "ucupa-core/internal/facet"
	stdlib := guardOracleStdlib()
	for file, dependencies := range map[string][]string{
		"service_fixture_test.go": {
			"reflect", "testing", "time", "shared/values",
			"ucupa-core/internal/access", "ucupa-core/internal/access/registry", "ucupa-core/internal/facet",
		},
		"service_test.go": {
			"context", "errors", "reflect", "testing", "shared/values", "ucupa-core/contract",
		},
	} {
		for _, dependency := range dependencies {
			if reason := checkDependency(directory+"/"+file, moduleImport("ucupa", dependency), "ucupa", true, stdlib); reason != "" {
				t.Errorf("allowed Core Facet service-test dependency %s -> %s: %s", file, dependency, reason)
			}
		}
	}
	for file, dependencies := range map[string][]string{
		"service_fixture_test.go": {
			"ucupa-core/internal/commit", "ucupa-core/internal/element", "ucupa-core/internal/mount",
			"ucupa-core/internal/query", "ucupa-core/internal/store", "ucupa-core/transport/grpc",
			"ucupa-facet/contract", "go.etcd.io/bbolt",
		},
		"service_test.go": {
			"ucupa-core/internal/access", "ucupa-core/internal/access/registry", "ucupa-core/internal/commit",
			"ucupa-core/internal/element", "ucupa-core/internal/facet", "ucupa-core/internal/mount",
			"ucupa-core/internal/query", "ucupa-core/internal/store", "ucupa-core/transport/grpc",
			"go.etcd.io/bbolt",
		},
	} {
		for _, dependency := range dependencies {
			if reason := checkDependency(directory+"/"+file, moduleImport("ucupa", dependency), "ucupa", true, stdlib); reason == "" {
				t.Errorf("forbidden Core Facet service-test dependency accepted: %s -> %s", file, dependency)
			}
		}
	}

	root, err := currentRepositoryRoot(t)
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range []string{"service_fixture_test.go", "service_test.go"} {
		filename := filepath.Join(root, filepath.FromSlash(directory+"/"+file))
		data, err := os.ReadFile(filename)
		if err != nil {
			t.Fatal(err)
		}
		parsed, err := parser.ParseFile(token.NewFileSet(), filename, data, parser.ParseComments)
		if err != nil {
			t.Fatal(err)
		}
		if parsed.Name == nil || parsed.Name.Name != "facet_test" {
			t.Errorf("%s must remain an external facet_test package", file)
		}
		for _, declaration := range parsed.Decls {
			switch declaration := declaration.(type) {
			case *ast.GenDecl:
				if declaration.Tok == token.IMPORT {
					continue
				}
				for _, spec := range declaration.Specs {
					for _, name := range facetServiceDeclarationNames(spec) {
						if ast.IsExported(name) {
							t.Errorf("%s exports test declaration %s", file, name)
						}
					}
				}
			case *ast.FuncDecl:
				if !ast.IsExported(declaration.Name.Name) {
					continue
				}
				if declaration.Recv != nil {
					// servicePolicy implements the real Authorizer interface. Its
					// receiver type is private to this external test package, so
					// these method names do not create a package-level test API.
					receiver := strings.TrimPrefix(guardASTType(declaration.Recv.List[0].Type), "*")
					if receiver == "" || ast.IsExported(receiver) {
						t.Errorf("%s exports a method on a non-private test type: %s", file, declaration.Name.Name)
					}
					continue
				}
				if !strings.HasPrefix(declaration.Name.Name, "Test") ||
					declaration.Type.Params == nil || len(declaration.Type.Params.List) != 1 ||
					len(declaration.Type.Params.List[0].Names) != 1 ||
					guardASTType(declaration.Type.Params.List[0].Type) != "*testing.T" ||
					declaration.Type.Results != nil && len(declaration.Type.Results.List) != 0 {
					t.Errorf("%s exports a non-test or malformed test function: %s", file, declaration.Name.Name)
				}
			}
		}
	}
}

func TestCoreFacetServiceExportChecksRejectMultiNameValues(t *testing.T) {
	for _, source := range []string{
		"package facet\nvar hidden, Exported = 1, 2\n",
		"package facet\nconst hidden, Exported = 1, 2\n",
	} {
		parsed, err := parser.ParseFile(token.NewFileSet(), "service-invalid.go", []byte(source), 0)
		if err != nil {
			t.Fatal(err)
		}
		if reason := facetServiceExportedDeclarationIssue(parsed); reason == "" {
			t.Fatalf("multi-name exported declaration was accepted: %q", source)
		}
	}
}

func facetServiceExportedDeclarationIssue(parsed *ast.File) string {
	for _, declaration := range parsed.Decls {
		gen, ok := declaration.(*ast.GenDecl)
		if !ok || gen.Tok == token.IMPORT {
			continue
		}
		for _, spec := range gen.Specs {
			for _, name := range facetServiceDeclarationNames(spec) {
				if ast.IsExported(name) {
					return "exports " + name
				}
			}
		}
	}
	return ""
}

func facetServiceDeclarationNames(spec ast.Spec) []string {
	switch spec := spec.(type) {
	case *ast.TypeSpec:
		return []string{spec.Name.Name}
	case *ast.ValueSpec:
		result := make([]string, 0, len(spec.Names))
		for _, name := range spec.Names {
			result = append(result, name.Name)
		}
		return result
	}
	return nil
}
