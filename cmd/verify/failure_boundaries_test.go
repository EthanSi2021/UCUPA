package main

import (
	"strings"
	"testing"
)

func TestReviewedCoreSentinelIdentityBoundary(t *testing.T) {
	const directory = "ucupa-core/internal/failure"
	if !reviewedFailurePackage("ucupa-core", directory) || reviewedFailurePackage("ucupa-core", directory+"/child") || reviewedFailurePackage("ucupa-facet", "ucupa-facet/internal/failure") {
		t.Fatal("sentinel registration must identify exactly one private package")
	}
	if got := checkSourceLocation(directory + "/identity.go"); got != "" {
		t.Fatal(got)
	}
	for _, file := range []string{directory + "/child/identity.go", "ucupa-facet/internal/failure/identity.go"} {
		if checkSourceLocation(file) == "" {
			t.Fatalf("unreviewed sentinel package accepted: %s", file)
		}
	}
	stdlib := testStdlib()
	stdlib["errors"] = true
	for _, tc := range []struct {
		source, target string
		allowed        bool
	}{
		{directory + "/identity.go", "errors", true},
		{directory + "/identity_test.go", "testing", true},
		{directory + "/identity_test.go", "ucupa/" + directory, true},
		{directory + "/identity.go", "ucupa/" + directory, false},
		{directory + "/identity.go", "ucupa/shared/values", false},
		{directory + "/identity.go", "ucupa/ucupa-core", false},
		{directory + "/identity.go", "ucupa/ucupa-core/contract", false},
		{directory + "/identity.go", "ucupa/ucupa-core/internal/token", false},
		{directory + "/identity.go", "ucupa/ucupa-core/internal/geometry", false},
		{directory + "/identity.go", "go.etcd.io/bbolt", false},
		{"ucupa-core/internal/token/revision.go", "ucupa/" + directory, true},
		{"ucupa-core/internal/geometry/pose.go", "ucupa/" + directory, true},
		{"ucupa-facet/internal/token/token.go", "ucupa/" + directory, false},
		{"ucupa-core/transport/grpc/wire.go", "ucupa/" + directory, false},
		{"ucupa-core/sdk/go/client.go", "ucupa/" + directory, false},
		{"composition/adapters.go", "ucupa/" + directory, false},
		{"acceptance/identity_test.go", "ucupa/" + directory, false},
	} {
		got := checkDependency(tc.source, tc.target, "ucupa", strings.HasSuffix(tc.source, "_test.go"), stdlib)
		if (got == "") != tc.allowed {
			t.Errorf("%s -> %s: allowed=%v got=%q", tc.source, tc.target, tc.allowed, got)
		}
	}
}
