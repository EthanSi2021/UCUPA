package main

import "path"

// checkCoreAccessRegistryDependency keeps the finite public-resource
// registry's lifecycle, read handles and observation state on explicit edges.
// The registry is an owner-local implementation, so the generic same-owner
// internal allowance must not turn this package into a second Core state.
func checkCoreAccessRegistryDependency(file, imported, module string, test bool, stdlib map[string]bool) (bool, string) {
	directory := path.Dir(file)
	if reviewedCoreAccessRegistryPackage(ownerOf(splitPath(directory)), directory) {
		var dependencies []string
		switch path.Base(file) {
		case "registry.go":
			dependencies = []string{
				"context", "sync", "time", "shared/values", "ucupa-core/contract",
				"ucupa-core/internal/access/observe", "ucupa-core/internal/query/federation/collect",
				"ucupa-core/internal/query/federation/denial",
			}
		case "read.go":
			dependencies = []string{
				"context", "shared/values", "ucupa-core/contract",
				"ucupa-core/internal/query/federation/collect",
			}
		case "source.go":
			dependencies = []string{
				"shared/values", "ucupa-core/contract", "ucupa-core/contract/federation",
				"ucupa-core/internal/mount/fact", "ucupa-core/internal/query/federation/denial",
			}
		case "observation.go":
			dependencies = []string{
				"shared/values", "ucupa-core/contract", "ucupa-core/internal/access/observe",
			}
		case "registry_test.go":
			if !test {
				return true, "Core access registry tests are test-only"
			}
			dependencies = []string{
				"errors", "sync", "testing", "time", "shared/values", "ucupa-core/contract",
				"ucupa-core/internal/access/observe", "ucupa-core/internal/element/fact",
				"ucupa-core/internal/facet/fact", "ucupa-core/internal/mount/fact",
			}
		case "source_test.go":
			if !test {
				return true, "Core access registry tests are test-only"
			}
			dependencies = []string{
				"errors", "testing", "shared/values", "ucupa-core/contract",
				"ucupa-core/contract/federation", "ucupa-core/internal/mount/fact",
				"ucupa-core/internal/query/federation/denial",
			}
		default:
			return true, "Core access registry source is not in the reviewed registry lifecycle file set"
		}
		if reviewedDependencyAllowed(imported, module, stdlib, dependencies...) {
			return true, ""
		}
		return true, "Core access registry owns finite resource handles, TTL, accounting and release; it cannot depend on Core state, Store, commit, query owner, transport, SDK or another workflow"
	}

	if imported != module+"/ucupa-core/internal/access/registry" {
		return false, ""
	}
	switch file {
	case "ucupa-core/internal/commit/public_durability_fixture_test.go":
		if test {
			return true, ""
		}
		return true, "Core access registry's public durability fixture is test-only"
	case "ucupa-core/instance/instance.go",
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
		"ucupa-core/internal/query/local/read.go":
		return true, ""
	default:
		return true, "Core access registry is consumed only by the exact instance, entry, access, resolve, federation, Element, Facet and local-query files; Store, transport, SDK and unrelated tests may not import it"
	}
}
