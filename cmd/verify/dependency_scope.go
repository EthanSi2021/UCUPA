package main

import (
	"path"
	"strings"
)

func ownerOf(parts []string) string {
	if len(parts) != 0 {
		switch parts[0] {
		case "ucupa-core", "ucupa-facet", "ucupa-realm":
			return parts[0]
		}
	}
	return ""
}

// Register concrete private responsibilities as each reviewed slice lands.
// In particular, an arbitrary path containing internal is not approval.
func reviewedBusinessPackage(owner, directory string) bool {
	return (owner == "ucupa-core" || owner == "ucupa-facet") && directory == owner+"/internal/token"
}

func reviewedStorePackage(owner, directory string) bool {
	return owner == "ucupa-core" && directory == owner+"/internal/store"
}

func reviewedRequestPackage(owner, directory string) bool {
	return owner == "ucupa-facet" && directory == owner+"/internal/namespace/request"
}

func reviewedQueryRequestPackage(owner, directory string) bool {
	return owner == "ucupa-core" && directory == owner+"/internal/query/request"
}

func reviewedQueryPackage(owner, directory string) bool {
	return owner == "ucupa-core" && directory == owner+"/internal/query"
}

func reviewedCoreQueryTestPackage(owner, directory string) bool {
	return owner == "ucupa-core" && directory == owner+"/tests/query"
}

func reviewedCoreInstanceTestPackage(owner, directory string) bool {
	return owner == "ucupa-core" && directory == owner+"/tests/instance"
}

func reviewedCoreInstancePackage(owner, directory string) bool {
	return owner == "ucupa-core" && directory == owner+"/instance"
}

func reviewedCoreInstanceSourceFile(file string) bool {
	switch file {
	case "instance.go", "token.go", "store.go", "contact.go", "maintenance.go", "contact_test.go", "maintenance_test.go":
		return true
	default:
		return false
	}
}

func reviewedCoreAccessEntryPackage(owner, directory string) bool {
	return owner == "ucupa-core" && directory == owner+"/internal/access/entry"
}

func reviewedCoreAccessEntrySourceFile(file string) bool {
	switch file {
	case "operation.go", "request.go", "space.go":
		return true
	default:
		return false
	}
}

func reviewedCoreInstanceTestSourceFile(file string) bool {
	return file == "foundation_test.go" || file == "durability_test.go" || file == "surface_test.go" || file == "store_open_test.go"
}

func reviewedCoreQueryTestSourceFile(file string) bool {
	return file == "snapshot_test.go" || file == "persistence_test.go" || file == "region_test.go" || file == "authorization_test.go" || file == "depth_test.go" || file == "mount_coverage_fixture_test.go" || file == "mount_coverage_test.go" || file == "attachment_snapshot_test.go" || file == "source_attachment_test.go"
}

func reviewedCoreElementTestPackage(owner, directory string) bool {
	return owner == "ucupa-core" && directory == owner+"/tests/element"
}

func reviewedCoreElementTestSourceFile(file string) bool {
	switch file {
	case "core_test.go", "fixture_test.go", "anchor_range_update_test.go", "write_convergence_test.go":
		return true
	default:
		return false
	}
}

func reviewedQuerySourceFile(file string) bool {
	switch file {
	case "config.go", "operation.go", "registry.go", "retained.go", "snapshot_read.go", "view.go",
		"query_entry_test.go", "snapshot_internal_test.go", "export_test.go", "connected_capture_resource_test.go", "connected_resource_test.go", "boundary_test.go":
		return true
	default:
		return false
	}
}

func checkCoreQueryCaptureDependency(file, imported, module string, test bool, stdlib map[string]bool) (bool, string) {
	if !test || path.Dir(file) != "ucupa-core/internal/query" {
		return false, ""
	}
	switch path.Base(file) {
	case "export_test.go":
		if stdlib[imported] {
			return true, ""
		}
		return true, "Core query's test bridge may contain only its fixed value projection and standard-library declarations"
	case "connected_capture_resource_test.go":
		if stdlib[imported] || imported == module+"/shared/values" || imported == module+"/ucupa-core/contract" ||
			imported == module+"/ucupa-core/contract/connected" || imported == module+"/ucupa-core/internal/commit" ||
			imported == module+"/ucupa-core/internal/element" || imported == module+"/ucupa-core/internal/mount" ||
			imported == module+"/ucupa-core/internal/query" || imported == module+"/ucupa-core/internal/query/connected" ||
			imported == module+"/ucupa-core/internal/query/exchange" {
			return true, ""
		}
		return true, "Core query capture-resource tests may use only the exact owner fixture chain, public contracts, shared values and standard library"
	case "connected_resource_test.go":
		if stdlib[imported] || imported == module+"/shared/values" || imported == module+"/ucupa-core/contract" || imported == module+"/ucupa-core/internal/query" {
			return true, ""
		}
		return true, "Core query resource tests may use only query's public contract, shared values, its exact query owner and standard library"
	default:
		return false, ""
	}
}

