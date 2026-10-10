package main

import (
	"strings"
	"testing"
)

const (
	coreFederationValueDirectory    = "ucupa-core/internal/query/federation/value"
	coreFederationContractDirectory = "ucupa-core/contract/federation"
)

var coreFederationValueSourceFiles = []string{"value.go", "observe.go", "value_test.go"}

var coreFederationContractSourceFiles = []string{"federation.go", "federation_test.go"}

func TestReviewedCoreFederationValueBoundary(t *testing.T) {
	if !reviewedCoreFederationValuePackage("ucupa-core", coreFederationValueDirectory) ||
		reviewedCoreFederationValuePackage("ucupa-facet", coreFederationValueDirectory) ||
		reviewedCoreFederationValuePackage("ucupa-core", coreFederationValueDirectory+"/child") {
		t.Fatal("Core Federation value registration must identify exactly one package")
	}
	for _, name := range coreFederationValueSourceFiles {
		file := coreFederationValueDirectory + "/" + name
		if got := checkSourceLocation(file); got != "" {
			t.Errorf("reviewed Federation value source %s: %s", file, got)
		}
	}
	for _, file := range []string{
		coreFederationValueDirectory + "/extra.go",
		coreFederationValueDirectory + "/value_extra_test.go",
		coreFederationValueDirectory + "/child/value.go",
		"ucupa-core/internal/query/federation/value/nested.go",
	} {
		if got := checkSourceLocation(file); got == "" {
			t.Errorf("unreviewed Federation value source accepted: %s", file)
		}
	}

	stdlib := testStdlib()
	for _, dependency := range []string{
		"ucupa/shared/values",
		"ucupa/ucupa-core/contract",
		"ucupa/ucupa-core/internal/geometry",
	} {
		if got := checkDependency(coreFederationValueDirectory+"/value.go", dependency, "ucupa", false, stdlib); got != "" {
			t.Errorf("allowed Federation value dependency %s: %s", dependency, got)
		}
	}
	for _, dependency := range []string{
		"ucupa/ucupa-core",
		"ucupa/ucupa-core/contract/federation",
		"ucupa/ucupa-core/internal/query",
		"ucupa/ucupa-core/internal/query/request",
		"ucupa/ucupa-core/internal/commit",
		"ucupa/ucupa-core/internal/store",
		"ucupa/ucupa-core/internal/token",
		"ucupa/ucupa-core/internal/element/fact",
		"ucupa/ucupa-core/internal/mount/fact",
		"ucupa/ucupa-core/transport/grpc",
		"ucupa/ucupa-core/sdk/go",
		"ucupa/ucupa-facet/contract",
		"go.etcd.io/bbolt",
	} {
		if got := checkDependency(coreFederationValueDirectory+"/value.go", dependency, "ucupa", false, stdlib); got == "" {
			t.Errorf("forbidden Federation value dependency accepted: %s", dependency)
		}
	}
	if got := checkDependency(coreFederationValueDirectory+"/value_test.go", "ucupa/ucupa-core/contract", "ucupa", true, stdlib); got != "" {
		t.Fatalf("Federation value white-box test lost its contract dependency: %s", got)
	}
}

