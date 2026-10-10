package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"testing"
)

func TestCoreAccessObservationSourcesAndDependenciesAreExact(t *testing.T) {
	const directory = "ucupa-core/internal/access/observation"
	for _, file := range []string{"operation.go", "observe.go", "operation_test.go"} {
		if reason := checkSourceLocation(directory + "/" + file); reason != "" {
			t.Errorf("reviewed Core access observation source rejected: %s: %s", file, reason)
		}
	}
	for _, file := range []string{
		"observe_test.go", "extra.go", "nested/operation.go",
	} {
		if reason := checkSourceLocation(directory + "/" + file); reason == "" {
			t.Errorf("unreviewed Core access observation source accepted: %s", file)
		}
	}

	stdlib := guardOracleStdlib()
	for file, dependencies := range map[string][]string{
		"operation.go": {
			"ucupa-core/internal/access", "ucupa-core/internal/access/registry",
			"ucupa-core/internal/commit",
		},
		"observe.go": {
			"shared/values", "ucupa-core/internal/access/registry", "ucupa-core/internal/access/value",
		},
	} {
		for _, dependency := range dependencies {
			if reason := checkDependency(directory+"/"+file, moduleImport("ucupa", dependency), "ucupa", false, stdlib); reason != "" {
				t.Errorf("allowed Core access observation dependency %s -> %s: %s", file, dependency, reason)
			}
		}
	}
	for _, dependency := range []string{
		"errors", "reflect", "testing", "time", "shared/values",
		"ucupa/ucupa-core/contract", "ucupa/ucupa-core/contract/federation",
		"ucupa/ucupa-core/internal/access", "ucupa/ucupa-core/internal/access/observation",
		"ucupa/ucupa-core/internal/access/registry", "ucupa/ucupa-core/internal/access/value",
		"ucupa/ucupa-core/internal/commit", "ucupa/ucupa-core/internal/element",
	} {
		if reason := checkDependency(directory+"/operation_test.go", moduleImport("ucupa", dependency), "ucupa", true, stdlib); reason != "" {
			t.Errorf("allowed Core access observation test dependency %s: %s", dependency, reason)
		}
	}
	for file, dependencies := range map[string][]string{
		"operation.go": {
			"ucupa-core", "ucupa-core/contract", "ucupa-core/internal/commit/binding",
			"ucupa-core/internal/element", "ucupa-core/internal/facet", "ucupa-core/internal/mount",
			"ucupa-core/internal/query", "ucupa-core/internal/store", "ucupa-core/transport/grpc",
			"ucupa-core/sdk/go", "ucupa-facet/contract", "go.etcd.io/bbolt",
		},
		"observe.go": {
			"ucupa-core", "ucupa-core/contract", "ucupa-core/internal/commit",
			"ucupa-core/internal/element", "ucupa-core/internal/facet", "ucupa-core/internal/mount",
			"ucupa-core/internal/query", "ucupa-core/internal/store", "ucupa-core/transport/grpc",
			"ucupa-core/sdk/go", "go.etcd.io/bbolt",
		},
	} {
		for _, dependency := range dependencies {
			if reason := checkDependency(directory+"/"+file, moduleImport("ucupa", dependency), "ucupa", false, stdlib); reason == "" {
				t.Errorf("forbidden Core access observation dependency accepted: %s -> %s", file, dependency)
			}
		}
	}
}

func TestCoreAccessObservationConsumersAreExact(t *testing.T) {
	const imported = "ucupa/ucupa-core/internal/access/observation"
	stdlib := guardOracleStdlib()
	if reason := checkDependency("ucupa-core/instance/instance.go", imported, "ucupa", false, stdlib); reason != "" {
		t.Fatalf("Core assembly should construct access observation: %s", reason)
	}
	for _, file := range []string{
		"ucupa-core/public_federated_observe.go", "ucupa-core/public_federation_observe.go",
		"ucupa-core/store.go", "ucupa-core/public_entry.go", "ucupa-core/internal/access/admission.go",
		"ucupa-core/internal/access/registry/registry.go", "ucupa-core/internal/query/operation.go",
		"ucupa-core/internal/facet/operation.go", "ucupa-core/transport/grpc/server.go",
		"ucupa-core/sdk/go/client.go",
	} {
		if reason := checkDependency(file, imported, "ucupa", false, stdlib); reason == "" {
			t.Errorf("unapproved Core access observation consumer accepted: %s", file)
		}
	}
}

