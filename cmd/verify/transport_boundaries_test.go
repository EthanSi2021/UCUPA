package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCoreProtocolEntryBoundary(t *testing.T) {
	std := testStdlib()
	std["context"], std["sync"] = true, true
	for _, file := range []string{"service.go", "methods.go", "call.go", "federation.go", "service_test.go", "call_test.go"} {
		location := coreEntryRPCDirectory + "/" + file
		test := strings.HasSuffix(file, "_test.go")
		if checkSourceLocation(location) != "" {
			t.Fatalf("actual location rejected: %s", file)
		}
		for _, dep := range []string{"context", "ucupa/shared/values", "ucupa/ucupa-core/contract/federation", "ucupa/ucupa-core/bindings/go/protobuf", "ucupa/" + coreWireDirectory, "ucupa/" + coreSessionContextDirectory, "ucupa/" + coreAdmissionDirectory, "google.golang.org/protobuf/proto"} {
			if message := checkDependency(location, dep, "ucupa", test, std); message != "" {
				t.Errorf("%s -> %s: %s", file, dep, message)
			}
		}
		for _, dep := range []string{"sync", "ucupa/ucupa-core/instance", "ucupa/ucupa-core/internal/access/entry", "ucupa/ucupa-core/internal/commit", "ucupa/ucupa-core/internal/store", "ucupa/ucupa-core/sdk/go", "ucupa/ucupa-core/transport/grpc", "ucupa/" + coreMountRPCDirectory, "ucupa/" + coreMountConnectionsDirectory, "ucupa/composition", "ucupa/ucupa-realm", "go.etcd.io/bbolt"} {
			allowed := file == "service_test.go" && dep == "sync" || file == "call_test.go" && dep == "ucupa/ucupa-core/sdk/go"
			if message := checkDependency(location, dep, "ucupa", test, std); (message == "") != allowed {
				t.Errorf("fixture-limited dependency %s -> %s: %s", file, dep, message)
			}
		}
		allowed := file == "service.go" || file == "call_test.go"
		if message := checkDependency(location, "ucupa/"+coreConnectionsDirectory, "ucupa", test, std); (message == "") != allowed {
			t.Errorf("connector location %s: %s", file, message)
		}
		if message := checkDependency(location, "google.golang.org/grpc/metadata", "ucupa", test, std); (message == "") != (file == "call_test.go") {
			t.Errorf("actual session fixture %s: %s", file, message)
		}
	}
	for _, file := range []string{coreEntryRPCDirectory + "/extra.go", coreEntryRPCDirectory + "/extra/service.go", "ucupa-core/transport/grpc/methods.go", "ucupa-core/transport/grpc/federation_public.go"} {
		if checkSourceLocation(file) == "" {
			t.Errorf("unreviewed or retired source %s", file)
		}
	}
	for _, file := range []string{"ucupa-core/transport/grpc/server.go", "ucupa-core/transport/grpc/other.go", coreAdmissionDirectory + "/service.go", "ucupa-core/sdk/go/client.go", "composition/cluster.go", "acceptance/network_test.go", coreConnectionsDirectory + "/federation_connections.go"} {
		if message := checkDependency(file, "ucupa/"+coreEntryRPCDirectory, "ucupa", strings.HasSuffix(file, "_test.go"), std); (message == "") != (file == "ucupa-core/transport/grpc/server.go") {
			t.Errorf("entry consumer %s: %s", file, message)
		}
	}
}

