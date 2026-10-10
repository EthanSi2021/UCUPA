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

const coreQueryDirectory = "ucupa-core/internal/query"
const coreQueryIntegrationTestDirectory = "ucupa-core/tests/query"

func TestSourceAttachmentSnapshotUsesOnlyPublicContracts(t *testing.T) {
	const file = coreQueryIntegrationTestDirectory + "/source_attachment_test.go"
	if reason := checkSourceLocation(file); reason != "" {
		t.Fatal(reason)
	}
	stdlib := testStdlib()
	for _, dependency := range []string{"context", "encoding/binary", "testing", "time"} {
		stdlib[dependency] = true
	}
	for _, dependency := range []string{"context", "encoding/binary", "testing", "time", "ucupa/shared/values", "ucupa/ucupa-core/instance", "ucupa/ucupa-core/contract", "ucupa/ucupa-core/contract/federation"} {
		if reason := checkDependency(file, dependency, "ucupa", true, stdlib); reason != "" {
			t.Errorf("allowed %s: %s", dependency, reason)
		}
	}
	for _, dependency := range []string{"ucupa/ucupa-core/internal/commit", "ucupa/ucupa-core/internal/access/registry", "ucupa/ucupa-core/internal/query/federation/source", "ucupa/ucupa-core/internal/facet", "ucupa/ucupa-core/internal/store", "ucupa/ucupa-facet/instance", "ucupa/ucupa-core/transport/grpc", "go.etcd.io/bbolt"} {
		if reason := checkDependency(file, dependency, "ucupa", true, stdlib); reason == "" {
			t.Errorf("private implementation exposed: %s", dependency)
		}
	}
	if reason := checkDependency(file, "ucupa/ucupa-core/instance", "ucupa", false, stdlib); reason == "" {
		t.Fatal("test fixture accepted as production")
	}
	if reason := checkDependency(coreQueryIntegrationTestDirectory+"/snapshot_test.go", "ucupa/ucupa-core/contract/federation", "ucupa", true, stdlib); reason == "" {
		t.Fatal("specific source contract allowance expanded to unrelated tests")
	}
}

var coreQuerySourceFiles = []string{
	"config.go",
	"operation.go",
	"registry.go",
	"retained.go",
	"snapshot_read.go",
	"view.go",
	"query_entry_test.go",
	"snapshot_internal_test.go",
	"export_test.go",
	"connected_capture_resource_test.go",
	"connected_resource_test.go",
}

