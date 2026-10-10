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

func TestReviewedCoreMountGuardAndOracleSourceBoundaries(t *testing.T) {
	if !reviewedQueryOraclePackage("ucupa-core", "ucupa-core/internal/query/oracle") ||
		reviewedQueryOraclePackage("ucupa-facet", "ucupa-core/internal/query/oracle") ||
		reviewedQueryOraclePackage("ucupa-core", "ucupa-core/internal/query/oracle/child") {
		t.Fatal("Core query oracle registration must identify exactly one package")
	}
	for _, file := range []string{
		"mount_guard_fixture_test.go",
		"mount_guard_proof_export_test.go",
		"mount_guard_read_test.go",
		"mount_guard_validation_test.go",
		"mount_guard_write_test.go",
	} {
		path := "ucupa-core/internal/commit/" + file
		if got := checkSourceLocation(path); got != "" {
			t.Errorf("reviewed Core mount-guard source %s: %s", path, got)
		}
	}
	for _, file := range []string{
		"scan.go",
		"fixture_test.go",
		"conformance_test.go",
		"rotation_test.go",
		"failure_test.go",
		"work_test.go",
	} {
		path := "ucupa-core/internal/query/oracle/" + file
		if got := checkSourceLocation(path); got != "" {
			t.Errorf("reviewed Core query oracle source %s: %s", path, got)
		}
	}
	for _, file := range []string{
		"ucupa-core/internal/commit/mount_guard_extra_test.go",
		"ucupa-core/internal/query/oracle/extra.go",
		"ucupa-facet/internal/query/oracle/scan.go",
		"ucupa-core/internal/query/oracle/child/scan.go",
	} {
		if got := checkSourceLocation(file); got == "" {
			t.Errorf("unreviewed mount-guard/oracle source accepted: %s", file)
		}
	}
}

