package main

import "testing"

func TestReviewedCoreFacetOperationBoundary(t *testing.T) {
	const directory = "ucupa-core/internal/facet"
	if !reviewedCoreFacetPackage("ucupa-core", directory) ||
		reviewedCoreFacetPackage("ucupa-facet", directory) ||
		reviewedCoreFacetPackage("ucupa-core", directory+"/child") {
		t.Fatal("Core Facet operation registration must identify exactly one package")
	}
	for _, file := range []string{
		directory + "/operation.go",
		directory + "/control.go",
		directory + "/read.go",
		directory + "/public_control.go",
		directory + "/service_attachment.go",
		directory + "/service_proposal.go",
		directory + "/operation_test.go",
		directory + "/public_control_test.go",
		directory + "/service_fixture_test.go",
		directory + "/service_test.go",
	} {
		if got := checkSourceLocation(file); got != "" {
			t.Errorf("reviewed Core Facet source %s: %s", file, got)
		}
	}
	for _, file := range []string{
		directory + "/extra.go",
		directory + "/control_extra_test.go",
		directory + "/child/operation.go",
	} {
		if got := checkSourceLocation(file); got == "" {
			t.Errorf("unreviewed Core Facet source accepted: %s", file)
		}
	}

	stdlib := testStdlib()
	for _, dependency := range []string{
		"errors", "fmt", "reflect", "sort", "testing", "time",
		"ucupa/shared/values",
		"ucupa/ucupa-core/contract",
		"ucupa/ucupa-core/internal/commit",
		"ucupa/ucupa-core/internal/commit/binding",
		"ucupa/ucupa-core/internal/facet/fact",
		"ucupa/ucupa-core/internal/token",
		"ucupa/ucupa-core/internal/access",
	} {
		if dependency == "errors" || dependency == "fmt" || dependency == "reflect" || dependency == "sort" || dependency == "testing" || dependency == "time" {
			stdlib[dependency] = true
		}
		if got := checkDependency(directory+"/operation.go", dependency, "ucupa", false, stdlib); got != "" {
			t.Errorf("allowed Core Facet dependency %s: %s", dependency, got)
		}
	}
	for _, dependency := range []string{
		"ucupa/ucupa-core",
		"ucupa/ucupa-core/internal/element/fact",
		"ucupa/ucupa-core/internal/element",
		"ucupa/ucupa-core/internal/mount/fact",
		"ucupa/ucupa-core/internal/query",
		"ucupa/ucupa-core/internal/store",
		"ucupa/ucupa-core/transport/grpc",
		"ucupa/ucupa-core/sdk/go",
		"ucupa/ucupa-facet/contract",
		"ucupa/composition",
		"go.etcd.io/bbolt",
	} {
		if got := checkDependency(directory+"/operation.go", dependency, "ucupa", false, stdlib); got == "" {
			t.Errorf("forbidden Core Facet dependency accepted: %s", dependency)
		}
	}

	for _, testFile := range []string{"operation_test.go", "public_control_test.go"} {
		if got := checkDependency(directory+"/"+testFile, "ucupa/ucupa-core/internal/element", "ucupa", true, stdlib); got != "" {
			t.Errorf("approved Facet fixture dependency rejected for %s: %s", testFile, got)
		}
	}
	if got := checkDependency(directory+"/control_test.go", "ucupa/ucupa-core/internal/element", "ucupa", true, stdlib); got == "" {
		t.Fatal("unapproved Core Facet test acquired an Element fixture dependency")
	}

	for _, tc := range []struct {
		source, dependency string
		allowed            bool
		test               bool
	}{
		{directory + "/operation.go", "ucupa/ucupa-core/internal/access", true, false},
		{"ucupa-core/instance/instance.go", "ucupa/ucupa-core/internal/facet", true, false},
		{"ucupa-core/internal/element/operation.go", "ucupa/ucupa-core/internal/facet", false, false},
		{"ucupa-core/internal/query/operation.go", "ucupa/ucupa-core/internal/facet", false, false},
		{"ucupa-core/internal/commit/state.go", "ucupa/ucupa-core/internal/facet", false, false},
	} {
		got := checkDependency(tc.source, tc.dependency, "ucupa", tc.test, stdlib)
		if (got == "") != tc.allowed {
			t.Errorf("%s -> %s: allowed=%v got=%q", tc.source, tc.dependency, tc.allowed, got)
		}
	}
}
