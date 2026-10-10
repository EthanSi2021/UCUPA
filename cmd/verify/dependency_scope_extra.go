package main

func reviewedCoreFederationDenialPackage(owner, directory string) bool {
	return owner == "ucupa-core" && directory == owner+"/internal/query/federation/denial"
}

func reviewedCoreFederationDenialSourceFile(file string) bool {
	return file == "evidence.go" || file == "evidence_test.go"
}

func reviewedCoreAccessRegistryPackage(owner, directory string) bool {
	return owner == "ucupa-core" && directory == owner+"/internal/access/registry"
}

func reviewedCoreAccessRegistrySourceFile(file string) bool {
	switch file {
	case "registry.go", "read.go", "source.go", "observation.go", "registry_test.go", "source_test.go",
		"public_resource_export_test.go", "public_resource_fixture_test.go", "public_resource_test.go", "source_lifetime_test.go", "composite_lifetime_test.go":
		return true
	default:
		return false
	}
}

func reviewedCoreAccessObservationPackage(owner, directory string) bool {
	return owner == "ucupa-core" && directory == owner+"/internal/access/observation"
}

func reviewedCoreAccessObservationSourceFile(file string) bool {
	switch file {
	case "operation.go", "observe.go", "operation_test.go":
		return true
	default:
		return false
	}
}

func reviewedCoreFederationSourcePackage(owner, directory string) bool {
	return owner == "ucupa-core" && directory == owner+"/internal/query/federation/source"
}

func reviewedCoreFederationSourceFile(file string) bool {
	switch file {
	case "check.go", "denial.go", "evidence.go", "hidden.go", "local.go", "observe_binding.go", "observe.go", "operation.go", "part.go", "projection.go", "evidence_test.go", "fixture_test.go", "observe_test.go", "operation_test.go", "relation_test.go":
		return true
	default:
		return false
	}
}

func reviewedStoreSourceFile(file string) bool {
	switch file {
	case "format.go", "format_test.go", "json.go", "json_test.go", "scalar.go", "scalar_test.go", "transaction.go", "transaction_test.go":
		return true
	default:
		return false
	}
}