func TestCoreProtocolEntryActualOwnerAndLimitedAdmission(t *testing.T) {
	for _, source := range []string{"type Service struct { sessions map[string]bool }", "type Service struct { *Server }", "func (s *Service) bad() { s.admission.BeginStop() }", "func (s *Service) bad() { alias := s.admission; alias.Wait() }", "func (s *Service) bad() any { return s.admission }"} {
		f, err := parser.ParseFile(token.NewFileSet(), "service.go", "package entry\n"+source, 0)
		if err != nil {
			t.Fatal(err)
		}
		if len(checkCoreEntryRPCDeclarations(coreEntryRPCDirectory+"/service.go", f, false)) == 0 {
			t.Errorf("accepted state or lifetime access: %s", source)
		}
	}
	old, err := parser.ParseFile(token.NewFileSet(), "server.go", "package coregrpc\nfunc (s *Server) ReadElement() {}", 0)
	if err != nil || len(checkCoreEntryRPCDeclarations("ucupa-core/transport/grpc/server.go", old, false)) == 0 {
		t.Fatal("old root RPC restored")
	}
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	root, err := repositoryRoot(cwd)
	if err != nil {
		t.Fatal(err)
	}
	for _, relative := range []string{"ucupa-core/transport/grpc", coreEntryRPCDirectory} {
		files, err := filepath.Glob(filepath.Join(root, filepath.FromSlash(relative), "*.go"))
		if err != nil || len(files) == 0 {
			t.Fatalf("missing actual source: %s %v", relative, err)
		}
		for _, file := range files {
			f, err := parser.ParseFile(token.NewFileSet(), file, nil, 0)
			if err != nil {
				t.Fatal(err)
			}
			for _, message := range checkCoreEntryRPCDeclarations(relative+"/"+filepath.Base(file), f, strings.HasSuffix(file, "_test.go")) {
				t.Errorf("%s: %s", file, message)
			}
		}
	}
	f, err := parser.ParseFile(token.NewFileSet(), filepath.Join(root, "ucupa-core/transport/grpc/server.go"), nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	registrations := 0
	ast.Inspect(f, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		function, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || function.Sel.Name != "RegisterCoreServiceServer" {
			return true
		}
		registrations++
		if len(call.Args) != 2 {
			t.Fatal("invalid CoreService registration")
		}
		constructor, ok := call.Args[1].(*ast.CallExpr)
		if !ok {
			t.Fatal("CoreService registered on an old host facade")
		}
		name, ok := constructor.Fun.(*ast.SelectorExpr)
		if !ok || name.Sel.Name != "New" {
			t.Fatal("CoreService missing actual entry constructor")
		}
		alias, ok := name.X.(*ast.Ident)
		if !ok || alias.Name != "entrytransport" || len(constructor.Args) != 3 {
			t.Fatal("CoreService not directly registered on its actual private entry")
		}
		return true
	})
	if registrations != 1 {
		t.Fatalf("CoreService registered %d times", registrations)
	}
}

