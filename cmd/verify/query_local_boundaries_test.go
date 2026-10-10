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

func TestCoreLocalPublicBoundariesHaveExactLocationsAndImports(t *testing.T) {
	for _, file := range []string{
		"ucupa-core/internal/query/local/operation.go",
		"ucupa-core/internal/query/local/read.go",
		"ucupa-core/internal/query/local/address.go",
		"ucupa-core/internal/query/local/list.go",
		"ucupa-core/internal/query/local/query.go",
		"ucupa-core/internal/element/public.go",
		"ucupa-core/internal/access/value/entry.go",
		"ucupa-core/tests/access/access_test.go",
		"ucupa-core/tests/access/boundary_test.go",
		"ucupa-core/tests/access/fixture_test.go",
	} {
		if reason := checkSourceLocation(file); reason != "" {
			t.Errorf("reviewed source rejected: %s: %s", file, reason)
		}
		assertActualImportsAccepted(t, file)
	}

	for _, file := range []string{
		"ucupa-core/internal/query/local/extra.go",
		"ucupa-core/internal/query/local/operation_test.go",
		"ucupa-core/internal/query/local/nested/query.go",
		"ucupa-core/internal/element/public_extra.go",
		"ucupa-core/internal/access/value/entry_extra.go",
		"ucupa-core/tests/access/extra_test.go",
		"ucupa-core/tests/access/nested_test.go",
	} {
		if reason := checkSourceLocation(file); reason == "" {
			t.Errorf("unreviewed source accepted: %s", file)
		}
	}
}

func TestCoreLocalQueryDependenciesArePerFileAndOneWay(t *testing.T) {
	stdlib := guardOracleStdlib()
	stdlib["strconv"] = true
	for _, tc := range []struct {
		file, dependency string
	}{
		{"operation.go", "ucupa-core/contract"},
		{"operation.go", "ucupa-core/internal/access/value"},
		{"operation.go", "ucupa-core/internal/store"},
		{"read.go", "ucupa-core/internal/commit"},
		{"read.go", "ucupa-core/internal/query/scan"},
		{"read.go", "ucupa-core/internal/token"},
		{"address.go", "ucupa-core/internal/query"},
		{"address.go", "ucupa-core/internal/commit"},
		{"list.go", "ucupa-core/internal/access/registry"},
		{"list.go", "ucupa-core/internal/query"},
		{"query.go", "ucupa-core/internal/access"},
		{"query.go", "ucupa-core/internal/mount/fact"},
		{"query.go", "ucupa-core/internal/store"},
	} {
		if got := checkDependency(coreQueryLocalDirectory+"/"+tc.file, moduleImport("ucupa", tc.dependency), "ucupa", false, stdlib); got == "" {
			t.Errorf("forbidden local dependency accepted: %s -> %s", tc.file, tc.dependency)
		}
	}
	for _, tc := range []struct {
		file, dependency string
		allowed          bool
	}{
		{"ucupa-core/instance/instance.go", "ucupa-core/internal/query/local", true},
		{"ucupa-core/internal/access/entry/operation.go", "ucupa-core/internal/query/local", true},
		{"ucupa-core/core.go", "ucupa-core/internal/query/local", false},
		{"ucupa-core/public_context.go", "ucupa-core/internal/query/local", false},
		{"ucupa-core/internal/access/entry/request.go", "ucupa-core/internal/query/local", false},
		{"ucupa-core/public_entry.go", "ucupa-core/internal/query/local", false},
		{"ucupa-core/public_read.go", "ucupa-core/internal/query/local", false},
		{"ucupa-core/internal/element/operation.go", "ucupa-core/internal/query/local", false},
		{"ucupa-core/internal/query/operation.go", "ucupa-core/internal/query/local", false},
	} {
		got := checkDependency(tc.file, moduleImport("ucupa", tc.dependency), "ucupa", false, stdlib)
		if (got == "") != tc.allowed {
			t.Errorf("local consumer %s -> %s allowed=%v: %s", tc.file, tc.dependency, tc.allowed, got)
		}
	}
}