func TestReviewedCoreFederationContractBoundary(t *testing.T) {
	if !reviewedCoreFederationContractPackage("ucupa-core", coreFederationContractDirectory) ||
		reviewedCoreFederationContractPackage("ucupa-facet", coreFederationContractDirectory) ||
		reviewedCoreFederationContractPackage("ucupa-core", coreFederationContractDirectory+"/child") {
		t.Fatal("Core Federation contract registration must identify exactly one package")
	}
	for _, name := range coreFederationContractSourceFiles {
		file := coreFederationContractDirectory + "/" + name
		if got := checkSourceLocation(file); got != "" {
			t.Errorf("reviewed Federation contract source %s: %s", file, got)
		}
	}
	for _, file := range []string{
		coreFederationContractDirectory + "/extra.go",
		coreFederationContractDirectory + "/federation_extra_test.go",
		coreFederationContractDirectory + "/child/federation.go",
		"ucupa-core/contract/federation/value.go",
	} {
		if got := checkSourceLocation(file); got == "" {
			t.Errorf("unreviewed Federation contract source accepted: %s", file)
		}
	}

	stdlib := testStdlib()
	stdlib["context"] = true
	for _, dependency := range []string{
		"context",
		"ucupa/shared/values",
		"ucupa/ucupa-core/contract",
		"ucupa/ucupa-core/internal/query/federation/value",
	} {
		if got := checkDependency(coreFederationContractDirectory+"/federation.go", dependency, "ucupa", false, stdlib); got != "" {
			t.Errorf("allowed Federation contract dependency %s: %s", dependency, got)
		}
	}
	if got := checkDependency(coreFederationContractDirectory+"/federation_test.go", "ucupa/"+coreFederationContractDirectory, "ucupa", true, stdlib); got != "" {
		t.Fatalf("Federation contract white-box package self import rejected: %s", got)
	}
	for _, dependency := range []string{
		"ucupa/ucupa-core",
		"ucupa/ucupa-core/contract/connected",
		"ucupa/ucupa-core/internal/query",
		"ucupa/ucupa-core/internal/query/request",
		"ucupa/ucupa-core/internal/commit",
		"ucupa/ucupa-core/internal/store",
		"ucupa/ucupa-core/internal/token",
		"ucupa/ucupa-core/internal/geometry",
		"ucupa/ucupa-core/transport/grpc",
		"ucupa/ucupa-core/sdk/go",
		"ucupa/ucupa-facet/contract",
		"go.etcd.io/bbolt",
	} {
		if got := checkDependency(coreFederationContractDirectory+"/federation.go", dependency, "ucupa", false, stdlib); got == "" {
			t.Errorf("forbidden Federation contract dependency accepted: %s", dependency)
		}
	}
}

func TestFederationValueConsumersAreExact(t *testing.T) {
	stdlib := testStdlib()
	for _, tc := range []struct {
		source, dependency string
		test               bool
		allowed            bool
	}{
		{"ucupa-core/public_federation_traversal.go", "ucupa/" + coreFederationValueDirectory, false, true},
		{"ucupa-core/public_federation_traversal_test.go", "ucupa/" + coreFederationValueDirectory, true, true},
		{"ucupa-core/contract/federation/federation.go", "ucupa/" + coreFederationValueDirectory, false, true},
		{"ucupa-core/contract/federation/federation_test.go", "ucupa/" + coreFederationValueDirectory, true, true},
		{"ucupa-core/public_context.go", "ucupa/" + coreFederationValueDirectory, false, false},
		{"ucupa-core/public_federated_read.go", "ucupa/" + coreFederationValueDirectory, false, false},
		{"ucupa-core/public_federation_projection_test.go", "ucupa/" + coreFederationValueDirectory, true, false},
		{"ucupa-core/internal/query/operation.go", "ucupa/" + coreFederationValueDirectory, false, false},
		{"ucupa-core/internal/commit/state.go", "ucupa/" + coreFederationValueDirectory, false, false},
		{"ucupa-core/transport/grpc/internal/wire/federation_wire.go", "ucupa/" + coreFederationValueDirectory, false, false},
		{"ucupa-core/transport/grpc/federationconn/federation_connections.go", "ucupa/" + coreFederationValueDirectory, false, false},
		{"ucupa-core/sdk/go/federation.go", "ucupa/" + coreFederationValueDirectory, false, false},
		{"composition/ports.go", "ucupa/" + coreFederationValueDirectory, false, false},
	} {
		got := checkDependency(tc.source, tc.dependency, "ucupa", tc.test, stdlib)
		if (got == "") != tc.allowed {
			t.Errorf("%s -> %s: allowed=%v got=%q", tc.source, tc.dependency, tc.allowed, got)
		}
	}
}

