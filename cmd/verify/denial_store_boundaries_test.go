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

func TestReviewedDenialStoreAndInstanceSources(t *testing.T) {
	for _, file := range []string{
		"ucupa-core/internal/query/federation/denial/evidence.go",
		"ucupa-core/internal/query/federation/denial/evidence_test.go",
		"ucupa-core/internal/store/format.go", "ucupa-core/internal/store/format_test.go",
		"ucupa-core/internal/store/json.go", "ucupa-core/internal/store/json_test.go",
		"ucupa-core/internal/store/scalar.go", "ucupa-core/internal/store/scalar_test.go",
		"ucupa-core/internal/store/transaction.go", "ucupa-core/internal/store/transaction_test.go",
		"ucupa-core/internal/commit/store_corruption_test.go",
		"ucupa-core/internal/commit/store_corruption_export_test.go",
		"ucupa-core/internal/commit/store_schema_test.go",
		"ucupa-core/tests/instance/store_open_test.go",
	} {
		if got := checkSourceLocation(file); got != "" {
			t.Errorf("reviewed source %s rejected: %s", file, got)
		}
	}
	for _, file := range []string{
		"ucupa-core/internal/query/federation/denial/extra.go",
		"ucupa-core/internal/query/federation/denial/nested/evidence.go",
		"ucupa-core/internal/store/format_extra_test.go",
		"ucupa-core/internal/store/nested/format.go",
		"ucupa-core/internal/commit/store_corruption_extra_test.go",
		"ucupa-core/tests/instance/store_extra_test.go",
	} {
		if got := checkSourceLocation(file); got == "" {
			t.Errorf("unreviewed source accepted: %s", file)
		}
	}
	if !reviewedCoreFederationDenialPackage("ucupa-core", "ucupa-core/internal/query/federation/denial") ||
		reviewedCoreFederationDenialPackage("ucupa-facet", "ucupa-core/internal/query/federation/denial") ||
		reviewedCoreFederationDenialPackage("ucupa-core", "ucupa-core/internal/query/federation/denial/nested") {
		t.Fatal("Federation denial registration must identify exactly one package")
	}
}

func TestReviewedFederationDenialDependenciesAndConsumers(t *testing.T) {
	stdlib := guardOracleStdlib()
	for _, dependency := range []string{
		"time", "ucupa/shared/values", "ucupa/ucupa-core/internal/mount/fact", "ucupa/ucupa-core/internal/query/federation/value",
	} {
		if got := checkDependency("ucupa-core/internal/query/federation/denial/evidence.go", dependency, "ucupa", false, stdlib); got != "" {
			t.Errorf("allowed denial production dependency %s: %s", dependency, got)
		}
	}
	for _, dependency := range []string{
		"reflect", "testing", "time", "ucupa/shared/values", "ucupa/ucupa-core/internal/mount/fact", "ucupa/ucupa-core/internal/query/federation/value",
	} {
		if got := checkDependency("ucupa-core/internal/query/federation/denial/evidence_test.go", dependency, "ucupa", true, stdlib); got != "" {
			t.Errorf("allowed denial test dependency %s: %s", dependency, got)
		}
	}
	for _, dependency := range []string{
		"ucupa/ucupa-core", "ucupa/ucupa-core/contract", "ucupa/ucupa-core/contract/federation",
		"ucupa/ucupa-core/internal/commit", "ucupa/ucupa-core/internal/query", "ucupa/ucupa-core/internal/query/request",
		"ucupa/ucupa-core/internal/store", "ucupa/ucupa-core/transport/grpc", "ucupa/ucupa-core/sdk/go", "go.etcd.io/bbolt",
	} {
		if got := checkDependency("ucupa-core/internal/query/federation/denial/evidence.go", dependency, "ucupa", false, stdlib); got == "" {
			t.Errorf("forbidden denial dependency accepted: %s", dependency)
		}
	}

	for _, source := range []string{
		"ucupa-core/contract/federation/federation.go", "ucupa-core/contract/federation/federation_test.go",
		"ucupa-core/public_federation_denial.go", "ucupa-core/public_federation_part.go",
		"ucupa-core/public_federation_evidence.go", "ucupa-core/public_federation_hidden.go",
	} {
		if got := checkDependency(source, "ucupa/ucupa-core/internal/query/federation/denial", "ucupa", strings.HasSuffix(source, "_test.go"), stdlib); got != "" {
			t.Errorf("approved denial consumer %s rejected: %s", source, got)
		}
	}
	for _, source := range []string{
		"ucupa-core/public_context.go", "ucupa-core/public_read.go", "ucupa-core/contract/query.go",
		"ucupa-core/internal/query/federation/collect/read.go", "ucupa-core/internal/query/federation/projection/query.go",
		"ucupa-core/internal/commit/state.go", "ucupa-core/transport/grpc/internal/wire/federation_wire.go",
	} {
		if got := checkDependency(source, "ucupa/ucupa-core/internal/query/federation/denial", "ucupa", strings.HasSuffix(source, "_test.go"), stdlib); got == "" {
			t.Errorf("unapproved denial consumer accepted: %s", source)
		}
	}
}

