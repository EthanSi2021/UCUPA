package main

import "path"

const coreFederationSourceImport = "ucupa-core/internal/query/federation/source"

// checkCoreFederationSourceDependency keeps the source-side federation
// service on its six injected owner components. Every source file gets its
// actual value edges; importing the implementation is limited to Core
// assembly and the one public federated-read entry point.
func checkCoreFederationSourceDependency(file, imported, module string, test bool, stdlib map[string]bool) (bool, string) {
	directory := path.Dir(file)
	if reviewedCoreFederationSourcePackage(ownerOf(splitPath(directory)), directory) {
		var dependencies []string
		switch path.Base(file) {
		case "check.go":
			dependencies = []string{
				"context", "shared/values", "ucupa-core/contract",
				"ucupa-core/internal/access", "ucupa-core/internal/access/registry",
				"ucupa-core/internal/access/value", "ucupa-core/internal/query/federation/denial",
			}
		case "denial.go":
			dependencies = []string{
				"context", "shared/values", "ucupa-core/contract/federation",
				"ucupa-core/internal/access", "ucupa-core/internal/mount/fact",
				"ucupa-core/internal/query/federation/denial",
			}
		case "evidence.go":
			dependencies = []string{
				"crypto/hmac", "crypto/sha256", "encoding/binary",
				"shared/values", "ucupa-core/contract/federation",
				"ucupa-core/internal/access/registry",
				"ucupa-core/internal/query/federation/denial",
				"ucupa-core/internal/query/federation/value", "ucupa-core/internal/token",
			}
		case "hidden.go":
			dependencies = []string{
				"context", "shared/values", "ucupa-core/contract",
				"ucupa-core/contract/federation", "ucupa-core/internal/access",
				"ucupa-core/internal/access/registry",
				"ucupa-core/internal/access/value", "ucupa-core/internal/query/federation/denial",
				"ucupa-core/internal/query/federation/projection",
			}
		case "local.go":
			dependencies = []string{"context", "shared/values", "ucupa-core/contract/federation"}
		case "observe_binding.go":
			dependencies = []string{
				"shared/values", "ucupa-core/contract/federation",
				"ucupa-core/internal/access/registry", "ucupa-core/internal/geometry",
				"ucupa-core/internal/query/federation/projection",
			}
		case "observe.go":
			dependencies = []string{
				"context", "shared/values", "ucupa-core/contract",
				"ucupa-core/contract/federation", "ucupa-core/internal/access",
				"ucupa-core/internal/access/registry", "ucupa-core/internal/access/value",
				"ucupa-core/internal/query/federation/denial",
			}
		case "operation.go":
			dependencies = []string{
				"context", "shared/values", "ucupa-core/contract",
				"ucupa-core/internal/access", "ucupa-core/internal/access/observation",
				"ucupa-core/internal/access/registry",
				"ucupa-core/internal/commit", "ucupa-core/internal/query",
			}
		case "part.go":
			dependencies = []string{
				"context", "errors", "shared/values", "ucupa-core/contract",
				"ucupa-core/contract/federation", "ucupa-core/internal/access",
				"ucupa-core/internal/access/registry",
				"ucupa-core/internal/mount/fact",
				"ucupa-core/internal/query/federation/projection",
			}
		case "projection.go":
			dependencies = []string{
				"context", "crypto/sha256", "encoding/binary", "encoding/json", "math",
				"shared/values", "ucupa-core/contract", "ucupa-core/contract/federation",
				"ucupa-core/internal/access", "ucupa-core/internal/access/registry",
				"ucupa-core/internal/access/value", "ucupa-core/internal/geometry",
				"ucupa-core/internal/query", "ucupa-core/internal/query/federation/denial",
				"ucupa-core/internal/query/federation/projection",
				"ucupa-core/internal/query/request",
			}
		case "evidence_test.go":
			if !test {
				return true, "Core Federation source tests are test-only"
			}
			dependencies = []string{
				"context", "slices", "testing", "shared/values", "ucupa-core/contract",
				"ucupa-core/contract/federation", "ucupa-core/internal/access/registry",
				"ucupa-core/internal/query/federation/projection", "ucupa-core/internal/query/federation/value",
				"ucupa-core/internal/token",
			}
		case "fixture_test.go":
			if !test {
				return true, "Core Federation source tests are test-only"
			}
			dependencies = []string{
				"context", "crypto/sha256", "fmt", "testing", "time", "shared/values",
				"ucupa-core/contract", "ucupa-core/internal/access", "ucupa-core/internal/access/observation",
				"ucupa-core/internal/access/registry", "ucupa-core/internal/commit",
				"ucupa-core/internal/element", "ucupa-core/internal/mount", "ucupa-core/internal/query",
			}
		case "operation_test.go":
			if !test {
				return true, "Core Federation source tests are test-only"
			}
			dependencies = []string{
				"context", "errors", "reflect", "sort", "testing", "shared/values",
				"ucupa-core/contract", "ucupa-core/contract/federation", "ucupa-core/internal/access",
			}
		case "relation_test.go":
			if !test {
				return true, "Core Federation source tests are test-only"
			}
			dependencies = []string{
				"context", "errors", "testing", "shared/values",
				"ucupa-core/contract", "ucupa-core/contract/federation", "ucupa-core/internal/mount",
			}
		case "observe_test.go":
			if !test {
				return true, "Core Federation source tests are test-only"
			}
			dependencies = []string{"testing", "shared/values", "ucupa-core/contract/federation"}
		default:
			return true, "Core Federation source is not in the reviewed production file set"
		}
		if reviewedDependencyAllowed(imported, module, stdlib, dependencies...) {
			return true, ""
		}
		return true, "Core Federation source owns the injected owner-local read service; it may not depend on Store, transport, SDK, another workflow owner or an unreviewed source file"
	}

	if imported != module+"/"+coreFederationSourceImport {
		return false, ""
	}
	switch file {
	case "ucupa-core/instance/instance.go",
		"ucupa-core/internal/query/federation/coordinator/operation.go",
		"ucupa-core/internal/query/federation/coordinator/read.go",
		"ucupa-core/internal/query/federation/coordinator/fixture_test.go":
		return true, ""
	default:
		return true, "Core Federation source is constructed only by the instance assembler and its exact coordinator fixture/read operations"
	}
}
