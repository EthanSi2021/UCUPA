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

const registryPackagePath = "ucupa-core/internal/access/registry"

func TestAccessRegistrySourceSetAndDependenciesAreExact(t *testing.T) {
	stdlib := guardOracleStdlib()
	module := "ucupa"
	production := map[string][]string{
		"registry.go": {
			"context", "sync", "time", "shared/values", "ucupa-core/contract",
			"ucupa-core/internal/access/observe", "ucupa-core/internal/query/federation/collect",
			"ucupa-core/internal/query/federation/denial",
		},
		"read.go": {
			"context", "shared/values", "ucupa-core/contract",
			"ucupa-core/internal/query/federation/collect",
		},
		"source.go": {
			"shared/values", "ucupa-core/contract", "ucupa-core/contract/federation",
			"ucupa-core/internal/mount/fact", "ucupa-core/internal/query/federation/denial",
		},
		"observation.go": {
			"shared/values", "ucupa-core/contract", "ucupa-core/internal/access/observe",
		},
	}
	tests := map[string][]string{
		"registry_test.go": {
			"errors", "sync", "testing", "time", "shared/values", "ucupa-core/contract",
			"ucupa-core/internal/access/observe", "ucupa-core/internal/element/fact",
			"ucupa-core/internal/facet/fact", "ucupa-core/internal/mount/fact",
			"ucupa-core/internal/transfer/fact",
		},
		"source_test.go": {
			"errors", "testing", "shared/values", "ucupa-core/contract",
			"ucupa-core/contract/federation", "ucupa-core/internal/mount/fact",
			"ucupa-core/internal/query/federation/denial",
		},
	}
	for file, dependencies := range production {
		path := registryPackagePath + "/" + file
		for _, dependency := range dependencies {
			if reason := checkDependency(path, moduleImport(module, dependency), module, false, stdlib); reason != "" {
				t.Errorf("%s should allow %s: %s", path, dependency, reason)
			}
		}
	}
	for file, dependencies := range tests {
		path := registryPackagePath + "/" + file
		for _, dependency := range dependencies {
			if reason := checkDependency(path, moduleImport(module, dependency), module, true, stdlib); reason != "" {
				t.Errorf("%s should allow %s: %s", path, dependency, reason)
			}
		}
	}
	for file, imports := range map[string][]string{
		"registry.go": {
			"ucupa-core", "ucupa-core/internal/commit", "ucupa-core/internal/element",
			"ucupa-core/internal/mount", "ucupa-core/internal/query", "ucupa-core/internal/store",
			"ucupa-core/transport/grpc", "go.etcd.io/bbolt",
		},
		"read.go": {
			"ucupa-core", "ucupa-core/internal/commit", "ucupa-core/internal/element",
			"ucupa-core/internal/mount", "ucupa-core/internal/query", "ucupa-core/internal/store",
			"ucupa-core/transport/grpc", "go.etcd.io/bbolt",
		},
		"source.go": {
			"ucupa-core", "ucupa-core/internal/commit", "ucupa-core/internal/element",
			"ucupa-core/internal/query", "ucupa-core/internal/store", "ucupa-core/transport/grpc",
			"go.etcd.io/bbolt",
		},
		"observation.go": {
			"ucupa-core", "ucupa-core/internal/commit", "ucupa-core/internal/element",
			"ucupa-core/internal/mount", "ucupa-core/internal/query", "ucupa-core/internal/store",
			"ucupa-core/transport/grpc", "go.etcd.io/bbolt",
		},
		"registry_test.go": {
			"ucupa-core/internal/commit", "ucupa-core/internal/query", "ucupa-core/internal/store",
			"ucupa-core/transport/grpc", "go.etcd.io/bbolt",
		},
		"source_test.go": {
			"ucupa-core/internal/commit", "ucupa-core/internal/query", "ucupa-core/internal/store",
			"ucupa-core/transport/grpc", "go.etcd.io/bbolt",
		},
	} {
		path := registryPackagePath + "/" + file
		for _, imported := range imports {
			if reason := checkDependency(path, moduleImport(module, imported), module, strings.HasSuffix(file, "_test.go"), stdlib); reason == "" {
				t.Errorf("%s unexpectedly allows %s", path, imported)
			}
		}
	}

	for _, file := range []string{
		"registry.go", "read.go", "source.go", "observation.go", "registry_test.go", "source_test.go",
	} {
		if reason := checkSourceLocation(registryPackagePath + "/" + file); reason != "" {
			t.Errorf("reviewed Registry file rejected: %s: %s", file, reason)
		}
	}
	for _, file := range []string{
		"registry_extra.go", "registry_test_extra.go", "nested/registry.go", "nested/registry_test.go",
	} {
		if reason := checkSourceLocation(registryPackagePath + "/" + file); reason == "" {
			t.Errorf("unreviewed Registry path accepted: %s", file)
		}
	}
}

