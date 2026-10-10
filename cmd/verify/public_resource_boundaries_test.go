package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
	"testing"
)

func TestPublicResourceRegressionBoundaryIsExact(t *testing.T) {
	const directory = "ucupa-core/internal/access/registry/"
	stdlib := testStdlib()
	for _, name := range []string{"context", "encoding/json", "errors", "fmt", "testing", "time"} {
		stdlib[name] = true
	}
	for file, dependencies := range publicResourceTestDependencies {
		file = directory + file
		if got := checkSourceLocation(file); got != "" {
			t.Errorf("source %s: %s", file, got)
		}
		for _, dependency := range dependencies {
			imported := moduleImport("ucupa", dependency)
			if got := checkDependency(file, imported, "ucupa", true, stdlib); got != "" {
				t.Errorf("%s -> %s: %s", file, imported, got)
			}
			if got := checkDependency(file, imported, "ucupa", false, stdlib); got == "" {
				t.Errorf("test-only dependency became production: %s -> %s", file, imported)
			}
		}
		for _, forbidden := range []string{"ucupa/ucupa-core/instance", "ucupa/ucupa-core/internal/store", "ucupa/ucupa-core/internal/element/fact", "ucupa/ucupa-core/internal/token", "ucupa/ucupa-core/transport/grpc", "ucupa/ucupa-core/sdk/go", "go.etcd.io/bbolt"} {
			if got := checkDependency(file, forbidden, "ucupa", true, stdlib); got == "" {
				t.Errorf("forbidden %s -> %s", file, forbidden)
			}
		}
	}
	for _, file := range []string{"registry.go", "public_resource_test.go", "public_resource_export_test.go", "unreviewed_test.go"} {
		if got := checkDependency(directory+file, "ucupa/ucupa-core/internal/access/entry", "ucupa", strings.HasSuffix(file, "_test.go"), stdlib); got == "" {
			t.Errorf("non-fixture imported public entry: %s", file)
		}
	}
	if got := checkSourceLocation(directory + "public_resource_extra_test.go"); got == "" {
		t.Fatal("unreviewed source accepted")
	}
}

func TestPublicFederationScenariosUseOnlyActualPublicEntries(t *testing.T) {
	const directory = "ucupa-core/tests/federation/"
	stdlib := testStdlib()
	for _, name := range []string{"context", "errors", "fmt", "reflect", "sync", "testing", "time"} {
		stdlib[name] = true
	}
	for name, dependencies := range coreFederationScenarioDependencies {
		file := directory + name
		if got := checkSourceLocation(file); got != "" {
			t.Fatal(file, got)
		}
		for _, dep := range dependencies {
			imported := moduleImport("ucupa", dep)
			if got := checkDependency(file, imported, "ucupa", true, stdlib); got != "" {
				t.Error(file, imported, got)
			}
			if got := checkDependency(file, imported, "ucupa", false, stdlib); got == "" {
				t.Error("scenario became production", file, dep)
			}
		}
		assertActualImportsAccepted(t, file)
		for _, forbidden := range []string{"ucupa/ucupa-core", "ucupa/ucupa-core/internal/commit", "ucupa/ucupa-core/internal/query", "ucupa/ucupa-core/internal/access/registry", "ucupa/ucupa-core/internal/query/federation/source", "ucupa/ucupa-core/internal/access/value", "ucupa/ucupa-core/internal/token", "ucupa/ucupa-core/internal/store", "ucupa/ucupa-core/transport/grpc", "ucupa/ucupa-core/sdk/go", "ucupa/ucupa-core/tests/query", "ucupa/ucupa-facet/instance", "go.etcd.io/bbolt"} {
			if got := checkDependency(file, forbidden, "ucupa", true, stdlib); got == "" {
				t.Error("private edge", file, forbidden)
			}
		}
		if name != "fixture_test.go" && checkDependency(file, "ucupa/composition", "ucupa", true, stdlib) == "" {
			t.Error("composition outside fixture", file)
		}
	}
	if got := checkSourceLocation(directory + "unreviewed_test.go"); got == "" {
		t.Fatal("unreviewed scenario accepted")
	}
	for _, file := range []string{directory + "unreviewed_test.go", "ucupa-core/internal/query/runtime.go", "ucupa-core/transport/grpc/server.go"} {
		if got := checkDependency(file, "ucupa/ucupa-core/tests/federation", "ucupa", true, stdlib); got == "" {
			t.Fatal("scenario helper import accepted", file)
		}
	}
}

func TestPublicResourceTestBridgeIsFixedAndPrivate(t *testing.T) {
	root, err := currentRepositoryRoot(t)
	if err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(root, "ucupa-core/internal/access/registry/public_resource_export_test.go")
	parsed, err := parser.ParseFile(token.NewFileSet(), file, nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]struct{ parameters, results []string }{
		"SetPublicResourceScenarioForTest": {[]string{"*Registry", "string"}, []string{"bool"}},
		"PublicResourceCountsForTest":      {[]string{"*Registry"}, []string{"int", "uint64"}},
	}
	var scenarios []string
	for _, declaration := range parsed.Decls {
		if group, ok := declaration.(*ast.GenDecl); ok && group.Tok == token.IMPORT {
			continue
		}
		function, ok := declaration.(*ast.FuncDecl)
		if !ok || function.Recv != nil {
			t.Fatal("test seam gained state, type or method")
		}
		shape, ok := want[function.Name.Name]
		if !ok || !sameStrings(foundationBridgeFieldTypes(function.Type.Params), shape.parameters) || !sameStrings(foundationBridgeFieldTypes(function.Type.Results), shape.results) {
			t.Fatalf("unexpected test seam %s", function.Name.Name)
		}
		delete(want, function.Name.Name)
		if function.Name.Name == "SetPublicResourceScenarioForTest" {
			ast.Inspect(function.Body, func(node ast.Node) bool {
				if clause, ok := node.(*ast.CaseClause); ok {
					for _, condition := range clause.List {
						literal, ok := condition.(*ast.BasicLit)
						if !ok || literal.Kind != token.STRING {
							t.Fatal("nonliteral fault selector")
						}
						scenarios = append(scenarios, literal.Value)
					}
				}
				return true
			})
		}
	}
	if len(want) != 0 || !sameStrings(scenarios, []string{`"queue-one"`, `"working-memory"`, `"expired"`}) {
		t.Fatalf("fault surface changed: missing=%v cases=%v", want, scenarios)
	}
	for _, symbol := range []string{"SetPublicResourceScenarioForTest", "PublicResourceCountsForTest"} {
		output, err := buildRegistryConsumer(t, "package main\nimport registry \"ucupa/ucupa-core/internal/access/registry\"\nvar _ = registry."+symbol+"\nfunc main() {}\n")
		if err == nil || !strings.Contains(output, "undefined: registry."+symbol) || strings.Contains(output, "use of internal package") {
			t.Fatalf("production must reject test symbol %s itself: %v\n%s", symbol, err, output)
		}
	}
}
