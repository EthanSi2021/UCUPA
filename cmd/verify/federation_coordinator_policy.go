package main

import (
	"path"
	"strings"
)

const coreCoordinatorDirectory = "ucupa-core/internal/query/federation/coordinator"
const coreResolveDirectory = "ucupa-core/internal/access/resolve"

// These are the reviewed component edges, not permission to import arbitrary
// query/access descendants. The coordinator owns no registry, snapshots or facts.
var coreCoordinatorDependencies = map[string][]string{
	"operation.go":           {"ucupa-core/internal/access", "ucupa-core/internal/access/observation", "ucupa-core/internal/access/registry", "ucupa-core/internal/commit", "ucupa-core/internal/query/federation/source"},
	"read.go":                {"shared/values", "ucupa-core/contract", "ucupa-core/contract/federation", "ucupa-core/internal/access/registry", "ucupa-core/internal/access/resolve", "ucupa-core/internal/access/value", "ucupa-core/internal/query/federation/collect", "ucupa-core/internal/query/federation/source"},
	"read_at.go":             {"shared/values", "ucupa-core/contract", "ucupa-core/internal/access", "ucupa-core/internal/access/registry", "ucupa-core/internal/access/resolve", "ucupa-core/internal/access/value", "ucupa-core/internal/geometry", "ucupa-core/internal/query/federation/collect"},
	"observe.go":             {"shared/values", "ucupa-core/internal/access/registry", "ucupa-core/internal/access/resolve", "ucupa-core/internal/access/value", "ucupa-core/internal/query/federation/collect"},
	"plan.go":                {"shared/values", "ucupa-core/contract", "ucupa-core/internal/access/value", "ucupa-core/internal/query/federation/collect"},
	"path.go":                {"shared/values", "ucupa-core/internal/access/value", "ucupa-core/internal/geometry", "ucupa-core/internal/query/federation/collect"},
	"disclosure.go":          {"shared/values", "ucupa-core/contract", "ucupa-core/internal/query/federation/collect"},
	"page.go":                {"shared/values", "ucupa-core/contract", "ucupa-core/contract/federation", "ucupa-core/internal/access/value", "ucupa-core/internal/query/federation/collect"},
	"fixture_test.go":        {"shared/values", "ucupa-core/contract", "ucupa-core/contract/federation", "ucupa-core/internal/access", "ucupa-core/internal/access/observation", "ucupa-core/internal/access/registry", "ucupa-core/internal/access/value", "ucupa-core/internal/commit", "ucupa-core/internal/element", "ucupa-core/internal/query", "ucupa-core/internal/query/federation/source"},
	"operation_test.go":      {"shared/values", "ucupa-core/contract", "ucupa-core/internal/access/value"},
	"representation_test.go": {"shared/values", "ucupa-core/contract", "ucupa-core/contract/federation", "ucupa-core/internal/query/federation/collect"},
}

var coreResolveDependencies = map[string][]string{
	"resolve.go":      {"shared/values", "ucupa-core/internal/access/registry", "ucupa-core/internal/access/value", "ucupa-core/internal/commit"},
	"resolve_test.go": {"shared/values", "ucupa-core/contract", "ucupa-core/internal/access/registry", "ucupa-core/internal/access/value", "ucupa-core/internal/commit"},
}

func checkCoreCoordinatorLocation(file string) (bool, string) {
	directory := path.Dir(file)
	for root, files := range map[string]map[string][]string{coreCoordinatorDirectory: coreCoordinatorDependencies, coreResolveDirectory: coreResolveDependencies} {
		if directory == root {
			if _, ok := files[path.Base(file)]; ok {
				return true, ""
			}
			return true, "Core coordinator/resolve file is not in the reviewed responsibility set"
		}
		if strings.HasPrefix(directory, root+"/") {
			return true, "Core coordinator/resolve has no approved nested package"
		}
	}
	return false, ""
}

func checkCoreCoordinatorDependency(file, imported, module string, test bool, stdlib map[string]bool) (bool, string) {
	directory := path.Dir(file)
	for root, files := range map[string]map[string][]string{coreCoordinatorDirectory: coreCoordinatorDependencies, coreResolveDirectory: coreResolveDependencies} {
		if directory == root {
			dependencies, ok := files[path.Base(file)]
			if !ok || test != strings.HasSuffix(file, "_test.go") {
				return true, "Core coordinator/resolve source is outside the exact reviewed file set"
			}
			if reviewedDependencyAllowed(imported, module, stdlib, dependencies...) {
				return true, ""
			}
			return true, "Core coordinator/resolve dependency is not its reviewed component or value edge"
		}
	}
	if imported == module+"/"+coreCoordinatorDirectory {
		if file == "ucupa-core/instance/instance.go" || file == "ucupa-core/internal/access/entry/operation.go" {
			return true, ""
		}
		return true, "Core coordinator is assembled only by the instance assembler and public entry operation, not a second workflow owner"
	}
	if imported == module+"/"+coreResolveDirectory {
		switch file {
		case "ucupa-core/public_entry.go", "ucupa-core/public_read.go", "ucupa-core/public_mutation.go",
			"ucupa-core/internal/access/observation/observe.go",
			"ucupa-core/internal/query/federation/source/check.go",
			"ucupa-core/internal/query/federation/source/hidden.go",
			"ucupa-core/internal/query/federation/source/observe.go",
			"ucupa-core/internal/query/federation/source/projection.go":
			return true, ""
		default:
			return true, "Core context resolution is used only by its exact admitted workflow callers"
		}
	}
	return false, ""
}