func TestAccessRegistryConsumersAreExact(t *testing.T) {
	stdlib := guardOracleStdlib()
	module := "ucupa"
	allowed := []string{
		"ucupa-core/instance/instance.go",
		"ucupa-core/internal/access/entry/operation.go",
		"ucupa-core/internal/access/observation/observe.go",
		"ucupa-core/internal/access/observation/operation.go",
		"ucupa-core/internal/access/observation/operation_test.go",
		"ucupa-core/internal/access/resolve/resolve.go",
		"ucupa-core/internal/access/resolve/resolve_test.go",
		"ucupa-core/internal/element/public.go",
		"ucupa-core/internal/facet/operation.go",
		"ucupa-core/internal/facet/service_attachment.go",
		"ucupa-core/internal/facet/service_fixture_test.go",
		"ucupa-core/internal/query/federation/coordinator/fixture_test.go",
		"ucupa-core/internal/query/federation/coordinator/observe.go",
		"ucupa-core/internal/query/federation/coordinator/operation.go",
		"ucupa-core/internal/query/federation/coordinator/read_at.go",
		"ucupa-core/internal/query/federation/coordinator/read.go",
		"ucupa-core/internal/query/federation/source/check.go",
		"ucupa-core/internal/query/federation/source/evidence_test.go",
		"ucupa-core/internal/query/federation/source/evidence.go",
		"ucupa-core/internal/query/federation/source/fixture_test.go",
		"ucupa-core/internal/query/federation/source/hidden.go",
		"ucupa-core/internal/query/federation/source/observe_binding.go",
		"ucupa-core/internal/query/federation/source/operation.go",
		"ucupa-core/internal/query/federation/source/part.go",
		"ucupa-core/internal/query/federation/source/projection.go",
		"ucupa-core/internal/query/local/operation.go",
		"ucupa-core/internal/query/local/read.go",
	}
	for _, file := range allowed {
		if reason := checkDependency(file, moduleImport(module, registryPackagePath), module, strings.HasSuffix(file, "_test.go"), stdlib); reason != "" {
			t.Errorf("approved Registry consumer rejected: %s: %s", file, reason)
		}
	}
	if reason := checkDependency("ucupa-core/internal/commit/public_durability_fixture_test.go", moduleImport(module, registryPackagePath), module, true, stdlib); reason != "" {
		t.Errorf("approved Registry test fixture rejected: %s", reason)
	}
	for _, file := range []string{
		"ucupa-core/internal/commit/public_durability_fixture_test.go",
		"ucupa-core/store.go",
		"ucupa-core/core.go",
		"ucupa-core/public_entry.go",
		"ucupa-core/public_attachment_control.go",
		"ucupa-core/public_proposal_control.go",
		"ucupa-core/internal/query/operation.go",
		"ucupa-core/internal/commit/state.go",
		"ucupa-core/internal/access/admission.go",
		"ucupa-core/internal/facet/service_proposal.go",
		"ucupa-core/transport/grpc/server.go",
		"ucupa-core/sdk/go/client.go",
	} {
		if reason := checkDependency(file, moduleImport(module, registryPackagePath), module, false, stdlib); reason == "" {
			t.Errorf("unapproved Registry consumer accepted: %s", file)
		}
	}
}

