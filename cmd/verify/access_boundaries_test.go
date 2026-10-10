package main

import "testing"

func TestFacetAccessBoundary(t *testing.T) {
	const dir = "ucupa-facet/internal/access"
	if !reviewedAccessPackage("ucupa-facet", dir) {
		t.Fatal("access unregistered")
	}
	for _, path := range []string{"ucupa-facet/internal/access/extra/extra.go", "ucupa-core/internal/access/access.go"} {
		if checkSourceLocation(path) == "" {
			t.Fatalf("unreviewed access source: %s", path)
		}
	}
	std := testStdlib()
	for _, p := range []string{"sync", "sync/atomic", "time"} {
		std[p] = true
	}
	for _, p := range []string{"sync", "sync/atomic", "time", "ucupa/shared/values", "ucupa/" + dir, "ucupa/ucupa-facet/internal/namespace/request"} {
		if got := checkDependency(dir+"/access_test.go", p, "ucupa", true, std); got != "" {
			t.Errorf("allowed dependency %s: %s", p, got)
		}
	}
	for _, p := range []string{"ucupa/ucupa-facet", "ucupa/ucupa-facet/instance", "ucupa/ucupa-facet/contract", "ucupa/ucupa-facet/internal/namespace", "ucupa/ucupa-facet/internal/token", "ucupa/ucupa-facet/internal/commit", "ucupa/ucupa-facet/internal/store", "ucupa/ucupa-core/contract", "ucupa/ucupa-realm", "ucupa/composition", "go.etcd.io/bbolt"} {
		for _, test := range []bool{false, true} {
			if checkDependency(dir+"/access.go", p, "ucupa", test, std) == "" {
				t.Errorf("access accepted fact/owner dependency %s test=%v", p, test)
			}
		}
	}
	for _, source := range []string{"ucupa-facet/contract/contract.go", "ucupa-facet/internal/token/token.go", "ucupa-facet/internal/namespace/request/access.go", "ucupa-facet/transport/grpc/handler.go", "ucupa-facet/sdk/go/client.go", "ucupa-core/core.go", "ucupa-realm/realm.go", "composition/assembly.go", "acceptance/facet_test.go"} {
		if checkDependency(source, "ucupa/"+dir, "ucupa", false, std) == "" {
			t.Errorf("forbidden resource consumer %s", source)
		}
	}
	if got := checkDependency("ucupa-facet/internal/namespace/public_context.go", "ucupa/"+dir, "ucupa", false, std); got != "" {
		t.Fatal(got)
	}
}
