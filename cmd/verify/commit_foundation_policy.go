package main

import "path"

func checkCoreCommitFoundationDependency(file, imported, module string, test bool, stdlib map[string]bool) (bool, string) {
	if !test || path.Dir(file) != "ucupa-core/internal/commit" {
		return false, ""
	}

	allowed := func(dependencies ...string) bool {
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

	switch path.Base(file) {
	case "public_durability_fixture_test.go":
		if allowed("shared/values", "ucupa-core/contract", "ucupa-core/internal/access",
			"ucupa-core/internal/access/entry", "ucupa-core/internal/access/observation",
			"ucupa-core/internal/access/registry", "ucupa-core/internal/commit",
			"ucupa-core/internal/element", "ucupa-core/internal/query",
			"ucupa-core/internal/query/federation/coordinator", "ucupa-core/internal/query/federation/source",
			"ucupa-core/internal/query/local") {
			return true, ""
		}
		return true, "Core public durability fixture assembles only the real public entry and its single commit/query/access components; it cannot inspect raw Store, import instance or fabricate fact/token values"
	case "public_durability_test.go":
		if allowed("shared/values", "ucupa-core/contract", "ucupa-core/internal/commit") {
			return true, ""
		}
		return true, "Core public durability assertions use only public requests, results and the existing test-only commit fault seam; assembly stays in their exact fixture"
	case "public_lookup_test.go":
		if allowed("shared/values", "ucupa-core/contract", "ucupa-core/internal/commit/binding",
			"ucupa-core/internal/element/fact", "ucupa-core/internal/token") {
			return true, ""
		}
		return true, "Core retained-lookup assertions inspect only the original binding, outcome and token values in the commit owner, not public entry, Store or query workflows"
	case "foundation_candidate_fixture_test.go":
		if allowed("shared/values", "ucupa-core/contract", "ucupa-core/internal/commit/binding", "ucupa-core/internal/element/fact") {
			return true, ""
		}
		return true, "Core Foundation candidate fixtures may use only the reviewed binding and Element fact values, public contracts, shared values and standard library"
	case "foundation_candidate_test.go":
		if allowed("ucupa-core/contract", "ucupa-core/internal/commit/binding", "ucupa-core/internal/element/fact", "ucupa-core/internal/facet/fact") {
			return true, ""
		}
		return true, "Core Foundation candidate tests may use only deterministic binding, Element and Facet facts, public contracts and standard library"
	case "foundation_history_test.go":
		if allowed("ucupa-core/contract", "ucupa-core/internal/element/fact") {
			return true, ""
		}
		return true, "Core Foundation history tests may use only Element fact operations, public contracts and standard library"
	case "foundation_store_test.go":
		if allowed("go.etcd.io/bbolt", "ucupa-core/contract", "ucupa-core/internal/element/fact") {
			return true, ""
		}
		return true, "Core Foundation Store tests may use only the exact bbolt Store fixture, Element fact operations, public contracts and standard library"
	case "foundation_store_fixture_test.go":
		if allowed("go.etcd.io/bbolt", "shared/values", "ucupa-core/contract", "ucupa-core/internal/commit/binding", "ucupa-core/internal/element/fact") {
			return true, ""
		}
		return true, "Core Foundation Store fixtures may use only the exact bbolt Store fixture, binding and Element fact values, public contracts, shared values and standard library"
	case "foundation_replaced_test.go":
		if allowed("shared/values", "ucupa-core/contract", "ucupa-core/internal/commit", "ucupa-core/internal/element") {
			return true, ""
		}
		return true, "Core replaced-authority tests may use only the real commit and Element operations, public contracts, shared values and standard library"
	case "foundation_fault_export_test.go":
		if allowed("go.etcd.io/bbolt", "shared/values", "ucupa-core/contract") {
			return true, ""
		}
		return true, "Core Foundation fault bridge may use only bbolt transaction types, public identity values, public contracts and standard library"
	default:
		return false, ""
	}
}
