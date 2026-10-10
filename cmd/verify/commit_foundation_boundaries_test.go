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

func TestReviewedCoreFoundationTestDependencies(t *testing.T) {
	stdlib := testStdlib()
	for _, dependency := range []string{
		"bytes", "crypto/rand", "encoding/binary", "encoding/json", "errors", "fmt",
		"path/filepath", "testing",
	} {
		stdlib[dependency] = true
	}
	cases := []struct {
		file    string
		allowed []string
	}{
		{
			file: "ucupa-core/internal/commit/foundation_candidate_fixture_test.go",
			allowed: []string{
				"fmt", "testing", "ucupa/shared/values", "ucupa/ucupa-core/contract",
				"ucupa/ucupa-core/internal/commit/binding", "ucupa/ucupa-core/internal/element/fact",
			},
		},
		{
			file: "ucupa-core/internal/commit/foundation_candidate_test.go",
			allowed: []string{
				"testing", "ucupa/ucupa-core/contract", "ucupa/ucupa-core/internal/commit/binding",
				"ucupa/ucupa-core/internal/element/fact", "ucupa/ucupa-core/internal/facet/fact",
			},
		},
		{
			file: "ucupa-core/internal/commit/foundation_history_test.go",
			allowed: []string{
				"path/filepath", "testing", "ucupa/ucupa-core/contract", "ucupa/ucupa-core/internal/element/fact",
			},
		},
		{
			file: "ucupa-core/internal/commit/foundation_store_test.go",
			allowed: []string{
				"bytes", "encoding/binary", "encoding/json", "errors", "go.etcd.io/bbolt", "os", "path/filepath", "testing",
				"ucupa/ucupa-core/contract", "ucupa/ucupa-core/internal/element/fact",
			},
		},
		{
			file: "ucupa-core/internal/commit/foundation_store_fixture_test.go",
			allowed: []string{
				"crypto/rand", "encoding/json", "errors", "fmt", "go.etcd.io/bbolt", "testing",
				"ucupa/shared/values", "ucupa/ucupa-core/contract", "ucupa/ucupa-core/internal/commit/binding",
				"ucupa/ucupa-core/internal/element/fact",
			},
		},
		{
			file: "ucupa-core/internal/commit/foundation_replaced_test.go",
			allowed: []string{
				"errors", "fmt", "path/filepath", "testing", "ucupa/shared/values",
				"ucupa/ucupa-core/contract", "ucupa/ucupa-core/internal/commit", "ucupa/ucupa-core/internal/element",
			},
		},
		{
			file: "ucupa-core/internal/commit/foundation_fault_export_test.go",
			allowed: []string{
				"go.etcd.io/bbolt", "ucupa/shared/values", "ucupa/ucupa-core/contract",
			},
		},
	}
	for _, tc := range cases {
		if got := checkSourceLocation(tc.file); got != "" {
			t.Errorf("reviewed Core Foundation test %s rejected: %s", tc.file, got)
		}
		for _, dependency := range tc.allowed {
			if got := checkDependency(tc.file, dependency, "ucupa", true, stdlib); got != "" {
				t.Errorf("allowed Core Foundation dependency %s -> %s: %s", tc.file, dependency, got)
			}
		}
	}
	for _, tc := range []struct {
		file         string
		dependencies []string
	}{
		{
			file: "ucupa-core/internal/commit/foundation_candidate_fixture_test.go",
			dependencies: []string{
				"ucupa/ucupa-core/internal/commit", "ucupa/ucupa-core/internal/store", "ucupa/ucupa-core/internal/token",
				"ucupa/ucupa-core/internal/query", "ucupa/ucupa-core/transport/grpc", "ucupa/ucupa-core/sdk/go", "go.etcd.io/bbolt",
			},
		},
		{
			file: "ucupa-core/internal/commit/foundation_candidate_test.go",
			dependencies: []string{
				"ucupa/ucupa-core/internal/commit", "ucupa/ucupa-core/internal/store", "ucupa/ucupa-core/internal/token",
				"ucupa/ucupa-core/internal/query", "ucupa/ucupa-core/transport/grpc", "ucupa/ucupa-core/sdk/go", "go.etcd.io/bbolt",
			},
		},
		{
			file: "ucupa-core/internal/commit/foundation_history_test.go",
			dependencies: []string{
				"ucupa/ucupa-core/internal/commit", "ucupa/ucupa-core/internal/store", "ucupa/ucupa-core/internal/token",
				"ucupa/ucupa-core/internal/query", "ucupa/ucupa-core/transport/grpc", "ucupa/ucupa-core/sdk/go", "go.etcd.io/bbolt",
			},
		},
		{
			file: "ucupa-core/internal/commit/foundation_store_test.go",
			dependencies: []string{
				"ucupa/ucupa-core/internal/commit", "ucupa/ucupa-core/internal/store", "ucupa/ucupa-core/internal/token",
				"ucupa/ucupa-core/internal/query", "ucupa/ucupa-core/transport/grpc", "ucupa/ucupa-core/sdk/go",
			},
		},
		{
			file: "ucupa-core/internal/commit/foundation_store_fixture_test.go",
			dependencies: []string{
				"ucupa/ucupa-core/internal/commit", "ucupa/ucupa-core/internal/store", "ucupa/ucupa-core/internal/token",
				"ucupa/ucupa-core/internal/query", "ucupa/ucupa-core/transport/grpc", "ucupa/ucupa-core/sdk/go",
			},
		},
		{
			file: "ucupa-core/internal/commit/foundation_replaced_test.go",
			dependencies: []string{
				"ucupa/ucupa-core/internal/store", "ucupa/ucupa-core/internal/token", "ucupa/ucupa-core/internal/query",
				"ucupa/ucupa-core/transport/grpc", "ucupa/ucupa-core/sdk/go", "go.etcd.io/bbolt",
			},
		},
		{
			file: "ucupa-core/internal/commit/foundation_fault_export_test.go",
			dependencies: []string{
				"ucupa/ucupa-core/internal/commit", "ucupa/ucupa-core/internal/store", "ucupa/ucupa-core/internal/token",
				"ucupa/ucupa-core/internal/query", "ucupa/ucupa-core/transport/grpc", "ucupa/ucupa-core/sdk/go",
			},
		},
	} {
		for _, dependency := range tc.dependencies {
			if got := checkDependency(tc.file, dependency, "ucupa", true, stdlib); got == "" {
				t.Errorf("forbidden Core Foundation dependency accepted: %s -> %s", tc.file, dependency)
			}
		}
	}
	if got := checkSourceLocation("ucupa-core/internal/commit/foundation_extra_test.go"); got == "" {
		t.Fatal("unreviewed Core Foundation test source accepted")
	}
}