func TestReviewedStoreCorruptionSchemaAndFormatDependencies(t *testing.T) {
	stdlib := guardOracleStdlib()
	for _, dependency := range []string{
		"errors", "path/filepath", "testing", "ucupa/ucupa-core/contract", "ucupa/ucupa-core/internal/commit",
	} {
		if got := checkDependency("ucupa-core/internal/commit/store_corruption_test.go", dependency, "ucupa", true, stdlib); got != "" {
			t.Errorf("allowed Store corruption dependency: %s", got)
		}
	}
	for _, dependency := range []string{
		"encoding/json", "strings", "testing", "go.etcd.io/bbolt", "ucupa/shared/values",
		"ucupa/ucupa-core/contract", "ucupa/ucupa-core/internal/geometry", "ucupa/ucupa-core/internal/store",
	} {
		if got := checkDependency("ucupa-core/internal/commit/store_corruption_export_test.go", dependency, "ucupa", true, stdlib); got != "" {
			t.Errorf("allowed Store corruption bridge dependency: %s", got)
		}
	}
	for _, dependency := range []string{"reflect", "testing"} {
		if got := checkDependency("ucupa-core/internal/commit/store_schema_test.go", dependency, "ucupa", true, stdlib); got != "" {
			t.Errorf("allowed Store schema dependency: %s", got)
		}
	}
	for _, dependency := range []string{
		"bytes", "io/fs", "os", "path/filepath", "strings", "testing",
	} {
		if got := checkDependency("ucupa-core/internal/store/format_test.go", dependency, "ucupa", true, stdlib); got != "" {
			t.Errorf("allowed Store format dependency: %s", got)
		}
	}
	for _, dependency := range []string{
		"errors", "os", "path/filepath", "testing", "ucupa/shared/values", "ucupa/ucupa-core/instance", "ucupa/ucupa-core/contract",
	} {
		if got := checkDependency("ucupa-core/tests/instance/store_open_test.go", dependency, "ucupa", true, stdlib); got != "" {
			t.Errorf("allowed instance Store dependency: %s", got)
		}
	}
	for _, tc := range []struct{ file, dependency string }{
		{"ucupa-core/internal/commit/store_corruption_test.go", "go.etcd.io/bbolt"},
		{"ucupa-core/internal/commit/store_corruption_test.go", "ucupa/ucupa-core/internal/store"},
		{"ucupa-core/internal/commit/store_corruption_test.go", "ucupa/ucupa-core/internal/token"},
		{"ucupa-core/internal/commit/store_corruption_export_test.go", "ucupa/ucupa-core/internal/commit"},
		{"ucupa-core/internal/commit/store_corruption_export_test.go", "ucupa/ucupa-core/internal/element"},
		{"ucupa-core/internal/commit/store_corruption_export_test.go", "ucupa/ucupa-core/internal/token"},
		{"ucupa-core/internal/commit/store_schema_test.go", "go.etcd.io/bbolt"},
		{"ucupa-core/internal/commit/store_schema_test.go", "ucupa/ucupa-core/contract"},
		{"ucupa-core/internal/store/format_test.go", "ucupa/shared/values"},
		{"ucupa-core/internal/store/format_test.go", "go.etcd.io/bbolt"},
		{"ucupa-core/tests/instance/store_open_test.go", "ucupa/ucupa-core/internal/store"},
		{"ucupa-core/tests/instance/store_open_test.go", "ucupa/ucupa-core/internal/commit"},
		{"ucupa-core/tests/instance/store_open_test.go", "go.etcd.io/bbolt"},
	} {
		if got := checkDependency(tc.file, tc.dependency, "ucupa", true, stdlib); got == "" {
			t.Errorf("forbidden Store dependency accepted: %s -> %s", tc.file, tc.dependency)
		}
	}
}

