package main

import "testing"

func TestCoreObservationProjectionBoundary(t *testing.T) {
	const dir = "ucupa-core/internal/access/observe"
	std := testStdlib()
	std["reflect"], std["testing"] = true, true
	if !reviewedCoreObservationPackage("ucupa-core", dir) || checkSourceLocation(dir+"/changes.go") != "" {
		t.Fatal("observation projection not registered")
	}
	for _, dep := range []string{"reflect", "ucupa/shared/values", "ucupa/ucupa-core/contract", "ucupa/ucupa-core/internal/element/fact", "ucupa/ucupa-core/internal/mount/fact", "ucupa/ucupa-core/internal/facet/fact"} {
		if reason := checkDependency(dir+"/changes.go", dep, "ucupa", false, std); reason != "" {
			t.Fatal(dep, reason)
		}
	}
	for _, dep := range []string{"ucupa/ucupa-core", "ucupa/ucupa-core/instance", "ucupa/ucupa-core/internal/commit", "ucupa/ucupa-core/internal/commit/binding", "ucupa/ucupa-core/internal/store", "ucupa/ucupa-core/internal/token", "ucupa/ucupa-core/transport/grpc", "ucupa/ucupa-core/sdk/go", "ucupa/ucupa-realm", "go.etcd.io/bbolt"} {
		for _, test := range []bool{false, true} {
			if checkDependency(dir+"/changes.go", dep, "ucupa", test, std) == "" {
				t.Fatal("forbidden observation dependency", dep, test)
			}
		}
	}
	for _, source := range []string{"ucupa-core/contract/observe.go", "ucupa-core/internal/element/fact/observe.go", "ucupa-core/internal/commit/binding/observe.go", "ucupa-core/internal/query/scan/observe.go", "ucupa-core/internal/token/observe.go", "ucupa-core/transport/grpc/observe.go", "ucupa-core/sdk/go/observe.go", "ucupa-facet/internal/access/observe.go", "composition/observe.go"} {
		for _, test := range []bool{false, true} {
			if checkDependency(source, "ucupa/"+dir, "ucupa", test, std) == "" {
				t.Fatal("unreviewed observer consumer", source, test)
			}
		}
	}
	if reason := checkDependency("ucupa-core/public_observe.go", "ucupa/"+dir, "ucupa", false, std); reason != "" {
		t.Fatal(reason)
	}
	for _, file := range []string{dir + "/extra/extra.go", "ucupa-facet/internal/access/observe/observe.go"} {
		if checkSourceLocation(file) == "" {
			t.Fatal("unreviewed package accepted", file)
		}
	}
}
