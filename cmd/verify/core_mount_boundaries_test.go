package main

import "testing"

func TestReviewedCoreMountOperationBoundary(t *testing.T) {
	const directory = "ucupa-core/internal/mount"
	if !reviewedCoreMountOperationPackage("ucupa-core", directory) ||
		reviewedCoreMountOperationPackage("ucupa-facet", directory) ||
		reviewedCoreMountOperationPackage("ucupa-core", directory+"/child") {
		t.Fatal("Core mount operation registration must identify exactly one package")
	}
	for _, name := range []string{
		"operation.go", "control.go", "address.go", "recovery.go", "peer.go", "peer_types.go", "commitment.go",
		"public_authorization.go", "public_command.go", "public_contact.go", "public_control.go", "public_prepare.go", "authorization.go",
		"operation_test.go", "public_control_test.go", "public_prepare_test.go", "public_command_test.go", "public_contact_test.go", "commitment_test.go",
	} {
		if got := checkSourceLocation(directory + "/" + name); got != "" {
			t.Errorf("reviewed Core mount source %s: %s", name, got)
		}
	}
	for _, file := range []string{
		directory + "/extra.go",
		directory + "/unknown_test.go",
		directory + "/child/operation.go",
		directory + "/fact.go",
	} {
		if got := checkSourceLocation(file); got == "" {
			t.Errorf("unreviewed Core mount source accepted: %s", file)
		}
	}

	stdlib := testStdlib()
	for _, dependency := range []string{"context", "errors", "fmt", "reflect", "sort", "sync", "testing", "time"} {
		stdlib[dependency] = true
	}
	for _, dependency := range []string{
		"context", "errors", "fmt", "reflect", "sort", "sync", "testing", "time",
		"ucupa/shared/values",
		"ucupa/ucupa-core/contract",
		"ucupa/ucupa-core/internal/commit",
		"ucupa/ucupa-core/internal/commit/binding",
		"ucupa/ucupa-core/internal/mount/fact",
		"ucupa/ucupa-core/internal/token",
		"ucupa/ucupa-core/internal/access",
		"ucupa/ucupa-core/internal/access/value",
	} {
		if got := checkDependency(directory+"/operation.go", dependency, "ucupa", false, stdlib); got != "" {
			t.Errorf("allowed Core mount dependency %s: %s", dependency, got)
		}
	}
	for _, dependency := range []string{
		"ucupa/ucupa-core",
		"ucupa/ucupa-core/internal/element",
		"ucupa/ucupa-core/internal/element/fact",
		"ucupa/ucupa-core/internal/element/read",
		"ucupa/ucupa-core/internal/facet",
		"ucupa/ucupa-core/internal/facet/fact",
		"ucupa/ucupa-core/internal/geometry",
		"ucupa/ucupa-core/internal/query",
		"ucupa/ucupa-core/internal/query/scan",
		"ucupa/ucupa-core/internal/store",
		"ucupa/ucupa-core/transport/grpc",
		"ucupa/ucupa-core/sdk/go",
		"ucupa/ucupa-facet/contract",
		"ucupa/composition",
		"go.etcd.io/bbolt",
	} {
		if got := checkDependency(directory+"/operation.go", dependency, "ucupa", false, stdlib); got == "" {
			t.Errorf("forbidden Core mount dependency accepted: %s", dependency)
		}
	}

	if got := checkDependency(directory+"/operation_test.go", "ucupa/ucupa-core/internal/element", "ucupa", true, stdlib); got != "" {
		t.Fatalf("approved Element fixture dependency rejected for operation_test.go: %s", got)
	}
	for _, testFile := range []string{"public_control_test.go", "public_prepare_test.go", "public_command_test.go", "public_contact_test.go", "commitment_test.go"} {
		if got := checkDependency(directory+"/"+testFile, "ucupa/ucupa-core/internal/element", "ucupa", true, stdlib); got == "" {
			t.Fatalf("unapproved Element fixture dependency accepted for %s", testFile)
		}
	}

	for _, tc := range []struct {
		source, dependency string
		allowed            bool
		test               bool
	}{
		{"ucupa-core/instance/instance.go", "ucupa/ucupa-core/internal/mount", true, false},
		{directory + "/operation_test.go", "ucupa/ucupa-core/internal/mount", true, true},
		{"ucupa-core/internal/access/admission.go", "ucupa/ucupa-core/internal/mount", false, false},
		{"ucupa-core/internal/commit/state.go", "ucupa/ucupa-core/internal/mount", false, false},
		{"ucupa-core/internal/element/operation.go", "ucupa/ucupa-core/internal/mount", false, false},
		{"ucupa-core/internal/query/operation.go", "ucupa/ucupa-core/internal/mount", false, false},
		{"ucupa-core/internal/mount/fact/state.go", "ucupa/ucupa-core/internal/mount", false, false},
		{"ucupa-core/transport/grpc/server.go", "ucupa/ucupa-core/internal/mount", false, false},
		{"ucupa-core/sdk/go/client.go", "ucupa/ucupa-core/internal/mount", false, false},
		{"composition/ports.go", "ucupa/ucupa-core/internal/mount", false, false},
	} {
		got := checkDependency(tc.source, tc.dependency, "ucupa", tc.test, stdlib)
		if (got == "") != tc.allowed {
			t.Errorf("%s -> %s: allowed=%v got=%q", tc.source, tc.dependency, tc.allowed, got)
		}
	}
}
