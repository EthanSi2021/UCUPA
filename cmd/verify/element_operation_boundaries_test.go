package main

import "testing"

func TestReviewedCoreElementOperationBoundary(t *testing.T) {
	const directory = "ucupa-core/internal/element"
	if !reviewedElementOperationPackage("ucupa-core", directory) ||
		reviewedElementOperationPackage("ucupa-facet", directory) ||
		reviewedElementOperationPackage("ucupa-core", directory+"/child") {
		t.Fatal("Core Element operation registration must identify exactly one package")
	}
	for _, file := range []string{directory + "/operation.go", directory + "/operation_test.go", directory + "/read.go", directory + "/projection.go", directory + "/read_test.go", directory + "/projection_test.go"} {
		if got := checkSourceLocation(file); got != "" {
			t.Errorf("reviewed Element operation source %s: %s", file, got)
		}
	}
	for _, file := range []string{
		directory + "/record.go",
		directory + "/child/operation.go",
		"ucupa-facet/internal/element/operation.go",
	} {
		if got := checkSourceLocation(file); got == "" {
			t.Errorf("unreviewed Element operation source accepted: %s", file)
		}
	}

	stdlib := testStdlib()
	for _, dependency := range []string{
		"errors", "reflect", "sort", "time",
		"ucupa/shared/values",
		"ucupa/ucupa-core/contract",
		"ucupa/ucupa-core/internal/commit",
		"ucupa/ucupa-core/internal/commit/binding",
		"ucupa/ucupa-core/internal/element/fact",
		"ucupa/ucupa-core/internal/element/read",
		"ucupa/ucupa-core/internal/geometry",
		"ucupa/ucupa-core/internal/mount/fact",
		"ucupa/ucupa-core/internal/token",
	} {
		stdlib[dependency] = true
	}
	for _, dependency := range []string{
		"ucupa/shared/values",
		"ucupa/ucupa-core/contract",
		"ucupa/ucupa-core/internal/commit",
		"ucupa/ucupa-core/internal/commit/binding",
		"ucupa/ucupa-core/internal/element/fact",
		"ucupa/ucupa-core/internal/token",
	} {
		if got := checkDependency(directory+"/operation.go", dependency, "ucupa", false, stdlib); got != "" {
			t.Errorf("allowed Element operation dependency %s: %s", dependency, got)
		}
	}
	for _, dependency := range []string{
		"ucupa/ucupa-core",
		"ucupa/ucupa-core/instance",
		"ucupa/ucupa-core/internal/access",
		"ucupa/ucupa-core/internal/facet/fact",
		"ucupa/ucupa-core/internal/query/scan",
		"ucupa/ucupa-core/internal/store",
		"ucupa/ucupa-core/transport/grpc",
		"ucupa/ucupa-core/sdk/go",
		"ucupa/ucupa-facet/contract",
		"ucupa/ucupa-realm",
		"ucupa/composition",
		"go.etcd.io/bbolt",
	} {
		if got := checkDependency(directory+"/operation.go", dependency, "ucupa", false, stdlib); got == "" {
			t.Errorf("forbidden Element operation dependency accepted: %s", dependency)
		}
	}

	for _, tc := range []struct {
		source, dependency string
		allowed            bool
		test               bool
	}{
		{"ucupa-core/instance/instance.go", "ucupa/ucupa-core/internal/element", true, false},
		{"ucupa-core/transport/grpc/server.go", "ucupa/ucupa-core/internal/element", false, false},
		{"ucupa-core/sdk/go/client.go", "ucupa/ucupa-core/internal/element", false, false},
		{"composition/ports.go", "ucupa/ucupa-core/internal/element", false, false},
		{"ucupa-core/internal/commit/state.go", "ucupa/ucupa-core/internal/element", false, false},
		{"ucupa-core/internal/element/fact/state.go", "ucupa/ucupa-core/internal/element", false, false},
		{"ucupa-core/internal/commit/element_store_test.go", "ucupa/ucupa-core/internal/element", true, true},
	} {
		got := checkDependency(tc.source, tc.dependency, "ucupa", tc.test, stdlib)
		if (got == "") != tc.allowed {
			t.Errorf("%s -> %s: allowed=%v got=%q", tc.source, tc.dependency, tc.allowed, got)
		}
	}
}

func TestReviewedCoreElementIntegrationTestBoundary(t *testing.T) {
	const directory = "ucupa-core/tests/element"
	if !reviewedCoreElementTestPackage("ucupa-core", directory) ||
		reviewedCoreElementTestPackage("ucupa-facet", directory) ||
		reviewedCoreElementTestPackage("ucupa-core", directory+"/nested") {
		t.Fatal("Core Element integration tests must identify exactly one package")
	}
	for _, file := range []string{
		directory + "/core_test.go",
		directory + "/fixture_test.go",
		directory + "/anchor_range_update_test.go",
		directory + "/write_convergence_test.go",
	} {
		if got := checkSourceLocation(file); got != "" {
			t.Errorf("reviewed Element integration test %s: %s", file, got)
		}
	}
	for _, file := range []string{
		directory + "/element_test.go",
		directory + "/nested/core_test.go",
		"ucupa-facet/tests/element/core_test.go",
	} {
		if got := checkSourceLocation(file); got == "" {
			t.Errorf("unreviewed Element integration test accepted: %s", file)
		}
	}

	stdlib := testStdlib()
	for _, dependency := range []string{
		"errors", "fmt", "math", "reflect", "sync", "testing",
	} {
		stdlib[dependency] = true
	}
	for _, dependency := range []string{
		"errors",
		"ucupa/shared/values",
		"ucupa/ucupa-core/instance",
		"ucupa/ucupa-core/contract",
	} {
		if got := checkDependency(directory+"/core_test.go", dependency, "ucupa", true, stdlib); got != "" {
			t.Errorf("allowed Element integration dependency %s: %s", dependency, got)
		}
	}
	for _, dependency := range []string{
		"ucupa/ucupa-core/internal/element",
		"ucupa/ucupa-core/internal/element/fact",
		"ucupa/ucupa-core/internal/element/read",
		"ucupa/ucupa-core/internal/commit",
		"ucupa/ucupa-core/internal/query",
		"ucupa/ucupa-core/internal/mount",
		"ucupa/ucupa-core/internal/facet",
		"ucupa/ucupa-core/transport/grpc",
		"ucupa/ucupa-core/sdk/go",
		"ucupa/ucupa-facet/contract",
		"go.etcd.io/bbolt",
	} {
		if got := checkDependency(directory+"/core_test.go", dependency, "ucupa", true, stdlib); got == "" {
			t.Errorf("forbidden Element integration dependency accepted: %s", dependency)
		}
	}
	for _, tc := range []struct {
		source, dependency string
		allowed            bool
		test               bool
	}{
		{directory + "/core_test.go", "ucupa/ucupa-core/instance", true, true},
		{"ucupa-core/core.go", "ucupa/ucupa-core/tests/element", false, false},
		{"ucupa-core/internal/query/query_entry_test.go", "ucupa/ucupa-core/tests/element", false, true},
		{"ucupa-facet/tests/element_test.go", "ucupa/ucupa-core/tests/element", false, true},
	} {
		got := checkDependency(tc.source, tc.dependency, "ucupa", tc.test, stdlib)
		if (got == "") != tc.allowed {
			t.Errorf("%s -> %s: allowed=%v got=%q", tc.source, tc.dependency, tc.allowed, got)
		}
	}
}
