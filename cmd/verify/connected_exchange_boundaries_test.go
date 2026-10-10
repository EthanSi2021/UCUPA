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

const (
	coreConnectedExchangeDirectory = "ucupa-core/internal/query/exchange"
	coreConnectedDirectory         = "ucupa-core/internal/query/connected"
	coreConnectedContractDirectory = "ucupa-core/contract/connected"
)

var coreConnectedExchangeSourceFiles = []string{"value.go", "cleanup.go", "exchange_test.go"}

var coreConnectedSourceFiles = []string{
	"operation.go",
	"access.go",
	"lifecycle.go",
	"pair_boundary.go",
	"peer.go",
	"plan.go",
	"query.go",
	"read.go",
	"registry.go",
	"replan.go",
	"world.go",
	"operation_test.go",
	"peer_test.go",
	"review_fixture_test.go",
	"review_test.go",
}

var coreConnectedContractSourceFiles = []string{"connected.go", "connected_test.go"}

func TestReviewedCoreConnectedExchangeBoundary(t *testing.T) {
	if !reviewedCoreConnectedExchangePackage("ucupa-core", coreConnectedExchangeDirectory) ||
		reviewedCoreConnectedExchangePackage("ucupa-facet", coreConnectedExchangeDirectory) ||
		reviewedCoreConnectedExchangePackage("ucupa-core", coreConnectedExchangeDirectory+"/child") {
		t.Fatal("Core connected exchange registration must identify exactly one package")
	}
	for _, name := range coreConnectedExchangeSourceFiles {
		if got := checkSourceLocation(coreConnectedExchangeDirectory + "/" + name); got != "" {
			t.Errorf("reviewed connected exchange source %s: %s", name, got)
		}
	}
	for _, file := range []string{
		coreConnectedExchangeDirectory + "/extra.go",
		coreConnectedExchangeDirectory + "/testexchange_test.go",
		coreConnectedExchangeDirectory + "/child/value.go",
		"ucupa-core/internal/query/exchange/connected.go",
	} {
		if got := checkSourceLocation(file); got == "" {
			t.Errorf("unreviewed connected exchange source accepted: %s", file)
		}
	}

	stdlib := connectedStdlib()
	for _, dependency := range []string{
		"sync",
		"ucupa/shared/values",
		"ucupa/ucupa-core/contract",
		"ucupa/ucupa-core/internal/element/fact",
		"ucupa/ucupa-core/internal/facet/fact",
		"ucupa/ucupa-core/internal/mount/fact",
	} {
		if got := checkDependency(coreConnectedExchangeDirectory+"/value.go", dependency, "ucupa", false, stdlib); got != "" {
			t.Errorf("allowed connected exchange dependency %s: %s", dependency, got)
		}
	}
	for _, dependency := range []string{
		"ucupa/ucupa-core",
		"ucupa/ucupa-core/contract/connected",
		"ucupa/ucupa-core/instance",
		"ucupa/ucupa-core/internal/query",
		"ucupa/ucupa-core/internal/query/request",
		"ucupa/ucupa-core/internal/query/scan",
		"ucupa/ucupa-core/internal/commit",
		"ucupa/ucupa-core/internal/store",
		"ucupa/ucupa-core/internal/token",
		"ucupa/ucupa-core/internal/geometry",
		"ucupa/ucupa-core/internal/access",
		"ucupa/ucupa-core/transport/grpc",
		"ucupa/ucupa-core/sdk/go",
		"go.etcd.io/bbolt",
	} {
		if got := checkDependency(coreConnectedExchangeDirectory+"/value.go", dependency, "ucupa", false, stdlib); got == "" {
			t.Errorf("forbidden connected exchange dependency accepted: %s", dependency)
		}
	}
	for _, source := range []string{
		"ucupa-core/contract/connected/connected.go",
		"ucupa-core/internal/query/connected/peer.go",
	} {
		if got := checkDependency(source, "ucupa/"+coreConnectedExchangeDirectory, "ucupa", false, stdlib); got != "" {
			t.Errorf("approved connected exchange consumer %s rejected: %s", source, got)
		}
	}
	for _, source := range []string{
		"ucupa-core/core.go",
		"ucupa-core/public_context.go",
		"ucupa-core/contract/query.go",
		"ucupa-core/internal/query/operation.go",
		"ucupa-core/internal/commit/state.go",
		"ucupa-core/internal/element/fact/state.go",
		"composition/connected_connections.go",
		"ucupa-core/transport/grpc/server.go",
		"ucupa-core/sdk/go/client.go",
	} {
		if got := checkDependency(source, "ucupa/"+coreConnectedExchangeDirectory, "ucupa", false, stdlib); got == "" {
			t.Errorf("unapproved connected exchange consumer accepted: %s", source)
		}
	}
}