func TestPublicCommitDurabilityTestBoundaryIsExact(t *testing.T) {
	stdlib := testStdlib()
	stdlib["testing"] = true
	const directory = "ucupa-core/internal/commit/"
	cases := []struct {
		file            string
		allowed, denied []string
	}{
		{"public_durability_fixture_test.go", []string{
			"ucupa/shared/values", "ucupa/ucupa-core/contract", "ucupa/ucupa-core/internal/commit",
			"ucupa/ucupa-core/internal/access", "ucupa/ucupa-core/internal/access/entry",
			"ucupa/ucupa-core/internal/access/observation", "ucupa/ucupa-core/internal/access/registry",
			"ucupa/ucupa-core/internal/element", "ucupa/ucupa-core/internal/query",
			"ucupa/ucupa-core/internal/query/local", "ucupa/ucupa-core/internal/query/federation/source",
			"ucupa/ucupa-core/internal/query/federation/coordinator",
		}, []string{"ucupa/ucupa-core/instance", "ucupa/ucupa-core/internal/token", "ucupa/ucupa-core/internal/element/fact"}},
		{"public_durability_test.go", []string{
			"ucupa/shared/values", "ucupa/ucupa-core/contract", "ucupa/ucupa-core/internal/commit",
		}, []string{"ucupa/ucupa-core/internal/access/entry", "ucupa/ucupa-core/internal/query", "ucupa/ucupa-core/internal/element"}},
		{"public_lookup_test.go", []string{
			"ucupa/shared/values", "ucupa/ucupa-core/contract", "ucupa/ucupa-core/internal/commit/binding",
			"ucupa/ucupa-core/internal/element/fact", "ucupa/ucupa-core/internal/token",
		}, []string{"ucupa/ucupa-core/internal/access/entry", "ucupa/ucupa-core/internal/query", "ucupa/ucupa-core/internal/commit"}},
	}
	for _, test := range cases {
		file := directory + test.file
		if reason := checkSourceLocation(file); reason != "" {
			t.Errorf("%s: %s", file, reason)
		}
		for _, dependency := range append(test.allowed, "testing") {
			if reason := checkDependency(file, dependency, "ucupa", true, stdlib); reason != "" {
				t.Errorf("reviewed edge %s -> %s: %s", file, dependency, reason)
			}
		}
		denied := append(test.denied, "ucupa/ucupa-core", "ucupa/ucupa-core/internal/store", "go.etcd.io/bbolt",
			"ucupa/ucupa-core/transport/grpc", "ucupa/ucupa-core/sdk/go", "ucupa/ucupa-facet/instance")
		for _, dependency := range denied {
			if reason := checkDependency(file, dependency, "ucupa", true, stdlib); reason == "" {
				t.Errorf("unreviewed edge accepted %s -> %s", file, dependency)
			}
		}
	}
	if reason := checkSourceLocation(directory + "public_durability_extra_test.go"); reason == "" {
		t.Fatal("unreviewed durability fixture file accepted")
	}
}

