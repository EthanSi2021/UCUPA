package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

const coreCommitDirectory = "ucupa-core/internal/commit"

var coreCommitSourceFiles = []string{
	"element.go",
	"facet_history.go",
	"facet_state.go",
	"facet_store.go",
	"facet_transition.go",
	"foundation_candidate_fixture_test.go",
	"foundation_candidate_test.go",
	"foundation_fault_export_test.go",
	"foundation_history_test.go",
	"foundation_replaced_test.go",
	"foundation_store_fixture_test.go",
	"foundation_store_test.go",
	"history.go",
	"mount_address.go",
	"mount_guard.go",
	"mount_guard_fixture_test.go",
	"mount_guard_proof_export_test.go",
	"mount_guard_read_test.go",
	"mount_guard_validation_test.go",
	"mount_guard_write_test.go",
	"mount_history.go",
	"mount_state.go",
	"mount_store_commit_test.go",
	"mount_store_event.go",
	"mount_transition.go",
	"mount_transition_peer.go",
	"mount_validation.go",
	"element_store_test.go",
	"export_test.go",
	"projection_authorization_test.go",
	"projection_fault_export_test.go",
	"oracle_boundary_test.go",
	"oracle_fault_export_test.go",
	"owner.go",
	"owner_publication_test.go",
	"public_element.go",
	"public_facet.go",
	"public_mount.go",
	"read_view.go",
	"state.go",
	"store.go",
	"store_codec.go",
	"store_load.go",
}

func TestReviewedCoreCommitOwnerBoundary(t *testing.T) {
	if !reviewedCommitPackage("ucupa-core", coreCommitDirectory) ||
		reviewedCommitPackage("ucupa-facet", coreCommitDirectory) ||
		reviewedCommitPackage("ucupa-core", coreCommitDirectory+"/child") {
		t.Fatal("Core commit registration must identify exactly one package")
	}
	for _, name := range coreCommitSourceFiles {
		file := coreCommitDirectory + "/" + name
		if got := checkSourceLocation(file); got != "" {
			t.Errorf("reviewed Core commit source %s: %s", file, got)
		}
	}
	for _, file := range []string{
		coreCommitDirectory + "/mount_store_messages.go",
		coreCommitDirectory + "/mount_store_values.go",
		coreCommitDirectory + "/commit.go",
		coreCommitDirectory + "/child/state.go",
		"ucupa-facet/internal/commit/state.go",
	} {
		if got := checkSourceLocation(file); got == "" {
			t.Errorf("unreviewed commit source accepted: %s", file)
		}
	}

	stdlib := testStdlib()
	for _, dependency := range []string{
		"bytes", "crypto/rand", "encoding/hex", "encoding/json", "errors", "fmt",
		"maps", "math", "os", "slices", "sort", "sync", "time",
	} {
		stdlib[dependency] = true
	}
	for _, dependency := range []string{
		"ucupa/shared/values",
		"ucupa/ucupa-core/contract",
		"ucupa/ucupa-core/internal/access/observe",
		"ucupa/ucupa-core/internal/commit/binding",
		"ucupa/ucupa-core/internal/element/fact",
		"ucupa/ucupa-core/internal/facet/fact",
		"ucupa/ucupa-core/internal/geometry",
		"ucupa/ucupa-core/internal/mount/fact",
		"ucupa/ucupa-core/internal/store",
		"ucupa/ucupa-core/internal/token",
	} {
		if got := checkDependency(coreCommitDirectory+"/state.go", dependency, "ucupa", false, stdlib); got != "" {
			t.Errorf("allowed Core commit dependency %s: %s", dependency, got)
		}
	}
	for _, dependency := range []string{
		"ucupa/ucupa-core",
		"ucupa/ucupa-core/instance",
		"ucupa/ucupa-core/internal/access",
		"ucupa/ucupa-core/internal/access/value",
		"ucupa/ucupa-core/internal/commit",
		"ucupa/ucupa-core/internal/query/scan",
		"ucupa/ucupa-core/transport/grpc",
		"ucupa/ucupa-core/sdk/go",
		"ucupa/ucupa-facet/contract",
		"ucupa/ucupa-realm",
		"ucupa/composition",
		"ucupa/acceptance",
		"go.etcd.io/bbolt/internal/common",
	} {
		if got := checkDependency(coreCommitDirectory+"/state.go", dependency, "ucupa", false, stdlib); got == "" {
			t.Errorf("forbidden Core commit dependency accepted: %s", dependency)
		}
	}
	for _, source := range []string{"store.go", "store_load.go", "mount_store_event.go"} {
		file := coreCommitDirectory + "/" + source
		if got := checkDependency(file, "go.etcd.io/bbolt", "ucupa", false, stdlib); got != "" {
			t.Errorf("Store database dependency rejected for %s: %s", file, got)
		}
	}
	for _, source := range []string{"state.go", "owner.go", "mount_state.go", "mount_store_commit_test.go"} {
		file := coreCommitDirectory + "/" + source
		if got := checkDependency(file, "go.etcd.io/bbolt", "ucupa", strings.HasSuffix(source, "_test.go"), stdlib); got == "" {
			t.Errorf("non-Store commit source acquired database dependency: %s", file)
		}
	}
	if got := checkDependency(coreCommitDirectory+"/element_store_test.go", "ucupa/ucupa-core/internal/element", "ucupa", true, stdlib); got != "" {
		t.Fatalf("the dedicated Element/Store composition fixture was rejected: %s", got)
	}
	for _, source := range []string{"owner.go", "owner_publication_test.go", "export_test.go"} {
		file := coreCommitDirectory + "/" + source
		if got := checkDependency(file, "ucupa/ucupa-core/internal/element", "ucupa", strings.HasSuffix(source, "_test.go"), stdlib); got == "" {
			t.Errorf("unapproved Core commit source imported Element operations: %s", file)
		}
	}
}

