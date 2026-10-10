package main

import (
	"go/ast"
	"go/token"
	"path"
	"strings"
)

const coreWireDirectory = "ucupa-core/transport/grpc/internal/wire"
const coreConnectionsDirectory = "ucupa-core/transport/grpc/federationconn"
const coreMountConnectionsDirectory = "ucupa-core/transport/grpc/mountconn"
const coreAttachmentRPCDirectory = "ucupa-core/transport/grpc/internal/attachment"
const coreFacetRPCDirectory = "ucupa-core/transport/grpc/internal/facet"
const coreMountRPCDirectory = "ucupa-core/transport/grpc/internal/mount"
const coreSessionContextDirectory = "ucupa-core/transport/grpc/internal/session"
const coreRPCFailureDirectory = "ucupa-core/transport/grpc/internal/rpcfailure"
const coreAdmissionDirectory = "ucupa-core/transport/grpc/internal/admission"
const coreEntryRPCDirectory = "ucupa-core/transport/grpc/internal/entry"

func reviewedCoreTransportPackage(directory string) bool {
	return directory == coreConnectionsDirectory || directory == coreWireDirectory || directory == coreMountConnectionsDirectory || reviewedCoreInboundPackage(directory)
}

func reviewedCoreFederationWireSourceFile(file string) bool {
	switch file {
	case "federation_wire.go", "federation_projection_wire.go", "federation_page_wire.go":
		return true
	default:
		return false
	}
}

func reviewedCoreFederationConnectionSourceFile(file string) bool {
	switch file {
	case "federation_connections.go", "federation_observe_remote.go", "federation_read_at_remote.go", "federation_remote.go":
		return true
	default:
		return false
	}
}

func reviewedCoreInboundPackage(directory string) bool {
	return directory == coreAttachmentRPCDirectory || directory == coreFacetRPCDirectory || directory == coreSessionContextDirectory || directory == coreRPCFailureDirectory
}

