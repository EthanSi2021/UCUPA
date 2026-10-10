package main

import "testing"

func TestReviewedCoreFacetFactBoundary(t *testing.T) {
	const directory = "ucupa-core/internal/facet/fact"
	if checkSourceLocation(directory+"/state.go") != "" {
		t.Fatal("reviewed Core Facet fact location rejected")
	}
	for _, other := range []string{directory + "/extra", "ucupa-core/internal/facet", "ucupa-facet/internal/facet/fact"} {
		if checkSourceLocation(other+"/state.go") == "" {
			t.Fatalf("unreviewed package accepted: %s", other)
		}
	}
	stdlib := testStdlib()
	for _, dependency := range []string{"ucupa/shared/values", "ucupa/ucupa-core/contract", "ucupa/ucupa-core/internal/token"} {
		if reason := checkDependency(directory+"/state.go", dependency, "ucupa", false, stdlib); reason != "" {
			t.Errorf("allowed edge rejected: %s: %s", dependency, reason)
		}
	}
	for _, dependency := range []string{"ucupa/ucupa-core", "ucupa/ucupa-core/instance", "ucupa/ucupa-core/internal/commit", "ucupa/ucupa-core/internal/store", "ucupa/ucupa-core/internal/element/fact", "ucupa/ucupa-core/internal/geometry", "ucupa/ucupa-core/internal/mount/fact", "ucupa/ucupa-realm", "ucupa/ucupa-facet/internal/namespace", "ucupa/ucupa-core/transport/grpc", "go.etcd.io/bbolt"} {
		if checkDependency(directory+"/state.go", dependency, "ucupa", false, stdlib) == "" {
			t.Errorf("forbidden edge accepted: %s", dependency)
		}
	}
	for _, source := range []string{"ucupa-core/contract/record.go", "ucupa-core/internal/token/token.go", "ucupa-core/internal/store/store.go", "ucupa-core/internal/element/fact/state.go", "ucupa-core/transport/grpc/server.go", "ucupa-core/sdk/go/client.go", "composition/ports.go", "ucupa-facet/internal/namespace/content.go"} {
		if checkDependency(source, "ucupa/"+directory, "ucupa", false, stdlib) == "" {
			t.Errorf("private fact boundary crossed from %s", source)
		}
	}
	if reason := checkDependency("ucupa-core/attachment_transition.go", "ucupa/"+directory, "ucupa", false, stdlib); reason != "" {
		t.Fatalf("actual Core commit consumer rejected: %s", reason)
	}
}
