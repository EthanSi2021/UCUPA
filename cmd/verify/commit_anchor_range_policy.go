package main

import "path"

func checkCoreCommitAnchorRangeDependency(file, imported, module string, test bool, stdlib map[string]bool) (bool, string) {
	if !test || path.Dir(file) != "ucupa-core/internal/commit" {
		return false, ""
	}
	var dependencies []string
	switch path.Base(file) {
	case "anchor_range_store_test.go":
		dependencies = []string{"shared/values", "ucupa-core/contract", "ucupa-core/internal/commit"}
	case "anchor_range_fixture_test.go":
		dependencies = []string{"shared/values", "ucupa-core/contract", "ucupa-core/internal/commit", "ucupa-core/internal/element"}
	case "anchor_range_fault_export_test.go":
		dependencies = []string{"go.etcd.io/bbolt", "ucupa-core/internal/store"}
	default:
		return false, ""
	}
	if reviewedDependencyAllowed(imported, module, stdlib, dependencies...) {
		return true, ""
	}
	return true, "Core Anchor/Range tests are limited to their exact durable fixture, Element/commit scenario or fixed Store-fault bridge"
}
