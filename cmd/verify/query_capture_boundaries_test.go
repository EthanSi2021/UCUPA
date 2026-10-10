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

func TestReviewedCoreQueryTestBridgeSurface(t *testing.T) {
	const file = "ucupa-core/internal/query/export_test.go"
	if got := checkSourceLocation(file); got != "" {
		t.Fatalf("reviewed query test bridge rejected: %s", got)
	}
	if got := checkDependency(file, "ucupa/ucupa-core/internal/query/connected", "ucupa", true, testStdlib()); got == "" {
		t.Fatal("query test bridge may not import connected operations")
	}
	root, err := repositoryRootForQueryBridge(t)
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
	var recordType *ast.StructType
	var functions []*ast.FuncDecl
	for _, declaration := range parsed.Decls {
		switch declaration := declaration.(type) {
		case *ast.GenDecl:
			if declaration.Tok != token.TYPE || len(declaration.Specs) != 1 {
				t.Fatalf("%s contains a non-type declaration", file)
			}
			specification, ok := declaration.Specs[0].(*ast.TypeSpec)
			if !ok || specification.Name.Name != "ResourceCountsForTest" {
				t.Fatalf("%s contains an unexpected type declaration", file)
			}
			var okStruct bool
			recordType, okStruct = specification.Type.(*ast.StructType)
			if !okStruct {
				t.Fatalf("%s test bridge is not a struct value", file)
			}
		case *ast.FuncDecl:
			if declaration.Recv != nil || declaration.Name == nil {
				t.Fatalf("%s contains a method or unnamed function", file)
			}
			functions = append(functions, declaration)
		default:
			t.Fatalf("%s contains an unexpected declaration", file)
		}
	}
	if len(functions) != 1 || functions[0].Name.Name != "ReadResourceCountsForTest" {
		t.Fatalf("%s bridge functions are not exactly ReadResourceCountsForTest", file)
	}
	expectedFields := map[string]string{
		"Views": "int", "Handles": "int", "Active": "uint32", "Bytes": "uint64",
	}
	if recordType == nil {
		t.Fatalf("%s bridge record type is missing", file)
	}
	if len(recordType.Fields.List) != len(expectedFields) {
		t.Fatalf("%s bridge field count changed: %d", file, len(recordType.Fields.List))
	}
	for _, field := range recordType.Fields.List {
		if len(field.Names) != 1 {
			t.Fatalf("%s bridge contains an embedded or multi-name field", file)
		}
		name := field.Names[0].Name
		want, ok := expectedFields[name]
		if !ok || queryBridgeTypeName(field.Type) != want {
			t.Fatalf("%s bridge field %s has unexpected type %s", file, name, queryBridgeTypeName(field.Type))
		}
		if queryBridgeContainsMutableType(field.Type) {
			t.Fatalf("%s bridge field %s exposes pointer or map state", file, name)
		}
		delete(expectedFields, name)
	}
	if len(expectedFields) != 0 {
		t.Fatalf("%s bridge lost fixed fields: %v", file, expectedFields)
	}
	function := functions[0]
	if len(function.Type.Params.List) != 1 || queryBridgeTypeName(function.Type.Params.List[0].Type) != "*Operations" {
		t.Fatalf("%s bridge parameter must be *Operations", file)
	}
	if function.Type.Results == nil || len(function.Type.Results.List) != 1 || queryBridgeTypeName(function.Type.Results.List[0].Type) != "ResourceCountsForTest" {
		t.Fatalf("%s bridge result must be a value ResourceCountsForTest", file)
	}
	if queryBridgeContainsMutableType(function.Type.Results.List[0].Type) {
		t.Fatalf("%s bridge result exposes pointer or map state", file)
	}
}

