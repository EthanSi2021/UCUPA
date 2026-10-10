package main

import "path"

// checkCoreAccessObservationDependency keeps local/source observation
// execution on the shared owner, admission and registry edges. The package
// owns no second queue, snapshot or fact store, so it is deliberately checked
// before the registry consumer rule below.
func checkCoreAccessObservationDependency(file, imported, module string, test bool, stdlib map[string]bool) (bool, string) {
	directory := path.Dir(file)
	if reviewedCoreAccessObservationPackage(ownerOf(splitPath(directory)), directory) {
		var dependencies []string
		switch path.Base(file) {
		case "operation.go":
			if test {
				return true, "Core access observation production files are not test files"
			}
			dependencies = []string{
				"ucupa-core/internal/access", "ucupa-core/internal/access/registry",
				"ucupa-core/internal/commit",
			}
		case "observe.go":
			if test {
				return true, "Core access observation production files are not test files"
			}
			dependencies = []string{
				"shared/values", "ucupa-core/internal/access", "ucupa-core/internal/access/registry", "ucupa-core/internal/access/value",
			}
		case "operation_test.go":
			if !test {
				return true, "Core access observation tests are test-only"
			}
			dependencies = []string{
				"errors", "reflect", "testing", "time", "shared/values",
				"ucupa-core/contract", "ucupa-core/contract/federation",
				"ucupa-core/internal/access", "ucupa-core/internal/access/observation",
				"ucupa-core/internal/access/registry", "ucupa-core/internal/access/value",
				"ucupa-core/internal/commit", "ucupa-core/internal/element",
			}
		default:
			return true, "Core access observation source is not in the reviewed observation operation file set"
		}
		if reviewedDependencyAllowed(imported, module, stdlib, dependencies...) {
			return true, ""
		}
		return true, "Core access observation owns only the shared local/source operation state machine; it may not depend on Core facts, mount/query workflows, Store, transport, SDK or another owner"
	}

	if imported != module+"/ucupa-core/internal/access/observation" {
		return false, ""
	}
	switch file {
	case "ucupa-core/instance/instance.go",
		"ucupa-core/internal/query/federation/coordinator/operation.go",
		"ucupa-core/internal/query/federation/source/operation.go",
		"ucupa-core/internal/query/federation/source/fixture_test.go":
		return true, ""
	default:
		return true, "Core access observation is consumed only by the exact instance, coordinator and source operation files"
	}
}
