package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
	"testing"
)

const coreFederationSourceDirectory = "ucupa-core/internal/query/federation/source"

var coreFederationSourceProductionFiles = []string{
	"check.go", "denial.go", "evidence.go", "hidden.go", "local.go",
	"observe_binding.go", "observe.go", "operation.go", "part.go", "projection.go",
}

var coreFederationSourceTestFiles = []string{"evidence_test.go", "fixture_test.go", "observe_test.go", "operation_test.go", "relation_test.go"}

func TestReviewedCoreFederationSourceBoundary(t *testing.T) {
	if !reviewedCoreFederationSourcePackage("ucupa-core", coreFederationSourceDirectory) ||
		reviewedCoreFederationSourcePackage("ucupa-facet", coreFederationSourceDirectory) ||
		reviewedCoreFederationSourcePackage("ucupa-core", coreFederationSourceDirectory+"/child") {
		t.Fatal("Core Federation source registration must identify exactly one package")
	}
	for _, name := range append(coreFederationSourceProductionFiles, coreFederationSourceTestFiles...) {
		file := coreFederationSourceDirectory + "/" + name
		if got := checkSourceLocation(file); got != "" {
			t.Errorf("reviewed Federation source file %s rejected: %s", file, got)
		}
	}
	for _, file := range []string{
		coreFederationSourceDirectory + "/extra.go",
		coreFederationSourceDirectory + "/extra_test.go",
		coreFederationSourceDirectory + "/child/source.go",
		"ucupa-core/internal/query/federation/source/nested/part.go",
	} {
		if got := checkSourceLocation(file); got == "" {
			t.Errorf("unreviewed Federation source file accepted: %s", file)
		}
	}
}