func TestReviewedCoreMountGuardDependencies(t *testing.T) {
	stdlib := guardOracleStdlib()
	cases := []struct {
		file    string
		allowed []string
	}{
		{
			file: "ucupa-core/internal/commit/mount_guard_fixture_test.go",
			allowed: []string{
				"errors", "fmt", "reflect", "sync", "testing", "ucupa/shared/values",
				"ucupa/ucupa-core/contract", "ucupa/ucupa-core/internal/commit",
				"ucupa/ucupa-core/internal/element", "ucupa/ucupa-core/internal/query",
			},
		},
		{
			file: "ucupa-core/internal/commit/mount_guard_proof_export_test.go",
			allowed: []string{
				"ucupa/shared/values", "ucupa/ucupa-core/contract",
			},
		},
		{
			file: "ucupa-core/internal/commit/mount_guard_read_test.go",
			allowed: []string{
				"errors", "reflect", "testing", "ucupa/shared/values",
				"ucupa/ucupa-core/contract", "ucupa/ucupa-core/internal/commit",
				"ucupa/ucupa-core/internal/query/oracle", "ucupa/ucupa-core/internal/token",
			},
		},
		{
			file: "ucupa-core/internal/commit/mount_guard_validation_test.go",
			allowed: []string{
				"testing", "ucupa/shared/values", "ucupa/ucupa-core/contract",
				"ucupa/ucupa-core/internal/commit/binding", "ucupa/ucupa-core/internal/element/fact",
				"ucupa/ucupa-core/internal/mount/fact",
			},
		},
		{
			file: "ucupa-core/internal/commit/mount_guard_write_test.go",
			allowed: []string{
				"errors", "math", "reflect", "testing", "ucupa/ucupa-core/contract",
				"ucupa/ucupa-core/internal/commit",
			},
		},
	}
	for _, tc := range cases {
		if got := checkSourceLocation(tc.file); got != "" {
			t.Errorf("reviewed Core mount-guard test %s rejected: %s", tc.file, got)
		}
		for _, dependency := range tc.allowed {
			if got := checkDependency(tc.file, dependency, "ucupa", true, stdlib); got != "" {
				t.Errorf("allowed Core mount-guard dependency %s -> %s: %s", tc.file, dependency, got)
			}
		}
	}
	for _, tc := range []struct {
		file         string
		dependencies []string
	}{
		{
			file: "ucupa-core/internal/commit/mount_guard_fixture_test.go",
			dependencies: []string{
				"ucupa/ucupa-core/internal/store", "ucupa/ucupa-core/internal/token",
				"ucupa/ucupa-core/internal/query/oracle", "go.etcd.io/bbolt",
				"ucupa/ucupa-core/transport/grpc", "ucupa/ucupa-core/sdk/go",
			},
		},
		{
			file: "ucupa-core/internal/commit/mount_guard_proof_export_test.go",
			dependencies: []string{
				"ucupa/ucupa-core", "ucupa/ucupa-core/internal/commit", "ucupa/ucupa-core/internal/element",
				"ucupa/ucupa-core/internal/store", "ucupa/ucupa-core/internal/query/oracle", "go.etcd.io/bbolt",
			},
		},
		{
			file: "ucupa-core/internal/commit/mount_guard_read_test.go",
			dependencies: []string{
				"ucupa/ucupa-core/internal/store", "ucupa/ucupa-core/internal/element",
				"ucupa/ucupa-core/internal/query", "go.etcd.io/bbolt",
				"ucupa/ucupa-core/transport/grpc", "ucupa/ucupa-core/sdk/go",
			},
		},
		{
			file: "ucupa-core/internal/commit/mount_guard_validation_test.go",
			dependencies: []string{
				"ucupa/ucupa-core/internal/commit", "ucupa/ucupa-core/internal/store",
				"ucupa/ucupa-core/internal/token", "ucupa/ucupa-core/internal/query/oracle",
				"go.etcd.io/bbolt", "ucupa/ucupa-core/transport/grpc",
			},
		},
		{
			file: "ucupa-core/internal/commit/mount_guard_write_test.go",
			dependencies: []string{
				"ucupa/ucupa-core/internal/store", "ucupa/ucupa-core/internal/token",
				"ucupa/ucupa-core/internal/query/oracle", "ucupa/ucupa-core/internal/element",
				"go.etcd.io/bbolt", "ucupa/ucupa-core/transport/grpc",
			},
		},
	} {
		for _, dependency := range tc.dependencies {
			if got := checkDependency(tc.file, dependency, "ucupa", true, stdlib); got == "" {
				t.Errorf("forbidden Core mount-guard dependency accepted: %s -> %s", tc.file, dependency)
			}
		}
	}
}