// Communication-private packages are not owner-business internal packages.
// These exact edges do not approve arbitrary transport or SDK descendants.
func checkCoreTransportDependency(file, imported, module string, test bool, stdlib map[string]bool) (bool, string) {
	dir := path.Dir(file)
	if dir == coreEntryRPCDirectory || strings.HasPrefix(dir, coreEntryRPCDirectory+"/") {
		if dir != coreEntryRPCDirectory || !reviewedCoreEntryRPCFile(path.Base(file)) {
			return true, "Core protocol entry permits only the reviewed complete RPC/JSON/disclosure adapter"
		}
		if test && path.Base(file) == "service_test.go" && imported == "sync" {
			return true, "" // The finite fixture protects its barrier call log, not production state.
		}
		if test && path.Base(file) == "call_test.go" && (imported == "google.golang.org/grpc/metadata" || imported == module+"/ucupa-core/sdk/go") {
			return true, "" // Actual session and lazy outbound config, never a simulated admission owner.
		}
		if stdlib[imported] && imported != "sync" && imported != "sync/atomic" || test && imported == module+"/"+dir {
			return true, ""
		}
		switch imported {
		case module + "/shared/values", module + "/shared/bindings/go/protobuf", module + "/ucupa-core/bindings/go/protobuf",
			module + "/ucupa-core/contract/federation", module + "/" + coreAdmissionDirectory, module + "/" + coreWireDirectory, module + "/" + coreSessionContextDirectory,
			"google.golang.org/grpc/codes", "google.golang.org/grpc/status", "google.golang.org/protobuf/proto", "google.golang.org/protobuf/reflect/protoreflect":
			return true, ""
		case module + "/" + coreConnectionsDirectory:
			if path.Base(file) == "service.go" || test && path.Base(file) == "call_test.go" {
				return true, ""
			}
		}
		return true, "entry adapts the public protocol using one admission owner; no business, Store, SDK, sibling RPC state or network host"
	}
	if imported == module+"/"+coreEntryRPCDirectory {
		if file == "ucupa-core/transport/grpc/server.go" {
			return true, ""
		}
		return true, "only the Core network host constructs and directly registers the private protocol entry"
	}
	if dir == coreAdmissionDirectory || strings.HasPrefix(dir, coreAdmissionDirectory+"/") {
		if dir != coreAdmissionDirectory || !reviewedCoreAdmissionFile(path.Base(file)) {
			return true, "Core admission is limited to the reviewed complete session/grant/challenge/approval lifetime"
		}
		if stdlib[imported] || test && imported == module+"/"+coreAdmissionDirectory {
			return true, ""
		}
		switch imported {
		case module + "/shared/values", module + "/shared/bindings/go/protobuf", module + "/ucupa-core/bindings/go/protobuf",
			module + "/ucupa-core/contract", module + "/ucupa-core/contract/federation", module + "/" + coreSessionContextDirectory,
			module + "/" + coreWireDirectory, module + "/" + coreRPCFailureDirectory,
			"google.golang.org/grpc", "google.golang.org/grpc/codes", "google.golang.org/grpc/status",
			"google.golang.org/grpc/credentials", "google.golang.org/grpc/metadata", "google.golang.org/grpc/peer",
			"google.golang.org/protobuf/proto", "google.golang.org/protobuf/reflect/protoreflect":
			return true, ""
		case module + "/internal/grpcbound":
			if path.Base(file) == "service.go" || path.Base(file) == "lifecycle.go" {
				return true, ""
			}
		}
		return true, "admission owns inbound credentials and finite lifetimes, never business internals, Store, SDK, outbound connections or Server"
	}
	if imported == module+"/"+coreAdmissionDirectory {
		if file == "ucupa-core/transport/grpc/server.go" {
			return true, ""
		}
		return true, "only Core network host constructs and directly registers the shared admission service"
	}
	if dir == coreMountRPCDirectory || strings.HasPrefix(dir, coreMountRPCDirectory+"/") {
		if dir != coreMountRPCDirectory || !reviewedCoreMountRPCFile(path.Base(file)) {
			return true, "Core application Mount RPCs are limited to reviewed control/contact/service files and their exact tests"
		}
		if stdlib[imported] && imported != "sync" && imported != "sync/atomic" || test && imported == module+"/"+coreMountRPCDirectory {
			return true, ""
		}
		switch imported {
		case module + "/shared/values", module + "/shared/bindings/go/protobuf", module + "/ucupa-core/bindings/go/protobuf", module + "/ucupa-core/contract",
			module + "/" + coreSessionContextDirectory, module + "/" + coreRPCFailureDirectory, module + "/" + coreWireDirectory,
			"google.golang.org/grpc/codes", "google.golang.org/grpc/status", "google.golang.org/protobuf/proto":
			return true, ""
		case module + "/" + coreMountConnectionsDirectory:
			if path.Base(file) == "service.go" || test && path.Base(file) == "fixture_test.go" {
				return true, ""
			}
		}
		return true, "application Mount RPCs consume fixed ports and context only; no shared admission state, SDK, business internals or sibling workflow"
	}
	if imported == module+"/"+coreMountRPCDirectory {
		if file == "ucupa-core/transport/grpc/server.go" {
			return true, ""
		}
		return true, "only Core transport service assembly directly registers the private application Mount RPC group"
	}
	if reviewedCoreInboundPackage(dir) {
		if stdlib[imported] || imported == module+"/shared/values" || test && imported == module+"/"+dir {
			return true, ""
		}
		if dir != coreSessionContextDirectory {
			switch imported {
			case module + "/shared/bindings/go/protobuf", "google.golang.org/grpc/codes", "google.golang.org/grpc/status":
				return true, ""
			}
		}
		if dir == coreRPCFailureDirectory && imported == module+"/ucupa-core/contract" {
			return true, ""
		}
		if dir == coreAttachmentRPCDirectory || dir == coreFacetRPCDirectory {
			switch imported {
			case module + "/ucupa-core/bindings/go/protobuf", module + "/ucupa-core/contract", "google.golang.org/protobuf/proto",
				module + "/" + coreSessionContextDirectory, module + "/" + coreRPCFailureDirectory, module + "/" + coreWireDirectory:
				return true, ""
			}
			if dir == coreFacetRPCDirectory && imported == module+"/ucupa-core/contract/replacement" {
				return true, ""
			}
		}
		return true, "Core inbound RPC groups use exact wire, context and failure boundaries; no owner state, registry, SDK, outbound connection or sibling workflow dependencies"
	}
	for _, private := range []string{coreAttachmentRPCDirectory, coreFacetRPCDirectory, coreSessionContextDirectory, coreRPCFailureDirectory} {
		if imported == module+"/"+private {
			if dir == "ucupa-core/transport/grpc" {
				return true, ""
			}
			return true, "Core inbound transport internals belong to the server and reviewed RPC consumers, not kernels, clients, peers or assembly"
		}
	}
	if reviewedCoreTransportPackage(dir) {
		if dir == coreWireDirectory && path.Base(file) == "representation.go" &&
			(imported == module+"/internal/protovalue" || imported == "google.golang.org/protobuf/reflect/protoreflect") {
			return true, ""
		}
		if dir == coreWireDirectory && test && path.Base(file) == "representation_test.go" && imported == "google.golang.org/protobuf/encoding/protowire" {
			return true, ""
		}
		if stdlib[imported] || test && imported == module+"/"+dir {
			return true, ""
		}
		if imported == module+"/ucupa-core/contract/federation" {
			if dir == coreWireDirectory && reviewedCoreFederationWireSourceFile(path.Base(file)) ||
				dir == coreConnectionsDirectory && reviewedCoreFederationConnectionSourceFile(path.Base(file)) {
				return true, ""
			}
			return true, "Core Federation transport values are registered only for the exact Federation wire and outbound-connection files"
		}
		switch imported {
		case module + "/shared/values", module + "/shared/bindings/go/protobuf", module + "/ucupa-core/bindings/go/protobuf", module + "/ucupa-core/contract", "google.golang.org/protobuf/proto":
			return true, ""
		}
		if dir == coreConnectionsDirectory || dir == coreMountConnectionsDirectory {
			switch imported {
			case module + "/ucupa-core/sdk/go", module + "/" + coreWireDirectory, "google.golang.org/grpc/codes", "google.golang.org/grpc/status":
				return true, ""
			}
		}
		return true, "Core transport codecs and outbound connections must use their exact public/wire dependencies, never business internals, server state or unreviewed packages"
	}
	if imported == module+"/"+coreWireDirectory {
		if dir == "ucupa-core/transport/grpc" {
			return true, ""
		}
		return true, "Core wire conversion is private to its server and reviewed outbound transport, not kernels, SDKs, peers or assembly"
	}
	return false, ""
}