func TestSharedCoreAdmissionExactBoundary(t *testing.T) {
	std := testStdlib()
	std["context"], std["sync"] = true, true
	files := []string{"service.go", "lifecycle.go", "entry.go", "interceptor.go", "session.go", "delegation.go", "delegation_issue.go", "identity.go", "identity_challenge.go", "identity_exchange.go", "mount_approval.go", "mount_peer.go", "transition_approval.go", "transition_approval_test.go", "federation_identity.go", "federation_read.go", "federation_read_at.go", "federation_observe.go", "delegation_test.go", "federation_identity_test.go", "federation_read_at_test.go", "federation_observe_test.go", "lifecycle_test.go", "entry_test.go"}
	for _, file := range files {
		location := coreAdmissionDirectory + "/" + file
		if message := checkSourceLocation(location); message != "" {
			t.Fatal(message)
		}
		test := strings.HasSuffix(file, "_test.go")
		for _, dep := range []string{"context", "sync", "ucupa/shared/values", "ucupa/ucupa-core/contract", "ucupa/ucupa-core/contract/federation", "ucupa/" + coreWireDirectory, "ucupa/" + coreSessionContextDirectory, "ucupa/" + coreRPCFailureDirectory, "google.golang.org/grpc", "google.golang.org/protobuf/proto"} {
			if message := checkDependency(location, dep, "ucupa", test, std); message != "" {
				t.Errorf("actual %s -> %s: %s", file, dep, message)
			}
		}
		for _, dep := range []string{"ucupa/ucupa-core/instance", "ucupa/ucupa-core/internal/commit", "ucupa/ucupa-core/internal/query", "ucupa/ucupa-core/internal/store", "ucupa/ucupa-core/transport/grpc", "ucupa/ucupa-core/sdk/go", "ucupa/ucupa-realm", "ucupa/composition", "go.etcd.io/bbolt", "ucupa/" + coreConnectionsDirectory, "ucupa/" + coreMountConnectionsDirectory, "ucupa/" + coreMountRPCDirectory} {
			if message := checkDependency(location, dep, "ucupa", test, std); message == "" {
				t.Errorf("forbidden %s -> %s", file, dep)
			}
		}
		message := checkDependency(location, "ucupa/internal/grpcbound", "ucupa", test, std)
		if (message == "") != (file == "service.go" || file == "lifecycle.go") {
			t.Errorf("listener ownership %s: %s", file, message)
		}
	}
	for _, file := range []string{coreAdmissionDirectory + "/extra.go", coreAdmissionDirectory + "/extra/test.go", "ucupa-core/transport/grpc/session.go", "ucupa-core/transport/grpc/federation_read_at_test.go"} {
		if checkSourceLocation(file) == "" {
			t.Errorf("unreviewed or retired location %s", file)
		}
	}
	if checkDependency(coreAdmissionDirectory+"/extra/service.go", "context", "ucupa", false, std) == "" {
		t.Fatal("unreviewed admission descendant accepted")
	}
	for _, file := range []string{"ucupa-core/transport/grpc/server.go", "ucupa-core/transport/grpc/other.go", "ucupa-core/sdk/go/client.go", "composition/cluster.go", "acceptance/network_test.go", coreSessionContextDirectory + "/context.go", coreWireDirectory + "/codec.go", coreMountConnectionsDirectory + "/mount_connections.go"} {
		message := checkDependency(file, "ucupa/"+coreAdmissionDirectory, "ucupa", strings.HasSuffix(file, "_test.go"), std)
		if (message == "") != (file == "ucupa-core/transport/grpc/server.go") {
			t.Errorf("private admission consumer %s: %s", file, message)
		}
	}
}

