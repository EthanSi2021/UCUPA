package main

import "path"

func checkCoreCurrentBoundaryDependency(file, imported, module string, test bool, stdlib map[string]bool) (bool, string) {
	if checked, reason := checkCoreFacetServiceDependency(file, imported, module, test, stdlib); checked {
		return checked, reason
	}
	if checked, reason := checkCoreAccessObservationDependency(file, imported, module, test, stdlib); checked {
		return checked, reason
	}
	if checked, reason := checkCoreAccessRegistryDependency(file, imported, module, test, stdlib); checked {
		return checked, reason
	}
	if checked, reason := checkCoreFederationDenialDependency(file, imported, module, test, stdlib); checked {
		return checked, reason
	}
	if checked, reason := checkCoreCommitStoreCorruptionDependency(file, imported, module, test, stdlib); checked {
		return checked, reason
	}
	if checked, reason := checkCoreCommitMountStoreDependency(file, imported, module, test, stdlib); checked {
		return checked, reason
	}
	if checked, reason := checkCoreCommitMountControlDependency(file, imported, module, test, stdlib); checked {
		return checked, reason
	}
	if checked, reason := checkCoreCommitNestedMountDependency(file, imported, module, test, stdlib); checked {
		return checked, reason
	}
	if checked, reason := checkCoreCommitQueryTestDependency(file, imported, module, test, stdlib); checked {
		return checked, reason
	}
	if checked, reason := checkCoreStoreFormatDependency(file, imported, module, test, stdlib); checked {
		return checked, reason
	}
	return false, ""
}

func checkCoreCommitNestedMountDependency(file, imported, module string, test bool, stdlib map[string]bool) (bool, string) {
	if !test || path.Dir(file) != "ucupa-core/internal/commit" {
		return false, ""
	}
	var dependencies []string
	switch path.Base(file) {
	case "mount_nested_fixture_test.go":
		dependencies = []string{"context", "fmt", "path/filepath", "testing", "shared/values", "ucupa-core/contract", "ucupa-core/internal/commit"}
	case "mount_nested_test.go":
		dependencies = []string{"context", "errors", "fmt", "testing", "ucupa-core/contract", "ucupa-core/internal/commit", "ucupa-core/internal/mount/fact"}
	default:
		return false, ""
	}
	if reviewedDependencyAllowed(imported, module, stdlib, dependencies...) {
		return true, ""
	}
	return true, "Core nested-mount tests use only the explicit multi-owner fixture, commit owner and finite mount facts; they may not widen into Store, query, transport or other workflows"
}

func checkCoreCommitMountControlDependency(file, imported, module string, test bool, stdlib map[string]bool) (bool, string) {
	if !test || path.Dir(file) != "ucupa-core/internal/commit" {
		return false, ""
	}
	var dependencies []string
	switch path.Base(file) {
	case "mount_control_export_test.go":
		dependencies = []string{"ucupa-core/contract"}
	case "mount_control_fixture_test.go":
		dependencies = []string{"context", "crypto/rand", "sync/atomic", "testing", "shared/values", "ucupa-core/contract", "ucupa-core/internal/commit", "ucupa-core/internal/element", "ucupa-core/internal/mount"}
	case "mount_control_test.go":
		dependencies = []string{"context", "errors", "reflect", "testing", "shared/values", "ucupa-core/contract", "ucupa-core/internal/commit", "ucupa-core/internal/mount/fact"}
	case "mount_interleaving_test.go":
		dependencies = []string{"context", "errors", "reflect", "sync/atomic", "testing", "time", "shared/values", "ucupa-core/contract", "ucupa-core/internal/commit"}
	default:
		return false, ""
	}
	if reviewedDependencyAllowed(imported, module, stdlib, dependencies...) {
		return true, ""
	}
	return true, "Core mount-control tests use only their exact control fixture, finite mount facts and fixed commit-owner probes; they may not widen commit state, Store, query or transport"
}

func checkCoreCommitMountStoreDependency(file, imported, module string, test bool, stdlib map[string]bool) (bool, string) {
	if !test || path.Dir(file) != "ucupa-core/internal/commit" {
		return false, ""
	}
	var dependencies []string
	switch path.Base(file) {
	case "mount_store_atomicity_test.go":
		dependencies = []string{"ucupa-core/contract", "ucupa-core/internal/commit", "ucupa-core/internal/mount/fact"}
	case "mount_store_corruption_test.go":
		dependencies = []string{"ucupa-core/contract", "ucupa-core/internal/commit"}
	case "mount_store_outcomes_test.go":
		dependencies = []string{"ucupa-core/contract", "ucupa-core/internal/commit"}
	case "mount_store_fixture_test.go":
		dependencies = []string{"shared/values", "ucupa-core/contract", "ucupa-core/internal/commit", "ucupa-core/internal/element", "ucupa-core/internal/mount"}
	case "mount_store_fault_export_test.go":
		dependencies = []string{"ucupa-core/contract"}
	case "mount_store_corruption_export_test.go":
		dependencies = []string{"go.etcd.io/bbolt", "ucupa-core/internal/store"}
	case "mount_store_history_export_test.go":
		dependencies = []string{"shared/values", "ucupa-core/contract", "ucupa-core/internal/mount/fact", "go.etcd.io/bbolt"}
	default:
		return false, ""
	}
	if reviewedDependencyAllowed(imported, module, stdlib, dependencies...) {
		return true, ""
	}
	return true, "Core mount Store tests use only their exact control fixture, bounded mount facts, fixed Store fault bridges and standard library; they may not widen the commit owner or expose another workflow"
}