func TestReviewedCoreQueryOracleDependencies(t *testing.T) {
	stdlib := guardOracleStdlib()
	for _, dependency := range []string{
		"errors", "ucupa/shared/values", "ucupa/ucupa-core/contract", "ucupa/ucupa-core/internal/commit",
		"ucupa/ucupa-core/internal/element/fact", "ucupa/ucupa-core/internal/geometry",
		"ucupa/ucupa-core/internal/mount/fact", "ucupa/ucupa-core/internal/query/scan", "ucupa/ucupa-core/internal/token",
	} {
		if got := checkDependency("ucupa-core/internal/query/oracle/scan.go", dependency, "ucupa", false, stdlib); got != "" {
			t.Errorf("allowed Core query oracle production dependency %s: %s", dependency, got)
		}
	}
	for _, dependency := range []string{
		"ucupa/ucupa-core", "ucupa/ucupa-core/internal/query", "ucupa/ucupa-core/internal/store",
		"ucupa/ucupa-core/internal/facet/fact", "ucupa/ucupa-core/internal/element",
		"ucupa/ucupa-core/internal/token/signer", "ucupa/ucupa-core/transport/grpc",
		"ucupa/ucupa-core/sdk/go", "go.etcd.io/bbolt", "ucupa/ucupa-core/internal/commit/binding",
	} {
		if got := checkDependency("ucupa-core/internal/query/oracle/scan.go", dependency, "ucupa", false, stdlib); got == "" {
			t.Errorf("forbidden Core query oracle production dependency accepted: %s", dependency)
		}
	}

	cases := []struct {
		file    string
		allowed []string
	}{
		{
			file: "ucupa-core/internal/query/oracle/fixture_test.go",
			allowed: []string{
				"crypto/rand", "fmt", "testing", "ucupa/shared/values", "ucupa/ucupa-core/contract",
				"ucupa/ucupa-core/internal/commit", "ucupa/ucupa-core/internal/element",
			},
		},
		{
			file: "ucupa-core/internal/query/oracle/conformance_test.go",
			allowed: []string{
				"fmt", "testing", "ucupa/shared/values", "ucupa/ucupa-core/contract",
				"ucupa/ucupa-core/internal/element/fact", "ucupa/ucupa-core/internal/query/oracle",
			},
		},
		{
			file: "ucupa-core/internal/query/oracle/failure_test.go",
			allowed: []string{
				"errors", "math", "testing", "ucupa/ucupa-core/contract",
				"ucupa/ucupa-core/internal/query/oracle",
			},
		},
		{
			file: "ucupa-core/internal/query/oracle/rotation_test.go",
			allowed: []string{
				"testing", "ucupa/shared/values", "ucupa/ucupa-core/contract",
				"ucupa/ucupa-core/internal/query/oracle",
			},
		},
		{
			file: "ucupa-core/internal/query/oracle/work_test.go",
			allowed: []string{
				"errors", "fmt", "math", "testing", "ucupa/ucupa-core/contract",
				"ucupa/ucupa-core/internal/query/oracle", "ucupa/ucupa-core/internal/query/scan",
			},
		},
	}
	for _, tc := range cases {
		if got := checkSourceLocation(tc.file); got != "" {
			t.Errorf("reviewed Core query oracle test %s rejected: %s", tc.file, got)
		}
		for _, dependency := range tc.allowed {
			if got := checkDependency(tc.file, dependency, "ucupa", true, stdlib); got != "" {
				t.Errorf("allowed Core query oracle dependency %s -> %s: %s", tc.file, dependency, got)
			}
		}
	}
	for _, tc := range []struct {
		file         string
		dependencies []string
	}{
		{
			file: "ucupa-core/internal/query/oracle/fixture_test.go",
			dependencies: []string{
				"ucupa/ucupa-core/internal/query/oracle", "ucupa/ucupa-core/internal/store",
				"ucupa/ucupa-core/internal/token", "ucupa/ucupa-core/internal/element/fact",
				"go.etcd.io/bbolt", "ucupa/ucupa-core/transport/grpc",
			},
		},
		{
			file: "ucupa-core/internal/query/oracle/conformance_test.go",
			dependencies: []string{
				"ucupa/ucupa-core/internal/commit", "ucupa/ucupa-core/internal/element",
				"ucupa/ucupa-core/internal/store", "ucupa/ucupa-core/internal/token",
				"go.etcd.io/bbolt", "ucupa/ucupa-core/transport/grpc",
			},
		},
		{
			file: "ucupa-core/internal/query/oracle/failure_test.go",
			dependencies: []string{
				"ucupa/shared/values", "ucupa/ucupa-core/internal/commit",
				"ucupa/ucupa-core/internal/element/fact", "ucupa/ucupa-core/internal/query/scan",
				"go.etcd.io/bbolt",
			},
		},
		{
			file: "ucupa-core/internal/query/oracle/rotation_test.go",
			dependencies: []string{
				"ucupa/ucupa-core/internal/commit", "ucupa/ucupa-core/internal/element/fact",
				"ucupa/ucupa-core/internal/query/scan", "ucupa/ucupa-core/internal/token",
				"go.etcd.io/bbolt",
			},
		},
		{
			file: "ucupa-core/internal/query/oracle/work_test.go",
			dependencies: []string{
				"ucupa/shared/values", "ucupa/ucupa-core/internal/commit",
				"ucupa/ucupa-core/internal/element", "ucupa/ucupa-core/internal/store",
				"ucupa/ucupa-core/internal/token", "go.etcd.io/bbolt",
			},
		},
	} {
		for _, dependency := range tc.dependencies {
			if got := checkDependency(tc.file, dependency, "ucupa", true, stdlib); got == "" {
				t.Errorf("forbidden Core query oracle dependency accepted: %s -> %s", tc.file, dependency)
			}
		}
	}
	for _, file := range []string{
		"ucupa-core/internal/query/oracle/extra_test.go",
	} {
		if got := checkDependency(file, "ucupa/ucupa-core/internal/commit", "ucupa", true, stdlib); got == "" {
			t.Errorf("unreviewed query oracle test dependency accepted: %s", file)
		}
	}

	for _, tc := range []struct {
		file string
		want bool
	}{
		{file: "ucupa-core/oracle_test.go", want: true},
		{file: "ucupa-core/query_internal_test.go", want: true},
		{file: "ucupa-core/other_test.go", want: false},
		{file: "ucupa-core/public_resource.go", want: false},
		{file: "ucupa-core/transport/grpc/oracle.go", want: false},
	} {
		got := checkDependency(tc.file, "ucupa/ucupa-core/internal/query/oracle", "ucupa", strings.HasSuffix(tc.file, "_test.go"), stdlib) == ""
		if got != tc.want {
			t.Errorf("oracle consumer %s allowed=%v, want=%v", tc.file, got, tc.want)
		}
	}
}