func reviewedCoreEntryRPCFile(file string) bool {
	return contains([]string{"service.go", "methods.go", "call.go", "federation.go", "service_test.go", "call_test.go"}, file)
}

func checkCoreEntryRPCDeclarations(file string, source *ast.File, test bool) []string {
	root := path.Dir(file) == "ucupa-core/transport/grpc"
	if test || !root && path.Dir(file) != coreEntryRPCDirectory {
		return nil
	}
	var issues []string
	approvedGateUses := make(map[token.Pos]bool)
	ast.Inspect(source, func(n ast.Node) bool {
		if function, ok := n.(*ast.FuncDecl); ok && root && contains([]string{"Enter", "Close", "ReadElement", "ListDirect", "QueryRegion", "BeginObserve", "PollChanges", "ReleaseObserve", "CreateAnchor", "CreateEntity", "UpdateAnchor", "UpdateEntity", "DeleteAnchor", "DeleteEntity", "RecoverOutcome", "call", "publicOwnerCall"}, function.Name.Name) {
			issues = append(issues, "ordinary Core RPCs and JSON/disclosure handling belong to the actual entry, not root forwarding")
		}
		if call, ok := n.(*ast.CallExpr); ok && !root {
			if method, ok := call.Fun.(*ast.SelectorExpr); ok {
				if gate, ok := method.X.(*ast.SelectorExpr); ok && gate.Sel.Name == "admission" && contains([]string{"BindEntrySession", "CheckEntrySession", "CheckDelegatedResult"}, method.Sel.Name) {
					if receiver, ok := gate.X.(*ast.Ident); ok && receiver.Name == "s" {
						approvedGateUses[gate.Pos()] = true
					}
				}
			}
		}
		if typ, ok := n.(*ast.TypeSpec); ok && !root && typ.Name.Name == "Service" {
			if structure, ok := typ.Type.(*ast.StructType); ok {
				for _, field := range structure.Fields.List {
					if len(field.Names) == 0 {
						selector, ok := field.Type.(*ast.SelectorExpr)
						if !ok || selector.Sel.Name != "UnimplementedCoreServiceServer" {
							issues = append(issues, "entry embeds only its generated CoreService declaration")
						}
					}
					for _, name := range field.Names {
						if !contains([]string{"owner", "admission", "config"}, name.Name) {
							issues = append(issues, "entry contains fixed dependencies only, no session or fact state")
						}
					}
				}
			}
		}
		return true
	})
	if !root {
		ast.Inspect(source, func(n ast.Node) bool {
			if selector, ok := n.(*ast.SelectorExpr); ok && selector.Sel.Name == "admission" {
				if receiver, ok := selector.X.(*ast.Ident); ok && receiver.Name == "s" && !approvedGateUses[selector.Pos()] {
					issues = append(issues, "entry uses only three bounded admission checks, without aliasing or changing its lifecycle")
				}
			}
			return true
		})
	}
	return issues
}

