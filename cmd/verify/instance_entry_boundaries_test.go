package main

import (
	"strings"
	"testing"
)

func TestCoreInstanceAndEntrySourcesHaveExactLocationsAndImports(t *testing.T) {
	for _, file := range []string{
		"ucupa-core/instance/instance.go",
		"ucupa-core/instance/token.go",
		"ucupa-core/instance/store.go",
		"ucupa-core/instance/contact.go",
		"ucupa-core/instance/maintenance.go",
		"ucupa-core/instance/contact_test.go",
		"ucupa-core/instance/maintenance_test.go",
		"ucupa-core/internal/access/entry/operation.go",
		"ucupa-core/internal/access/entry/request.go",
		"ucupa-core/internal/access/entry/space.go",
	} {
		if reason := checkSourceLocation(file); reason != "" {
			t.Errorf("reviewed instance/entry source rejected: %s: %s", file, reason)
		}
		assertActualImportsAccepted(t, file)
	}
	for _, file := range []string{
		"ucupa-core/instance/extra.go",
		"ucupa-core/instance/instance_test.go",
		"ucupa-core/instance/nested/instance.go",
		"ucupa-core/internal/access/entry/extra.go",
		"ucupa-core/internal/access/entry/operation_test.go",
		"ucupa-core/internal/access/entry/nested/request.go",
	} {
		if reason := checkSourceLocation(file); reason == "" {
			t.Errorf("unreviewed instance/entry source accepted: %s", file)
		}
	}
}

func TestCoreInstanceAndEntryDependenciesArePerFileAndOneWay(t *testing.T) {
	stdlib := guardOracleStdlib()
	for file, dependencies := range coreInstanceDependencies {
		for _, dependency := range dependencies {
			path := coreInstanceDirectory + "/" + file
			if got := checkDependency(path, moduleImport("ucupa", dependency), "ucupa", false, stdlib); got != "" {
				t.Errorf("allowed Core instance dependency %s -> %s: %s", file, dependency, got)
			}
		}
	}
	for file, dependencies := range coreAccessEntryDependencies {
		for _, dependency := range dependencies {
			path := coreAccessEntryDirectory + "/" + file
			if got := checkDependency(path, moduleImport("ucupa", dependency), "ucupa", false, stdlib); got != "" {
				t.Errorf("allowed Core public entry dependency %s -> %s: %s", file, dependency, got)
			}
		}
	}

	for file, dependencies := range map[string][]string{
		"instance.go": {
			"ucupa-core", "ucupa-core/internal/store", "ucupa-core/internal/element/fact",
			"ucupa-core/internal/query/scan", "ucupa-core/internal/token", "ucupa-core/transport/grpc",
			"ucupa-core/sdk/go", "ucupa-facet/instance", "go.etcd.io/bbolt",
		},
		"token.go": {
			"shared/values", "ucupa-core/contract", "ucupa-core/internal/commit", "ucupa-core/instance",
		},
		"store.go": {
			"ucupa-core", "ucupa-core/internal/store", "ucupa-core/internal/element",
			"ucupa-core/internal/mount", "ucupa-core/internal/query/local", "ucupa-core/transport/grpc",
			"ucupa-core/sdk/go", "go.etcd.io/bbolt",
		},
	} {
		for _, dependency := range dependencies {
			path := coreInstanceDirectory + "/" + file
			if got := checkDependency(path, moduleImport("ucupa", dependency), "ucupa", false, stdlib); got == "" {
				t.Errorf("forbidden Core instance dependency accepted: %s -> %s", file, dependency)
			}
		}
	}
	for file, dependencies := range map[string][]string{
		"operation.go": {
			"ucupa-core", "ucupa-core/internal/store", "ucupa-core/internal/element/fact",
			"ucupa-core/internal/facet", "ucupa-core/internal/mount", "ucupa-core/internal/query",
			"ucupa-core/internal/query/scan", "ucupa-core/transport/grpc", "ucupa-core/sdk/go",
			"go.etcd.io/bbolt",
		},
		"request.go": {
			"ucupa-core", "ucupa-core/internal/commit", "ucupa-core/internal/element",
			"ucupa-core/internal/access/registry", "ucupa-core/internal/query/local",
			"ucupa-core/internal/mount", "ucupa-core/internal/store", "ucupa-core/transport/grpc",
			"ucupa-core/sdk/go", "go.etcd.io/bbolt",
		},
		"space.go": {
			"ucupa-core", "ucupa-core/internal/commit", "ucupa-core/internal/element",
			"ucupa-core/internal/access/registry", "ucupa-core/internal/query", "ucupa-core/internal/store",
			"ucupa-core/transport/grpc", "ucupa-core/sdk/go", "go.etcd.io/bbolt",
		},
	} {
		for _, dependency := range dependencies {
			path := coreAccessEntryDirectory + "/" + file
			if got := checkDependency(path, moduleImport("ucupa", dependency), "ucupa", false, stdlib); got == "" {
				t.Errorf("forbidden Core public entry dependency accepted: %s -> %s", file, dependency)
			}
		}
	}
}