func checkCoreCommitQueryTestDependency(file, imported, module string, test bool, stdlib map[string]bool) (bool, string) {
	if !test || path.Dir(file) != "ucupa-core/internal/commit" {
		return false, ""
	}
	var dependencies []string
	switch path.Base(file) {
	case "query_conformance_test.go":
		dependencies = []string{"shared/values", "ucupa-core/contract", "ucupa-core/internal/commit"}
	case "query_limits_test.go":
		dependencies = []string{"shared/values", "ucupa-core/contract", "ucupa-core/internal/commit", "ucupa-core/internal/geometry", "ucupa-core/internal/query/oracle"}
	case "query_corruption_test.go":
		dependencies = []string{"ucupa-core/contract", "ucupa-core/internal/commit"}
	case "query_durability_test.go":
		dependencies = []string{"shared/values", "ucupa-core/contract", "ucupa-core/internal/commit"}
	case "query_fixture_test.go":
		dependencies = []string{"crypto/rand", "shared/values", "ucupa-core/contract", "ucupa-core/internal/commit", "ucupa-core/internal/element", "ucupa-core/internal/query", "ucupa-core/internal/query/oracle"}
	case "query_fixture_export_test.go":
		dependencies = []string{"shared/values", "ucupa-core/contract", "ucupa-core/internal/commit/binding", "ucupa-core/internal/element/fact", "ucupa-core/internal/geometry"}
	case "query_fault_export_test.go":
		dependencies = []string{"ucupa-core/contract"}
	default:
		return false, ""
	}
	if reviewedDependencyAllowed(imported, module, stdlib, dependencies...) {
		return true, ""
	}
	return true, "Core query conformance tests use only their exact owner fixture, finite oracle and fixed test bridges; they may not widen commit state, Store, mount workflows or transport"
}

func checkCoreFederationDenialDependency(file, imported, module string, test bool, stdlib map[string]bool) (bool, string) {
	directory := path.Dir(file)
	if reviewedCoreFederationDenialPackage(ownerOf(splitPath(directory)), directory) {
		var dependencies []string
		switch path.Base(file) {
		case "evidence.go":
			dependencies = []string{"shared/values", "ucupa-core/internal/mount/fact", "ucupa-core/internal/query/federation/value"}
		case "evidence_test.go":
			if !test {
				return true, "Federation denial tests are test-only"
			}
			dependencies = []string{"shared/values", "ucupa-core/internal/mount/fact", "ucupa-core/internal/query/federation/value"}
		default:
			return true, "Core Federation denial source is not in the reviewed opaque-evidence file set"
		}
		if reviewedDependencyAllowed(imported, module, stdlib, dependencies...) {
			return true, ""
		}
		return true, "Core Federation denial owns only opaque evidence and its bounded mount/value inputs; it cannot depend on contracts, owners, Store, commit, transport or SDK"
	}

	if imported != module+"/ucupa-core/internal/query/federation/denial" {
		return false, ""
	}
	switch file {
	case "ucupa-core/contract/federation/federation.go",
		"ucupa-core/public_federation_denial.go",
		"ucupa-core/public_federation_part.go",
		"ucupa-core/public_federation_evidence.go",
		"ucupa-core/public_federation_hidden.go",
		"ucupa-core/contract/federation/federation_test.go":
		return true, ""
	default:
		return true, "Federation denial is consumed only by the Federation contract alias, its contract shape test and the five reviewed Core denial owners"
	}
}

func checkCoreCommitStoreCorruptionDependency(file, imported, module string, test bool, stdlib map[string]bool) (bool, string) {
	if !test || path.Dir(file) != "ucupa-core/internal/commit" {
		return false, ""
	}
	var dependencies []string
	switch path.Base(file) {
	case "store_corruption_test.go":
		dependencies = []string{"ucupa-core/contract", "ucupa-core/internal/commit"}
	case "store_corruption_export_test.go":
		dependencies = []string{"go.etcd.io/bbolt", "shared/values", "ucupa-core/contract", "ucupa-core/internal/geometry", "ucupa-core/internal/store"}
	case "store_schema_test.go":
		dependencies = nil
	default:
		return false, ""
	}
	if reviewedDependencyAllowed(imported, module, stdlib, dependencies...) {
		return true, ""
	}
	return true, "Core Store corruption/schema tests are limited to their exact durable fixture, fixed fault bridge or standard-library schema reflection"
}

func checkCoreStoreFormatDependency(file, imported, module string, test bool, stdlib map[string]bool) (bool, string) {
	if !test || path.Dir(file) != "ucupa-core/internal/store" || path.Base(file) != "format_test.go" {
		return false, ""
	}
	if stdlib[imported] {
		return true, ""
	}
	return true, "Core Store format marker test may use only the standard library while scanning all owning production Go files"
}