func reviewedCoreAdmissionFile(file string) bool {
	switch file {
	case "service.go", "lifecycle.go", "entry.go", "interceptor.go", "session.go",
		"entry_context.go", "entry_context_test.go",
		"transition_approval.go", "transition_approval_test.go",
		"transition_accept.go", "transition_accept_test.go",
		"transition_peer.go", "transition_peer_test.go",
		"carrier_approval.go",
		"delegation.go", "delegation_issue.go", "identity.go", "identity_challenge.go", "identity_exchange.go",
		"mount_approval.go", "mount_peer.go", "federation_identity.go", "federation_read.go", "federation_read_at.go", "federation_observe.go",
		"delegation_test.go", "federation_identity_test.go", "federation_read_at_test.go", "federation_observe_test.go", "lifecycle_test.go", "entry_test.go":
		return true
	}
	return false
}

func checkCoreAdmissionDeclarations(file string, source *ast.File, test bool) []string {
	if test || path.Dir(file) != "ucupa-core/transport/grpc" && path.Dir(file) != coreAdmissionDirectory {
		return nil
	}
	root := path.Dir(file) == "ucupa-core/transport/grpc"
	var issues []string
	for _, declaration := range source.Decls {
		if function, ok := declaration.(*ast.FuncDecl); ok && root {
			switch function.Name.Name {
			case "EstablishSession", "ReleaseSession", "CreateReadDelegation", "RevokeReadDelegation", "BeginIdentityChallenge", "ExchangeIdentity",
				"IssueIdentityProof", "CheckIdentityProof", "CreateMountApproval", "CheckMountApproval", "RevokeMountApproval",
				"CreateTransitionApproval", "CheckTransitionApproval", "RevokeTransitionApproval",
				"PrepareMount", "FinishMount", "ReadMountDecision", "BeginFederationReadChallenge", "ExchangeFederationReadIdentity",
				"BeginFederationReadAtChallenge", "ProjectFederationReadPart", "CheckFederationReadPart", "ReleaseFederationReadPart", "ObserveFederationReadPart":
				issues = append(issues, "shared admission RPC belongs to its actual private owner, not a root forwarding method")
			}
		}
		group, ok := declaration.(*ast.GenDecl)
		if !ok {
			continue
		}
		for _, spec := range group.Specs {
			typ, ok := spec.(*ast.TypeSpec)
			if !ok {
				continue
			}
			structure, ok := typ.Type.(*ast.StructType)
			if !ok {
				continue
			}
			for _, field := range structure.Fields.List {
				if !root && typ.Name.Name == "Service" && len(field.Names) == 0 {
					selector, ok := field.Type.(*ast.SelectorExpr)
					if !ok || !contains([]string{"UnimplementedSessionServiceServer", "UnimplementedReadDelegationServiceServer", "UnimplementedIdentityProofServiceServer", "UnimplementedMountApprovalServiceServer", "UnimplementedTransitionApprovalServiceServer", "UnimplementedCarrierApprovalServiceServer", "UnimplementedMountPeerServiceServer", "UnimplementedFederationReadServiceServer"}, selector.Sel.Name) {
						issues = append(issues, "admission embeds only its eight generated RPC declarations")
					}
				}
				if !root && typ.Name.Name == "EntrySession" && len(field.Names) == 0 {
					issues = append(issues, "entry witness cannot embed another mutable owner")
				}
				for _, name := range field.Names {
					if root && typ.Name.Name == "Server" {
						switch name.Name {
						case "sessions", "grants", "pending", "challenges", "approvals", "transitionApprovals", "stopped", "active", "running", "mu", "listener":
							issues = append(issues, "Server must not retain a second admission or listener transition owner")
						}
					}
					if !root && (typ.Name.Name == "Service" || typ.Name.Name == "EntrySession") && name.IsExported() {
						issues = append(issues, "admission state and entry witness fields stay private; no exported tables, locks or mutable handles")
					}
					if !root && typ.Name.Name == "Service" && !contains([]string{"owner", "auth", "config", "mu", "sessions", "stopped", "active", "running", "listener", "grants", "pending", "challenges", "approvals", "transitionApprovals", "carrierApprovals"}, name.Name) {
						issues = append(issues, "admission owns only the original finite registries and shared lifetime, not new state")
					}
					if !root && typ.Name.Name == "EntrySession" && name.Name != "key" && name.Name != "entry" {
						issues = append(issues, "entry witness contains only the original session key and value")
					}
				}
			}
		}
	}
	return issues
}