func TestMountGuardProofExportSurfaceIsExact(t *testing.T) {
	const file = "ucupa-core/internal/commit/mount_guard_proof_export_test.go"
	root, err := repositoryRootForGuardOracle(t)
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

	wantTypes := map[string][][]string{
		"CompleteStateProofForTest": {
			{"state", "*coreState"},
		},
		"MountGuardInspectionForTest": {
			{"Anchors", "Entities", "Claims", "Tombstones", "int"},
			{"RootExists", "SelectedClaimed", "bool"},
			{"SelectedKind", "contract.ElementKind"},
			{"OperationRetained", "bool"},
			{"IncomingRange", "float64"},
		},
	}
	wantFunctions := map[string]struct {
		params  []string
		results []string
	}{
		"CaptureCompleteStateForTest": {
			params: []string{"*Owner"}, results: []string{"CompleteStateProofForTest"},
		},
		"MatchesCompleteStateForTest": {
			params: []string{"*Owner", "CompleteStateProofForTest"}, results: []string{"bool"},
		},
		"ValidateCompleteStateForTest": {
			params: []string{"*Owner"}, results: []string{"error"},
		},
		"InspectMountGuardForTest": {
			params: []string{"*Owner", "values.ElementID", "contract.OperationID"}, results: []string{"MountGuardInspectionForTest"},
		},
	}
	foundTypes := make(map[string]bool)
	foundFunctions := make(map[string]bool)
	for _, declaration := range parsed.Decls {
		switch declaration := declaration.(type) {
		case *ast.GenDecl:
			if declaration.Tok == token.IMPORT {
				continue
			}
			if declaration.Tok != token.TYPE || len(declaration.Specs) != 1 {
				t.Fatalf("%s contains an unexpected non-type declaration", file)
			}
			specification, ok := declaration.Specs[0].(*ast.TypeSpec)
			if !ok || specification.Name == nil {
				t.Fatalf("%s contains an invalid type declaration", file)
			}
			expected, ok := wantTypes[specification.Name.Name]
			if !ok || foundTypes[specification.Name.Name] {
				t.Fatalf("%s contains unexpected or duplicate type %s", file, specification.Name.Name)
			}
			structure, ok := specification.Type.(*ast.StructType)
			if !ok || !sameGuardStructFields(structure.Fields, expected) {
				t.Fatalf("%s type %s has an unexpected fixed shape", file, specification.Name.Name)
			}
			foundTypes[specification.Name.Name] = true
		case *ast.FuncDecl:
			if declaration.Recv != nil || declaration.Name == nil {
				t.Fatalf("%s contains a method or unnamed function", file)
			}
			expected, ok := wantFunctions[declaration.Name.Name]
			if !ok || foundFunctions[declaration.Name.Name] {
				t.Fatalf("%s contains unexpected or duplicate function %s", file, declaration.Name.Name)
			}
			if got := guardFieldTypes(declaration.Type.Params); !sameStringSlice(got, expected.params) ||
				!sameStringSlice(guardFieldTypes(declaration.Type.Results), expected.results) {
				t.Fatalf("%s function %s has an unexpected signature", file, declaration.Name.Name)
			}
			foundFunctions[declaration.Name.Name] = true
		default:
			t.Fatalf("%s contains an unexpected declaration", file)
		}
	}
	if len(foundTypes) != len(wantTypes) || len(foundFunctions) != len(wantFunctions) {
		t.Fatalf("%s surface types=%v functions=%v, want exactly %v and %v", file, foundTypes, foundFunctions, wantTypes, wantFunctions)
	}
}

