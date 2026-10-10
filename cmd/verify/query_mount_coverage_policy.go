package main

import "path"

func checkCoreQueryMountCoverageDependency(file, imported, module string, test bool, stdlib map[string]bool) (bool, string) {
	if !test || path.Dir(file) != "ucupa-core/tests/query" {
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
	case "mount_coverage_fixture_test.go":
		if allowed("composition", "shared/values", "ucupa-core/instance", "ucupa-core/contract") {
			return true, ""
		}
		return true, "Core mount-coverage fixtures may use only composition, Core entry points, public contracts, shared values and standard library"
	case "mount_coverage_test.go":
		if allowed("shared/values", "ucupa-core/instance") {
			return true, ""
		}
		return true, "Core mount-coverage assertions may use only Core public entry points, shared values and standard library"
	default:
		return false, ""
	}
}