func TestSharedCoreAdmissionNoDuplicateStateOrRootRPC(t *testing.T) {
	for _, snippet := range []struct{ location, source string }{
		{"ucupa-core/transport/grpc/other.go", "package coregrpc\nfunc (s *Server) ReleaseSession() {}"},
		{"ucupa-core/transport/grpc/other.go", "package coregrpc\nfunc (s *Server) CreateTransitionApproval() {}"},
		{"ucupa-core/transport/grpc/other.go", "package coregrpc\ntype Server struct { sessions map[string]bool }"},
		{"ucupa-core/transport/grpc/other.go", "package coregrpc\ntype Server struct { listener int }"},
		{"ucupa-core/transport/grpc/other.go", "package coregrpc\ntype Server struct { transitionApprovals map[string]bool }"},
		{coreAdmissionDirectory + "/service.go", "package admission\ntype Service struct { Sessions map[string]bool }"},
		{coreAdmissionDirectory + "/service.go", "package admission\ntype Service struct { mirror map[string]bool }"},
		{coreAdmissionDirectory + "/service.go", "package admission\ntype Service struct { *Server }"},
		{coreAdmissionDirectory + "/entry.go", "package admission\ntype EntrySession struct { Key [32]byte }"},
		{coreAdmissionDirectory + "/entry.go", "package admission\ntype EntrySession struct { *Service }"},
		{coreAdmissionDirectory + "/entry.go", "package admission\ntype EntrySession struct { registry map[string]bool }"},
	} {
		f, err := parser.ParseFile(token.NewFileSet(), snippet.location, snippet.source, 0)
		if err != nil {
			t.Fatal(err)
		}
		if len(checkCoreAdmissionDeclarations(snippet.location, f, false)) == 0 {
			t.Errorf("accepted root forwarding or duplicate state: %s", snippet.source)
		}
	}
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	root, err := repositoryRoot(cwd)
	if err != nil {
		t.Fatal(err)
	}
	for _, relative := range []string{"ucupa-core/transport/grpc", coreAdmissionDirectory} {
		files, err := filepath.Glob(filepath.Join(root, filepath.FromSlash(relative), "*.go"))
		if err != nil || len(files) == 0 {
			t.Fatalf("missing actual source set %s: %v", relative, err)
		}
		for _, file := range files {
			f, err := parser.ParseFile(token.NewFileSet(), file, nil, 0)
			if err != nil {
				t.Fatal(err)
			}
			for _, message := range checkCoreAdmissionDeclarations(relative+"/"+filepath.Base(file), f, strings.HasSuffix(file, "_test.go")) {
				t.Errorf("%s: %s", file, message)
			}
		}
	}
	f, err := parser.ParseFile(token.NewFileSet(), filepath.Join(root, "ucupa-core/transport/grpc/server.go"), nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	registered := map[string]int{"RegisterSessionServiceServer": 0, "RegisterReadDelegationServiceServer": 0, "RegisterIdentityProofServiceServer": 0, "RegisterMountApprovalServiceServer": 0, "RegisterMountPeerServiceServer": 0, "RegisterFederationReadServiceServer": 0}
	ast.Inspect(f, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		selector, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		if _, expected := registered[selector.Sel.Name]; expected {
			registered[selector.Sel.Name]++
			if len(call.Args) != 2 {
				t.Fatal("wrong registration shape")
			}
			service, ok := call.Args[1].(*ast.SelectorExpr)
			if !ok || service.Sel.Name != "admission" {
				t.Fatal("admission RPCs must be directly registered on their actual service")
			}
			host, ok := service.X.(*ast.Ident)
			if !ok || host.Name != "s" {
				t.Fatal("admission RPCs do not share the host's one service")
			}
		}
		return true
	})
	for name, count := range registered {
		if count != 1 {
			t.Errorf("%s registered %d times", name, count)
		}
	}
}

func TestApplicationMountRPCExactBoundary(t *testing.T) {
	std := testStdlib()
	std["context"], std["sync"], std["sync/atomic"] = true, true, true
	for _, file := range []string{"service.go", "control.go", "contact.go", "fixture_test.go", "service_test.go", "command_test.go"} {
		path := coreMountRPCDirectory + "/" + file
		if message := checkSourceLocation(path); message != "" {
			t.Fatal(message)
		}
		isTest := strings.HasSuffix(file, "_test.go")
		for _, dep := range []string{"context", "ucupa/shared/values", "ucupa/ucupa-core/contract", "ucupa/ucupa-core/bindings/go/protobuf", "ucupa/" + coreWireDirectory, "ucupa/" + coreSessionContextDirectory, "ucupa/" + coreRPCFailureDirectory, "google.golang.org/protobuf/proto"} {
			if message := checkDependency(path, dep, "ucupa", isTest, std); message != "" {
				t.Errorf("actual %s -> %s: %s", path, dep, message)
			}
		}
		for _, dep := range []string{"sync", "sync/atomic", "ucupa/ucupa-core/transport/grpc", "ucupa/ucupa-core/instance", "ucupa/ucupa-core/internal/mount", "ucupa/ucupa-core/internal/commit", "ucupa/ucupa-core/internal/store", "ucupa/ucupa-core/sdk/go", "ucupa/ucupa-realm", "ucupa/composition", "go.etcd.io/bbolt", "ucupa/" + coreConnectionsDirectory, "ucupa/" + coreAttachmentRPCDirectory} {
			if message := checkDependency(path, dep, "ucupa", isTest, std); message == "" {
				t.Errorf("forbidden %s -> %s", path, dep)
			}
		}
		message := checkDependency(path, "ucupa/"+coreMountConnectionsDirectory, "ucupa", isTest, std)
		if (message == "") != (file == "service.go" || file == "fixture_test.go") {
			t.Errorf("outbound dependency %s: %s", path, message)
		}
	}
	for _, file := range []string{coreMountRPCDirectory + "/extra.go", coreMountRPCDirectory + "/extra/test.go", "ucupa-core/transport/grpc/mount_control.go", "ucupa-core/transport/grpc/mount_contact.go"} {
		if checkSourceLocation(file) == "" {
			t.Errorf("unreviewed location %s", file)
		}
	}
	if checkDependency(coreMountRPCDirectory+"/extra/service.go", "ucupa/composition", "ucupa", false, std) == "" {
		t.Fatal("unreviewed child fell through to generic assembly rules")
	}
	for _, file := range []string{"ucupa-core/transport/grpc/server.go", "ucupa-core/transport/grpc/other.go", "ucupa-core/sdk/go/client.go", "composition/cluster.go", "acceptance/network_test.go", coreSessionContextDirectory + "/context.go", coreWireDirectory + "/codec.go", coreMountConnectionsDirectory + "/mount_connections.go"} {
		message := checkDependency(file, "ucupa/"+coreMountRPCDirectory, "ucupa", strings.HasSuffix(file, "_test.go"), std)
		if (message == "") != (file == "ucupa-core/transport/grpc/server.go") {
			t.Errorf("private consumer %s: %s", file, message)
		}
	}
}

