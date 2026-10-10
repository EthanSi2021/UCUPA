package main

import "testing"

func TestCoreQueryRequestBoundary(t *testing.T) {
	const dir = "ucupa-core/internal/query/request"
	std := testStdlib()
	std["slices"], std["math"], std["testing"] = true, true, true
	if !reviewedQueryRequestPackage("ucupa-core", dir) || checkSourceLocation(dir+"/canonical.go") != "" {
		t.Fatal("query request boundary not registered")
	}
	for _, dep := range []string{"slices", "ucupa/shared/values", "ucupa/ucupa-core/internal/geometry", "ucupa/ucupa-core/internal/token"} {
		if got := checkDependency(dir+"/canonical.go", dep, "ucupa", false, std); got != "" {
			t.Errorf("%s: %s", dep, got)
		}
	}
	for _, dep := range []string{"ucupa/ucupa-core", "ucupa/ucupa-core/contract", "ucupa/ucupa-core/instance", "ucupa/ucupa-core/internal/element/fact", "ucupa/ucupa-core/internal/mount/fact", "ucupa/ucupa-core/internal/store", "ucupa/ucupa-core/transport/grpc", "ucupa/ucupa-core/sdk/go", "ucupa/ucupa-realm", "go.etcd.io/bbolt"} {
		for _, test := range []bool{false, true} {
			if checkDependency(dir+"/canonical.go", dep, "ucupa", test, std) == "" {
				t.Errorf("unexpected request dependency %s test=%v", dep, test)
			}
		}
	}
	for _, file := range []string{"ucupa-core/internal/query/extra.go", dir + "/extra/extra.go", "ucupa-facet/internal/query/request/query.go"} {
		if checkSourceLocation(file) == "" {
			t.Errorf("unreviewed query boundary accepted %s", file)
		}
	}
	for _, tc := range []struct {
		file    string
		allowed bool
	}{
		{"ucupa-core/query.go", true}, {"ucupa-core/contract/query.go", true},
		{"ucupa-core/internal/token/query.go", false}, {"ucupa-core/internal/geometry/query.go", false},
		{"ucupa-core/transport/grpc/query.go", false}, {"ucupa-core/sdk/go/query.go", false},
		{"ucupa-core/transport/grpc/internal/wire/query.go", false},
		{"ucupa-facet/internal/namespace/query.go", false}, {"ucupa-realm/realm.go", false}, {"composition/query.go", false},
	} {
		if got := checkDependency(tc.file, "ucupa/"+dir, "ucupa", false, std); (got == "") != tc.allowed {
			t.Errorf("%s allowed=%v: %s", tc.file, tc.allowed, got)
		}
	}
}