func TestCorePublicElementAndEntryValueDependenciesAreNarrow(t *testing.T) {
	stdlib := guardOracleStdlib()
	for _, dependency := range []string{
		"shared/values", "ucupa-core/contract", "ucupa-core/internal/access",
		"ucupa-core/internal/access/registry", "ucupa-core/internal/access/resolve",
		"ucupa-core/internal/access/value", "ucupa-core/internal/commit",
		"ucupa-core/internal/commit/binding", "ucupa-core/internal/token",
	} {
		if got := checkDependency(coreElementPublicFile, moduleImport("ucupa", dependency), "ucupa", false, stdlib); got != "" {
			t.Errorf("public Element dependency rejected: %s: %s", dependency, got)
		}
	}
	for _, dependency := range []string{
		"ucupa-core", "ucupa-core/internal/query", "ucupa-core/internal/store",
		"ucupa-core/internal/mount", "ucupa-core/internal/element/fact",
		"ucupa-core/transport/grpc", "ucupa-core/sdk/go", "go.etcd.io/bbolt",
	} {
		if got := checkDependency(coreElementPublicFile, moduleImport("ucupa", dependency), "ucupa", false, stdlib); got == "" {
			t.Errorf("forbidden public Element dependency accepted: %s", dependency)
		}
	}

	entry := "ucupa-core/internal/access/value/entry.go"
	for _, dependency := range []string{"shared/values", "ucupa-core/contract"} {
		if got := checkDependency(entry, moduleImport("ucupa", dependency), "ucupa", false, stdlib); got != "" {
			t.Errorf("entry dependency rejected: %s: %s", dependency, got)
		}
	}
	for _, dependency := range []string{
		"ucupa-core/internal/access", "ucupa-core/internal/geometry", "ucupa-core/internal/token",
		"ucupa-core/internal/commit", "ucupa-core/internal/store", "ucupa-core/transport/grpc",
	} {
		if got := checkDependency(entry, moduleImport("ucupa", dependency), "ucupa", false, stdlib); got == "" {
			t.Errorf("forbidden entry dependency accepted: %s", dependency)
		}
	}
}

func TestCoreAccessIntegrationTestsHaveExactPublicDependencies(t *testing.T) {
	stdlib := guardOracleStdlib()
	stdlib["encoding/json"] = true
	stdlib["path/filepath"] = true
	for _, file := range []string{"access_test.go", "boundary_test.go", "fixture_test.go"} {
		path := coreAccessIntegrationTestDirectory + "/" + file
		if !strings.HasSuffix(file, "_test.go") {
			t.Fatalf("fixture is not a test file: %s", file)
		}
		if got := checkDependency(path, moduleImport("ucupa", "ucupa-core/internal/access"), "ucupa", true, stdlib); got == "" {
			t.Errorf("access integration test unexpectedly reached internal access: %s", file)
		}
	}
	for _, tc := range []struct {
		file, dependency string
	}{
		{"access_test.go", "ucupa-core/instance"},
		{"access_test.go", "ucupa-core/contract"},
		{"boundary_test.go", "shared/values"},
		{"boundary_test.go", "ucupa-core/contract"},
		{"fixture_test.go", "ucupa-core/instance"},
		{"fixture_test.go", "ucupa-core/contract"},
	} {
		if got := checkDependency(coreAccessIntegrationTestDirectory+"/"+tc.file, moduleImport("ucupa", tc.dependency), "ucupa", true, stdlib); got != "" {
			t.Errorf("allowed access integration dependency rejected: %s -> %s: %s", tc.file, tc.dependency, got)
		}
	}
	for _, tc := range []struct {
		file, dependency string
	}{
		{"access_test.go", "ucupa-core/internal/access"},
		{"access_test.go", "ucupa-core/internal/commit"},
		{"access_test.go", "ucupa-core/internal/element"},
		{"access_test.go", "ucupa-core/transport/grpc"},
		{"boundary_test.go", "ucupa-core/internal/query"},
		{"fixture_test.go", "ucupa-core/internal/mount"},
		{"fixture_test.go", "ucupa-core/tests/query"},
		{"fixture_test.go", "ucupa-facet/contract"},
	} {
		if got := checkDependency(coreAccessIntegrationTestDirectory+"/"+tc.file, moduleImport("ucupa", tc.dependency), "ucupa", true, stdlib); got == "" {
			t.Errorf("forbidden access integration dependency accepted: %s -> %s", tc.file, tc.dependency)
		}
	}
}