func TestCoreAccessObservationExportSurfaceIsNarrow(t *testing.T) {
	root, err := currentRepositoryRoot(t)
	if err != nil {
		t.Fatal(err)
	}
	wantTypes := map[string]bool{"Operations": true}
	wantFunctions := map[string]bool{"New": true}
	wantMethods := map[string]bool{"Run": true, "RunLocked": true}
	foundTypes := map[string]bool{}
	foundFunctions := map[string]bool{}
	foundMethods := map[string]bool{}
	fset := token.NewFileSet()
	for _, file := range []string{"operation.go", "observe.go"} {
		filename := filepath.Join(root, "ucupa-core", "internal", "access", "observation", file)
		parsed, err := parser.ParseFile(fset, filename, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", filename, err)
		}
		for _, declaration := range parsed.Decls {
			switch declaration := declaration.(type) {
			case *ast.GenDecl:
				if declaration.Tok == token.VAR || declaration.Tok == token.CONST {
					for _, spec := range declaration.Specs {
						if value, ok := spec.(*ast.ValueSpec); ok {
							for _, name := range value.Names {
								if ast.IsExported(name.Name) {
									t.Errorf("%s exports unexpected variable/constant %s", file, name.Name)
								}
							}
						}
					}
				}
				if declaration.Tok != token.TYPE {
					continue
				}
				for _, spec := range declaration.Specs {
					typeSpec, ok := spec.(*ast.TypeSpec)
					if !ok || !ast.IsExported(typeSpec.Name.Name) {
						continue
					}
					if !wantTypes[typeSpec.Name.Name] {
						t.Errorf("%s exports unexpected type %s", file, typeSpec.Name.Name)
					}
					foundTypes[typeSpec.Name.Name] = true
					if typeSpec.Name.Name == "Operations" {
						structType, ok := typeSpec.Type.(*ast.StructType)
						if !ok {
							t.Errorf("%s Operations is not a struct", file)
							continue
						}
						for _, field := range structType.Fields.List {
							for _, name := range field.Names {
								if ast.IsExported(name.Name) {
									t.Errorf("%s Operations exports field %s", file, name.Name)
								}
							}
						}
					}
				}
			case *ast.FuncDecl:
				if !ast.IsExported(declaration.Name.Name) {
					continue
				}
				if declaration.Recv == nil {
					if !wantFunctions[declaration.Name.Name] {
						t.Errorf("%s exports unexpected function %s", file, declaration.Name.Name)
					}
					foundFunctions[declaration.Name.Name] = true
					continue
				}
				if !wantMethods[declaration.Name.Name] {
					t.Errorf("%s exports unexpected method %s", file, declaration.Name.Name)
					continue
				}
				if len(declaration.Recv.List) != 1 || guardASTType(declaration.Recv.List[0].Type) != "*Operations" {
					t.Errorf("%s %s has an unexpected receiver", file, declaration.Name.Name)
				}
				foundMethods[declaration.Name.Name] = true
			}
		}
	}
	for name := range wantTypes {
		if !foundTypes[name] {
			t.Errorf("Core access observation type %s is missing", name)
		}
	}
	for name := range wantFunctions {
		if !foundFunctions[name] {
			t.Errorf("Core access observation function %s is missing", name)
		}
	}
	for name := range wantMethods {
		if !foundMethods[name] {
			t.Errorf("Core access observation method %s is missing", name)
		}
	}
}
