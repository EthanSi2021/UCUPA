package main

import "path"

// These files are the registry-owned external regression for the
// original public queue/expiry/working-memory cases. Only its fixture may
// assemble real operations; production registry dependencies do not expand.
var publicResourceTestDependencies = map[string][]string{
	"source_lifetime_test.go":        {"context", "errors", "testing", "time", "shared/values", "ucupa-core/contract", "ucupa-core/contract/federation", "ucupa-core/internal/access/registry"},
	"composite_lifetime_test.go":     {"context", "errors", "sync", "sync/atomic", "testing", "time", "shared/values", "ucupa-core/contract", "ucupa-core/contract/federation", "ucupa-core/internal/access/registry", "ucupa-core/internal/query/federation/collect", "ucupa-core/internal/query/federation/source"},
	"public_resource_export_test.go": {"time"},
	"public_resource_test.go": {
		"errors", "testing", "shared/values", "ucupa-core/contract", "ucupa-core/internal/access/registry",
	},
	"public_resource_fixture_test.go": {
		"context", "encoding/json", "fmt", "testing", "time", "shared/values", "ucupa-core/contract",
		"ucupa-core/internal/access", "ucupa-core/internal/access/entry", "ucupa-core/internal/access/observation",
		"ucupa-core/internal/access/registry", "ucupa-core/internal/commit", "ucupa-core/internal/element",
		"ucupa-core/internal/query", "ucupa-core/internal/query/federation/coordinator",
		"ucupa-core/internal/query/federation/source", "ucupa-core/internal/query/local",
	},
}

func checkPublicResourceTestDependency(file, imported, module string, test bool, stdlib map[string]bool) (bool, string) {
	if path.Dir(file) != "ucupa-core/internal/access/registry" {
		return false, ""
	}
	dependencies, reviewed := publicResourceTestDependencies[path.Base(file)]
	if !reviewed {
		return false, ""
	}
	if test && reviewedDependencyAllowed(imported, module, stdlib, dependencies...) {
		return true, ""
	}
	return true, "Core public resource fault regression is test-only; only its exact fixture may assemble actual operations, never instance, Store, fact mutation, transport or SDK"
}