func TestCommitExportTestSurfaceIsTestOnlyAndNarrow(t *testing.T) {
	working, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	root, err := repositoryRoot(working)
	if err != nil {
		t.Fatal(err)
	}
	file := coreCommitDirectory + "/export_test.go"
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(file)))
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := parser.ParseFile(token.NewFileSet(), file, data, parser.ParseComments)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, declaration := range parsed.Decls {
		function, ok := declaration.(*ast.FuncDecl)
		if !ok || function.Recv != nil || function.Name == nil {
			t.Fatalf("%s contains a non-function or method declaration", file)
		}
		names = append(names, function.Name.Name)
	}
	sort.Strings(names)
	if got := strings.Join(names, ","); got != "InjectWriteFailureForTest,MatchesStoreForTest" {
		t.Fatalf("test-only commit export surface: %q", got)
	}
}

func TestCoreCommitOwnerPublicSurfaceDoesNotExposeAggregateInternals(t *testing.T) {
	working, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	root, err := repositoryRoot(working)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range coreCommitSourceFiles {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		file := coreCommitDirectory + "/" + name
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(file)))
		if err != nil {
			t.Fatalf("read %s: %v", file, err)
		}
		parsed, err := parser.ParseFile(token.NewFileSet(), file, data, parser.ParseComments)
		if err != nil {
			t.Fatalf("parse %s: %v", file, err)
		}
		checkCommitPublicDeclarations(t, file, parsed)
	}
}

func checkCommitPublicDeclarations(t *testing.T, file string, parsed *ast.File) {
	t.Helper()
	for _, declaration := range parsed.Decls {
		switch declaration := declaration.(type) {
		case *ast.GenDecl:
			for _, specification := range declaration.Specs {
				typeSpec, ok := specification.(*ast.TypeSpec)
				if !ok || !ast.IsExported(typeSpec.Name.Name) {
					continue
				}
				switch value := typeSpec.Type.(type) {
				case *ast.StructType:
					for _, field := range value.Fields.List {
						for _, name := range field.Names {
							if name.Name == "State" || name.Name == "Store" || name.Name == "DB" || name.Name == "Database" || name.Name == "PublicKey" || name.Name == "SecretKey" || name.Name == "PrivateKey" {
								t.Errorf("%s exports aggregate or key field %s.%s", file, typeSpec.Name.Name, name.Name)
							}
						}
					}
				case *ast.FuncType:
					t.Errorf("%s exports a callback type %s", file, typeSpec.Name.Name)
				}
				if typeSpec.Name.Name == "State" || typeSpec.Name.Name == "Store" || typeSpec.Name.Name == "CoreState" || typeSpec.Name.Name == "LocalStore" {
					t.Errorf("%s exports aggregate type %s", file, typeSpec.Name.Name)
				}
			}
		case *ast.FuncDecl:
			if declaration.Name == nil || !ast.IsExported(declaration.Name.Name) {
				continue
			}
			if declaration.Name.Name == "State" || declaration.Name.Name == "Store" || declaration.Name.Name == "DB" || declaration.Name.Name == "Database" || declaration.Name.Name == "Key" || declaration.Name.Name == "PublicKey" || declaration.Name.Name == "SecretKey" || declaration.Name.Name == "PrivateKey" || declaration.Name.Name == "Sign" || declaration.Name.Name == "SignKey" {
				t.Errorf("%s exports forbidden owner method/function %s", file, declaration.Name.Name)
			}
			if hasCallbackType(declaration.Type.Params) {
				t.Errorf("%s exports transaction callback through %s", file, declaration.Name.Name)
			}
			if hasCommitInternalType(declaration.Type.Params) || hasCommitInternalType(declaration.Type.Results) {
				t.Errorf("%s exposes an internal aggregate or database type through %s", file, declaration.Name.Name)
			}
			if hasMapType(declaration.Type.Results) {
				t.Errorf("%s exposes mutable map state through %s", file, declaration.Name.Name)
			}
			if hasRawKeyResult(declaration.Type.Results) {
				t.Errorf("%s exposes a raw revision key through %s", file, declaration.Name.Name)
			}
		}
	}
}

func hasCallbackType(fields *ast.FieldList) bool {
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

func hasCommitInternalType(fields *ast.FieldList) bool {
	if fields == nil {
		return false
	}
	for _, field := range fields.List {
		found := ""
		ast.Inspect(field.Type, func(node ast.Node) bool {
			if found != "" {
				return false
			}
			switch node := node.(type) {
			case *ast.Ident:
				switch node.Name {
				case "coreState", "localStore", "mountState", "attachmentState", "retainedOperation", "coreCommit", "storeByteSink", "attachmentHistory", "mountHistory", "nextHopOperation", "mountSnapshotFacts":
					found = node.Name
				}
			case *ast.SelectorExpr:
				if packageName, ok := node.X.(*ast.Ident); ok && packageName.Name == "bbolt" {
					found = "bbolt"
				}
			}
			return true
		})
		if found != "" {
			return true
		}
	}
	return false
}

func hasMapType(fields *ast.FieldList) bool {
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

func hasRawKeyResult(fields *ast.FieldList) bool {
	if fields == nil {
		return false
	}
	for _, field := range fields.List {
		array, ok := field.Type.(*ast.ArrayType)
		if !ok || array.Len == nil {
			continue
		}
		length, ok := array.Len.(*ast.BasicLit)
		if !ok || length.Value != "32" {
			continue
		}
		if element, ok := array.Elt.(*ast.Ident); ok && (element.Name == "byte" || element.Name == "uint8") {
			return true
		}
	}
	return false
}