func TestApplicationMountRPCNoRootForwardingOrPrivateState(t *testing.T) {
	for _, snippet := range []struct{ path, source string }{
		{"ucupa-core/transport/grpc/renamed.go", "package coregrpc\nfunc (s *Server) ApplyMount() {}"},
		{"ucupa-core/transport/grpc/renamed.go", "package coregrpc\nfunc publicMountCommand() {}"},
		{coreMountRPCDirectory + "/service.go", "package mount\ntype Service struct { sessions map[string]bool }"},
		{coreMountRPCDirectory + "/service.go", "package mount\ntype Service struct { *Server }"},
	} {
		f, err := parser.ParseFile(token.NewFileSet(), snippet.path, snippet.source, 0)
		if err != nil {
			t.Fatal(err)
		}
		if len(checkCoreMountRPCDeclarations(snippet.path, f, false)) == 0 {
			t.Errorf("accepted old root or shared state: %s", snippet.source)
		}
	}
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	root, err := repositoryRoot(cwd)
	if err != nil {
		t.Fatal(err)
	}
	for _, relative := range []string{"ucupa-core/transport/grpc", coreMountRPCDirectory} {
		files, err := filepath.Glob(filepath.Join(root, filepath.FromSlash(relative), "*.go"))
		if err != nil || len(files) == 0 {
			t.Fatalf("missing actual source set %s: %v", relative, err)
		}
		for _, file := range files {
			f, err := parser.ParseFile(token.NewFileSet(), file, nil, 0)
			if err != nil {
				t.Fatal(err)
			}
			for _, message := range checkCoreMountRPCDeclarations(relative+"/"+filepath.Base(file), f, strings.HasSuffix(file, "_test.go")) {
				t.Errorf("%s: %s", file, message)
			}
		}
	}
	data, err := os.ReadFile(filepath.Join(root, "ucupa-core/transport/grpc/server.go"))
	if err != nil {
		t.Fatal(err)
	}
	f, err := parser.ParseFile(token.NewFileSet(), "server.go", data, 0)
	if err != nil {
		t.Fatal(err)
	}
	registered := map[string]int{"RegisterMountControlServiceServer": 0, "RegisterMountContactServiceServer": 0}
	ast.Inspect(f, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		selector, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		if _, expected := registered[selector.Sel.Name]; expected {
			registered[selector.Sel.Name]++
			if len(call.Args) != 2 {
				t.Fatal("wrong registration shape")
			}
			service, ok := call.Args[1].(*ast.Ident)
			if !ok || service.Name != "mount" {
				t.Fatal("application Mount RPC group must be registered directly, not on Server")
			}
		}
		return true
	})
	for name, count := range registered {
		if count != 1 {
			t.Errorf("%s registered %d times", name, count)
		}
	}
}

