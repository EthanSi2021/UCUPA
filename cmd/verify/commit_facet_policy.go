package main

import "path"

func checkCoreCommitFacetDependency(file, imported, module string, test bool, stdlib map[string]bool) (bool, string) {
	if !test || path.Dir(file) != "ucupa-core/internal/commit" {
		return false, ""
	}
	var dependencies []string
	switch path.Base(file) {
	case "facet_lifecycle_fixture_test.go":
		dependencies = []string{
			"shared/values", "ucupa-core/contract", "ucupa-core/internal/access",
			"ucupa-core/internal/commit", "ucupa-core/internal/element", "ucupa-core/internal/facet",
		}
	case "facet_lifecycle_test.go", "facet_recovery_test.go":
		dependencies = []string{"shared/values", "ucupa-core/contract", "ucupa-core/internal/commit"}
	case "facet_fault_export_test.go":
		dependencies = []string{
			"go.etcd.io/bbolt", "shared/values", "ucupa-core/contract",
			"ucupa-core/internal/facet/fact", "ucupa-core/internal/store",
		}
	default:
		return false, ""
	}
	if reviewedDependencyAllowed(imported, module, stdlib, dependencies...) {
		return true, ""
	}
	return true, "Core commit Facet tests are limited to their exact fixture, lifecycle/recovery or fixed Store-fault dependency closure"
}