func TestFederationDenialAndStoreFaultExportSurfaces(t *testing.T) {
	root, err := currentRepositoryRoot(t)
	if err != nil {
		t.Fatal(err)
	}
	assertDenialEvidenceSurface(t, root)
	assertSingleExportedFunction(t, root, "ucupa-core/internal/commit/store_corruption_export_test.go", "CorruptCurrentStoreForTest", []string{"*testing.T", "string", "string"}, nil)

	goTool, err := exec.LookPath("go")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, source string
		want         []string
		underCore    bool
	}{
		{
			name:   "denial-private-field",
			source: "package main\nimport f \"ucupa/ucupa-core/contract/federation\"\nfunc main() { _ = f.FederationDenial{owner: \"forged\"} }\n",
			want:   []string{"cannot refer to unexported field", "implicit assignment to unexported field", "unknown field owner"},
		},
		{
			name:   "denial-no-constructor",
			source: "package main\nimport f \"ucupa/ucupa-core/contract/federation\"\nvar _ = f.New\nfunc main() {}\n",
			want:   []string{"undefined: f.New"},
		},
		{
			name:   "denial-no-root-alias",
			source: "package main\nimport c \"ucupa/ucupa-core\"\nvar _ c.FederationDenial\nfunc main() {}\n",
			want:   []string{"undefined: c.FederationDenial", "package ucupa/ucupa-core is not in std"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			directory := t.TempDir()
			writeFixture(t, directory, "main.go", tc.source)
			outputPath := filepath.Join(directory, "consumer.exe")
			command := exec.Command(goTool, "build", "-o", outputPath, filepath.Join(directory, "main.go"))
			command.Dir = root
			command.Env = commandEnvironment(false)
			output, err := command.CombinedOutput()
			if err == nil {
				t.Fatal("forged Federation denial consumer unexpectedly compiled")
			}
			text := string(output)
			found := false
			for _, want := range tc.want {
				if strings.Contains(text, want) {
					found = true
					break
				}
			}
			if !found || strings.Contains(text, "use of internal package") {
				t.Fatalf("Federation denial consumer was not rejected at the public surface: %v\n%s", err, text)
			}
		})
	}

	directory, err := os.MkdirTemp(filepath.Join(root, "ucupa-core"), ".store-corruption-build-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(directory)
	writeFixture(t, directory, "main.go", "package main\n\nimport (\n\tcommitpkg \"ucupa/ucupa-core/internal/commit\"\n\t\"testing\"\n)\n\nfunc main() {\nvar test *testing.T\ncommitpkg.CorruptCurrentStoreForTest(test, \"path\", \"fault\")\n}\n")
	outputPath := filepath.Join(directory, "consumer.exe")
	command := exec.Command(goTool, "build", "-o", outputPath, filepath.Join(directory, "main.go"))
	command.Dir = root
	command.Env = commandEnvironment(false)
	output, err := command.CombinedOutput()
	if err == nil {
		t.Fatal("production build unexpectedly included Store corruption test export")
	}
	text := string(output)
	if !strings.Contains(text, "undefined: commitpkg.CorruptCurrentStoreForTest") || strings.Contains(text, "use of internal package") {
		t.Fatalf("production build did not reject Store corruption export as undefined: %v\n%s", err, text)
	}
}