func reviewedCommitBindingPackage(owner, directory string) bool {
	return owner == "ucupa-core" && directory == owner+"/internal/commit/binding"
}

func reviewedCommitPackage(owner, directory string) bool {
	return owner == "ucupa-core" && directory == owner+"/internal/commit"
}

func reviewedCoreObservationPackage(owner, directory string) bool {
	return owner == "ucupa-core" && directory == owner+"/internal/access/observe"
}

func reviewedAccessValuePackage(owner, directory string) bool {
	return owner == "ucupa-core" && directory == owner+"/internal/access/value"
}

func reviewedAccessValueSourceFile(file string) bool {
	switch file {
	case "object.go", "spatial.go", "entry.go", "record.go", "conditions.go", "value_test.go", "record_test.go":
		return true
	default:
		return false
	}
}

func reviewedQueryScanPackage(owner, directory string) bool {
	return owner == "ucupa-core" && directory == owner+"/internal/query/scan"
}

func reviewedQueryOraclePackage(owner, directory string) bool {
	return owner == "ucupa-core" && directory == owner+"/internal/query/oracle"
}

func reviewedQueryOracleSourceFile(file string) bool {
	switch file {
	case "scan.go", "fixture_test.go", "conformance_test.go", "rotation_test.go", "failure_test.go", "work_test.go":
		return true
	default:
		return false
	}
}

func reviewedCoreConnectedExchangePackage(owner, directory string) bool {
	return owner == "ucupa-core" && directory == owner+"/internal/query/exchange"
}

func reviewedCoreConnectedExchangeSourceFile(file string) bool {
	switch file {
	case "value.go", "cleanup.go", "exchange_test.go":
		return true
	default:
		return false
	}
}

func reviewedCoreConnectedPackage(owner, directory string) bool {
	return owner == "ucupa-core" && directory == owner+"/internal/query/connected"
}

func reviewedCoreConnectedIntegrationTestPackage(owner, directory string) bool {
	return owner == "ucupa-core" && directory == owner+"/tests/connected"
}

func reviewedCoreConnectedIntegrationTestSourceFile(file string) bool {
	switch file {
	case "fixture_test.go", "local_test.go", "pair_fixture_test.go", "pair_test.go", "nested_test.go", "attachment_test.go":
		return true
	default:
		return false
	}
}

func reviewedCoreConnectedSourceFile(file string) bool {
	switch file {
	case "operation.go", "access.go", "lifecycle.go", "pair_boundary.go", "peer.go", "plan.go", "query.go", "read.go", "registry.go", "replan.go", "world.go", "operation_test.go", "peer_test.go", "world_test.go", "filter_test.go", "review_fixture_test.go", "review_test.go", "boundary_test.go":
		return true
	default:
		return false
	}
}

