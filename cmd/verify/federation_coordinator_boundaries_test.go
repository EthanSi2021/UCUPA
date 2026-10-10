package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestCoordinatorAndResolveExactLocationsAndDependencies(t *testing.T) {
	stdlib := map[string]bool{"context": true, "testing": true}
	for directory, files := range map[string]map[string][]string{coreCoordinatorDirectory: coreCoordinatorDependencies, coreResolveDirectory: coreResolveDependencies} {
		for file, dependencies := range files {
			name := directory + "/" + file
			isTest := strings.HasSuffix(file, "_test.go")
			if reason := checkSourceLocation(name); reason != "" {
				t.Fatalf("%s: %s", name, reason)
			}
			for _, dependency := range dependencies {
				if reason := checkDependency(name, "ucupa/"+dependency, "ucupa", isTest, stdlib); reason != "" {
					t.Errorf("%s -> %s: %s", name, dependency, reason)
				}
			}
			for _, dependency := range []string{"ucupa-core", "ucupa-core/instance", "ucupa-core/internal/store", "ucupa-core/internal/mount", "ucupa-core/internal/element/fact", "ucupa-core/internal/query/connected", "ucupa-core/transport/grpc", "ucupa-core/sdk/go", "ucupa-facet/contract", "go.etcd.io/bbolt"} {
				if reason := checkDependency(name, moduleImport("ucupa", dependency), "ucupa", isTest, stdlib); reason == "" {
					t.Errorf("unreviewed edge accepted: %s -> %s", name, dependency)
				}
			}
			if handled, reason := checkCoreCoordinatorDependency(name, "context", "ucupa", !isTest, stdlib); !handled || reason == "" {
				t.Errorf("production/test disguise: %s", name)
			}
		}
		for _, file := range []string{"extra.go", "extra_test.go", "nested/operation.go"} {
			if reason := checkSourceLocation(directory + "/" + file); reason == "" {
				t.Errorf("unreviewed location accepted: %s/%s", directory, file)
			}
		}
	}
	for _, file := range []string{"ucupa-core/store.go", "ucupa-core/internal/commit/owner.go", "ucupa-core/internal/query/federation/collect/read.go", "ucupa-core/internal/access/registry/registry.go", "ucupa-core/transport/grpc/server.go", "ucupa-facet/instance/facet.go"} {
		for _, target := range []string{coreCoordinatorDirectory, coreResolveDirectory} {
			if reason := checkDependency(file, "ucupa/"+target, "ucupa", false, stdlib); reason == "" {
				t.Errorf("private consumer accepted: %s -> %s", file, target)
			}
		}
	}
}

func coordinatorSurfaceErrors(parsed *ast.File, resolve bool) []string {
	var issues []string
	wantFields := map[string]string{"owner": "*commit.Owner", "admission": "*access.Admission", "registry": "*registry.Registry", "sources": "*source.Operations", "observations": "*observation.Operations"}
	for _, d := range parsed.Decls {
		switch d := d.(type) {
		case *ast.GenDecl:
			if d.Tok == token.VAR || d.Tok == token.CONST {
				issues = append(issues, "package state")
				continue
			}
			if d.Tok != token.TYPE {
				continue
			}
			for _, s := range d.Specs {
				spec := s.(*ast.TypeSpec)
				if resolve || spec.Name.Name != "Operations" {
					issues = append(issues, "unexpected type")
					continue
				}
				st, ok := spec.Type.(*ast.StructType)
				if !ok || spec.Assign.IsValid() {
					issues = append(issues, "not an operation struct")
					continue
				}
				got := map[string]string{}
				for _, field := range st.Fields.List {
					if len(field.Names) == 0 {
						issues = append(issues, "embedded capability")
					}
					for _, name := range field.Names {
						got[name.Name] = guardASTType(field.Type)
					}
				}
				if !reflect.DeepEqual(got, wantFields) {
					issues = append(issues, "operation fields")
				}
			}
		case *ast.FuncDecl:
			if !ast.IsExported(d.Name.Name) {
				continue
			}
			if resolve {
				switch d.Name.Name {
				case "TokenLocked", "SpaceLocked", "ReadLocked", "ContinuationLocked", "PreparationLocked", "CursorLocked":
					if d.Recv != nil {
						issues = append(issues, "resolver method")
					}
				default:
					issues = append(issues, "unexpected resolver function")
				}
			} else if d.Recv == nil {
				if d.Name.Name != "New" {
					issues = append(issues, "unexpected constructor/helper")
				}
			} else if len(d.Recv.List) != 1 || guardASTType(d.Recv.List[0].Type) != "*Operations" || d.Name.Name != "Read" && d.Name.Name != "Observe" {
				issues = append(issues, "unexpected operation method")
			}
		}
	}
	return issues
}

func TestCoordinatorAndResolveHaveNoSecondOwnerOrExportEscape(t *testing.T) {
	root, err := currentRepositoryRoot(t)
	if err != nil {
		t.Fatal(err)
	}
	for directory, files := range map[string]map[string][]string{coreCoordinatorDirectory: coreCoordinatorDependencies, coreResolveDirectory: coreResolveDependencies} {
		for name := range files {
			if strings.HasSuffix(name, "_test.go") {
				continue
			}
			parsed, err := parser.ParseFile(token.NewFileSet(), filepath.Join(root, filepath.FromSlash(directory), name), nil, 0)
			if err != nil {
				t.Fatal(err)
			}
			if issues := coordinatorSurfaceErrors(parsed, directory == coreResolveDirectory); len(issues) != 0 {
				t.Fatalf("%s/%s: %v", directory, name, issues)
			}
		}
	}
	for _, source := range []string{
		"package coordinator; var private, Public int",
		"package coordinator; type Operations struct { owner *commit.Owner; registry *registry.Registry; contexts map[string]any }",
		"package coordinator; type Operations = Other",
		"package coordinator; type Operations struct { *commit.Owner }",
		"package coordinator; func (o *Operations) State() any { return nil }",
		"package coordinator; func WithState(f func(any)) {}",
	} {
		parsed, err := parser.ParseFile(token.NewFileSet(), "injected.go", source, 0)
		if err != nil {
			t.Fatal(err)
		}
		if len(coordinatorSurfaceErrors(parsed, false)) == 0 {
			t.Fatal("surface escape accepted", source)
		}
	}
	for _, source := range []string{"package resolve; type State struct{}", "package resolve; var entries map[string]any", "package resolve; func Mint(){}"} {
		parsed, err := parser.ParseFile(token.NewFileSet(), "injected.go", source, 0)
		if err != nil {
			t.Fatal(err)
		}
		if len(coordinatorSurfaceErrors(parsed, true)) == 0 {
			t.Fatal("resolver state/issuance accepted", source)
		}
	}
}
