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

func TestReviewedCoreFacetAndCollectSourceFiles(t *testing.T) {
	for _, file := range []string{
		"facet_fault_export_test.go", "facet_lifecycle_fixture_test.go",
		"facet_lifecycle_test.go", "facet_recovery_test.go",
	} {
		path := "ucupa-core/internal/commit/" + file
		if got := checkSourceLocation(path); got != "" {
			t.Errorf("reviewed Core Facet test %s rejected: %s", path, got)
		}
	}
	for _, file := range []string{
		"types.go", "sources.go", "evidence.go", "read.go", "path.go", "plan.go",
		"page.go", "closure.go", "observation.go", "exact.go", "collect_test.go",
	} {
		path := "ucupa-core/internal/query/federation/collect/" + file
		if got := checkSourceLocation(path); got != "" {
			t.Errorf("reviewed Federation collect source %s rejected: %s", path, got)
		}
	}
	if !reviewedCoreFederationCollectPackage("ucupa-core", "ucupa-core/internal/query/federation/collect") ||
		reviewedCoreFederationCollectPackage("ucupa-facet", "ucupa-core/internal/query/federation/collect") ||
		reviewedCoreFederationCollectPackage("ucupa-core", "ucupa-core/internal/query/federation/collect/child") {
		t.Fatal("Federation collect registration must identify exactly one package")
	}
	for _, file := range []string{
		"ucupa-core/internal/commit/facet_extra_test.go",
		"ucupa-core/internal/commit/facet/fault_export_test.go",
		"ucupa-core/internal/query/federation/collect/extra.go",
		"ucupa-core/internal/query/federation/collect/child/read.go",
	} {
		if got := checkSourceLocation(file); got == "" {
			t.Errorf("unreviewed source accepted: %s", file)
		}
	}
	if got := checkSourceLocation("ucupa-core/tests/query/attachment_snapshot_test.go"); got != "" {
		t.Fatalf("reviewed attachment snapshot test rejected: %s", got)
	}
	if got := checkSourceLocation("ucupa-core/tests/query/attachment_snapshot_extra_test.go"); got == "" {
		t.Fatal("unreviewed attachment snapshot test accepted")
	}
}

func TestReviewedCoreFacetTestDependencies(t *testing.T) {
	stdlib := guardOracleStdlib()
	stdlib["sync/atomic"] = true
	cases := []struct {
		file    string
		allowed []string
	}{
		{
			file: "ucupa-core/internal/commit/facet_lifecycle_fixture_test.go",
			allowed: []string{
				"context", "crypto/rand", "encoding/binary", "fmt", "path/filepath", "sync/atomic", "testing",
				"ucupa/shared/values", "ucupa/ucupa-core/contract", "ucupa/ucupa-core/internal/access",
				"ucupa/ucupa-core/internal/commit", "ucupa/ucupa-core/internal/element", "ucupa/ucupa-core/internal/facet",
			},
		},
		{
			file: "ucupa-core/internal/commit/facet_lifecycle_test.go",
			allowed: []string{
				"context", "errors", "testing", "ucupa/shared/values", "ucupa/ucupa-core/contract",
				"ucupa/ucupa-core/internal/commit",
			},
		},
		{
			file: "ucupa-core/internal/commit/facet_recovery_test.go",
			allowed: []string{
				"context", "errors", "sync/atomic", "testing", "ucupa/shared/values",
				"ucupa/ucupa-core/contract", "ucupa/ucupa-core/internal/commit",
			},
		},
		{
			file: "ucupa-core/internal/commit/facet_fault_export_test.go",
			allowed: []string{
				"encoding/json", "errors", "fmt", "go.etcd.io/bbolt", "ucupa/shared/values",
				"ucupa/ucupa-core/contract", "ucupa/ucupa-core/internal/facet/fact", "ucupa/ucupa-core/internal/store",
			},
		},
	}
	for _, tc := range cases {
		for _, dependency := range tc.allowed {
			if got := checkDependency(tc.file, dependency, "ucupa", true, stdlib); got != "" {
				t.Errorf("allowed Core Facet dependency %s -> %s: %s", tc.file, dependency, got)
			}
		}
	}
	for _, tc := range []struct {
		file, dependency string
	}{
		{"ucupa-core/internal/commit/facet_lifecycle_fixture_test.go", "ucupa/ucupa-core/internal/store"},
		{"ucupa-core/internal/commit/facet_lifecycle_fixture_test.go", "ucupa/ucupa-core/internal/query"},
		{"ucupa-core/internal/commit/facet_lifecycle_fixture_test.go", "ucupa/ucupa-core/internal/token"},
		{"ucupa-core/internal/commit/facet_lifecycle_test.go", "ucupa/ucupa-core/internal/element"},
		{"ucupa-core/internal/commit/facet_recovery_test.go", "go.etcd.io/bbolt"},
		{"ucupa-core/internal/commit/facet_fault_export_test.go", "ucupa/ucupa-core/internal/commit"},
		{"ucupa-core/internal/commit/facet_fault_export_test.go", "ucupa/ucupa-core/internal/facet"},
		{"ucupa-core/internal/commit/facet_fault_export_test.go", "ucupa/ucupa-core/internal/token"},
		{"ucupa-core/internal/commit/facet_fault_export_test.go", "ucupa/ucupa-core/transport/grpc"},
	} {
		if got := checkDependency(tc.file, tc.dependency, "ucupa", true, stdlib); got == "" {
			t.Errorf("forbidden Core Facet dependency accepted: %s -> %s", tc.file, tc.dependency)
		}
	}
}