func TestCoreFederationSourceDependenciesAreExact(t *testing.T) {
	stdlib := federationSourceStdlib()
	production := map[string][]string{
		"check.go":           {"context", "shared/values", "ucupa-core/contract", "ucupa-core/internal/access", "ucupa-core/internal/access/registry", "ucupa-core/internal/access/value", "ucupa-core/internal/query/federation/denial"},
		"denial.go":          {"context", "shared/values", "ucupa-core/contract/federation", "ucupa-core/internal/access", "ucupa-core/internal/mount/fact", "ucupa-core/internal/query/federation/denial"},
		"evidence.go":        {"crypto/hmac", "crypto/sha256", "encoding/binary", "shared/values", "ucupa-core/contract/federation", "ucupa-core/internal/access/registry", "ucupa-core/internal/query/federation/denial", "ucupa-core/internal/query/federation/value", "ucupa-core/internal/token"},
		"hidden.go":          {"context", "shared/values", "ucupa-core/contract", "ucupa-core/contract/federation", "ucupa-core/internal/access", "ucupa-core/internal/access/registry", "ucupa-core/internal/access/value", "ucupa-core/internal/query/federation/denial", "ucupa-core/internal/query/federation/projection"},
		"local.go":           {"context", "shared/values", "ucupa-core/contract/federation"},
		"observe_binding.go": {"shared/values", "ucupa-core/contract/federation", "ucupa-core/internal/access/registry", "ucupa-core/internal/geometry", "ucupa-core/internal/query/federation/projection"},
		"observe.go":         {"context", "shared/values", "ucupa-core/contract/federation", "ucupa-core/internal/access", "ucupa-core/internal/access/registry", "ucupa-core/internal/access/value", "ucupa-core/internal/query/federation/denial"},
		"operation.go":       {"context", "shared/values", "ucupa-core/contract", "ucupa-core/internal/access", "ucupa-core/internal/access/observation", "ucupa-core/internal/access/registry", "ucupa-core/internal/commit", "ucupa-core/internal/query"},
		"part.go":            {"context", "errors", "shared/values", "ucupa-core/contract", "ucupa-core/contract/federation", "ucupa-core/internal/access", "ucupa-core/internal/access/registry", "ucupa-core/internal/mount/fact", "ucupa-core/internal/query/federation/projection"},
		"projection.go":      {"context", "crypto/sha256", "encoding/binary", "encoding/json", "math", "shared/values", "ucupa-core/contract", "ucupa-core/contract/federation", "ucupa-core/internal/access", "ucupa-core/internal/access/registry", "ucupa-core/internal/access/value", "ucupa-core/internal/geometry", "ucupa-core/internal/query", "ucupa-core/internal/query/federation/denial", "ucupa-core/internal/query/federation/projection", "ucupa-core/internal/query/request"},
	}
	for file, dependencies := range production {
		for _, dependency := range dependencies {
			if reason := checkDependency(coreFederationSourceDirectory+"/"+file, moduleImport("ucupa", dependency), "ucupa", false, stdlib); reason != "" {
				t.Errorf("allowed Federation source dependency %s -> %s: %s", file, dependency, reason)
			}
		}
	}
	tests := map[string][]string{
		"evidence_test.go":  {"context", "slices", "testing", "shared/values", "ucupa-core/contract", "ucupa-core/contract/federation", "ucupa-core/internal/access/registry", "ucupa-core/internal/query/federation/projection", "ucupa-core/internal/query/federation/value", "ucupa-core/internal/token"},
		"fixture_test.go":   {"context", "crypto/sha256", "fmt", "testing", "time", "shared/values", "ucupa-core/contract", "ucupa-core/internal/access", "ucupa-core/internal/access/observation", "ucupa-core/internal/access/registry", "ucupa-core/internal/commit", "ucupa-core/internal/element", "ucupa-core/internal/mount", "ucupa-core/internal/query"},
		"observe_test.go":   {"testing", "shared/values", "ucupa-core/contract/federation"},
		"operation_test.go": {"context", "errors", "reflect", "sort", "testing", "shared/values", "ucupa-core/contract", "ucupa-core/contract/federation", "ucupa-core/internal/access"},
		"relation_test.go":  {"context", "errors", "testing", "shared/values", "ucupa-core/contract", "ucupa-core/contract/federation", "ucupa-core/internal/mount"},
	}
	for file, dependencies := range tests {
		for _, dependency := range dependencies {
			if reason := checkDependency(coreFederationSourceDirectory+"/"+file, moduleImport("ucupa", dependency), "ucupa", true, stdlib); reason != "" {
				t.Errorf("allowed Federation source test dependency %s -> %s: %s", file, dependency, reason)
			}
		}
	}
	for _, file := range coreFederationSourceProductionFiles {
		for _, dependency := range []string{
			"ucupa-core", "ucupa-core/internal/store", "ucupa-core/internal/query/connected",
			"ucupa-core/internal/facet", "ucupa-core/internal/mount", "ucupa-core/internal/element",
			"ucupa-core/transport/grpc", "ucupa-core/sdk/go", "ucupa-facet/contract", "go.etcd.io/bbolt",
		} {
			if reason := checkDependency(coreFederationSourceDirectory+"/"+file, moduleImport("ucupa", dependency), "ucupa", false, stdlib); reason == "" {
				t.Errorf("forbidden Federation source dependency accepted: %s -> %s", file, dependency)
			}
		}
	}
	for _, file := range coreFederationSourceTestFiles {
		for _, dependency := range []string{
			"ucupa-core", "ucupa-core/internal/store", "ucupa-core/internal/query/federation/projection",
			"ucupa-core/internal/mount", "ucupa-core/transport/grpc", "ucupa-core/sdk/go", "go.etcd.io/bbolt",
		} {
			if file == "evidence_test.go" && dependency == "ucupa-core/internal/query/federation/projection" ||
				(file == "fixture_test.go" || file == "relation_test.go") && dependency == "ucupa-core/internal/mount" {
				continue // Exact value proof / real Mount fixture above, not a package-wide exception.
			}
			if reason := checkDependency(coreFederationSourceDirectory+"/"+file, moduleImport("ucupa", dependency), "ucupa", true, stdlib); reason == "" {
				t.Errorf("forbidden Federation source test dependency accepted: %s -> %s", file, dependency)
			}
		}
	}
	for _, file := range append(coreFederationSourceProductionFiles, coreFederationSourceTestFiles...) {
		assertActualImportsAccepted(t, coreFederationSourceDirectory+"/"+file)
	}
}