func TestReviewedCoreQueryOwnerBoundary(t *testing.T) {
	if !reviewedQueryPackage("ucupa-core", coreQueryDirectory) ||
		reviewedQueryPackage("ucupa-facet", coreQueryDirectory) ||
		reviewedQueryPackage("ucupa-core", coreQueryDirectory+"/child") {
		t.Fatal("Core query registration must identify exactly one package")
	}
	for _, name := range coreQuerySourceFiles {
		file := coreQueryDirectory + "/" + name
		if got := checkSourceLocation(file); got != "" {
			t.Errorf("reviewed Core query source %s: %s", file, got)
		}
	}
	if !reviewedCoreQueryTestPackage("ucupa-core", coreQueryIntegrationTestDirectory) ||
		reviewedCoreQueryTestPackage("ucupa-facet", coreQueryIntegrationTestDirectory) ||
		reviewedCoreQueryTestPackage("ucupa-core", coreQueryIntegrationTestDirectory+"/nested") {
		t.Fatal("Core query integration test registration must identify exactly one package")
	}
	for _, file := range []string{
		coreQueryIntegrationTestDirectory + "/snapshot_test.go",
		coreQueryIntegrationTestDirectory + "/persistence_test.go",
		coreQueryIntegrationTestDirectory + "/region_test.go",
		coreQueryIntegrationTestDirectory + "/authorization_test.go",
		coreQueryIntegrationTestDirectory + "/depth_test.go",
	} {
		if got := checkSourceLocation(file); got != "" {
			t.Errorf("reviewed Core query integration test source %s: %s", file, got)
		}
	}
	for _, file := range []string{
		coreQueryIntegrationTestDirectory + "/extra_test.go",
		coreQueryIntegrationTestDirectory + "/snapshot.go",
		coreQueryIntegrationTestDirectory + "/persistence.go",
		coreQueryIntegrationTestDirectory + "/nested/persistence_test.go",
		coreQueryIntegrationTestDirectory + "/nested/snapshot_test.go",
	} {
		if got := checkSourceLocation(file); got == "" {
			t.Errorf("unreviewed Core query integration test source accepted: %s", file)
		}
	}
	for _, file := range []string{
		coreQueryDirectory + "/query.go",
		coreQueryDirectory + "/extra.go",
		coreQueryDirectory + "/snapshot_lifecycle_test.go",
		coreQueryDirectory + "/child/view.go",
		"ucupa-core/internal/query/fact/view.go",
		"ucupa-facet/internal/query/view.go",
	} {
		if got := checkSourceLocation(file); got == "" {
			t.Errorf("unreviewed query source accepted: %s", file)
		}
	}

	stdlib := testStdlib()
	for _, dependency := range []string{
		"errors", "math", "reflect", "sort", "sync", "time",
	} {
		stdlib[dependency] = true
	}
	for _, dependency := range []string{
		"ucupa/shared/values",
		"ucupa/ucupa-core/contract",
		"ucupa/ucupa-core/internal/commit",
		"ucupa/ucupa-core/internal/element/fact",
		"ucupa/ucupa-core/internal/mount/fact",
		"ucupa/ucupa-core/internal/query/request",
		"ucupa/ucupa-core/internal/query/scan",
		"ucupa/ucupa-core/internal/token",
	} {
		if got := checkDependency(coreQueryDirectory+"/operation.go", dependency, "ucupa", false, stdlib); got != "" {
			t.Errorf("allowed Core query dependency %s: %s", dependency, got)
		}
	}
	for _, dependency := range []string{
		"ucupa/ucupa-core",
		"ucupa/ucupa-core/instance",
		"ucupa/ucupa-core/internal/access",
		"ucupa/ucupa-core/internal/element",
		"ucupa/ucupa-core/internal/facet/fact",
		"ucupa/ucupa-core/internal/mount",
		"ucupa/ucupa-core/internal/store",
		"ucupa/ucupa-core/transport/grpc",
		"ucupa/ucupa-core/sdk/go",
		"ucupa/ucupa-facet/contract",
		"ucupa/ucupa-realm",
		"ucupa/composition",
		"ucupa/acceptance",
		"go.etcd.io/bbolt",
	} {
		if got := checkDependency(coreQueryDirectory+"/operation.go", dependency, "ucupa", false, stdlib); got == "" {
			t.Errorf("forbidden Core query dependency accepted: %s", dependency)
		}
	}
	for _, dependency := range []string{
		"errors", "fmt", "testing", "time",
		"ucupa/shared/values",
		"ucupa/ucupa-core/instance",
		"ucupa/ucupa-core/contract",
	} {
		if got := checkDependency(coreQueryIntegrationTestDirectory+"/snapshot_test.go", dependency, "ucupa", true, stdlib); got != "" {
			t.Errorf("allowed Core query integration-test dependency %s: %s", dependency, got)
		}
	}
	for _, dependency := range []string{
		"ucupa/ucupa-core/internal/commit",
		"ucupa/ucupa-core/internal/element",
		"ucupa/ucupa-core/internal/query",
		"ucupa/ucupa-core/transport/grpc",
		"ucupa/ucupa-core/sdk/go",
		"ucupa/ucupa-facet/contract",
		"ucupa/composition",
		"ucupa/acceptance",
	} {
		if got := checkDependency(coreQueryIntegrationTestDirectory+"/snapshot_test.go", dependency, "ucupa", true, stdlib); got == "" {
			t.Errorf("forbidden Core query integration-test dependency accepted: %s", dependency)
		}
	}
	if got := checkDependency(coreQueryIntegrationTestDirectory+"/snapshot_test.go", "ucupa/ucupa-core", "ucupa", false, stdlib); got == "" {
		t.Fatal("Core query integration-test package was accepted as production source")
	}
	if got := checkDependency("ucupa-core/core.go", "ucupa/ucupa-core/tests/query", "ucupa", false, stdlib); got == "" {
		t.Fatal("production Core source imported the query integration-test package")
	}

	for _, tc := range []struct {
		source, dependency string
		allowed            bool
		test               bool
	}{
		{"ucupa-core/core.go", "ucupa/ucupa-core/internal/query", true, false},
		{"ucupa-core/transport/grpc/server.go", "ucupa/ucupa-core/internal/query", false, false},
		{"ucupa-core/sdk/go/client.go", "ucupa/ucupa-core/internal/query", false, false},
		{"composition/ports.go", "ucupa/ucupa-core/internal/query", false, false},
		{"ucupa-core/internal/commit/state.go", "ucupa/ucupa-core/internal/query", false, false},
		{"ucupa-core/internal/element/fact/state.go", "ucupa/ucupa-core/internal/query", false, false},
		{"ucupa-core/internal/mount/fact/state.go", "ucupa/ucupa-core/internal/query", false, false},
		{"ucupa-core/internal/facet/fact/state.go", "ucupa/ucupa-core/internal/query", false, false},
		{"ucupa-core/internal/query/request/canonical.go", "ucupa/ucupa-core/internal/query", false, false},
		{"ucupa-core/internal/query/scan/candidate.go", "ucupa/ucupa-core/internal/query", false, false},
		{"ucupa-core/contract/query.go", "ucupa/ucupa-core/internal/query", false, false},
		{"ucupa-core/internal/query/query_entry_test.go", "ucupa/ucupa-core/internal/query", true, true},
		{"ucupa-core/internal/query/query_entry_test.go", "ucupa/ucupa-core/internal/element", true, true},
		{"ucupa-core/internal/query/operation.go", "ucupa/ucupa-core/internal/element", false, false},
		{"ucupa-core/internal/query/snapshot_internal_test.go", "ucupa/ucupa-core/internal/element", false, true},
	} {
		got := checkDependency(tc.source, tc.dependency, "ucupa", tc.test, stdlib)
		if (got == "") != tc.allowed {
			t.Errorf("%s -> %s: allowed=%v got=%q", tc.source, tc.dependency, tc.allowed, got)
		}
	}

	if got := checkDependency("ucupa-core/internal/query/scan/element.go", "ucupa/ucupa-core/internal/facet/fact", "ucupa", false, stdlib); got == "" {
		t.Fatal("deleted scan Element projection retained its Facet dependency exception")
	}
	if got := checkDependency("ucupa-core/internal/query/scan/candidate.go", "ucupa/ucupa-core/internal/facet/fact", "ucupa", false, stdlib); got == "" {
		t.Fatal("unrelated scan candidate acquired a Facet dependency")
	}
}