func TestReviewedCoreFederationCollectDependencies(t *testing.T) {
	stdlib := guardOracleStdlib()
	stdlib["slices"] = true
	stdlib["sort"] = true
	cases := map[string][]string{
		"closure.go":      {"ucupa/shared/values", "ucupa/ucupa-core/contract/federation", "ucupa/ucupa-core/internal/query/federation/transcript"},
		"evidence.go":     {"ucupa/shared/values", "ucupa/ucupa-core/contract/federation"},
		"exact.go":        {"context", "ucupa/shared/values", "ucupa/ucupa-core/contract/federation"},
		"observation.go":  {"context", "errors", "ucupa/shared/values", "ucupa/ucupa-core/contract/federation"},
		"page.go":         {"context", "slices", "sort", "ucupa/shared/values", "ucupa/ucupa-core/contract", "ucupa/ucupa-core/contract/federation"},
		"path.go":         {"context", "ucupa/shared/values", "ucupa/ucupa-core/contract", "ucupa/ucupa-core/contract/federation", "ucupa/ucupa-core/internal/geometry"},
		"plan.go":         {"context", "errors", "sort", "ucupa/shared/values", "ucupa/ucupa-core/contract", "ucupa/ucupa-core/contract/federation", "ucupa/ucupa-core/internal/geometry", "ucupa/ucupa-core/internal/query/request"},
		"read.go":         {"context", "ucupa/shared/values", "ucupa/ucupa-core/contract"},
		"sources.go":      {"context", "errors", "time", "ucupa/shared/values", "ucupa/ucupa-core/contract", "ucupa/ucupa-core/contract/federation", "ucupa/ucupa-core/internal/mount/fact"},
		"types.go":        {"context", "sync", "ucupa/shared/values", "ucupa/ucupa-core/contract", "ucupa/ucupa-core/contract/federation"},
		"collect_test.go": {"context", "errors", "reflect", "sync", "testing", "ucupa/shared/values", "ucupa/ucupa-core/contract", "ucupa/ucupa-core/contract/federation", "ucupa/ucupa-core/internal/geometry", "ucupa/ucupa-core/internal/token"},
	}
	for file, dependencies := range cases {
		file = "ucupa-core/internal/query/federation/collect/" + file
		for _, dependency := range dependencies {
			if got := checkDependency(file, dependency, "ucupa", strings.HasSuffix(file, "_test.go"), stdlib); got != "" {
				t.Errorf("allowed Federation collect dependency %s -> %s: %s", file, dependency, got)
			}
		}
	}
	for _, tc := range []struct {
		file, dependency string
	}{
		{"ucupa-core/internal/query/federation/collect/closure.go", "ucupa/ucupa-core/internal/geometry"},
		{"ucupa-core/internal/query/federation/collect/path.go", "ucupa/ucupa-core/internal/query/federation/transcript"},
		{"ucupa-core/internal/query/federation/collect/plan.go", "ucupa/ucupa-core/internal/mount/fact"},
		{"ucupa-core/internal/query/federation/collect/sources.go", "ucupa/ucupa-core/internal/query/request"},
		{"ucupa-core/internal/query/federation/collect/collect_test.go", "ucupa/ucupa-core/internal/commit"},
		{"ucupa-core/internal/query/federation/collect/types.go", "ucupa/ucupa-core/internal/query"},
		{"ucupa-core/internal/query/federation/collect/read.go", "ucupa/ucupa-core"},
	} {
		if got := checkDependency(tc.file, tc.dependency, "ucupa", strings.HasSuffix(tc.file, "_test.go"), stdlib); got == "" {
			t.Errorf("forbidden Federation collect dependency accepted: %s -> %s", tc.file, tc.dependency)
		}
	}
	for _, source := range []string{
		"ucupa-core/internal/query/federation/coordinator/read.go", "ucupa-core/internal/query/federation/coordinator/read_at.go",
		"ucupa-core/internal/query/federation/coordinator/observe.go", "ucupa-core/internal/query/federation/coordinator/page.go",
		"ucupa-core/internal/query/federation/coordinator/path.go", "ucupa-core/internal/query/federation/coordinator/plan.go",
		"ucupa-core/internal/query/federation/coordinator/disclosure.go", "ucupa-core/internal/query/federation/coordinator/representation_test.go",
		"ucupa-core/internal/access/registry/registry.go", "ucupa-core/internal/access/registry/read.go",
		"ucupa-core/internal/query/federation/collect/actual_fixture_test.go", "ucupa-core/internal/query/federation/collect/actual_sources_test.go",
	} {
		if got := checkDependency(source, "ucupa/ucupa-core/internal/query/federation/collect", "ucupa", strings.HasSuffix(source, "_test.go"), stdlib); got != "" {
			t.Errorf("approved Federation collect consumer %s rejected: %s", source, got)
		}
	}
	for _, source := range []string{
		"ucupa-core/public_federated_sources_test.go", "ucupa-core/public_federated_read.go", "ucupa-core/public_context.go",
		"ucupa-core/contract/federation/federation.go", "ucupa-core/internal/query/federation/projection/query.go",
		"ucupa-core/internal/query/operation.go", "ucupa-core/internal/commit/state.go", "ucupa-core/public_read.go",
		"ucupa-core/transport/grpc/internal/wire/federation_wire.go", "ucupa-core/sdk/go/federation.go", "composition/ports.go",
	} {
		if got := checkDependency(source, "ucupa/ucupa-core/internal/query/federation/collect", "ucupa", strings.HasSuffix(source, "_test.go"), stdlib); got == "" {
			t.Errorf("unapproved Federation collect consumer accepted: %s", source)
		}
	}
}

