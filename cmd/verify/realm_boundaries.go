package main

import (
	"path"
	"strings"
)

const realmTransport = "ucupa-realm/transport/grpc"

// Each Realm directory is a real dependency boundary, not a new wildcard
// exception to the owner rules. The transport consumes ports, never policy.
func checkRealmDependency(file, imported, module string, test bool, stdlib map[string]bool) (bool, string) {
	if imported == module+"/ucupa-core" || imported == module+"/ucupa-facet" || imported == module+"/ucupa-realm" {
		return true, "owner module root has no Go entry; use instance or contract, never an old-path alias"
	}
	dir := path.Dir(file)
	dep := strings.TrimPrefix(imported, module+"/")
	if !strings.HasPrefix(dir, "ucupa-realm/") {
		if strings.HasPrefix(dep, "ucupa-realm/internal/") || strings.HasPrefix(dep, realmTransport+"/internal/") {
			return true, "Realm implementation is private; consumers use instance, contract or network protocol"
		}
		return false, ""
	}
	if dir == "ucupa-realm/bindings/go/protobuf" || dir == "ucupa-realm/sdk/go" {
		return false, "" // Keep the existing generated/runtime and SDK rules.
	}
	allowed := func(names ...string) bool {
		for _, name := range names {
			if dep == name {
				return true
			}
		}
		return false
	}
	standard := stdlib[imported]
	protocol := strings.HasPrefix(imported, "google.golang.org/protobuf/") || imported == "google.golang.org/grpc/codes" || imported == "google.golang.org/grpc/status"
	ok := false
	switch dir {
	case "ucupa-realm/contract":
		ok = allowed("context", "crypto/ed25519", "errors", "time", "shared/values", "ucupa-realm/internal/token")
	case "ucupa-realm/internal/token":
		ok = test && standard
	case "ucupa-realm/internal/policy":
		ok = test && standard || allowed("bytes", "context", "crypto/hmac", "crypto/rand", "crypto/sha256", "encoding/binary", "errors", "fmt", "math", "os", "sort", "strings", "sync", "time", "shared/values", "ucupa-realm/contract", "ucupa-realm/internal/token", "go.etcd.io/bbolt")
		if test && path.Base(file) == "durability_test.go" {
			ok = ok || allowed("ucupa-core/instance", "ucupa-core/contract", "ucupa-facet/instance", "ucupa-facet/contract")
		}
	case "ucupa-realm/internal/identity":
		ok = test && standard || allowed("bytes", "context", "crypto/ed25519", "crypto/rand", "crypto/sha256", "encoding/json", "errors", "sync", "time", "shared/values", "ucupa-realm/contract")
		ok = ok || test && path.Base(file) == "identity_assertion_test.go" && dep == "ucupa-realm/internal/policy"
	case "ucupa-realm/instance":
		ok = allowed("crypto/ed25519", "time", "shared/values", "ucupa-realm/contract", "ucupa-realm/internal/policy", "ucupa-realm/internal/identity")
	case "ucupa-realm/tests/instance":
		ok = test && (standard || allowed("shared/values", "ucupa-realm/contract", "ucupa-realm/instance"))
	case realmTransport:
		ok = standard || protocol || imported == "google.golang.org/grpc" || strings.HasPrefix(imported, "google.golang.org/grpc/") ||
			allowed("shared/values", "internal/grpcbound", "ucupa-realm/bindings/go/protobuf", realmTransport+"/internal/entry", realmTransport+"/internal/admission", realmTransport+"/internal/wire")
		ok = ok || test && allowed("ucupa-realm/instance", "ucupa-realm/contract")
	case realmTransport + "/internal/admission":
		ok = test && standard || protocol || allowed("context", "crypto/rand", "crypto/sha256", "sync", "time", "google.golang.org/grpc", "google.golang.org/grpc/credentials", "google.golang.org/grpc/metadata", "google.golang.org/grpc/peer", "shared/values", "ucupa-realm/bindings/go/protobuf", realmTransport+"/internal/wire")
	case realmTransport + "/internal/entry":
		ok = test && standard || protocol || allowed("context", "crypto/sha256", "errors", "time", "shared/values", "ucupa-realm/contract", "ucupa-realm/bindings/go/protobuf", realmTransport+"/internal/admission", realmTransport+"/internal/wire")
	case realmTransport + "/internal/wire":
		ok = allowed("errors", "fmt", "shared/values", "internal/protovalue", "ucupa-realm/bindings/go/protobuf") || protocol || test && standard
	default:
		return true, "Realm source has no reviewed responsibility; do not add a generic internal container"
	}
	if ok {
		return true, ""
	}
	return true, "Realm dependency crosses its reviewed contract, policy, identity, assembly or transport boundary"
}

var realmSourceFiles = map[string]string{
	"ucupa-realm/contract":                 "binding.go credential.go errors.go identity.go",
	"ucupa-realm/instance":                 "instance.go identity.go",
	"ucupa-realm/internal/token":           "revision.go revision_test.go",
	"ucupa-realm/internal/identity":        "identity_assertion.go identity_verifier.go identity_assertion_test.go",
	"ucupa-realm/internal/policy":          "owner.go binding.go credential.go authorization_view.go authorization_denial.go store.go store_commit.go store_codec.go store_load.go store_binding_codec.go store_binding_history.go store_credential_codec.go store_credential_history.go realm_test.go binding_test.go credential_test.go authorization_view_test.go authorization_denial_test.go caller_fork_test.go authority_scope_test.go facet_scope_test.go durability_process_test.go durability_test.go store_atomicity_test.go credential_durability_test.go credential_store_atomicity_test.go store_codec_test.go store_fixture_test.go type_fixture_test.go",
	"ucupa-realm/tests/instance":           "instance_test.go",
	realmTransport:                         "server.go config.go server_test.go network_test.go service_test.go authorization_direct_test.go foreign_direct_test.go clock_fixture_test.go requirement_fixture_test.go",
	realmTransport + "/internal/admission": "service.go handles.go challenge.go handles_test.go",
	realmTransport + "/internal/entry":     "entry.go service.go authorization.go",
	realmTransport + "/internal/wire":      "requirements.go representation.go requirements_test.go representation_test.go",
}

func realmLocation(file string) (bool, string) {
	dir := path.Dir(file)
	if dir == "ucupa-realm" {
		return true, "Realm root is navigation only; no code, tests or legacy alias"
	}
	if names, ok := realmSourceFiles[dir]; ok {
		for _, name := range strings.Fields(names) {
			if name == path.Base(file) {
				return true, ""
			}
		}
		return true, "Realm source requires exact responsibility registration, including transport and nested packages"
	}
	return false, ""
}