func TestFoundationFaultExportSurfaceIsExact(t *testing.T) {
	const file = "ucupa-core/internal/commit/foundation_fault_export_test.go"
	root, err := repositoryRootForFoundation(t)
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
	want := map[string]struct {
		params  []string
		results []string
	}{
		"InstallAuthorityForTest": {
			params: []string{"*Owner", "values.FactAuthorityRef"}, results: []string{"error"},
		},
		"ReloadStateAbsentForTest": {
			params: []string{"*Owner"}, results: []string{"bool", "error"},
		},
		"StoredAuthorityAndOperationForTest": {
			params: []string{"*Owner", "contract.OperationID"}, results: []string{"values.FactAuthorityRef", "bool", "error"},
		},
	}
	found := map[string]bool{}
	for _, declaration := range parsed.Decls {
		function, ok := declaration.(*ast.FuncDecl)
		if declaration, isImport := declaration.(*ast.GenDecl); isImport && declaration.Tok == token.IMPORT {
			continue
		}
		if !ok || function.Recv != nil || function.Name == nil {
			t.Fatalf("%s contains a non-function, method or unnamed declaration", file)
		}
		expected, ok := want[function.Name.Name]
		if !ok || found[function.Name.Name] {
			t.Fatalf("%s contains unexpected or duplicate function %s", file, function.Name.Name)
		}
		found[function.Name.Name] = true
		if got := foundationBridgeFieldTypes(function.Type.Params); !sameStrings(got, expected.params) {
			t.Fatalf("%s %s params=%v, want=%v", file, function.Name.Name, got, expected.params)
		}
		if got := foundationBridgeFieldTypes(function.Type.Results); !sameStrings(got, expected.results) {
			t.Fatalf("%s %s results=%v, want=%v", file, function.Name.Name, got, expected.results)
		}
	}
	if len(found) != len(want) {
		t.Fatalf("%s exported bridge functions=%v, want exactly=%v", file, found, want)
	}
}

func TestFoundationFaultExportsAreAbsentFromProductionBuild(t *testing.T) {
	goTool, err := exec.LookPath("go")
	if err != nil {
		t.Fatal(err)
	}
	root, err := repositoryRootForFoundation(t)
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name string
		body string
	}{
		{
			name: "InstallAuthorityForTest",
			body: `
var owner *commitpkg.Owner
var replacement values.FactAuthorityRef
_ = commitpkg.InstallAuthorityForTest(owner, replacement)
`,
		},
		{
			name: "ReloadStateAbsentForTest",
			body: `
var owner *commitpkg.Owner
_, _ = commitpkg.ReloadStateAbsentForTest(owner)
`,
		},
		{
			name: "StoredAuthorityAndOperationForTest",
			body: `
var owner *commitpkg.Owner
var operation contract.OperationID
_, _, _ = commitpkg.StoredAuthorityAndOperationForTest(owner, operation)
`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			directory, err := os.MkdirTemp(filepath.Join(root, "ucupa-core"), ".commit-fault-build-")
			if err != nil {
				t.Fatal(err)
			}
			defer os.RemoveAll(directory)
			writeFixture(t, directory, "main.go", "package main\n\nimport (\n\tcontract \"ucupa/ucupa-core/contract\"\n\tcommitpkg \"ucupa/ucupa-core/internal/commit\"\n\t\"ucupa/shared/values\"\n)\n\nfunc main() {\nvar _ values.FactAuthorityRef\nvar _ contract.OperationID\n"+tc.body+"}\n")
			outputPath := filepath.Join(directory, "consumer.exe")
			command := exec.Command(goTool, "build", "-o", outputPath, filepath.Join(directory, "main.go"))
			command.Dir = root
			command.Env = commandEnvironment(false)
			output, err := command.CombinedOutput()
			if err == nil {
				t.Fatal("production build unexpectedly included a Foundation fault export")
			}
			text := string(output)
			if !strings.Contains(text, "undefined: commitpkg."+tc.name) || strings.Contains(text, "use of internal package") {
				t.Fatalf("production build did not reject %s as an undefined test-only symbol: %v\n%s", tc.name, err, text)
			}
		})
	}
}