func TestFederationContractConsumersAndTransportEdgesAreExact(t *testing.T) {
	stdlib := testStdlib()
	for _, source := range []string{
		"ucupa-core/core.go",
		"ucupa-core/public_context.go",
		"ucupa-core/transport/grpc/internal/wire/federation_wire.go",
		"ucupa-core/transport/grpc/internal/wire/federation_projection_wire.go",
		"ucupa-core/transport/grpc/internal/wire/federation_page_wire.go",
		"ucupa-core/transport/grpc/federationconn/federation_connections.go",
		"ucupa-core/transport/grpc/federationconn/federation_observe_remote.go",
		"ucupa-core/transport/grpc/federationconn/federation_read_at_remote.go",
		"ucupa-core/transport/grpc/federationconn/federation_remote.go",
		"composition/ports.go",
	} {
		if got := checkDependency(source, "ucupa/"+coreFederationContractDirectory, "ucupa", false, stdlib); got != "" {
			t.Errorf("approved Federation contract consumer %s rejected: %s", source, got)
		}
	}
	for _, source := range []string{
		"ucupa-core/contract/query.go",
		"ucupa-core/internal/query/operation.go",
		"ucupa-core/internal/commit/state.go",
		"ucupa-core/internal/query/federation/value/value.go",
		"ucupa-core/transport/grpc/internal/wire/mount_wire.go",
		"ucupa-core/transport/grpc/mountconn/mount_connections.go",
		"ucupa-core/sdk/go/federation.go",
		"ucupa-facet/contract/contract.go",
	} {
		if got := checkDependency(source, "ucupa/"+coreFederationContractDirectory, "ucupa", false, stdlib); got == "" {
			t.Errorf("unapproved Federation contract consumer accepted: %s", source)
		}
	}
	for _, source := range []string{
		"ucupa-core/contract/query.go",
	} {
		if got := checkDependency(source, "ucupa/"+coreFederationValueDirectory, "ucupa", false, stdlib); got == "" {
			t.Errorf("reverse Core contract dependency on Federation value accepted: %s", source)
		}
	}
}

func TestFederationSourceOwnershipDoesNotBroaden(t *testing.T) {
	stdlib := testStdlib()
	for _, tc := range []struct {
		source, dependency string
		allowed            bool
	}{
		{coreFederationValueDirectory + "/value.go", "ucupa/" + coreFederationContractDirectory, false},
		{coreFederationContractDirectory + "/federation.go", "ucupa/" + coreFederationValueDirectory, true},
		{"ucupa-core/contract/query.go", "ucupa/" + coreFederationContractDirectory, false},
		{"ucupa-core/public_context.go", "ucupa/" + coreFederationContractDirectory, true},
	} {
		got := checkDependency(tc.source, tc.dependency, "ucupa", false, stdlib)
		if (got == "") != tc.allowed {
			t.Errorf("%s -> %s: allowed=%v got=%q", tc.source, tc.dependency, tc.allowed, got)
		}
	}
}

func TestFederationBoundaryMessagesRemainSpecific(t *testing.T) {
	stdlib := testStdlib()
	for _, tc := range []struct {
		source, dependency, want string
	}{
		{coreFederationValueDirectory + "/value.go", "ucupa/ucupa-core/internal/query", "detached values"},
		{coreFederationContractDirectory + "/federation.go", "ucupa/ucupa-core/internal/commit", "Federation contract"},
		{"ucupa-core/transport/grpc/internal/wire/mount_wire.go", "ucupa/" + coreFederationContractDirectory, "exact Federation wire"},
	} {
		if got := checkDependency(tc.source, tc.dependency, "ucupa", false, stdlib); !strings.Contains(got, tc.want) {
			t.Errorf("%s -> %s: got %q, want message containing %q", tc.source, tc.dependency, got, tc.want)
		}
	}
}
