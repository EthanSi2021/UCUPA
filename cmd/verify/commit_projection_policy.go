package main

import "path"

func checkCoreCommitProjectionDependency(file, imported, module string, test bool, stdlib map[string]bool) (bool, string) {
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
	case "projection_fault_export_test.go":
		if allowed("shared/values") {
			return true, ""
		}
		return true, "Core projection fault bridges may use only shared values and standard library"
	case "projection_authorization_test.go":
		if allowed("shared/values", "ucupa-core/contract", "ucupa-core/internal/commit", "ucupa-core/internal/element") {
			return true, ""
		}
		return true, "Core projection authorization tests may use only the real commit and Element fixtures, public contracts, shared values and standard library"
	default:
		return false, ""
	}
}