func TestCoreLocalAndPublicElementExportSurfacesAreExact(t *testing.T) {
	cases := []struct {
		file      string
		types     map[string]bool
		functions map[string]bool
		methods   map[string]map[string]bool
		fields    map[string][][2]string
	}{
		{"ucupa-core/internal/query/local/operation.go", map[string]bool{"Operations": true}, map[string]bool{"New": true}, nil, map[string][][2]string{
			"Operations": {{"owner", "*commit.Owner"}, {"admission", "*access.Admission"}, {"access", "*registry.Registry"}, {"queries", "*query.Operations"}},
		}},
		{"ucupa-core/internal/query/local/read.go", nil, nil, map[string]map[string]bool{"*Operations": {"Read": true}}, nil},
		{"ucupa-core/internal/query/local/address.go", nil, nil, nil, nil},
		{"ucupa-core/internal/query/local/list.go", nil, nil, nil, nil},
		{"ucupa-core/internal/query/local/query.go", nil, nil, nil, nil},
		{"ucupa-core/internal/element/public.go", map[string]bool{"PublicOperations": true}, map[string]bool{"NewPublic": true}, map[string]map[string]bool{"*PublicOperations": {"Mutate": true}}, map[string][][2]string{
			"PublicOperations": {{"owner", "*commitpkg.Owner"}, {"admission", "*accesspkg.Admission"}, {"access", "*accessregistry.Registry"}},
		}},
		{"ucupa-core/internal/access/value/entry.go", nil, map[string]bool{"Intent": true, "Reference": true}, nil, nil},
	}
	for _, tc := range cases {
		parsed := parseCurrentCoreBoundarySource(t, tc.file)
		assertExactPublicSurface(t, tc.file, parsed, tc.types, tc.functions, tc.methods)
		for typeName, fields := range tc.fields {
			assertExactPrivateFields(t, tc.file, parsed, typeName, fields)
		}
	}
}

func assertActualImportsAccepted(t *testing.T, relative string) {
	t.Helper()
	root, err := currentRepositoryRoot(t)
	if err != nil {
		t.Fatal(err)
	}
	filename := filepath.Join(root, filepath.FromSlash(relative))
	data, err := os.ReadFile(filename)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := parser.ParseFile(token.NewFileSet(), filename, data, 0)
	if err != nil {
		t.Fatal(err)
	}
	stdlib := guardOracleStdlib()
	stdlib["encoding/base64"] = true
	stdlib["strconv"] = true
	for _, spec := range parsed.Imports {
		pathValue := strings.Trim(spec.Path.Value, "\"")
		if got := checkDependency(relative, pathValue, "ucupa", strings.HasSuffix(relative, "_test.go"), stdlib); got != "" {
			t.Errorf("actual import rejected: %s -> %s: %s", relative, pathValue, got)
		}
	}
}