func TestCoreInboundTransportKeepsExactResponsibilities(t *testing.T) {
	stdlib := testStdlib()
	stdlib["context"] = true
	for _, dir := range []string{coreAttachmentRPCDirectory, coreFacetRPCDirectory, coreSessionContextDirectory, coreRPCFailureDirectory} {
		if got := checkSourceLocation(dir + "/service.go"); got != "" {
			t.Fatal(got)
		}
		if got := checkSourceLocation(dir + "/extra/service.go"); got == "" {
			t.Fatal("unreviewed descendant accepted")
		}
		for _, test := range []bool{false, true} {
			for _, dep := range []string{"context", "ucupa/shared/values"} {
				if got := checkDependency(dir+"/service.go", dep, "ucupa", test, stdlib); got != "" {
					t.Errorf("%s -> %s: %s", dir, dep, got)
				}
			}
			for _, dep := range []string{"ucupa/ucupa-core", "ucupa/ucupa-core/instance", "ucupa/ucupa-core/internal/commit", "ucupa/ucupa-core/internal/token", "ucupa/ucupa-core/internal/element/fact", "ucupa/ucupa-core/transport/grpc", "ucupa/ucupa-core/sdk/go", "ucupa/" + coreConnectionsDirectory, "ucupa/" + coreMountConnectionsDirectory, "ucupa/ucupa-facet/contract", "ucupa/ucupa-realm", "ucupa/composition", "go.etcd.io/bbolt"} {
				if got := checkDependency(dir+"/service.go", dep, "ucupa", test, stdlib); got == "" {
					t.Errorf("forbidden %s -> %s accepted (test=%v)", dir, dep, test)
				}
			}
		}
		if got := checkDependency(dir+"/service_test.go", "ucupa/"+dir, "ucupa", true, stdlib); got != "" {
			t.Fatal(got)
		}
		for _, source := range []string{"ucupa-core/core.go", "ucupa-core/contract/types.go", "ucupa-core/sdk/go/client.go", "ucupa-facet/transport/grpc/server.go", "composition/cluster.go", "cmd/ucupa-core/main.go", "acceptance/network_test.go", coreWireDirectory + "/codec.go", coreConnectionsDirectory + "/connections.go"} {
			if got := checkDependency(source, "ucupa/"+dir, "ucupa", strings.HasSuffix(source, "_test.go"), stdlib); got == "" {
				t.Errorf("unreviewed inbound consumer %s -> %s accepted", source, dir)
			}
		}
		if got := checkDependency("ucupa-core/transport/grpc/server.go", "ucupa/"+dir, "ucupa", false, stdlib); got != "" {
			t.Fatal(got)
		}
	}
	for _, tc := range []struct {
		source, target string
		allowed        bool
	}{
		{coreAttachmentRPCDirectory, "ucupa/ucupa-core/contract", true},
		{coreAttachmentRPCDirectory, "ucupa/ucupa-core/bindings/go/protobuf", true},
		{coreAttachmentRPCDirectory, "ucupa/" + coreWireDirectory, true},
		{coreAttachmentRPCDirectory, "ucupa/" + coreSessionContextDirectory, true},
		{coreAttachmentRPCDirectory, "ucupa/" + coreRPCFailureDirectory, true},
		{coreAttachmentRPCDirectory, "google.golang.org/protobuf/proto", true},
		{coreSessionContextDirectory, "ucupa/" + coreAttachmentRPCDirectory, false},
		{coreRPCFailureDirectory, "ucupa/" + coreAttachmentRPCDirectory, false},
		{coreRPCFailureDirectory, "ucupa/" + coreSessionContextDirectory, false},
		{coreRPCFailureDirectory, "ucupa/shared/bindings/go/protobuf", true},
		{coreRPCFailureDirectory, "ucupa/ucupa-core/contract", true},
		{coreFacetRPCDirectory, "ucupa/ucupa-core/contract", true},
		{coreFacetRPCDirectory, "ucupa/ucupa-core/bindings/go/protobuf", true},
		{coreFacetRPCDirectory, "ucupa/" + coreWireDirectory, true},
		{coreFacetRPCDirectory, "ucupa/" + coreSessionContextDirectory, true},
		{coreFacetRPCDirectory, "ucupa/" + coreRPCFailureDirectory, true},
		{coreFacetRPCDirectory, "google.golang.org/protobuf/proto", true},
		{coreFacetRPCDirectory, "ucupa/" + coreAttachmentRPCDirectory, false},
		{coreAttachmentRPCDirectory, "ucupa/" + coreFacetRPCDirectory, false},
		{coreRPCFailureDirectory, "ucupa/" + coreFacetRPCDirectory, false},
		{coreSessionContextDirectory, "ucupa/" + coreFacetRPCDirectory, false},
		{coreSessionContextDirectory, "google.golang.org/grpc/status", false},
		{coreRPCFailureDirectory, "google.golang.org/grpc/status", true},
		{coreSessionContextDirectory, "ucupa/ucupa-core/contract", false},
		{coreSessionContextDirectory, "ucupa/" + coreRPCFailureDirectory, false},
	} {
		if got := checkDependency(tc.source+"/service.go", tc.target, "ucupa", false, stdlib); (got == "") != tc.allowed {
			t.Errorf("%s -> %s: expected allowed=%v, got %q", tc.source, tc.target, tc.allowed, got)
		}
	}
}