// checkCoreConnectedDependency keeps the connected query, its opaque exchange,
// and its public contract as one reviewed dependency surface.  Keeping this
// policy separate from the repository-wide fallback rules prevents a generic
// same-owner internal allowance from widening the connected boundary.
func checkCoreConnectedDependency(file, imported, module string, test bool, stdlib map[string]bool) (bool, string) {
	source := strings.Split(path.Dir(file), "/")
	sourceOwner := ownerOf(source)

	if reviewedCoreConnectedExchangePackage(sourceOwner, path.Dir(file)) {
		if stdlib[imported] || imported == module+"/shared/values" || imported == module+"/ucupa-core/contract" ||
			imported == module+"/ucupa-core/internal/element/fact" || imported == module+"/ucupa-core/internal/facet/fact" ||
			imported == module+"/ucupa-core/internal/mount/fact" || test && imported == module+"/"+path.Dir(file) {
			return true, ""
		}
		return true, "Core connected exchange owns only opaque trusted-local values and disposal state; it cannot depend on query owners, commit, Store, authorization, transport or SDK"
	}
	if reviewedCoreConnectedPackage(sourceOwner, path.Dir(file)) {
		if test && path.Base(file) == "review_fixture_test.go" {
			if stdlib[imported] || imported == module+"/shared/values" || imported == module+"/ucupa-core/contract" ||
				imported == module+"/ucupa-core/contract/connected" || imported == module+"/ucupa-core/internal/commit" ||
				imported == module+"/ucupa-core/internal/element" || imported == module+"/ucupa-core/internal/mount" ||
				imported == module+"/ucupa-core/internal/query" || imported == module+"/ucupa-core/internal/query/exchange" {
				return true, ""
			}
			return true, "Core connected review fixtures may use only the exact commit, Element, Mount, query and exchange owners plus public contracts, shared values and standard library"
		}
		if test && path.Base(file) == "review_test.go" {
			if stdlib[imported] || imported == module+"/shared/values" || imported == module+"/ucupa-core/contract" || imported == module+"/ucupa-core/internal/query/exchange" {
				return true, ""
			}
			return true, "Core connected review tests may use only the local exchange value, public contracts, shared values and standard library"
		}
		if test && path.Base(file) == "world_test.go" {
			if stdlib[imported] || imported == module+"/shared/values" || imported == module+"/ucupa-core/contract" ||
				imported == module+"/ucupa-core/contract/connected" || imported == module+"/ucupa-core/internal/element/fact" ||
				imported == module+"/ucupa-core/internal/facet/fact" || imported == module+"/ucupa-core/internal/mount/fact" ||
				imported == module+"/ucupa-core/internal/query/exchange" || imported == module+"/ucupa-core/internal/token" ||
				imported == module+"/"+path.Dir(file) {
				return true, ""
			}
			return true, "Core connected world white-box tests may use only shared values, connected contracts, finite fact views, exchange and token"
		}
		if test && path.Base(file) == "filter_test.go" {
			if stdlib[imported] || imported == module+"/shared/values" || imported == module+"/ucupa-core/contract" ||
				imported == module+"/ucupa-core/contract/connected" || imported == module+"/ucupa-core/internal/element/fact" ||
				imported == module+"/ucupa-core/internal/facet/fact" || imported == module+"/ucupa-core/internal/mount/fact" ||
				imported == module+"/ucupa-core/internal/query/exchange" || imported == module+"/ucupa-core/internal/token" ||
				imported == module+"/ucupa-core/internal/commit" || imported == module+"/ucupa-core/internal/element" ||
				imported == module+"/ucupa-core/internal/query" || imported == module+"/"+path.Dir(file) {
				return true, ""
			}
			return true, "Core connected filter white-box tests may use only shared values, connected contracts, finite fact views, exchange, token and the exact commit/Element/query fixture owners"
		}
		if stdlib[imported] || imported == module+"/shared/values" || imported == module+"/ucupa-core/contract" ||
			imported == module+"/ucupa-core/contract/connected" || imported == module+"/ucupa-core/internal/commit" ||
			imported == module+"/ucupa-core/internal/element/fact" || imported == module+"/ucupa-core/internal/facet/fact" ||
			imported == module+"/ucupa-core/internal/geometry" || imported == module+"/ucupa-core/internal/mount/fact" ||
			imported == module+"/ucupa-core/internal/query" || imported == module+"/ucupa-core/internal/query/exchange" ||
			imported == module+"/ucupa-core/internal/query/request" || imported == module+"/ucupa-core/internal/query/scan" ||
			imported == module+"/ucupa-core/internal/token" || test && imported == module+"/"+path.Dir(file) {
			return true, ""
		}
		return true, "Core connected query operations may use only the reviewed local query, commit and finite fact boundaries; no root owner, Store, Element/Facet/Mount operation, transport or SDK dependency"
	}
	if reviewedCoreConnectedIntegrationTestPackage(sourceOwner, path.Dir(file)) {
		if !test {
			return true, "Core connected integration tests are test-only"
		}
		if stdlib[imported] || imported == module+"/shared/values" || imported == module+"/ucupa-core/instance" || imported == module+"/ucupa-core/contract" || imported == module+"/ucupa-core/contract/connected" {
			return true, ""
		}
		return true, "Core connected integration tests may use only standard library, shared values, Core entry points and Core contract"
	}
	if reviewedCoreConnectedContractPackage(sourceOwner, path.Dir(file)) {
		if stdlib[imported] || imported == module+"/shared/values" || imported == module+"/ucupa-core/internal/query/exchange" ||
			imported == module+"/"+path.Dir(file) && test {
			return true, ""
		}
		return true, "Core connected contract owns only the opaque aliases, narrow ports and existing connected error; it cannot depend on owner workflows or other internal implementations"
	}
	if imported == module+"/ucupa-core/contract/connected" && sourceOwner == "ucupa-core" &&
		(len(source) == 1 || path.Dir(file) == "ucupa-core/internal/query/connected") {
		return true, ""
	}
	if imported == module+"/ucupa-core/internal/query/connected" && file == "ucupa-core/instance/instance.go" ||
		imported == module+"/ucupa-core/internal/query/connected" && file == "ucupa-core/instance/store.go" {
		return true, ""
	}
	if imported == module+"/ucupa-core/internal/query/connected" {
		return true, "connected query operations are assembled only by Core core.go and store.go"
	}
	if imported == module+"/ucupa-core/internal/query/exchange" {
		return true, "connected exchange is reachable only from the connected operation owner and its opaque contract alias"
	}
	return false, ""
}