func reviewedCoreMountRPCFile(file string) bool {
	switch file {
	case "service.go", "control.go", "contact.go", "fixture_test.go", "service_test.go", "command_test.go":
		return true
	}
	return false
}

// Moving the RPC group must remove its old root entry, not add another same-
// named forwarding layer. Its implementation owns no session or approval state.
func checkCoreMountRPCDeclarations(file string, source *ast.File, test bool) []string {
	if test {
		return nil
	}
	var issues []string
	for _, declaration := range source.Decls {
		if path.Dir(file) == "ucupa-core/transport/grpc" {
			if function, ok := declaration.(*ast.FuncDecl); ok {
				switch function.Name.Name {
				case "InspectMount", "DescribeMount", "ApplyMount", "InspectMountContact", "RepairMountContact", "mountController", "publicMountCommand", "publicMountContactWire":
					issues = append(issues, "application Mount RPCs and helpers belong to the actual private service, not a root forwarding layer")
				}
			}
		}
		if path.Dir(file) != coreMountRPCDirectory {
			continue
		}
		group, ok := declaration.(*ast.GenDecl)
		if !ok {
			continue
		}
		for _, spec := range group.Specs {
			typ, ok := spec.(*ast.TypeSpec)
			if !ok || typ.Name.Name != "Service" {
				continue
			}
			structure, ok := typ.Type.(*ast.StructType)
			if !ok {
				issues = append(issues, "Mount Service must own only its fixed actual dependencies")
				continue
			}
			for _, field := range structure.Fields.List {
				if len(field.Names) == 0 {
					selector, ok := field.Type.(*ast.SelectorExpr)
					if !ok || selector.Sel.Name != "UnimplementedMountControlServiceServer" && selector.Sel.Name != "UnimplementedMountContactServiceServer" {
						issues = append(issues, "Mount Service may embed only its two generated RPC declarations")
					}
					continue
				}
				for _, name := range field.Names {
					if name.Name != "owner" && name.Name != "expected" && name.Name != "connections" {
						issues = append(issues, "Mount Service does not own shared session, approval, lifecycle or fact state")
					}
				}
			}
		}
	}
	return issues
}
