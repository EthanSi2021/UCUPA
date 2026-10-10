package main

import (
	"path"
	"strings"
)

func checkCoreRealmContactImport(file, imported, module string, test bool, _ map[string]bool) (bool, string) {
	if imported != module+"/ucupa-core/internal/realmcontact" {
		return false, ""
	}
	if path.Dir(file) == "ucupa-core/instance" || test && path.Dir(file) == "ucupa-core/internal/realmcontact" {
		return true, ""
	}
	return true, "Core Realm contact control is assembled only by Core instance"
}

// The commit owner is registered by its current, reviewed source set rather
// than by an arbitrary wildcard. A new file needs an explicit responsibility
// review before the location checker accepts it.
func reviewedCommitSourceFile(file string) bool {
	switch file {
	case "writer.go", "writer_record.go", "writer_store.go", "writer_test.go":
		return true
	case "element.go",
		"public_durability_fixture_test.go",
		"public_durability_test.go",
		"public_lookup_test.go",
		"facet_history.go",
		"facet_state.go",
		"facet_store.go",
		"facet_transition.go",
		"foundation_candidate_fixture_test.go",
		"foundation_candidate_test.go",
		"foundation_fault_export_test.go",
		"foundation_history_test.go",
		"foundation_replaced_test.go",
		"foundation_store_fixture_test.go",
		"foundation_store_test.go",
		"history.go",
		"mount_address.go",
		"mount_guard.go",
		"mount_guard_fixture_test.go",
		"mount_guard_proof_export_test.go",
		"mount_guard_read_test.go",
		"mount_guard_validation_test.go",
		"mount_guard_write_test.go",
		"mount_history.go",
		"mount_state.go",
		"mount_store_commit_test.go",
		"mount_store_event.go",
		"mount_transition.go",
		"mount_transition_peer.go",
		"mount_validation.go",
		"anchor_range_fault_export_test.go",
		"anchor_range_fixture_test.go",
		"anchor_range_store_test.go",
		"mount_store_atomicity_test.go",
		"mount_store_corruption_export_test.go",
		"mount_store_corruption_test.go",
		"mount_store_fault_export_test.go",
		"mount_store_fixture_test.go",
		"mount_store_history_export_test.go",
		"mount_store_outcomes_test.go",
		"mount_control_export_test.go",
		"mount_control_fixture_test.go",
		"mount_control_test.go",
		"mount_interleaving_test.go",
		"mount_nested_fixture_test.go",
		"mount_nested_test.go",
		"query_conformance_test.go",
		"query_corruption_test.go",
		"query_durability_test.go",
		"query_fault_export_test.go",
		"query_fixture_export_test.go",
		"query_fixture_test.go",
		"query_limits_test.go",
		"store_corruption_export_test.go",
		"store_corruption_test.go",
		"store_schema_test.go",
		"element_store_test.go",
		"export_test.go",
		"facet_fault_export_test.go",
		"facet_lifecycle_fixture_test.go",
		"facet_lifecycle_test.go",
		"facet_recovery_test.go",
		"facet_release_test.go",
		"projection_authorization_test.go",
		"projection_fault_export_test.go",
		"oracle_boundary_test.go",
		"oracle_fault_export_test.go",
		"owner.go",
		"owner_publication_test.go",
		"public_element.go",
		"public_facet.go",
		"public_mount.go",
		"read_view.go",
		"realm_contact.go",
		"realm_contact_store.go",
		"realm_contact_store_test.go",
		"realm_contact_test.go",
		"state.go",
		"store.go",
		"store_codec.go",
		"store_load.go":
		return true
	default:
		return false
	}
}

