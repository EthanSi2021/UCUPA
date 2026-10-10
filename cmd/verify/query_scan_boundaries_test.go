package main

import "testing"

func TestCoreQueryScanBoundary(t *testing.T) {
	const dir = "ucupa-core/internal/query/scan"
	std := testStdlib()
	std["reflect"], std["errors"], std["fmt"], std["testing"] = true, true, true, true
	if !reviewedQueryScanPackage("ucupa-core", dir) || checkSourceLocation(dir+"/candidate.go") != "" {
		t.Fatal("query scan boundary not registered")
	}
	for _, dep := range []string{"errors", "ucupa/shared/values", "ucupa/ucupa-core/contract", "ucupa/ucupa-core/internal/element/fact", "ucupa/ucupa-core/internal/geometry", "ucupa/ucupa-core/internal/mount/fact", "ucupa/ucupa-core/internal/query/request"} {
		if got := checkDependency(dir+"/candidate.go", dep, "ucupa", false, std); got != "" {
			t.Errorf("%s: %s", dep, got)
		}
	}
	for _, dep := range []string{"ucupa/ucupa-core", "ucupa/ucupa-core/instance", "ucupa/ucupa-core/internal/facet/fact", "ucupa/ucupa-core/internal/store", "ucupa/ucupa-core/internal/token", "ucupa/ucupa-core/transport/grpc", "ucupa/ucupa-core/sdk/go", "ucupa/ucupa-realm", "go.etcd.io/bbolt"} {
		for _, test := range []bool{false, true} {
			if checkDependency(dir+"/candidate.go", dep, "ucupa", test, std) == "" {
				t.Errorf("unexpected scan dependency %s test=%v", dep, test)
			}
		}
	}
	for _, file := range []string{"ucupa-core/internal/query/extra.go", dir + "/extra/extra.go", "ucupa-facet/internal/query/scan/scan.go"} {
		if checkSourceLocation(file) == "" {
			t.Errorf("unreviewed scan boundary accepted %s", file)
		}
	}
	for _, tc := range []struct {
		file    string
		allowed bool
	}{
		{"ucupa-core/query.go", true}, {"ucupa-core/contract/query.go", false},
		{"ucupa-core/internal/token/query.go", false}, {"ucupa-core/internal/element/fact/query.go", false},
		{"ucupa-core/internal/query/request/query.go", false}, {"ucupa-core/internal/geometry/query.go", false},
		{"ucupa-core/transport/grpc/query.go", false}, {"ucupa-core/sdk/go/query.go", false},
		{"ucupa-core/transport/grpc/internal/wire/query.go", false},
		{"ucupa-facet/internal/namespace/query.go", false}, {"ucupa-realm/realm.go", false}, {"composition/query.go", false},
	} {
		for _, test := range []bool{false, true} {
			if got := checkDependency(tc.file, "ucupa/"+dir, "ucupa", test, std); (got == "") != tc.allowed {
				t.Errorf("%s test=%v allowed=%v: %s", tc.file, test, tc.allowed, got)
			}
		}
	}
}