func TestCoreQueryPublicSurfaceDoesNotExposeOwnerState(t *testing.T) {
	working, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	root, err := repositoryRoot(working)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range coreQuerySourceFiles {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		file := coreQueryDirectory + "/" + name
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(file)))
		if err != nil {
			t.Fatalf("read %s: %v", file, err)
		}
		parsed, err := parser.ParseFile(token.NewFileSet(), file, data, parser.ParseComments)
		if err != nil {
			t.Fatalf("parse %s: %v", file, err)
		}
		checkQueryPublicDeclarations(t, file, parsed)
	}
}

func TestQueryPersistenceDatabaseAccessIsTestLocal(t *testing.T) {
	stdlib := testStdlib()
	for _, tc := range []struct {
		file    string
		test    bool
		allowed bool
	}{
		{coreQueryIntegrationTestDirectory + "/persistence_test.go", true, true},
		{coreQueryIntegrationTestDirectory + "/persistence_test.go", false, false},
		{coreQueryIntegrationTestDirectory + "/snapshot_test.go", true, false},
		{coreQueryIntegrationTestDirectory + "/extra_test.go", true, false},
		{"ucupa-core/tests/element/fixture_test.go", true, false},
		{coreQueryDirectory + "/snapshot_internal_test.go", true, false},
	} {
		got := checkDependency(tc.file, "go.etcd.io/bbolt", "ucupa", tc.test, stdlib)
		if (got == "") != tc.allowed {
			t.Errorf("%s test=%v: database allowed=%v got=%q", tc.file, tc.test, tc.allowed, got)
		}
	}
	for _, dependency := range []string{
		"ucupa/ucupa-core/internal/commit", "ucupa/ucupa-core/internal/store",
		"ucupa/ucupa-core/internal/query", "ucupa/ucupa-core/internal/element",
		"ucupa/ucupa-core/transport/grpc", "ucupa/ucupa-core/sdk/go",
		"ucupa/ucupa-facet/contract", "ucupa/ucupa-realm",
	} {
		if got := checkDependency(coreQueryIntegrationTestDirectory+"/persistence_test.go", dependency, "ucupa", true, stdlib); got == "" {
			t.Errorf("persistence test leaked into %s", dependency)
		}
	}
}