func TestCollectorActualSourceScenariosHaveExactTestEdges(t *testing.T) {
	const directory = coreFederationCollectImport + "/"
	stdlib := guardOracleStdlib()
	for _, name := range []string{"context", "errors", "fmt", "reflect", "sync", "testing", "time"} {
		stdlib[name] = true
	}
	for name, dependencies := range coreCollectActualTestDependencies {
		file := directory + name
		if got := checkSourceLocation(file); got != "" {
			t.Error(file, got)
		}
		assertActualImportsAccepted(t, file)
		for _, dependency := range dependencies {
			imported := moduleImport("ucupa", dependency)
			if got := checkDependency(file, imported, "ucupa", true, stdlib); got != "" {
				t.Error(file, imported, got)
			}
			if got := checkDependency(file, imported, "ucupa", false, stdlib); got == "" {
				t.Error("actual source fixture escaped test build", file, imported)
			}
		}
		for _, forbidden := range []string{"ucupa/ucupa-core", "ucupa/ucupa-core/internal/commit", "ucupa/ucupa-core/internal/query", "ucupa/ucupa-core/internal/access/registry", "ucupa/ucupa-core/internal/store", "ucupa/ucupa-core/internal/element/fact", "ucupa/ucupa-core/internal/token", "ucupa/ucupa-core/transport/grpc", "ucupa/ucupa-core/sdk/go", "ucupa/ucupa-core/tests/federation", "ucupa/ucupa-facet/instance", "go.etcd.io/bbolt"} {
			if got := checkDependency(file, forbidden, "ucupa", true, stdlib); got == "" {
				t.Error("unreviewed private fixture edge", file, forbidden)
			}
		}
		if name != "actual_fixture_test.go" {
			for _, fixtureOnly := range []string{"ucupa/composition", "ucupa/ucupa-core/instance", "ucupa/ucupa-core/internal/query/federation/source"} {
				if got := checkDependency(file, fixtureOnly, "ucupa", true, stdlib); got == "" {
					t.Error("assembly escaped exact fixture", file, fixtureOnly)
				}
			}
		}
	}
	for _, name := range []string{"sources.go", "actual_extra_test.go", "child/actual_fixture_test.go"} {
		for _, imported := range []string{"ucupa/composition", "ucupa/ucupa-core/instance", "ucupa/ucupa-core/internal/query/federation/source"} {
			if got := checkDependency(directory+name, imported, "ucupa", strings.HasSuffix(name, "_test.go"), stdlib); got == "" {
				t.Error("unreviewed assembly location", name, imported)
			}
		}
	}
	for _, name := range []string{"actual_extra_test.go", "child/actual_fixture_test.go"} {
		if got := checkSourceLocation(directory + name); got == "" {
			t.Error("unreviewed actual-source scenario accepted", name)
		}
	}
}