func TestCoreInstanceAndEntryConsumersAreExact(t *testing.T) {
	stdlib := guardOracleStdlib()
	instanceImport := moduleImport("ucupa", coreInstanceDirectory)
	for _, file := range []string{
		"acceptance/connected_fixture_test.go",
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
		"composition/cluster_test.go",
		"composition/connected_connections.go",
		"composition/connected_connections_test.go",
		"composition/core_connections_test.go",
		"composition/facet_fixture_test.go",
		"composition/mount_connections.go",
		"composition/mount_connections_test.go",
		"composition/realm_independent_consumers_test.go",
		"ucupa-core/tests/access/access_test.go",
		"ucupa-core/tests/access/fixture_test.go",
		"ucupa-core/tests/connected/fixture_test.go",
		"ucupa-core/tests/connected/nested_test.go",
		"ucupa-core/tests/connected/pair_fixture_test.go",
		"ucupa-core/tests/element/anchor_range_update_test.go",
		"ucupa-core/tests/element/core_test.go",
		"ucupa-core/tests/element/fixture_test.go",
		"ucupa-core/tests/element/write_convergence_test.go",
		"ucupa-core/tests/instance/durability_test.go",
		"ucupa-core/tests/instance/foundation_test.go",
		"ucupa-core/tests/instance/store_open_test.go",
		"ucupa-core/tests/query/attachment_snapshot_test.go",
		"ucupa-core/tests/query/authorization_test.go",
		"ucupa-core/tests/query/depth_test.go",
		"ucupa-core/tests/query/mount_coverage_fixture_test.go",
		"ucupa-core/tests/query/mount_coverage_test.go",
		"ucupa-core/tests/query/persistence_test.go",
		"ucupa-core/tests/query/region_test.go",
		"ucupa-core/tests/query/snapshot_test.go",
		"ucupa-realm/internal/policy/durability_test.go",
	} {
		isTest := strings.HasSuffix(file, "_test.go")
		if got := checkDependency(file, instanceImport, "ucupa", isTest, stdlib); got != "" {
			t.Errorf("approved Core instance consumer rejected: %s: %s", file, got)
		}
	}
	for _, file := range []string{
		"ucupa-core/core.go", "ucupa-core/public_context.go", "ucupa-core/public_entry.go",
		"ucupa-core/store.go", "ucupa-core/internal/commit/owner.go", "ucupa-core/internal/query/operation.go",
		"ucupa-core/internal/access/entry/request.go", "ucupa-core/internal/element/operation.go",
		"ucupa-core/internal/mount/operation.go", "ucupa-core/internal/facet/operation.go",
		"ucupa-core/transport/grpc/server.go", "ucupa-core/sdk/go/client.go", "composition/ports.go",
		"acceptance/unused_test.go", "ucupa-core/tests/query/extra_test.go", "ucupa-core/tests/instance/extra_test.go",
	} {
		if got := checkDependency(file, instanceImport, "ucupa", strings.HasSuffix(file, "_test.go"), stdlib); got == "" {
			t.Errorf("unapproved Core instance consumer accepted: %s", file)
		}
	}

	entryImport := moduleImport("ucupa", coreAccessEntryDirectory)
	if got := checkDependency("ucupa-core/instance/instance.go", entryImport, "ucupa", false, stdlib); got != "" {
		t.Fatalf("instance assembler should construct public entry: %s", got)
	}
	for _, file := range []string{
		"ucupa-core/core.go", "ucupa-core/store.go", "ucupa-core/public_context.go",
		"ucupa-core/public_entry.go", "ucupa-core/internal/access/entry/request.go",
		"ucupa-core/internal/query/operation.go", "ucupa-core/internal/commit/owner.go",
		"ucupa-core/transport/grpc/server.go", "ucupa-core/sdk/go/client.go",
	} {
		if got := checkDependency(file, entryImport, "ucupa", strings.HasSuffix(file, "_test.go"), stdlib); got == "" {
			t.Errorf("unapproved public entry consumer accepted: %s", file)
		}
	}
}

