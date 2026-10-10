package main

import "testing"

func TestReviewedCoreInstanceScenarioBoundary(t *testing.T) {
	const directory = "ucupa-core/tests/instance"
	if !reviewedCoreInstanceTestPackage("ucupa-core", directory) ||
		reviewedCoreInstanceTestPackage("ucupa-facet", directory) ||
		reviewedCoreInstanceTestPackage("ucupa-core", directory+"/child") {
		t.Fatal("instance scenarios must identify exactly one Core test package")
	}
	for _, name := range []string{"foundation_test.go", "durability_test.go", "surface_test.go"} {
		if got := checkSourceLocation(directory + "/" + name); got != "" {
			t.Errorf("reviewed instance scenario %s rejected: %s", name, got)
		}
	}
	for _, file := range []string{
		directory + "/extra_test.go", directory + "/foundation.go", directory + "/child/durability_test.go",
		"ucupa-facet/tests/instance/foundation_test.go",
	} {
		if got := checkSourceLocation(file); got == "" {
			t.Errorf("unreviewed instance scenario accepted: %s", file)
		}
	}
	stdlib := testStdlib()
	stdlib["testing"], stdlib["errors"], stdlib["path/filepath"] = true, true, true
	for _, imported := range []string{
		"testing", "errors", "path/filepath", "ucupa/shared/values", "ucupa/ucupa-core/instance", "ucupa/ucupa-core/contract",
	} {
		if got := checkDependency(directory+"/foundation_test.go", imported, "ucupa", true, stdlib); got != "" {
			t.Errorf("public instance dependency %s rejected: %s", imported, got)
		}
		if got := checkDependency(directory+"/foundation.go", imported, "ucupa", false, stdlib); got == "" {
			t.Errorf("production code accepted in instance tests: %s", imported)
		}
	}
	for _, imported := range []string{
		"ucupa/ucupa-core/internal/element", "ucupa/ucupa-core/internal/commit", "ucupa/ucupa-core/internal/store",
		"ucupa/ucupa-core/internal/query", "ucupa/ucupa-core/internal/query/connected", "ucupa/ucupa-core/internal/query/exchange",
		"ucupa/ucupa-core/transport/grpc", "ucupa/ucupa-core/sdk/go", "ucupa/ucupa-facet/contract",
		"ucupa/ucupa-core/tests/element", "ucupa/ucupa-core/tests/query", "go.etcd.io/bbolt",
	} {
		if got := checkDependency(directory+"/durability_test.go", imported, "ucupa", true, stdlib); got == "" {
			t.Errorf("private instance dependency accepted: %s", imported)
		}
	}
	for _, source := range []string{
		"ucupa-core/core.go", "ucupa-core/internal/commit/state.go", "ucupa-core/tests/query/snapshot_test.go",
		"ucupa-core/tests/element/core_test.go", "composition/cluster_test.go",
	} {
		if got := checkDependency(source, "ucupa/"+directory, "ucupa", true, stdlib); got == "" {
			t.Errorf("instance scenarios became a reusable test library: %s", source)
		}
	}
}
