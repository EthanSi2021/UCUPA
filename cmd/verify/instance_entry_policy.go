package main

import "path"

const coreInstanceDirectory = "ucupa-core/instance"
const coreAccessEntryDirectory = "ucupa-core/internal/access/entry"

var coreInstanceDependencies = map[string][]string{
	"instance.go": {
		"fmt", "shared/values", "ucupa-core/contract",
		"ucupa-core/internal/access", "ucupa-core/internal/access/entry",
		"ucupa-core/internal/access/observation", "ucupa-core/internal/access/registry",
		"ucupa-core/internal/commit", "ucupa-core/internal/element", "ucupa-core/internal/facet",
		"ucupa-core/internal/mount", "ucupa-core/internal/query", "ucupa-core/internal/query/connected",
		"ucupa-core/internal/query/federation/coordinator", "ucupa-core/internal/query/federation/source",
		"ucupa-core/internal/query/local",
	},
	"token.go": {"crypto/rand", "errors"},
	"store.go": {
		"strings", "shared/values", "ucupa-core/contract", "ucupa-core/internal/commit",
		"ucupa-core/internal/query", "ucupa-core/internal/query/connected",
	},
	"contact.go": {"shared/values", "ucupa-core/contract", "ucupa-core/internal/commit"},
	"maintenance.go": {
		"fmt", "strings", "shared/values", "ucupa-core/contract", "ucupa-core/internal/commit",
		"ucupa-core/internal/query", "ucupa-core/internal/realmcontact",
	},
	"contact_test.go": {
		"errors", "path/filepath", "testing", "shared/values", "ucupa-core/contract",
		"ucupa-core/instance",
	},
	"maintenance_test.go": {
		"errors", "path/filepath", "reflect", "testing", "shared/values", "ucupa-core/contract",
		"ucupa-core/instance",
	},
}

var coreAccessEntryDependencies = map[string][]string{
	"operation.go": {
		"shared/values", "ucupa-core/internal/access", "ucupa-core/internal/access/registry",
		"ucupa-core/internal/commit", "ucupa-core/internal/element",
		"ucupa-core/internal/query/federation/coordinator", "ucupa-core/internal/query/local",
	},
	"request.go": {
		"context", "errors", "shared/values", "ucupa-core/contract", "ucupa-core/contract/federation",
		"ucupa-core/internal/access",
	},
	"space.go": {
		"shared/values", "ucupa-core/internal/access", "ucupa-core/internal/access/resolve",
		"ucupa-core/internal/access/value", "ucupa-core/internal/mount/fact",
	},
}

func checkCoreInstanceEntryDependency(file, imported, module string, test bool, stdlib map[string]bool) (bool, string) {
	directory := path.Dir(file)
	owner := ownerOf(splitPath(directory))
	base := path.Base(file)
	if reviewedCoreInstancePackage(owner, directory) {
		if !reviewedCoreInstanceSourceFile(base) {
			return true, "Core instance source is not in the reviewed assembly, lifecycle and Realm contact file set"
		}
		if reviewedDependencyAllowed(imported, module, stdlib, coreInstanceDependencies[base]...) {
			return true, ""
		}
		return true, "Core instance only assembles the fixed owner, access, query, relationship and Realm contact handles; it cannot expose or import raw State, Store, transport, SDK or another owner"
	}
	if reviewedCoreAccessEntryPackage(owner, directory) {
		if !reviewedCoreAccessEntrySourceFile(base) {
			return true, "Core public entry source is not in the reviewed operation, request or space file set"
		}
		if test {
			return true, "Core public entry operation files are production-only"
		}
		if reviewedDependencyAllowed(imported, module, stdlib, coreAccessEntryDependencies[base]...) {
			return true, ""
		}
		return true, "Core public entry owns only the six injected operation components and public envelope dispatch; it cannot import Core root, Store, facts, transport, SDK or another owner"
	}
	if imported == module+"/"+coreInstanceDirectory {
		if coreInstanceConsumerAllowed(file, test) {
			return true, ""
		}
		return true, "Core instance is consumed only by the reviewed external assembly and exact Core integration tests; Core root and owner internals may not reverse-import the public instance"
	}
	if imported == module+"/"+coreAccessEntryDirectory {
		if file == "ucupa-core/instance/instance.go" {
			return true, ""
		}
		return true, "Core public entry is constructed only by the reviewed instance assembler"
	}
	return false, ""
}

