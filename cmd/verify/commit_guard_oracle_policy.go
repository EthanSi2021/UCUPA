package main

import "path"

func checkCoreCommitMountGuardDependency(file, imported, module string, test bool, stdlib map[string]bool) (bool, string) {
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
	case "mount_guard_fixture_test.go":
		if allowed("shared/values", "ucupa-core/contract", "ucupa-core/internal/commit", "ucupa-core/internal/element", "ucupa-core/internal/query") {
			return true, ""
		}
		return true, "Core mount-guard fixtures may use only the real commit, Element and query fixture owners, public contracts, shared values and standard library"
	case "mount_guard_proof_export_test.go":
		if allowed("shared/values", "ucupa-core/contract") {
			return true, ""
		}
		return true, "Core mount-guard proof exports may use only public identity values, public contracts and standard library"
	case "mount_guard_read_test.go":
		if allowed("shared/values", "ucupa-core/contract", "ucupa-core/internal/commit", "ucupa-core/internal/query/oracle", "ucupa-core/internal/token") {
			return true, ""
		}
		return true, "Core mount-guard read tests may use only the real commit owner, oracle, token values, public contracts, shared values and standard library"
	case "mount_guard_validation_test.go":
		if allowed("shared/values", "ucupa-core/contract", "ucupa-core/internal/commit/binding", "ucupa-core/internal/element/fact", "ucupa-core/internal/mount/fact") {
			return true, ""
		}
		return true, "Core mount-guard validation tests may use only deterministic binding and finite fact values, public contracts, shared values and standard library"
	case "mount_guard_write_test.go":
		if allowed("ucupa-core/contract", "ucupa-core/internal/commit") {
			return true, ""
		}
		return true, "Core mount-guard write tests may use only the real commit owner, public contracts and standard library"
	default:
		return false, ""
	}
}

func checkCoreCommitOracleDependency(file, imported, module string, test bool, stdlib map[string]bool) (bool, string) {
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
	case "oracle_fault_export_test.go":
		if allowed("shared/values", "ucupa-core/contract", "ucupa-core/internal/commit/binding", "ucupa-core/internal/element/fact", "ucupa-core/internal/geometry", "ucupa-core/internal/token") {
			return true, ""
		}
		return true, "Core oracle fault bridges may use only deterministic binding, Element fact, geometry and token values, public contracts, shared values and standard library"
	case "oracle_boundary_test.go":
		if allowed("shared/values", "ucupa-core/contract", "ucupa-core/internal/commit", "ucupa-core/internal/element", "ucupa-core/internal/query/oracle") {
			return true, ""
		}
		return true, "Core oracle boundary tests may use only the real commit and Element fixture owners, the reviewed oracle, public contracts, shared values and standard library"
	default:
		return false, ""
	}
}

func checkCoreQueryOracleDependency(file, imported, module string, test bool, stdlib map[string]bool) (bool, string) {
	if path.Dir(file) != "ucupa-core/internal/query/oracle" {
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

	if !test {
		if path.Base(file) == "scan.go" && allowed(
			"shared/values", "ucupa-core/contract", "ucupa-core/internal/commit",
			"ucupa-core/internal/element/fact", "ucupa-core/internal/geometry",
			"ucupa-core/internal/mount/fact", "ucupa-core/internal/query/scan",
			"ucupa-core/internal/token",
		) {
			return true, ""
		}
		return true, "Core query oracle production owns only the reviewed linear scan and its finite read dependencies; it cannot depend on query owner, Store, transport, SDK or root workflows"
	}

	var dependencies []string
	switch path.Base(file) {
	case "fixture_test.go":
		dependencies = []string{"shared/values", "ucupa-core/contract", "ucupa-core/internal/commit", "ucupa-core/internal/element"}
	case "conformance_test.go":
		dependencies = []string{"shared/values", "ucupa-core/contract", "ucupa-core/internal/element/fact", "ucupa-core/internal/query/oracle"}
	case "failure_test.go":
		dependencies = []string{"ucupa-core/contract", "ucupa-core/internal/query/oracle"}
	case "rotation_test.go":
		dependencies = []string{"shared/values", "ucupa-core/contract", "ucupa-core/internal/query/oracle"}
	case "work_test.go":
		dependencies = []string{"ucupa-core/contract", "ucupa-core/internal/query/oracle", "ucupa-core/internal/query/scan"}
	default:
		return true, "Core query oracle tests are limited to the reviewed fixture and conformance, rotation, failure and work files"
	}
	if allowed(dependencies...) {
		return true, ""
	}
	return true, "Core query oracle tests may use only their exact deterministic assertion dependencies; only the fixture may assemble a real commit and Element owner"
}