func TestAccessRegistryExportSurfaceIsOpaqueAndFinite(t *testing.T) {
	root, err := currentRepositoryRoot(t)
	if err != nil {
		t.Fatal(err)
	}
	files := []string{"registry.go", "read.go", "source.go", "observation.go"}
	exportedTypes := map[string]bool{
		"SnapshotOwner": true, "Ref": true, "Reservation": true, "Registry": true,
		"SpaceUse": true, "ReadUse": true, "ContinuationUse": true,
		"SourceConfig": true, "SourceUse": true,
		"ChangeGroup": true, "PreparationUse": true, "CursorUse": true,
	}
	expectedFunctions := map[string]bool{"New": true, "CheckSourceConfig": true}
	expectedMethods := map[string]bool{
		"ReserveLocked": true, "ReleaseLocked": true, "Release": true, "RemoveLocked": true,
		"ExpireLocked": true, "CloseLocked": true, "DrainRetired": true,
		"DenialDeadlineLocked": true, "DenialExpiredLocked": true, "Publish": true,
		"IssueSpaceLocked": true, "ResolveSpaceLocked": true, "SpaceLocked": true,
		"IssueLocalReadLocked": true, "IssueCompositeReadLocked": true, "IssueDerivedReadLocked": true,
		"ResolveReadLocked": true, "ReadLocked": true, "BindBasisLocked": true,
		"InstallSnapshotLocked": true, "InstallCompositeLocked": true, "InstallExactLocked": true,
		"LinkPreparationLocked": true, "IssueContinuationLocked": true, "ResolveContinuationLocked": true,
		"PrepareLocalLocked": true, "PrepareSourceLocked": true, "PrepareControlLocked": true,
		"PrepareCompositeLocked": true, "PrepareHiddenLocked": true, "ResolvePreparationLocked": true,
		"PreparationLocked": true, "ResolveCursorLocked": true, "CursorLocked": true,
		"BeginLocalLocked": true, "PollLocalLocked": true, "CompositePositionLocked": true,
		"GapLocked": true, "BindSourceObservationLocked": true,
		"IssueSourceLocked": true, "IssueHiddenSourceLocked": true, "CheckSourceRelationsLocked": true,
		"SourceReadViewLocked": true, "BindSourceReadViewLocked": true, "BindSourceDependenciesLocked": true,
		"OwnAuthorizationReleaseLocked": true, // Cleanup metadata on the existing finite receipt, not another registry.
	}
	fset := token.NewFileSet()
	foundTypes := map[string]bool{}
	foundFunctions := map[string]bool{}
	foundMethods := map[string]bool{}
	for _, file := range files {
		filename := filepath.Join(root, "ucupa-core", "internal", "access", "registry", file)
		parsed, err := parser.ParseFile(fset, filename, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", filename, err)
		}
		for _, declaration := range parsed.Decls {
			switch declaration := declaration.(type) {
			case *ast.GenDecl:
				if declaration.Tok == token.VAR || declaration.Tok == token.CONST {
					for _, spec := range declaration.Specs {
						value, ok := spec.(*ast.ValueSpec)
						if !ok {
							continue
						}
						for _, name := range value.Names {
							if ast.IsExported(name.Name) {
								t.Errorf("%s exports unexpected variable/constant %s", file, name.Name)
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
					if !exportedTypes[typeSpec.Name.Name] {
						t.Errorf("%s exports unexpected type %s", file, typeSpec.Name.Name)
						continue
					}
					if foundTypes[typeSpec.Name.Name] {
						t.Errorf("exported type %s is declared more than once", typeSpec.Name.Name)
					}
					foundTypes[typeSpec.Name.Name] = true
					registryCheckExportedType(t, file, typeSpec, exportedTypes)
				}
			case *ast.FuncDecl:
				if !ast.IsExported(declaration.Name.Name) {
					continue
				}
				if declaration.Recv == nil {
					if !expectedFunctions[declaration.Name.Name] {
						t.Errorf("%s exports unexpected function %s", file, declaration.Name.Name)
					}
					foundFunctions[declaration.Name.Name] = true
					continue
				}
				if !expectedMethods[declaration.Name.Name] {
					t.Errorf("%s exports unexpected method %s", file, declaration.Name.Name)
					continue
				}
				if len(declaration.Recv.List) != 1 || guardASTType(declaration.Recv.List[0].Type) != "*Registry" {
					t.Errorf("%s %s has an unexpected receiver", file, declaration.Name.Name)
				}
				foundMethods[declaration.Name.Name] = true
			}
		}
	}
	for name := range exportedTypes {
		if !foundTypes[name] {
			t.Errorf("exported Registry type %s is missing", name)
		}
	}
	for name := range expectedFunctions {
		if !foundFunctions[name] {
			t.Errorf("Registry function %s is missing", name)
		}
	}
	for name := range expectedMethods {
		if !foundMethods[name] {
			t.Errorf("Registry method %s is missing", name)
		}
	}
}

func registryCheckExportedType(t *testing.T, file string, declaration *ast.TypeSpec, exportedTypes map[string]bool) {
	t.Helper()
	if declaration.Name.Name == "SnapshotOwner" {
		interfaceType, ok := declaration.Type.(*ast.InterfaceType)
		if !ok || len(interfaceType.Methods.List) != 1 {
			t.Errorf("%s SnapshotOwner must have exactly one method", file)
			return
		}
		method, ok := interfaceType.Methods.List[0].Type.(*ast.FuncType)
		if !ok || len(interfaceType.Methods.List[0].Names) != 1 || interfaceType.Methods.List[0].Names[0].Name != "DiscardOwnedSnapshot" {
			t.Errorf("%s SnapshotOwner method shape changed", file)
			return
		}
		if !sameStringSlice(expandedASTFieldTypes(method.Params), []string{"contract.CoreReadSnapshot"}) || method.Results != nil && len(method.Results.List) != 0 {
			t.Errorf("%s SnapshotOwner method signature changed", file)
		}
		return
	}
	structType, ok := declaration.Type.(*ast.StructType)
	if !ok {
		return
	}
	for _, field := range structType.Fields.List {
		for _, name := range field.Names {
			if ast.IsExported(name.Name) && (declaration.Name.Name == "Ref" || declaration.Name.Name == "Reservation" || declaration.Name.Name == "Registry") {
				t.Errorf("%s opaque type %s exports field %s", file, declaration.Name.Name, name.Name)
			}
		}
		if declaration.Name.Name == "Ref" || declaration.Name.Name == "Reservation" || declaration.Name.Name == "Registry" {
			continue
		}
		if registryContainsMutableField(field.Type) {
			t.Errorf("%s finite type %s exposes a map/function field", file, declaration.Name.Name)
		}
		fieldType := guardASTType(field.Type)
		for _, forbidden := range []string{"entry", "Registry", "sync", "Mutex", "State", "Store"} {
			if strings.Contains(fieldType, forbidden) {
				t.Errorf("%s finite type %s exposes forbidden field type %s", file, declaration.Name.Name, fieldType)
				break
			}
		}
	}
}

func registryContainsMutableField(expression ast.Expr) bool {
	switch expression := expression.(type) {
	case *ast.MapType, *ast.FuncType:
		return true
	case *ast.ArrayType:
		return registryContainsMutableField(expression.Elt)
	case *ast.StarExpr:
		return registryContainsMutableField(expression.X)
	default:
		return false
	}
}

func TestOpaqueRegistrySurfaceCompilesOnlyAsFiniteHandles(t *testing.T) {
	positive := `package main

import (
	contract "ucupa/ucupa-core/contract"
	registry "ucupa/ucupa-core/internal/access/registry"
)

type sink struct{}

func (sink) DiscardOwnedSnapshot(contract.CoreReadSnapshot) {}

var _ registry.SnapshotOwner = sink{}

func main() {
	_ = registry.Ref{}
	_ = registry.Reservation{}
	_ = registry.Registry{}
	_ = registry.SpaceUse{}
	_ = registry.ReadUse{}
	_ = registry.ContinuationUse{}
	_ = registry.SourceConfig{}
	_ = registry.SourceUse{}
	_ = registry.ChangeGroup{}
	_ = registry.PreparationUse{}
	_ = registry.CursorUse{}
}
`
	if output, err := buildRegistryConsumer(t, positive); err != nil {
		t.Fatalf("finite Registry consumer failed to compile: %v\n%s", err, output)
	}

	for _, source := range []string{
		`package main
import registry "ucupa/ucupa-core/internal/access/registry"
func main() { var r *registry.Registry; _ = r.Entries }
`,
		`package main
import registry "ucupa/ucupa-core/internal/access/registry"
func main() { var r *registry.Registry; _ = r.Bytes }
`,
		`package main
import registry "ucupa/ucupa-core/internal/access/registry"
func main() { var r *registry.Registry; _ = r.State }
`,
		`package main
import registry "ucupa/ucupa-core/internal/access/registry"
func main() { var r *registry.Registry; _ = r.Store }
`,
		`package main
import registry "ucupa/ucupa-core/internal/access/registry"
func main() { var r *registry.Registry; _ = r.Key }
`,
		`package main
import registry "ucupa/ucupa-core/internal/access/registry"
func main() { var r *registry.Registry; _ = r.Locker }
`,
		`package main
import registry "ucupa/ucupa-core/internal/access/registry"
func main() { _ = registry.Ref{entry: nil} }
`,
		`package main
import registry "ucupa/ucupa-core/internal/access/registry"
func main() { _ = registry.Reservation{registry: nil} }
`,
	} {
		output, err := buildRegistryConsumer(t, source)
		if err == nil {
			t.Fatalf("private Registry surface unexpectedly compiled:\n%s", output)
		}
		if strings.Contains(output, "use of internal package") ||
			!strings.Contains(output, "undefined") && !strings.Contains(output, "unexported") {
			t.Fatalf("private Registry surface failed for the wrong reason: %v\n%s", err, output)
		}
	}
}

func buildRegistryConsumer(t *testing.T, source string) (string, error) {
	t.Helper()
	root, err := currentRepositoryRoot(t)
	if err != nil {
		return "", err
	}
	goTool, err := findGo()
	if err != nil {
		return "", err
	}
	directory, err := os.MkdirTemp(filepath.Join(root, "ucupa-core"), ".registry-build-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(directory)
	writeFixture(t, directory, "main.go", source)
	outputPath := filepath.Join(directory, "consumer.exe")
	command := exec.Command(goTool, "build", "-o", outputPath, filepath.Join(directory, "main.go"))
	command.Dir = root
	command.Env = commandEnvironment(false)
	output, err := command.CombinedOutput()
	return string(output), err
}

func moduleImport(module, dependency string) string {
	if dependency == "shared/values" || strings.HasPrefix(dependency, "ucupa-core/") || strings.HasPrefix(dependency, "ucupa-facet/") {
		return module + "/" + dependency
	}
	return dependency
}