func checkQueryPublicDeclarations(t *testing.T, file string, parsed *ast.File) {
	t.Helper()
	for _, declaration := range parsed.Decls {
		switch declaration := declaration.(type) {
		case *ast.GenDecl:
			for _, specification := range declaration.Specs {
				typeSpec, ok := specification.(*ast.TypeSpec)
				if !ok || !ast.IsExported(typeSpec.Name.Name) {
					continue
				}
				if typeSpec.Name.Name == "State" || typeSpec.Name.Name == "Store" || typeSpec.Name.Name == "Registry" || typeSpec.Name.Name == "SnapshotRegistry" {
					t.Errorf("%s exports owner type %s", file, typeSpec.Name.Name)
				}
				structType, ok := typeSpec.Type.(*ast.StructType)
				if !ok {
					continue
				}
				for _, field := range structType.Fields.List {
					for _, name := range field.Names {
						if name.Name == "State" || name.Name == "Store" || name.Name == "DB" || name.Name == "Database" || name.Name == "PublicKey" || name.Name == "SecretKey" || name.Name == "PrivateKey" || name.Name == "Registry" || name.Name == "SnapshotRegistry" {
							t.Errorf("%s exports owner or key field %s.%s", file, typeSpec.Name.Name, name.Name)
						}
					}
				}
			}
		case *ast.FuncDecl:
			if declaration.Name == nil || !ast.IsExported(declaration.Name.Name) {
				continue
			}
			if declaration.Name.Name == "State" || declaration.Name.Name == "Store" || declaration.Name.Name == "DB" || declaration.Name.Name == "Database" || declaration.Name.Name == "Registry" || declaration.Name.Name == "SnapshotRegistry" || declaration.Name.Name == "Entries" || declaration.Name.Name == "Views" || declaration.Name.Name == "Key" || declaration.Name.Name == "PublicKey" || declaration.Name.Name == "SecretKey" || declaration.Name.Name == "PrivateKey" {
				t.Errorf("%s exports forbidden query getter %s", file, declaration.Name.Name)
			}
			if queryHasCallbackType(declaration.Type.Params) {
				t.Errorf("%s exports a callback through %s", file, declaration.Name.Name)
			}
			if queryHasPrivateOwnerType(declaration.Type.Params) || queryHasPrivateOwnerType(declaration.Type.Results) {
				t.Errorf("%s exposes query owner internals through %s", file, declaration.Name.Name)
			}
			if queryHasMapType(declaration.Type.Results) {
				t.Errorf("%s exposes mutable registry state through %s", file, declaration.Name.Name)
			}
		}
	}
}

func queryHasCallbackType(fields *ast.FieldList) bool {
	if fields == nil {
		return false
	}
	for _, field := range fields.List {
		found := false
		ast.Inspect(field.Type, func(node ast.Node) bool {
			if _, ok := node.(*ast.FuncType); ok {
				found = true
				return false
			}
			return true
		})
		if found {
			return true
		}
	}
	return false
}

func queryHasPrivateOwnerType(fields *ast.FieldList) bool {
	if fields == nil {
		return false
	}
	for _, field := range fields.List {
		found := false
		ast.Inspect(field.Type, func(node ast.Node) bool {
			if ident, ok := node.(*ast.Ident); ok {
				switch ident.Name {
				case "snapshotRegistry", "readView", "coreState", "localStore":
					found = true
				}
			}
			return !found
		})
		if found {
			return true
		}
	}
	return false
}

func queryHasMapType(fields *ast.FieldList) bool {
	if fields == nil {
		return false
	}
	for _, field := range fields.List {
		found := false
		ast.Inspect(field.Type, func(node ast.Node) bool {
			if _, ok := node.(*ast.MapType); ok {
				found = true
				return false
			}
			return true
		})
		if found {
			return true
		}
	}
	return false
}
