package main

import (
	"path"
	"strings"
)

const coreFederationCollectImport = "ucupa-core/internal/query/federation/collect"

// Public combination scenarios use only the real instance and public values.
// Composition is restricted to the exact fixture; no business internal or
// another test package is a public-test helper library.
var coreFederationScenarioDependencies = map[string][]string{
	"fixture_test.go":       {"composition", "shared/values", "ucupa-core/instance", "ucupa-core/contract"},
	"connections_test.go":   {"shared/values", "ucupa-core/contract/federation"},
	"entry_fixture_test.go": {"shared/values", "ucupa-core/instance", "ucupa-core/contract/federation"},
	"access_test.go":        {"shared/values"},
	"lifecycle_test.go":     {"shared/values", "ucupa-core/contract/federation"},
	"policy_test.go":        {"shared/values", "ucupa-core/contract", "ucupa-core/contract/federation"},
	"read_at_test.go":       {"shared/values", "ucupa-core/contract"},
	"observation_test.go":   {"shared/values"},
}

// These owner-level tests assemble real Core sources but inspect only the
// collector's result/lease boundary. Only the fixture opens actual sources;
// this must not authorize a production collect -> instance/source cycle.
var coreCollectActualTestDependencies = map[string][]string{
	"actual_admission_test.go":   {"shared/values", "ucupa-core/contract", "ucupa-core/contract/federation"},
	"actual_fixture_test.go":     {"composition", "shared/values", "ucupa-core/contract", "ucupa-core/contract/federation", "ucupa-core/instance", coreFederationCollectImport, "ucupa-core/internal/query/federation/source"},
	"actual_sources_test.go":     {"shared/values", "ucupa-core/contract", "ucupa-core/contract/federation", coreFederationCollectImport},
	"actual_closure_test.go":     {"shared/values", "ucupa-core/contract", "ucupa-core/contract/federation"},
	"actual_observation_test.go": {"shared/values", "ucupa-core/contract", "ucupa-core/contract/federation"},
}

func checkCoreCollectActualTestDependency(file, imported, module string, test bool, stdlib map[string]bool) (bool, string) {
	if strings.HasPrefix(path.Dir(file), coreFederationCollectImport+"/") {
		return true, "Core collector has no reviewed nested assembly or test package"
	}
	if path.Dir(file) != coreFederationCollectImport {
		return false, ""
	}
	dependencies, reviewed := coreCollectActualTestDependencies[path.Base(file)]
	if !reviewed {
		return false, ""
	}
	if test && reviewedDependencyAllowed(imported, module, stdlib, dependencies...) {
		return true, ""
	}
	return true, "Core collector actual-source scenarios have exact test-only edges; no production assembly, arbitrary owners, Store or shared test library"
}

func checkCoreFederationScenarioDependency(file, imported, module string, test bool, stdlib map[string]bool) (bool, string) {
	if imported == module+"/ucupa-core/tests/federation" {
		return true, "Core Federation scenarios are tests, not a shared fixture library"
	}
	if path.Dir(file) != "ucupa-core/tests/federation" {
		return false, ""
	}
	dependencies, reviewed := coreFederationScenarioDependencies[path.Base(file)]
	if test && reviewed && reviewedDependencyAllowed(imported, module, stdlib, dependencies...) {
		return true, ""
	}
	return true, "Core public Federation tests use only their exact public instance and value edges; no private owners, transport, SDK, Store or other test helpers"
}

func reviewedDependencyAllowed(imported, module string, stdlib map[string]bool, dependencies ...string) bool {
	if stdlib[imported] {
		return true
	}
	for _, dependency := range dependencies {
		if imported == dependency || imported == module+"/"+dependency {
			return true
		}
	}
	return false
}

// checkCoreFederationCollectDependency keeps the finite cross-source read
// owner explicit. Its source files have per-file edges because geometry,
// request, mount-fact and transcript are not interchangeable dependencies.
func checkCoreFederationCollectDependency(file, imported, module string, test bool, stdlib map[string]bool) (bool, string) {
	if checked, reason := checkCoreAccessRegistryDependency(file, imported, module, test, stdlib); checked {
		return checked, reason
	}
	directory := path.Dir(file)
	if reviewedCoreFederationCollectPackage(ownerOf(splitPath(directory)), directory) {
		var dependencies []string
		switch path.Base(file) {
		case "closure.go":
			dependencies = []string{"shared/values", "ucupa-core/contract/federation", "ucupa-core/internal/query/federation/transcript"}
		case "evidence.go", "exact.go", "observation.go":
			dependencies = []string{"shared/values", "ucupa-core/contract/federation"}
		case "page.go":
			dependencies = []string{"shared/values", "ucupa-core/contract", "ucupa-core/contract/federation"}
		case "path.go":
			dependencies = []string{"shared/values", "ucupa-core/contract", "ucupa-core/contract/federation", "ucupa-core/internal/geometry"}
		case "plan.go":
			dependencies = []string{"shared/values", "ucupa-core/contract", "ucupa-core/contract/federation", "ucupa-core/internal/geometry", "ucupa-core/internal/query/request"}
		case "read.go":
			dependencies = []string{"shared/values", "ucupa-core/contract"}
		case "sources.go":
			dependencies = []string{"shared/values", "ucupa-core/contract", "ucupa-core/contract/federation", "ucupa-core/internal/mount/fact"}
		case "types.go":
			dependencies = []string{"shared/values", "ucupa-core/contract", "ucupa-core/contract/federation"}
		case "collect_test.go":
			if !test {
				return true, "Federation collect tests are test-only"
			}
			dependencies = []string{"shared/values", "ucupa-core/contract", "ucupa-core/contract/federation", "ucupa-core/internal/geometry", "ucupa-core/internal/token"}
		default:
			return true, "Core Federation collect source is not in the reviewed finite-read file set"
		}
		if reviewedDependencyAllowed(imported, module, stdlib, dependencies...) {
			return true, ""
		}
		return true, "Core Federation collect owns only the reviewed finite source-set read; each file has an explicit value, geometry, request, mount-fact or transcript edge"
	}

	if imported != module+"/"+coreFederationCollectImport {
		return false, ""
	}
	// Actual coordinator, Registry and test edges are handled by their exact
	// source policies above. Retired root files are not permitted consumers.
	return true, "Federation collect is consumed only by its reviewed coordinator, resource owner and exact owner-level tests"
}

func reviewedCoreFederationCollectPackage(owner, directory string) bool {
	return owner == "ucupa-core" && directory == owner+"/internal/query/federation/collect"
}

func reviewedCoreFederationCollectSourceFile(file string) bool {
	if _, reviewed := coreCollectActualTestDependencies[file]; reviewed {
		return true
	}
	switch file {
	case "types.go", "sources.go", "evidence.go", "read.go", "path.go", "plan.go", "page.go", "closure.go", "observation.go", "exact.go", "collect_test.go":
		return true
	default:
		return false
	}
}

func splitPath(directory string) []string {
	parts := make([]string, 0, 4)
	for directory != "." && directory != "" {
		parent := path.Dir(directory)
		base := path.Base(directory)
		parts = append([]string{base}, parts...)
		if parent == directory {
			break
		}
		directory = parent
	}
	return parts
}