func TestReviewedCoreAttachmentSnapshotDependencies(t *testing.T) {
	const file = "ucupa-core/tests/query/attachment_snapshot_test.go"
	stdlib := guardOracleStdlib()
	for _, dependency := range []string{
		"context", "encoding/binary", "testing", "time", "ucupa/shared/values", "ucupa/ucupa-core/instance", "ucupa/ucupa-core/contract",
	} {
		if got := checkDependency(file, dependency, "ucupa", true, stdlib); got != "" {
			t.Errorf("allowed attachment snapshot dependency %s: %s", dependency, got)
		}
	}
	for _, dependency := range []string{
		"ucupa/ucupa-core/internal/commit", "ucupa/ucupa-core/internal/query", "ucupa/ucupa-core/internal/query/federation/collect",
		"ucupa/ucupa-core/internal/store", "ucupa/ucupa-core/transport/grpc", "ucupa/ucupa-core/sdk/go", "go.etcd.io/bbolt",
	} {
		if got := checkDependency(file, dependency, "ucupa", true, stdlib); got == "" {
			t.Errorf("forbidden attachment snapshot dependency accepted: %s", dependency)
		}
	}
}

func TestFacetFaultExportSurfaceIsExact(t *testing.T) {
	const file = "ucupa-core/internal/commit/facet_fault_export_test.go"
	root, err := repositoryRoot(os.Getenv("PWD"))
	if err != nil {
		working, getwdErr := os.Getwd()
		if getwdErr != nil {
			t.Fatal(getwdErr)
		}
		root, err = repositoryRoot(working)
	}
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
	want := map[string]struct{ params, results []string }{
		"AttachmentHistoryForTest":      {params: []string{"*Owner", "contract.OperationID"}, results: []string{"int", "bool"}},
		"CorruptAttachmentStoreForTest": {params: []string{"string", "string"}, results: []string{"error"}},
	}
	found := map[string]bool{}
	for _, declaration := range parsed.Decls {
		switch declaration := declaration.(type) {
		case *ast.GenDecl:
			if declaration.Tok != token.IMPORT {
				for _, specification := range declaration.Specs {
					switch specification := specification.(type) {
					case *ast.TypeSpec:
						if ast.IsExported(specification.Name.Name) {
							t.Fatalf("%s exports type %s", file, specification.Name.Name)
						}
					case *ast.ValueSpec:
						for _, name := range specification.Names {
							if ast.IsExported(name.Name) {
								t.Fatalf("%s exports value %s", file, name.Name)
							}
						}
					}
				}
			}
		case *ast.FuncDecl:
			if declaration.Recv != nil || declaration.Name == nil || !ast.IsExported(declaration.Name.Name) {
				if declaration.Recv != nil && ast.IsExported(declaration.Name.Name) {
					t.Fatalf("%s exports method %s", file, declaration.Name.Name)
				}
				continue
			}
			expected, ok := want[declaration.Name.Name]
			if !ok || found[declaration.Name.Name] {
				t.Fatalf("%s contains unexpected or duplicate exported function %s", file, declaration.Name.Name)
			}
			found[declaration.Name.Name] = true
			if got := guardFieldTypes(declaration.Type.Params); !sameStringSlice(got, expected.params) {
				t.Fatalf("%s %s params=%v, want=%v", file, declaration.Name.Name, got, expected.params)
			}
			if got := guardFieldTypes(declaration.Type.Results); !sameStringSlice(got, expected.results) {
				t.Fatalf("%s %s results=%v, want=%v", file, declaration.Name.Name, got, expected.results)
			}
		default:
			t.Fatalf("%s contains an unexpected declaration", file)
		}
	}
	if len(found) != len(want) {
		t.Fatalf("%s exported functions=%v, want exactly=%v", file, found, want)
	}
}