func assertDenialEvidenceSurface(t *testing.T, root string) {
	t.Helper()
	const file = "ucupa-core/internal/query/federation/denial/evidence.go"
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(file)))
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := parser.ParseFile(token.NewFileSet(), file, data, parser.ParseComments)
	if err != nil {
		t.Fatal(err)
	}
	wantFields := []struct{ name, typ string }{
		{"owner", "values.FactAuthorityRef"}, {"source", "values.FactAuthorityRef"},
		{"caller", "values.CallerContext"}, {"request", "federationvalue.FederationReadRequest"},
		{"requirement", "values.Requirement"}, {"view", "values.AuthorizationDenialView"},
		{"incoming", "mountfact.Incoming"}, {"expires", "time.Time"},
	}
	wantFunctions := map[string]struct{ params, results []string }{
		"New":             {[]string{"values.FactAuthorityRef", "values.FactAuthorityRef", "values.CallerContext", "federationvalue.FederationReadRequest", "values.Requirement", "values.AuthorizationDenialView", "mountfact.Incoming", "time.Time"}, []string{"Evidence"}},
		"MatchesBinding":  {[]string{"Evidence", "values.FactAuthorityRef", "values.FactAuthorityRef", "values.CallerContext", "federationvalue.FederationReadRequest"}, []string{"bool"}},
		"Authorization":   {[]string{"Evidence"}, []string{"values.AuthorizationDenialView", "values.Requirement"}},
		"Incoming":        {[]string{"Evidence"}, []string{"mountfact.Incoming"}},
		"Expired":         {[]string{"Evidence", "time.Time"}, []string{"bool"}},
		"MatchesIncoming": {[]string{"Evidence", "mountfact.Incoming"}, []string{"bool"}},
	}
	foundEvidence := false
	foundFunctions := map[string]bool{}
	for _, declaration := range parsed.Decls {
		switch declaration := declaration.(type) {
		case *ast.GenDecl:
			if declaration.Tok == token.IMPORT {
				continue
			}
			for _, specification := range declaration.Specs {
				item, ok := specification.(*ast.TypeSpec)
				if !ok || item.Name.Name != "Evidence" || foundEvidence {
					t.Fatalf("%s contains an unexpected type declaration", file)
				}
				structure, ok := item.Type.(*ast.StructType)
				if !ok || len(structure.Fields.List) != len(wantFields) {
					t.Fatalf("%s Evidence is not the exact private struct", file)
				}
				for index, field := range structure.Fields.List {
					if len(field.Names) != 1 || field.Names[0].Name != wantFields[index].name || ast.IsExported(field.Names[0].Name) || guardASTType(field.Type) != wantFields[index].typ {
						t.Fatalf("%s Evidence field %d changed", file, index)
					}
				}
				foundEvidence = true
			}
		case *ast.FuncDecl:
			if declaration.Recv != nil {
				if declaration.Name != nil && ast.IsExported(declaration.Name.Name) {
					t.Fatalf("%s exports method %s", file, declaration.Name.Name)
				}
				continue
			}
			if declaration.Name == nil || !ast.IsExported(declaration.Name.Name) {
				continue
			}
			expected, ok := wantFunctions[declaration.Name.Name]
			if !ok || foundFunctions[declaration.Name.Name] {
				t.Fatalf("%s contains unexpected exported function %s", file, declaration.Name.Name)
			}
			foundFunctions[declaration.Name.Name] = true
			if got := denialASTFieldTypes(declaration.Type.Params); !sameStringSlice(got, expected.params) ||
				!sameStringSlice(denialASTFieldTypes(declaration.Type.Results), expected.results) {
				t.Fatalf("%s %s signature changed", file, declaration.Name.Name)
			}
		default:
			t.Fatalf("%s contains an unexpected declaration", file)
		}
	}
	if !foundEvidence || len(foundFunctions) != len(wantFunctions) {
		t.Fatalf("%s exported surface changed: Evidence=%v functions=%v", file, foundEvidence, foundFunctions)
	}
}

func assertSingleExportedFunction(t *testing.T, root, file, name string, params, results []string) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(file)))
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := parser.ParseFile(token.NewFileSet(), file, data, parser.ParseComments)
	if err != nil {
		t.Fatal(err)
	}
	found := false
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
						for _, item := range specification.Names {
							if ast.IsExported(item.Name) {
								t.Fatalf("%s exports value %s", file, item.Name)
							}
						}
					}
				}
			}
		case *ast.FuncDecl:
			if declaration.Recv != nil || declaration.Name == nil || declaration.Name.Name != name || found {
				if declaration.Recv != nil && declaration.Name != nil && ast.IsExported(declaration.Name.Name) {
					t.Fatalf("%s exports method %s", file, declaration.Name.Name)
				}
				if declaration.Name != nil && ast.IsExported(declaration.Name.Name) {
					t.Fatalf("%s exports unexpected function %s", file, declaration.Name.Name)
				}
				continue
			}
			if !sameStringSlice(storeASTFieldTypes(declaration.Type.Params), params) || !sameStringSlice(storeASTFieldTypes(declaration.Type.Results), results) {
				t.Fatalf("%s %s signature changed", file, name)
			}
			found = true
		default:
			t.Fatalf("%s contains an unexpected declaration", file)
		}
	}
	if !found {
		t.Fatalf("%s does not contain %s", file, name)
	}
}

func denialASTFieldTypes(fields *ast.FieldList) []string {
	return expandedASTFieldTypes(fields)
}

func storeASTFieldTypes(fields *ast.FieldList) []string {
	return expandedASTFieldTypes(fields)
}

func expandedASTFieldTypes(fields *ast.FieldList) []string {
	if fields == nil {
		return nil
	}
	types := make([]string, 0, len(fields.List))
	for _, field := range fields.List {
		if len(field.Names) == 0 {
			types = append(types, guardASTType(field.Type))
			continue
		}
		for range field.Names {
			types = append(types, guardASTType(field.Type))
		}
	}
	return types
}