func parseCurrentCoreBoundarySource(t *testing.T, relative string) *ast.File {
	t.Helper()
	root, err := currentRepositoryRoot(t)
	if err != nil {
		t.Fatal(err)
	}
	filename := filepath.Join(root, filepath.FromSlash(relative))
	parsed, err := parser.ParseFile(token.NewFileSet(), filename, nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	return parsed
}

func assertExactPublicSurface(t *testing.T, file string, parsed *ast.File, wantTypes, wantFunctions map[string]bool, wantMethods map[string]map[string]bool) {
	t.Helper()
	foundTypes := make(map[string]bool)
	foundFunctions := make(map[string]bool)
	foundMethods := make(map[string]map[string]bool)
	for _, declaration := range parsed.Decls {
		switch declaration := declaration.(type) {
		case *ast.GenDecl:
			if declaration.Tok == token.IMPORT {
				continue
			}
			for _, specification := range declaration.Specs {
				for _, name := range queryLocalDeclarationNames(specification) {
					if ast.IsExported(name) {
						if declaration.Tok != token.TYPE || !wantTypes[name] {
							t.Errorf("%s exports unexpected declaration %s", file, name)
						}
						foundTypes[name] = true
					}
				}
			}
		case *ast.FuncDecl:
			if declaration.Name == nil || !ast.IsExported(declaration.Name.Name) {
				continue
			}
			if declaration.Recv == nil {
				if !wantFunctions[declaration.Name.Name] {
					t.Errorf("%s exports unexpected function %s", file, declaration.Name.Name)
				}
				foundFunctions[declaration.Name.Name] = true
				continue
			}
			if len(declaration.Recv.List) != 1 {
				t.Errorf("%s exports method with unexpected receiver shape: %s", file, declaration.Name.Name)
				continue
			}
			receiver := guardASTType(declaration.Recv.List[0].Type)
			if wantMethods[receiver] == nil || !wantMethods[receiver][declaration.Name.Name] {
				t.Errorf("%s exports unexpected method %s on %s", file, declaration.Name.Name, receiver)
			}
			if foundMethods[receiver] == nil {
				foundMethods[receiver] = make(map[string]bool)
			}
			foundMethods[receiver][declaration.Name.Name] = true
		}
	}
	for name := range wantTypes {
		if !foundTypes[name] {
			t.Errorf("%s is missing expected type %s", file, name)
		}
	}
	for name := range wantFunctions {
		if !foundFunctions[name] {
			t.Errorf("%s is missing expected function %s", file, name)
		}
	}
	for receiver, methods := range wantMethods {
		for name := range methods {
			if !foundMethods[receiver][name] {
				t.Errorf("%s is missing expected method %s on %s", file, name, receiver)
			}
		}
	}
}

func assertExactPrivateFields(t *testing.T, file string, parsed *ast.File, typeName string, want [][2]string) {
	t.Helper()
	for _, declaration := range parsed.Decls {
		gen, ok := declaration.(*ast.GenDecl)
		if !ok || gen.Tok != token.TYPE {
			continue
		}
		for _, specification := range gen.Specs {
			typeSpec, ok := specification.(*ast.TypeSpec)
			if !ok || typeSpec.Name.Name != typeName {
				continue
			}
			structType, ok := typeSpec.Type.(*ast.StructType)
			if !ok || structType.Fields == nil || len(structType.Fields.List) != len(want) {
				t.Fatalf("%s.%s does not have the exact private component fields", file, typeName)
			}
			for index, field := range structType.Fields.List {
				if len(field.Names) != 1 || field.Names[0].Name != want[index][0] || ast.IsExported(field.Names[0].Name) || guardASTType(field.Type) != want[index][1] {
					t.Errorf("%s.%s field %d is not %s %s", file, typeName, index, want[index][0], want[index][1])
				}
			}
			return
		}
	}
	t.Fatalf("%s.%s was not found", file, typeName)
}

func queryLocalDeclarationNames(specification ast.Spec) []string {
	switch specification := specification.(type) {
	case *ast.TypeSpec:
		return []string{specification.Name.Name}
	case *ast.ValueSpec:
		names := make([]string, 0, len(specification.Names))
		for _, name := range specification.Names {
			names = append(names, name.Name)
		}
		return names
	default:
		return nil
	}
}