func TestReviewedCoreConnectedOperationsBoundary(t *testing.T) {
	if !reviewedCoreConnectedPackage("ucupa-core", coreConnectedDirectory) ||
		reviewedCoreConnectedPackage("ucupa-facet", coreConnectedDirectory) ||
		reviewedCoreConnectedPackage("ucupa-core", coreConnectedDirectory+"/child") {
		t.Fatal("Core connected registration must identify exactly one package")
	}
	for _, name := range coreConnectedSourceFiles {
		if got := checkSourceLocation(coreConnectedDirectory + "/" + name); got != "" {
			t.Errorf("reviewed connected source %s: %s", name, got)
		}
	}
	for _, file := range []string{
		coreConnectedDirectory + "/extra.go",
		coreConnectedDirectory + "/connected_test.go",
		coreConnectedDirectory + "/child/operation.go",
		"ucupa-core/internal/query/connected/exchange.go",
	} {
		if got := checkSourceLocation(file); got == "" {
			t.Errorf("unreviewed connected source accepted: %s", file)
		}
	}

	stdlib := connectedStdlib()
	for _, dependency := range []string{
		"context",
		"crypto/sha256",
		"encoding/binary",
		"errors",
		"math",
		"reflect",
		"slices",
		"sort",
		"sync",
		"time",
		"ucupa/shared/values",
		"ucupa/ucupa-core/contract",
		"ucupa/ucupa-core/contract/connected",
		"ucupa/ucupa-core/internal/commit",
		"ucupa/ucupa-core/internal/element/fact",
		"ucupa/ucupa-core/internal/facet/fact",
		"ucupa/ucupa-core/internal/geometry",
		"ucupa/ucupa-core/internal/mount/fact",
		"ucupa/ucupa-core/internal/query",
		"ucupa/ucupa-core/internal/query/exchange",
		"ucupa/ucupa-core/internal/query/request",
		"ucupa/ucupa-core/internal/query/scan",
		"ucupa/ucupa-core/internal/token",
	} {
		if got := checkDependency(coreConnectedDirectory+"/operation.go", dependency, "ucupa", false, stdlib); got != "" {
			t.Errorf("allowed connected dependency %s: %s", dependency, got)
		}
	}
	for _, dependency := range []string{
		"ucupa/ucupa-core",
		"ucupa/ucupa-core/instance",
		"ucupa/ucupa-core/internal/access",
		"ucupa/ucupa-core/internal/element",
		"ucupa/ucupa-core/internal/facet",
		"ucupa/ucupa-core/internal/mount",
		"ucupa/ucupa-core/internal/store",
		"ucupa/ucupa-core/internal/query/connected",
		"ucupa/ucupa-core/transport/grpc",
		"ucupa/ucupa-core/sdk/go",
		"ucupa/ucupa-facet/contract",
		"ucupa/composition",
		"go.etcd.io/bbolt",
	} {
		if got := checkDependency(coreConnectedDirectory+"/operation.go", dependency, "ucupa", false, stdlib); got == "" {
			t.Errorf("forbidden connected dependency accepted: %s", dependency)
		}
	}
	if got := checkDependency(coreConnectedDirectory+"/operation_test.go", "ucupa/"+coreConnectedDirectory, "ucupa", true, stdlib); got != "" {
		t.Fatalf("connected white-box test self dependency rejected: %s", got)
	}
	for _, source := range []string{
		"ucupa-core/public_context.go",
		"ucupa-core/internal/query/operation.go",
		"ucupa-core/internal/access/admission.go",
	} {
		if got := checkDependency(source, "ucupa/"+coreConnectedDirectory, "ucupa", false, stdlib); got == "" {
			t.Errorf("unapproved connected consumer accepted: %s", source)
		}
	}
	for _, source := range []string{"ucupa-core/instance/instance.go", "ucupa-core/instance/store.go"} {
		if got := checkDependency(source, "ucupa/"+coreConnectedDirectory, "ucupa", false, stdlib); got != "" {
			t.Fatalf("Core assembly source %s was rejected as connected consumer: %s", source, got)
		}
	}
}