func TestCoreFederationSourceConsumersAreExact(t *testing.T) {
	stdlib := federationSourceStdlib()
	imported := moduleImport("ucupa", coreFederationSourceDirectory)
	for _, file := range []string{"ucupa-core/instance/instance.go", coreCoordinatorDirectory + "/operation.go", coreCoordinatorDirectory + "/read.go"} {
		if reason := checkDependency(file, imported, "ucupa", false, stdlib); reason != "" {
			t.Errorf("approved Federation source consumer rejected: %s: %s", file, reason)
		}
	}
	for _, file := range []string{
		"ucupa-core/public_context.go", "ucupa-core/public_federated_read.go", "ucupa-core/public_entry.go", "ucupa-core/public_federated_observe.go",
		"ucupa-core/public_federation_observe.go", "ucupa-core/public_federation_part.go", "ucupa-core/public_federation_projection.go",
		"ucupa-core/store.go", "ucupa-core/internal/access/observation/operation.go", "ucupa-core/internal/access/registry/registry.go",
		"ucupa-core/internal/query/operation.go", "ucupa-core/internal/query/federation/collect/read.go",
		"ucupa-core/internal/facet/operation.go", "ucupa-core/transport/grpc/server.go", "ucupa-core/sdk/go/client.go",
		coreFederationSourceDirectory + "/operation.go",
	} {
		if reason := checkDependency(file, imported, "ucupa", false, stdlib); reason == "" {
			t.Errorf("unapproved Federation source consumer accepted: %s", file)
		}
	}
}