func TestReviewedCoreTransportDependencies(t *testing.T) {
	stdlib := testStdlib()
	stdlib["context"] = true
	for _, dir := range []string{coreWireDirectory, coreConnectionsDirectory, coreMountConnectionsDirectory} {
		if got := checkSourceLocation(dir + "/messages.go"); got != "" {
			t.Fatal(got)
		}
		if checkSourceLocation(dir+"/extra/messages.go") == "" {
			t.Fatal("approved an unreviewed descendant")
		}
		for _, dep := range []string{"context", "ucupa/shared/values", "ucupa/shared/bindings/go/protobuf", "ucupa/ucupa-core/bindings/go/protobuf", "ucupa/ucupa-core/contract", "google.golang.org/protobuf/proto"} {
			if got := checkDependency(dir+"/messages.go", dep, "ucupa", false, stdlib); got != "" {
				t.Errorf("%s -> %s rejected: %s", dir, dep, got)
			}
		}
		for _, dep := range []string{"ucupa/ucupa-core", "ucupa/ucupa-core/transport/grpc", "ucupa/ucupa-core/internal/geometry", "ucupa/ucupa-core/internal/token", "ucupa/ucupa-core/internal/store", "ucupa/ucupa-core/internal/failure", "ucupa/ucupa-core/instance", "ucupa/ucupa-facet/contract", "ucupa/ucupa-realm", "ucupa/composition", "go.etcd.io/bbolt", "ucupa/internal/protovalue"} {
			for _, test := range []bool{false, true} {
				if got := checkDependency(dir+"/messages.go", dep, "ucupa", test, stdlib); got == "" {
					t.Errorf("forbidden %s -> %s accepted (test=%v)", dir, dep, test)
				}
			}
		}
	}
	for _, tc := range []struct {
		source, target string
		allowed        bool
	}{
		{coreWireDirectory + "/codec_test.go", "ucupa/" + coreWireDirectory, true},
		{coreConnectionsDirectory + "/connections_test.go", "ucupa/" + coreConnectionsDirectory, true},
		{coreWireDirectory + "/codec.go", "ucupa/" + coreWireDirectory, false},
		{coreConnectionsDirectory + "/connections.go", "ucupa/" + coreConnectionsDirectory, false},
		{coreConnectionsDirectory + "/connections.go", "ucupa/ucupa-core/sdk/go", true},
		{coreConnectionsDirectory + "/connections.go", "ucupa/" + coreWireDirectory, true},
		{coreConnectionsDirectory + "/connections.go", "google.golang.org/grpc/status", true},
		{coreWireDirectory + "/codec.go", "google.golang.org/grpc/status", false},
		{coreWireDirectory + "/codec.go", "ucupa/" + coreConnectionsDirectory, false},
		{coreWireDirectory + "/codec.go", "ucupa/ucupa-core/sdk/go", false},
		{"ucupa-core/transport/grpc/server.go", "ucupa/" + coreWireDirectory, true},
		{"ucupa-core/transport/grpc/server.go", "ucupa/" + coreConnectionsDirectory, true},
		{"ucupa-core/core.go", "ucupa/" + coreWireDirectory, false},
		{"ucupa-core/core.go", "ucupa/" + coreConnectionsDirectory, false},
		{coreMountConnectionsDirectory + "/connections_test.go", "ucupa/" + coreMountConnectionsDirectory, true},
		{coreMountConnectionsDirectory + "/connections.go", "ucupa/" + coreMountConnectionsDirectory, false},
		{coreMountConnectionsDirectory + "/connections.go", "ucupa/ucupa-core/sdk/go", true},
		{coreMountConnectionsDirectory + "/connections.go", "ucupa/" + coreWireDirectory, true},
		{coreMountConnectionsDirectory + "/connections.go", "google.golang.org/grpc/status", true},
		{coreMountConnectionsDirectory + "/connections.go", "ucupa/" + coreConnectionsDirectory, false},
		{coreConnectionsDirectory + "/connections.go", "ucupa/" + coreMountConnectionsDirectory, false},
		{coreWireDirectory + "/codec.go", "ucupa/" + coreMountConnectionsDirectory, false},
		{"ucupa-core/core.go", "ucupa/" + coreMountConnectionsDirectory, false},
		{"ucupa-core/sdk/go/client.go", "ucupa/" + coreMountConnectionsDirectory, false},
		{"ucupa-core/transport/grpc/server.go", "ucupa/" + coreMountConnectionsDirectory, true},
		{"cmd/ucupa-core/main.go", "ucupa/" + coreMountConnectionsDirectory, true},
		{"acceptance/connection_test.go", "ucupa/" + coreMountConnectionsDirectory, true},
		{"ucupa-core/sdk/go/client.go", "ucupa/" + coreWireDirectory, false},
		{"ucupa-core/sdk/go/client.go", "ucupa/" + coreConnectionsDirectory, false},
		{"ucupa-facet/transport/grpc/server.go", "ucupa/" + coreWireDirectory, false},
		{"composition/connection.go", "ucupa/" + coreWireDirectory, false},
		{"cmd/ucupa-core/main.go", "ucupa/" + coreWireDirectory, false},
		{"acceptance/wire_test.go", "ucupa/" + coreWireDirectory, false},
		{"cmd/ucupa-core/main.go", "ucupa/" + coreConnectionsDirectory, true},
		{"acceptance/connection_test.go", "ucupa/" + coreConnectionsDirectory, true},
	} {
		got := checkDependency(tc.source, tc.target, "ucupa", strings.HasSuffix(tc.source, "_test.go"), stdlib)
		if (got == "") != tc.allowed {
			t.Errorf("%s -> %s: allowed=%v got=%q", tc.source, tc.target, tc.allowed, got)
		}
	}
}