func TestReviewedCoreConnectedContractBoundary(t *testing.T) {
	if !reviewedCoreConnectedContractPackage("ucupa-core", coreConnectedContractDirectory) ||
		reviewedCoreConnectedContractPackage("ucupa-facet", coreConnectedContractDirectory) ||
		reviewedCoreConnectedContractPackage("ucupa-core", coreConnectedContractDirectory+"/child") {
		t.Fatal("Core connected contract registration must identify exactly one package")
	}
	for _, name := range coreConnectedContractSourceFiles {
		if got := checkSourceLocation(coreConnectedContractDirectory + "/" + name); got != "" {
			t.Errorf("reviewed connected contract source %s: %s", name, got)
		}
	}
	for _, file := range []string{
		coreConnectedContractDirectory + "/extra.go",
		coreConnectedContractDirectory + "/request.go",
		coreConnectedContractDirectory + "/child/connected.go",
	} {
		if got := checkSourceLocation(file); got == "" {
			t.Errorf("unreviewed connected contract source accepted: %s", file)
		}
	}

	stdlib := connectedStdlib()
	for _, dependency := range []string{
		"context",
		"errors",
		"ucupa/shared/values",
		"ucupa/ucupa-core/internal/query/exchange",
	} {
		if got := checkDependency(coreConnectedContractDirectory+"/connected.go", dependency, "ucupa", false, stdlib); got != "" {
			t.Errorf("allowed connected contract dependency %s: %s", dependency, got)
		}
	}
	for _, dependency := range []string{
		"ucupa/ucupa-core",
		"ucupa/ucupa-core/contract",
		"ucupa/ucupa-core/internal/query",
		"ucupa/ucupa-core/internal/query/request",
		"ucupa/ucupa-core/internal/commit",
		"ucupa/ucupa-core/internal/element/fact",
		"ucupa/ucupa-core/internal/token",
		"go.etcd.io/bbolt",
	} {
		if got := checkDependency(coreConnectedContractDirectory+"/connected.go", dependency, "ucupa", false, stdlib); got == "" {
			t.Errorf("forbidden connected contract dependency accepted: %s", dependency)
		}
	}
	for _, source := range []string{
		"ucupa-core/internal/query/connected/operation.go",
		"composition/connected_connections.go",
	} {
		if got := checkDependency(source, "ucupa/"+coreConnectedContractDirectory, "ucupa", false, stdlib); got != "" {
			t.Errorf("approved connected contract consumer %s rejected: %s", source, got)
		}
	}
	for _, source := range []string{
		"ucupa-core/instance/instance.go",
		"ucupa-core/contract/query.go",
		"ucupa-core/internal/query/operation.go",
		"ucupa-core/internal/commit/state.go",
		"ucupa-core/internal/element/fact/state.go",
		"ucupa-core/internal/mount/fact/state.go",
	} {
		if got := checkDependency(source, "ucupa/"+coreConnectedContractDirectory, "ucupa", false, stdlib); got == "" {
			t.Errorf("reverse connected contract consumer accepted: %s", source)
		}
	}
}