func reviewedCoreConnectedContractPackage(owner, directory string) bool {
	return owner == "ucupa-core" && directory == owner+"/contract/connected"
}

func reviewedCoreConnectedContractSourceFile(file string) bool {
	switch file {
	case "connected.go", "connected_test.go":
		return true
	default:
		return false
	}
}

func reviewedCoreFederationValuePackage(owner, directory string) bool {
	return owner == "ucupa-core" && directory == owner+"/internal/query/federation/value"
}

func reviewedCoreFederationValueSourceFile(file string) bool {
	switch file {
	case "value.go", "observe.go", "value_test.go":
		return true
	default:
		return false
	}
}

func reviewedCoreFederationContractPackage(owner, directory string) bool {
	return owner == "ucupa-core" && directory == owner+"/contract/federation"
}

func reviewedCoreFederationContractSourceFile(file string) bool {
	switch file {
	case "federation.go", "federation_test.go":
		return true
	default:
		return false
	}
}

func reviewedCoreFederationProjectionPackage(owner, directory string) bool {
	return owner == "ucupa-core" && directory == owner+"/internal/query/federation/projection"
}

func reviewedCoreFederationProjectionSourceFile(file string) bool {
	switch file {
	case "projection.go", "path.go", "record.go", "query.go", "traversal.go", "relation.go", "failure.go",
		"traversal_test.go", "failure_test.go", "projection_test.go", "query_test.go":
		return true
	default:
		return false
	}
}

func reviewedCoreFederationTranscriptPackage(owner, directory string) bool {
	return owner == "ucupa-core" && directory == owner+"/internal/query/federation/transcript"
}

func reviewedCoreFederationTranscriptSourceFile(file string) bool {
	switch file {
	case "transcript.go", "transcript_test.go":
		return true
	default:
		return false
	}
}

func reviewedGeometryPackage(owner, directory string) bool {
	return owner == "ucupa-core" && directory == owner+"/internal/geometry"
}

func reviewedElementFactPackage(owner, directory string) bool {
	return owner == "ucupa-core" && directory == owner+"/internal/element/fact"
}

func reviewedElementOperationPackage(owner, directory string) bool {
	return owner == "ucupa-core" && directory == owner+"/internal/element"
}

func reviewedElementOperationSourceFile(file string) bool {
	switch file {
	case "operation.go", "operation_test.go", "read.go", "projection.go", "public.go", "read_test.go", "projection_test.go":
		return true
	default:
		return false
	}
}

func reviewedElementReadPackage(owner, directory string) bool {
	return owner == "ucupa-core" && directory == owner+"/internal/element/read"
}

func reviewedElementReadSourceFile(file string) bool {
	switch file {
	case "element.go", "direct.go", "adjacency_test.go":
		return true
	default:
		return false
	}
}

func reviewedCoreAccessPackage(owner, directory string) bool {
	return owner == "ucupa-core" && directory == owner+"/internal/access"
}

func reviewedCoreAccessSourceFile(file string) bool {
	switch file {
	case "admission.go", "admission_test.go", "permit.go":
		return true
	default:
		return false
	}
}

func reviewedCoreFacetPackage(owner, directory string) bool {
	return owner == "ucupa-core" && directory == owner+"/internal/facet"
}

