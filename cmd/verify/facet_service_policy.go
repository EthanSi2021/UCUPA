package main

import "path"

// checkCoreFacetServiceDependency keeps the two fixed Core-side Facet service
// files on their actual narrow edges. Their methods are part of Operations,
// but the service implementation must not inherit the whole Facet package's
// generic workflow allowance.
func checkCoreFacetServiceDependency(file, imported, module string, test bool, stdlib map[string]bool) (bool, string) {
	if path.Dir(file) != "ucupa-core/internal/facet" {
		return false, ""
	}
	var dependencies []string
	switch path.Base(file) {
	case "service_attachment.go":
		if test {
			return true, "Core Facet service production files are not test files"
		}
		dependencies = []string{
			"shared/values", "ucupa-core/contract", "ucupa-core/internal/access/registry",
			"ucupa-core/internal/access/value",
		}
	case "service_proposal.go":
		if test {
			return true, "Core Facet service production files are not test files"
		}
		dependencies = []string{"shared/values", "ucupa-core/contract"}
	case "service_fixture_test.go":
		if !test {
			return true, "Core Facet service fixtures are test-only"
		}
		dependencies = []string{
			"reflect", "testing", "time", "shared/values",
			"ucupa-core/internal/access", "ucupa-core/internal/access/registry", "ucupa-core/internal/facet",
		}
	case "service_test.go":
		if !test {
			return true, "Core Facet service tests are test-only"
		}
		dependencies = []string{"context", "errors", "reflect", "testing", "shared/values", "ucupa-core/contract"}
	default:
		return false, ""
	}
	if reviewedDependencyAllowed(imported, module, stdlib, dependencies...) {
		return true, ""
	}
	return true, "Core Facet service files expose only their fixed attachment/proposal service edges; they may not depend on Element, mount, query, Store, transport, SDK or unrelated owners"
}
