package main

import "testing"

func TestReviewedCoreElementReadBoundary(t *testing.T) {
	const directory = "ucupa-core/internal/element/read"
	if !reviewedElementReadPackage("ucupa-core", directory) ||
		reviewedElementReadPackage("ucupa-facet", directory) ||
		reviewedElementReadPackage("ucupa-core", directory+"/child") {
		t.Fatal("Core Element read registration must identify exactly one package")
	}
	for _, file := range []string{directory + "/element.go", directory + "/direct.go"} {
		if got := checkSourceLocation(file); got != "" {
			t.Errorf("reviewed Element read source %s: %s", file, got)
		}
	}
	for _, file := range []string{
		directory + "/read_test.go",
		directory + "/extra.go",
		directory + "/child/element.go",
	} {
		if got := checkSourceLocation(file); got == "" {
			t.Errorf("unreviewed Element read source accepted: %s", file)
		}
	}

	stdlib := testStdlib()
	stdlib["context"], stdlib["sort"] = true, true
	for _, dependency := range []string{"context", "sort", "ucupa/shared/values", "ucupa/ucupa-core/contract", "ucupa/ucupa-core/internal/element/fact", "ucupa/ucupa-core/internal/facet/fact"} {
		if got := checkDependency(directory+"/direct.go", dependency, "ucupa", false, stdlib); got != "" {
			t.Errorf("allowed Element read dependency %s: %s", dependency, got)
		}
	}
	for _, dependency := range []string{
		"ucupa/ucupa-core",
		"ucupa/ucupa-core/internal/element",
		"ucupa/ucupa-core/internal/geometry",
		"ucupa/ucupa-core/internal/mount/fact",
		"ucupa/ucupa-core/internal/query",
		"ucupa/ucupa-core/internal/query/scan",
		"ucupa/ucupa-core/internal/commit",
		"ucupa/ucupa-core/internal/token",
		"ucupa/ucupa-core/internal/store",
		"ucupa/ucupa-core/transport/grpc",
		"ucupa/ucupa-core/sdk/go",
		"ucupa/ucupa-facet/contract",
		"ucupa/composition",
		"go.etcd.io/bbolt",
	} {
		if got := checkDependency(directory+"/element.go", dependency, "ucupa", false, stdlib); got == "" {
			t.Errorf("forbidden Element read dependency accepted: %s", dependency)
		}
	}

	for _, tc := range []struct {
		source, dependency string
		allowed            bool
	}{
		{"ucupa-core/internal/element/read.go", "ucupa/ucupa-core/internal/element/read", true},
		{"ucupa-core/internal/element/operation.go", "ucupa/ucupa-core/internal/element/read", true},
		{"ucupa-core/internal/query/operation.go", "ucupa/ucupa-core/internal/element/read", true},
		{"ucupa-core/internal/commit/state.go", "ucupa/ucupa-core/internal/element/read", false},
		{"ucupa-core/internal/element/fact/state.go", "ucupa/ucupa-core/internal/element/read", false},
		{"ucupa-core/internal/mount/fact/state.go", "ucupa/ucupa-core/internal/element/read", false},
		{"ucupa-core/transport/grpc/server.go", "ucupa/ucupa-core/internal/element/read", false},
		{"ucupa-core/sdk/go/client.go", "ucupa/ucupa-core/internal/element/read", false},
		{"composition/ports.go", "ucupa/ucupa-core/internal/element/read", false},
		{"ucupa-facet/internal/namespace/content.go", "ucupa/ucupa-core/internal/element/read", false},
	} {
		got := checkDependency(tc.source, tc.dependency, "ucupa", false, stdlib)
		if (got == "") != tc.allowed {
			t.Errorf("%s -> %s: allowed=%v got=%q", tc.source, tc.dependency, tc.allowed, got)
		}
	}
}

func TestReviewedCoreElementAdjacencyFixtureBoundary(t *testing.T) {
	const file = "ucupa-core/internal/element/read/adjacency_test.go"
	stdlib := testStdlib()
	for _, dependency := range []string{"context", "crypto/rand", "fmt", "reflect", "testing"} {
		stdlib[dependency] = true
	}
	for _, dependency := range []string{
		"ucupa/shared/values", "ucupa/ucupa-core/contract", "ucupa/ucupa-core/internal/commit",
		"ucupa/ucupa-core/internal/element", "ucupa/ucupa-core/internal/element/read",
	} {
		if got := checkDependency(file, dependency, "ucupa", true, stdlib); got != "" {
			t.Errorf("allowed Element adjacency dependency %s: %s", dependency, got)
		}
	}
	for _, dependency := range []string{
		"ucupa/ucupa-core", "ucupa/ucupa-core/internal/element/fact", "ucupa/ucupa-core/internal/facet/fact",
		"ucupa/ucupa-core/internal/mount", "ucupa/ucupa-core/internal/query", "ucupa/ucupa-core/internal/store",
		"ucupa/ucupa-core/internal/token", "ucupa/ucupa-core/transport/grpc", "ucupa/ucupa-core/sdk/go",
		"go.etcd.io/bbolt",
	} {
		if got := checkDependency(file, dependency, "ucupa", true, stdlib); got == "" {
			t.Errorf("forbidden Element adjacency dependency accepted: %s", dependency)
		}
	}
	for _, source := range []string{
		"ucupa-core/internal/element/read/extra_test.go",
		"ucupa-core/internal/element/read/adjacency_extra_test.go",
	} {
		if got := checkSourceLocation(source); got == "" {
			t.Errorf("unreviewed Element adjacency source accepted: %s", source)
		}
	}
	if got := checkDependency("ucupa-core/internal/element/read/adjacency_extra_test.go", "ucupa/ucupa-core/internal/commit", "ucupa", true, stdlib); got == "" {
		t.Fatal("unreviewed Element adjacency dependency accepted")
	}
}
