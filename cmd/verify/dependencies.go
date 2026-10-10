package main

import (
	"path"
	"strings"
)

func checkDependency(file, imported, module string, test bool, stdlib map[string]bool) string {
	source := strings.Split(path.Dir(file), "/")
	sourceOwner := ownerOf(source)
	// Order matters: reviewed source edges precede exact consumer restrictions.
	policies := []func(string, string, string, bool, map[string]bool) (bool, string){
		checkRealmDependency,
		checkCoreRealmContactImport,
		checkTransferDependency,
		checkWriterDependency,
		checkPublicResourceTestDependency,
		checkCoreFederationScenarioDependency,
		checkCoreCollectActualTestDependency,
		checkCoreCommitFoundationDependency,
		checkCoreInstanceEntryDependency,
		checkCoreLocalPublicDependency,
		checkCoreCoordinatorDependency,
		checkCoreFederationSourceDependency,
		checkCoreAccessObservationDependency,
		checkCoreCommitOracleDependency,
		checkCoreCommitProjectionDependency,
		checkCoreCommitMountGuardDependency,
		checkCoreQueryOracleDependency,
		checkCoreQueryMountCoverageDependency,
		checkCoreQueryCaptureDependency,
		checkCoreCommitFacetDependency,
		checkCoreCommitAnchorRangeDependency,
		checkCoreFederationCollectDependency,
		checkCoreCurrentBoundaryDependency,
	}
	for _, check := range policies {
		if checked, reason := check(file, imported, module, test, stdlib); checked {
			return reason
		}
	}
	if imported == module+"/ucupa-core/tests/instance" {
		return "Core instance scenarios are test-only, not a shared fixture library"
	}
	if imported == module+"/ucupa-core/tests/connected" {
		return "Core connected integration tests are test-only behavior scenarios, not a shared fixture library"
	}
	if reviewedCoreInstanceTestPackage(sourceOwner, path.Dir(file)) {
		if test && (stdlib[imported] || imported == module+"/shared/values" || imported == module+"/ucupa-core/instance" || imported == module+"/ucupa-core/contract") {
			return ""
		}
		return "Core instance scenarios use only public assembly and value contracts, not internal owners, raw Store, transport or another test package"
	}
	if reviewedCoreFederationValuePackage(sourceOwner, path.Dir(file)) {
		if stdlib[imported] || imported == module+"/shared/values" || imported == module+"/ucupa-core/contract" || imported == module+"/ucupa-core/internal/geometry" || test && imported == module+"/"+path.Dir(file) {
			return ""
		}
		return "Core Federation detached values own only public field shapes, range construction and opaque traversal storage; no query owner, commit, Store, transport, SDK or other internal implementation dependency"
	}
	if reviewedCoreFederationContractPackage(sourceOwner, path.Dir(file)) {
		if stdlib[imported] || imported == module+"/shared/values" || imported == module+"/ucupa-core/contract" || imported == module+"/ucupa-core/internal/query/federation/value" || test && imported == module+"/"+path.Dir(file) {
			return ""
		}
		return "Core Federation contract owns only aliases of the detached values and the two narrow source ports; it cannot depend on Core workflows, Store, transport, SDK or other owners"
	}
	if reviewedCoreFederationProjectionPackage(sourceOwner, path.Dir(file)) {
		base := path.Base(file)
		if !test && imported == module+"/ucupa-core/internal/query/federation/value" &&
			base != "path.go" && base != "query.go" && base != "record.go" && base != "relation.go" {
			return "Federation detached values are consumed by only the reviewed projection producers"
		}
		if !test && imported == module+"/ucupa-core/internal/query/federation/transcript" && base != "traversal.go" {
			return "Federation transcript framing is consumed only by projection traversal"
		}
		if stdlib[imported] || imported == module+"/shared/values" || imported == module+"/ucupa-core/contract" ||
			imported == module+"/ucupa-core/internal/element/fact" || imported == module+"/ucupa-core/internal/element/read" ||
			imported == module+"/ucupa-core/internal/facet/fact" || imported == module+"/ucupa-core/internal/geometry" ||
			imported == module+"/ucupa-core/internal/mount/fact" || imported == module+"/ucupa-core/internal/query/federation/transcript" ||
			imported == module+"/ucupa-core/internal/query/federation/value" || imported == module+"/ucupa-core/internal/query/scan" ||
			imported == module+"/ucupa-core/internal/token" || test && imported == module+"/"+path.Dir(file) {
			return ""
		}
		return "Core Federation projection computes bounded results from finite fact views; it cannot depend on commit, query owner, registry, Store, root workflows, transport or SDK"
	}
	if reviewedCoreFederationTranscriptPackage(sourceOwner, path.Dir(file)) {
		if stdlib[imported] || imported == module+"/shared/values" || imported == module+"/ucupa-core/internal/query/federation/value" || imported == module+"/ucupa-core/internal/token" ||
			test && (imported == module+"/ucupa-core/contract" || imported == module+"/"+path.Dir(file)) {
			return ""
		}
		return "Core Federation transcript owns only call-local canonical framing; it cannot depend on projection, query owners, contracts, commit, authorization, Store or transport"
	}
	if imported == module+"/ucupa-core/internal/query/federation/projection" {
		if sourceOwner == "ucupa-core" && len(source) == 1 {
			base := path.Base(file)
			if !test {
				switch base {
				case "public_federation_projection.go", "public_federation_observe_binding.go", "public_federation_part.go", "public_federation_hidden.go":
					return ""
				}
			} else if base == "public_federation_parent_test.go" {
				return ""
			}
		}
		return "Federation projection is consumed only by the four reviewed Core public producers and its parent integration test"
	}
	if imported == module+"/ucupa-core/internal/query/federation/transcript" {
		if sourceOwner == "ucupa-core" && len(source) == 1 {
			base := path.Base(file)
			if !test && (base == "public_federated_closure.go" || base == "public_federated_disclosure.go") {
				return ""
			}
		}
		return "Federation transcript framing is consumed only by projection traversal and the two reviewed Core replay/disclosure producers"
	}
	if imported == module+"/ucupa-core/internal/query/federation/value" {
		if sourceOwner == "ucupa-core" && path.Dir(file) == "ucupa-core/contract/federation" {
			return ""
		}
		if sourceOwner == "ucupa-core" && len(source) == 1 {
			switch path.Base(file) {
			case "public_federation_traversal.go":
				if !test {
					return ""
				}
			case "public_federation_traversal_test.go":
				if test {
					return ""
				}
			}
		}
		return "Federation detached values are consumed only by the Federation contract alias and the two reviewed Core traversal producers"
	}
	if imported == module+"/ucupa-core/contract/federation" && sourceOwner == "ucupa-core" && len(source) == 1 {
		return ""
	}
	if checked, reason := checkCoreConnectedDependency(file, imported, module, test, stdlib); checked {
		return reason
	}
	if imported == module+"/ucupa-core/internal/access" &&
		!(sourceOwner == "ucupa-core" && (len(source) == 1 || path.Dir(file) == "ucupa-core/internal/facet" || path.Dir(file) == "ucupa-core/internal/mount") ||
			test && path.Dir(file) == "ucupa-core/internal/access") {
		return "Core access is consumed only by the Core root and reviewed Facet operations"
	}
	if imported == module+"/ucupa-core/internal/facet" &&
		!(sourceOwner == "ucupa-core" && (len(source) == 1 || test && path.Dir(file) == "ucupa-core/internal/facet")) {
		return "Core Facet operations are assembled only by the Core root"
	}
	if imported == module+"/ucupa-core/internal/mount" &&
		!(sourceOwner == "ucupa-core" && (len(source) == 1 || test && path.Dir(file) == "ucupa-core/internal/mount")) {
		return "Core mount operations are assembled only by the Core root"
	}
	if imported == module+"/ucupa-core/tests/query" && !test {
		return "Core query integration tests may not be imported by production source"
	}
	if imported == module+"/ucupa-core/tests/element" &&
		!(test && reviewedCoreElementTestPackage(sourceOwner, path.Dir(file))) {
		return "Core Element integration tests are test-only and owner-local"
	}
	if imported == module+"/ucupa-core/internal/query/oracle" {
		if test && path.Dir(file) == "ucupa-core" &&
			(path.Base(file) == "oracle_test.go" || path.Base(file) == "query_internal_test.go") {
			return ""
		}
		return "Core query oracle is a private conformance implementation consumed only by its reviewed tests, the reviewed mount-guard read test and the two current root white-box tests"
	}
	if checked, reason := checkCoreTransportDependency(file, imported, module, test, stdlib); checked {
		return reason
	}
	if reviewedCommitBindingPackage(sourceOwner, path.Dir(file)) {
		if stdlib[imported] || imported == module+"/shared/values" || imported == module+"/ucupa-core/contract" || imported == module+"/ucupa-core/internal/element/fact" || imported == module+"/ucupa-core/internal/geometry" || imported == module+"/ucupa-core/internal/token" || imported == module+"/ucupa-core/internal/access/value" || test && imported == module+"/"+path.Dir(file) {
			return ""
		}
		return "Core commit bindings own retained operation input and its canonical public syntax; no owner state, authorization, Store, workflow, registry or transport dependency"
	}
	if sourceOwner == "ucupa-core" && path.Dir(file) == "ucupa-core/internal/realmcontact" {
		if stdlib[imported] || imported == module+"/shared/values" || imported == module+"/ucupa-core/contract" || imported == module+"/ucupa-core/internal/commit" || test && imported == module+"/"+path.Dir(file) {
			return ""
		}
		return "Core Realm contact control may use only contract, shared values and the original commit owner; no Store, transport, SDK or second authorization owner"
	}
	if reviewedCommitPackage(sourceOwner, path.Dir(file)) {
		if imported == "go.etcd.io/bbolt" {
			switch path.Base(file) {
			case "store.go", "store_load.go", "mount_store_event.go":
				if !test {
					return ""
				}
			case "foundation_fault_export_test.go", "foundation_store_fixture_test.go", "foundation_store_test.go", "realm_contact_store_test.go":
				if test {
					return ""
				}
			}
			return "bbolt is registered only for Core commit Store files; aggregate rules and tests may not acquire database access"
		}
		if test && (path.Base(file) == "element_store_test.go" || path.Base(file) == "foundation_replaced_test.go") && imported == module+"/ucupa-core/internal/element" {
			return ""
		}
		if stdlib[imported] || imported == module+"/shared/values" || imported == module+"/ucupa-core/contract" || imported == module+"/ucupa-core/internal/access/observe" || imported == module+"/ucupa-core/internal/commit/binding" || imported == module+"/ucupa-core/internal/element/fact" || imported == module+"/ucupa-core/internal/facet/fact" || imported == module+"/ucupa-core/internal/geometry" || imported == module+"/ucupa-core/internal/mount/fact" || imported == module+"/ucupa-core/internal/store" || imported == module+"/ucupa-core/internal/token" || test && imported == module+"/"+path.Dir(file) {
			return ""
		}
		return "Core commit owns aggregate state, deterministic transitions, Store/replay and synchronous publication; it may use only reviewed facts, values, token/binding rules, observation projection and Store bytes"
	}
	if reviewedCoreObservationPackage(sourceOwner, path.Dir(file)) {
		if stdlib[imported] || imported == module+"/shared/values" || imported == module+"/ucupa-core/contract" ||
			imported == module+"/ucupa-core/internal/element/fact" || imported == module+"/ucupa-core/internal/mount/fact" ||
			imported == module+"/ucupa-core/internal/facet/fact" || test && imported == module+"/"+path.Dir(file) {
			return ""
		}
		return "Core observation projection compares finite current facts; no owner, Store, authorization, history, registry or transport dependency"
	}
	if reviewedCoreQueryTestPackage(sourceOwner, path.Dir(file)) {
		if !test {
			return "Core query integration tests are test-only"
		}
		if imported == "go.etcd.io/bbolt" && path.Base(file) == "persistence_test.go" {
			return ""
		}
		if imported == module+"/ucupa-core/contract/federation" && path.Base(file) == "source_attachment_test.go" {
			return ""
		}
		if stdlib[imported] || imported == module+"/shared/values" || imported == module+"/ucupa-core/instance" || imported == module+"/ucupa-core/contract" {
			return ""
		}
		return "Core query integration tests may use only standard library, shared values, Core entry points and Core contract"
	}
	if reviewedCoreElementTestPackage(sourceOwner, path.Dir(file)) {
		if !test {
			return "Core Element integration tests are test-only"
		}
		if stdlib[imported] || imported == module+"/shared/values" || imported == module+"/ucupa-core/instance" || imported == module+"/ucupa-core/contract" {
			return ""
		}
		return "Core Element integration tests may use only standard library, shared values, Core entry points and Core contract"
	}
	if reviewedAccessValuePackage(sourceOwner, path.Dir(file)) {
		if imported == module+"/ucupa-core/internal/token" && (path.Base(file) == "record.go" || test && path.Base(file) == "record_test.go") {
			return ""
		}
		if stdlib[imported] || imported == module+"/shared/values" || imported == module+"/ucupa-core/contract" || imported == module+"/ucupa-core/internal/geometry" || test && imported == module+"/"+path.Dir(file) {
			return ""
		}
		return "Core public values own only field shapes and representation; only record projection uses the original signer; no binding, facts, owner, authorization, Store, registry or transport dependency"
	}
	if reviewedQueryScanPackage(sourceOwner, path.Dir(file)) {
		if stdlib[imported] || imported == module+"/shared/values" || imported == module+"/ucupa-core/contract" || imported == module+"/ucupa-core/internal/element/fact" || imported == module+"/ucupa-core/internal/geometry" || imported == module+"/ucupa-core/internal/mount/fact" || imported == module+"/ucupa-core/internal/query/request" || test && imported == module+"/"+path.Dir(file) {
			return ""
		}
		return "Core query scans read immutable element and relationship views; no owner, mount control, authorization, commit, Store, registry or transport dependency"
	}
	if reviewedQueryRequestPackage(sourceOwner, path.Dir(file)) {
		if stdlib[imported] || imported == module+"/shared/values" || imported == module+"/ucupa-core/internal/geometry" || imported == module+"/ucupa-core/internal/token" || test && imported == module+"/"+path.Dir(file) {
			return ""
		}
		return "Core query requests own only request shapes and opaque positions; no reverse contract, owner, registry, authorization, facts, Store or transport dependency"
	}
	if reviewedMountFactPackage(sourceOwner, path.Dir(file)) {
		if !test && path.Base(file) == "validation.go" && imported == module+"/ucupa-core/internal/commit/binding" {
			return ""
		}
		if test && path.Base(file) == "validation_test.go" && imported == module+"/ucupa-core/internal/token" {
			return ""
		}
		if stdlib[imported] || imported == module+"/shared/values" || imported == module+"/ucupa-core/contract" || test && imported == module+"/"+path.Dir(file) {
			return ""
		}
		return "Core mount facts own only current relationship records; no owner, Element tables, authorization, commit, Store, token issuance or transport dependency"
	}
	if reviewedFacetFactPackage(sourceOwner, path.Dir(file)) {
		if stdlib[imported] || imported == module+"/shared/values" || imported == module+"/ucupa-core/contract" || imported == module+"/ucupa-core/internal/token" || test && imported == module+"/"+path.Dir(file) {
			return ""
		}
		return "Core Facet relationship facts contain only current records and deterministic plans; no owner, Element tables, authorization, commit, Store, Facet content or transport dependency"
	}
	if reviewedElementFactPackage(sourceOwner, path.Dir(file)) {
		if stdlib[imported] || imported == module+"/shared/values" || imported == module+"/ucupa-core/contract" || imported == module+"/ucupa-core/internal/geometry" || imported == module+"/ucupa-core/internal/token" || test && imported == module+"/"+path.Dir(file) {
			return ""
		}
		return "Core element facts contain deterministic records and rules; no owner, authorization, commit, Store, mount workflow or transport dependency"
	}
	if reviewedElementOperationPackage(sourceOwner, path.Dir(file)) {
		if test && path.Base(file) == "operation_test.go" && imported == module+"/ucupa-core/internal/element" {
			return ""
		}
		if stdlib[imported] || imported == module+"/shared/values" || imported == module+"/ucupa-core/contract" || imported == module+"/ucupa-core/internal/commit" || imported == module+"/ucupa-core/internal/commit/binding" || imported == module+"/ucupa-core/internal/element/fact" || imported == module+"/ucupa-core/internal/element/read" || imported == module+"/ucupa-core/internal/geometry" || imported == module+"/ucupa-core/internal/mount/fact" || imported == module+"/ucupa-core/internal/token" || test && imported == module+"/"+path.Dir(file) {
			return ""
		}
		return "Core Element operations and finite reads own request entry points and projections; no Store, query, transport, SDK or other owner dependency"
	}
	if reviewedElementReadPackage(sourceOwner, path.Dir(file)) {
		if test && path.Base(file) == "adjacency_test.go" {
			if stdlib[imported] || imported == module+"/shared/values" || imported == module+"/ucupa-core/contract" ||
				imported == module+"/ucupa-core/internal/commit" || imported == module+"/ucupa-core/internal/element" || imported == module+"/ucupa-core/internal/element/read" {
				return ""
			}
			return "Core Element adjacency tests may use only the real commit and Element read fixtures, public contracts, shared values and standard library"
		}
		if stdlib[imported] || imported == module+"/shared/values" || imported == module+"/ucupa-core/contract" || imported == module+"/ucupa-core/internal/element/fact" || imported == module+"/ucupa-core/internal/facet/fact" {
			return ""
		}
		return "Core Element reads own finite projections and direct adjacency; no mutable owner, authorization, commit, token, geometry, mount workflow, Store or transport dependency"
	}
	if reviewedCoreAccessPackage(sourceOwner, path.Dir(file)) {
		if stdlib[imported] || imported == module+"/shared/values" || imported == module+"/ucupa-core/contract" || test && imported == module+"/"+path.Dir(file) {
			return ""
		}
		return "Core access owns public authorization admission and its bounded active count; no facts, commit, Store, root, registry or transport dependency"
	}
	if reviewedCoreFacetPackage(sourceOwner, path.Dir(file)) {
		if test && (path.Base(file) == "operation_test.go" || path.Base(file) == "public_control_test.go" || path.Base(file) == "boundary_test.go") && imported == module+"/ucupa-core/internal/element" {
			return ""
		}
		if stdlib[imported] || imported == module+"/shared/values" || imported == module+"/ucupa-core/contract" || imported == module+"/ucupa-core/internal/commit" || imported == module+"/ucupa-core/internal/commit/binding" || imported == module+"/ucupa-core/internal/facet/fact" || imported == module+"/ucupa-core/internal/access" || imported == module+"/ucupa-core/internal/token" || test && imported == module+"/"+path.Dir(file) {
			return ""
		}
		return "Core Facet operations own control/read requests and deterministic commit coordination; only exact tests may use Element fixtures"
	}
	if reviewedCoreMountOperationPackage(sourceOwner, path.Dir(file)) {
		if test && (path.Base(file) == "operation_test.go" || path.Base(file) == "boundary_test.go") && imported == module+"/ucupa-core/internal/element" {
			return ""
		}
		if stdlib[imported] || imported == module+"/shared/values" || imported == module+"/ucupa-core/contract" || imported == module+"/ucupa-core/internal/commit" || imported == module+"/ucupa-core/internal/commit/binding" || imported == module+"/ucupa-core/internal/mount/fact" || imported == module+"/ucupa-core/internal/token" || imported == module+"/ucupa-core/internal/access" || imported == module+"/ucupa-core/internal/access/value" || test && imported == module+"/"+path.Dir(file) {
			return ""
		}
		return "Core mount operations own mount control, peer and recovery workflows; only exact tests may use Element fixtures"
	}
	if reviewedQueryPackage(sourceOwner, path.Dir(file)) {
		if test && path.Base(file) == "query_entry_test.go" && imported == module+"/ucupa-core/internal/element" {
			return ""
		}
		if stdlib[imported] || imported == module+"/shared/values" || imported == module+"/ucupa-core/contract" || imported == module+"/ucupa-core/internal/commit" || imported == module+"/ucupa-core/internal/element/fact" || imported == module+"/ucupa-core/internal/element/read" || imported == module+"/ucupa-core/internal/mount/fact" || imported == module+"/ucupa-core/internal/query/request" || imported == module+"/ucupa-core/internal/query/scan" || imported == module+"/ucupa-core/internal/token" || test && imported == module+"/"+path.Dir(file) {
			return ""
		}
		return "Core query owns local read operations and the bounded snapshot registry; it may use only contract, finite facts, query rules, token values and the commit read boundary"
	}
	if path.Dir(file) == "ucupa-facet" || imported == module+"/ucupa-facet" {
		return "Facet root is no longer an implementation or compatibility package"
	}
	if reviewedNamespacePackage(sourceOwner, path.Dir(file)) {
		if stdlib[imported] || imported == module+"/shared/values" || imported == module+"/ucupa-facet/contract" || imported == module+"/ucupa-facet/internal/token" || imported == module+"/ucupa-facet/internal/access" || imported == module+"/ucupa-facet/internal/namespace/request" || test && imported == module+"/"+path.Dir(file) {
			return ""
		}
		if imported == "go.etcd.io/bbolt" && (path.Base(file) == "store.go" || path.Base(file) == "store_operations.go" || test && (path.Base(file) == "store_atomicity_test.go" || path.Base(file) == "store_lifecycle_test.go" || path.Base(file) == "boundary_test.go" || path.Base(file) == "realm_contact_test.go")) {
			return ""
		}
		return "Facet namespace owns one content/acceptance commit and recovery lifecycle; it cannot depend on instance, transport, peers or unreviewed internals"
	}
	if reviewedInstancePackage(sourceOwner, path.Dir(file)) {
		if stdlib[imported] || imported == module+"/shared/values" || imported == module+"/ucupa-facet/contract" || imported == module+"/ucupa-facet/internal/namespace" || test && imported == module+"/"+path.Dir(file) {
			return ""
		}
		return "Facet instance only constructs and closes namespace; it cannot reach raw storage, resource internals, peers or wire adapters"
	}
	if reviewedAccessPackage(sourceOwner, path.Dir(file)) {
		if stdlib[imported] || imported == module+"/shared/values" || imported == module+"/ucupa-facet/internal/namespace/request" || test && imported == module+"/"+path.Dir(file) {
			return ""
		}
		return "Facet access owns only finite contexts and observation resources; it cannot import facts, commit, Store, instance or external authorization"
	}
	if reviewedFailurePackage(sourceOwner, path.Dir(file)) {
		if stdlib[imported] || test && imported == module+"/"+path.Dir(file) {
			return ""
		}
		return "Core failure defines only shared sentinel identity; it cannot depend on values, workflows or adapters"
	}
	if reviewedGeometryPackage(sourceOwner, path.Dir(file)) {
		if stdlib[imported] || imported == module+"/ucupa-core/internal/failure" || test && imported == module+"/"+path.Dir(file) {
			return ""
		}
		return "Core geometry owns pure spatial values and exact arithmetic; it may not depend on contract, facts, commit, Store or other owners"
	}
	if reviewedRequestPackage(sourceOwner, path.Dir(file)) {
		if stdlib[imported] || imported == module+"/shared/values" || test && imported == module+"/"+path.Dir(file) {
			return ""
		}
		return "Facet request values may use only standard library and shared/values; no instance, mutable facts, Store or external authorization dependency"
	}
	if reviewedStorePackage(sourceOwner, path.Dir(file)) {
		if imported == "go.etcd.io/bbolt" {
			if path.Base(file) == "transaction.go" || test && path.Base(file) == "transaction_test.go" {
				return ""
			}
			return "bbolt is registered only for Core Store transaction preparation and its tests; pure codecs may not acquire database access"
		}
		if stdlib[imported] || imported == module+"/shared/values" || test && imported == module+"/"+path.Dir(file) {
			return ""
		}
		return "Core Store may use only standard library, existing bbolt and shared/values; no owner, contract, commit, transport, SDK or peripheral dependency"
	}
	if reviewedBusinessPackage(sourceOwner, path.Dir(file)) {
		if stdlib[imported] || imported == module+"/shared/values" || sourceOwner == "ucupa-core" && imported == module+"/ucupa-core/internal/failure" || test && imported == module+"/"+path.Dir(file) {
			return ""
		}
		return "opaque tokens may use only standard library, shared/values and Core sentinel identity; no reverse contract or owner dependency"
	}
	if reviewedContractPackage(sourceOwner, path.Dir(file)) {
		if stdlib[imported] || imported == module+"/shared/values" || imported == module+"/"+sourceOwner+"/internal/token" || sourceOwner == "ucupa-core" && (imported == module+"/ucupa-core/internal/geometry" || imported == module+"/ucupa-core/internal/query/request") || test && imported == module+"/"+path.Dir(file) {
			return ""
		}
		return "public values may use only their reviewed opaque definition and pure values, not instance or owner workflows"
	}
	if path.Dir(file) == "internal/grpcdial" {
		if stdlib[imported] || imported == "google.golang.org/grpc" || strings.HasPrefix(imported, "google.golang.org/grpc/") || imported == module+"/shared/values" {
			return ""
		}
		return "pinned dialing cannot depend on owners or session state"
	}
	if imported == module+"/internal/grpcdial" && path.Dir(file) != "ucupa-core/sdk/go" && path.Dir(file) != "ucupa-facet/sdk/go" {
		return "pinned dialing belongs to the two SDKs"
	}
	if path.Dir(file) == "internal/grpcbound" {
		if stdlib[imported] || strings.HasPrefix(imported, "google.golang.org/grpc/") {
			return ""
		}
		return "connection bounds cannot depend on owners or authentication state"
	}
	if imported == module+"/internal/grpcbound" && path.Dir(file) != "ucupa-core/transport/grpc" && path.Dir(file) != "ucupa-facet/transport/grpc" && path.Dir(file) != "ucupa-realm/transport/grpc" {
		return "connection bounds belong to the three transport adapters"
	}
	if path.Dir(file) == "internal/protovalue" {
		if stdlib[imported] || imported == module+"/shared/values" || imported == module+"/shared/bindings/go/protobuf" || strings.HasPrefix(imported, "google.golang.org/protobuf/") {
			return ""
		}
		return "private protocol conversion cannot depend on owners, transport or assembly"
	}
	if imported == module+"/internal/protovalue" && path.Dir(file) != "ucupa-core/transport/grpc" && path.Dir(file) != "ucupa-facet/transport/grpc" && path.Dir(file) != "ucupa-realm/transport/grpc" {
		return "private protocol conversion is restricted to the three declared transport adapters"
	}
	generated := generatedSource(file)
	if generated {
		if stdlib[imported] || imported == module+"/shared/bindings/go/protobuf" || strings.HasPrefix(imported, "google.golang.org/protobuf/") || imported == "google.golang.org/grpc" || imported == "google.golang.org/grpc/codes" || imported == "google.golang.org/grpc/status" {
			return ""
		}
		return "generated bindings may depend only on their protocol runtime and shared bindings"
	}
	if !test && ownerOf(source) != "" && (len(source) == 1 || source[1] == "internal") && (strings.HasPrefix(imported, "google.golang.org/grpc") || strings.HasPrefix(imported, "google.golang.org/protobuf")) {
		return "owner kernel must not depend on wire runtimes"
	}
	if source[0] == "shared" && !stdlib[imported] {
		return "shared may depend only on the standard library (including its tests)"
	}
	if module == "" {
		return ""
	}
	if imported == module {
		return "the module root is not an approved implementation package"
	}
	if !strings.HasPrefix(imported, module+"/") {
		return ""
	}
	target := strings.Split(strings.TrimPrefix(imported, module+"/"), "/")
	targetOwner := ownerOf(target)
	targetInternal := targetOwner != "" && contains(target[1:], "internal")
	if targetInternal && sourceOwner != targetOwner {
		return "owner internal packages are private to that owner; commands, acceptance and peers must use public boundaries"
	}
	peripheral := sourceOwner != "" && len(source) > 1 && (source[1] == "sdk" || source[1] == "bindings" || source[1] == "transport")
	if peripheral && !test && targetOwner != "" && reviewedInstancePackage(targetOwner, strings.Join(target, "/")) {
		return "wire adapters consume narrow ports, not local instance construction"
	}
	if peripheral && targetInternal {
		return "SDK, bindings and transport cannot access owner internals"
	}
	if targetInternal && !reviewedInternalPackage(targetOwner, strings.Join(target, "/")) {
		return "owner internal dependency has no reviewed responsibility or dependency edges"
	}
	if sourceOwner != "" && len(source) > 1 && (source[1] == "sdk" || source[1] == "bindings") && targetOwner != "" {
		if source[1] == "sdk" && targetOwner == sourceOwner && reviewedContractPackage(targetOwner, strings.Join(target, "/")) {
			return ""
		}
		if len(target) < 2 || target[1] != "bindings" {
			return "SDK and bindings may use public protocol bindings, not owner implementations"
		}
	}
	// Owner tests may use peer public implementations as real port fixtures.
	// This exception is test-only; production collaboration goes through ports.
	if test || sourceOwner == "" {
		return ""
	}
	if targetOwner != "" && sourceOwner != targetOwner {
		return "owner production code cannot import another owner; use a consumer-owned port and composition adapter"
	}
	if target[0] == "composition" || target[0] == "cmd" || target[0] == "acceptance" || target[0] == "transport" || target[0] == "sdk" {
		return "owner production code cannot depend on assembly, commands, tests, transport or SDK"
	}
	if !peripheral && targetOwner == sourceOwner && len(target) > 1 && (target[1] == "transport" || target[1] == "sdk" || target[1] == "bindings") {
		return "owner kernel cannot depend on transport, SDK or wire bindings"
	}
	if len(source) > 1 && source[1] == "internal" && targetOwner == sourceOwner && len(target) == 1 {
		return "owner internal capability cannot depend back on its public owner package"
	}
	if !peripheral {
		if imported == module+"/shared/values" || targetOwner == sourceOwner && (reviewedInternalPackage(targetOwner, strings.Join(target, "/")) || reviewedContractPackage(targetOwner, strings.Join(target, "/"))) {
			return ""
		}
		return "owner production dependencies inside this repository are limited to shared/values and its own internal capabilities"
	}
	return ""
}
