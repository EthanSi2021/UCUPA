package main

import (
	"strings"
	"testing"
)

func TestReviewedCoreGeometryBoundary(t *testing.T) {
	const directory = "ucupa-core/internal/geometry"
	if !reviewedGeometryPackage("ucupa-core", directory) {
		t.Fatal("Core geometry package is not registered")
	}
	if reviewedGeometryPackage("ucupa-core", directory+"/child") {
		t.Fatal("geometry registration accepted a child package")
	}
	if reviewedGeometryPackage("ucupa-facet", "ucupa-facet/internal/geometry") {
		t.Fatal("geometry registration leaked to Facet")
	}
	for _, file := range []string{
		directory + "/pose.go",
		directory + "/geometry.go",
		directory + "/intersection.go",
		directory + "/geometry_test.go",
	} {
		if got := checkSourceLocation(file); got != "" {
			t.Errorf("registered geometry source rejected: %s: %s", file, got)
		}
	}
	for _, file := range []string{
		"ucupa-core/internal/geometry/child/child.go",
		"ucupa-core/internal/other/geometry.go",
		"ucupa-facet/internal/geometry/geometry.go",
		"ucupa-core/internal/geometry/geometry/extra.go",
	} {
		if got := checkSourceLocation(file); got == "" {
			t.Errorf("unreviewed geometry source accepted: %s", file)
		}
	}

	stdlib := testStdlib()
	for _, dependency := range []string{"errors", "math", "math/big"} {
		stdlib[dependency] = true
	}
	for _, dependency := range []string{"errors", "math", "math/big"} {
		if got := checkDependency(directory+"/geometry.go", dependency, "ucupa", false, stdlib); got != "" {
			t.Errorf("geometry production dependency rejected: %s: %s", dependency, got)
		}
	}
	if got := checkDependency(directory+"/geometry_test.go", "testing", "ucupa", true, stdlib); got != "" {
		t.Errorf("geometry test dependency rejected: %s", got)
	}
	if got := checkDependency(directory+"/geometry_test.go", "ucupa/"+directory, "ucupa", true, stdlib); got != "" {
		t.Errorf("geometry self-test dependency rejected: %s", got)
	}
	for _, tc := range []struct {
		source, dependency string
		test               bool
	}{
		{directory + "/pose.go", "ucupa/ucupa-core", false},
		{directory + "/pose.go", "ucupa/ucupa-core/contract", false},
		{directory + "/pose.go", "ucupa/ucupa-core/internal/token", false},
		{directory + "/pose.go", "ucupa/ucupa-core/internal/commit", false},
		{directory + "/pose.go", "ucupa/ucupa-core/internal/store", false},
		{directory + "/pose.go", "ucupa/ucupa-facet/contract", false},
		{directory + "/pose.go", "ucupa/ucupa-facet/internal/token", false},
		{directory + "/pose.go", "ucupa/ucupa-realm", false},
		{directory + "/pose.go", "ucupa/composition", false},
		{directory + "/pose.go", "go.etcd.io/bbolt", false},
		{"ucupa-facet/contract/contract.go", "ucupa/ucupa-core/internal/geometry", false},
		{"ucupa-core/internal/store/format.go", "ucupa/ucupa-core/internal/geometry", false},
		{"ucupa-core/transport/grpc/wire.go", "ucupa/ucupa-core/internal/geometry", false},
		{"ucupa-core/sdk/go/client.go", "ucupa/ucupa-core/internal/geometry", false},
		{"ucupa-facet/internal/token/token.go", "ucupa/ucupa-core/internal/geometry", false},
		{"ucupa-facet/facet.go", "ucupa/ucupa-core/internal/geometry", false},
		{"composition/ports.go", "ucupa/ucupa-core/internal/geometry", false},
		{"acceptance/lifecycle_test.go", "ucupa/ucupa-core/internal/geometry", false},
	} {
		got := checkDependency(tc.source, tc.dependency, "ucupa", tc.test, stdlib)
		if got == "" {
			t.Errorf("accepted forbidden geometry edge %s -> %s", tc.source, tc.dependency)
		}
	}

	for _, tc := range []struct {
		source, dependency string
		allowed            bool
	}{
		{"ucupa-core/contract/spatial.go", "ucupa/ucupa-core/internal/geometry", true},
		{"ucupa-core/contract/spatial_test.go", "ucupa/ucupa-core/internal/geometry", true},
		{"ucupa-core/contract/spatial.go", "ucupa/ucupa-core/internal/geometry", true},
		{"ucupa-facet/contract/contract.go", "ucupa/ucupa-core/internal/geometry", false},
		{"ucupa-core/internal/geometry/pose.go", "ucupa/ucupa-core/internal/geometry", false},
	} {
		got := checkDependency(tc.source, tc.dependency, "ucupa", strings.HasSuffix(tc.source, "_test.go"), stdlib)
		if (got == "") != tc.allowed {
			t.Errorf("%s -> %s: allowed=%v got=%q", tc.source, tc.dependency, tc.allowed, got)
		}
	}
}