func checkSourceLocation(file string) string {
	if checked, reason := realmLocation(file); checked {
		return reason
	}
	if checked, reason := transferLocation(file); checked {
		return reason
	}
	if checked, reason := writerLocation(file); checked {
		return reason
	}
	if path.Dir(file) == coreEntryRPCDirectory {
		if reviewedCoreEntryRPCFile(path.Base(file)) {
			return ""
		}
		return "Core protocol entry source requires exact responsibility registration"
	}
	if file == "ucupa-core/transport/grpc/methods.go" || file == "ucupa-core/transport/grpc/federation_public.go" {
		return "Core RPC and caller adapter have left the network host; no retired entry or root forwarding"
	}
	if path.Dir(file) == coreAdmissionDirectory {
		if reviewedCoreAdmissionFile(path.Base(file)) {
			return ""
		}
		return "shared transport admission source requires exact responsibility registration"
	}
	if path.Dir(file) == "ucupa-core/transport/grpc" && reviewedCoreAdmissionFile(path.Base(file)) && path.Base(file) != "service.go" {
		return "shared admission implementation and owner-level tests have left the transport root"
	}
	if path.Dir(file) == coreMountRPCDirectory {
		if reviewedCoreMountRPCFile(path.Base(file)) {
			return ""
		}
		return "Core application Mount RPC source must belong to its reviewed complete control/contact group"
	}
	if file == "ucupa-core/transport/grpc/mount_control.go" || file == "ucupa-core/transport/grpc/mount_contact.go" {
		return "application Mount RPC implementation has moved; no old root handler or forwarding entry"
	}
	if path.Dir(file) == "ucupa-core/tests/federation" {
		if _, reviewed := coreFederationScenarioDependencies[path.Base(file)]; reviewed {
			return ""
		}
		return "Core public Federation scenarios are limited to the reviewed real-assembly fixture and behavior files"
	}
	if checked, reason := checkCoreCoordinatorLocation(file); checked {
		return reason
	}
	if reviewedCoreTransportPackage(path.Dir(file)) {
		return ""
	}
	if path.Dir(file) == "ucupa-core" || path.Dir(file) == "ucupa-facet" {
		return "Core/Facet module root is navigation only; no production source, tests or compatibility facade"
	}
	switch path.Dir(file) {
	case "shared/bindings/go/protobuf", "ucupa-core/bindings/go/protobuf", "ucupa-facet/bindings/go/protobuf", "ucupa-realm/bindings/go/protobuf":
		if generatedSource(file) {
			return ""
		}
		return "only registered generated bindings may live here"
	case "ucupa-core/transport/grpc", "ucupa-core/sdk/go", "ucupa-facet/transport/grpc", "ucupa-facet/sdk/go", "ucupa-realm/transport/grpc", "ucupa-realm/sdk/go", "internal/protovalue", "internal/grpcbound", "internal/grpcdial":
		return ""
	}
	parts := strings.Split(path.Dir(file), "/")
	if owner := ownerOf(parts); owner != "" {
		directory := path.Dir(file)
		if reviewedCoreConnectedExchangePackage(owner, directory) && !reviewedCoreConnectedExchangeSourceFile(path.Base(file)) {
			return "Core connected exchange source is not in the reviewed opaque-envelope file set"
		}
		if reviewedCoreConnectedPackage(owner, directory) && !reviewedCoreConnectedSourceFile(path.Base(file)) {
			return "Core connected query source is not in the reviewed owner file set"
		}
		if reviewedCoreConnectedIntegrationTestPackage(owner, directory) && !reviewedCoreConnectedIntegrationTestSourceFile(path.Base(file)) {
			return "Core connected integration tests are limited to the reviewed fixture and local behavior files"
		}
		if reviewedCoreConnectedContractPackage(owner, directory) && !reviewedCoreConnectedContractSourceFile(path.Base(file)) {
			return "Core connected contract source is not in the reviewed port and opaque-value file set"
		}
		if reviewedCoreFederationValuePackage(owner, directory) && !reviewedCoreFederationValueSourceFile(path.Base(file)) {
			return "Core Federation value source is not in the reviewed detached-value file set"
		}
		if reviewedCoreFederationContractPackage(owner, directory) && !reviewedCoreFederationContractSourceFile(path.Base(file)) {
			return "Core Federation contract source is not in the reviewed value-alias and port file set"
		}
		if reviewedCoreFederationProjectionPackage(owner, directory) && !reviewedCoreFederationProjectionSourceFile(path.Base(file)) {
			return "Core Federation projection source is not in the reviewed projection implementation and test file set"
		}
		if reviewedCoreFederationTranscriptPackage(owner, directory) && !reviewedCoreFederationTranscriptSourceFile(path.Base(file)) {
			return "Core Federation transcript source is not in the reviewed framing implementation and test file set"
		}
		if reviewedCoreFederationCollectPackage(owner, directory) && !reviewedCoreFederationCollectSourceFile(path.Base(file)) {
			return "Core Federation collect source is not in the reviewed finite-read implementation and test file set"
		}
		if reviewedCoreFederationDenialPackage(owner, directory) && !reviewedCoreFederationDenialSourceFile(path.Base(file)) {
			return "Core Federation denial source is not in the reviewed opaque-evidence implementation and test file set"
		}
		if reviewedCoreAccessRegistryPackage(owner, directory) && !reviewedCoreAccessRegistrySourceFile(path.Base(file)) {
			return "Core access registry source is not in the reviewed registry lifecycle file set"
		}
		if reviewedCoreAccessObservationPackage(owner, directory) && !reviewedCoreAccessObservationSourceFile(path.Base(file)) {
			return "Core access observation source is not in the reviewed observation operation file set"
		}
		if reviewedCoreFederationSourcePackage(owner, directory) && !reviewedCoreFederationSourceFile(path.Base(file)) {
			return "Core Federation source is not in the reviewed source operation file set"
		}
		if reviewedCoreQueryLocalPackage(owner, directory) && !reviewedCoreQueryLocalSourceFile(path.Base(file)) {
			return "Core local query source is not in the reviewed five-file read operation set"
		}
		if reviewedCoreInstancePackage(owner, directory) && !reviewedCoreInstanceSourceFile(path.Base(file)) {
			return "Core instance source is not in the reviewed assembly, lifecycle and Realm contact file set"
		}
		if reviewedCoreAccessEntryPackage(owner, directory) && !reviewedCoreAccessEntrySourceFile(path.Base(file)) {
			return "Core public entry source is not in the reviewed operation, request or space file set"
		}
		if reviewedCoreAccessIntegrationTestPackage(owner, directory) && !reviewedCoreAccessIntegrationTestSourceFile(path.Base(file)) {
			return "Core public access scenarios are limited to the reviewed access, boundary and fixture tests"
		}
		if reviewedStorePackage(owner, directory) && !reviewedStoreSourceFile(path.Base(file)) {
			return "Core Store source is not in the reviewed format, codec or transaction file set"
		}
		if reviewedQueryOraclePackage(owner, directory) && !reviewedQueryOracleSourceFile(path.Base(file)) {
			return "Core query oracle source is not in the reviewed conformance implementation and test file set"
		}
		if reviewedMountFactPackage(owner, directory) && !reviewedMountFactSourceFile(path.Base(file)) {
			return "Core mount fact source is not in the reviewed current-fact and validation file set"
		}
		if reviewedCommitPackage(owner, directory) && !reviewedCommitSourceFile(path.Base(file)) {
			return "Core commit source is not in the reviewed owner file set"
		}
		if owner == "ucupa-core" && directory == "ucupa-core/internal/realmcontact" &&
			path.Base(file) != "control.go" && path.Base(file) != "control_test.go" {
			return "Core Realm contact control contains only the reviewed owner-local workflow and its tests"
		}
		if reviewedElementOperationPackage(owner, directory) && !reviewedElementOperationSourceFile(path.Base(file)) {
			return "Core Element operation source is not in the reviewed operation file set"
		}
		if reviewedElementReadPackage(owner, directory) && !reviewedElementReadSourceFile(path.Base(file)) {
			return "Core Element read source is not in the reviewed read file set"
		}
		if reviewedCoreAccessPackage(owner, directory) && !reviewedCoreAccessSourceFile(path.Base(file)) {
			return "Core access source is not in the reviewed access file set"
		}
		if reviewedAccessValuePackage(owner, directory) && !reviewedAccessValueSourceFile(path.Base(file)) {
			return "Core access value source is not in the reviewed representation file set"
		}
		if reviewedCoreFacetPackage(owner, directory) && !reviewedCoreFacetSourceFile(path.Base(file)) {
			return "Core Facet operation source is not in the reviewed operation file set"
		}
		if reviewedCoreMountOperationPackage(owner, directory) && !reviewedCoreMountOperationSourceFile(path.Base(file)) {
			return "Core mount operation source is not in the reviewed operation file set"
		}
		if reviewedQueryPackage(owner, directory) && !reviewedQuerySourceFile(path.Base(file)) {
			return "Core query source is not in the reviewed owner file set"
		}
		if reviewedCoreQueryTestPackage(owner, directory) && !reviewedCoreQueryTestSourceFile(path.Base(file)) {
			return "Core query integration tests are limited to the reviewed query, authorization, depth, snapshot and persistence test files"
		}
		if reviewedCoreInstanceTestPackage(owner, directory) && !reviewedCoreInstanceTestSourceFile(path.Base(file)) {
			return "Core instance integration tests are limited to the reviewed identity and durable lifecycle scenarios"
		}
		if reviewedCoreElementTestPackage(owner, directory) && !reviewedCoreElementTestSourceFile(path.Base(file)) {
			return "Core Element integration tests are limited to the reviewed Element test files"
		}
		if len(parts) == 1 || reviewedInternalPackage(owner, directory) || owner == "ucupa-core" && directory == "ucupa-core/internal/realmcontact" || reviewedContractPackage(owner, directory) || reviewedCoreConnectedContractPackage(owner, directory) || reviewedCoreFederationContractPackage(owner, directory) || reviewedCoreInstancePackage(owner, directory) || reviewedInstancePackage(owner, directory) {
			return ""
		}
		if reviewedCoreQueryTestPackage(owner, directory) {
			return ""
		}
		if reviewedCoreConnectedIntegrationTestPackage(owner, directory) {
			return ""
		}
		if reviewedCoreInstanceTestPackage(owner, directory) {
			return ""
		}
		if reviewedCoreElementTestPackage(owner, directory) {
			return ""
		}
		if reviewedCoreAccessIntegrationTestPackage(owner, directory) {
			return ""
		}
		return "owner source location is not approved; register its concrete responsibility and dependency edges"
	}
	switch path.Dir(file) {
	case "shared/values", "composition", "acceptance":
		return ""
	}
	if len(parts) == 2 && parts[0] == "cmd" {
		return ""
	}
	return "Go source location is not approved; review its responsibility before adding a new package"
}
