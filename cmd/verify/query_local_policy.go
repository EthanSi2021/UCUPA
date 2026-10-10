package main

import (
	"path"
	"strings"
)

const coreQueryLocalDirectory = "ucupa-core/internal/query/local"

const coreElementPublicFile = "ucupa-core/internal/element/public.go"

const coreAccessIntegrationTestDirectory = "ucupa-core/tests/access"

func reviewedCoreQueryLocalPackage(owner, directory string) bool {
	return owner == "ucupa-core" && directory == coreQueryLocalDirectory
}

func reviewedCoreQueryLocalSourceFile(file string) bool {
	switch file {
	case "operation.go", "read.go", "address.go", "list.go", "query.go":
		return true
	default:
		return false
	}
}

func reviewedCoreAccessIntegrationTestPackage(owner, directory string) bool {
	return owner == "ucupa-core" && directory == coreAccessIntegrationTestDirectory
}

func reviewedCoreAccessIntegrationTestSourceFile(file string) bool {
	switch file {
	case "access_test.go", "boundary_test.go", "fixture_test.go":
		return true
	default:
		return false
	}
}

var coreQueryLocalDependencies = map[string]map[string]bool{
	"operation.go": {
		"ucupa-core/internal/access":          true,
		"ucupa-core/internal/access/registry": true,
		"ucupa-core/internal/commit":          true,
		"ucupa-core/internal/query":           true,
	},
	"read.go": {
		"shared/values":                       true,
		"ucupa-core/contract":                 true,
		"ucupa-core/internal/access":          true,
		"ucupa-core/internal/access/registry": true,
		"ucupa-core/internal/access/resolve":  true,
		"ucupa-core/internal/access/value":    true,
		"ucupa-core/internal/element/read":    true,
		"ucupa-core/internal/query":           true,
	},
	"address.go": {
		"shared/values":                    true,
		"ucupa-core/internal/access/value": true,
		"ucupa-core/internal/element/fact": true,
		"ucupa-core/internal/mount/fact":   true,
	},
	"list.go": {
		"shared/values":                    true,
		"ucupa-core/internal/access/value": true,
		"ucupa-core/internal/commit":       true,
		"ucupa-core/internal/element/fact": true,
		"ucupa-core/internal/element/read": true,
		"ucupa-core/internal/mount/fact":   true,
		"ucupa-core/internal/query/scan":   true,
		"ucupa-core/internal/token":        true,
	},
	"query.go": {
		"shared/values":                     true,
		"ucupa-core/contract":               true,
		"ucupa-core/internal/access/value":  true,
		"ucupa-core/internal/commit":        true,
		"ucupa-core/internal/query/request": true,
		"ucupa-core/internal/query/scan":    true,
		"ucupa-core/internal/token":         true,
	},
}

var coreElementPublicDependencies = map[string]bool{
	"shared/values":                       true,
	"ucupa-core/contract":                 true,
	"ucupa-core/internal/access":          true,
	"ucupa-core/internal/access/registry": true,
	"ucupa-core/internal/access/resolve":  true,
	"ucupa-core/internal/access/value":    true,
	"ucupa-core/internal/commit":          true,
	"ucupa-core/internal/commit/binding":  true,
	"ucupa-core/internal/token":           true,
}

var coreAccessValueEntryDependencies = map[string]bool{
	"shared/values":       true,
	"ucupa-core/contract": true,
}

func checkCoreLocalPublicDependency(file, imported, module string, test bool, stdlib map[string]bool) (bool, string) {
	directory := path.Dir(file)
	owner := ownerOf(strings.Split(directory, "/"))
	base := path.Base(file)
	if reviewedCoreQueryLocalPackage(owner, directory) {
		if !reviewedCoreQueryLocalSourceFile(base) {
			return true, "Core local query source is not in the reviewed five-file read operation set"
		}
		if test {
			return true, "Core local query tests require an exact file review before they may import dependencies"
		}
		if stdlib[imported] || coreQueryLocalDependencies[base][localDependencyName(imported, module)] {
			return true, ""
		}
		return true, "Core local query owns the public read/list/query flow over its four assembled components; it cannot depend on Store, facts outside the finite read views, transport, SDK or another owner"
	}
	if reviewedCoreAccessIntegrationTestPackage(owner, directory) {
		if !reviewedCoreAccessIntegrationTestSourceFile(base) {
			return true, "Core public access scenarios are limited to the reviewed access, boundary and fixture tests"
		}
		if !test {
			return true, "Core public access scenarios are test-only"
		}
		if stdlib[imported] || coreAccessIntegrationDependencyAllowed(base, localDependencyName(imported, module)) {
			return true, ""
		}
		return true, "Core public access scenarios may use only the exact public Core, Realm, schema and value contracts"
	}
	if directory == path.Dir(coreElementPublicFile) && base == path.Base(coreElementPublicFile) {
		if test {
			return true, "Core public Element operations have no unreviewed test file"
		}
		if stdlib[imported] || coreElementPublicDependencies[localDependencyName(imported, module)] {
			return true, ""
		}
		return true, "Core public Element operations own only the fixed admission/registry and commit entry; no query, Store, mount workflow, transport or SDK dependency"
	}
	if directory == "ucupa-core/internal/access/value" && base == "entry.go" {
		if test {
			return true, "Core public entry values have no unreviewed test file"
		}
		if stdlib[imported] || coreAccessValueEntryDependencies[localDependencyName(imported, module)] {
			return true, ""
		}
		return true, "Core public entry values own only Intent/Reference field parsing; no authorization, geometry, token, owner, Store or transport dependency"
	}
	if imported == module+"/"+coreQueryLocalDirectory {
		if !test && (file == "ucupa-core/instance/instance.go" || file == "ucupa-core/internal/access/entry/operation.go") {
			return true, ""
		}
		return true, "Core local query is consumed only by the instance assembler and the public entry operation"
	}
	return false, ""
}

func localDependencyName(imported, module string) string {
	return strings.TrimPrefix(imported, module+"/")
}

func coreAccessIntegrationDependencyAllowed(file, dependency string) bool {
	switch file {
	case "access_test.go":
		return dependency == "shared/values" || dependency == "ucupa-core/instance" || dependency == "ucupa-core/contract" || dependency == "ucupa-realm/contract" || dependency == "ucupa-realm/instance" || dependency == "github.com/santhosh-tekuri/jsonschema/v6"
	case "boundary_test.go":
		return dependency == "shared/values" || dependency == "ucupa-core/contract"
	case "fixture_test.go":
		return dependency == "shared/values" || dependency == "ucupa-core/instance" || dependency == "ucupa-core/contract"
	default:
		return false
	}
}
