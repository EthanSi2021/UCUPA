package main

import (
	"strings"
	"testing"
)

func TestReviewedFacetRequestBoundary(t *testing.T) {
	const directory = "ucupa-facet/internal/namespace/request"
	if !reviewedRequestPackage("ucupa-facet", directory) {
		t.Fatal("Facet request-value boundary is not registered")
	}
	if reviewedRequestPackage("ucupa-core", "ucupa-core/internal/namespace/request") {
		t.Fatal("request registration leaked to another owner")
	}
	for _, file := range []string{"encoding.go", "access.go", "condition.go", "encoding_test.go"} {
		if got := checkSourceLocation(directory + "/" + file); got != "" {
			t.Errorf("reviewed request source rejected: %s: %s", file, got)
		}
	}
	for _, file := range []string{
		"ucupa-facet/internal/namespace/extra/namespace.go",
		"ucupa-facet/internal/namespace/request/extra/extra.go",
		"ucupa-facet/internal/access/extra/access.go",
		"ucupa-core/internal/namespace/request/encoding.go",
	} {
		if got := checkSourceLocation(file); got == "" {
			t.Errorf("unreviewed source accepted by request registration: %s", file)
		}
	}
	stdlib := testStdlib()
	for _, dependency := range []string{"bytes", "encoding/binary", "testing"} {
		stdlib[dependency] = true
	}
	for _, dependency := range []string{"bytes", "encoding/binary", "ucupa/shared/values"} {
		if got := checkDependency(directory+"/encoding.go", dependency, "ucupa", false, stdlib); got != "" {
			t.Errorf("pure request dependency rejected: %s: %s", dependency, got)
		}
	}
	for _, dependency := range []string{
		"ucupa/ucupa-facet", "ucupa/ucupa-facet/instance", "ucupa/ucupa-facet/contract",
		"ucupa/ucupa-facet/internal/token", "ucupa/ucupa-facet/internal/namespace",
		"ucupa/ucupa-facet/internal/commit", "ucupa/ucupa-facet/internal/store",
		"ucupa/ucupa-facet/transport/grpc", "ucupa/ucupa-facet/sdk/go",
		"ucupa/ucupa-realm", "ucupa/composition", "go.etcd.io/bbolt",
	} {
		for _, test := range []bool{false, true} {
			if got := checkDependency(directory+"/access.go", dependency, "ucupa", test, stdlib); got == "" {
				t.Errorf("request rule accepted reverse/state dependency: %s (test=%v)", dependency, test)
			}
		}
	}
	if got := checkDependency(directory+"/encoding_test.go", "ucupa/"+directory, "ucupa", true, stdlib); got != "" {
		t.Fatalf("external test cannot use request API: %s", got)
	}
}

func TestFacetRequestConsumersStayInsideOwner(t *testing.T) {
	for _, tc := range []struct {
		source  string
		allowed bool
	}{
		{"ucupa-facet/internal/namespace/public_mutation.go", true},
		{"ucupa-facet/internal/namespace/state.go", true},
		{"ucupa-facet/contract/contract.go", false},
		{"ucupa-facet/internal/token/token.go", false},
		{"ucupa-facet/transport/grpc/handler.go", false},
		{"ucupa-facet/sdk/go/client.go", false},
		{"ucupa-core/core.go", false},
		{"ucupa-realm/realm.go", false},
		{"composition/ports.go", false},
		{"acceptance/facet_test.go", false},
	} {
		got := checkDependency(tc.source, "ucupa/ucupa-facet/internal/namespace/request", "ucupa", strings.HasSuffix(tc.source, "_test.go"), testStdlib())
		if (got == "") != tc.allowed {
			t.Errorf("%s -> request: allowed=%v, got=%q", tc.source, tc.allowed, got)
		}
	}
}