func TestCoreQueryTestBridgeIsAbsentFromProductionBuild(t *testing.T) {
	goTool, err := exec.LookPath("go")
	if err != nil {
		t.Fatal(err)
	}
	root, err := repositoryRootForQueryBridge(t)
	if err != nil {
		t.Fatal(err)
	}
	directory, err := os.MkdirTemp(filepath.Join(root, "ucupa-core"), ".query-bridge-build-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(directory)
	writeFixture(t, directory, "main.go", `package main
import querypkg "ucupa/ucupa-core/internal/query"
func main() {
 var operations *querypkg.Operations
 _ = querypkg.ReadResourceCountsForTest(operations)
}`)
	outputPath := filepath.Join(directory, "consumer.exe")
	command := exec.Command(goTool, "build", "-o", outputPath, filepath.Join(directory, "main.go"))
	command.Dir = root
	command.Env = commandEnvironment(false)
	out, err := command.CombinedOutput()
	if err == nil {
		t.Fatal("production go build unexpectedly included the query test bridge")
	}
	if !strings.Contains(string(out), "undefined: querypkg.ReadResourceCountsForTest") || strings.Contains(string(out), "use of internal package") {
		t.Fatalf("production go build did not reject the test-only bridge itself: %v\n%s", err, out)
	}
}

func TestReviewedCoreQueryCaptureResourceTestBoundary(t *testing.T) {
	const file = "ucupa-core/internal/query/connected_capture_resource_test.go"
	if got := checkSourceLocation(file); got != "" {
		t.Fatalf("reviewed query capture-resource test rejected: %s", got)
	}
	stdlib := testStdlib()
	for _, dependency := range []string{"context", "errors", "fmt", "reflect", "testing", "time"} {
		stdlib[dependency] = true
	}
	for _, dependency := range []string{
		"ucupa/shared/values",
		"ucupa/ucupa-core/contract",
		"ucupa/ucupa-core/contract/connected",
		"ucupa/ucupa-core/internal/commit",
		"ucupa/ucupa-core/internal/element",
		"ucupa/ucupa-core/internal/mount",
		"ucupa/ucupa-core/internal/query",
		"ucupa/ucupa-core/internal/query/connected",
		"ucupa/ucupa-core/internal/query/exchange",
	} {
		if got := checkDependency(file, dependency, "ucupa", true, stdlib); got != "" {
			t.Errorf("allowed query capture-resource dependency %s: %s", dependency, got)
		}
	}
	for _, dependency := range []string{
		"ucupa/ucupa-core",
		"ucupa/ucupa-core/internal/access",
		"ucupa/ucupa-core/internal/store",
		"ucupa/ucupa-core/internal/element/fact",
		"ucupa/ucupa-core/internal/mount/fact",
		"ucupa/ucupa-core/internal/query/scan",
		"ucupa/ucupa-core/internal/query/request",
		"ucupa/ucupa-core/internal/token",
		"ucupa/ucupa-core/transport/grpc",
		"ucupa/ucupa-core/sdk/go",
		"go.etcd.io/bbolt",
	} {
		if got := checkDependency(file, dependency, "ucupa", true, stdlib); got == "" {
			t.Errorf("forbidden query capture-resource dependency accepted: %s", dependency)
		}
	}
	if got := checkDependency("ucupa-core/internal/query/operation.go", "ucupa/ucupa-core/internal/query/connected", "ucupa", false, stdlib); got == "" {
		t.Fatal("production query source imported connected operations")
	}
	if got := checkDependency("ucupa-core/internal/query/export_test.go", "ucupa/ucupa-core/internal/query/connected", "ucupa", true, stdlib); got == "" {
		t.Fatal("query test bridge imported connected operations")
	}
}

func TestReviewedCoreQueryConnectedResourceTestBoundary(t *testing.T) {
	const file = "ucupa-core/internal/query/connected_resource_test.go"
	if got := checkSourceLocation(file); got != "" {
		t.Fatalf("reviewed query connected-resource test rejected: %s", got)
	}
	stdlib := testStdlib()
	for _, dependency := range []string{"context", "errors", "reflect", "testing"} {
		stdlib[dependency] = true
	}
	for _, dependency := range []string{
		"context",
		"errors",
		"reflect",
		"testing",
		"ucupa/shared/values",
		"ucupa/ucupa-core/contract",
		"ucupa/ucupa-core/internal/query",
	} {
		if got := checkDependency(file, dependency, "ucupa", true, stdlib); got != "" {
			t.Errorf("allowed query connected-resource dependency %s: %s", dependency, got)
		}
	}
	for _, dependency := range []string{
		"ucupa/ucupa-core",
		"ucupa/ucupa-core/internal/access",
		"ucupa/ucupa-core/internal/commit",
		"ucupa/ucupa-core/internal/element",
		"ucupa/ucupa-core/internal/element/fact",
		"ucupa/ucupa-core/internal/mount",
		"ucupa/ucupa-core/internal/query/connected",
		"ucupa/ucupa-core/internal/query/exchange",
		"ucupa/ucupa-core/internal/query/request",
		"ucupa/ucupa-core/internal/store",
		"ucupa/ucupa-core/internal/token",
		"ucupa/ucupa-core/transport/grpc",
		"ucupa/ucupa-core/sdk/go",
		"go.etcd.io/bbolt",
	} {
		if got := checkDependency(file, dependency, "ucupa", true, stdlib); got == "" {
			t.Errorf("forbidden query connected-resource dependency accepted: %s", dependency)
		}
	}
	if got := checkDependency("ucupa-core/internal/query/operation.go", "ucupa/ucupa-core/internal/query/connected", "ucupa", false, stdlib); got == "" {
		t.Fatal("production query source imported connected operations")
	}
}

func repositoryRootForQueryBridge(t *testing.T) (string, error) {
	t.Helper()
	working, err := os.Getwd()
	if err != nil {
		return "", err
	}
	return repositoryRoot(working)
}

func queryBridgeTypeName(expression ast.Expr) string {
	switch expression := expression.(type) {
	case *ast.Ident:
		return expression.Name
	case *ast.StarExpr:
		return "*" + queryBridgeTypeName(expression.X)
	default:
		return ""
	}
}

func queryBridgeContainsMutableType(expression ast.Expr) bool {
	found := false
	ast.Inspect(expression, func(node ast.Node) bool {
		switch node.(type) {
		case *ast.MapType, *ast.StarExpr:
			found = true
			return false
		default:
			return !found
		}
	})
	return found
}