func coreInstanceConsumerAllowed(file string, test bool) bool {
	if test {
		switch file {
		case "acceptance/connected_fixture_test.go",
			"acceptance/connected_lifecycle_test.go",
			"acceptance/connected_query_test.go",
			"acceptance/facet_fixture_test.go",
			"acceptance/facet_local_test.go",
			"acceptance/mount_authenticated_prepare_test.go",
			"acceptance/mount_fixture_test.go",
			"acceptance/network_admission_test.go",
			"acceptance/network_failure_test.go",
			"acceptance/network_facet_reacceptance_test.go",
			"acceptance/network_facet_recovery_test.go",
			"acceptance/network_facet_replacement_test.go",
			"acceptance/network_federation_nested_fixture_test.go",
			"acceptance/network_real_host_federation_fixture_test.go",
			"acceptance/network_federation_part_test.go",
			"acceptance/network_fixture_test.go",
			"acceptance/writer_boundary_test.go",
			"acceptance/network_mount_approval_test.go",
			"acceptance/network_mount_crash_helper_test.go",
			"acceptance/network_mount_crash_test.go",
			"acceptance/network_mount_product_test.go",
			"acceptance/network_realm_contact_maintenance_test.go",
			"acceptance/network_mount_control_test.go",
			"acceptance/network_realm_contact_process_test.go",
			"acceptance/realm_online_caller_authentication_test.go",
			"acceptance/realm_permission_binding_administration_test.go",
			"composition/cluster_test.go",
			"composition/connected_connections_test.go",
			"composition/core_connections_test.go",
			"composition/facet_fixture_test.go",
			"composition/facet_reacceptance_chain_test.go",
			"composition/facet_recovery_chain_test.go",
			"composition/facet_replacement_chain_test.go",
			"composition/facet_retirement_chain_test.go",
			"composition/facet_retirement_transfer_test.go",
			"composition/mount_connections_test.go",
			"composition/realm_independent_consumers_test.go",
			"ucupa-core/tests/access/access_test.go",
			"ucupa-core/tests/access/fixture_test.go",
			"ucupa-core/tests/connected/fixture_test.go",
			"ucupa-core/tests/connected/local_test.go",
			"ucupa-core/tests/connected/nested_test.go",
			"ucupa-core/tests/connected/pair_fixture_test.go",
			"ucupa-core/tests/connected/pair_test.go",
			"ucupa-core/tests/connected/attachment_test.go",
			"ucupa-core/tests/element/anchor_range_update_test.go",
			"ucupa-core/tests/element/core_test.go",
			"ucupa-core/tests/element/fixture_test.go",
			"ucupa-core/tests/element/write_convergence_test.go",
			"ucupa-core/tests/instance/durability_test.go",
			"ucupa-core/tests/instance/foundation_test.go",
			"ucupa-core/tests/instance/store_open_test.go",
			"ucupa-core/tests/instance/surface_test.go",
			"ucupa-core/tests/query/attachment_snapshot_test.go",
			"ucupa-core/tests/query/authorization_test.go",
			"ucupa-core/tests/query/depth_test.go",
			"ucupa-core/tests/query/mount_coverage_fixture_test.go",
			"ucupa-core/tests/query/mount_coverage_test.go",
			"ucupa-core/tests/query/persistence_test.go",
			"ucupa-core/tests/query/region_test.go",
			"ucupa-core/tests/query/snapshot_test.go",
			"ucupa-core/tests/query/source_attachment_test.go",
			"ucupa-realm/internal/policy/durability_test.go":
			return true
		}
		return false
	}
	switch file {
	case "acceptance/connected_fixture_test.go",
		"acceptance/connected_lifecycle_test.go",
		"acceptance/connected_query_test.go",
		"acceptance/facet_fixture_test.go",
		"acceptance/facet_local_test.go",
		"acceptance/mount_authenticated_prepare_test.go",
		"acceptance/mount_fixture_test.go",
		"acceptance/network_admission_test.go",
		"acceptance/network_failure_test.go",
		"acceptance/network_federation_nested_fixture_test.go",
		"acceptance/network_real_host_federation_fixture_test.go",
		"acceptance/network_federation_part_test.go",
		"acceptance/network_fixture_test.go",
		"acceptance/writer_boundary_test.go",
		"acceptance/network_mount_approval_test.go",
		"acceptance/network_mount_crash_helper_test.go",
		"acceptance/network_mount_crash_test.go",
		"acceptance/network_mount_product_test.go",
		"acceptance/realm_online_caller_authentication_test.go",
		"acceptance/realm_permission_binding_administration_test.go",
		"cmd/ucupa-core/main.go",
		"cmd/ucupa/connected.go",
		"composition/cluster.go",
		"composition/connected_connections.go",
		"composition/mount_connections.go":
		return true
	}
	return false
}