func TestReviewedCoreFoundationSurfaceTests(t *testing.T) {
	stdlib := testStdlib()
	for _, dependency := range []string{"reflect", "time"} {
		stdlib[dependency] = true
	}
	for _, tc := range []struct {
		file      string
		allowed   []string
		forbidden []string
	}{
		{
			file: "ucupa-core/contract/config_test.go",
			allowed: []string{
				"reflect", "testing", "time", "ucupa/shared/values", "ucupa/ucupa-core/contract",
			},
			forbidden: []string{
				"ucupa/ucupa-core/internal/commit", "ucupa/ucupa-core/internal/store", "ucupa/ucupa-core/internal/query",
				"ucupa/ucupa-core/transport/grpc", "ucupa/ucupa-core/sdk/go", "go.etcd.io/bbolt",
			},
		},
		{
			file: "ucupa-core/contract/connected_read_test.go",
			allowed: []string{
				"reflect", "testing", "ucupa/shared/values", "ucupa/ucupa-core/contract",
			},
			forbidden: []string{
				"ucupa/ucupa-core/internal/commit", "ucupa/ucupa-core/internal/store", "ucupa/ucupa-core/internal/query",
				"ucupa/ucupa-core/transport/grpc", "ucupa/ucupa-core/sdk/go", "go.etcd.io/bbolt",
			},
		},
		{
			file: "ucupa-core/tests/instance/surface_test.go",
			allowed: []string{
				"reflect", "testing", "ucupa/ucupa-core/instance", "ucupa/ucupa-core/contract",
			},
			forbidden: []string{
				"ucupa/ucupa-core/internal/commit", "ucupa/ucupa-core/internal/element", "ucupa/ucupa-core/internal/store",
				"ucupa/ucupa-core/internal/query", "ucupa/ucupa-core/transport/grpc", "ucupa/ucupa-core/sdk/go",
				"ucupa/ucupa-core/tests/element", "ucupa/ucupa-core/tests/query", "go.etcd.io/bbolt",
			},
		},
	} {
		if got := checkSourceLocation(tc.file); got != "" {
			t.Errorf("reviewed Core surface test %s rejected: %s", tc.file, got)
		}
		for _, dependency := range tc.allowed {
			if got := checkDependency(tc.file, dependency, "ucupa", true, stdlib); got != "" {
				t.Errorf("allowed Core surface dependency %s -> %s: %s", tc.file, dependency, got)
			}
		}
		for _, dependency := range tc.forbidden {
			if got := checkDependency(tc.file, dependency, "ucupa", true, stdlib); got == "" {
				t.Errorf("forbidden Core surface dependency accepted: %s -> %s", tc.file, dependency)
			}
		}
	}
	if got := checkSourceLocation("ucupa-core/tests/instance/surface_extra_test.go"); got == "" {
		t.Error("unreviewed instance surface test accepted")
	}
}

func repositoryRootForFoundation(t *testing.T) (string, error) {
	t.Helper()
	working, err := os.Getwd()
	if err != nil {
		return "", err
	}
	return repositoryRoot(working)
}

func foundationBridgeFieldTypes(fields *ast.FieldList) []string {
	if fields == nil {
		return nil
	}
	types := make([]string, 0, len(fields.List))
	for _, field := range fields.List {
		if len(field.Names) > 1 {
			return nil
		}
		types = append(types, foundationBridgeTypeName(field.Type))
	}
	return types
}

func foundationBridgeTypeName(expression ast.Expr) string {
	switch expression := expression.(type) {
	case *ast.Ident:
		return expression.Name
	case *ast.StarExpr:
		return "*" + foundationBridgeTypeName(expression.X)
	case *ast.SelectorExpr:
		return foundationBridgeTypeName(expression.X) + "." + expression.Sel.Name
	default:
		return ""
	}
}

func sameStrings(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for index := range got {
		if got[index] != want[index] {
			return false
		}
	}
	return true
}