func reviewedCoreFacetSourceFile(file string) bool {
	switch file {
	case "operation.go", "control.go", "read.go", "public_control.go", "service_attachment.go", "service_proposal.go", "operation_test.go", "public_control_test.go", "release_control_test.go", "service_fixture_test.go", "service_test.go", "boundary_test.go":
		return true
	default:
		return false
	}
}

func reviewedCoreMountOperationPackage(owner, directory string) bool {
	return owner == "ucupa-core" && directory == owner+"/internal/mount"
}

func reviewedCoreMountOperationSourceFile(file string) bool {
	switch file {
	case "operation.go", "control.go", "address.go", "recovery.go", "peer.go", "peer_types.go", "commitment.go", "public_authorization.go", "public_command.go", "public_contact.go", "public_control.go", "public_prepare.go", "authorization.go", "operation_test.go", "public_control_test.go", "public_prepare_test.go", "public_command_test.go", "public_contact_test.go", "commitment_test.go", "boundary_test.go":
		return true
	default:
		return false
	}
}

func reviewedFacetFactPackage(owner, directory string) bool {
	return owner == "ucupa-core" && directory == owner+"/internal/facet/fact"
}

func reviewedMountFactPackage(owner, directory string) bool {
	return owner == "ucupa-core" && directory == owner+"/internal/mount/fact"
}

func reviewedMountFactSourceFile(file string) bool {
	switch file {
	case "read.go", "record.go", "state.go", "state_test.go", "validation.go", "validation_test.go", "view.go", "view_test.go":
		return true
	default:
		return false
	}
}

func reviewedFailurePackage(owner, directory string) bool {
	return owner == "ucupa-core" && directory == owner+"/internal/failure"
}

func reviewedAccessPackage(owner, directory string) bool {
	return owner == "ucupa-facet" && directory == owner+"/internal/access"
}

func reviewedNamespacePackage(owner, directory string) bool {
	return owner == "ucupa-facet" && directory == owner+"/internal/namespace"
}

func reviewedInstancePackage(owner, directory string) bool {
	return owner == "ucupa-facet" && directory == owner+"/instance"
}

func reviewedInternalPackage(owner, directory string) bool {
	return reviewedBusinessPackage(owner, directory) || reviewedStorePackage(owner, directory) || reviewedRequestPackage(owner, directory) || reviewedGeometryPackage(owner, directory) || reviewedFailurePackage(owner, directory) || reviewedAccessPackage(owner, directory) || reviewedNamespacePackage(owner, directory) || reviewedElementFactPackage(owner, directory) || reviewedElementOperationPackage(owner, directory) || reviewedElementReadPackage(owner, directory) || reviewedCoreAccessPackage(owner, directory) || reviewedCoreAccessEntryPackage(owner, directory) || reviewedCoreFacetPackage(owner, directory) || reviewedCoreMountOperationPackage(owner, directory) || reviewedFacetFactPackage(owner, directory) || reviewedMountFactPackage(owner, directory) || reviewedQueryRequestPackage(owner, directory) || reviewedQueryPackage(owner, directory) || reviewedCoreQueryLocalPackage(owner, directory) || reviewedQueryScanPackage(owner, directory) || reviewedQueryOraclePackage(owner, directory) || reviewedCoreConnectedExchangePackage(owner, directory) || reviewedCoreConnectedPackage(owner, directory) || reviewedCommitBindingPackage(owner, directory) || reviewedCommitPackage(owner, directory) || reviewedAccessValuePackage(owner, directory) || reviewedCoreObservationPackage(owner, directory) || reviewedCoreFederationValuePackage(owner, directory) || reviewedCoreFederationProjectionPackage(owner, directory) || reviewedCoreFederationTranscriptPackage(owner, directory) || reviewedCoreFederationCollectPackage(owner, directory) || reviewedCoreFederationDenialPackage(owner, directory) || reviewedCoreAccessRegistryPackage(owner, directory) || reviewedCoreAccessObservationPackage(owner, directory) || reviewedCoreFederationSourcePackage(owner, directory)
}

func reviewedContractPackage(owner, directory string) bool {
	return (owner == "ucupa-core" || owner == "ucupa-facet") && directory == owner+"/contract"
}

func contains(parts []string, wanted string) bool {
	for _, part := range parts {
		if part == wanted {
			return true
		}
	}
	return false
}