func TestFacetFaultExportsAreAbsentFromProductionBuild(t *testing.T) {
	goTool, err := exec.LookPath("go")
	if err != nil {
		t.Fatal(err)
	}
	working, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	root, err := repositoryRoot(working)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ name, body string }{
		{"AttachmentHistoryForTest", "var owner *commitpkg.Owner\nvar operation contract.OperationID\n_, _ = commitpkg.AttachmentHistoryForTest(owner, operation)"},
		{"CorruptAttachmentStoreForTest", "_, _ = commitpkg.CorruptAttachmentStoreForTest(\"path\", \"part\")"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			directory, err := os.MkdirTemp(filepath.Join(root, "ucupa-core"), ".facet-fault-build-")
			if err != nil {
				t.Fatal(err)
			}
			defer os.RemoveAll(directory)
			writeFixture(t, directory, "main.go", "package main\n\nimport (\n\tcontract \"ucupa/ucupa-core/contract\"\n\tcommitpkg \"ucupa/ucupa-core/internal/commit\"\n)\n\nfunc main() {\n"+tc.body+"\n}\n")
			outputPath := filepath.Join(directory, "consumer.exe")
			command := exec.Command(goTool, "build", "-o", outputPath, filepath.Join(directory, "main.go"))
			command.Dir = root
			command.Env = commandEnvironment(false)
			output, err := command.CombinedOutput()
			if err == nil {
				t.Fatal("production build unexpectedly included a Facet fault export")
			}
			text := string(output)
			if !strings.Contains(text, "undefined: commitpkg."+tc.name) || strings.Contains(text, "use of internal package") {
				t.Fatalf("production build did not reject %s as an undefined test-only symbol: %v\n%s", tc.name, err, text)
			}
		})
	}
}