func TestMountGuardProofExportsAreAbsentFromProductionBuild(t *testing.T) {
	goTool, err := exec.LookPath("go")
	if err != nil {
		t.Fatal(err)
	}
	root, err := repositoryRootForGuardOracle(t)
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name string
		body string
	}{
		{
			name: "CompleteStateProofForTest",
			body: "var proof commitpkg.CompleteStateProofForTest\n_ = proof",
		},
		{
			name: "MountGuardInspectionForTest",
			body: "var inspection commitpkg.MountGuardInspectionForTest\n_ = inspection",
		},
		{
			name: "CaptureCompleteStateForTest",
			body: "_ = commitpkg.CaptureCompleteStateForTest(owner)",
		},
		{
			name: "MatchesCompleteStateForTest",
			body: "var proof commitpkg.CompleteStateProofForTest\n_ = commitpkg.MatchesCompleteStateForTest(owner, proof)",
		},
		{
			name: "ValidateCompleteStateForTest",
			body: "_ = commitpkg.ValidateCompleteStateForTest(owner)",
		},
		{
			name: "InspectMountGuardForTest",
			body: "_ = commitpkg.InspectMountGuardForTest(owner, element, operation)",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			directory, err := os.MkdirTemp(filepath.Join(root, "ucupa-core"), ".mount-guard-proof-build-")
			if err != nil {
				t.Fatal(err)
			}
			defer os.RemoveAll(directory)
			writeFixture(t, directory, "main.go", "package main\n\nimport (\n\tcontract \"ucupa/ucupa-core/contract\"\n\tcommitpkg \"ucupa/ucupa-core/internal/commit\"\n\t\"ucupa/shared/values\"\n)\n\nfunc main() {\nvar owner *commitpkg.Owner\nvar element values.ElementID\nvar operation contract.OperationID\nvar _ values.ElementID\nvar _ contract.OperationID\n"+tc.body+"\n}\n")
			outputPath := filepath.Join(directory, "consumer.exe")
			command := exec.Command(goTool, "build", "-o", outputPath, filepath.Join(directory, "main.go"))
			command.Dir = root
			command.Env = commandEnvironment(false)
			output, err := command.CombinedOutput()
			if err == nil {
				t.Fatal("production build unexpectedly included a mount-guard proof export")
			}
			text := string(output)
			if !strings.Contains(text, "undefined: commitpkg."+tc.name) || strings.Contains(text, "use of internal package") {
				t.Fatalf("production build did not reject %s as an undefined test-only symbol: %v\n%s", tc.name, err, text)
			}
		})
	}
}