func TestCoreInstanceAndEntryExportSurfacesAreExact(t *testing.T) {
	for _, tc := range []struct {
		file      string
		types     map[string]bool
		functions map[string]bool
		methods   map[string]map[string]bool
		fields    map[string][][2]string
	}{
		{
			file:      "ucupa-core/instance/instance.go",
			types:     map[string]bool{"Core": true},
			functions: map[string]bool{"New": true},
			methods: map[string]map[string]bool{"*Core": {
				"AuthorityRef": true, "Elements": true, "Queries": true, "Facets": true,
				"Mounts": true, "Connected": true, "Federation": true, "Access": true, "Writer": true, "Transfers": true,
				"RealmContact": true, "CurrentRealmContact": true,
			}},
			fields: map[string][][2]string{"Core": {
				{"owner", "*commitpkg.Owner"},
				{"authority", "values.FactAuthorityRef"},
				{"contact", "contract.RealmConnections"},
				{"elements", "*element.Operations"},
				{"mounts", "*mountpkg.Operations"},
				{"queries", "*querypkg.Operations"},
				{"connected", "*connectedpkg.Operations"},
				{"facets", "*facetpkg.Operations"},
				{"sources", "*federationsource.Operations"},
				{"access", "*accessentry.Operations"},
				{"resources", "*accessregistry.Registry"},
				{"writer", "*handoff.Operations"},
				{"transfers", "*transfer.Operations"},
				{"realm", "*realmcontact.Operations"},
			}},
		},
		{file: "ucupa-core/instance/token.go"},
		{file: "ucupa-core/instance/contact.go", methods: map[string]map[string]bool{"*contactAuthorizer": {
			"AuthorizePrincipal": true, "AuthorizeView": true, "ValidateView": true,
			"CaptureDenialView": true, "ValidateDenialView": true,
			"ReleaseRealmEvidence": true, // Resource-only cleanup on the private adapter, not a public Core method.
		}}},
		{file: "ucupa-core/instance/store.go", functions: map[string]bool{"CreateDurable": true, "OpenDurable": true}, methods: map[string]map[string]bool{"*Core": {"Close": true}}},
		{
			file:      "ucupa-core/instance/maintenance.go",
			types:     map[string]bool{"RealmMaintenance": true},
			functions: map[string]bool{"OpenRealmMaintenance": true},
			methods:   map[string]map[string]bool{"*RealmMaintenance": {"Inspect": true, "Update": true, "Recover": true, "Close": true}},
			fields: map[string][][2]string{"RealmMaintenance": {
				{"owner", "*commitpkg.Owner"},
				{"realm", "*realmcontact.Operations"},
			}},
		},
		{
			file:      "ucupa-core/internal/access/entry/operation.go",
			types:     map[string]bool{"Operations": true},
			functions: map[string]bool{"New": true},
			methods:   map[string]map[string]bool{"*Operations": {"AuthorityRef": true, "CurrentRealmContact": true}},
			fields: map[string][][2]string{"Operations": {
				{"owner", "*commit.Owner"},
				{"admission", "*access.Admission"},
				{"access", "*registry.Registry"},
				{"local", "*local.Operations"},
				{"federation", "*coordinator.Operations"},
				{"mutations", "*element.PublicOperations"},
			}},
		},
		{file: "ucupa-core/internal/access/entry/request.go", methods: map[string]map[string]bool{"*Operations": {"PublicCall": true, "PublicCallWithFederation": true}}},
		{file: "ucupa-core/internal/access/entry/space.go"},
	} {
		parsed := parseCurrentCoreBoundarySource(t, tc.file)
		assertExactPublicSurface(t, tc.file, parsed, tc.types, tc.functions, tc.methods)
		for typeName, fields := range tc.fields {
			assertExactPrivateFields(t, tc.file, parsed, typeName, fields)
		}
	}
}