func TestConnectedContractExposesOnlyOpaqueValuesAndNarrowPorts(t *testing.T) {
	working, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	root, err := repositoryRoot(working)
	if err != nil {
		t.Fatal(err)
	}
	file := coreConnectedContractDirectory + "/connected.go"
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(file)))
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := parser.ParseFile(token.NewFileSet(), file, data, parser.ParseComments)
	if err != nil {
		t.Fatal(err)
	}
	found := map[string]bool{}
	for _, declaration := range parsed.Decls {
		switch declaration := declaration.(type) {
		case *ast.GenDecl:
			switch declaration.Tok {
			case token.TYPE:
				for _, specification := range declaration.Specs {
					typeSpec, ok := specification.(*ast.TypeSpec)
					if !ok || !ast.IsExported(typeSpec.Name.Name) {
						continue
					}
					found[typeSpec.Name.Name] = true
					switch typeSpec.Name.Name {
					case "Request", "Reply":
						if typeSpec.Assign == token.NoPos || selectorText(typeSpec.Type) != "exchange."+typeSpec.Name.Name {
							t.Errorf("%s must alias exchange.%s without a public payload", file, typeSpec.Name.Name)
						}
					case "PeerPort":
						checkConnectedPortInterface(t, typeSpec, []string{"AuthorityRef", "ReadConnected"})
					case "Transport":
						checkConnectedPortInterface(t, typeSpec, []string{"ReadConnected"})
					default:
						t.Errorf("unexpected exported connected contract type %s", typeSpec.Name.Name)
					}
				}
			case token.VAR:
				for _, specification := range declaration.Specs {
					valueSpec, ok := specification.(*ast.ValueSpec)
					if !ok {
						continue
					}
					for _, name := range valueSpec.Names {
						if !ast.IsExported(name.Name) {
							continue
						}
						if name.Name != "ErrIdentityConflict" {
							t.Errorf("unexpected exported connected contract variable %s", name.Name)
							continue
						}
						found[name.Name] = true
					}
				}
			case token.CONST:
				for _, specification := range declaration.Specs {
					valueSpec, ok := specification.(*ast.ValueSpec)
					if !ok {
						continue
					}
					for _, name := range valueSpec.Names {
						if ast.IsExported(name.Name) {
							t.Errorf("unexpected exported connected contract constant %s", name.Name)
						}
					}
				}
			}
		case *ast.FuncDecl:
			if declaration.Name != nil && ast.IsExported(declaration.Name.Name) {
				t.Errorf("connected contract exposes function %s", declaration.Name.Name)
			}
		}
	}
	for _, name := range []string{"Request", "Reply", "PeerPort", "Transport"} {
		if !found[name] {
			t.Errorf("connected contract lost required declaration %s", name)
		}
	}
	if !found["ErrIdentityConflict"] {
		t.Fatal("connected contract lost the existing identity-conflict sentinel")
	}
	if bytes := string(data); strings.Contains(bytes, "NewRequest") || strings.Contains(bytes, "ReadRequest") || strings.Contains(bytes, "NewReply") || strings.Contains(bytes, "ReadReply") {
		t.Fatal("connected contract exposes exchange constructors or readers")
	}
}

func TestConnectedExternalConsumerCannotReachExchangePayload(t *testing.T) {
	stdlib := connectedStdlib()
	for _, tc := range []struct {
		source, dependency string
		allowed            bool
	}{
		{"composition/connected_connections.go", "ucupa/ucupa-core/contract/connected", true},
		{"composition/connected_connections.go", "ucupa/ucupa-core/internal/query/exchange", false},
		{"acceptance/connected_fixture_test.go", "ucupa/ucupa-core/contract/connected", true},
		{"acceptance/connected_fixture_test.go", "ucupa/ucupa-core/internal/query/exchange", false},
		{"ucupa-core/transport/grpc/server.go", "ucupa/ucupa-core/internal/query/exchange", false},
		{"ucupa-core/sdk/go/client.go", "ucupa/ucupa-core/internal/query/exchange", false},
	} {
		got := checkDependency(tc.source, tc.dependency, "ucupa", strings.HasSuffix(tc.source, "_test.go"), stdlib)
		if (got == "") != tc.allowed {
			t.Errorf("%s -> %s: allowed=%v got=%q", tc.source, tc.dependency, tc.allowed, got)
		}
	}
}

func checkConnectedPortInterface(t *testing.T, specification *ast.TypeSpec, want []string) {
	t.Helper()
	interfaceType, ok := specification.Type.(*ast.InterfaceType)
	if !ok {
		t.Errorf("%s must be an interface", specification.Name.Name)
		return
	}
	got := make([]string, 0, len(interfaceType.Methods.List))
	for _, field := range interfaceType.Methods.List {
		if len(field.Names) != 1 {
			t.Errorf("%s has an embedded or unnamed port member", specification.Name.Name)
			continue
		}
		got = append(got, field.Names[0].Name)
	}
	sort.Strings(got)
	sort.Strings(want)
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("%s methods=%v, want=%v", specification.Name.Name, got, want)
	}
}

func selectorText(expression ast.Expr) string {
	selector, ok := expression.(*ast.SelectorExpr)
	if !ok {
		return ""
	}
	identifier, ok := selector.X.(*ast.Ident)
	if !ok {
		return ""
	}
	return identifier.Name + "." + selector.Sel.Name
}

func connectedStdlib() map[string]bool {
	stdlib := testStdlib()
	for _, dependency := range []string{
		"context",
		"crypto/sha256",
		"encoding/binary",
		"errors",
		"math",
		"reflect",
		"slices",
		"sort",
		"sync",
		"time",
	} {
		stdlib[dependency] = true
	}
	return stdlib
}
