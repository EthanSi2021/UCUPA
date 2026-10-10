package main

import "testing"

func TestReviewedCoreAccessBoundary(t *testing.T) {
	const directory = "ucupa-core/internal/access"
	if !reviewedCoreAccessPackage("ucupa-core", directory) ||
		reviewedCoreAccessPackage("ucupa-facet", directory) ||
		reviewedCoreAccessPackage("ucupa-core", directory+"/child") {
		t.Fatal("Core access registration must identify exactly one package")
	}
	for _, file := range []string{directory + "/admission.go", directory + "/admission_test.go"} {
		if got := checkSourceLocation(file); got != "" {
			t.Errorf("reviewed Core access source %s: %s", file, got)
		}
	}
	for _, file := range []string{directory + "/extra.go", directory + "/admission_extra_test.go", directory + "/child/admission.go"} {
		if got := checkSourceLocation(file); got == "" {
			t.Errorf("unreviewed Core access source accepted: %s", file)
		}
	}

	stdlib := testStdlib()
	for _, dependency := range []string{"errors", "fmt", "sync", "testing", "ucupa/shared/values", "ucupa/ucupa-core/contract"} {
		stdlib[dependency] = true
		if got := checkDependency(directory+"/admission.go", dependency, "ucupa", false, stdlib); got != "" {
			t.Errorf("allowed Core access dependency %s: %s", dependency, got)
		}
	}
	for _, dependency := range []string{
		"ucupa/ucupa-core",
		"ucupa/ucupa-core/internal/element/fact",
		"ucupa/ucupa-core/internal/element",
		"ucupa/ucupa-core/internal/commit",
		"ucupa/ucupa-core/internal/store",
		"ucupa/ucupa-core/internal/query",
		"ucupa/ucupa-core/internal/token",
		"ucupa/ucupa-core/transport/grpc",
		"ucupa/ucupa-core/sdk/go",
		"ucupa/ucupa-facet/contract",
		"ucupa/composition",
		"go.etcd.io/bbolt",
	} {
		if got := checkDependency(directory+"/admission.go", dependency, "ucupa", false, stdlib); got == "" {
			t.Errorf("forbidden Core access dependency accepted: %s", dependency)
		}
	}
	for _, tc := range []struct {
		source, dependency string
		allowed            bool
		test               bool
	}{
		{"ucupa-core/instance/instance.go", "ucupa/ucupa-core/internal/access", true, false},
		{"ucupa-core/internal/facet/operation.go", "ucupa/ucupa-core/internal/access", true, false},
		{"ucupa-core/internal/mount/operation.go", "ucupa/ucupa-core/internal/access", true, false},
		{"ucupa-core/internal/query/operation.go", "ucupa/ucupa-core/internal/access", false, false},
		{"ucupa-core/internal/element/operation.go", "ucupa/ucupa-core/internal/access", false, false},
		{"ucupa-core/internal/commit/state.go", "ucupa/ucupa-core/internal/access", false, false},
		{directory + "/admission_test.go", "ucupa/ucupa-core/internal/access", true, true},
		{"ucupa-core/instance/instance.go", "ucupa/ucupa-core/internal/facet", true, false},
		{"ucupa-core/internal/element/operation.go", "ucupa/ucupa-core/internal/facet", false, false},
		{"ucupa-core/internal/access/admission.go", "ucupa/ucupa-core/internal/facet", false, false},
	} {
		got := checkDependency(tc.source, tc.dependency, "ucupa", tc.test, stdlib)
		if (got == "") != tc.allowed {
			t.Errorf("%s -> %s: allowed=%v got=%q", tc.source, tc.dependency, tc.allowed, got)
		}
	}
}