func TestCoreFederationSourceExportSurfaceIsNarrow(t *testing.T) {
	root, err := currentRepositoryRoot(t)
	if err != nil {
		t.Fatal(err)
	}
	wantTypes := map[string]bool{"Operations": true}
	wantFunctions := map[string]bool{"New": true, "OpenLocal": true}
	wantMethods := map[string]map[string]bool{
		"Operations": {
			"AuthorityRef": true, "CaptureFederationDenial": true, "CheckFederationDenial": true,
			"CheckFederationReadPart": true, "ObserveFederationReadPart": true,
			"OpenFederationReadPart": true, "ProjectFederationReadPart": true,
			"ReleaseFederationReadPart": true,
		},
		"localFederationPart": {"Part": true, "Project": true, "Check": true, "Observe": true, "Release": true},
	}
	wantFields := map[string]map[string]string{
		"Operations": {
			"owner": "*commit.Owner", "admission": "*accesspkg.Admission",
			"registry": "*accessregistry.Registry", "queries": "*query.Operations", "observations": "*observation.Operations",
		},
		"localFederationPart": {
			"owner": "*Operations", "caller": "values.CallerContext", "source": "values.FactAuthorityRef",
			"part": "federationcontract.FederationReadPart",
		},
	}
	foundTypes := map[string]bool{}
	foundFunctions := map[string]bool{}
	foundMethods := map[string]map[string]bool{}
	fset := token.NewFileSet()
	for _, file := range coreFederationSourceProductionFiles {
		filename := filepath.Join(root, filepath.FromSlash(coreFederationSourceDirectory), file)
		parsed, err := parser.ParseFile(fset, filename, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", filename, err)
		}
		for _, declaration := range parsed.Decls {
			switch declaration := declaration.(type) {
			case *ast.GenDecl:
				if declaration.Tok == token.VAR || declaration.Tok == token.CONST {
					for _, spec := range declaration.Specs {
						if value, ok := spec.(*ast.ValueSpec); ok {
							for _, name := range value.Names {
								if ast.IsExported(name.Name) {
									t.Errorf("%s exports unexpected variable/constant %s", file, name.Name)
								}
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
					if !wantTypes[typeSpec.Name.Name] || foundTypes[typeSpec.Name.Name] {
						t.Errorf("%s exports unexpected or duplicate type %s", file, typeSpec.Name.Name)
						continue
					}
					foundTypes[typeSpec.Name.Name] = true
					structType, ok := typeSpec.Type.(*ast.StructType)
					if !ok {
						t.Errorf("%s exported type %s is not a struct", file, typeSpec.Name.Name)
						continue
					}
					fields := map[string]string{}
					for _, field := range structType.Fields.List {
						for _, name := range field.Names {
							if ast.IsExported(name.Name) {
								t.Errorf("%s.%s exports field %s", file, typeSpec.Name.Name, name.Name)
							}
							fields[name.Name] = guardASTType(field.Type)
						}
					}
					want, ok := wantFields[typeSpec.Name.Name]
					if !ok || len(fields) != len(want) {
						t.Errorf("%s.%s fields=%v, want=%v", file, typeSpec.Name.Name, fields, want)
					} else {
						for name, typ := range want {
							if fields[name] != typ {
								t.Errorf("%s.%s.%s=%q, want %q", file, typeSpec.Name.Name, name, fields[name], typ)
							}
						}
					}
				}
			case *ast.FuncDecl:
				if !ast.IsExported(declaration.Name.Name) {
					continue
				}
				if declaration.Recv == nil {
					if !wantFunctions[declaration.Name.Name] || foundFunctions[declaration.Name.Name] {
						t.Errorf("%s exports unexpected or duplicate function %s", file, declaration.Name.Name)
						continue
					}
					foundFunctions[declaration.Name.Name] = true
					continue
				}
				if len(declaration.Recv.List) != 1 {
					t.Errorf("%s.%s has an unexpected receiver list", file, declaration.Name.Name)
					continue
				}
				receiver := strings.TrimPrefix(guardASTType(declaration.Recv.List[0].Type), "*")
				methods, ok := wantMethods[receiver]
				if !ok || !methods[declaration.Name.Name] {
					t.Errorf("%s exports unexpected method %s on %s", file, declaration.Name.Name, receiver)
					continue
				}
				if guardASTType(declaration.Recv.List[0].Type) != "*"+receiver {
					t.Errorf("%s.%s must use a pointer receiver", file, declaration.Name.Name)
				}
				if foundMethods[receiver] == nil {
					foundMethods[receiver] = map[string]bool{}
				}
				if foundMethods[receiver][declaration.Name.Name] {
					t.Errorf("%s.%s is declared more than once", file, declaration.Name.Name)
				}
				foundMethods[receiver][declaration.Name.Name] = true
			}
		}
	}
	for name := range wantTypes {
		if !foundTypes[name] {
			t.Errorf("Federation source type %s is missing", name)
		}
	}
	for name := range wantFunctions {
		if !foundFunctions[name] {
			t.Errorf("Federation source function %s is missing", name)
		}
	}
	for receiver, methods := range wantMethods {
		for name := range methods {
			if !foundMethods[receiver][name] {
				t.Errorf("Federation source method %s.%s is missing", receiver, name)
			}
		}
	}
}

func TestCoreFederationSourceAdmissionBoundary(t *testing.T) {
	root, err := currentRepositoryRoot(t)
	if err != nil {
		t.Fatal(err)
	}
	parse := func(relative string) *ast.File {
		t.Helper()
		filename := filepath.Join(root, filepath.FromSlash(relative))
		parsed, err := parser.ParseFile(token.NewFileSet(), filename, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", relative, err)
		}
		return parsed
	}
	operation := parse(coreFederationSourceDirectory + "/operation.go")
	var hasAdmission, withAdmission bool
	ast.Inspect(operation, func(node ast.Node) bool {
		selector, ok := node.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		switch selector.Sel.Name {
		case "HasAdmission":
			hasAdmission = true
		case "WithAdmission":
			withAdmission = true
		}
		return true
	})
	if !hasAdmission || withAdmission {
		t.Fatalf("source operation admission calls: HasAdmission=%v WithAdmission=%v", hasAdmission, withAdmission)
	}
	publicContext := parse("ucupa-core/internal/access/entry/request.go")
	var acquirePos, withPos token.Pos
	ast.Inspect(publicContext, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		selector, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		switch selector.Sel.Name {
		case "Acquire":
			if acquirePos == token.NoPos || call.Pos() < acquirePos {
				acquirePos = call.Pos()
			}
		case "WithAdmission":
			if withPos == token.NoPos || call.Pos() < withPos {
				withPos = call.Pos()
			}
		}
		return true
	})
	if acquirePos == token.NoPos || withPos == token.NoPos || acquirePos >= withPos {
		t.Fatalf("public context must acquire before WithAdmission: acquire=%v with=%v", acquirePos, withPos)
	}
}

func federationSourceStdlib() map[string]bool {
	stdlib := guardOracleStdlib()
	for _, dependency := range []string{"crypto/hmac", "crypto/sha256", "encoding/base64", "encoding/binary", "encoding/json", "strconv"} {
		stdlib[dependency] = true
	}
	return stdlib
}
